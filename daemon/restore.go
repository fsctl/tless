package daemon

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/fsctl/tless/pkg/backup"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

// Callback for rpc.DaemonCtlServer.Restore requests
func (s *server) Restore(ctx context.Context, in *pb.RestoreRequest) (*pb.RestoreResponse, error) {
	log.Printf(">> GOT COMMAND: Restore (%s) into dir %s", in.GetSnapshotRawName(), in.GetRestorePath())

	// Can we start a backup right now?  Judge from the current status
	gGlobalsLock.Lock()
	state := gStatus.state
	gGlobalsLock.Unlock()

	if state != Idle {
		if state == Restoring {
			log.Println("Cannot start restore right b/c we are already doing a restore")
			log.Println(">> COMPLETED COMMAND: Restore")
			return &pb.RestoreResponse{
				IsStarting: false,
				ErrMsg:     "already doing a restore right now",
			}, nil
		} else {
			log.Println("Cannot start restore right now because we are not in Idle state")
			log.Println(">> COMPLETED COMMAND: Restore")
			return &pb.RestoreResponse{
				IsStarting: false,
				ErrMsg:     "busy with other work",
			}, nil
		}
	} else {
		gGlobalsLock.Lock()
		gStatus.state = Restoring
		gStatus.msg = "Preparing to restore"
		gStatus.percentage = 0.0
		gGlobalsLock.Unlock()

		// Diagnostic print out of the rel paths to restore
		selRelPaths := in.GetSelectedRelPaths()
		if len(selRelPaths) == 0 {
			log.Printf("Restoring:  entire snapshot")
		} else {
			log.Printf("Restoring:  selected rel paths")
			for _, relPath := range selRelPaths {
				log.Printf("+ %s", relPath)
			}
		}
	}

	go Restore(in.GetSnapshotRawName(),
		in.GetRestorePath(),
		in.GetSelectedRelPaths(),
		func() { log.Println(">> COMPLETED COMMAND: Restore") })

	log.Println("Starting restore")
	return &pb.RestoreResponse{
		IsStarting: true,
		ErrMsg:     "",
	}, nil
}

func Restore(snapshotRawName string, restorePath string, selectedRelPaths []string, completion func()) {
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
	encKey := make([]byte, 32)
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	copy(encKey, gKey)
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey)
	if ok, err := objst.IsReachableWithRetries(ctx, 10, bucket); !ok {
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
		log.Printf("Malformed restore snapshot: '%s'", snapshotRawName)
		done()
		return
	}
	backupName := parts[0]
	snapshotName := parts[1]

	// get all the relpaths for this snapshot
	var selectedRelPathsMap map[string]int
	log.Printf("RESTORE: selectedRelPaths = %v\n", selectedRelPaths)
	if len(selectedRelPaths) == 0 {
		selectedRelPathsMap = nil
	} else {
		selectedRelPathsMap := make(map[string]int, len(selectedRelPaths))
		for _, rp := range selectedRelPaths {
			selectedRelPathsMap[rp] = 1
		}
	}
	log.Printf("RESTORE: selectedRelPathsMap = %v\n", selectedRelPathsMap)
	mRelPathsObjsMap, err := objstorefs.ReconstructSnapshotFileList(ctx, objst, bucket, encKey, backupName, snapshotName, "", selectedRelPathsMap)
	if err != nil {
		log.Println("error: reconstructSnapshotFileList failed: ", err)
		done()
		return
	}
	totalItems := len(mRelPathsObjsMap)
	doneItems := 0
	log.Printf("RESTORE: have %d items to restore", totalItems)

	// Get uid/gid for user at the console daemon is working on behalf of
	gGlobalsLock.Lock()
	username := gUsername
	gGlobalsLock.Unlock()
	uid, gid, err := util.GetUidGid(username)
	if err != nil {
		log.Printf("error: cannot get user'%s's UID/GID: %v", username, err)
	}

	// loop over all the relpaths and restore each
	dirChmodQueue := make([]backup.DirChmodQueueItem, 0) // all directory mode bits are set at end
	for relPath := range mRelPathsObjsMap {
		log.Printf("RESTORING: '%s' from %s/%s", relPath, backupName, snapshotName)

		if len(mRelPathsObjsMap[relPath]) > 1 {
			relPathChunks := mRelPathsObjsMap[relPath]

			err = backup.RestoreDirEntryFromChunks(ctx, encKey, restorePath, relPathChunks, backupName, snapshotName, relPath, objst, bucket, false, uid, gid)
			if err != nil {
				log.Printf("error: could not restore a dir entry '%s'", relPath)
			}
		} else if len(mRelPathsObjsMap[relPath]) == 1 {
			objName := mRelPathsObjsMap[relPath][0]

			err = backup.RestoreDirEntry(ctx, encKey, restorePath, objName, backupName, snapshotName, relPath, objst, bucket, false, &dirChmodQueue, uid, gid)
			if err != nil {
				log.Printf("error: could not restore a dir entry '%s'", relPath)
			}
		} else {
			log.Fatalf("error: invalid number of chunks planned for %s", relPath)
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

	done()
}
