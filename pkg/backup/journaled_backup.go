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
type CheckAndHandleCancelationFuncType func(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, backupDirPath string, snapshotName string) bool
type UpdateProgressFuncType func(finished int64, total int64, vlog *util.VLog)
type SetReplayInitialProgressFuncType func(finished int64, total int64, backupDirName string, vlog *util.VLog)
type SetBackupInitialProgressFuncType func(finished int64, total int64, backupDirName string, vlog *util.VLog)

//
// MemDB Functions (see note in DoJournaledBackup)
//
func initMemDb(dbLock *sync.Mutex, db *database.DB) (*database.DB, int64) {
	util.LockIf(dbLock)
	dbMem, err := database.NewDB(":memory:")
	util.UnlockIf(dbLock)
	if err != nil {
		log.Fatalf("error: cannot open memory database: %v", err)
	}
	util.LockIf(dbLock)
	db.BackupTo(dbMem)
	util.UnlockIf(dbLock)
	memDbLastPersistedToFileUnixtime := time.Now().Unix()
	return dbMem, memDbLastPersistedToFileUnixtime
}

// This function will persist the memory db back to disk (overwriting disk db) in 3 cases:
// 1) If forcePersist is true
// 2) If maximum time since last persist has been exceeded
// 3) If minimum time since last persist exceeded AND it's a "good time" to parallelize with
// upload operation (ie, goodTime is true)
func makePersistMemDbToFile(db *database.DB, dbMem *database.DB, dbLock *sync.Mutex, memDbLastPersistedToFileUnixtime int64, vlog *util.VLog) func(runWhileUploadingFinished chan bool, goodTime bool, forcePersist bool) {
	return func(runWhileUploadingFinished chan bool, goodTime bool, forcePersist bool) {
		defer func() {
			if runWhileUploadingFinished != nil {
				runWhileUploadingFinished <- true
			}
		}()
		secondsSinceLastPersist := time.Now().Unix() - memDbLastPersistedToFileUnixtime
		minPersistInterval := int64(5 * 60)
		maxPersistInterval := int64(10 * 60)
		if (forcePersist) || (secondsSinceLastPersist > maxPersistInterval) || ((secondsSinceLastPersist > minPersistInterval) && goodTime) {
			vlog.Printf("PERSIST_MEMDB> starting persist (b/c forcePersist=%v || max=%v || min=%v && goodTime=%v)", forcePersist, (secondsSinceLastPersist > maxPersistInterval), (secondsSinceLastPersist > minPersistInterval), goodTime)
			util.LockIf(dbLock)
			dbMem.BackupTo(db)
			util.UnlockIf(dbLock)
			vlog.Println("PERSIST_MEMDB> finished persist")
			memDbLastPersistedToFileUnixtime = time.Now().Unix()
		}
	}
}

func DoJournaledBackup(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, dbLock *sync.Mutex, db *database.DB, backupDirPath string, excludes []string, vlog *util.VLog, checkAndHandleTraversalCancelation fstraverse.CheckAndHandleTraversalCancelationFuncType, checkAndHandleCancelationFunc CheckAndHandleCancelationFuncType, setBackupInitialProgressFunc SetBackupInitialProgressFuncType, updateBackupProgressFunc UpdateProgressFuncType, stats *BackupStats, resourceUtilization string) (backupReportedEvents []util.ReportedEvent, breakFromLoop bool, continueLoop bool, fatalError bool) {
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
	dbMem, memDbLastPersistedToFileUnixtime := initMemDb(dbLock, db)
	persistMemDbToFile := makePersistMemDbToFile(db, dbMem, dbLock, memDbLastPersistedToFileUnixtime, vlog)
	defer func() {
		persistMemDbToFile(nil, true, true)
		util.LockIf(dbLock)
		dbMem.Close()
		util.UnlockIf(dbLock)
	}()

	// Traverse the filesystem looking for changed directory entries
	backupDirName := filepath.Base(backupDirPath)
	util.LockIf(dbLock)
	prevPaths, err := dbMem.GetAllKnownPaths(backupDirName)
	util.UnlockIf(dbLock)
	if err != nil {
		log.Printf("error: DoJournaledBackup: cannot get paths list: %v", err)
		fatalError = true
		return
	}
	var backupIdsQueue fstraverse.BackupIdsQueue
	backupReportedEvents, err = fstraverse.Traverse(backupDirPath, prevPaths, dbMem, dbLock, &backupIdsQueue, excludes, checkAndHandleTraversalCancelation, vlog)
	if errors.Is(err, fstraverse.ErrTraversalCanceled) {
		breakFromLoop = true // signals cancelation to caller
		return
	}

	// Iterate over the queue of backup dirent id's inserting them into journal
	util.LockIf(dbLock)
	insertBJTxn, err := dbMem.NewInsertBackupJournalStmt(backupDirPath)
	util.UnlockIf(dbLock)
	if err != nil {
		log.Printf("error: DoJournaledBackup: could not bulk insert into journal: %v", err)
		fatalError = true
		return
	}
	for _, backupQueueItem := range backupIdsQueue.Items {
		util.LockIf(dbLock)
		err = insertBJTxn.InsertBackupJournalRow(int64(backupQueueItem.Id), database.Unstarted, backupQueueItem.ChangeType)
		util.UnlockIf(dbLock)
		if err != nil {
			log.Printf("error: DoJournaledBackup: could not insert backup item into journal txn: %v", err)
			fatalError = true
			return
		}
	}
	util.LockIf(dbLock)
	insertBJTxn.Close()
	util.UnlockIf(dbLock)

	// Now iterate over queue of deleted items, bulk insert them into journal
	deletedDirentIds := make([]int64, 0)
	for deletedPath := range prevPaths {
		// deletedPath is backupDirName/deletedRelPath.  Make it just deletedRelPath
		deletedPath = strings.TrimPrefix(deletedPath, backupDirName)
		deletedPath = strings.TrimPrefix(deletedPath, "/")

		util.LockIf(dbLock)
		isFound, _, dirEntId, err := dbMem.HasDirEnt(backupDirName, deletedPath)
		util.UnlockIf(dbLock)
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
	util.LockIf(dbLock)
	insertBJTxn, err = dbMem.NewInsertBackupJournalStmt(backupDirPath)
	util.UnlockIf(dbLock)
	if err != nil {
		log.Printf("error: DoJournaledBackup: could not bulk insert into journal: %v", err)
		fatalError = true
		return
	}
	for _, dirEntId := range deletedDirentIds {
		util.LockIf(dbLock)
		err = insertBJTxn.InsertBackupJournalRow(dirEntId, database.Unstarted, database.Deleted)
		util.UnlockIf(dbLock)
		if err != nil {
			log.Printf("error: DoJournaledBackup: could not insert deleted item into journal txn: %v", err)
			fatalError = true
			return
		}
	}
	util.LockIf(dbLock)
	insertBJTxn.Close()
	util.UnlockIf(dbLock)

	// Get the snapshot name from timestamp in backup_info
	util.LockIf(dbLock)
	_, snapshotUnixtime, err := dbMem.GetJournaledBackupInfo()
	util.UnlockIf(dbLock)
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
		util.LockIf(dbLock)
		finished, total, err := dbMem.GetBackupJournalCounts()
		util.UnlockIf(dbLock)
		if err != nil {
			log.Printf("error: DoJournaledBackup: dbMem.GetBackupJournalCounts: %v", err)
		}
		setBackupInitialProgressFunc(finished, total, backupDirName, vlog)
	}

	breakFromLoop = PlayBackupJournal(ctx, key, dbLock, dbMem, backupDirPath, snapshotName, objst, bucket, vlog, checkAndHandleCancelationFunc, updateBackupProgressFunc, persistMemDbToFile, stats, resourceUtilization)
	return
}

func PlayBackupJournal(ctx context.Context, key []byte, dbLock *sync.Mutex, db *database.DB, backupDirPath string, snapshotName string, objst *objstore.ObjStore, bucket string, vlog *util.VLog, checkAndHandleCancelationFunc CheckAndHandleCancelationFuncType, updateProgressFunc UpdateProgressFuncType, persistMemDbToFile runWhileUploadingFuncType, stats *BackupStats, resourceUtilization string) (breakFromLoop bool) {
	// By default, don't signal we want to break out of caller's loop over backups
	breakFromLoop = false

	util.LockIf(dbLock)
	finishedCountJournal, totalCntJournal, err := db.GetBackupJournalCounts()
	util.UnlockIf(dbLock)
	if err != nil {
		log.Println("error: PlayBackupJournal: db.GetBackupJournalCounts failed: ", err)
		return
	}

	progressUpdateClosure := func(totalCntJournal int64, finishedCountJournal int64) {
		// Update the percentage based on where we are now
		if updateProgressFunc != nil {
			updateProgressFunc(finishedCountJournal, totalCntJournal, vlog)
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
		progressUpdateClosure(totalCntJournal, finishedCountJournal)

		err = snapshots.WriteIndexFile(ctx, dbLock, db, objst, bucket, key, filepath.Base(backupDirPath), snapshotName)
		if err != nil {
			log.Println("error: PlayBackupJournal: writeIndexFileAndWipeJournal: couldn't write index file: ", err)
		}
		vlog.Printf("Deleting all journal rows")
		util.LockIf(dbLock)
		err = db.WipeBackupJournal()
		util.UnlockIf(dbLock)
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

	cp := newChunkPacker(ctx, objst, bucket, db, dbLock, key, vlog, persistMemDbToFile, &totalCntJournal, &finishedCountJournal, stats)

	// Force persist once before the backup starts
	if persistMemDbToFile != nil {
		persistMemDbToFile(nil, true, true)
	}

	n := 0
	for {

		// Sleep this go routine briefly on every iteration of the for loop
		if resourceUtilization == "low" {
			time.Sleep(time.Millisecond * 50)
		}

		// Has cancelation been requested?
		if checkAndHandleCancelationFunc != nil {
			isCanceled := checkAndHandleCancelationFunc(ctx, key, objst, bucket, backupDirPath, snapshotName)
			if isCanceled {
				return true
			}
		}

		// On every iteration, we persist mem db without the force option so it can decide when
		// it's been long enough (time b/t persists is defined in the function)
		if persistMemDbToFile != nil {
			persistMemDbToFile(nil, false, false)
		}

		util.LockIf(dbLock)
		bjt, err := db.ClaimNextBackupJournalTask()
		util.UnlockIf(dbLock)
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

		util.LockIf(dbLock)
		rootDirName, relPath, err := db.GetDirEntPaths(int(bjt.DirEntId))
		util.UnlockIf(dbLock)
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
				completeTask(db, dbLock, bjt, nil, &totalCntJournal, &finishedCountJournal)
				finishTaskImmediately = false
			} else {
				if pendingInChunkPacker {
					finishTaskImmediately = false
				} else {
					crp.ChunkExtents = chunkExtents
					if stats != nil {
						stats.AddBytesFromChunkExtents(chunkExtents)
					}
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
					completeTask(db, dbLock, bjt, nil, &totalCntJournal, &finishedCountJournal)
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
			if err = purgeFromDb(db, dbLock, filepath.Base(backupDirPath), relPath); err != nil {
				log.Printf("error: PlayBackupJournal (Deleted): failed to purge from dirents '%s': %v", relPath, err)
			}
			crp = nil
		} else {
			log.Printf("error: PlayBackupJournal: unrecognized journal type '%v' on '%s'", bjt.ChangeType, relPath)
		}

		if finishTaskImmediately {
			updateLastBackupTime(db, dbLock, bjt.DirEntId)
			isJournalComplete := completeTask(db, dbLock, bjt, crp, &totalCntJournal, &finishedCountJournal)
			if isJournalComplete {
				writeIndexFileAndWipeJournal()
				return
			}
		}

		n += 1
		if (totalCntJournal <= 1000) || (n%1000 == 0) {
			progressUpdateClosure(totalCntJournal, finishedCountJournal)
		}
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

func completeTask(db *database.DB, dbLock *sync.Mutex, bjt *database.BackupJournalTask, crp *snapshots.CloudRelPath, totalCntJournal *int64, finishedCountJournal *int64) (isJournalComplete bool) {
	var err error
	util.LockIf(dbLock)
	if crp == nil {
		err = db.CompleteBackupJournalTask(bjt, nil)
	} else {
		err = db.CompleteBackupJournalTask(bjt, crp.ToJson())
	}
	util.UnlockIf(dbLock)
	if err != nil {
		log.Printf("error: completeTask: db.CompleteBackupJournalTask: %v", err)
	}
	*finishedCountJournal += 1

	// Double check with values from db before concluding that isJournalComplete
	isJournalComplete = false
	if *finishedCountJournal >= *totalCntJournal {
		util.LockIf(dbLock)
		*finishedCountJournal, *totalCntJournal, err = db.GetBackupJournalCounts()
		util.UnlockIf(dbLock)
		if err != nil {
			log.Printf("error: completeTask: db.GetBackupJournalCounts failed: %v", err)
		}

		isJournalComplete = (*finishedCountJournal >= *totalCntJournal)
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

func ReplayBackupJournal(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string, dbLock *sync.Mutex, db *database.DB, vlog *util.VLog, setReplayInitialProgressFunc SetReplayInitialProgressFuncType, checkAndHandleCancelationFunc CheckAndHandleCancelationFuncType, updateProgressFunc UpdateProgressFuncType, resourceUtilization string) util.ReportedEvent {
	// MemDB - see note at top of DoJournaledBackup
	dbMem, memDbLastPersistedToFileUnixtime := initMemDb(dbLock, db)
	persistMemDbToFile := makePersistMemDbToFile(db, dbMem, dbLock, memDbLastPersistedToFileUnixtime, vlog)
	defer func() {
		persistMemDbToFile(nil, true, true)
		util.LockIf(dbLock)
		dbMem.Close()
		util.UnlockIf(dbLock)
	}()

	// Reset all InProgress -> Unstarted
	util.LockIf(dbLock)
	err := dbMem.ResetAllInProgressBackupJournalTasks()
	util.UnlockIf(dbLock)
	if err != nil {
		log.Println("error: ReplayBackupJournal: dbMem.ResetAllInProgressBackupJournalTasks: ", err)
	}

	// Reconstruct backupDirPath, backupDirName and snapshotName from backup_info table
	util.LockIf(dbLock)
	backupDirPath, snapshotUnixtime, err := dbMem.GetJournaledBackupInfo()
	util.UnlockIf(dbLock)
	if err != nil {
		log.Printf("error: ReplayBackupJournal: dbMem.GetJournaledBackupInfo(): %v", err)
	}
	backupDirName := filepath.Base(backupDirPath)
	snapshotName := time.Unix(snapshotUnixtime, 0).UTC().Format("2006-01-02_15.04.05")

	// Set the initial progress where the back up is starting
	if setReplayInitialProgressFunc != nil {
		util.LockIf(dbLock)
		finished, total, err := dbMem.GetBackupJournalCounts()
		util.UnlockIf(dbLock)
		if err != nil {
			log.Printf("error: ReplayBackupJournal: dbMem.GetBackupJournalCounts: %v", err)
		}
		setReplayInitialProgressFunc(finished, total, backupDirName, vlog)
	}

	breakFromLoop := PlayBackupJournal(ctx, key, dbLock, dbMem, backupDirPath, snapshotName, objst, bucket, vlog, checkAndHandleCancelationFunc, updateProgressFunc, persistMemDbToFile, nil, resourceUtilization)

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
			Msg:      "with interruptions",
		}
	}
}
