package snapshots

import (
	"context"
	"fmt"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
)

func ComputeTotalCloudSpaceUsage(ctx context.Context, objst *objstore.ObjStore, bucket string, encKey []byte, vlog *util.VLog) (int64, error) {
	sizeAccum := int64(0)

	// add size of metadata file
	metadataFileMap, err := objst.GetObjList(ctx, bucket, "metadata", false, vlog)
	if err != nil {
		msg := fmt.Sprintln("error: ComputeTotalSpaceUsage: objst.GetObjListTopLevel failed: ", err)
		log.Println(msg)
		return 0, err
	}
	for _, byteCnt := range metadataFileMap {
		sizeAccum += byteCnt
	}

	// add sizes of all snapshot index files
	topLevelObjs, err := objst.GetObjListTopLevel(ctx, bucket, []string{"metadata", "chunks"})
	if err != nil {
		msg := fmt.Sprintln("error: ComputeTotalSpaceUsage: objst.GetObjListTopLevel failed: ", err)
		log.Println(msg)
		return 0, err
	}
	for _, encBackupName := range topLevelObjs {
		m, err := objst.GetObjList(ctx, bucket, encBackupName+"/@", false, vlog)
		if err != nil {
			msg := fmt.Sprintf("error: ComputeTotalSpaceUsage: could not get list of snapshot index files for '%s': %v\n", encBackupName, err)
			log.Println(msg)
			return 0, err
		}
		for _, byteCnt := range m {
			sizeAccum += byteCnt
		}
	}

	// add sizes of all chunks
	mCloudChunks, err := objst.GetObjList(ctx, bucket, "chunks/", false, vlog)
	if err != nil {
		msg := fmt.Sprintf("error: ComputeTotalSpaceUsage: could not iterate over chunks in cloud: %v", err)
		log.Println(msg)
		return 0, err
	}
	for _, byteCnt := range mCloudChunks {
		sizeAccum += byteCnt
	}

	return sizeAccum, nil
}
