package database

import (
	"database/sql"
	"errors"
	"log"

	"github.com/fsctl/tless/pkg/util"
)

var (
	createTableVersion = `
	DROP TABLE IF EXISTS version;
	CREATE TABLE version (
		version INTEGER
	);
	INSERT INTO version (version) VALUES (1);
	`

	createTableUsageHistory = `
	DROP TABLE IF EXISTS space_usage_history;
	CREATE TABLE space_usage_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		date INTEGER,
		space_used INTEGER
	);
	`

	createTableBandwidthHistory = `
	DROP TABLE IF EXISTS bandwidth_usage_history;
	CREATE TABLE bandwidth_usage_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		date INTEGER,
		bandwidth_used INTEGER
	);
	`
)

func (db *DB) PerformDbMigrations(vlog *util.VLog) error {
	dbVersion, err := db.getDbVersion()
	if err != nil {
		log.Println("error: PerformDbMigrations: couldn't get version (treating as -1)", err)
		dbVersion = -1
	}

	switch dbVersion {
	case -1:
		// Fresh db => create everything
		vlog.Println("notice: PerformDbMigrations: at ver -1 (fresh db => creating everything)")
		db.DropAllTables()
		db.CreateTablesIfNotExist()
		// now we're at db version 0
		vlog.Println("notice: PerformDbMigrations: at ver 0 (migrating forward)")
		err = db.migrateToVer1()
		if err != nil {
			log.Println("error: PerformDbMigrations: failed to migrate to v1", err)
			return err
		}
		vlog.Println("notice: PerformDbMigrations: now at ver 1")
	case 0:
		vlog.Println("notice: PerformDbMigrations: at ver 0 (migrating forward)")
		err = db.migrateToVer1()
		if err != nil {
			log.Println("error: PerformDbMigrations: failed to migrate to v1", err)
			return err
		}
		vlog.Println("notice: PerformDbMigrations: now at ver 1")
	case 1:
		// No versions higher than 1 yet
		vlog.Println("notice: PerformDbMigrations: at ver 1 (latest)")
	}

	return nil
}

// Returns -1 for brand new DB with zero tables
// Returns version 0 if no "version" table
// Else returns version in the "version" table
func (db *DB) getDbVersion() (int, error) {
	s := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND (name<>'sqlite_sequence');"
	cnt, err := db.querySingleRowCount(s)
	if err != nil {
		log.Printf("error: getDbVersion: %q: %s\n", err, s)
		return -999, err
	}

	if cnt == 0 {
		return -1, nil
	}

	s = "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND (name='version');"
	cnt, err = db.querySingleRowCount(s)
	if err != nil {
		log.Printf("error: getDbVersion: %q: %s\n", err, s)
		return -999, err
	}

	if cnt == 0 {
		return 0, nil
	}

	stmt, err := db.dbConn.Prepare("SELECT version FROM version LIMIT 1")
	if err != nil {
		log.Println("error: getDbVersion: ", err)
		return -999, err
	}
	defer stmt.Close()
	var version int
	err = stmt.QueryRow().Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	} else if err != nil {
		log.Println("error: getDbVersion: ", err)
		return -999, err
	} else {
		return version, nil
	}
}

func (db *DB) migrateToVer1() error {
	// Create space and bandwidth history tables
	_, err := db.dbConn.Exec(createTableUsageHistory)
	if err != nil {
		log.Printf("error: migrateToVer1: %q\n", err)
		return err
	}
	_, err = db.dbConn.Exec(createTableBandwidthHistory)
	if err != nil {
		log.Printf("error: migrateToVer1: %q\n", err)
		return err
	}

	// Create the version table with initial value of version 1
	_, err = db.dbConn.Exec(createTableVersion)
	if err != nil {
		log.Printf("error: migrateToVer1: %q\n", err)
		return err
	}

	return nil
}
