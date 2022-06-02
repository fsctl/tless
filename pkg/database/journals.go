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

var (
	// ErrNoWork is returned by ClaimNextBackupJournalTask() when there are no unstarted tasks left
	ErrNoWork = errors.New("queue: no work available")
)

type InsertBackupJournalStmt struct {
	stmt          *sql.Stmt
	tx            *sql.Tx
	backupsInfoId int64
}

func (db *DB) NewInsertBackupJournalStmt() (*InsertBackupJournalStmt, error) {
	// First insert the backup_info row so we have its id
	stmtInfoInsert, err := db.dbConn.Prepare("INSERT INTO backup_info (snapshot_time) VALUES (strftime('%s','now'))")
	if err != nil {
		log.Printf("Error: NewInsertBackupJournalStmt: %v", err)
		return nil, err
	}
	defer stmtInfoInsert.Close()

	result, err := stmtInfoInsert.Exec()
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

	stmt, err := tx.Prepare("INSERT INTO backup_journal (backup_info_id, dirent_id, status) values (?, ?, ?)")
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
func (ibst *InsertBackupJournalStmt) InsertBackupJournalRow(dirEntId int64, status JournalStatus) error {
	_, err := ibst.stmt.Exec(ibst.backupsInfoId, dirEntId, status)
	if err != nil {
		log.Printf("Error: InsertBackupJournalRow: %v", err)
		return err
	}

	return nil
}

type BackupJournalTask struct {
	id       int64
	DirEntId int64
}

func (db *DB) ClaimNextBackupJournalTask() (backupJournalTask *BackupJournalTask, err error) {
	for {
		id, dirEntId, err := db.selectNextBackupJournalCandidateTask()
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
					id:       id,
					DirEntId: dirEntId,
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

func (db *DB) selectNextBackupJournalCandidateTask() (id int64, dirEntId int64, err error) {
	stmt, err := db.dbConn.Prepare("SELECT id, dirent_id FROM backup_journal WHERE status = ? LIMIT 1")
	if err != nil {
		log.Printf("Error: selectNextBackupJournalCandidateTask: %v", err)
		return 0, 0, err
	}
	defer stmt.Close()

	err = stmt.QueryRow(Unstarted).Scan(&id, &dirEntId)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	} else if err != nil {
		log.Printf("Error: selectNextBackupJournalCandidateTask: %v", err)
		return 0, 0, err
	}

	return id, dirEntId, nil
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
