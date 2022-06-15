package cmd

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
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

	tless extras check-conn
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			checkConnMain()
		},
	}

	wipeCloudCmd = &cobra.Command{
		Use:   "wipe-cloud",
		Short: "Clears all objects in bucket",
		Long: `Run this command to delete all the contents of the bucket.

Example:

	tless extras wipe-cloud
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			wipeCloudMain()
		},
	}

	genTemplateCmd = &cobra.Command{
		Use:   "print-template",
		Short: "Prints a config file template to stdout",
		Long: `Run this command to print an example $HOME/.tless/config.toml file template. This
template can be copy-pasted into an actual config file, and includes comments to help you fill it
in with your settings.

Example:

	tless extras print-template
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			genTemplateMain()
		},
	}

	decObjNameCmd = &cobra.Command{
		Use:   "dec-objname",
		Short: "Decrypts an object name",
		Long: `This command decrypts object store object names, either full ones with multiple slashes
or individual components.  See below for some examples.

Example:

	tless extras dec-objname KVNWnYqs9WjINZhAU6UCWCEGoJcqNpKgbOWVEs7IBlCKzaFkbih4i9sYRyMyZurFohPDGUypMA==/4EyQgOMGXAyubu52fp5wpG_AVR5n3XLE1-fBtn_p9klvMiiC35S_i8N-3EW9HEBsikhRvGl4ZUoQI8c=/KOguujOV6osTNUcQUx6_XBCG50JhsjX8Vx8iAXzM/g7TiJMogCcRqVFLV9j0BGseTtLLid7NYIZ-7EO4=.003
	=> test-backup-src / 2020-10-16_12.23.41 / subdir1/subdir2/file1.txt

	tless extras dec-objname KVNWnYqs9WjINZhAU6UCWCEGoJcqNpKgbOWVEs7IBlCKzaFkbih4i9sYRyMyZurFohPDGUypMA==
	=> test-backup-src

	tless extras dec-objname 4EyQgOMGXAyubu52fp5wpG_AVR5n3XLE1-fBtn_p9klvMiiC35S_i8N-3EW9HEBsikhRvGl4ZUoQI8c=
	=> 2020-10-16_12.23.41

	tless extras dec-objname KOguujOV6osTNUcQUx6_XBCG50JhsjX8Vx8iAXzM/g7TiJMogCcRqVFLV9j0BGseTtLLid7NYIZ-7EO4=.003
	=> subdir1/subdir2/file1.txt

	Note that the last example contains a single slash in the encrypted object component name. This
	indicates that it is a relative path. Previous examples either contain all possible slashes or
	no slashes.
`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				fmt.Printf("Expecting one argument: the object name to decrypt. (--help for examples)")
			}
			decObjNameMain(args[0])
		},
	}
)

func init() {
	extrasCmd.AddCommand(checkConnCmd)
	extrasCmd.AddCommand(wipeCloudCmd)
	extrasCmd.AddCommand(genTemplateCmd)
	extrasCmd.AddCommand(decObjNameCmd)
	rootCmd.AddCommand(extrasCmd)
}

func checkConnMain() {
	objst := objstore.NewObjStore(context.Background(), cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)
	if ok, err := objst.IsReachableWithRetries(context.Background(), 10, cfgBucket, nil); !ok {
		fmt.Println("connectivity check failed: are your settings correct in config.toml? error: ", err)
	} else {
		fmt.Println("connectivity check successful")
	}
}

func wipeCloudMain() {
	vlog := util.NewVLog(nil, func() bool { return cfgVerbose })

	objst := objstore.NewObjStore(context.Background(), cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)
	ctx := context.Background()

	// initialize progress bar container
	progressBarContainer := mpb.New()

	allObjects, err := objst.GetObjList(ctx, cfgBucket, "", true, vlog)
	if err != nil {
		log.Printf("error: wipeCloudMain: GetObjList failed: %v", err)
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
			log.Printf("error: wipeCloudMain: objst.DeleteObj failed: %v", err)
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
	template := util.GenerateConfigTemplate(nil)
	fmt.Println(template)
}

func decObjNameMain(encObjectName string) {
	// validate encryptedObjectName format
	encObjectNameParts := strings.Split(encObjectName, "/")
	if len(encObjectNameParts) != 1 && len(encObjectNameParts) != 2 && len(encObjectNameParts) != 4 {
		fmt.Printf("malformed input: either 0, 1 or 3 slashes expected")
		return
	}

	// decrypt each format
	if len(encObjectNameParts) == 1 {
		if decrypted, err := cryptography.DecryptFilename(encKey, encObjectNameParts[0]); err != nil {
			fmt.Println("error: ", err)
		} else {
			fmt.Printf("=> %s\n", decrypted)
		}
	} else if len(encObjectNameParts) == 2 {
		// We assume one slash means its a relpath, so join the two parts and decrypt
		encRelPath := encObjectNameParts[0] + encObjectNameParts[1]
		if strings.Contains(encRelPath, ".") {
			encRelPath = encRelPath[:len(encRelPath)-4] // strip off .NNN
		}
		if decrypted, err := cryptography.DecryptFilename(encKey, encRelPath); err != nil {
			fmt.Println("error: ", err)
		} else {
			fmt.Printf("=> %s\n", decrypted)
		}
	} else if len(encObjectNameParts) == 4 {
		// We assume four slashes means its a full object name, so join the last two parts and decrypt
		encBackupDirName := encObjectNameParts[0]
		decBackupDirName, err := cryptography.DecryptFilename(encKey, encBackupDirName)
		if err != nil {
			fmt.Println("error: ", err)
		}

		encSnapshotName := encObjectNameParts[1]
		decSnapshotName, err := cryptography.DecryptFilename(encKey, encSnapshotName)
		if err != nil {
			fmt.Println("error: ", err)
		}

		encRelPath := encObjectNameParts[2] + encObjectNameParts[3]
		if strings.Contains(encRelPath, ".") {
			encRelPath = encRelPath[:len(encRelPath)-4] // strip off .NNN
		}
		if decRelPath, err := cryptography.DecryptFilename(encKey, encRelPath); err != nil {
			fmt.Println("error: ", err)
		} else {
			fmt.Printf("=> %s / %s / %s\n", decBackupDirName, decSnapshotName, decRelPath)
		}
	}
}
