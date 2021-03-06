package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
	"github.com/spf13/viper"
)

var (
	// Protected by gGlobalsLock
	gCfg         *util.CfgSettings
	gUsername    string
	gUserHomeDir string
	gEncKey      []byte
	gHmacKey     []byte
)

func initConfig(globalsLock *sync.Mutex) error {
	// hmacKey is presently unused but may be in the future
	_ = gHmacKey

	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	globalsLock.Lock()
	username := gUsername
	userHomeDir := gUserHomeDir
	globalsLock.Unlock()

	viper.SetConfigType("toml")
	viper.SetConfigName("config")
	viper.AddConfigPath(filepath.Join(userHomeDir, ".tless"))

	if err := viper.ReadInConfig(); err == nil {
		vlog.Println("Using config file:", viper.ConfigFileUsed())
	} else {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file could not be found, make one and read it in
			makeTemplateConfigFile(username, userHomeDir, nil)
			if err := viper.ReadInConfig(); err == nil {
				log.Println("Using config file:", viper.ConfigFileUsed())
			} else {
				log.Fatalf("Error reading config file: %v\n", err)
			}
		} else {
			// Config file was found but there was an error parsing it
			log.Fatalf("Error reading config file: %v\n", err)
		}
	}

	globalsLock.Lock()
	gCfg = &util.CfgSettings{
		Endpoint:             viper.GetString("objectstore.endpoint"),
		AccessKeyId:          viper.GetString("objectstore.access_key_id"),
		SecretAccessKey:      viper.GetString("objectstore.access_secret"),
		Bucket:               viper.GetString("objectstore.bucket"),
		TrustSelfSignedCerts: viper.GetBool("objectstore.trust_self_signed_certs"),
		MasterPassword:       viper.GetString("backups.master_password"),
		Dirs:                 viper.GetStringSlice("backups.dirs"),
		ExcludePaths:         viper.GetStringSlice("backups.excludes"),
		VerboseDaemon:        viper.GetBool("daemon.verbose"),
		CachesPath:           viper.GetString("system.caches_path"),
		MaxChunkCacheMb:      viper.GetInt64("system.max_chunk_cache_mb"),
		ResourceUtilization:  viper.GetString("system.system_resource_utilization"),
	}
	globalsLock.Unlock()

	// Check that cloud is reachable
	globalsLock.Lock()
	endpoint := gCfg.Endpoint
	bucket := gCfg.Bucket
	accessKeyId := gCfg.AccessKeyId
	secretAccessKey := gCfg.SecretAccessKey
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	globalsLock.Unlock()
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, endpoint, accessKeyId, secretAccessKey, trustSelfSignedCerts)
	if ok, err := objst.IsReachable(ctx, bucket, vlog); !ok {
		msg := fmt.Sprintln("error: cloud server not reachable: ", err)
		return fmt.Errorf(msg)
	}

	// Grab the master password
	globalsLock.Lock()
	masterPassword := gCfg.MasterPassword
	globalsLock.Unlock()

	// Download (or create) the metadata
	salt, bucketVersion, encKey, hmacKey, err := objst.GetOrCreateBucketMetadata(ctx, bucket, masterPassword, vlog)
	if err != nil {
		e := fmt.Errorf("error: could not read or initialize bucket metadata: %v", err)
		vlog.Println(e.Error())
		return e
	}
	if len(salt) == 0 {
		e := fmt.Errorf("error: invalid salt (value='%s')", salt)
		vlog.Println(e.Error())
		return e
	}
	if !util.IntSliceContains(objstore.SupportedBucketVersions, bucketVersion) {
		e := fmt.Errorf("error: bucket version %d is not supported by this version of the program", bucketVersion)
		vlog.Println(e.Error())
		return e
	}

	// Verify the keys
	if err = objst.VerifyKeys(ctx, bucket, masterPassword, encKey, hmacKey, vlog); err != nil {
		vlog.Println(err.Error())
		return err
	}

	// Store keys in global
	globalsLock.Lock()
	gCfg.Salt = salt
	gEncKey = encKey
	gHmacKey = hmacKey
	globalsLock.Unlock()

	return nil
}

func makeTemplateConfigFile(username string, userHomeDir string, configValues *util.CfgSettings) {
	// get the user's numeric uid and primary group's gid
	u, err := user.Lookup(username)
	if err != nil {
		log.Fatalf("error: could not lookup user '%s': %v\n", username, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		log.Fatalf("error: could not convert uid string '%s' to int: %v\n", u.Uid, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		log.Fatalf("error: could not convert gid string '%s' to int: %v\n", u.Gid, err)
	}

	// make the config file dir
	configFileDir, err := util.MkdirUserConfig(username, userHomeDir)
	if err != nil {
		log.Fatalf("error: making config dir: %v", err)
	}
	log.Printf("Created config directory at '%s'\n", configFileDir)

	// make the config file
	configFilePath := filepath.Join(configFileDir, standardConfigFileName)
	template := util.GenerateConfigTemplate(configValues)
	if err := os.WriteFile(configFilePath, []byte(template), 0600); err != nil {
		log.Fatalln("error: unable to write template file: ", err)
	}
	if err := os.Chmod(configFilePath, 0600); err != nil {
		log.Fatalf("error: chmod on created config file failed: %v\n", err)
	}
	if err := os.Chown(configFilePath, uid, gid); err != nil {
		log.Fatalf("error: could not chown config file to '%d/%d': %v\n", uid, gid, err)
	}
	log.Printf("Created config file at '%s'\n", configFilePath)
}

// Callback for rpc.DaemonCtlServer.ReadDaemonConfig requests
func (s *server) ReadDaemonConfig(ctx context.Context, in *pb.ReadConfigRequest) (*pb.ReadConfigResponse, error) {
	log.Println(">> GOT COMMAND: ReadDaemonConfig")
	defer log.Println(">> COMPLETED COMMAND: ReadDaemonConfig")

	gGlobalsLock.Lock()
	cfgIsNil := gCfg == nil
	gGlobalsLock.Unlock()

	if cfgIsNil {
		log.Println("Config not available for reading yet, returning nothing")
		return &pb.ReadConfigResponse{
			IsValid: false,
			ErrMsg:  "Config not available yet",
		}, nil
	} else {
		log.Println("Returning all config file settings")
		gGlobalsLock.Lock()
		resp := &pb.ReadConfigResponse{
			IsValid:              true,
			ErrMsg:               "",
			Endpoint:             gCfg.Endpoint,
			AccessKey:            gCfg.AccessKeyId,
			SecretKey:            gCfg.SecretAccessKey,
			BucketName:           gCfg.Bucket,
			TrustSelfSignedCerts: gCfg.TrustSelfSignedCerts,
			MasterPassword:       gCfg.MasterPassword,
			Salt:                 gCfg.Salt,
			Dirs:                 gCfg.Dirs,
			Excludes:             gCfg.ExcludePaths,
			Verbose:              gCfg.VerboseDaemon,
			CachesPath:           gCfg.CachesPath,
			MaxChunkCacheMb:      gCfg.MaxChunkCacheMb,
			ResourceUtilization:  gCfg.ResourceUtilization,
		}
		gGlobalsLock.Unlock()
		return resp, nil
	}
}

// Callback for rpc.DaemonCtlServer.ReadDaemonConfig requests
func (s *server) WriteToDaemonConfig(ctx context.Context, in *pb.WriteConfigRequest) (*pb.WriteConfigResponse, error) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Println(">> GOT COMMAND: WriteToDaemonConfig")
	defer log.Println(">> COMPLETED COMMAND: WriteToDaemonConfig")

	gGlobalsLock.Lock()
	cfgIsNil := gCfg == nil
	gGlobalsLock.Unlock()

	if cfgIsNil {
		vlog.Println("Config not available yet, took no action")

		return &pb.WriteConfigResponse{
			DidSucceed: false,
			ErrMsg:     "Not ready yet",
		}, nil
	}

	vlog.Println("Overwriting old config file settings")

	configToWrite := &util.CfgSettings{
		Endpoint:             in.GetEndpoint(),
		AccessKeyId:          in.GetAccessKey(),
		SecretAccessKey:      in.GetSecretKey(),
		Bucket:               in.GetBucketName(),
		TrustSelfSignedCerts: in.GetTrustSelfSignedCerts(),
		MasterPassword:       in.GetMasterPassword(),
		Dirs:                 in.GetDirs(),
		ExcludePaths:         in.GetExcludes(),
		VerboseDaemon:        in.GetVerbose(),
		CachesPath:           in.GetCachesPath(),
		MaxChunkCacheMb:      in.GetMaxChunkCacheMb(),
		ResourceUtilization:  in.GetResourceUtilization(),
	}

	gGlobalsLock.Lock()
	username := gUsername
	userHomeDir := gUserHomeDir
	gGlobalsLock.Unlock()

	makeTemplateConfigFile(username, userHomeDir, configToWrite)

	// read this new config back into daemon
	initConfig(&gGlobalsLock)

	return &pb.WriteConfigResponse{
		DidSucceed: true,
		ErrMsg:     "",
	}, nil
}
