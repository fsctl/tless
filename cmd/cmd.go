package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/fsctl/trustlessbak/pkg/cryptography"
	"github.com/fsctl/trustlessbak/pkg/objstore"
)

var (
	// Module level variables
	encKey []byte

	// Flags
	cfgEndpoint        string
	cfgAccessKeyId     string
	cfgSecretAccessKey string
	cfgBucket          string
	cfgMasterPassword  string
	cfgSalt            string

	// Root command
	rootCmd = &cobra.Command{
		Use:   "trustlessbak",
		Short: "Backup directories to the cloud without trusting it",
		Long: `                         === trustlessbak ===
trustlessbak is a tool for cloud backups for people who don't want to place
any trust in cloud providers. It encrypts files and filenames locally, with 
a password that never leaves the local machine.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			var err error

			// derive encryption key
			encKey, err = cryptography.DeriveKey(cfgSalt, cfgMasterPassword)
			if err != nil {
				log.Fatalf("Could not derive key: %v", err)
			}
		},
	}
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&cfgEndpoint, "endpoint", "e", "", "endpoint (ex: your-cloud.com:5000)")
	rootCmd.PersistentFlags().StringVarP(&cfgAccessKeyId, "access-key", "a", "", "access key for your cloud account")
	rootCmd.PersistentFlags().StringVarP(&cfgSecretAccessKey, "access-secret", "s", "", "secret key for your cloud account")
	rootCmd.PersistentFlags().StringVarP(&cfgBucket, "bucket", "b", "", "name of object store bucket to use")
	rootCmd.PersistentFlags().StringVarP(&cfgMasterPassword, "master-password", "p", "", "master password known only on your local machine")
	rootCmd.PersistentFlags().StringVarP(&cfgSalt, "salt", "l", "", "salt used for key derivation from master password")

	rootCmd.Flags().Bool("version", false, "print the version")

	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
}

func initConfig() {
	// look for config in:  $HOME/.trustlessbak/config.toml or './config.toml'
	viper.SetConfigType("toml")
	viper.SetConfigName("config")
	viper.AddConfigPath("$HOME/.trustlessbak")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	} else {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file could not be found
			makeTemplateConfigFile()
			os.Exit(1)
		} else {
			// Config file was found but there was an error parsing it
			log.Fatalf("Error reading config file: %v\n", err)
		}
	}

	// Read viper for any cfg variables not already overridden by CLI args
	configFallbackToTomlFile()
	if err := validateConfigVars(); err != nil {
		log.Fatalf("Error validating config: %v", err)
	}
}

func makeTemplateConfigFile() {
	fmt.Printf(
		`No config file was found in $HOME/.trustlessbak/config.toml or the current 
directory. 

A template config file will be written for you, but you must fill in its values
for the program to work.

`)
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("error: could not write config file template: %v\n", err)
	}
	appCfgDir := filepath.Join(userHomeDir, ".trustlessbak")
	os.Mkdir(appCfgDir, 0700)
	configFilePath := filepath.Join(appCfgDir, "config.toml")
	writeTemplateConfigToPath(configFilePath)
	fmt.Printf("Fill in config template at: %s\n", configFilePath)
}

func writeTemplateConfigToPath(configFilePath string) {
	template := `[objectstore]
# Customize this section with the real host:port of your S3-compatible object 
# store, your credentials for the object store, and a bucket you have ALREADY 
# created for storing backups.
endpoint = "127.0.0.1:9000"
access_key_id = "<your object store user id>"
access_secret = "<your object store password>"
bucket = "<name of an empty bucket you have created on object store>"

[backups]
# You can specify as many directories to back up as you want. 
# Example (Linux): /home/<yourname>/Documents
# Example (macOS): /Users/<yourname>/Documents
dirs = [ "<absolute path to directory>", "<optional additional directory>" ]

# The 10-word Diceware passphrase below has been randomly generated for you. 
# It has ~128 bits of entropy and thus is very resistant to brute force 
# cracking through at least the middle of this century.
#
# Note that your passphrase resides in this file but never leaves this machine.
master_password = "`

	template += cryptography.GenerateRandomPassphrase(10)

	template += `"

# This salt has been randomly generated for you; there's no need to change it.
# The salt does not need to be kept secret. In fact, a backup copy is stored 
# on the object store server as 'SALT-[salt_string]' in the bucket root.
salt = "`
	template += cryptography.GenerateRandomSalt() + "\"\n"
	if err := os.WriteFile(configFilePath, []byte(template), 0600); err != nil {
		fmt.Println("Unable to write template file: ", err)
	}
}

func configFallbackToTomlFile() {
	if cfgEndpoint == "" {
		cfgEndpoint = viper.GetString("objectstore.endpoint")
	}
	if cfgAccessKeyId == "" {
		cfgAccessKeyId = viper.GetString("objectstore.access_key_id")
	}
	if cfgSecretAccessKey == "" {
		cfgSecretAccessKey = viper.GetString("objectstore.access_secret")
	}
	if cfgBucket == "" {
		cfgBucket = viper.GetString("objectstore.bucket")
	}
	if cfgMasterPassword == "" {
		cfgMasterPassword = viper.GetString("backups.master_password")
	}
	if cfgSalt == "" {
		cfgSalt = viper.GetString("backups.salt")
	}
	if len(cfgDirs) == 0 {
		cfgDirs = viper.GetStringSlice("backups.dirs")
	}
}

// Checks that configuration variables have sensible values (e.g., non-blannk, sane length)
func validateConfigVars() error {
	// fmt.Println("hello from main")
	// fmt.Println("cfg variables:")
	// fmt.Printf("  endpoint:            '%v'\n", cfgEndpoint)
	// fmt.Printf("  cfgAccessKeyId:      '%v'\n", cfgAccessKeyId)
	// fmt.Printf("  cfgSecretAccessKey:  '%v'\n", cfgSecretAccessKey)
	// fmt.Printf("  cfgMasterPassword:   '%v'\n", cfgMasterPassword)
	// fmt.Printf("  cfgSalt:             '%v'\n", cfgSalt)
	// fmt.Printf("  cfgDirs:             '%v'\n", cfgDirs)
	if cfgEndpoint == "" {
		return fmt.Errorf("endpoint invalid (value='%s')", cfgEndpoint)
	}
	if cfgAccessKeyId == "" {
		return fmt.Errorf("access key id invalid (value='%s')", cfgAccessKeyId)
	}
	if cfgSecretAccessKey == "" {
		return fmt.Errorf("secret key invalid (value='%s')", cfgSecretAccessKey)
	}
	if cfgBucket == "" {
		return fmt.Errorf("bucket name invalid (value='%s')", cfgBucket)
	}
	if cfgMasterPassword == "" {
		return fmt.Errorf("master password invalid (value='%s')", cfgMasterPassword)
	}
	if len(cfgSalt) == 0 {
		// If salt is blank/missing, try to read it from the server.
		// If successful, use that salt but also save it to local config file.
		objst := objstore.NewObjStore(context.Background(), cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)
		if !objst.IsReachableWithRetries(context.Background(), 10, cfgBucket) {
			log.Fatalln("error: exiting because server not reachable")
		}
		salt, err := objst.TryReadSalt(context.Background(), cfgBucket)
		if err != nil {
			log.Println("warning: no salt in config and failed to read salt from server (will generate and save): ", err)
		}

		if salt == "" {
			// If salt is not on the server, generate one and write it to config file + backup
			// to server.
			salt = cryptography.GenerateRandomSalt()
			if err = objst.TryWriteSalt(context.Background(), cfgBucket, salt); err != nil {
				log.Println("warning: generated a salt because no salt in config, but could not write salt to server: ", err)
			}
		}

		viper.Set("backups.salt", salt)
		viper.WriteConfig()
		cfgSalt = salt
	}
	if len(cfgDirs) == 0 {
		return fmt.Errorf("backup dirs invalid (value='%v')", cfgDirs)
	}
	for _, dir := range cfgDirs {
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("backup dir '%s' does not exist)", dir)
		}
	}
	return nil
}
