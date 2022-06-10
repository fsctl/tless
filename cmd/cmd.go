package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
)

var (
	// Module level variables
	encKey []byte

	// Flags
	cfgEndpoint             string
	cfgAccessKeyId          string
	cfgSecretAccessKey      string
	cfgBucket               string
	cfgTrustSelfSignedCerts bool
	cfgMasterPassword       string
	cfgSalt                 string
	cfgVerbose              bool
	cfgForce                bool

	// Root command
	rootCmd = &cobra.Command{
		Use:   "tless",
		Short: "Backup directories to the cloud without trusting it",
		Long: `                         === tless ===
tless is a tool for cloud backups for people who don't want to place
any trust in cloud providers. It encrypts files and filenames locally, with 
a password that never leaves the local machine.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			var err error

			// Deriving the encryption key is slow, so only do it if DeriveKey was not already
			// called as part of objstore.CheckCryptoConfigMatchesServer during package init
			if encKey == nil {
				encKey, err = cryptography.DeriveKey(cfgSalt, cfgMasterPassword)
				if err != nil {
					log.Fatalf("Could not derive key: %v", err)
				}
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
	rootCmd.PersistentFlags().BoolVarP(&cfgVerbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&cfgForce, "force", "f", false, "override check that salt and master password match\nwhat was previously used on server")
	rootCmd.PersistentFlags().BoolVarP(&cfgTrustSelfSignedCerts, "trust-certs", "C", false, "trust a self-signed cert from your cloud provider")

	rootCmd.Flags().Bool("version", false, "print the version")

	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
}

func initConfig() {
	// look for config in:  $HOME/.tless/config.toml or './config.toml'
	viper.SetConfigType("toml")
	viper.SetConfigName("config")
	viper.AddConfigPath("$HOME/.tless")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err == nil {
		if cfgVerbose {
			fmt.Println("Using config file:", viper.ConfigFileUsed())
		}
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
		`No config file was found in $HOME/.tless/config.toml or the current 
directory. 

A template config file will be written for you, but you must fill in its values
for the program to work.

`)
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("error: could not write config file template: %v\n", err)
	}
	appCfgDir := filepath.Join(userHomeDir, ".tless")
	os.Mkdir(appCfgDir, 0700)
	configFilePath := filepath.Join(appCfgDir, "config.toml")
	writeTemplateConfigToPath(configFilePath)
	fmt.Printf("Fill in config template at: %s\n", configFilePath)
}

func writeTemplateConfigToPath(configFilePath string) {
	template := util.GenerateConfigTemplate(nil)
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
	if !cfgTrustSelfSignedCerts {
		cfgTrustSelfSignedCerts = viper.GetBool("objectstore.trust_self_signed_certs")
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

	// Check crypto config in SALT-xxxx file
	if !cfgForce {
		ctx := context.Background()
		objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)
		if ok, err := objst.IsReachableWithRetries(context.Background(), 10, cfgBucket, nil); !ok {
			log.Fatalln("error: exiting because server not reachable: ", err)
		}

		var err error
		encKey, err = cryptography.DeriveKey(cfgSalt, cfgMasterPassword)
		if err != nil {
			log.Fatalf("Could not derive key: %v", err)
		}
		if !objst.CheckCryptoConfigMatchesServer(ctx, encKey, cfgBucket, cfgSalt, cfgVerbose) {
			fmt.Println("Halting because of salt file problem; use --force to override this check")
			log.Fatalf("halting due to salt file problem")
		}
	}
	if len(cfgSalt) == 0 {
		return fmt.Errorf("invalid salt (value='%s')", cfgSalt)
	}

	return nil
}
