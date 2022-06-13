package daemon

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsctl/tless/pkg/backup"
	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/fstraverse"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
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
			log.Println("Cannot start backup right b/c we are already doing a backup")
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
		gGlobalsLock.Lock()
		err := gDb.ResetLastBackedUpTimeForAllDirents()
		gGlobalsLock.Unlock()
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
	if ok, err := objst.IsReachableWithRetries(ctx, 10, bucket, vlog); !ok {
		log.Println("error: exiting because server not reachable: ", err)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.percentage = -1.0
		gStatus.msg = "Cannot connect to cloud"
		gGlobalsLock.Unlock()
		return
	}

	// Get a copy of the encryption key
	encKey := make([]byte, 32)
	gGlobalsLock.Lock()
	copy(encKey, gKey)
	gGlobalsLock.Unlock()

	gGlobalsLock.Lock()
	dirs := gCfg.Dirs
	excludePaths := gCfg.ExcludePaths
	gGlobalsLock.Unlock()
	for _, backupDirPath := range dirs {
		backupDirName := filepath.Base(backupDirPath)

		// log what iteration of the loop we're in
		vlog.Printf("Inspecting %s...\n", backupDirPath)

		// Display the backup dir we're doing this iteration of the loop
		gGlobalsLock.Lock()
		gStatus.msg = backupDirName
		gGlobalsLock.Unlock()

		// Traverse the filesystem looking for changed directory entries
		gGlobalsLock.Lock()
		prevPaths, err := gDb.GetAllKnownPaths(backupDirName)
		gGlobalsLock.Unlock()
		if err != nil {
			log.Printf("Error: cannot get paths list: %v", err)
			goto done
		}
		var backupIdsQueue fstraverse.BackupIdsQueue
		fstraverse.Traverse(backupDirPath, prevPaths, gDb, &gGlobalsLock, &backupIdsQueue, excludePaths)

		// Iterate over the queue of dirent id's inserting them into journal
		gGlobalsLock.Lock()
		insertBJTxn, err := gDb.NewInsertBackupJournalStmt(backupDirPath)
		gGlobalsLock.Unlock()
		if err != nil {
			log.Printf("Error: could not bulk insert into journal: %v", err)
			goto done
		}
		for _, dirEntId := range backupIdsQueue.Ids {
			gGlobalsLock.Lock()
			insertBJTxn.InsertBackupJournalRow(int64(dirEntId), database.Unstarted, database.Updated)
			gGlobalsLock.Unlock()
		}
		for deletedPath := range prevPaths {
			// deletedPath is backupDirName/deletedRelPath.  Make it just deletedRelPath
			deletedPath = strings.TrimPrefix(deletedPath, backupDirName)
			deletedPath = strings.TrimPrefix(deletedPath, "/")

			gGlobalsLock.Lock()
			isFound, _, dirEntId, err := gDb.HasDirEnt(backupDirName, deletedPath)
			gGlobalsLock.Unlock()
			if err != nil {
				log.Printf("error: while trying to find '%s'/'%s' in dirents: %v", backupDirName, deletedPath, err)
				continue
			}
			if !isFound {
				log.Printf("error: could not find '%s'/'%s' in dirents: %v", backupDirName, deletedPath, err)
				continue
			}
			vlog.Printf("Found deleted file '%s'/'%s' (dirents id = %d)", backupDirName, deletedPath, dirEntId)
			gGlobalsLock.Lock()
			insertBJTxn.InsertBackupJournalRow(int64(dirEntId), database.Unstarted, database.Deleted)
			gGlobalsLock.Unlock()
		}
		gGlobalsLock.Lock()
		insertBJTxn.Close()
		gGlobalsLock.Unlock()

		// Get the snapshot name from timestamp in backup_info
		gGlobalsLock.Lock()
		_, snapshotUnixtime, err := gDb.GetJournaledBackupInfo()
		gGlobalsLock.Unlock()
		if errors.Is(err, sql.ErrNoRows) {
			// If no rows were just inserted into journal, then nothing to backup for this snapshot
			vlog.Printf("nothing inserted in journal => nothing to back up")
			continue
		} else if err != nil {
			log.Printf("error: gDb.GetJournaledBackupInfo(): %v", err)
			goto done
		}
		snapshotName := time.Unix(snapshotUnixtime, 0).UTC().Format("2006-01-02_15.04.05")

		// Set the initial progress bar
		gGlobalsLock.Lock()
		finished, total, err := gDb.GetBackupJournalCounts()
		gGlobalsLock.Unlock()
		if err != nil {
			log.Printf("error: db.GetBackupJournalCounts: %v", err)
		}
		percentDone := (float32(finished)/float32(total))*float32(100) + 0.1
		gGlobalsLock.Lock()
		gStatus.percentage = float32(percentDone)
		gGlobalsLock.Unlock()

		breakFromLoop := playBackupJournal(ctx, encKey, gDb, &gGlobalsLock, backupDirPath, snapshotName, objst, bucket, vlog)
		if breakFromLoop {
			break
		}

		// Bring percentage up to 100% for 2 seconds for aesthetics
		gGlobalsLock.Lock()
		gStatus.percentage = 100.0
		gGlobalsLock.Unlock()
		time.Sleep(time.Second * 2)
	}

	// Finally set the status back to Idle since we are done with backup
done:
	lastBackupTimeFormatted := getLastBackupTimeFormatted(&gGlobalsLock)
	gGlobalsLock.Lock()
	gStatus.state = Idle
	gStatus.percentage = -1.0
	gStatus.msg = "Last backup: " + lastBackupTimeFormatted
	gGlobalsLock.Unlock()
}

func replayBackupJournal() {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	// Reset all InProgress -> Unstarted
	gGlobalsLock.Lock()
	err := gDb.ResetAllInProgressBackupJournalTasks()
	gGlobalsLock.Unlock()
	if err != nil {
		log.Println("error: gDb.ResetAllInProgressBackupJournalTasks: ", err)
	}

	// Reconstruct backupDirPath, backupDirName and snapshotName from backup_info table
	gGlobalsLock.Lock()
	backupDirPath, snapshotUnixtime, err := gDb.GetJournaledBackupInfo()
	gGlobalsLock.Unlock()
	if err != nil {
		log.Printf("error: gDb.GetJournaledBackupInfo(): %v", err)
	}
	backupDirName := filepath.Base(backupDirPath)
	snapshotName := time.Unix(snapshotUnixtime, 0).UTC().Format("2006-01-02_15.04.05")

	// Set the gStatus for backing up, with correct percentage done to start
	gGlobalsLock.Lock()
	finished, total, err := gDb.GetBackupJournalCounts()
	gGlobalsLock.Unlock()
	if err != nil {
		log.Printf("error: db.GetBackupJournalCounts: %v", err)
	}
	percentDone := (float32(finished)/float32(total))*float32(100) + 0.1
	gGlobalsLock.Lock()
	gStatus.state = BackingUp
	gStatus.msg = backupDirName
	gStatus.percentage = percentDone
	gGlobalsLock.Unlock()

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
	if ok, err := objst.IsReachableWithRetries(ctx, 10, bucket, vlog); !ok {
		log.Println("error: exiting because server not reachable: ", err)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.percentage = -1.0
		gStatus.msg = "Cannot connect to cloud"
		gGlobalsLock.Unlock()
		return
	}

	// Get a copy of the encryption key
	encKey := make([]byte, 32)
	gGlobalsLock.Lock()
	copy(encKey, gKey)
	gGlobalsLock.Unlock()

	// Roll the journal forward
	_ = playBackupJournal(ctx, encKey, gDb, &gGlobalsLock, backupDirPath, snapshotName, objst, bucket, vlog)

	// Finally set the status back to Idle since we are done with backup
	lastBackupTimeFormatted := getLastBackupTimeFormatted(&gGlobalsLock)
	gGlobalsLock.Lock()
	gStatus.state = Idle
	gStatus.percentage = -1.0
	gStatus.msg = "Last backup: " + lastBackupTimeFormatted
	gGlobalsLock.Unlock()
}

func playBackupJournal(ctx context.Context, key []byte, db *database.DB, globalsLock *sync.Mutex, backupDirPath string, snapshotName string, objst *objstore.ObjStore, bucket string, vlog *util.VLog) (breakFromLoop bool) {
	// By default, don't signal we want to break out of caller's loop over backups
	breakFromLoop = false

	for {
		// Sleep this go routine briefly on every iteration of the for loop
		time.Sleep(time.Millisecond * 50)

		// Has cancelation been requested?
		globalsLock.Lock()
		isCancelRequested := gCancelRequested
		globalsLock.Unlock()
		if isCancelRequested {
			cancelBackup(ctx, key, db, globalsLock, backupDirPath, snapshotName, objst, bucket)
			globalsLock.Lock()
			gCancelRequested = false
			globalsLock.Unlock()
			return true
		}

		globalsLock.Lock()
		bjt, err := db.ClaimNextBackupJournalTask()
		globalsLock.Unlock()
		if err != nil {
			if errors.Is(err, database.ErrNoWork) {
				vlog.Println("playBackupJournal: no work found in journal... done")
				return
			} else {
				log.Println("error: db.ClaimNextBackupJournalTask: ", err)
				return
			}
		}

		globalsLock.Lock()
		rootDirName, relPath, err := db.GetDirEntPaths(int(bjt.DirEntId))
		globalsLock.Unlock()
		if err != nil {
			log.Printf("error: db.GetDirEntPaths(): could not get dirent id '%d'\n", bjt.DirEntId)
		}

		if bjt.ChangeType == database.Updated {
			vlog.Printf("Backing up '%s/%s'", rootDirName, relPath)
			if err := backup.Backup(ctx, key, rootDirName, relPath, backupDirPath, snapshotName, objst, bucket, false); err != nil {
				log.Printf("error: backup.Backup(): %v", err)
				continue
			}
		} else if bjt.ChangeType == database.Deleted {
			vlog.Printf("Deleting '%s/%s'", rootDirName, relPath)
			if err = createDeletedPathKeyAndPurgeFromDb(ctx, objst, bucket, gDb, &gGlobalsLock, key, rootDirName, snapshotName, relPath); err != nil {
				log.Printf("error: failed on deleting path '%s': %v", relPath, err)
				continue
			}
		}

		globalsLock.Lock()
		err = db.UpdateLastBackupTime(int(bjt.DirEntId))
		globalsLock.Unlock()
		if err != nil {
			log.Printf("error: db.UpdateLastBackupTime(): %v", err)
		}

		globalsLock.Lock()
		isJournalComplete, err := db.CompleteBackupJournalTask(bjt)
		globalsLock.Unlock()
		if err != nil {
			log.Printf("error: db.CompleteBackupJournalTask: %v", err)
		}
		if isJournalComplete {
			vlog.Printf("Finished the journal (re)play")
			return
		} else {
			// Update the percentage on gStatus based on where we are now
			globalsLock.Lock()
			finished, total, err := db.GetBackupJournalCounts()
			globalsLock.Unlock()
			if err != nil {
				log.Printf("error: db.GetBackupJournalCounts: %v", err)
			} else {
				percentDone := (float32(finished)/float32(total))*float32(100) + 0.1
				globalsLock.Lock()
				gStatus.percentage = percentDone
				globalsLock.Unlock()
				vlog.Printf("%.2f%% done", percentDone)
			}
		}
	}
}

func cancelBackup(ctx context.Context, key []byte, db *database.DB, globalsLock *sync.Mutex, backupDirPath string, snapshotName string, objst *objstore.ObjStore, bucket string) {
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
	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctx, objst, key, bucket)
	if err != nil {
		log.Printf("Could not get grouped snapshots for cancelation: %v", err)
	}
	err = snapshots.DeleteSnapshot(ctx, key, groupedObjects, filepath.Base(backupDirPath), snapshotName, objst, bucket)
	if err != nil {
		log.Printf("Could not delete partially created snapshot: %v", err)
	}

	// Get all completed items in journal and set their dirents.last_backup time to 0
	vlog.Printf("CANCEL: Resetting last_backup times to zero for processed dirents")
	globalsLock.Lock()
	err = db.CancelationResetLastBackupTime()
	globalsLock.Unlock()
	if err != nil {
		log.Println("error: cancelBackup: db.CancelationResetLastBackupTime failed")
	}

	// Delete all items in journal + delete backup_info row so this doesn't look like a completed backup
	vlog.Printf("CANCEL: Clearing journal and deleting backup_info row")
	globalsLock.Lock()
	err = db.CancelationCleanupJournal()
	globalsLock.Unlock()
	if err != nil {
		log.Println("error: cancelBackup: db.CancelationCleanupJournal failed")
	}

	log.Println(">> COMPLETED COMMAND: CancelBackup")

	// (When we return, status will be set back to Idle)
}

func createDeletedPathKeyAndPurgeFromDb(ctx context.Context, objst *objstore.ObjStore, bucket string, db *database.DB, dbLock *sync.Mutex, key []byte, backupDirName string, snapshotName string, deletedPath string) error {
	// get the encrypted representation of backupDirName and snapshotName
	encryptedSnapshotName, err := cryptography.EncryptFilename(key, snapshotName)
	if err != nil {
		log.Printf("error: createDeletedPathsKeys(): could not encrypt snapshot name (%s): %v\n", snapshotName, err)
		return err
	}
	encryptedBackupDirName, err := cryptography.EncryptFilename(key, backupDirName)
	if err != nil {
		log.Printf("error: createDeletedPathsKeys(): could not encrypt backup dir name (%s): %v\n", backupDirName, err)
		return err
	}

	// encrypt the deleted path name
	encryptedDeletedRelPath, err := cryptography.EncryptFilename(key, deletedPath)
	if err != nil {
		log.Printf("error: createDeletedPathsKeys(): could not encrypt deleted rel path ('%s'): %v\n", deletedPath, err)
		return err
	}

	// Insert a slash in the middle of encrypted relPath b/c server won't
	// allow path components > 255 characters
	encryptedDeletedRelPath = backup.InsertSlashIntoEncRelPath(encryptedDeletedRelPath)

	// create an object in this snapshot like encBackupDirName/encSnapshotName/##encRelPath
	// where ## prefix indicates rel path was deleted since prev snapshot
	objName := encryptedBackupDirName + "/" + encryptedSnapshotName + "/##" + encryptedDeletedRelPath
	if err = objst.UploadObjFromBuffer(ctx, bucket, objName, make([]byte, 0), objstore.ComputeETag([]byte{})); err != nil {
		log.Printf("error: createDeletedPathsKeys(): could not UploadObjFromBuffer ('%s'): %v\n", objName, err)
		return err
	}

	// Delete dirents row for backupDirName/relPath
	dbLock.Lock()
	err = db.DeleteDirEntByPath(backupDirName, deletedPath)
	dbLock.Unlock()
	if err != nil {
		log.Printf("DeleteDirEntByPath failed: %v", err)
		return err
	}

	return nil
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
