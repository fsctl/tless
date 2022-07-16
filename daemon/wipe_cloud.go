package daemon

import (
	"context"
	"fmt"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

func (s *server) WipeCloud(in *pb.WipeCloudRequest, srv pb.DaemonCtl_WipeCloudServer) error {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	sendPartialFunc := func(didSucceed bool, percentDone float64, errMsg string) {
		resp := pb.WipeCloudResponse{
			DidSucceed:  didSucceed,
			PercentDone: percentDone,
			ErrMsg:      errMsg,
		}
		if err := srv.Send(&resp); err != nil {
			log.Println("error: server.Send failed: ", err)
		}
	}

	log.Println(">> GOT COMMAND: Wipe Cloud")

	gGlobalsLock.Lock()
	isIdle := gStatus.state == Idle
	gGlobalsLock.Unlock()
	if !isIdle {
		log.Println("WIPE-CLOUD> Not in Idle state, cannot wipe cloud right now")
		log.Println(">> COMPLETED COMMAND: Wipe Cloud")
		sendPartialFunc(false, float64(0), "not in Idle state")
		return nil
	}

	gGlobalsLock.Lock()
	gStatus.state = CleaningUp
	gStatus.msg = "Deleting all cloud data"
	gStatus.percentage = 0.0
	gGlobalsLock.Unlock()

	// Sets status back to Idle when routine is done
	done := func() {
		lastBackupTimeFormatted := getLastBackupTimeFormatted(&gDbLock)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.msg = "Last backup: " + lastBackupTimeFormatted
		gStatus.percentage = -1.0
		gGlobalsLock.Unlock()

		sendPartialFunc(true, float64(100), "")
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

	allObjects, err := objst.GetObjList(ctx, bucket, "", true, vlog)
	if err != nil {
		msg := fmt.Sprintf("error: WipeCloud: GetObjList failed: %v", err)
		log.Println(msg)
		sendPartialFunc(false, float64(0), msg)
		return nil
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

		sendPartialFunc(true, float64(percentDone), "")
	}

	return nil
}
