package daemon

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/fsctl/tless/pkg/snapshots"
	pb "github.com/fsctl/tless/rpc"
)

const (
	SendPartialResponseEveryNRelPaths int = 5_000
)

// Callback for rpc.DaemonCtlServer.ReadAllSnapshots requests
func (s *server) ReadAllSnapshots(in *pb.ReadAllSnapshotsRequest, srv pb.DaemonCtl_ReadAllSnapshotsServer) error {
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
	key := gKey
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctxBkg, endpoint, accessKey, secretKey)

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctxBkg, objst, key, bucket)
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
			DidSucceed:      false,
			ErrMsg:          "no objects found in cloud",
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
		fmt.Printf("Processing objects>> backup '%s':\n", groupName)

		// Update the backup name for next partial return
		ret.PartialSnapshot.BackupName = groupName

		snapshotKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots))
		for snapshotName := range groupedObjects[groupName].Snapshots {
			snapshotKeys = append(snapshotKeys, snapshotName)
		}
		sort.Strings(snapshotKeys)

		for _, snapshotName := range snapshotKeys {
			fmt.Printf("Processing objects>>  %s\n", snapshotName)

			// Update the snapshot name for next partial send
			ret.PartialSnapshot.SnapshotName = snapshotName
			ret.PartialSnapshot.SnapshotRawName = groupName + "/" + snapshotName
			ret.PartialSnapshot.SnapshotTimestamp = getUnixTimeFromSnapshotName(snapshotName)

			relPathKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots[snapshotName].RelPaths))
			for relPath := range groupedObjects[groupName].Snapshots[snapshotName].RelPaths {
				relPathKeys = append(relPathKeys, relPath)
			}
			sort.Strings(relPathKeys)

			for _, relPath := range relPathKeys {
				val := groupedObjects[groupName].Snapshots[snapshotName].RelPaths[relPath]
				deletedMsg := ""
				if val.IsDeleted {
					deletedMsg = " (deleted)"
				}
				fmt.Printf("Processing objects>>    %s%s\n", relPath, deletedMsg)

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

func getUnixTimeFromSnapshotName(snapshotName string) int64 {
	tm, err := time.Parse("2006-01-02_15:04:05", snapshotName)
	if err != nil {
		log.Fatalln("error: getUnixTimeFromSnapshotName: ", err)
	}
	return tm.Unix()
}

// Callback for rpc.DaemonCtlServer.ReadAllSnapshots requests
func (s *server) DeleteSnapshot(ctx context.Context, in *pb.DeleteSnapshotRequest) (*pb.DeleteSnapshotResponse, error) {
	log.Printf(">> GOT COMMAND: DeleteSnapshot (%s)", in.SnapshotRawName)
	defer log.Println(">> COMPLETED COMMAND: DeleteSnapshot")

	ctxBkg := context.Background()
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	key := gKey
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctxBkg, endpoint, accessKey, secretKey)

	backupDirName, snapshotTimestamp, err := snapshots.SplitSnapshotName(in.SnapshotRawName)
	if err != nil {
		log.Printf("Cannot split '%s' into backupDirName/snapshotTimestamp", in.SnapshotRawName)
		return &pb.DeleteSnapshotResponse{
			DidSucceed: false,
			ErrMsg:     "snapshot name invalid",
		}, nil
	}

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctx, objst, key, bucket)
	if err != nil {
		log.Printf("Could not get grouped snapshots: %v", err)
		return &pb.DeleteSnapshotResponse{
			DidSucceed: false,
			ErrMsg:     "could not read objects from cloud",
		}, nil
	}

	err = snapshots.DeleteSnapshot(ctx, key, groupedObjects, backupDirName, snapshotTimestamp, objst, bucket)
	if err != nil {
		return &pb.DeleteSnapshotResponse{
			DidSucceed: false,
			ErrMsg:     err.Error(),
		}, nil
	} else {
		return &pb.DeleteSnapshotResponse{
			DidSucceed: true,
			ErrMsg:     "",
		}, nil
	}
}
