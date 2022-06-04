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

// Callback for rpc.DaemonCtlServer.ReadAllSnapshots requests
func (s *server) ReadAllSnapshots(ctx context.Context, in *pb.ReadAllSnapshotsRequest) (*pb.ReadAllSnapshotsResponse, error) {
	log.Println(">> GOT COMMAND: ReadAllSnapshots")
	defer log.Println(">> COMPLETED COMMAND: ReadAllSnapshots")

	ctxBkg := context.Background()
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	key := gKey
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctxBkg, endpoint, accessKey, secretKey)

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctx, objst, key, bucket)
	if err != nil {
		log.Printf("Could not get grouped snapshots: %v", err)
		return &pb.ReadAllSnapshotsResponse{
			DidSucceed: false,
			ErrMsg:     err.Error(),
			Backups:    nil,
		}, nil
	}

	if len(groupedObjects) == 0 {
		err = fmt.Errorf("no objects found in cloud")
		log.Println(err)
		return &pb.ReadAllSnapshotsResponse{
			DidSucceed: true,
			ErrMsg:     "",
			Backups:    nil,
		}, nil
	}

	ret := pb.ReadAllSnapshotsResponse{
		Backups: make([]*pb.Snapshot, 0),
	}

	groupNameKeys := make([]string, 0, len(groupedObjects))
	for groupName := range groupedObjects {
		groupNameKeys = append(groupNameKeys, groupName)
	}
	sort.Strings(groupNameKeys)

	for _, groupName := range groupNameKeys {
		fmt.Printf("Processing objects>> backup '%s':\n", groupName)

		snapshotKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots))
		for snapshotName := range groupedObjects[groupName].Snapshots {
			snapshotKeys = append(snapshotKeys, snapshotName)
		}
		sort.Strings(snapshotKeys)

		for _, snapshotName := range snapshotKeys {
			fmt.Printf("Processing objects>>  %s\n", snapshotName)

			relPathKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots[snapshotName].RelPaths))
			for relPath := range groupedObjects[groupName].Snapshots[snapshotName].RelPaths {
				relPathKeys = append(relPathKeys, relPath)
			}
			sort.Strings(relPathKeys)

			rawRelPaths := make([]string, 0)
			for _, relPath := range relPathKeys {
				val := groupedObjects[groupName].Snapshots[snapshotName].RelPaths[relPath]
				deletedMsg := ""
				if val.IsDeleted {
					deletedMsg = " (deleted)"
				}
				fmt.Printf("Processing objects>>    %s%s\n", relPath, deletedMsg)

				if !val.IsDeleted {
					rawRelPaths = append(rawRelPaths, relPath)
				}
			}

			pbSnapshot := pb.Snapshot{
				BackupName:        groupName,
				SnapshotName:      snapshotName,
				SnapshotTimestamp: getUnixTimeFromSnapshotName(snapshotName),
				SnapshotRawName:   groupName + "/" + snapshotName,
				RawRelPaths:       rawRelPaths,
			}
			ret.Backups = append(ret.Backups, &pbSnapshot)
		}
	}

	ret.DidSucceed = true
	ret.ErrMsg = ""
	return &ret, nil
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
