package daemon

import (
	"context"
	"fmt"
	"log"

	"github.com/fsctl/tless/pkg/cryptography"
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

// Callback for rpc.DaemonCtlServer.GetBucketSalt requests
func (s *server) GetBucketSalt(ctx context.Context, in *pb.GetBucketSaltRequest) (*pb.GetBucketSaltResponse, error) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Printf(">> GOT COMMAND: GetBucketSalt (%s)", in.GetBucketName())
	defer log.Println(">> COMPLETED COMMAND: GetBucketSalt")

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil
	gGlobalsLock.Unlock()
	if !isGlobalConfigReady {
		log.Println("error: GetBucketSalt: global config not yet initialized")
		return &pb.GetBucketSaltResponse{
			Result: pb.GetBucketSaltResponse_ERR_OTHER,
			ErrMsg: "global config not yet initialized",
			Salt:   "",
		}, nil
	}

	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	// Try to fetch all objects starting with "SALT-"
	m, err := objst.GetObjList(ctx, in.GetBucketName(), "SALT-", vlog)
	if err != nil {
		log.Println("error: GetBucketSalt: GetObjList failed: ", err)
		return &pb.GetBucketSaltResponse{
			Result: pb.GetBucketSaltResponse_ERR_OTHER,
			ErrMsg: err.Error(),
			Salt:   "",
		}, nil
	}

	// Check if there's >1 SALT-xxxx file and warn user if so
	var saltObjName string
	if len(m) > 1 {
		for k := range m {
			msg := fmt.Sprintf("warning: found salt: '%s' in bucket\n", k)
			if vlog != nil {
				vlog.Println(msg)
			} else {
				log.Println(msg)
			}
		}
		log.Println("warning: there are multiple SALT-xxxx files in bucket '$s'; you need to manually delete the wrong one(s)", in.GetBucketName())
		return &pb.GetBucketSaltResponse{
			Result: pb.GetBucketSaltResponse_ERR_MULTIPLE_SALTS,
			ErrMsg: "",
			Salt:   "",
		}, nil
	} else if len(m) == 0 {
		// There is no SALT-xxxx file.
		return &pb.GetBucketSaltResponse{
			Result: pb.GetBucketSaltResponse_ERR_NO_SALT,
			ErrMsg: "",
			Salt:   "",
		}, nil
	} else {
		// There is only one salt; get its value
		for k := range m {
			saltObjName = k
			if len(saltObjName) < 6 {
				msg := fmt.Sprintf("error: salt too short (object name '%s')", saltObjName)
				log.Println(msg)
				return &pb.GetBucketSaltResponse{
					Result: pb.GetBucketSaltResponse_ERR_OTHER,
					ErrMsg: msg,
					Salt:   "",
				}, nil
			}
			salt := saltObjName[5:]
			msg := fmt.Sprintf("found salt '%s' in bucket '%s'", salt, in.GetBucketName())
			vlog.Println(msg)
			return &pb.GetBucketSaltResponse{
				Result: pb.GetBucketSaltResponse_SUCCESS,
				ErrMsg: "",
				Salt:   salt,
			}, nil
		}
	}

	return &pb.GetBucketSaltResponse{
		Result: pb.GetBucketSaltResponse_ERR_OTHER,
		ErrMsg: fmt.Sprintf("unknown error trying to read salt from bucket '%s'", in.GetBucketName()),
		Salt:   "",
	}, nil
}

// Callback for rpc.DaemonCtlServer.CheckBucketSaltPassword requests
func (s *server) CheckBucketSaltPassword(ctx context.Context, in *pb.CheckBucketSaltPasswordRequest) (*pb.CheckBucketSaltPasswordResponse, error) {
	log.Printf(">> GOT COMMAND: CheckBucketSaltPassword (%s)", in.GetBucketName())
	defer log.Println(">> COMPLETED COMMAND: CheckBucketSaltPassword")

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil
	gGlobalsLock.Unlock()
	if !isGlobalConfigReady {
		log.Println("error: CheckBucketSaltPassword: global config not yet initialized")
		return &pb.CheckBucketSaltPasswordResponse{
			Result: pb.CheckBucketSaltPasswordResponse_ERR_OTHER,
			ErrMsg: "global config not yet initialized",
		}, nil
	}

	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	// Try to fetch "SALT-" + input salt file
	saltObjName := "SALT-" + in.GetSalt()
	saltFileContents, err := objst.DownloadObjToBuffer(ctx, in.GetBucketName(), saltObjName)
	if err != nil {
		log.Printf("error: CheckBucketSaltPassword: could not download '%s': %v", saltObjName, err)
		return &pb.CheckBucketSaltPasswordResponse{
			Result: pb.CheckBucketSaltPasswordResponse_ERR_SALT_WRONG,
			ErrMsg: err.Error(),
		}, nil
	}

	// Derive the key
	key, err := cryptography.DeriveKey(in.GetSalt(), in.GetPassword())
	if err != nil {
		log.Println("error: CheckBucketSaltPassword: DeriveKey failed: ", err)
		return &pb.CheckBucketSaltPasswordResponse{
			Result: pb.CheckBucketSaltPasswordResponse_ERR_OTHER,
			ErrMsg: err.Error(),
		}, nil
	}

	// Try to decrypt the salt file just downloaded
	_, err = cryptography.DecryptBuffer(key, saltFileContents)
	if err != nil {
		log.Println("error: CheckBucketSaltPassword: DecryptBuffer failed: ", err)
		return &pb.CheckBucketSaltPasswordResponse{
			Result: pb.CheckBucketSaltPasswordResponse_ERR_PASSWORD_WRONG,
			ErrMsg: err.Error(),
		}, nil
	}

	// Otherwise, we have the right Bucket/Salt/Password combination
	return &pb.CheckBucketSaltPasswordResponse{
		Result: pb.CheckBucketSaltPasswordResponse_SUCCESS,
		ErrMsg: "",
	}, nil
}
