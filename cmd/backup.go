package cmd

import (
	"context"
	"log"
	"path/filepath"
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
		var backupIdsQueue fstraverse.BackupIdsQueue
		fstraverse.Traverse(backupDirPath, prevPaths, db, &backupIdsQueue)

		// Any remaining prevPaths represent deleted files, so upload keys to mark them deleted and remove from
		// dirents table
		if err = createDeletedPathsKeysAndPurgeFromDb(ctx, objst, cfgBucket, db, encKey, backupDirName, snapshotName, prevPaths); err != nil {
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
				break
			}

			backupIdsQueue.Lock.Unlock()
			if id != 0 {
				doActionBackup(ctx, objst, cfgBucket, id, &backupIdsQueue, db, backupDirPath, snapshotName)
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

		// Insert a slash in the middle of encrypted relPath b/c server won't
		// allow path components > 255 characters
		encryptedDeletedRelPath = backup.InsertSlashIntoEncRelPath(encryptedDeletedRelPath)

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

func doActionBackup(ctx context.Context, objst *objstore.ObjStore, bucket string, dirEntId int, backupIdsQueue *fstraverse.BackupIdsQueue, db *database.DB, backupDirPath string, snapshotName string) {
	if err := backup.Backup(ctx, encKey, db, backupDirPath, snapshotName, dirEntId, objst, bucket); err != nil {
		log.Printf("error: Backup(): %v", err)
		reEnqueue(backupIdsQueue, dirEntId)
		return
	}
	err := db.UpdateLastBackupTime(dirEntId)
	if err != nil {
		log.Printf("error: UpdateLastBackupTime(): %v", err)
		reEnqueue(backupIdsQueue, dirEntId)
		return
	}
}

func reEnqueue(backupIdsQueue *fstraverse.BackupIdsQueue, dirEntId int) {
	backupIdsQueue.Lock.Lock()
	backupIdsQueue.Ids = append(backupIdsQueue.Ids, dirEntId)
	backupIdsQueue.Lock.Unlock()
}
