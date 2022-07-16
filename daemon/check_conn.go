package daemon

import (
	"context"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

// Callback for rpc.DaemonCtlServer.CheckConn requests
func (s *server) CheckConn(ctx context.Context, in *pb.CheckConnRequest) (*pb.CheckConnResponse, error) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Println(">> GOT COMMAND: CheckConn")
	defer log.Println(">> COMPLETED COMMAND: CheckConn")

	isBusy := false
	gGlobalsLock.Lock()
	if gStatus.state == Idle {
		gStatus.state = CheckingConn
		gStatus.msg = ""
	} else {
		isBusy = true
	}
	gGlobalsLock.Unlock()
	if isBusy {
		log.Println("Busy: can't do CheckConn right now")
		return &pb.CheckConnResponse{
			Result:   pb.CheckConnResponse_ERROR,
			ErrorMsg: "busy with other work",
		}, nil
	}

	log.Println("with these parameters:")
	log.Printf("    Endpoint:             '%s'\n", in.GetEndpoint())
	log.Printf("    Access Key:           '%s'\n", in.GetAccessKey())
	log.Printf("    Secret Key:           '%s'\n", util.MakeLogSafe(in.GetSecretKey()))
	log.Printf("    Bucket:               '%s'\n", in.GetBucketName())
	log.Printf("    TrustSelfSignedCerts: '%v'\n", in.GetTrustSelfSignedCerts())
	objst := objstore.NewObjStore(ctx, in.GetEndpoint(), in.GetAccessKey(), in.GetSecretKey(), in.GetTrustSelfSignedCerts())
	var isSuccessful bool
	var errMsg string = ""
	var err error
	if in.GetBucketName() == "" {
		_, err = objst.ListBuckets(context.Background())
		isSuccessful = err == nil
		if err != nil {
			errMsg = err.Error()
		}
	} else {
		isSuccessful, err = objst.IsReachable(context.Background(), in.GetBucketName(), vlog)
		if err != nil {
			errMsg = err.Error()
		}
	}

	lastBackupTimeFormatted := getLastBackupTimeFormatted(&gDbLock)
	gGlobalsLock.Lock()
	gStatus.state = Idle
	gStatus.msg = "Last backup: " + lastBackupTimeFormatted
	gGlobalsLock.Unlock()

	if isSuccessful {
		log.Println("CheckConn succeeded")
		return &pb.CheckConnResponse{
			Result:   pb.CheckConnResponse_SUCCESS,
			ErrorMsg: "",
		}, nil
	} else {
		log.Println("CheckConn failed")
		return &pb.CheckConnResponse{
			Result:   pb.CheckConnResponse_ERROR,
			ErrorMsg: errMsg,
		}, nil
	}
}
