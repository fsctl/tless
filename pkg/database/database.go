// Package db abstracts operations on local sqlite3 db that stores state
package database

import (
	"database/sql"
	"errors"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	dbConn *sql.DB
}

func NewDB(dbFilePath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbFilePath)
	if err != nil {
		log.Fatal(err)
	}

	return &DB{
		dbConn: db,
	}, nil
}

func (db *DB) Close() {
	db.dbConn.Close()
}

func (db *DB) querySingleRowCount(sql string) (int, error) {
	rows, err := db.dbConn.Query(sql)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var cnt int
	for rows.Next() {
		err = rows.Scan(&cnt)
		if err != nil {
			return 0, err
		} else {
			break
		}
	}
	err = rows.Err()
	if err != nil {
		return 0, err
	}
	return cnt, nil
}

func (db *DB) DropAllTables() error {
	sqlStmt := `
	drop table if exists dirents;
	drop table if exists queue;
	`
	_, err := db.dbConn.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
	}
	return err
}

func (db *DB) CreateTablesIfNotExist() error {
	sql := "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='dirents';"
	cnt, err := db.querySingleRowCount(sql)
	if err != nil {
		log.Printf("%q: %s\n", err, sql)
		return err
	}

	if cnt != 1 {
		err = db.createTables()
		if err != nil {
			log.Printf("Error: could not create tables")
			return err
		}
	}

	return nil
}

func (db *DB) createTables() error {
	sqlStmt := `
	drop table if exists dirents;
	create table dirents (
		id integer not null primary key autoincrement, 
		rootdir text,
		relpath text,
		last_backup integer
	);
	create index idx_rootdir_relpaths ON dirents (rootdir, relpath);
	`
	_, err := db.dbConn.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
	}
	return err
}

// Looks for dirent with specified rootdir and relpath. Returns
// (true,last_backup,id,nil) if found.  Returns (false,0,0,err) if not
// found.
func (db *DB) HasDirEnt(rootDirName string, relPath string) (isFound bool, lastBackupUnix int64, id int, err error) {
	stmt, err := db.dbConn.Prepare("select id, last_backup from dirents where rootdir = ? AND relpath = ?")
	if err != nil {
		log.Printf("Error: HasDirEnt: %v", err)
		return false, 0, 0, err
	}
	defer stmt.Close()
	err = stmt.QueryRow(rootDirName, relPath).Scan(&id, &lastBackupUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, 0, nil
	} else if err != nil {
		log.Printf("Error: HasDirEnt: %v", err)
		return false, 0, 0, err
	} else {
		return true, lastBackupUnix, id, nil
	}
}

type InsertDirEntStmt struct {
	stmt *sql.Stmt
	tx   *sql.Tx
}

func NewInsertDirEntStmt(db *DB) (*InsertDirEntStmt, error) {
	tx, err := db.dbConn.Begin()
	if err != nil {
		log.Printf("Error: NewInsertDirEntStmt: %v", err)
		return nil, err
	}

	stmt, err := tx.Prepare("insert into dirents(rootdir, relpath, last_backup) values (?, ?, ?)")
	if err != nil {
		log.Printf("Error: NewInsertDirEntStmt: %v", err)
		return nil, err
	}

	return &InsertDirEntStmt{stmt: stmt, tx: tx}, nil
}

func (idst *InsertDirEntStmt) Close() {
	idst.tx.Commit()

	idst.stmt.Close()
}

// Inserts a new path into dirent table and returns id of row.
func (idst *InsertDirEntStmt) InsertDirEnt(rootDirName string, relPath string, lastBackupUnix int64) error {
	_, err := idst.stmt.Exec(rootDirName, relPath, lastBackupUnix)
	if err != nil {
		log.Printf("Error: InsertDirEnt: %v", err)
		return err
	}

	return nil
}

// // Inserts a new path into dirent table and returns id of row.
// func (idst *InsertDirEntStmt) InsertDirEnt(rootDirName string, relPath string, lastBackupUnix int64) (int, error) {
// 	result, err := idst.stmt.Exec(rootDirName, relPath, lastBackupUnix)
// 	if err != nil {
// 		log.Printf("Error: InsertDirEnt: %v", err)
// 		return 0, err
// 	}

// 	id, err := result.LastInsertId()
// 	if err != nil {
// 		log.Printf("Error: InsertDirEnt: %v", err)
// 		return 0, err
// 	}

// 	return int(id), nil
// }

func (db *DB) GetAllKnownPaths(rootDirName string) (map[string]int, error) {
	paths := make(map[string]int, 0)

	rows, err := db.dbConn.Query("select id, rootdir, relpath from dirents where rootdir = '" + rootDirName + "'")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var rootdir string
		var relpath string
		err = rows.Scan(&id, &rootdir, &relpath)
		if err != nil {
			log.Fatal(err)
		}
		paths[rootdir+"/"+relpath] = id
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

	return paths, nil
}

func (db *DB) UpdateLastBackupTime(dirEntId int) error {
	stmt, err := db.dbConn.Prepare("update dirents set last_backup = strftime('%s','now') where id = ?")
	if err != nil {
		log.Printf("Error: UpdateLastBackupTime: %v", err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(dirEntId)
	if err != nil {
		log.Printf("Error: UpdateLastBackupTime: %v", err)
		return err
	}

	return nil
}

func (db *DB) getDirEntById(dirEntId int) (id int, rootDirName string, relPath string, lastBackupUnix int64, err error) {
	stmt, err := db.dbConn.Prepare("select id, rootdir, relpath, last_backup from dirents where id = ?")
	if err != nil {
		log.Printf("Error: getDirEntById: %v", err)
		return 0, "", "", 0, err
	}
	defer stmt.Close()

	err = stmt.QueryRow(dirEntId).Scan(&id, &rootDirName, &relPath, &lastBackupUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", "", 0, err
	} else if err != nil {
		log.Printf("error: getDirEntById: %v", err)
		return 0, "", "", 0, err
	}

	return id, rootDirName, relPath, lastBackupUnix, nil
}

func (db *DB) GetDirEntPaths(dirEntId int) (rootDirName string, relPath string, err error) {
	_, rootDirName, relPath, _, err = db.getDirEntById(dirEntId)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("error: no such dirents db row (id='%d')", dirEntId)
		return "", "", err
	} else if err != nil {
		log.Printf("error: GetDirEntRelPath: %v", err)
		return "", "", err
	} else {
		return rootDirName, relPath, nil
	}
}

func (db *DB) DeleteDirEntByPath(rootDirName string, relPath string) error {
	stmt, err := db.dbConn.Prepare("delete from dirents where rootdir = ? AND relpath = ?")
	if err != nil {
		log.Printf("Error: DeleteDirEntByPath: %v", err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(rootDirName, relPath)
	if err != nil {
		log.Printf("Error: DeleteDirEntByPath: %v", err)
		return err
	}
	return nil
}
