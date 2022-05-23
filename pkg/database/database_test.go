package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsertAndGetPaths(t *testing.T) {
	db, err := NewDB("./trustlessbak-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	dirEntStmt, err := NewInsertDirEntStmt(db)
	assert.NoError(t, err)
	id, err := dirEntStmt.InsertDirEnt("root", "dir/dir2/file.txt", 0)
	assert.NoError(t, err)
	assert.Equal(t, id, 1)
	id, err = dirEntStmt.InsertDirEnt("root", "dir/file", 0)
	assert.NoError(t, err)
	assert.Equal(t, id, 2)
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
	db, err := NewDB("./trustlessbak-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	dirEntStmt, err := NewInsertDirEntStmt(db)
	assert.NoError(t, err)
	id, err := dirEntStmt.InsertDirEnt("root", "dir/dir2/file.txt", 0)
	assert.NoError(t, err)
	dirEntStmt.Close()

	err = db.UpdateLastBackupTime(id)
	assert.NoError(t, err)

	_, lastBackupUnix, _, err := db.HasDirEnt("root", "dir/dir2/file.txt")
	assert.NoError(t, err)
	assert.NotEqual(t, 0, lastBackupUnix)
}

func TestGetDirEntRelPath(t *testing.T) {
	db, err := NewDB("./trustlessbak-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	dirEntStmt, err := NewInsertDirEntStmt(db)
	assert.NoError(t, err)
	insertId, err := dirEntStmt.InsertDirEnt("root", "dir/dir2/file.txt", 0)
	assert.NoError(t, err)
	dirEntStmt.Close()

	rootDirName, relPath, err := db.GetDirEntPaths(insertId)
	assert.NoError(t, err)
	assert.Equal(t, "root", rootDirName)
	assert.Equal(t, "dir/dir2/file.txt", relPath)

	_, _, err = db.GetDirEntPaths(insertId + 1)
	assert.Error(t, err)
}
