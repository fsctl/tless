package daemon

import (
	"context"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	pb "github.com/fsctl/tless/rpc"
)

// Callback for rpc.DaemonCtlServer.CheckConn requests
func (s *server) CheckConn(ctx context.Context, in *pb.CheckConnRequest) (*pb.CheckConnResponse, error) {
	log.Println(">> GOT COMMAND: CheckConn")
	defer log.Println(">> COMPLETED COMMAND: CheckConn")

	isBusy := false
	Status.lock.Lock()
	if Status.state == Idle {
		Status.state = CheckingConn
		Status.msg = ""
	} else {
		isBusy = true
	}
	Status.lock.Unlock()
	if isBusy {
		log.Println("Busy: can't do CheckConn right now")
		return &pb.CheckConnResponse{
			Result:   pb.CheckConnResponse_ERROR,
			ErrorMsg: "Busy with other work",
		}, nil
	}

	log.Println("with these parameters:")
	log.Printf("    Endpoint:   '%s'\n", in.GetEndpoint())
	log.Printf("    Access Key: '%s'\n", in.GetAccessKey())
	log.Printf("    Secret Key: '%s'\n", in.GetSecretKey())
	log.Printf("    Bucket:     '%s'\n", in.GetBucketName())
	objst := objstore.NewObjStore(ctx, in.GetEndpoint(), in.GetAccessKey(), in.GetSecretKey())
	isSuccessful := objst.IsReachableWithRetries(context.Background(), 5, in.GetBucketName())

	Status.lock.Lock()
	Status.state = Idle
	Status.msg = ""
	Status.lock.Unlock()

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
			ErrorMsg: "Unable to connect",
		}, nil
	}
}
