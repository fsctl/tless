package cmd

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fsctl/tless/pkg/backup"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

var (
	// Flags
	cfgPartialRestore string

	restoreCmd = &cobra.Command{
		Use:   "restore [name] [/restore/into/dir]",
		Short: "Restores a snapshot into a specified directory",
		Long: `Restores the named snapshot into the specified path. Usage:

tless restore [snapshot] [/restore/into/dir]

(Use 'tless cloudls' to get the names of your stored snapshots.)

Example:

	tless restore Documents/2020-01-15_04:56:00 /home/myname/Recovered-Documents

This command will restore the entire backup of the 'Documents' hierarchy at the time
4:56:00am on 2020-01-15. To restore just a subset of Documents, try the next two commands.

	tless restore Documents/2020-01-15_04:56:00 /home/myname/Recovered-Documents --partial Journal
	tless restore Documents/2020-01-15_04:56:00 /home/myname/Recovered-Documents --partial Journal/Feb.docx

These two commands show how to do a partial restore. The same backup set ('Documents') and
snapshot will be used, but in the first example only the files under Documents/Journal
will be restored.

In the second command, only a single file will be restored: 'Documents/Journal/Feb.docx'.

The available snapshot times are displayed in 'unbackupcloud cloudls'.
`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			restoreMain(args[0], args[1])
		},
	}
)

func init() {
	restoreCmd.Flags().StringVarP(&cfgPartialRestore, "partial", "r", "", "relative path for a partial restore")
	rootCmd.AddCommand(restoreCmd)
}

func restoreMain(backupAndSnapshotName string, pathToRestoreInto string) {
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)

	backupAndSnapshotNameParts := strings.Split(backupAndSnapshotName, "/")
	if len(backupAndSnapshotNameParts) != 2 {
		log.Fatalf("Malformed snapshot name: '%s'", backupAndSnapshotName)
	}
	backupName := backupAndSnapshotNameParts[0]
	snapshotName := backupAndSnapshotNameParts[1]
	// TODO: validate both of these further to make sure argument is well-formed

	// initialize progress bar container
	progressBarContainer := mpb.New()

	// get all the relpaths for this snapshot
	mRelPathsObjsMap, err := objstorefs.ReconstructSnapshotFileList(ctx, objst, cfgBucket, encKey, backupName, snapshotName, cfgPartialRestore, nil)
	if err != nil {
		log.Fatalln("error: reconstructSnapshotFileList failed: ", err)
	}

	// create the progress bar
	var progressBarTotalItems int
	var progressBar *mpb.Bar = nil
	if !cfgVerbose {
		progressBarTotalItems = len(mRelPathsObjsMap)

		progressBar = progressBarContainer.New(
			int64(progressBarTotalItems),
			mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Rbound("]"),
			mpb.PrependDecorators(
				decor.Name(backupName, decor.WC{W: len(backupName) + 1, C: decor.DidentRight}),
				// replace ETA decorator with "done" message on OnComplete event
				decor.OnComplete(
					decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 4}), "done",
				),
			),
			mpb.AppendDecorators(decor.Percentage()),
		)
	}

	// loop over all the relpaths and restore each
	dirChmodQueue := make([]backup.DirChmodQueueItem, 0) // all directory mode bits are set at end
	for relPath := range mRelPathsObjsMap {
		if len(mRelPathsObjsMap[relPath]) > 1 {
			relPathChunks := mRelPathsObjsMap[relPath]

			err = backup.RestoreDirEntryFromChunks(ctx, encKey, pathToRestoreInto, relPathChunks, backupName, snapshotName, relPath, objst, cfgBucket, cfgVerbose, -1, -1)
			if err != nil {
				log.Printf("error: could not restore a dir entry '%s'", relPath)
			}
		} else if len(mRelPathsObjsMap[relPath]) == 1 {
			objName := mRelPathsObjsMap[relPath][0]

			err = backup.RestoreDirEntry(ctx, encKey, pathToRestoreInto, objName, backupName, snapshotName, relPath, objst, cfgBucket, cfgVerbose, &dirChmodQueue, -1, -1)
			if err != nil {
				log.Printf("error: could not restore a dir entry '%s'", relPath)
			}
		} else {
			log.Fatalf("error: invalid number of chunks planned for %s", relPath)
		}

		// Update the progress bar
		if !cfgVerbose {
			progressBar.Increment()
		}
	}

	// Let the progress bar finish and reach 100%
	if !cfgVerbose {
		// Give progress bar 0.1 sec to draw itself for final time
		time.Sleep(1e8)
	}

	// Do all the queued up directory chmods
	for _, dirChmodQueueItem := range dirChmodQueue {
		err := os.Chmod(dirChmodQueueItem.AbsPath, dirChmodQueueItem.FinalMode)
		if err != nil {
			log.Printf("error: could not chmod dir '%s' to final %#o\n", dirChmodQueueItem.AbsPath, dirChmodQueueItem.FinalMode)
		}
	}

}
