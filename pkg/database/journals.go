package database

import (
	"database/sql"
	"errors"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

type JournalStatus int

const (
	Unstarted  JournalStatus = 1
	InProgress JournalStatus = 2
	Finished   JournalStatus = 3
)

// The type of change the rel path underwent that we are now going to backup (updated file or deleted file)
type ChangeType int

const (
	Updated ChangeType = 1
	Deleted ChangeType = 2
)

var (
	// ErrNoWork is returned by ClaimNextBackupJournalTask() when there are no unstarted tasks left
	ErrNoWork = errors.New("queue: no work available")
)

type InsertBackupJournalStmt struct {
	stmt          *sql.Stmt
	tx            *sql.Tx
	backupsInfoId int64
}

func (db *DB) NewInsertBackupJournalStmt(backupDirPath string) (*InsertBackupJournalStmt, error) {
	// First insert the backup_info row so we have its id
	stmtInfoInsert, err := db.dbConn.Prepare("INSERT INTO backup_info (dirpath, snapshot_time) VALUES (?, strftime('%s','now'))")
	if err != nil {
		log.Printf("Error: NewInsertBackupJournalStmt: %v", err)
		return nil, err
	}
	defer stmtInfoInsert.Close()

	result, err := stmtInfoInsert.Exec(backupDirPath)
	if err != nil {
		log.Printf("Error: NewInsertBackupJournalStmt: %v", err)
		return nil, err
	}

	backupsInfoId, err := result.LastInsertId()
	if err != nil {
		log.Printf("Error: NewInsertBackupJournalStmt: %v", err)
		return nil, err
	}

	// Now begin the transaction to insert all the other rows
	tx, err := db.dbConn.Begin()
	if err != nil {
		log.Printf("Error: NewInsertBackupJournalStmt: %v", err)
		return nil, err
	}

	stmt, err := tx.Prepare("INSERT INTO backup_journal (backup_info_id, dirent_id, status, change_type) values (?, ?, ?, ?)")
	if err != nil {
		log.Printf("Error: NewInsertBackupJournalStmt: %v", err)
		return nil, err
	}

	return &InsertBackupJournalStmt{stmt: stmt, tx: tx, backupsInfoId: backupsInfoId}, nil
}

func (ibst *InsertBackupJournalStmt) Close() {
	ibst.tx.Commit()

	ibst.stmt.Close()
}

// Inserts a single backup_journal row as part of larger transaction
func (ibst *InsertBackupJournalStmt) InsertBackupJournalRow(dirEntId int64, status JournalStatus, changeType ChangeType) error {
	_, err := ibst.stmt.Exec(ibst.backupsInfoId, dirEntId, status, changeType)
	if err != nil {
		log.Printf("Error: InsertBackupJournalRow: %v", err)
		return err
	}

	return nil
}

type BackupJournalTask struct {
	id         int64
	DirEntId   int64
	ChangeType ChangeType
}

func (db *DB) ClaimNextBackupJournalTask() (backupJournalTask *BackupJournalTask, err error) {
	for {
		id, dirEntId, changeType, err := db.selectNextBackupJournalCandidateTask()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoWork
		} else if err != nil {
			log.Printf("Error: ClaimNextBackupJournalTask(): %v", err)
			return nil, err
		}

		stmt, err := db.dbConn.Prepare("UPDATE backup_journal SET status = ? WHERE id = ?")
		if err != nil {
			log.Printf("Error: ClaimNextBackupJournalTask: %v", err)
			return nil, err
		}
		defer stmt.Close()

		result, err := stmt.Exec(InProgress, id)
		if err != nil {
			log.Printf("Error: ClaimNextBackupJournalTask: %v", err)
			return nil, err
		}
		if rowsAffected, err := result.RowsAffected(); err == nil {
			if rowsAffected == 1 {
				return &BackupJournalTask{
					id:         id,
					DirEntId:   dirEntId,
					ChangeType: changeType,
				}, nil
			} else {
				continue
			}
		} else {
			log.Printf("Error: ClaimNextBackupJournalTask: %v", err)
			return nil, err
		}
	}
}

func (db *DB) selectNextBackupJournalCandidateTask() (id int64, dirEntId int64, changeType ChangeType, err error) {
	stmt, err := db.dbConn.Prepare("SELECT id, dirent_id, change_type FROM backup_journal WHERE status = ? LIMIT 1")
	if err != nil {
		log.Printf("Error: selectNextBackupJournalCandidateTask: %v", err)
		return 0, 0, 0, err
	}
	defer stmt.Close()

	err = stmt.QueryRow(Unstarted).Scan(&id, &dirEntId, &changeType)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, err
	} else if err != nil {
		log.Printf("Error: selectNextBackupJournalCandidateTask: %v", err)
		return 0, 0, 0, err
	}

	return id, dirEntId, changeType, nil
}

// Marks backupJournalTask as Finished.  If this was the last task that was not yet complete,
// deletes all rows from backup_journal and returns true for isJournalComplete.
func (db *DB) CompleteBackupJournalTask(backupJournalTask *BackupJournalTask) (isJournalComplete bool, err error) {
	// Mark this task as done
	stmt, err := db.dbConn.Prepare("UPDATE backup_journal SET status = ? WHERE id = ?")
	if err != nil {
		log.Printf("Error: CompleteBackupJournalTask: %v", err)
		return false, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(Finished, backupJournalTask.id)
	if err != nil {
		log.Printf("Error: CompleteBackupJournalTask: %v", err)
		return false, err
	}

	// Get count of finished and total. If all tasks are finished, delete all rows and return isJournalComplete==true
	totalCount, err := db.getCountTotalBackupJournal()
	if err != nil {
		return false, err
	}
	finishedCount, err := db.getCountFinishedBackupJournal()
	if err != nil {
		return false, err
	}
	if finishedCount >= totalCount {
		if err = db.deleteAllRowsBackupJournal(); err != nil {
			log.Printf("Error: CompleteBackupJournalTask: %v", err)
			return true, err
		} else {
			return true, nil
		}
	}
	return false, nil
}

// Call this function when finishing an incomplete backup journal on startup. It rolls all the
// InProgress tasks back to Unstarted.
func (db *DB) ResetAllInProgressBackupJournalTasks() error {
	stmt, err := db.dbConn.Prepare("UPDATE backup_journal SET status = ? WHERE status = ?")
	if err != nil {
		log.Printf("Error: ResetAllInProgressBackupJournalTasks: %v", err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(Unstarted, InProgress)
	if err != nil {
		log.Printf("Error: ResetAllInProgressBackupJournalTasks: %v", err)
		return err
	}

	return nil
}

// For setting initial progress bar values when resuming an incomplete backup journal
func (db *DB) GetBackupJournalCounts() (finishedCount int64, totalCount int64, err error) {
	totalCount, err = db.getCountTotalBackupJournal()
	if err != nil {
		return 0, 0, err
	}
	finishedCount, err = db.getCountFinishedBackupJournal()
	if err != nil {
		return 0, 0, err
	}
	return finishedCount, totalCount, nil
}

func (db *DB) getCountFinishedBackupJournal() (count int64, err error) {
	stmt, err := db.dbConn.Prepare("SELECT COUNT(*) FROM backup_journal WHERE status = ?")
	if err != nil {
		log.Printf("Error: getCountFinishedBackupJournal: %v", err)
		return 0, err
	}
	defer stmt.Close()

	err = stmt.QueryRow(Finished).Scan(&count)
	if err != nil {
		log.Printf("Error: getCountFinishedBackupJournal: %v", err)
		return 0, err
	}

	return count, nil
}

func (db *DB) getCountTotalBackupJournal() (count int64, err error) {
	stmt, err := db.dbConn.Prepare("SELECT COUNT(*) FROM backup_journal")
	if err != nil {
		log.Printf("Error: getCountTotalBackupJournal: %v", err)
		return 0, err
	}
	defer stmt.Close()

	err = stmt.QueryRow().Scan(&count)
	if err != nil {
		log.Printf("Error: getCountTotalBackupJournal: %v", err)
		return 0, err
	}

	return count, nil
}

func (db *DB) deleteAllRowsBackupJournal() error {
	stmt, err := db.dbConn.Prepare("DELETE FROM backup_journal")
	if err != nil {
		log.Printf("Error: deleteAllRowsBackupJournal: %v", err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec()
	if err != nil {
		log.Printf("Error: deleteAllRowsBackupJournal: %v", err)
		return err
	}

	return nil
}

func (db *DB) GetLastCompletedBackupUnixTime() (unixtime int64, err error) {
	count, err := db.getCountTotalBackupJournal()
	if err != nil {
		log.Printf("Error: GetLastCompletedBackupUnixTime: %v", err)
		return 0, err
	}

	var stmt *sql.Stmt
	if count > 0 {
		stmt, err = db.dbConn.Prepare(`SELECT MAX(backup_info.snapshot_time) FROM backup_info 
			WHERE backup_info.id 
			NOT IN
			(SELECT backup_journal.backup_info_id FROM backup_journal);`)
	} else {
		stmt, err = db.dbConn.Prepare(`SELECT MAX(backup_info.snapshot_time) FROM backup_info`)
	}
	if err != nil {
		log.Printf("Error: GetLastCompletedBackupUnixTime: %v", err)
		return 0, err
	}
	defer stmt.Close()

	err = stmt.QueryRow().Scan(&unixtime)
	if err != nil {
		// This error is probably because true result is null (no previous backup) but go's
		// sql package will not automatically convert that to int64(0)
		return 0, nil
	} else {
		return unixtime, nil
	}
}

func (db *DB) HasDirtyBackupJournal() (bool, error) {
	stmt, err := db.dbConn.Prepare(`SELECT COUNT(*) FROM backup_journal;`)
	if err != nil {
		log.Printf("Error: HasDirtyBackupJournal: %v", err)
		return false, err
	}
	defer stmt.Close()

	var count int64 = 0
	err = stmt.QueryRow().Scan(&count)
	if err != nil {
		log.Printf("Error: HasDirtyBackupJournal: %v", err)
		return false, err
	}

	return count > 0, nil
}

func (db *DB) GetJournaledBackupInfo() (backupDirPath string, snapshotUnixTime int64, err error) {
	stmt, err := db.dbConn.Prepare(`SELECT dirpath, snapshot_time FROM backup_info WHERE backup_info.id IN (SELECT backup_journal.backup_info_id FROM backup_journal);`)
	if err != nil {
		log.Printf("Error: GetJournaledBackupInfo: %v", err)
		return "", 0, err
	}
	defer stmt.Close()

	err = stmt.QueryRow().Scan(&backupDirPath, &snapshotUnixTime)
	if errors.Is(err, sql.ErrNoRows) {
		// Sometimes we get called when backup_journal is empty, so just suppress any error message
		// and return the error for caller to handle
		return "", 0, err
	} else if err != nil {
		log.Printf("Error: GetJournaledBackupInfo: %v", err)
		return "", 0, err
	}

	return backupDirPath, snapshotUnixTime, nil
}

// Resets the dirents.last_backup time stamps to 0 for all InProgress and Finished items in journal
func (db *DB) CancelationResetLastBackupTime() error {
	stmt, err := db.dbConn.Prepare("UPDATE dirents SET last_backup=0 WHERE id in (SELECT dirent_id FROM backup_journal WHERE status = ? OR status = ?)")
	if err != nil {
		log.Printf("Error: CancelationResetLastBackupTime: %v", err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(InProgress, Finished)
	if err != nil {
		log.Printf("Error: CancelationResetLastBackupTime: %v", err)
		return err
	}

	return nil
}

// Wipes out the journal and removes the backup row from backup_info so it doesn't get
// remembered as a successful backup
func (db *DB) CancelationCleanupJournal() error {
	// Delete backup_info row so this doesn't look like a completed backup
	stmt, err := db.dbConn.Prepare("DELETE FROM backup_info WHERE id in (SELECT DISTINCT(backup_info_id) FROM backup_journal)")
	if err != nil {
		log.Printf("Error: CancelationResetLastBackupTime: %v", err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec()
	if err != nil {
		log.Printf("Error: CancelationResetLastBackupTime: %v", err)
		return err
	}

	// Delete all items in journal
	err = db.deleteAllRowsBackupJournal()
	if err != nil {
		log.Printf("Error: CancelationResetLastBackupTime: %v", err)
		return err
	}

	return nil
}
