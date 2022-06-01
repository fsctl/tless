package daemon

import (
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/fsctl/tless/pkg/util"
	"github.com/spf13/viper"
)

type CfgSettings struct {
	Endpoint        string
	AccessKeyId     string
	SecretAccessKey string
	Bucket          string
	MasterPassword  string
	Salt            string
	Dirs            []string
	ExcludePaths    []string
}

var (
	// Module level variables
	cfg *CfgSettings
)

func initConfig(username string, userHomeDir string) {
	viper.SetConfigType("toml")
	viper.SetConfigName("config")
	viper.AddConfigPath(filepath.Join(userHomeDir, ".tless"))

	if err := viper.ReadInConfig(); err == nil {
		log.Println("Using config file:", viper.ConfigFileUsed())
	} else {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file could not be found, make one and read it in
			makeTemplateConfigFile(username, userHomeDir)
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
	cfg = &CfgSettings{
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

func makeTemplateConfigFile(username string, userHomeDir string) {
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
	configFileDir := filepath.Join(userHomeDir, ".tless")
	if err := os.Mkdir(configFileDir, 0755); err != nil {
		log.Fatalf("error: mkdir failed: %v\n", err)
	}
	if err := os.Chmod(configFileDir, 0755); err != nil {
		log.Fatalf("error: chmod on created config dir failed: %v\n", err)
	}
	if err := os.Chown(configFileDir, uid, gid); err != nil {
		log.Fatalf("error: could not chown dir to '%d/%d': %v\n", uid, gid, err)
	}
	log.Printf("Created config directory at '%s'\n", configFileDir)

	// make the config file
	configFilePath := filepath.Join(configFileDir, standardConfigFileName)
	template := util.GenerateConfigTemplate()
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
