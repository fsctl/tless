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

	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/fstraverse"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
)

// Dependency injection function types
type CheckAndHandleCancelationFuncType func(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, globalsLock *sync.Mutex, db *database.DB, backupDirPath string, snapshotName string) bool
type UpdateProgressFuncType func(finished int64, total int64, globalsLock *sync.Mutex, vlog *util.VLog)
type SetReplayInitialProgressFuncType func(finished int64, total int64, backupDirName string, globalsLock *sync.Mutex, vlog *util.VLog)
type SetBackupInitialProgressFuncType func(finished int64, total int64, backupDirName string, globalsLock *sync.Mutex, vlog *util.VLog)

func DoJournaledBackup(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, db *database.DB, globalsLock *sync.Mutex, backupDirPath string, excludes []string, vlog *util.VLog, checkAndHandleTraversalCancelation fstraverse.CheckAndHandleTraversalCancelationFuncType, checkAndHandleCancelationFunc CheckAndHandleCancelationFuncType, setBackupInitialProgressFunc SetBackupInitialProgressFuncType, updateBackupProgressFunc UpdateProgressFuncType, stats *BackupStats) (backupReportedEvents []util.ReportedEvent, breakFromLoop bool, continueLoop bool, fatalError bool) {
	// Return values
	breakFromLoop = false
	continueLoop = false
	fatalError = false

	// The memory DB pnly exists within this routine and the subroutines it calls:
	// - We create it here at the beginning using file DB's current contents.
	// - We defer til exit of this routine mem db's persist back to disk and closing of mem db.
	// - Periodically, during uploads, we persist the mem db back to disk for protection in
	// case of crash.
	// - Mem db accesses are serialized by same lock as regular db.

	// Init the mem db
	util.LockIf(globalsLock)
	dbMem, err := database.NewDB(":memory:")
	util.UnlockIf(globalsLock)
	if err != nil {
		log.Fatalf("error: cannot open memory database: %v", err)
	}
	util.LockIf(globalsLock)
	db.BackupTo(dbMem)
	util.UnlockIf(globalsLock)
	memDbLastPersistedToFileUnixtime := time.Now().Unix()

	// This function will persist the memory db back to disk (overwriting disk db) in 3 cases:
	// 1) If forcePersist is true
	// 2) If maximum time since last persist has been exceeded
	// 3) If minimum time since last persist exceeded AND it's a "good time" to parallelize with
	// upload operation (ie, goodTime is true)
	persistMemDbToFile := func(runWhileUploadingFinished chan bool, goodTime bool, forcePersist bool) {
		defer func() {
			if runWhileUploadingFinished != nil {
				runWhileUploadingFinished <- true
			}
		}()
		secondsSinceLastPersist := time.Now().Unix() - memDbLastPersistedToFileUnixtime
		minPersistInterval := int64(10 * 60)
		maxPersistInterval := int64(20 * 60)
		if (forcePersist) || (secondsSinceLastPersist > maxPersistInterval) || ((secondsSinceLastPersist > minPersistInterval) && goodTime) {
			vlog.Printf("PERSIST_MEMDB> starting persist (b/c forcePersist=%v || max=%v || min=%v && goodTime=%v)", forcePersist, (secondsSinceLastPersist > maxPersistInterval), (secondsSinceLastPersist > minPersistInterval), goodTime)
			util.LockIf(globalsLock)
			dbMem.BackupTo(db)
			util.UnlockIf(globalsLock)
			vlog.Println("PERSIST_MEMDB> finished persist")
			memDbLastPersistedToFileUnixtime = time.Now().Unix()
		}
	}
	defer func() {
		persistMemDbToFile(nil, true, true)
		dbMem.Close()
	}()

	// Traverse the filesystem looking for changed directory entries
	backupDirName := filepath.Base(backupDirPath)
	util.LockIf(globalsLock)
	prevPaths, err := dbMem.GetAllKnownPaths(backupDirName)
	util.UnlockIf(globalsLock)
	if err != nil {
		log.Printf("error: DoJournaledBackup: cannot get paths list: %v", err)
		fatalError = true
		return
	}
	var backupIdsQueue fstraverse.BackupIdsQueue
	backupReportedEvents, err = fstraverse.Traverse(backupDirPath, prevPaths, dbMem, globalsLock, &backupIdsQueue, excludes, checkAndHandleTraversalCancelation)
	if errors.Is(err, fstraverse.ErrTraversalCanceled) {
		breakFromLoop = true // signals cancelation to caller
		return
	}

	// Iterate over the queue of backup dirent id's inserting them into journal
	util.LockIf(globalsLock)
	insertBJTxn, err := dbMem.NewInsertBackupJournalStmt(backupDirPath)
	util.UnlockIf(globalsLock)
	if err != nil {
		log.Printf("error: DoJournaledBackup: could not bulk insert into journal: %v", err)
		fatalError = true
		return
	}
	for _, backupQueueItem := range backupIdsQueue.Items {
		util.LockIf(globalsLock)
		err = insertBJTxn.InsertBackupJournalRow(int64(backupQueueItem.Id), database.Unstarted, backupQueueItem.ChangeType)
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: DoJournaledBackup: could not insert backup item into journal txn: %v", err)
			fatalError = true
			return
		}
	}
	util.LockIf(globalsLock)
	insertBJTxn.Close()
	util.UnlockIf(globalsLock)

	// Now iterate over queue of deleted items, bulk insert them into journal
	deletedDirentIds := make([]int64, 0)
	for deletedPath := range prevPaths {
		// deletedPath is backupDirName/deletedRelPath.  Make it just deletedRelPath
		deletedPath = strings.TrimPrefix(deletedPath, backupDirName)
		deletedPath = strings.TrimPrefix(deletedPath, "/")

		util.LockIf(globalsLock)
		isFound, _, dirEntId, err := dbMem.HasDirEnt(backupDirName, deletedPath)
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: DoJournaledBackup: failed while trying to find '%s'/'%s' in dirents: %v", backupDirName, deletedPath, err)
			continue
		}
		if !isFound {
			log.Printf("error: DoJournaledBackup: could not find '%s'/'%s' in dirents: %v", backupDirName, deletedPath, err)
			continue
		}
		vlog.Printf("Found deleted file '%s' / '%s' (dirents id = %d)", backupDirName, deletedPath, dirEntId)
		deletedDirentIds = append(deletedDirentIds, int64(dirEntId))
	}
	util.LockIf(globalsLock)
	insertBJTxn, err = dbMem.NewInsertBackupJournalStmt(backupDirPath)
	util.UnlockIf(globalsLock)
	if err != nil {
		log.Printf("error: DoJournaledBackup: could not bulk insert into journal: %v", err)
		fatalError = true
		return
	}
	for _, dirEntId := range deletedDirentIds {
		util.LockIf(globalsLock)
		err = insertBJTxn.InsertBackupJournalRow(dirEntId, database.Unstarted, database.Deleted)
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: DoJournaledBackup: could not insert deleted item into journal txn: %v", err)
			fatalError = true
			return
		}
	}
	util.LockIf(globalsLock)
	insertBJTxn.Close()
	util.UnlockIf(globalsLock)

	// Get the snapshot name from timestamp in backup_info
	util.LockIf(globalsLock)
	_, snapshotUnixtime, err := dbMem.GetJournaledBackupInfo()
	util.UnlockIf(globalsLock)
	if errors.Is(err, sql.ErrNoRows) {
		// If no rows were just inserted into journal, then nothing to backup for this snapshot
		vlog.Printf("Nothing inserted in journal => nothing to back up")
		continueLoop = true
		return
	} else if err != nil {
		log.Printf("error: DoJournaledBackup: dbMem.GetJournaledBackupInfo: %v", err)
		fatalError = true
		return
	}
	snapshotName := time.Unix(snapshotUnixtime, 0).UTC().Format("2006-01-02_15.04.05")

	// Set the initial progress bar
	if setBackupInitialProgressFunc != nil {
		util.LockIf(globalsLock)
		finished, total, err := dbMem.GetBackupJournalCounts()
		util.UnlockIf(globalsLock)
		if err != nil {
			log.Printf("error: DoJournaledBackup: dbMem.GetBackupJournalCounts: %v", err)
		}
		setBackupInitialProgressFunc(finished, total, backupDirName, globalsLock, vlog)
	}

	breakFromLoop = PlayBackupJournal(ctx, key, dbMem, globalsLock, backupDirPath, snapshotName, objst, bucket, vlog, checkAndHandleCancelationFunc, updateBackupProgressFunc, persistMemDbToFile, stats)
	return
}

func PlayBackupJournal(ctx context.Context, key []byte, db *database.DB, globalsLock *sync.Mutex, backupDirPath string, snapshotName string, objst *objstore.ObjStore, bucket string, vlog *util.VLog, checkAndHandleCancelationFunc CheckAndHandleCancelationFuncType, updateProgressFunc UpdateProgressFuncType, persistMemDbToFile runWhileUploadingFuncType, stats *BackupStats) (breakFromLoop bool) {
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

	// Get the previous snapshot so we know the chunk extents for all the unchanged files
	groupedObjects, err := snapshots.GetGroupedSnapshots(ctx, objst, key, bucket, vlog, nil, nil)
	if err != nil {
		log.Printf("Could not get grouped snapshots: %v", err)
		return true
	}
	prevSnapshot := groupedObjects[filepath.Base(backupDirPath)].GetMostRecentSnapshot()

	// closure used inside loop to eliminate duplicated code
	writeIndexFileAndWipeJournal := func() {
		vlog.Printf("Finished the journal (re-)play")
		progressUpdateClosure()

		err = snapshots.WriteIndexFile(ctx, globalsLock, db, objst, bucket, key, filepath.Base(backupDirPath), snapshotName)
		if err != nil {
			log.Println("error: PlayBackupJournal: writeIndexFileAndWipeJournal: couldn't write index file: ", err)
		}
		vlog.Printf("Deleting all journal rows")
		util.LockIf(globalsLock)
		err = db.WipeBackupJournal()
		util.UnlockIf(globalsLock)
		if err != nil {
			if errors.Is(err, database.ErrJournalHasUnfinishedTasks) {
				log.Println("error: PlayBackupJournal: writeIndexFileAndWipeJournal: tried to complete journal while it still had unfinished tasks (skipping)")
				return
			} else {
				log.Println("error: PlayBackupJournal: writeIndexFileAndWipeJournal: db.CompleteBackupJournal failed: ", err)
				return
			}
		}
		vlog.Printf("Done with journal")
	}

	cp := newChunkPacker(ctx, objst, bucket, db, globalsLock, key, vlog, persistMemDbToFile)

	// Force persist once before the backup starts
	if persistMemDbToFile != nil {
		persistMemDbToFile(nil, true, true)
	}

	for {
		// Sleep this go routine briefly on every iteration of the for loop
		time.Sleep(time.Millisecond * 1)

		// Has cancelation been requested?
		if checkAndHandleCancelationFunc != nil {
			isCanceled := checkAndHandleCancelationFunc(ctx, key, objst, bucket, globalsLock, db, backupDirPath, snapshotName)
			if isCanceled {
				return true
			}
		}

		// On every iteration, we persist mem db without the force option so it can decide when
		// it's been long enough (time b/t persists is defined in the function)
		if persistMemDbToFile != nil {
			persistMemDbToFile(nil, false, false)
		}

		util.LockIf(globalsLock)
		bjt, err := db.ClaimNextBackupJournalTask()
		util.UnlockIf(globalsLock)
		if err != nil {
			if errors.Is(err, database.ErrNoWork) {
				vlog.Println("PlayBackupJournal: no work found in journal... done")

				// Commit the pending chunk packer if it has anything in it
				isJournalComplete := cp.Complete()
				if !isJournalComplete {
					log.Println("error: PlayBackupJournal: something's wrong, journal should be complete at this point")
				}

				// Journal is completed: write index file, wipe journal and return
				writeIndexFileAndWipeJournal()
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
			log.Printf("error: PlayBackupJournal: db.GetDirEntPaths: could not get dirent id '%d'\n", bjt.DirEntId)
		}

		// For JSON serialization into journal
		crp := &snapshots.CloudRelPath{
			RelPath: relPath,
		}

		if stats != nil {
			stats.AddFile()
		}

		finishTaskImmediately := true
		if bjt.ChangeType == database.Updated {
			//vlog.Printf("Backing up '%s/%s'", rootDirName, relPath)
			chunkExtents, pendingInChunkPacker, err := Backup(ctx, key, rootDirName, relPath, backupDirPath, snapshotName, objst, bucket, vlog, cp, bjt)
			if err != nil {
				log.Printf("error: PlayBackupJournal (Updated): backup.Backup: %v", err)
				completeTask(db, globalsLock, bjt, nil)
				finishTaskImmediately = false
			} else {
				if stats != nil {
					stats.AddBytesFromChunkExtents(chunkExtents)
				}

				if pendingInChunkPacker {
					finishTaskImmediately = false
				} else {
					crp.ChunkExtents = chunkExtents
				}
			}
		} else if bjt.ChangeType == database.Unchanged {
			if prevSnapshot != nil {
				// Just use the same extents as prev snapshot had
				chunkExtents := prevSnapshot.RelPaths[relPath].ChunkExtents
				crp.ChunkExtents = chunkExtents

				if stats != nil {
					stats.AddBytesFromChunkExtents(chunkExtents)
				}
			} else {
				log.Printf("warning: found an unchanged file but have no previous snapshot; treating it as updated: '%s/%s'", rootDirName, relPath)
				chunkExtents, pendingInChunkPacker, err := Backup(ctx, key, rootDirName, relPath, backupDirPath, snapshotName, objst, bucket, vlog, cp, bjt)
				if err != nil {
					log.Printf("error: PlayBackupJournal (Unchanged): backup.Backup: %v", err)
					completeTask(db, globalsLock, bjt, nil)
					finishTaskImmediately = false
				} else {
					if stats != nil {
						stats.AddBytesFromChunkExtents(chunkExtents)
					}

					if pendingInChunkPacker {
						finishTaskImmediately = false
					} else {
						crp.ChunkExtents = chunkExtents
					}
				}
			}
		} else if bjt.ChangeType == database.Deleted {
			// Remove from dirents table
			if err = purgeFromDb(db, globalsLock, filepath.Base(backupDirPath), relPath); err != nil {
				log.Printf("error: PlayBackupJournal (Deleted): failed to purge from dirents '%s': %v", relPath, err)
			}
			crp = nil
		} else {
			log.Printf("error: PlayBackupJournal: unrecognized journal type '%v' on '%s'", bjt.ChangeType, relPath)
		}

		if finishTaskImmediately {
			updateLastBackupTime(db, globalsLock, bjt.DirEntId)
			isJournalComplete := completeTask(db, globalsLock, bjt, crp)
			if isJournalComplete {
				writeIndexFileAndWipeJournal()
				return
			}
		}

		progressUpdateClosure()
	}
}

func updateLastBackupTime(db *database.DB, dbLock *sync.Mutex, dirEntId int64) {
	util.LockIf(dbLock)
	err := db.UpdateLastBackupTime(int(dirEntId))
	util.UnlockIf(dbLock)
	if err != nil {
		log.Printf("error: updateLastBackupTime: db.UpdateLastBackupTime: %v", err)
	}
}

func completeTask(db *database.DB, dbLock *sync.Mutex, bjt *database.BackupJournalTask, crp *snapshots.CloudRelPath) (isJournalComplete bool) {
	var err error
	util.LockIf(dbLock)
	if crp == nil {
		isJournalComplete, err = db.CompleteBackupJournalTask(bjt, nil)
	} else {
		isJournalComplete, err = db.CompleteBackupJournalTask(bjt, crp.ToJson())
	}
	util.UnlockIf(dbLock)
	if err != nil {
		log.Printf("error: completeTask: db.CompleteBackupJournalTask: %v", err)
	}
	return
}

func purgeFromDb(db *database.DB, dbLock *sync.Mutex, backupDirName string, deletedPath string) error {
	// Delete dirents row for backupDirName/relPath
	util.LockIf(dbLock)
	err := db.DeleteDirEntByPath(backupDirName, deletedPath)
	util.UnlockIf(dbLock)
	if err != nil {
		log.Printf("error: createDeletedPathKeyAndPurgeFromDb: DeleteDirEntByPath failed: %v", err)
		return err
	}

	return nil
}

func ReplayBackupJournal(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, db *database.DB, globalsLock *sync.Mutex, vlog *util.VLog, setReplayInitialProgressFunc SetReplayInitialProgressFuncType, checkAndHandleCancelationFunc CheckAndHandleCancelationFuncType, updateProgressFunc UpdateProgressFuncType) util.ReportedEvent {
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

	breakFromLoop := PlayBackupJournal(ctx, key, db, globalsLock, backupDirPath, snapshotName, objst, bucket, vlog, checkAndHandleCancelationFunc, updateProgressFunc, nil, nil)

	vlog.Println("Journal replay finished")

	if breakFromLoop {
		return util.ReportedEvent{
			Kind:     util.INFO_BACKUP_CANCELED,
			Path:     "",
			IsDir:    false,
			Datetime: time.Now().Unix(),
			Msg:      "",
		}
	} else {
		return util.ReportedEvent{
			Kind:     util.INFO_BACKUP_COMPLETED,
			Path:     "",
			IsDir:    false,
			Datetime: time.Now().Unix(),
			Msg:      "",
		}
	}
}
