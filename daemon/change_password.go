package daemon

import (
	"context"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

// Callback for rpc.DaemonCtlServer.CheckBucketPassword requests
func (s *server) ChangePassword(ctx context.Context, in *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Println(">> GOT COMMAND: ChangePassword")
	defer log.Println(">> COMPLETED COMMAND: ChangePassword")

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isGlobalConfigReady := gCfg != nil
	gGlobalsLock.Unlock()
	if !isGlobalConfigReady {
		log.Println("error: ChangePassword: global config not yet initialized")
		return &pb.ChangePasswordResponse{
			DidSucceed: false,
			ErrMsg:     "global config not yet initialized",
		}, nil
	}

	// Decrypt and reencrypt key file in bucket
	oldPassword := in.OldPassword
	newPassword := in.NewPassword
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	bucket := gCfg.Bucket
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	// Check if a metadata file with salt and encrypted keys exists and retrieve it
	_, _, encKey, hmacKey, err := objst.GetOrCreateBucketMetadata(ctx, bucket, oldPassword, vlog)
	if err != nil {
		log.Println("error: ChangePassword: GetOrCreateBucketMetadata failed: ", err)
		return &pb.ChangePasswordResponse{
			DidSucceed: false,
			ErrMsg:     err.Error(),
		}, nil
	}

	// Verify the encKey by trying to decrypt some filenames in bucket
	if err = objst.VerifyKeys(ctx, bucket, oldPassword, encKey, hmacKey, vlog); err != nil {
		log.Println("error: ChangePassword: VerifyKeyAndSalt failed: ", err)
		return &pb.ChangePasswordResponse{
			DidSucceed: false,
			ErrMsg:     "Old password is incorrect for bucket",
		}, nil
	}

	// Change the password
	if err = objst.ChangePassword(ctx, bucket, encKey, hmacKey, newPassword, vlog); err != nil {
		log.Println("error: ChangePassword: objst.ChangePassword failed: ", err)
		return &pb.ChangePasswordResponse{
			DidSucceed: false,
			ErrMsg:     err.Error(),
		}, nil
	}

	// Re-fetch updated metadata file and verify it
	salt, _, encKey, hmacKey, err := objst.GetOrCreateBucketMetadata(ctx, bucket, newPassword, vlog)
	if err != nil {
		log.Println("error: ChangePassword: GetOrCreateBucketMetadata 2 failed: ", err)
		return &pb.ChangePasswordResponse{
			DidSucceed: false,
			ErrMsg:     err.Error(),
		}, nil
	}
	if err = objst.VerifyKeys(ctx, bucket, newPassword, encKey, hmacKey, vlog); err != nil {
		log.Println("error: ChangePassword: VerifyKeyAndSalt 2 failed: ", err)
		return &pb.ChangePasswordResponse{
			DidSucceed: false,
			ErrMsg:     "New password was not set correctly",
		}, nil
	}

	// Update globals
	gGlobalsLock.Lock()
	gCfg.Salt = salt
	gEncKey = encKey
	gHmacKey = hmacKey
	gGlobalsLock.Unlock()

	// Update config file if requested
	if in.UpdateConfigFile {
		vlog.Println("Overwriting old config file settings")

		configToWrite := &util.CfgSettings{
			Endpoint:             gCfg.Endpoint,
			AccessKeyId:          gCfg.AccessKeyId,
			SecretAccessKey:      gCfg.SecretAccessKey,
			Bucket:               gCfg.Bucket,
			TrustSelfSignedCerts: gCfg.TrustSelfSignedCerts,
			MasterPassword:       newPassword,
			Dirs:                 gCfg.Dirs,
			ExcludePaths:         gCfg.ExcludePaths,
			VerboseDaemon:        gCfg.VerboseDaemon,
		}

		gGlobalsLock.Lock()
		username := gUsername
		userHomeDir := gUserHomeDir
		gGlobalsLock.Unlock()

		makeTemplateConfigFile(username, userHomeDir, configToWrite)

		// read this new config back into daemon
		initConfig(&gGlobalsLock)
	}

	return &pb.ChangePasswordResponse{
		DidSucceed: true,
		ErrMsg:     "",
	}, nil
}
