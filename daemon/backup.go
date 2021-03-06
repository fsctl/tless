package daemon

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsctl/tless/pkg/backup"
	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

// Callback for rpc.DaemonCtlServer.CheckConn requests
func (s *server) Backup(ctx context.Context, in *pb.BackupRequest) (*pb.BackupResponse, error) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Println(">> GOT COMMAND: Backup")

	// Can we start a backup right now?  Judge from the current status
	gGlobalsLock.Lock()
	state := gStatus.state
	gGlobalsLock.Unlock()

	if state != Idle {
		if state == BackingUp {
			log.Println("Cannot start backup right because we are already doing a backup")
			log.Println(">> COMPLETED COMMAND: Backup")
			return &pb.BackupResponse{
				IsStarting: false,
				ErrMsg:     "already doing a backup right now",
			}, nil
		} else {
			log.Println("Cannot start backup right now because we are not in Idle state")
			log.Println(">> COMPLETED COMMAND: Backup")
			return &pb.BackupResponse{
				IsStarting: false,
				ErrMsg:     "busy with other work",
			}, nil
		}
	} else {
		gGlobalsLock.Lock()
		gStatus.state = BackingUp
		gStatus.msg = "Preparing"
		gStatus.percentage = 0.0
		gGlobalsLock.Unlock()
	}

	// Force full backup?
	isForceFullBackup := in.ForceFullBackup
	if isForceFullBackup {
		gDbLock.Lock()
		err := gDb.ResetLastBackedUpTimeForAllDirents()
		gDbLock.Unlock()
		if err != nil {
			log.Println("error: failed to reset dirents last_backup times to zero: ", err)
			return &pb.BackupResponse{
				IsStarting: false,
				ErrMsg:     "internal database failure",
			}, nil
		}
		vlog.Println(">> Forcing full backup now")
	}

	go Backup(vlog, func() { log.Println(">> COMPLETED COMMAND: Backup") })

	vlog.Println("Starting backup")
	return &pb.BackupResponse{
		IsStarting: true,
		ErrMsg:     "",
	}, nil
}

func Backup(vlog *util.VLog, completion func()) {
	// Last step:  call the completion routine
	defer completion()

	// open connection to cloud server
	ctx := context.Background()
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	bucket := gCfg.Bucket
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)
	if ok, err := objst.IsReachable(ctx, bucket, vlog); !ok {
		log.Println("error: cloud server not reachable: ", err)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.percentage = -1.0
		gStatus.msg = "Cannot connect to cloud"
		gGlobalsLock.Unlock()
		return
	}

	// Make sure we have the latest bucket metadata in case user just wiped the bucket.
	gGlobalsLock.Lock()
	masterPassword := gCfg.MasterPassword
	gGlobalsLock.Unlock()
	salt, _, encKey, hmacKey, err := objst.GetOrCreateBucketMetadata(ctx, bucket, masterPassword, vlog)
	if err != nil || len(salt) == 0 {
		msg := fmt.Sprintf("error: could not read or initialize bucket metadata: %v", err)
		log.Println(msg)

		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.percentage = -1.0
		gStatus.msg = "Cannot init cloud bucket"
		gGlobalsLock.Unlock()
		return
	}

	gGlobalsLock.Lock()
	gCfg.Salt = salt
	gEncKey = encKey
	gHmacKey = hmacKey
	gGlobalsLock.Unlock()

	// Get a copy of the encryption key
	encKey = make([]byte, 32)
	gGlobalsLock.Lock()
	copy(encKey, gEncKey)
	gGlobalsLock.Unlock()

	// Now start backing up
	stats := backup.NewBackupStats()
	gGlobalsLock.Lock()
	dirs := gCfg.Dirs
	excludes := gCfg.ExcludePaths
	gGlobalsLock.Unlock()
	backupEndedInError := false
	backupEndedInCancelation := false
	for _, backupDirPath := range dirs {
		backupDirName := filepath.Base(backupDirPath)

		// log what iteration of the loop we're in
		vlog.Printf("Inspecting %s...\n", backupDirPath)

		// Display the backup dir we're doing this iteration of the loop
		// Each iteration of the loop we start back at 0%
		gGlobalsLock.Lock()
		gStatus.msg = backupDirName
		gStatus.percentage = 0.0
		gGlobalsLock.Unlock()

		// Set up closure for setting initial backup progress, capturing locks from here
		setBackupInitialProgressFunc := func(finished int64, total int64, backupDirName string, vlog *util.VLog) {
			percentDone := (float32(finished) / float32(total)) * float32(100)
			util.LockIf(&gGlobalsLock)
			gStatus.percentage = float32(percentDone)
			util.UnlockIf(&gGlobalsLock)
		}

		// Setup update progress callback
		updateBackupProgressFunc := func(finished int64, total int64, vlog *util.VLog) {
			percentDone := (float32(finished) / float32(total)) * float32(100)
			util.LockIf(&gGlobalsLock)
			gStatus.percentage = percentDone
			util.UnlockIf(&gGlobalsLock)
			vlog.Printf("%.2f%% written to cloud", percentDone)
		}

		// Define traversal cancel func
		checkAndHandleTraversalCancelation := func() bool {
			util.LockIf(&gGlobalsLock)
			defer util.UnlockIf(&gGlobalsLock)
			isCancelRequested := gCancelRequested
			gCancelRequested = false
			return isCancelRequested
		}

		// Set up backup cancelation closure capturing locks from here
		checkAndHandleBackupCancelationFunc := func(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, backupDirPath string, snapshotName string) bool {
			return checkAndHandleCancelation(ctx, key, objst, bucket, &gDbLock, gDb, &gGlobalsLock, backupDirPath, snapshotName)
		}

		// Traverse the FS for changed files and do the journaled backup
		util.LockIf(&gGlobalsLock)
		resourceUtilization := gCfg.ResourceUtilization
		util.UnlockIf(&gGlobalsLock)
		backupReportedEvents, breakFromLoop, continueLoop, fatalError := backup.DoJournaledBackup(ctx, encKey, objst, bucket, &gDbLock, gDb, backupDirPath, excludes, vlog, checkAndHandleTraversalCancelation, checkAndHandleBackupCancelationFunc, setBackupInitialProgressFunc, updateBackupProgressFunc, stats, resourceUtilization)
		for _, e := range backupReportedEvents {
			if e.Kind == util.ERR_OP_NOT_PERMITTED {
				backupEndedInError = true
			}
			gGlobalsLock.Lock()
			gStatus.reportedEvents = append(gStatus.reportedEvents, e)
			gGlobalsLock.Unlock()
		}
		if fatalError {
			backupEndedInError = true
			goto done
		}
		if continueLoop {
			continue
		}
		if breakFromLoop {
			backupEndedInCancelation = true
			break
		}

		// Bring percentage up to 100% for 2 seconds for aesthetics
		gGlobalsLock.Lock()
		gStatus.percentage = 100.0
		gGlobalsLock.Unlock()
		time.Sleep(time.Second * 2)
	}

	// Add a backup completed event only if none of the reported events were error type,
	// and backup was not canceled
	if backupEndedInError {
		gGlobalsLock.Lock()
		gStatus.reportedEvents = append(gStatus.reportedEvents, util.ReportedEvent{
			Kind:     util.INFO_BACKUP_COMPLETED_WITH_ERRORS,
			Path:     "",
			IsDir:    false,
			Datetime: time.Now().Unix(),
			Msg:      stats.FinalReport(),
		})
		gGlobalsLock.Unlock()
	} else if backupEndedInCancelation {
		gGlobalsLock.Lock()
		gStatus.reportedEvents = append(gStatus.reportedEvents, util.ReportedEvent{
			Kind:     util.INFO_BACKUP_CANCELED,
			Path:     "",
			IsDir:    false,
			Datetime: time.Now().Unix(),
			Msg:      "",
		})
		gGlobalsLock.Unlock()
	} else {
		gGlobalsLock.Lock()
		gStatus.reportedEvents = append(gStatus.reportedEvents, util.ReportedEvent{
			Kind:     util.INFO_BACKUP_COMPLETED,
			Path:     "",
			IsDir:    false,
			Datetime: time.Now().Unix(),
			Msg:      stats.FinalReport(),
		})
		gGlobalsLock.Unlock()
	}

done:
	// On finished, log the new total space usage
	persistUsage(true, true, vlog)

	// On finished, set the status back to Idle
	lastBackupTimeFormatted := getLastBackupTimeFormatted(&gDbLock)
	gGlobalsLock.Lock()
	gStatus.state = Idle
	gStatus.percentage = -1.0
	gStatus.msg = "Last backup: " + lastBackupTimeFormatted
	gGlobalsLock.Unlock()
}

func replayBackupJournal() {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	// Reconstruct obj store handle
	ctx := context.Background()
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)
	if ok, err := objst.IsReachable(ctx, bucket, vlog); !ok {
		log.Println("error: cloud server not reachable: ", err)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.percentage = -1.0
		gStatus.msg = "Cannot connect to cloud"
		gGlobalsLock.Unlock()
		return
	}

	// Get a copy of the encryption encKey
	encKey := make([]byte, 32)
	gGlobalsLock.Lock()
	copy(encKey, gEncKey)
	gGlobalsLock.Unlock()

	// Setup replay initial progress closure capturing locks from here
	setReplayInitialProgressFunc := func(finished int64, total int64, backupDirName string, vlog *util.VLog) {
		percentDone := (float32(finished) / float32(total)) * float32(100)
		util.LockIf(&gGlobalsLock)
		gStatus.state = BackingUp
		gStatus.msg = backupDirName
		gStatus.percentage = percentDone
		util.UnlockIf(&gGlobalsLock)
		vlog.Printf("%.2f%% written to cloud (starting replay)", percentDone)
	}

	// Setup update progress callback
	updateBackupProgressFunc := func(finished int64, total int64, vlog *util.VLog) {
		percentDone := (float32(finished) / float32(total)) * float32(100)
		util.LockIf(&gGlobalsLock)
		gStatus.percentage = percentDone
		util.UnlockIf(&gGlobalsLock)
		vlog.Printf("%.2f%% written to cloud (replay)", percentDone)
	}

	// Set up cancelation closure capturing locks from here
	checkAndHandleReplayCancelationFunc := func(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, backupDirPath string, snapshotName string) bool {
		return checkAndHandleCancelation(ctx, key, objst, bucket, &gDbLock, gDb, &gGlobalsLock, backupDirPath, snapshotName)
	}

	// Replay the journal
	gGlobalsLock.Lock()
	resourceUtilization := gCfg.ResourceUtilization
	gGlobalsLock.Unlock()
	re := backup.ReplayBackupJournal(ctx, encKey, objst, bucket, &gDbLock, gDb, vlog, setReplayInitialProgressFunc, checkAndHandleReplayCancelationFunc, updateBackupProgressFunc, resourceUtilization)
	gGlobalsLock.Lock()
	gStatus.reportedEvents = append(gStatus.reportedEvents, re)
	gGlobalsLock.Unlock()
	persistUsage(true, true, vlog)

	// Finally set the status back to Idle since we are done with backup
	lastBackupTimeFormatted := getLastBackupTimeFormatted(&gDbLock)
	gGlobalsLock.Lock()
	gStatus.state = Idle
	gStatus.percentage = -1.0
	gStatus.msg = "Last backup: " + lastBackupTimeFormatted
	gGlobalsLock.Unlock()
}

func checkAndHandleCancelation(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, dbLock *sync.Mutex, db *database.DB, globalsLock *sync.Mutex, backupDirPath string, snapshotName string) bool {
	util.LockIf(globalsLock)
	isCancelRequested := gCancelRequested
	util.UnlockIf(globalsLock)
	if isCancelRequested {
		cancelBackup(ctx, key, dbLock, db, globalsLock, backupDirPath, snapshotName, objst, bucket)
		util.LockIf(globalsLock)
		gCancelRequested = false
		util.UnlockIf(globalsLock)
		return true
	}
	return false
}

func cancelBackup(ctx context.Context, key []byte, dbLock *sync.Mutex, db *database.DB, globalsLock *sync.Mutex, backupDirPath string, snapshotName string, objst *objstore.ObjStore, bucket string) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	vlog.Printf("CANCEL: Starting unwind")

	// Update status to cleaning up, no percentage
	globalsLock.Lock()
	gStatus.state = CleaningUp
	gStatus.msg = "Canceling..."
	gStatus.percentage = 0.0
	globalsLock.Unlock()

	// Delete the snapshot we've been creating
	vlog.Printf("CANCEL: Deleting partially created snapshot")
	ssDel := snapshots.SnapshotForDeletion{
		BackupDirName: filepath.Base(backupDirPath),
		SnapshotName:  snapshotName,
	}
	err := snapshots.DeleteSnapshots(ctx, key, []snapshots.SnapshotForDeletion{ssDel}, objst, bucket, vlog, nil, nil)
	if err != nil {
		// This is ok and just means snapshot index file wasn't writetn to cloud yet
		log.Printf("warning: cancelBackup: could not delete partially created snapshot's index (probably doesn't exist yet): %v", err)

		// Garbage collect any orphaned chunks that were written while creating unused snapshot index file
		if err = snapshots.GCChunks(ctx, objst, bucket, key, vlog, nil, nil); err != nil {
			log.Println("error: handleReplay: could not garbage collect chunks: ", err)
		}
	}

	// Get all completed items in journal and set their dirents.last_backup time to 0
	vlog.Printf("CANCEL: Resetting last_backup times to zero for processed dirents")
	dbLock.Lock()
	err = db.CancelationResetLastBackupTime()
	dbLock.Unlock()
	if err != nil {
		log.Println("error: cancelBackup: db.CancelationResetLastBackupTime failed")
	}

	// Delete all items in journal + delete backup_info row so this doesn't look like a completed backup
	vlog.Printf("CANCEL: Clearing journal and deleting backup_info row")
	dbLock.Lock()
	err = db.CancelationCleanupJournal()
	dbLock.Unlock()
	if err != nil {
		log.Println("error: cancelBackup: db.CancelationCleanupJournal failed")
	}

	log.Println(">> COMPLETED COMMAND: CancelBackup")

	// (When we return, status will be set back to Idle)
}

func (s *server) CancelBackup(ctx context.Context, in *pb.CancelRequest) (*pb.CancelResponse, error) {
	log.Println(">> GOT COMMAND: CancelBackup")

	gGlobalsLock.Lock()
	gCancelRequested = true
	gStatus.msg = "Canceling..."
	gGlobalsLock.Unlock()

	return &pb.CancelResponse{
		IsStarting: true,
		ErrMsg:     "",
	}, nil
}
