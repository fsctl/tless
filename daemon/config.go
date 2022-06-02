package daemon

import (
	"context"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
	"github.com/spf13/viper"
)

var (
	// Module level variables
	gCfg         *util.CfgSettings
	gUsername    string
	gUserHomeDir string
)

func initConfig() {
	viper.SetConfigType("toml")
	viper.SetConfigName("config")
	viper.AddConfigPath(filepath.Join(gUserHomeDir, ".tless"))

	if err := viper.ReadInConfig(); err == nil {
		log.Println("Using config file:", viper.ConfigFileUsed())
	} else {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file could not be found, make one and read it in
			makeTemplateConfigFile(gUsername, gUserHomeDir, nil)
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
	gCfg = &util.CfgSettings{
		Endpoint:        viper.GetString("objectstore.endpoint"),
		AccessKeyId:     viper.GetString("objectstore.access_key_id"),
		SecretAccessKey: viper.GetString("objectstore.access_secret"),
		Bucket:          viper.GetString("objectstore.bucket"),
		MasterPassword:  viper.GetString("backups.master_password"),
		Salt:            viper.GetString("backups.salt"),
		Dirs:            viper.GetStringSlice("backups.dirs"),
		ExcludePaths:    viper.GetStringSlice("backups.excludes"),
	}
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

	if gCfg == nil {
		log.Println("Config not available for reading yet, returning nothing")
		return &pb.ReadConfigResponse{
			IsValid: false,
			ErrMsg:  "Config not available yet",
		}, nil
	} else {
		log.Println("Returning all config file settings")
		return &pb.ReadConfigResponse{
			IsValid:        true,
			ErrMsg:         "",
			Endpoint:       gCfg.Endpoint,
			AccessKey:      gCfg.AccessKeyId,
			SecretKey:      gCfg.SecretAccessKey,
			BucketName:     gCfg.Bucket,
			MasterPassword: gCfg.MasterPassword,
			Salt:           gCfg.Salt,
			Dirs:           gCfg.Dirs,
			Excludes:       gCfg.ExcludePaths,
		}, nil
	}
}

// Callback for rpc.DaemonCtlServer.ReadDaemonConfig requests
func (s *server) WriteToDaemonConfig(ctx context.Context, in *pb.WriteConfigRequest) (*pb.WriteConfigResponse, error) {
	log.Println(">> GOT COMMAND: WriteToDaemonConfig")
	defer log.Println(">> COMPLETED COMMAND: WriteToDaemonConfig")

	if gCfg == nil {
		log.Println("Config not available yet, took no action")

		return &pb.WriteConfigResponse{
			DidSucceed: false,
			ErrMsg:     "Not ready yet",
		}, nil
	} else {
		log.Println("Overwriting old config file settings")

		configToWrite := &util.CfgSettings{
			Endpoint:        in.GetEndpoint(),
			AccessKeyId:     in.GetAccessKey(),
			SecretAccessKey: in.GetSecretKey(),
			Bucket:          in.GetBucketName(),
			MasterPassword:  in.GetMasterPassword(),
			Salt:            in.GetSalt(),
			Dirs:            in.GetDirs(),
			ExcludePaths:    in.GetExcludes(),
		}
		makeTemplateConfigFile(gUsername, gUserHomeDir, configToWrite)

		// read this new config back into daemon
		initConfig()

		return &pb.WriteConfigResponse{
			DidSucceed: true,
			ErrMsg:     "",
		}, nil
	}
}
