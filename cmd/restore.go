package cmd

import (
	"context"
	"log"
	"os"
	"sort"
	"time"

	"github.com/fsctl/tless/pkg/backup"
	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
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

	tless restore Documents/2020-01-15_04.56.00 /home/myname/Recovered-Documents

This command will restore the entire backup of the 'Documents' hierarchy at the time
4:56:00am on 2020-01-15. To restore just a subset of Documents, try the next two commands.

	tless restore Documents/2020-01-15_04.56.00 /home/myname/Recovered-Documents --partial Journal
	tless restore Documents/2020-01-15_04.56.00 /home/myname/Recovered-Documents --partial Journal/Feb.docx

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
	vlog := util.NewVLog(nil, func() bool { return cfgVerbose })

	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)

	backupName, snapshotName, err := util.SplitSnapshotName(backupAndSnapshotName)
	if err != nil {
		log.Fatalf("Cannot split '%s' into backupDirName/snapshotTimestamp", backupAndSnapshotName)
	}

	// initialize progress bar container
	progressBarContainer := mpb.New()

	// Encrypt the backup name and snapshot name so we can form the index file name
	encBackupName, err := cryptography.EncryptFilename(encKey, backupName)
	if err != nil {
		log.Fatalf("error: cannot encrypt backup name '%s': %v", backupName, err)
	}
	encSnapshotName, err := cryptography.EncryptFilename(encKey, snapshotName)
	if err != nil {
		log.Fatalf("error: cannot encrypt snapshot name '%s': %v", backupName, err)
	}

	// Get the snapshot index
	encSsIndexObjName := encBackupName + "/@" + encSnapshotName
	ssIndexJson, err := snapshots.GetSnapshotIndexFile(ctx, objst, cfgBucket, encKey, encSsIndexObjName)
	if err != nil {
		log.Fatalf("error: cannot get snapshot index file for '%s': %v", backupAndSnapshotName, err)
	}
	snapshotObj, err := snapshots.UnmarshalSnapshotObj(ssIndexJson)
	if err != nil {
		log.Fatalf("error: cannot get snapshot object for '%s': %v", backupAndSnapshotName, err)
	}

	// Filter the rel paths we want to restore
	selectedRelPaths := []string{cfgPartialRestore}
	mRelPathsObjsMap := backup.FilterRelPaths(snapshotObj, nil, selectedRelPaths)

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

	// Initialize a chunk cache
	cc := backup.NewChunkCache(objst, encKey, vlog, -1, -1, cfgCachesPath, cfgMaxChunkCacheMb)

	// For locality of reference reasons, we'll get the best cache hit rate if we restore in lexiconigraphical
	// order of rel paths.
	relPathKeys := make([]string, 0, len(mRelPathsObjsMap))
	for relPath := range mRelPathsObjsMap {
		relPathKeys = append(relPathKeys, relPath)
	}
	sort.Strings(relPathKeys)

	// loop over all the relpaths and restore each
	dirChmodQueue := make([]backup.DirChmodQueueItem, 0) // all directory mode bits are set at end
	for _, relPath := range relPathKeys {
		err = backup.RestoreDirEntry(ctx, encKey, pathToRestoreInto, mRelPathsObjsMap[relPath], backupName, snapshotName, relPath, objst, cfgBucket, vlog, &dirChmodQueue, -1, -1, cc)
		if err != nil {
			log.Printf("error: could not restore a dir entry '%s'", relPath)
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

	// Print the cache hit rate to vlog for diagnostics
	cc.PrintCacheStatistics()
}
