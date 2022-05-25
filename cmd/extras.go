package cmd

import (
	"context"
	"fmt"

	"github.com/fsctl/trustlessbak/pkg/objstore"
	"github.com/spf13/cobra"
)

var (
	extrasCmd = &cobra.Command{
		Use:   "extras",
		Short: "Additional commands",
		Long:  `Additional, less commonly used commands.`,
		Args:  cobra.NoArgs,
	}

	checkConnCmd = &cobra.Command{
		Use:   "check-conn",
		Short: "Checks connectivity to object store server",
		Long: `Run this command to verify that you can connect to your cloud provider's object
store using the credentials in the config file.

Example:

	trustlessbak extras check-conn
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			checkConnMain()
		},
	}
)

func init() {
	extrasCmd.AddCommand(checkConnCmd)
	rootCmd.AddCommand(extrasCmd)
}

func checkConnMain() {
	objst := objstore.NewObjStore(context.Background(), cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)
	if !objst.IsReachableWithRetries(context.Background(), 10, cfgBucket) {
		fmt.Println("connectivity check failed: are your settings correct in config.toml?")
	} else {
		fmt.Println("connectivity check successful")
	}
}
