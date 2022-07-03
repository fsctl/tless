package daemon

import (
	"context"
	"fmt"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
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

// Callback for rpc.DaemonCtlServer.CheckBucketPassword requests
func (s *server) CheckBucketPassword(ctx context.Context, in *pb.CheckBucketPasswordRequest) (*pb.CheckBucketPasswordResponse, error) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Printf(">> GOT COMMAND: CheckBucketPassword (%s)", in.GetBucketName())
	defer log.Println(">> COMPLETED COMMAND: CheckBucketPassword")

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil
	gGlobalsLock.Unlock()
	if !isGlobalConfigReady {
		log.Println("error: CheckBucketPassword: global config not yet initialized")
		return &pb.CheckBucketPasswordResponse{
			Result: pb.CheckBucketPasswordResponse_ERR_OTHER,
			ErrMsg: "global config not yet initialized",
		}, nil
	}

	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	masterPassword := gCfg.MasterPassword
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	// Check if a metadata file with salt and encrypted keys exists and retrieve it
	_, bucketVersion, encKey, hmacKey, err := objst.GetOrCreateBucketMetadata(ctx, in.GetBucketName(), masterPassword, vlog)
	if err != nil {
		log.Println("error: CheckBucketPassword: GetOrCreateBucketMetadata failed: ", err)
		return &pb.CheckBucketPasswordResponse{
			Result: pb.CheckBucketPasswordResponse_ERR_OTHER,
			ErrMsg: err.Error(),
		}, nil
	}
	if !util.IntSliceContains(objstore.SupportedBucketVersions, bucketVersion) {
		msg := fmt.Sprintf("error: bucket version %d is not supported by this version of the program", bucketVersion)
		log.Println(msg)
		return &pb.CheckBucketPasswordResponse{
			Result: pb.CheckBucketPasswordResponse_ERR_INCOMPATIBLE_BUCKET_VERSION,
			ErrMsg: msg,
		}, nil
	}

	// Verify the encKey by trying to decrypt some filenames in bucket
	if err = objst.VerifyKeys(ctx, in.GetBucketName(), masterPassword, encKey, hmacKey, vlog); err != nil {
		log.Println("error: CheckBucketPassword: VerifyKeyAndSalt failed: ", err)
		return &pb.CheckBucketPasswordResponse{
			Result: pb.CheckBucketPasswordResponse_ERR_PASSWORD_WRONG,
			ErrMsg: err.Error(),
		}, nil
	}

	// Otherwise, we have the right Bucket/Password combination
	return &pb.CheckBucketPasswordResponse{
		Result: pb.CheckBucketPasswordResponse_SUCCESS,
		ErrMsg: "",
	}, nil
}
