package daemon

import (
	"context"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/fsctl/tless/pkg/backup"
	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

// Callback for rpc.DaemonCtlServer.Restore requests
func (s *server) Restore(stream pb.DaemonCtl_RestoreServer) error {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	// Read in all the RestoreRequest records from the client
	snapshotRawName := ""
	restorePath := ""
	selRelPaths := make([]string, 0)
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			goto EofReached
		}
		if err != nil {
			return err
		}
		snapshotRawName = req.SnapshotRawName
		restorePath = req.RestorePath
		selRelPaths = append(selRelPaths, req.SelectedRelPaths...)

		vlog.Printf("Got batch of %d paths", len(req.SelectedRelPaths))
	}

EofReached:
	log.Printf(">> GOT COMMAND: Restore (%s) into dir %s with %d rel paths selected", snapshotRawName, restorePath, len(selRelPaths))

	// Can we start a restore right now?  Judge from the current status
	gGlobalsLock.Lock()
	state := gStatus.state
	gGlobalsLock.Unlock()

	if state != Idle {
		if state == Restoring {
			log.Println("Cannot start restore right b/c we are already doing a restore")
			log.Println(">> COMPLETED COMMAND: Restore")
			return stream.SendAndClose(&pb.RestoreResponse{
				IsStarting: false,
				ErrMsg:     "already doing a restore right now",
			})
		} else {
			log.Println("Cannot start restore right now because we are not in Idle state")
			log.Println(">> COMPLETED COMMAND: Restore")
			return stream.SendAndClose(&pb.RestoreResponse{
				IsStarting: false,
				ErrMsg:     "busy with other work",
			})
		}
	} else {
		gGlobalsLock.Lock()
		gStatus.state = Restoring
		gStatus.msg = "Preparing to restore"
		gStatus.percentage = 0.0
		gGlobalsLock.Unlock()

		// Diagnostic print out of the rel paths to restore
		if len(selRelPaths) == 0 {
			vlog.Printf("Restoring:  entire snapshot")
		} else {
			vlog.Printf("Restoring:  selected rel paths")
			for _, relPath := range selRelPaths {
				log.Printf("+ %s", relPath)
			}
		}
	}

	go Restore(snapshotRawName, restorePath, selRelPaths,
		func() { log.Println(">> COMPLETED COMMAND: Restore") }, vlog)

	log.Println("Starting restore")
	return stream.SendAndClose(&pb.RestoreResponse{
		IsStarting: true,
		ErrMsg:     "",
	})
}

func Restore(snapshotRawName string, restorePath string, selectedRelPaths []string, completion func(), vlog *util.VLog) {
	// Last step:  call the completion routine
	defer completion()

	// to call when we exit early
	done := func() {
		lastBackupTimeFormatted := getLastBackupTimeFormatted(&gGlobalsLock)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.percentage = -1.0
		gStatus.msg = "Last backup: " + lastBackupTimeFormatted
		gGlobalsLock.Unlock()
	}

	// open connection to cloud server
	ctx := context.Background()
	key := make([]byte, 32)
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	copy(key, gKey)
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)
	if ok, err := objst.IsReachable(ctx, bucket, vlog); !ok {
		log.Println("error: exiting because server not reachable: ", err)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.percentage = -1.0
		gStatus.msg = "Cannot connect to cloud"
		gGlobalsLock.Unlock()
		return
	}

	// Split the name to get both parts
	parts := strings.Split(snapshotRawName, "/")
	if len(parts) != 2 {
		log.Printf("error: malformed restore snapshot: '%s'", snapshotRawName)
		done()
		return
	}
	backupName := parts[0]
	snapshotName := parts[1]

	// Encrypt the backup name and snapshot name so we can form the index file name
	encBackupName, err := cryptography.EncryptFilename(key, backupName)
	if err != nil {
		log.Printf("error: cannot encrypt backup name '%s': %v", backupName, err)
		done()
		return
	}
	encSnapshotName, err := cryptography.EncryptFilename(key, snapshotName)
	if err != nil {
		log.Printf("error: cannot encrypt snapshot name '%s': %v", backupName, err)
		done()
		return
	}

	// Get the snapshot index
	encSsIndexObjName := encBackupName + "/@" + encSnapshotName
	ssIndexJson, err := snapshots.GetSnapshotIndexFile(ctx, objst, bucket, key, encSsIndexObjName)
	if err != nil {
		log.Printf("error: cannot get snapshot index file for '%s/%s': %v", backupName, snapshotName, err)
		done()
		return
	}
	snapshotObj, err := snapshots.UnmarshalSnapshotObj(ssIndexJson)
	if err != nil {
		log.Printf("error: cannot get snapshot object for '%s/%s': %v", backupName, snapshotName, err)
		done()
		return
	}

	// Filter the rel paths we want to restore
	mRelPathsObjsMap := backup.FilterRelPaths(snapshotObj, selectedRelPaths)
	totalItems := len(mRelPathsObjsMap)
	doneItems := 0
	vlog.Printf("RESTORE: have %d items to restore", totalItems)

	// Get uid/gid for user at the console daemon is working on behalf of
	gGlobalsLock.Lock()
	username := gUsername
	gGlobalsLock.Unlock()
	uid, gid, err := util.GetUidGid(username)
	if err != nil {
		log.Printf("error: cannot get user'%s's UID/GID: %v", username, err)
	}

	// Initialize a chunk cache
	cc := backup.NewChunkCache(objst, key, vlog, uid, gid)

	// For locality of reference reasons, we'll get the best cache hit rate if we restore in lexiconigraphical
	// order of rel paths.
	relPathKeys := make([]string, 0, len(mRelPathsObjsMap))
	for relPath := range mRelPathsObjsMap {
		relPathKeys = append(relPathKeys, relPath)
	}
	sort.Strings(relPathKeys)

	// loop over all the relpaths and restore each
	dirChmodQueue := make([]backup.DirChmodQueueItem, 0) // all directory mode bits are set at end
	for _, relPath := range relPathKeys {
		// Check if cancel signal has been received
		util.LockIf(&gGlobalsLock)
		isCancelRequested := gCancelRequested
		util.UnlockIf(&gGlobalsLock)
		if isCancelRequested {
			util.LockIf(&gGlobalsLock)
			gCancelRequested = false
			util.UnlockIf(&gGlobalsLock)
			vlog.Println("RESTORING: Canceled restore")
			done()
			return
		}

		vlog.Printf("RESTORING: '%s' from %s/%s", relPath, backupName, snapshotName)

		err = backup.RestoreDirEntry(ctx, key, restorePath, mRelPathsObjsMap[relPath], backupName, snapshotName, relPath, objst, bucket, vlog, &dirChmodQueue, uid, gid, cc)
		if err != nil {
			log.Printf("error: could not restore a dir entry '%s'", relPath)
		}

		// Update the percentage done
		doneItems += 1
		percentDone := (float32(doneItems) / float32(totalItems)) * float32(100)
		gGlobalsLock.Lock()
		gStatus.state = Restoring
		gStatus.percentage = percentDone
		gStatus.msg = backupName
		gGlobalsLock.Unlock()
	}

	// Do all the queued up directory chmods
	for _, dirChmodQueueItem := range dirChmodQueue {
		err := os.Chmod(dirChmodQueueItem.AbsPath, dirChmodQueueItem.FinalMode)
		if err != nil {
			log.Printf("error: could not chmod dir '%s' to final %#o\n", dirChmodQueueItem.AbsPath, dirChmodQueueItem.FinalMode)
		}
	}

	// Print the cache hit rate to vlog for diagnostics
	cc.PrintCacheStatistics()

	done()
}

// Callback for rpc.DaemonCtlServer.Restore requests
func (s *server) CancelRestore(ctx context.Context, in *pb.CancelRequest) (*pb.CancelResponse, error) {
	log.Println(">> GOT COMMAND: CancelRestore")

	gGlobalsLock.Lock()
	gCancelRequested = true
	gStatus.msg = "Canceling..."
	gGlobalsLock.Unlock()

	return &pb.CancelResponse{
		IsStarting: true,
		ErrMsg:     "",
	}, nil
}
