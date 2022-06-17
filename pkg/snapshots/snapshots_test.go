package snapshots

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func SetupBackups(t *testing.T) map[string]BackupDir {
	// History
	//   2020:  file1
	//   2021:  file1 (deleted)
	//   2022:  file1 (new file)
	file1ChunkExtents2020 := []ChunkExtent{
		{
			ChunkName: "Uzabcdef==",
			Offset:    0,
			Len:       1000,
		},
	}
	file1ChunkExtents2022 := []ChunkExtent{
		{
			ChunkName: "Uzabcdef==",
			Offset:    0,
			Len:       1000,
		},
	}
	file1RelPath2020 := CloudRelPath{
		RelPath:      "file1",
		ChunkExtents: file1ChunkExtents2020,
	}
	file1RelPath2022 := CloudRelPath{
		RelPath:      "file1",
		ChunkExtents: file1ChunkExtents2022,
	}

	ss2020Name := "2020-01-01_01.01.01"
	ss2020Datetime, err := time.Parse("2006-01-02_15.04.05", ss2020Name)
	assert.NoError(t, err)
	ss2020 := Snapshot{
		EncryptedName: "Ubzbase64==",
		DecryptedName: ss2020Name,
		Datetime:      ss2020Datetime,
		RelPaths:      map[string]CloudRelPath{"file1": file1RelPath2020},
	}
	ss2021Name := "2021-01-01_01.01.01"
	ss2021Datetime, err := time.Parse("2006-01-02_15.04.05", ss2021Name)
	assert.NoError(t, err)
	ss2021 := Snapshot{
		EncryptedName: "Ubzbase64a==",
		DecryptedName: ss2021Name,
		Datetime:      ss2021Datetime,
		RelPaths:      map[string]CloudRelPath{},
	}
	ss2022Name := "2022-01-01_01.01.01"
	ss2022Datetime, err := time.Parse("2006-01-02_15.04.05", ss2022Name)
	assert.NoError(t, err)
	ss2022 := Snapshot{
		EncryptedName: "Ubzbase64b==",
		DecryptedName: ss2022Name,
		Datetime:      ss2022Datetime,
		RelPaths:      map[string]CloudRelPath{"file1": file1RelPath2022},
	}
	snaps := map[string]Snapshot{
		ss2020Name: ss2020,
		ss2021Name: ss2021,
		ss2022Name: ss2022,
	}

	bd := BackupDir{
		EncryptedName: "Ubzbase64c==",
		DecryptedName: "backupdir",
		Snapshots:     snaps,
	}

	m := map[string]BackupDir{
		"backupdir": bd,
	}

	return m
}

func TestGetMostRecentSnapshot(t *testing.T) {
	mBackupDirs := SetupBackups(t)

	mostRecentSnapshot := mBackupDirs["backupdir"].GetMostRecentSnapshot()
	assert.Equal(t, "2022-01-01_01.01.01", mostRecentSnapshot.DecryptedName)
}
