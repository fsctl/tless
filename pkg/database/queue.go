package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"
)

type QueueState int64

const (
	Unstarted QueueState = iota
	InProgress
	// FailedRetryable  // just an idea, not implemented yet
)

const (
	QueueActionBackup string = "backup"
	QueueActionDelete string = "delete"
)

var (
	// ErrNoWork is returned by DequeueNextItem() when there is no startable
	// work in the queue, meaning that either the queue is empty or every item
	// in it is in progress or failed and waiting to retry
	ErrNoWork = errors.New("queue: no work available")
)

// Generic ExecNoResult functions
func dbExecNoResultOneParam[P any](db *DB, sqlOneParam string, param P) error {
	stmt, err := db.dbConn.Prepare(sqlOneParam)
	if err != nil {
		log.Printf("Error: dbExecNoResultOneParam (%s): %v", sqlOneParam, err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(param)
	if err != nil {
		log.Printf("Error: dbExecNoResultOneParam (%s): %v", sqlOneParam, err)
		return err
	}

	return nil
}

func dbExecNoResultTwoParams[P1 any, P2 any](db *DB, sqlTwoParams string, param1 P1, param2 P2) error {
	stmt, err := db.dbConn.Prepare(sqlTwoParams)
	if err != nil {
		log.Printf("Error: ExecNoResultTwoParams (%s): %v", sqlTwoParams, err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(param1, param2)
	if err != nil {
		log.Printf("Error: ExecNoResultTwoParams (%s): %v", sqlTwoParams, err)
		return err
	}

	return nil
}

func dbExecNoResultFourParams[P1 any, P2 any, P3 any, P4 any](db *DB, sqlFourParams string, param1 P1, param2 P2, param3 P3, param4 P4) error {
	stmt, err := db.dbConn.Prepare(sqlFourParams)
	if err != nil {
		log.Printf("Error: dbExecNoResultFourParams (%s): %v", sqlFourParams, err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(param1, param2, param3, param4)
	if err != nil {
		log.Printf("Error: dbExecNoResultFourParams (%s): %v", sqlFourParams, err)
		return err
	}

	return nil
}

// Queue functions
func (db *DB) EnqueueBackupItem(direntsId int) error {
	return dbExecNoResultFourParams(db, "insert into queue (action, arg1, status, last_updated) values (?, ?, ?, ?)", QueueActionBackup, fmt.Sprintf("%d", direntsId), Unstarted, time.Now().Unix())
}

func (db *DB) EnqueueDeleteItem(path string) error {
	return dbExecNoResultFourParams(db, "insert into queue (action, arg1, status, last_updated) values (?, ?, ?, ?)", QueueActionDelete, path, Unstarted, time.Now().Unix())
}

type QueueItemDescription struct {
	QueueId int64
	Action  string
	Arg1    string
}

func (db *DB) DequeueNextItem() (*QueueItemDescription, error) {
	for {
		id, action, arg1, err := db.selectNextItemCandidate()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoWork
		} else if err != nil {
			log.Printf("Error: DequeueNextItem(): %v", err)
			return nil, err
		}

		stmt, err := db.dbConn.Prepare("update queue set status = ?, last_updated = strftime('%s','now') where id = ?")
		if err != nil {
			log.Printf("Error: DequeueNextItem: %v", err)
			return nil, err
		}
		defer stmt.Close()

		result, err := stmt.Exec(InProgress, id)
		if err != nil {
			log.Printf("Error: DequeueNextItem: %v", err)
			return nil, err
		}
		if rowsAffected, err := result.RowsAffected(); err == nil {
			if rowsAffected == 1 {
				return &QueueItemDescription{
					QueueId: id,
					Action:  action,
					Arg1:    arg1,
				}, nil
			} else {
				continue
			}
		} else {
			log.Printf("Error: DequeueNextItem: %v", err)
			return nil, err
		}
	}
}

func (db *DB) selectNextItemCandidate() (id int64, action string, arg1 string, err error) {
	stmt, err := db.dbConn.Prepare("select id, action, arg1 from queue where status = ? limit 1")
	if err != nil {
		log.Printf("Error: selectNextItemCandidate: %v", err)
		return 0, "", "", err
	}
	defer stmt.Close()

	err = stmt.QueryRow(Unstarted).Scan(&id, &action, &arg1)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", "", err
	} else if err != nil {
		log.Printf("Error: selectNextItemCandidate: %v", err)
		return 0, "", "", err
	}

	return id, action, arg1, nil
}

func (db *DB) CompleteQueueItem(queueId int64) error {
	return dbExecNoResultOneParam(db, "delete from queue where id = ?", queueId)
}

func (db *DB) ReEnqueuQueueItem(queueId int64) error {
	return dbExecNoResultTwoParams(db, "update queue set status = ? where id = ?", Unstarted, queueId)
}
