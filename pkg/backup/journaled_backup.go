package backup

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/fstraverse"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
)

// Dependency injection function types
type CheckAndHandleCancelationFuncType func(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, globalsLock *sync.Mutex, db *database.DB, backupDirPath string, snapshotName string) bool
type UpdateProgressFuncType func(finished int64, total int64, globalsLock *sync.Mutex, vlog *util.VLog)
type SetReplayInitialProgressFuncType func(finished int64, total int64, backupDirName string, globalsLock *sync.Mutex, vlog *util.VLog)
type SetBackupInitialProgressFuncType func(finished int64, total int64, backupDirName string, globalsLock *sync.Mutex, vlog *util.VLog)

func DoJournaledBackup(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, db *database.DB, globalsLock *sync.Mutex, backupDirPath string, excludePaths []string, vlog *util.VLog, checkAndHandleCancelationFunc CheckAndHandleCancelationFuncType, setBackupInitialProgressFunc SetBackupInitialProgressFuncType, updateBackupProgressFunc UpdateProgressFuncType) (breakFromLoop bool, continueLoop bool, fatalError bool) {
	// Return values
	breakFromLoop = false
	continueLoop = false
	fatalError = false

	// Traverse the filesystem looking for changed directory entries
	backupDirName := filepath.Base(backupDirPath)
	util.LockIf(globalsLock)
	prevPaths, err := db.GetAllKnownPaths(backupDirName)
	util.UnlockIf(globalsLock)
	if err != nil {
		log.Printf("error: DoJournaledBackup: cannot get paths list: %v", err)
		fatalError = true
		return
	}
	var backupIdsQueue fstraverse.BackupIdsQueue
	fstraverse.Traverse(backupDirPath, prevPaths, db, globalsLock, &backupIdsQueue, excludePaths)

	// Iterate over the queue of dirent id's inserting them into journal
	util.LockIf(globalsLock)
	insertBJTxn, err := db.NewInsertBackupJournalStmt(backupDirPath)
	util.UnlockIf(globalsLock)
	if err != nil {
		log.Printf("error: DoJournaledBackup: could not bulk insert into journal: %v", err)
		fatalError = true
		return
	}
	for _, dirEntId := range backupIdsQueue.Ids {
		util.LockIf(globalsLock)
		insertBJTxn.InsertBackupJournalRow(int64(dirEntId), database.Unstarted, database.Updated)
		util.UnlockIf(globalsLock)
	}
	for deletedPath := range prevPaths {
		// deletedPath is backupDirName/deletedRelPath.  Make it just deletedRelPath
		deletedPath = strings.TrimPrefix(deletedPath, backupDirName)
		deletedPath = strings.TrimPrefix(deletedPath, "/")

		util.LockIf(globalsLock)
		isFound, _, dirEntId, err := db.HasDirEnt(backupDirName, deletedPath)
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: DoJournaledBackup: hile trying to find '%s'/'%s' in dirents: %v", backupDirName, deletedPath, err)
			continue
		}
		if !isFound {
			log.Printf("error: DoJournaledBackup: could not find '%s'/'%s' in dirents: %v", backupDirName, deletedPath, err)
			continue
		}
		vlog.Printf("Found deleted file '%s'/'%s' (dirents id = %d)", backupDirName, deletedPath, dirEntId)
		util.LockIf(globalsLock)
		insertBJTxn.InsertBackupJournalRow(int64(dirEntId), database.Unstarted, database.Deleted)
		util.UnlockIf(globalsLock)
	}
	util.LockIf(globalsLock)
	insertBJTxn.Close()
	util.UnlockIf(globalsLock)

	// Get the snapshot name from timestamp in backup_info
	util.LockIf(globalsLock)
	_, snapshotUnixtime, err := db.GetJournaledBackupInfo()
	util.UnlockIf(globalsLock)
	if errors.Is(err, sql.ErrNoRows) {
		// If no rows were just inserted into journal, then nothing to backup for this snapshot
		vlog.Printf("Nothing inserted in journal => nothing to back up")
		continueLoop = true
		return
	} else if err != nil {
		log.Printf("error: DoJournaledBackup: db.GetJournaledBackupInfo: %v", err)
		fatalError = true
		return
	}
	snapshotName := time.Unix(snapshotUnixtime, 0).UTC().Format("2006-01-02_15.04.05")

	// Set the initial progress bar
	if setBackupInitialProgressFunc != nil {
		util.LockIf(globalsLock)
		finished, total, err := db.GetBackupJournalCounts()
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: DoJournaledBackup: db.GetBackupJournalCounts: %v", err)
		}
		setBackupInitialProgressFunc(finished, total, backupDirName, globalsLock, vlog)
	}

	breakFromLoop = PlayBackupJournal(ctx, key, db, globalsLock, backupDirPath, snapshotName, objst, bucket, vlog, checkAndHandleCancelationFunc, updateBackupProgressFunc)
	return
}

func PlayBackupJournal(ctx context.Context, key []byte, db *database.DB, globalsLock *sync.Mutex, backupDirPath string, snapshotName string, objst *objstore.ObjStore, bucket string, vlog *util.VLog, checkAndHandleCancelationFunc CheckAndHandleCancelationFuncType, updateProgressFunc UpdateProgressFuncType) (breakFromLoop bool) {
	// By default, don't signal we want to break out of caller's loop over backups
	breakFromLoop = false

	progressUpdateClosure := func() {
		// Update the percentage based on where we are now
		util.LockIf(globalsLock)
		finished, total, err := db.GetBackupJournalCounts()
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: PlayBackupJournal: db.GetBackupJournalCounts: %v", err)
		} else {
			if updateProgressFunc != nil {
				updateProgressFunc(finished, total, globalsLock, vlog)
			}
		}
	}

	for {
		// Sleep this go routine briefly on every iteration of the for loop
		time.Sleep(time.Millisecond * 50)

		// Has cancelation been requested?
		if checkAndHandleCancelationFunc != nil {
			isCanceled := checkAndHandleCancelationFunc(ctx, key, objst, bucket, globalsLock, db, backupDirPath, snapshotName)
			if isCanceled {
				return true
			}
		}

		util.LockIf(globalsLock)
		bjt, err := db.ClaimNextBackupJournalTask()
		util.UnlockIf(globalsLock)
		if err != nil {
			if errors.Is(err, database.ErrNoWork) {
				vlog.Println("PlayBackupJournal: no work found in journal... done")
				return
			} else {
				log.Println("error: PlayBackupJournal: db.ClaimNextBackupJournalTask: ", err)
				return
			}
		}

		util.LockIf(globalsLock)
		rootDirName, relPath, err := db.GetDirEntPaths(int(bjt.DirEntId))
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: PlayBackupJournal: db.GetDirEntPaths(): could not get dirent id '%d'\n", bjt.DirEntId)
		}

		crp := objstorefs.CloudRelPath{}
		if bjt.ChangeType == database.Updated {
			vlog.Printf("Backing up '%s/%s'", rootDirName, relPath)
			encryptedRelPath, encryptedChunks, err := Backup(ctx, key, rootDirName, relPath, backupDirPath, snapshotName, objst, bucket, false)
			if err != nil {
				log.Printf("error: PlayBackupJournal: backup.Backup(): %v", err)
				continue
			}

			// For JSON serialization into journal
			crp.DecryptedRelPath = relPath
			crp.EncryptedRelPathStripped = encryptedRelPath
			crp.EncryptedChunkNames = encryptedChunks
			crp.IsDeleted = false
		} else if bjt.ChangeType == database.Deleted {
			vlog.Printf("Deleting '%s/%s'", rootDirName, relPath)
			encryptedRelPath, err := createDeletedPathKeyAndPurgeFromDb(db, globalsLock, key, rootDirName, relPath)
			if err != nil {
				log.Printf("error: PlayBackupJournal: failed on deleting path '%s': %v", relPath, err)
				continue
			}

			// For JSON serialization into journal
			crp.DecryptedRelPath = relPath
			crp.EncryptedRelPathStripped = encryptedRelPath
			crp.EncryptedChunkNames = nil
			crp.IsDeleted = true
		}

		util.LockIf(globalsLock)
		err = db.UpdateLastBackupTime(int(bjt.DirEntId))
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: PlayBackupJournal: db.UpdateLastBackupTime(): %v", err)
		}

		util.LockIf(globalsLock)
		isJournalComplete, err := db.CompleteBackupJournalTask(bjt, crp.ToJson())
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: PlayBackupJournal: db.CompleteBackupJournalTask: %v", err)
		}
		if isJournalComplete {
			vlog.Printf("Finished the journal (re)play")
			progressUpdateClosure()

			err = snapshots.WriteIndexFile(ctx, globalsLock, db, objst, bucket, key, filepath.Base(backupDirPath), snapshotName)
			if err != nil {
				log.Println("error: PlayBackupJournal: couldn't write index file: ", err)
			}
			vlog.Printf("Deleting all journal rows")
			util.LockIf(globalsLock)
			err = db.CompleteBackupJournal()
			util.UnlockIf(globalsLock)
			if err != nil {
				if errors.Is(err, database.ErrJournalHasUnfinishedTasks) {
					log.Println("error: PlayBackupJournal: tried to complete journal while it still had unfinished tasks")
					continue
				} else {
					log.Println("error: PlayBackupJournal: db.CompleteBackupJournal() failed: ", err)
					continue
				}
			}
			vlog.Printf("Done with journal")
			return
		} else {
			progressUpdateClosure()
		}
	}
}

func createDeletedPathKeyAndPurgeFromDb(db *database.DB, dbLock *sync.Mutex, key []byte, backupDirName string, deletedPath string) (encryptedDeletedRelPath string, err error) {
	// encrypt the deleted path name
	encryptedDeletedRelPath, err = cryptography.EncryptFilename(key, deletedPath)
	if err != nil {
		log.Printf("error: createDeletedPathKeyAndPurgeFromDb(): could not encrypt deleted rel path ('%s'): %v\n", deletedPath, err)
		return "", err
	}

	// Insert a slash in the middle of encrypted relPath b/c server won't
	// allow path components > 255 characters
	encryptedDeletedRelPath = InsertSlashIntoEncRelPath(encryptedDeletedRelPath)

	// Delete dirents row for backupDirName/relPath
	util.LockIf(dbLock)
	err = db.DeleteDirEntByPath(backupDirName, deletedPath)
	util.UnlockIf(dbLock)
	if err != nil {
		log.Printf("error: createDeletedPathKeyAndPurgeFromDb: DeleteDirEntByPath failed: %v", err)
		return "", err
	}

	return encryptedDeletedRelPath, nil
}

func ReplayBackupJournal(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, db *database.DB, globalsLock *sync.Mutex, vlog *util.VLog, setReplayInitialProgressFunc SetReplayInitialProgressFuncType, checkAndHandleCancelationFunc CheckAndHandleCancelationFuncType, updateProgressFunc UpdateProgressFuncType) {
	// Reset all InProgress -> Unstarted
	util.LockIf(globalsLock)
	err := db.ResetAllInProgressBackupJournalTasks()
	util.UnlockIf(globalsLock)
	if err != nil {
		log.Println("error: ReplayBackupJournal: db.ResetAllInProgressBackupJournalTasks: ", err)
	}

	// Reconstruct backupDirPath, backupDirName and snapshotName from backup_info table
	util.LockIf(globalsLock)
	backupDirPath, snapshotUnixtime, err := db.GetJournaledBackupInfo()
	util.UnlockIf(globalsLock)
	if err != nil {
		log.Printf("error: ReplayBackupJournal: db.GetJournaledBackupInfo(): %v", err)
	}
	backupDirName := filepath.Base(backupDirPath)
	snapshotName := time.Unix(snapshotUnixtime, 0).UTC().Format("2006-01-02_15.04.05")

	// Set the initial progress where the back up is starting
	if setReplayInitialProgressFunc != nil {
		util.LockIf(globalsLock)
		finished, total, err := db.GetBackupJournalCounts()
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: ReplayBackupJournal: db.GetBackupJournalCounts: %v", err)
		}
		setReplayInitialProgressFunc(finished, total, backupDirName, globalsLock, vlog)
	}

	// Roll the journal forward
	_ = PlayBackupJournal(ctx, key, db, globalsLock, backupDirPath, snapshotName, objst, bucket, vlog, checkAndHandleCancelationFunc, updateProgressFunc)

	vlog.Println("Journal replay finished")
}
