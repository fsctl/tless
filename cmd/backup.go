package cmd

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsctl/trustlessbak/pkg/backup"
	"github.com/fsctl/trustlessbak/pkg/cryptography"
	"github.com/fsctl/trustlessbak/pkg/database"
	"github.com/fsctl/trustlessbak/pkg/fstraverse"
	"github.com/fsctl/trustlessbak/pkg/objstore"
	"github.com/spf13/cobra"
)

var (
	// Flags
	cfgDirs []string

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
	backupCmd.Flags().StringArrayVarP(&cfgDirs, "dirs", "d", nil, "directories to backup (can use multiple times)")
	rootCmd.AddCommand(backupCmd)
}

func backupMain() {
	ctx := context.Background()

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
	trySaveSaltToServer(ctx, objst, cfgBucket)

	for _, backupDirPath := range cfgDirs {
		backupDirName := filepath.Base(backupDirPath)

		snapshotName := time.Now().UTC().Format("2006-01-02_15:04:05")

		// Traverse the filesystem looking for changed directory entries
		prevPaths, err := db.GetAllKnownPaths(backupDirName)
		if err != nil {
			log.Fatalf("Error: cannot get paths list: %v", err)
		}
		fstraverse.Traverse(backupDirPath, prevPaths, db)

		// Any remaining prevPaths represent deleted files, so upload keys to mark them deleted and remove from
		// dirents table
		if err = createDeletedPathsKeysAndPurgeFromDb(ctx, objst, cfgBucket, db, encKey, backupDirName, snapshotName, prevPaths); err != nil {
			log.Println("error: failed creatingn deleted paths keys")
		}

		// Work through the queue
		for {
			item, err := db.DequeueNextItem()
			if errors.Is(err, database.ErrNoWork) {
				log.Println("queue empty")
				break
			} else if err != nil {
				log.Printf("error: cannot dequeue item: %v", err)
				break
			}

			switch item.Action {
			case database.QueueActionBackup:
				doActionBackup(ctx, objst, cfgBucket, item, db, backupDirPath, snapshotName)
			// case database.QueueActionDelete:
			// 	doActionDelete(ctx, objst, cfgBucket, encKey, item, db, snapshotName)
			default:
				log.Printf("error: dequeued unrecognized '%s'", item.Action)
			}
		}
	}

	log.Printf("done")
}

func createDeletedPathsKeysAndPurgeFromDb(ctx context.Context, objst *objstore.ObjStore, bucket string, db *database.DB, key []byte, backupDirName string, snapshotName string, deletedPaths map[string]int) error {
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

		// create an object in this snapshot like encBackupDirName/encSnapshotName/__encRelPath
		// where __ prefix indicates rel path was deleted since prev snapshot
		objName := encryptedBackupDirName + "/" + encryptedSnapshotName + "/__" + encryptedDeletedRelPath
		if err = objst.UploadObjFromBuffer(ctx, bucket, objName, make([]byte, 0)); err != nil {
			log.Printf("error: createDeletedPathsKeys(): could not UploadObjFromBuffer ('%s'): %v\n", objName, err)
			return err
		}

		// Delete dirents row for backupDirName/relPath
		err = db.DeleteDirEntByPath(backupDirName, deletedPath)
		if err != nil {
			log.Printf("DeleteDirEntByPath failed: %v", err)
			return err
		}
	}

	return nil
}

// SNAPSHOTTODO: delete this
// func createDeletedPathsList(key []byte, deletedPaths map[string]int) string {
// 	deletedPathsList := ""
// 	for deletedPath := range deletedPaths {
// 		encryptedDeletedPath, err := cryptography.EncryptFilename(key, deletedPath)
// 		if err != nil {
// 			log.Printf("error: could not encrypt filename %s: %v", deletedPath, err)
// 			return ""
// 		}
// 		deletedPathsList = deletedPathsList + encryptedDeletedPath + "\n"
// 	}
// 	return deletedPathsList
// }

func trySaveSaltToServer(ctx context.Context, objst *objstore.ObjStore, bucket string) {
	var salt string
	salt, err := objst.TryReadSalt(ctx, bucket)
	if err != nil {
		log.Printf("warning: failed to read salt from server: %v\n", err)
	}
	if salt == "" {
		// If salt read back from server is blank, try to save the current salt to the server for
		// future use.
		if err = objst.TryWriteSalt(ctx, bucket, cfgSalt); err != nil {
			log.Printf("warning: failed to write salt to server for backup: %v\n", err)
		}
	} else if salt != cfgSalt {
		// Warn user that we found a different salt on the server than what we have locally
		log.Printf("warning: local salt =/= server saved salt; using local config salt ('%s') and ignoring server salt ('%s')\n", cfgSalt, salt)
	} else {
		return
	}
}

/*
func doActionDelete(ctx context.Context, objst *objstore.ObjStore, bucket string, key []byte, item *database.QueueItemDescription, db *database.DB, snapshotName string) {
	// SNAPSHOTTODO:
	// - this should go away
	// - don't enqueue deletes
	// - do them in bulk on return from Traverse to create one big DELETED file
	////////////////////////////////

	pathParts := strings.SplitN(item.Arg1, "/", 2)
	backupDirName := pathParts[0]
	relPath := pathParts[1]

	// Encrypt backupDirName and relPath to get encrypted representations matching server
	// object names
	encryptedBackupDirName, err := cryptography.EncryptFilename(key, backupDirName)
	if err != nil {
		log.Printf("error: could not encrypt filename '%s'", backupDirName)
		return
	}
	encryptedRelPath, err := cryptography.EncryptFilename(key, relPath)
	if err != nil {
		log.Printf("error: could not encrypt filename '%s'", relPath)
		return
	}

	// Get a list of all objects starting with encryptedBackupDirName/encryptedRelPath. There
	// may be >1 because of chunking.
	objectsForPath, err := objst.GetObjList(ctx, bucket, encryptedBackupDirName+"/"+encryptedRelPath)
	if err != nil {
		log.Printf("GetObjList failed: %v", err)
		return
	}

	// Delete all objects in list
	for objectName := range objectsForPath {
		err = objst.DeleteObj(ctx, bucket, objectName)
		if err != nil {
			log.Printf("DeleteObj failed: %v", err)
			return
		}
	}

	// Delete dirents row for backupDirName/relPath
	err = db.DeleteDirEntByPath(backupDirName, relPath)
	if err != nil {
		log.Printf("DeleteDirEntByPath failed: %v", err)
		return
	}
}
*/

func doActionBackup(ctx context.Context, objst *objstore.ObjStore, bucket string, item *database.QueueItemDescription, db *database.DB, backupDirPath string, snapshotName string) {
	dirEntId, err := strconv.Atoi(item.Arg1)
	if err != nil {
		log.Printf("error: Atoi(): malformed arg1 '%s': %v", item.Arg1, err)
		forceReEnqueueItem(db, item)
		return
	}
	if err = backup.Backup(ctx, encKey, db, backupDirPath, snapshotName, dirEntId, objst, bucket); err != nil {
		log.Printf("error: Backup(): %v", err)
		forceReEnqueueItem(db, item)
		return
	}
	err = db.UpdateLastBackupTime(dirEntId)
	if err != nil {
		log.Printf("error: UpdateLastBackupTime(): %v", err)
		forceReEnqueueItem(db, item)
		return
	}
	forceCompleteQueueItem(db, item)
}

// Calls CompleteQueueItem() and logs any error.  Returns nothing.
func forceCompleteQueueItem(db *database.DB, item *database.QueueItemDescription) {
	err := db.CompleteQueueItem(item.QueueId)
	if err != nil {
		log.Printf("warning: CompleteQueueItem(): %v", err)
	}
}

func forceReEnqueueItem(db *database.DB, item *database.QueueItemDescription) {
	err := db.ReEnqueuQueueItem(item.QueueId)
	if err != nil {
		log.Printf("warning: CompleteQueueItem(): %v", err)
	}
}
