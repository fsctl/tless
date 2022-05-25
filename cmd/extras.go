package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fsctl/trustlessbak/pkg/objstore"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
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

	wipeServerCmd = &cobra.Command{
		Use:   "wipe-server",
		Short: "Clears all objects in bucket",
		Long: `Run this command to delete all the contents of the bucket.

Example:

	trustlessbak extras wipe-server
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			wipeServerMain()
		},
	}

	genTemplateCmd = &cobra.Command{
		Use:   "print-template",
		Short: "Prints a config file template to stdout",
		Long: `Run this command to print an example $HOME/.trustlessbak/config.toml file template. This
template can be copy-pasted into an actual config file, and includes comments to help you fill it
in with your settings.

Example:

	trustlessbak extras print-template
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			genTemplateMain()
		},
	}
)

func init() {
	extrasCmd.AddCommand(checkConnCmd)
	extrasCmd.AddCommand(wipeServerCmd)
	extrasCmd.AddCommand(genTemplateCmd)
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

func wipeServerMain() {
	objst := objstore.NewObjStore(context.Background(), cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)
	ctx := context.Background()

	// initialize progress bar container
	progressBarContainer := mpb.New()

	allObjects, err := objst.GetObjList(ctx, cfgBucket, "")
	if err != nil {
		log.Printf("error: wipeServerMain: GetObjList failed: %v", err)
	}

	// create the progress bar
	var progressBarTotalItems int
	var progressBar *mpb.Bar = nil
	if !cfgVerbose {
		progressBarTotalItems = len(allObjects)

		progressBar = progressBarContainer.New(
			int64(progressBarTotalItems),
			mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Rbound("]"),
			mpb.PrependDecorators(
				decor.Name("Wipe", decor.WC{W: len("Wipe") + 1, C: decor.DidentRight}),
				// replace ETA decorator with "done" message on OnComplete event
				decor.OnComplete(
					decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 4}), "done",
				),
			),
			mpb.AppendDecorators(decor.Percentage()),
		)
	}

	for objName := range allObjects {
		err = objst.DeleteObj(ctx, cfgBucket, objName)
		if err != nil {
			log.Printf("error: wipeServerMain: objst.DeleteObj failed: %v", err)
		} else {
			if cfgVerbose {
				log.Printf("deleted %s\n", objName)
			} else {
				progressBar.Increment()
			}
		}
	}

	if !cfgVerbose {
		// Give progress bar 0.1 sec to draw itself for final time
		time.Sleep(1e8)
	}
}

func genTemplateMain() {
	template := generateConfigTemplate()
	fmt.Println(template)
}
