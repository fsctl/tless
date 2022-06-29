package daemon

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

const (
	SendPartialResponseEveryNRelPaths int = 5_000
)

// Callback for rpc.DaemonCtlServer.ReadAllSnapshotsMetadata requests
func (s *server) ReadAllSnapshotsMetadata(context.Context, *pb.ReadAllSnapshotsMetadataRequest) (*pb.ReadAllSnapshotsMetadataResponse, error) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Println(">> GOT COMMAND: ReadAllSnapshotsMetadata")
	defer log.Println(">> COMPLETED COMMAND: ReadAllSnapshotsMetadata")

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil && gKey != nil
	gGlobalsLock.Unlock()
	if !isGlobalConfigReady {
		log.Println("ReadAllSnapshotsMetadata: global config not yet initialized")
		return &pb.ReadAllSnapshotsMetadataResponse{
			DidSucceed:       false,
			ErrMsg:           "global config not yet initialized",
			SnapshotMetadata: nil,
		}, nil
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

	mSnapshots, err := snapshots.GetAllSnapshotInfos(ctxBkg, key, objst, bucket)
	if err != nil {
		msg := fmt.Sprintf("error: ReadAllSnapshotsMetadata: %v", err)
		log.Println(msg)
		return &pb.ReadAllSnapshotsMetadataResponse{
			DidSucceed:       false,
			ErrMsg:           msg,
			SnapshotMetadata: nil,
		}, nil
	}

	pbSnapshotMetadatas := make([]*pb.SnapshotMetadata, 0)
	for backupName, ssInfos := range mSnapshots {
		vlog.Printf("SNAPSHOT_METADATA> '%s'", backupName)
		for _, ssInfo := range ssInfos {
			vlog.Printf("SNAPSHOT_METADATA>     '%s' (%d, %s)", ssInfo.Name, ssInfo.TimestampUnix, ssInfo.RawSnapshotName)
			pbSnapshotMetadatas = append(pbSnapshotMetadatas, &pb.SnapshotMetadata{
				BackupName:        backupName,
				SnapshotName:      ssInfo.Name,
				SnapshotTimestamp: ssInfo.TimestampUnix,
				SnapshotRawName:   ssInfo.RawSnapshotName,
			})
		}
	}

	return &pb.ReadAllSnapshotsMetadataResponse{
		DidSucceed:       true,
		ErrMsg:           "",
		SnapshotMetadata: pbSnapshotMetadatas,
	}, nil
}

// Callback for rpc.DaemonCtlServer.ReadSnapshotPaths requests
func (s *server) ReadSnapshotPaths(in *pb.ReadSnapshotPathsRequest, srv pb.DaemonCtl_ReadSnapshotPathsServer) error {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Printf(">> GOT COMMAND: ReadSnapshotPaths (for '%s')", in.BackupName+"/"+in.SnapshotName)
	defer log.Println(">> COMPLETED COMMAND: ReadSnapshotPaths")

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil && gKey != nil
	gGlobalsLock.Unlock()
	if !isGlobalConfigReady {
		log.Println("ReadSnapshotPaths: global config not yet initialized")
		resp := pb.ReadSnapshotPathsResponse{
			DidSucceed: false,
			ErrMsg:     "global config not yet initialized",
			RelPaths:   nil,
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

	// Encrypt the backup name and snapshot name to form enc obj name for snapshot index obj
	encBackupName, err := cryptography.EncryptFilename(key, in.BackupName)
	if err != nil {
		msg := fmt.Sprintf("error: ReadSnapshotPaths: could not encrypt backup name (%s): %v\n", in.BackupName, err)
		log.Println(msg)
		resp := pb.ReadSnapshotPathsResponse{
			DidSucceed: false,
			ErrMsg:     msg,
			RelPaths:   nil,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}
	encSsName, err := cryptography.EncryptFilename(key, in.SnapshotName)
	if err != nil {
		msg := fmt.Sprintf("error: ReadSnapshotPaths: could not encrypt snapshot name (%s): %v\n", in.SnapshotName, err)
		log.Println(msg)
		resp := pb.ReadSnapshotPathsResponse{
			DidSucceed: false,
			ErrMsg:     msg,
			RelPaths:   nil,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}
	encObjName := encBackupName + "/@" + encSsName
	//vlog.Printf("SNAPSHOT_PATHS> encObjName = '%s'", encObjName)

	// Download the snapshot file and unmarshall it
	plaintextIndexFileBuf, err := snapshots.GetSnapshotIndexFile(ctxBkg, objst, bucket, key, encObjName)
	if err != nil {
		msg := fmt.Sprintf("error: ReadSnapshotPaths: could not retrieve snapshot index file (%s): %v\n", in.SnapshotName, err)
		log.Println(msg)
		resp := pb.ReadSnapshotPathsResponse{
			DidSucceed: false,
			ErrMsg:     msg,
			RelPaths:   nil,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}
	ssObj, err := snapshots.UnmarshalSnapshotObj(plaintextIndexFileBuf)
	if err != nil {
		msg := fmt.Sprintf("error: ReadSnapshotPaths: could not unmarshall to snapshot obj for '%s': %v\n", in.SnapshotName, err)
		log.Println(msg)
		resp := pb.ReadSnapshotPathsResponse{
			DidSucceed: false,
			ErrMsg:     msg,
			RelPaths:   nil,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}

	// Sort the rel paths
	vlog.Printf("SNAPSHOT_PATHS> Found %d rel paths", len(ssObj.RelPaths))
	sortedRelPaths := make([]string, 0, len(ssObj.RelPaths))
	for rp := range ssObj.RelPaths {
		sortedRelPaths = append(sortedRelPaths, rp)
	}
	sort.Strings(sortedRelPaths)

	// Send relPaths back one chunk at a time
	pbPartialResponse := &pb.ReadSnapshotPathsResponse{
		DidSucceed:  true,
		ErrMsg:      "",
		RelPaths:    make([]string, 0),
		PercentDone: float64(0),
	}
	cntSent := float64(0)
	for _, rp := range sortedRelPaths {
		pbPartialResponse.RelPaths = append(pbPartialResponse.RelPaths, rp)

		if len(pbPartialResponse.RelPaths) >= SendPartialResponseEveryNRelPaths {
			cntSent += float64(len(pbPartialResponse.RelPaths))
			percentDone := float64(100) * (cntSent / float64(len(sortedRelPaths)))
			pbPartialResponse.PercentDone = percentDone
			if err := srv.Send(pbPartialResponse); err != nil {
				log.Println("error: server.Send failed: ", err)
			}
			vlog.Printf("SNAPSHOT_PATHS> Sent %d rel paths (%d / %d); %02f%% done", len(pbPartialResponse.RelPaths), int64(cntSent), len(sortedRelPaths), percentDone)
			pbPartialResponse.RelPaths = make([]string, 0)
		}
	}

	// Send the final incomplete chhunk
	if len(pbPartialResponse.RelPaths) > 0 {
		cntSent += float64(len(pbPartialResponse.RelPaths))
		pbPartialResponse.PercentDone = float64(100)
		if err := srv.Send(pbPartialResponse); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		vlog.Printf("SNAPSHOT_PATHS> Sent %d rel paths (%d / %d); %02f%% done (FINAL)", len(pbPartialResponse.RelPaths), int64(cntSent), len(sortedRelPaths), float64(100))
	}
	vlog.Println("SNAPSHOT_PATHS> Done")
	return nil
}

// Callback for rpc.DaemonCtlServer.ReadAllSnapshots requests
func (s *server) DeleteSnapshots(in *pb.DeleteSnapshotsRequest, srv pb.DaemonCtl_DeleteSnapshotsServer) error {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Printf(">> GOT COMMAND: DeleteSnapshots (%v)", in.SnapshotRawNames)
	defer log.Println(">> COMPLETED COMMAND: DeleteSnapshots")

	gGlobalsLock.Lock()
	isBusy := (gStatus.state != Idle)
	gGlobalsLock.Unlock()
	if isBusy {
		msg := "Cannot delete snapshots right now because a backup or other operation is running"
		log.Println(msg)
		resp := pb.DeleteSnapshotsResponse{
			DidSucceed:  false,
			ErrMsg:      msg,
			PercentDone: 0.0,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}

	// Set the status for duration of this deletion
	gGlobalsLock.Lock()
	gStatus.state = CleaningUp
	gStatus.msg = "Deleting snapshot(s)"
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

	snapshotRawNames := in.SnapshotRawNames
	ssDelItems := make([]snapshots.SnapshotForDeletion, 0)
	for _, ssRawName := range snapshotRawNames {
		backupDirName, snapshotTimestamp, err := snapshots.SplitSnapshotName(ssRawName)
		if err != nil {
			log.Printf("Cannot split '%s' into backupDirName/snapshotTimestamp", ssRawName)
			resp := pb.DeleteSnapshotsResponse{
				DidSucceed:  false,
				ErrMsg:      "snapshot name invalid",
				PercentDone: 0.0,
			}
			if err := srv.Send(&resp); err != nil {
				log.Println("error: server.Send failed: ", err)
			}
			return nil
		}
		ssDelItems = append(ssDelItems, snapshots.SnapshotForDeletion{
			BackupDirName: backupDirName,
			SnapshotName:  snapshotTimestamp,
		})
	}

	// progress closures for first slow operation
	setInitialGGS1Progress := func(finished int64, total int64) {
		resp := pb.DeleteSnapshotsResponse{
			DidSucceed:  true,
			ErrMsg:      "",
			PercentDone: 0.0,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
	}
	updateGGS1Progress := func(finished int64, total int64) {
		// this is only the first 0-50%
		percentDone := float64(50.0) * (float64(finished) / float64(total))
		resp := pb.DeleteSnapshotsResponse{
			DidSucceed:  true,
			ErrMsg:      "",
			PercentDone: percentDone,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
	}

	groupedObjects, err := snapshots.GetGroupedSnapshots(ctxBkg, objst, key, bucket, vlog, setInitialGGS1Progress, updateGGS1Progress)
	if err != nil {
		log.Printf("Could not get grouped snapshots: %v", err)
		resp := pb.DeleteSnapshotsResponse{
			DidSucceed:  false,
			ErrMsg:      "could not read objects from cloud",
			PercentDone: 0.0,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}

	// If we're about to delete the most recent snapshot, make sure next backup of this backup dir is a FULL backup
	for _, ssDelItem := range ssDelItems {
		isMostRecent := snapshots.IsMostRecentSnapshotForBackup(ctxBkg, objst, bucket, groupedObjects, ssDelItem.BackupDirName, ssDelItem.SnapshotName)
		if isMostRecent {
			vlog.Println(">>> We are deleting the most recent snapshot; next backup will be a full backup")
			gGlobalsLock.Lock()
			err = gDb.ResetLastBackedUpTimeForEntireBackup(ssDelItem.BackupDirName)
			gGlobalsLock.Unlock()
			if err != nil {
				log.Printf("Could not reset last backup times on backup '%s': %v", ssDelItem.BackupDirName, err)
				resp := pb.DeleteSnapshotsResponse{
					DidSucceed:  false,
					ErrMsg:      "could not reset last backup times. You should manually perform a full backup.",
					PercentDone: 0.0,
				}
				if err := srv.Send(&resp); err != nil {
					log.Println("error: server.Send failed: ", err)
				}
				return nil
			}
		}
	}

	// progress closures for 2nd slow operation
	setInitialGGS2Progress := func(finished int64, total int64) {
		// starts at 50% since this is the second slow operation
		resp := pb.DeleteSnapshotsResponse{
			DidSucceed:  true,
			ErrMsg:      "",
			PercentDone: 50.0,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
	}
	updateGGS2Progress := func(finished int64, total int64) {
		// this is the 2nd part (50-100%)
		percentDone := float64(50.0) + ((float64(100) * (float64(finished) / float64(total))) / 2)
		resp := pb.DeleteSnapshotsResponse{
			DidSucceed:  true,
			ErrMsg:      "",
			PercentDone: percentDone,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
	}

	err = snapshots.DeleteSnapshots(ctxBkg, key, ssDelItems, objst, bucket, vlog, setInitialGGS2Progress, updateGGS2Progress)
	if err != nil {
		resp := pb.DeleteSnapshotsResponse{
			DidSucceed:  false,
			ErrMsg:      err.Error(),
			PercentDone: 0.0,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}

	return nil
}
