package daemon

import (
	"context"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	pb "github.com/fsctl/tless/rpc"
)

// Callback for rpc.DaemonCtlServer.ListBuckets requests
func (s *server) ListBuckets(ctx context.Context, in *pb.ListBucketsRequest) (*pb.ListBucketsResponse, error) {
	log.Println(">> GOT COMMAND: ListBuckets")
	defer log.Println(">> COMPLETED COMMAND: ListBuckets")

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil
	gGlobalsLock.Unlock()
	if !isGlobalConfigReady {
		log.Println("ListBuckets: global config not yet initialized")
		return &pb.ListBucketsResponse{
			Buckets: []string{},
			ErrMsg:  "global config not yet initialized",
		}, nil
	}

	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)
	buckets, err := objst.ListBuckets(ctx)
	if err != nil {
		log.Println("error: ListBuckets: ", err)
		return &pb.ListBucketsResponse{
			Buckets: []string{},
			ErrMsg:  err.Error(),
		}, err
	}

	return &pb.ListBucketsResponse{
		Buckets: buckets,
		ErrMsg:  "",
	}, nil
}

// Callback for rpc.DaemonCtlServer.ListBuckets requests
func (s *server) MakeBucket(ctx context.Context, in *pb.MakeBucketRequest) (*pb.MakeBucketResponse, error) {
	log.Println(">> GOT COMMAND: MakeBucket")
	defer log.Println(">> COMPLETED COMMAND: MakeBucket")

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil
	gGlobalsLock.Unlock()
	if !isGlobalConfigReady {
		log.Println("ListBuckets: global config not yet initialized")
		return &pb.MakeBucketResponse{
			DidSucceed: false,
			ErrMsg:     "global config not yet initialized",
		}, nil
	}

	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)
	err := objst.MakeBucket(ctx, in.GetBucketName(), in.GetRegion())
	if err != nil {
		log.Println("error: MakeBucket: ", err)
		return &pb.MakeBucketResponse{
			DidSucceed: false,
			ErrMsg:     err.Error(),
		}, err
	}

	return &pb.MakeBucketResponse{
		DidSucceed: true,
		ErrMsg:     "",
	}, nil
}
