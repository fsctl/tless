package daemon

import (
	"context"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

func (s *server) WipeCloud(ctx context.Context, in *pb.WipeCloudRequest) (*pb.WipeCloudResponse, error) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Println(">> GOT COMMAND: Wipe Cloud")

	gGlobalsLock.Lock()
	isIdle := gStatus.state == Idle
	gGlobalsLock.Unlock()
	if !isIdle {
		log.Println("WIPE-CLOUD> Not in Idle state, cannot wipe cloud right now")
		log.Println(">> COMPLETED COMMAND: Wipe Cloud")
		return &pb.WipeCloudResponse{
			IsStarting: false,
			ErrMsg:     "not in Idle state",
		}, nil
	}

	gGlobalsLock.Lock()
	gStatus.state = CleaningUp
	gStatus.msg = "Deleting all cloud data"
	gStatus.percentage = 0.0
	gGlobalsLock.Unlock()

	go WipeCloud(vlog, func() { log.Println(">> COMPLETED COMMAND: Wipe Cloud") })

	return &pb.WipeCloudResponse{
		IsStarting: true,
		ErrMsg:     "",
	}, nil
}

func WipeCloud(vlog *util.VLog, completion func()) {
	// Last step:  call the completion routine
	defer completion()

	// Sets status back to Idle when routine is done
	done := func() {
		lastBackupTimeFormatted := getLastBackupTimeFormatted(&gGlobalsLock)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.msg = "Last backup: " + lastBackupTimeFormatted
		gStatus.percentage = -1.0
		gGlobalsLock.Unlock()
	}
	defer done()

	ctx := context.Background()
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	allObjects, err := objst.GetObjList(ctx, bucket, "", nil)
	if err != nil {
		log.Printf("error: WipeCloud: GetObjList failed: %v", err)
		return
	}
	totalObjects := len(allObjects)
	doneObjects := 0

	for objName := range allObjects {
		err = objst.DeleteObj(ctx, bucket, objName)
		if err != nil {
			log.Printf("error: WipeCloud: objst.DeleteObj failed: %v", err)
		}
		doneObjects += 1

		percentDone := (float32(doneObjects) / float32(totalObjects)) * float32(100)
		gGlobalsLock.Lock()
		gStatus.percentage = percentDone
		gGlobalsLock.Unlock()

		vlog.Printf("WIPE-CLOUD> Deleted %s (%.2f%% done)\n", objName, percentDone)
	}
}
