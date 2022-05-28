package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsctl/trustlessbak/pkg/backup"
	"github.com/fsctl/trustlessbak/pkg/cryptography"
	"github.com/fsctl/trustlessbak/pkg/database"
	"github.com/fsctl/trustlessbak/pkg/fstraverse"
	"github.com/fsctl/trustlessbak/pkg/objstore"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

var (
	// Flags
	cfgDirs         []string
	cfgExcludePaths []string

	// Command
	backupCmd = &cobra.Command{
		Use:   "backup",
		Short: "Performs an incremental backup",
		Long: `Performs an incremental backup. The directories to recursively back up are listed
in the config file, or can be specified on the command line (-d). It is recommended that you create
a configuration file rather than use the command line flags.

Example:

	trustlessbak backup

This will read your config.toml configuration file for all information about the backup,
cloud provider credentials, and master password. It will then perform an incremental backup to
your cloud provider, uploading only those files that have changed since the last backup. Files
on the cloud provider will be overwritten by these newer local copies.
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			backupMain()
		},
	}
)

func init() {
	backupCmd.Flags().StringArrayVarP(&cfgDirs, "dir", "d", nil, "directories to backup (can use multiple times)")
	backupCmd.Flags().StringArrayVarP(&cfgExcludePaths, "exclude", "x", nil, "paths starting with this will be excluded from backup (can use multiple times)")
	rootCmd.AddCommand(backupCmd)
}

func backupMain() {
	ctx := context.Background()

	// check that cfgDirs is set, and allow cfgExcludePaths to be set from toml file if no arg
	if err := validateDirs(); err != nil {
		log.Fatalln("no valid dirs to back up: ", err)
	}
	if cfgExcludePaths == nil {
		cfgExcludePaths = viper.GetStringSlice("backups.excludes")
	}

	// open and prepare sqlite database
	db, err := database.NewDB("./trustlessbak-state.db")
	if err != nil {
		log.Fatalf("Error: cannot open database: %v", err)
	}
	defer db.Close()
	if err := db.CreateTablesIfNotExist(); err != nil {
		log.Fatalf("Error: cannot initialize database: %v", err)
	}

	// open connection to cloud server
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)
	if !objst.IsReachableWithRetries(ctx, 10, cfgBucket) {
		log.Fatalln("error: exiting because server not reachable")
	}

	// initialize progress bar container
	progressBarContainer := mpb.New()

	for _, backupDirPath := range cfgDirs {
		backupDirName := filepath.Base(backupDirPath)

		snapshotName := time.Now().UTC().Format("2006-01-02_15:04:05")

		// Traverse the filesystem looking for changed directory entries
		prevPaths, err := db.GetAllKnownPaths(backupDirName)
		if err != nil {
			log.Fatalf("Error: cannot get paths list: %v", err)
		}
		var backupIdsQueue fstraverse.BackupIdsQueue
		fstraverse.Traverse(backupDirPath, prevPaths, db, &backupIdsQueue, cfgExcludePaths)

		// create the progress bar
		var progressBarTotalItems int
		var progressBar *mpb.Bar = nil
		if !cfgVerbose {
			backupIdsQueue.Lock.Lock()
			progressBarTotalItems = len(backupIdsQueue.Ids)
			backupIdsQueue.Lock.Unlock()

			progressBar = progressBarContainer.New(
				int64(progressBarTotalItems),
				mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Rbound("]"),
				mpb.PrependDecorators(
					decor.Name(backupDirName, decor.WC{W: len(backupDirName) + 1, C: decor.DidentRight}),
					// replace ETA decorator with "done" message on OnComplete event
					decor.OnComplete(
						decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 4}), "done",
					),
				),
				mpb.AppendDecorators(decor.Percentage()),
			)
		}

		// Any remaining prevPaths represent deleted files, so upload keys to mark them deleted and remove from
		// dirents table
		if err = createDeletedPathsKeysAndPurgeFromDb(ctx, objst, cfgBucket, db, encKey, backupDirName, snapshotName, prevPaths, cfgVerbose); err != nil {
			log.Println("error: failed creatingn deleted paths keys")
		}

		// Work through the queue
		for {
			var id int = 0
			backupIdsQueue.Lock.Lock()
			if len(backupIdsQueue.Ids) >= 1 {
				id = backupIdsQueue.Ids[0]
				backupIdsQueue.Ids = backupIdsQueue.Ids[1:]
			} else {
				backupIdsQueue.Lock.Unlock()
				break
			}
			backupIdsQueue.Lock.Unlock()
			if id != 0 {
				if err := backup.Backup(ctx, encKey, db, backupDirPath, snapshotName, id, objst, cfgBucket, cfgVerbose); err != nil {
					log.Printf("error: Backup(): %v", err)
					continue
				}
				err := db.UpdateLastBackupTime(id)
				if err != nil {
					log.Printf("error: UpdateLastBackupTime(): %v", err)
				}
			}

			// Update the progress bar
			if !cfgVerbose {
				progressBar.Increment()
			}
		}
	}

	if cfgVerbose {
		fmt.Printf("done\n")
	} else {
		// Give progress bar 0.1 sec to draw itself for final time
		time.Sleep(1e8)
	}
}

func validateDirs() error {
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

func createDeletedPathsKeysAndPurgeFromDb(ctx context.Context, objst *objstore.ObjStore, bucket string, db *database.DB, key []byte, backupDirName string, snapshotName string, deletedPaths map[string]int, showNameOnSuccess bool) error {
	// get the encrypted representation of backupDirName and snapshotName
	encryptedSnapshotName, err := cryptography.EncryptFilename(key, snapshotName)
	if err != nil {
		log.Printf("error: createDeletedPathsKeys(): could not encrypt snapshot name (%s): %v\n", snapshotName, err)
		return err
	}
	encryptedBackupDirName, err := cryptography.EncryptFilename(key, backupDirName)
	if err != nil {
		log.Printf("error: createDeletedPathsKeys(): could not encrypt backup dir name (%s): %v\n", backupDirName, err)
		return err
	}

	// iterate over the deleted paths
	for deletedPath := range deletedPaths {
		// deletedPath is backupDirName/deletedRelPath.  Make it just deletedRelPath
		deletedPath = strings.TrimPrefix(deletedPath, backupDirName)
		deletedPath = strings.TrimPrefix(deletedPath, "/")

		// encrypt the deleted path name
		encryptedDeletedRelPath, err := cryptography.EncryptFilename(key, deletedPath)
		if err != nil {
			log.Printf("error: createDeletedPathsKeys(): could not encrypt deleted rel path ('%s'): %v\n", deletedPath, err)
			return err
		}

		// Insert a slash in the middle of encrypted relPath b/c server won't
		// allow path components > 255 characters
		encryptedDeletedRelPath = backup.InsertSlashIntoEncRelPath(encryptedDeletedRelPath)

		// create an object in this snapshot like encBackupDirName/encSnapshotName/##encRelPath
		// where ## prefix indicates rel path was deleted since prev snapshot
		objName := encryptedBackupDirName + "/" + encryptedSnapshotName + "/##" + encryptedDeletedRelPath
		if err = objst.UploadObjFromBuffer(ctx, bucket, objName, make([]byte, 0), objstore.ComputeETag([]byte{})); err != nil {
			log.Printf("error: createDeletedPathsKeys(): could not UploadObjFromBuffer ('%s'): %v\n", objName, err)
			return err
		}

		// Delete dirents row for backupDirName/relPath
		err = db.DeleteDirEntByPath(backupDirName, deletedPath)
		if err != nil {
			log.Printf("DeleteDirEntByPath failed: %v", err)
			return err
		}

		if showNameOnSuccess {
			fmt.Printf("Marked as deleted %s\n", deletedPath)
		}
	}

	return nil
}
