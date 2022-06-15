package daemon

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

const (
	SendPartialResponseEveryNRelPaths int = 5_000
)

// Callback for rpc.DaemonCtlServer.ReadAllSnapshots requests
func (s *server) ReadAllSnapshots(in *pb.ReadAllSnapshotsRequest, srv pb.DaemonCtl_ReadAllSnapshotsServer) error {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Println(">> GOT COMMAND: ReadAllSnapshots")
	defer log.Println(">> COMPLETED COMMAND: ReadAllSnapshots")

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil && gKey != nil
	gGlobalsLock.Unlock()
	if !isGlobalConfigReady {
		log.Printf("ReadAllSnapshots: global config not yet initialized")
		resp := pb.ReadAllSnapshotsResponse{
			DidSucceed:      false,
			ErrMsg:          "global config not yet initialized",
			PartialSnapshot: nil,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}

	ctxBkg := context.Background()
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	key := gKey
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctxBkg, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctxBkg, objst, key, bucket, vlog)
	if err != nil {
		log.Printf("Could not get grouped snapshots: %v", err)
		resp := pb.ReadAllSnapshotsResponse{
			DidSucceed:      false,
			ErrMsg:          err.Error(),
			PartialSnapshot: nil,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}

	if len(groupedObjects) == 0 {
		err = fmt.Errorf("no objects found in cloud")
		log.Println(err)
		resp := pb.ReadAllSnapshotsResponse{
			DidSucceed:      true,
			ErrMsg:          "",
			PartialSnapshot: nil,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}

	// Set up a blank partial response object
	ret := pb.ReadAllSnapshotsResponse{
		DidSucceed: true,
		ErrMsg:     "",
		PartialSnapshot: &pb.PartialSnapshot{
			BackupName:        "",
			SnapshotName:      "",
			SnapshotTimestamp: -1,
			SnapshotRawName:   "",
			RawRelPaths:       make([]string, 0),
		},
	}
	relPathsSinceLastSend := 0

	groupNameKeys := make([]string, 0, len(groupedObjects))
	for groupName := range groupedObjects {
		groupNameKeys = append(groupNameKeys, groupName)
	}
	sort.Strings(groupNameKeys)

	for _, groupName := range groupNameKeys {
		vlog.Printf("Processing objects>> backup '%s':\n", groupName)

		// Update the backup name for next partial return
		ret.PartialSnapshot.BackupName = groupName

		snapshotKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots))
		for snapshotName := range groupedObjects[groupName].Snapshots {
			snapshotKeys = append(snapshotKeys, snapshotName)
		}
		sort.Strings(snapshotKeys)

		for _, snapshotName := range snapshotKeys {
			vlog.Printf("Processing objects>>  %s\n", snapshotName)

			// Update the snapshot name for next partial send
			ret.PartialSnapshot.SnapshotName = snapshotName
			ret.PartialSnapshot.SnapshotRawName = groupName + "/" + snapshotName
			ret.PartialSnapshot.SnapshotTimestamp = util.GetUnixTimeFromSnapshotName(snapshotName)

			relPathKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots[snapshotName].RelPaths))
			mFilelist, err := objstorefs.ReconstructSnapshotFileList(ctxBkg, objst, bucket, key, groupName, snapshotName, "", nil, groupedObjects, vlog)
			if err != nil {
				log.Println("error: ReadAllSnapshots: objstorefs.ReconstructSnapshotFileList: ", err)
			}
			for relPath := range mFilelist {
				relPathKeys = append(relPathKeys, relPath)
			}
			sort.Strings(relPathKeys)

			for _, relPath := range relPathKeys {
				val := groupedObjects[groupName].Snapshots[snapshotName].RelPaths[relPath]
				deletedMsg := ""
				if val.IsDeleted {
					deletedMsg = " (deleted)"
				}
				vlog.Printf("Processing objects>>    %s%s\n", relPath, deletedMsg)

				if !val.IsDeleted {
					ret.PartialSnapshot.RawRelPaths = append(ret.PartialSnapshot.RawRelPaths, relPath)
					relPathsSinceLastSend += 1

					// time for partial send?
					if relPathsSinceLastSend >= SendPartialResponseEveryNRelPaths {
						if err := srv.Send(&ret); err != nil {
							log.Println("error: server.Send failed: ", err)
						}
						ret.PartialSnapshot.RawRelPaths = make([]string, 0)
						relPathsSinceLastSend = 0
					}
				}
			}

			// send last partial for this snapshot
			if err := srv.Send(&ret); err != nil {
				log.Println("error: server.Send failed: ", err)
			}
			ret.PartialSnapshot.RawRelPaths = make([]string, 0)
			relPathsSinceLastSend = 0
		}
	}

	return nil
}

// Callback for rpc.DaemonCtlServer.ReadAllSnapshots requests
func (s *server) DeleteSnapshot(ctx context.Context, in *pb.DeleteSnapshotRequest) (*pb.DeleteSnapshotResponse, error) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Printf(">> GOT COMMAND: DeleteSnapshot (%s)", in.SnapshotRawName)
	defer log.Println(">> COMPLETED COMMAND: DeleteSnapshot")

	gGlobalsLock.Lock()
	isBusy := (gStatus.state != Idle)
	gGlobalsLock.Unlock()
	if isBusy {
		msg := fmt.Sprintf("Cannot delete snapshot '%s' right now because a backup or other operation is running", in.SnapshotRawName)
		log.Println(msg)
		return &pb.DeleteSnapshotResponse{
			DidSucceed: false,
			ErrMsg:     msg,
		}, nil
	}

	// Set the status for duration of this deletion
	gGlobalsLock.Lock()
	gStatus.state = CleaningUp
	gStatus.msg = "Deleting snapshot"
	gStatus.percentage = -1.0
	gGlobalsLock.Unlock()

	// When we exit this routine, we'll revert to Idle status
	resetStatus := func() {
		lastBackupTimeFormatted := getLastBackupTimeFormatted(&gGlobalsLock)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.percentage = -1.0
		gStatus.msg = "Last backup: " + lastBackupTimeFormatted
		gGlobalsLock.Unlock()
	}
	defer resetStatus()

	// Now do the actual snapshot deletion
	ctxBkg := context.Background()
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	key := gKey
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctxBkg, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	backupDirName, snapshotTimestamp, err := snapshots.SplitSnapshotName(in.SnapshotRawName)
	if err != nil {
		log.Printf("Cannot split '%s' into backupDirName/snapshotTimestamp", in.SnapshotRawName)
		return &pb.DeleteSnapshotResponse{
			DidSucceed: false,
			ErrMsg:     "snapshot name invalid",
		}, nil
	}

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctx, objst, key, bucket, vlog)
	if err != nil {
		log.Printf("Could not get grouped snapshots: %v", err)
		return &pb.DeleteSnapshotResponse{
			DidSucceed: false,
			ErrMsg:     "could not read objects from cloud",
		}, nil
	}

	// If we're about to delete the most recent snapshot, make sure next backup of this backup dir is a FULL backup
	isMostRecent := snapshots.IsMostRecentSnapshotForBackup(ctx, objst, bucket, groupedObjects, backupDirName, snapshotTimestamp)
	if isMostRecent {
		gGlobalsLock.Lock()
		err = gDb.ResetLastBackedUpTimeForEntireBackup(backupDirName)
		gGlobalsLock.Unlock()
		if err != nil {
			log.Printf("Could not reset last backup times on backup '%s': %v", backupDirName, err)
			return &pb.DeleteSnapshotResponse{
				DidSucceed: false,
				ErrMsg:     "could not reset last backup times. You should manually perform a full backup.",
			}, nil
		}
	}

	err = snapshots.DeleteSnapshot(ctx, key, groupedObjects, backupDirName, snapshotTimestamp, objst, bucket)
	if err != nil {
		return &pb.DeleteSnapshotResponse{
			DidSucceed: false,
			ErrMsg:     err.Error(),
		}, nil
	}

	return &pb.DeleteSnapshotResponse{
		DidSucceed: true,
		ErrMsg:     "",
	}, nil
}
