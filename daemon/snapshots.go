package daemon

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

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
	isGlobalConfigReady := gCfg != nil && gEncKey != nil
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
	encKey := gEncKey
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctxBkg, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	mSnapshots, err := snapshots.GetAllSnapshotInfos(ctxBkg, encKey, objst, bucket)
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

	log.Printf(">> GOT COMMAND: ReadSnapshotPaths (for '%s'/'%s')", in.BackupName, in.SnapshotName)
	defer log.Println(">> COMPLETED COMMAND: ReadSnapshotPaths")

	// Make sure arguments are non-blank as expected
	if in.BackupName == "" || in.SnapshotName == "" {
		msg := fmt.Sprintf("error: ReadSnapshotPaths: received blank argument(s): in.BackupName='%s', in.SnapshotName='%s'", in.BackupName, in.SnapshotName)
		log.Println(msg)
		resp := pb.ReadSnapshotPathsResponse{
			DidSucceed: false,
			ErrMsg:     "backup name or snapshot name was blank",
			RelPaths:   nil,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
		return nil
	}

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil && gEncKey != nil
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
	encKey := gEncKey
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctxBkg, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	// Encrypt the backup name and snapshot name to form enc obj name for snapshot index obj
	encBackupName, err := cryptography.EncryptFilename(encKey, in.BackupName)
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
	encSsName, err := cryptography.EncryptFilename(encKey, in.SnapshotName)
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
	plaintextIndexFileBuf, err := snapshots.GetSnapshotIndexFile(ctxBkg, objst, bucket, encKey, encObjName)
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
		lastBackupTimeFormatted := getLastBackupTimeFormatted(&gDbLock)
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
	encKey := gEncKey
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctxBkg, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	snapshotRawNames := in.SnapshotRawNames
	ssDelItems := make([]snapshots.SnapshotForDeletion, 0)
	for _, ssRawName := range snapshotRawNames {
		backupDirName, snapshotTimestamp, err := util.SplitSnapshotName(ssRawName)
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

	groupedObjects, err := snapshots.GetGroupedSnapshots(ctxBkg, objst, encKey, bucket, vlog, setInitialGGS1Progress, updateGGS1Progress)
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
			gDbLock.Lock()
			err = gDb.ResetLastBackedUpTimeForEntireBackup(ssDelItem.BackupDirName)
			gDbLock.Unlock()
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

	err = snapshots.DeleteSnapshots(ctxBkg, encKey, ssDelItems, objst, bucket, vlog, setInitialGGS2Progress, updateGGS2Progress)
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

// Callback for rpc.DaemonCtlServer.GetSnapshotSpaceUsage requests
func (s *server) GetSnapshotSpaceUsage(in *pb.GetSnapshotSpaceUsageRequest, srv pb.DaemonCtl_GetSnapshotSpaceUsageServer) error {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Printf(">> GOT COMMAND: GetSnapshotSpaceUsage")
	defer log.Println(">> COMPLETED COMMAND: GetSnapshotSpaceUsage")

	encKey := make([]byte, 32)
	ctx := context.Background()
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	copy(encKey, gEncKey)
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	doneWithError := func(msg string) {
		resp := pb.GetSnapshotSpaceUsageResponse{
			DidSucceed:    false,
			ErrMsg:        msg,
			SnapshotUsage: make([]*pb.SnapshotUsage, 0),
			PercentDone:   0.0,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
	}

	sendPartial := func(done int, total int, pbSsUsage *pb.SnapshotUsage) {
		percentDone := float64(100) * float64(done) / float64(total)
		resp := pb.GetSnapshotSpaceUsageResponse{
			DidSucceed:    true,
			ErrMsg:        "",
			SnapshotUsage: []*pb.SnapshotUsage{pbSsUsage},
			PercentDone:   percentDone,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
	}

	// get all backup names from cloud (top level paths)
	topLevelObjs, err := objst.GetObjListTopLevel(ctx, bucket, []string{"metadata", "chunks"})
	if err != nil {
		msg := fmt.Sprintln("error: GetSnapshotSpaceUsage: objst.GetObjListTopLevel failed: ", err)
		log.Println(msg)
		doneWithError(msg)
		return nil
	}

	// count the snapshot index files
	snapshotIndexCnt := 0
	for _, encBackupName := range topLevelObjs {
		m, err := objst.GetObjList(ctx, bucket, encBackupName+"/@", false, vlog)
		if err != nil {
			msg := fmt.Sprintf("error: GetSnapshotSpaceUsage: could not get list of snapshot index files for '%s': %v\n", encBackupName, err)
			log.Println(msg)
			doneWithError(msg)
			return nil
		}
		snapshotIndexCnt += len(m)
	}

	// Get map of all chunks and their sizes
	mCloudChunksBayKeys, err := objst.GetObjList(ctx, bucket, "chunks/", false, vlog)
	if err != nil {
		msg := fmt.Sprintf("error: GetSnapshotSpaceUsage: could not iterate over chunks in cloud: %v", err)
		log.Println(msg)
		doneWithError(msg)
		return nil
	}
	mCloudChunks := make(map[string]int64, len(mCloudChunksBayKeys))
	for cc := range mCloudChunksBayKeys {
		mCloudChunks[strings.TrimPrefix(cc, "chunks/")] = mCloudChunksBayKeys[cc]
	}

	// Get all the snapshots one by one
	snapshotsDoneCnt := 0
	for _, encBackupName := range topLevelObjs {
		backupName, err := cryptography.DecryptFilename(encKey, encBackupName)
		if err != nil {
			msg := fmt.Sprintf("error: GetSnapshotSpaceUsage: could not decrypt backup dir name (%s): %v\n", encBackupName, err)
			log.Println(msg)
			doneWithError(msg)
			return nil
		}

		mSnapshotIndexObjs, err := objst.GetObjList(ctx, bucket, encBackupName+"/@", false, vlog)
		if err != nil {
			msg := fmt.Sprintf("error: GetSnapshotSpaceUsage: could not get list of snapshot index files for '%s': %v\n", encBackupName, err)
			log.Println(msg)
			doneWithError(msg)
			return nil
		}

		for encObjName, encObjBcount := range mSnapshotIndexObjs {
			encSsName := strings.TrimPrefix(encObjName, encBackupName+"/@")

			ssName, err := cryptography.DecryptFilename(encKey, encSsName)
			if err != nil {
				vlog.Printf("error: GetSnapshotSpaceUsage: could not decrypt snapshot name (%s) - skipping: %v\n", encSsName, err)
				snapshotsDoneCnt += 1
				continue
			}

			plaintextIndexFileBuf, err := snapshots.GetSnapshotIndexFile(ctx, objst, bucket, encKey, encObjName)
			if err != nil {
				msg := fmt.Sprintf("error: GetSnapshotSpaceUsage: could not retrieve snapshot index file (%s) - skipping: %v\n", ssName, err)
				log.Println(msg)
				doneWithError(msg)
				return nil
			}

			ssObj, err := snapshots.UnmarshalSnapshotObj(plaintextIndexFileBuf)
			if err != nil {
				msg := fmt.Sprintf("error: GetSnapshotSpaceUsage: could not get snapshot obj for '%s': %v\n", ssName, err)
				log.Println(msg)
				doneWithError(msg)
				return nil
			}

			//vlog.Printf("Backup %s/%s", backupName, ssName)
			chunkNames := getAllReferencedChunkNames(ssObj)
			retPbChunks := make([]*pb.Chunk, 0)
			for _, chunkName := range chunkNames {
				chunkByteCount := mCloudChunks[chunkName]
				retPbChunks = append(retPbChunks, &pb.Chunk{
					Name:      chunkName,
					ByteCount: chunkByteCount,
				})
				//vlog.Printf("  %s chunk %s", util.FormatBytesAsString(chunkByteCount), chunkName)
			}
			ssUsageRet := pb.SnapshotUsage{
				BackupName:         backupName,
				SnapshotName:       ssName,
				SnapshotRawName:    backupName + "/" + ssName,
				IndexFileByteCount: encObjBcount,
				Chunks:             retPbChunks,
			}
			snapshotsDoneCnt += 1
			sendPartial(snapshotsDoneCnt, snapshotIndexCnt, &ssUsageRet)
			vlog.Printf("Completed %d of %d snapshots", snapshotsDoneCnt, snapshotIndexCnt)
		}
	}

	return nil
}

func getAllReferencedChunkNames(ssObj *snapshots.Snapshot) []string {
	chunkNamesMap := make(map[string]int)
	for k := range ssObj.RelPaths {
		crp := ssObj.RelPaths[k]
		for _, chunkExt := range crp.ChunkExtents {
			chunkNamesMap[chunkExt.ChunkName] = 0
		}
	}
	chunkNames := make([]string, 0, len(chunkNamesMap))
	for k := range chunkNamesMap {
		chunkNames = append(chunkNames, k)
	}
	return chunkNames
}
