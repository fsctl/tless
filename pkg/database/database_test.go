package database

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInsertAndGetPaths(t *testing.T) {
	db, err := NewDB("./test-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	dirEntStmt, err := NewInsertDirEntStmt(db)
	assert.NoError(t, err)
	err = dirEntStmt.InsertDirEnt("root", "dir/dir2/file.txt", 0)
	assert.NoError(t, err)
	err = dirEntStmt.InsertDirEnt("root", "dir/file", 0)
	assert.NoError(t, err)
	dirEntStmt.Close()

	paths, err := db.GetAllKnownPaths("root")
	assert.NoError(t, err)
	expectPaths := map[string]int{"root/dir/dir2/file.txt": 1, "root/dir/file": 2}
	assert.Equal(t, expectPaths, paths)

	hasDirEnt, lastBackupUnix, id, err := db.HasDirEnt("root", "dir/dir2/file.txt")
	assert.NoError(t, err)
	assert.Equal(t, true, hasDirEnt)
	assert.Equal(t, int64(0), lastBackupUnix)
	assert.Equal(t, 1, id)

	hasDirEnt, _, _, err = db.HasDirEnt("doesnotexist", "dir/dir2/file.txt")
	assert.NoError(t, err)
	assert.Equal(t, false, hasDirEnt)
}

func TestUpdateLastBackupTime(t *testing.T) {
	db, err := NewDB("./test-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	dirEntStmt, err := NewInsertDirEntStmt(db)
	assert.NoError(t, err)
	err = dirEntStmt.InsertDirEnt("root", "dir/dir2/file.txt", 0)
	assert.NoError(t, err)
	dirEntStmt.Close()

	err = db.UpdateLastBackupTime(1)
	assert.NoError(t, err)

	_, lastBackupUnix, _, err := db.HasDirEnt("root", "dir/dir2/file.txt")
	assert.NoError(t, err)
	assert.NotEqual(t, 0, lastBackupUnix)
}

func TestGetDirEntRelPath(t *testing.T) {
	db, err := NewDB("./test-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	dirEntStmt, err := NewInsertDirEntStmt(db)
	assert.NoError(t, err)
	err = dirEntStmt.InsertDirEnt("root", "dir/dir2/file.txt", 0)
	assert.NoError(t, err)
	dirEntStmt.Close()

	rootDirName, relPath, err := db.GetDirEntPaths(1)
	assert.NoError(t, err)
	assert.Equal(t, "root", rootDirName)
	assert.Equal(t, "dir/dir2/file.txt", relPath)

	_, _, err = db.GetDirEntPaths(1 + 1)
	assert.Error(t, err)
}

func TestResetLastBackedUpTimeForEntireBackup(t *testing.T) {
	db, err := NewDB("./test-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	dirEntStmt, err := NewInsertDirEntStmt(db)
	assert.NoError(t, err)
	err = dirEntStmt.InsertDirEnt("backup1", "dir/file1", time.Now().Unix()) // id = 1
	assert.NoError(t, err)
	err = dirEntStmt.InsertDirEnt("backup2", "dir/file2", time.Now().Unix()) // id = 2
	assert.NoError(t, err)
	err = dirEntStmt.InsertDirEnt("backup2", "dir/file3", time.Now().Unix()) // id = 3
	assert.NoError(t, err)
	dirEntStmt.Close()

	// Verify initial state of the table
	_, rootDirName, _, lastBackupUnix, err := db.getDirEntById(1)
	assert.NoError(t, err)
	assert.Equal(t, "backup1", rootDirName)
	assert.NotEqual(t, int64(0), lastBackupUnix)
	_, rootDirName, _, lastBackupUnix, err = db.getDirEntById(2)
	assert.NoError(t, err)
	assert.Equal(t, "backup2", rootDirName)
	assert.NotEqual(t, int64(0), lastBackupUnix)
	_, rootDirName, _, lastBackupUnix, err = db.getDirEntById(3)
	assert.NoError(t, err)
	assert.Equal(t, "backup2", rootDirName)
	assert.NotEqual(t, int64(0), lastBackupUnix)

	// Try to reset backup2 to zero mtime
	err = db.ResetLastBackedUpTimeForEntireBackup("backup2")
	assert.NoError(t, err)

	// Verify that (only) the backup2 entries are now zero
	_, rootDirName, _, lastBackupUnix, err = db.getDirEntById(1)
	assert.NoError(t, err)
	assert.Equal(t, "backup1", rootDirName)
	assert.NotEqual(t, int64(0), lastBackupUnix)
	_, rootDirName, _, lastBackupUnix, err = db.getDirEntById(2)
	assert.NoError(t, err)
	assert.Equal(t, "backup2", rootDirName)
	assert.Equal(t, int64(0), lastBackupUnix)
	_, rootDirName, _, lastBackupUnix, err = db.getDirEntById(3)
	assert.NoError(t, err)
	assert.Equal(t, "backup2", rootDirName)
	assert.Equal(t, int64(0), lastBackupUnix)
}

func TestBackupJournalFunctions(t *testing.T) {
	db, err := NewDB("./test-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	// Make sure last backup completed is initially zero (i.e., never)
	unixtime, err := db.GetLastCompletedBackupUnixTime()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), unixtime)

	// Test bulk insert txn
	insertBJTxn, err := db.NewInsertBackupJournalStmt("/dir/subdir")
	assert.NoError(t, err)
	for i := 0; i < 32; i++ {
		insertBJTxn.InsertBackupJournalRow(int64(i), Unstarted, Updated)
	}
	insertBJTxn.Close()

	finished, total, err := db.GetBackupJournalCounts()
	assert.NoError(t, err)
	assert.Equal(t, int64(32), total)
	assert.Equal(t, int64(0), finished)

	// Test GetJournaledBackupInfo()
	dirPath, snapshotUnixtime, err := db.GetJournaledBackupInfo()
	assert.NoError(t, err)
	assert.Equal(t, "/dir/subdir", dirPath)
	assert.GreaterOrEqual(t, snapshotUnixtime+5, time.Now().Unix())

	// Verify that HasDirtyBackupJournal is returns true when it sees items in backup_journal
	hasDirty, err := db.HasDirtyBackupJournal()
	assert.NoError(t, err)
	assert.Equal(t, true, hasDirty)

	// Simulate 3 threads all working through queue. Assert that last one marks everything complete
	// and deletes all database rows.
	var lock sync.Mutex
	goRoutinesDone := 0
	fDoTasks := func() {
		for {
			lock.Lock()
			bjt, err := db.ClaimNextBackupJournalTask()
			lock.Unlock()
			if errors.Is(err, ErrNoWork) {
				// exit and let some other go routine that completes the last task clear table
				lock.Lock()
				goRoutinesDone++
				lock.Unlock()
				return
			} else {
				assert.NoError(t, err)
			}

			// do .1 second of work on this task
			time.Sleep(time.Second / 10)

			lock.Lock()
			err = db.CompleteBackupJournalTask(bjt, []byte(""), nil)
			assert.NoError(t, err)
			finished, total, err := db.GetBackupJournalCounts()
			assert.NoError(t, err)
			isJournalComplete := finished >= total
			if isJournalComplete {
				err = db.WipeBackupJournal()
				assert.NoError(t, err)
			}
			finished, total, err = db.GetBackupJournalCounts()
			assert.NoError(t, err)
			if isJournalComplete {
				assert.Equal(t, int64(0), total)
				assert.Equal(t, int64(0), finished)
				goRoutinesDone++
				return
			} else {
				assert.Less(t, finished, total)
			}
			lock.Unlock()
		}
	}
	go fDoTasks()
	go fDoTasks()
	go fDoTasks()

	for goRoutinesDone < 3 {
		time.Sleep(time.Second)
	}

	finished, total, err = db.GetBackupJournalCounts()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, int64(0), finished)

	// Make sure last backup completed is within last 5 seconds
	unixtime, err = db.GetLastCompletedBackupUnixTime()
	assert.NoError(t, err)
	assert.NotEqual(t, int64(0), unixtime)
	assert.GreaterOrEqual(t, unixtime+5, time.Now().Unix())

	// Verify that HasDirtyBackupJournal now returns false since table is empty
	hasDirty, err = db.HasDirtyBackupJournal()
	assert.NoError(t, err)
	assert.Equal(t, false, hasDirty)
}
