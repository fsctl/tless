package objstorefs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNextSnapshot(t *testing.T) {
	snapshotKeys := make([]string, 0)
	snapshotKeys = append(snapshotKeys, "2020-01-01_01.02.03")
	snapshotKeys = append(snapshotKeys, "2021-01-01_01.02.03")
	snapshotKeys = append(snapshotKeys, "2022-01-01_01.02.03")
	snapshotKeys = append(snapshotKeys, "2023-01-01_01.02.03")

	nextSnapshot := getNextSnapshot(snapshotKeys, "2020-01-01_01.02.03")
	assert.Equal(t, "2021-01-01_01.02.03", nextSnapshot)

	nextSnapshot = getNextSnapshot(snapshotKeys, "2021-01-01_01.02.03")
	assert.Equal(t, "2022-01-01_01.02.03", nextSnapshot)

	nextSnapshot = getNextSnapshot(snapshotKeys, "2022-01-01_01.02.03")
	assert.Equal(t, "2023-01-01_01.02.03", nextSnapshot)
}

func SetupSnapshots() (snapshots map[string]Snapshot) {
	// 2020:  	 	file1
	//				file2
	//				file3
	//				file4
	//
	// 2021:		##file1
	//				file2.001 changed
	//				file2.002 changed
	//
	// 2022:		file1
	// 				file2.001 changed
	//				file2.002 changed

	file1RelPath2020 := RelPath{
		EncryptedRelPathStripped: "111111",
		DecryptedRelPath:         "file1",
		EncryptedChunkNames:      map[string]int64{"111111": 0},
		IsDeleted:                false,
	}
	file2RelPath2020 := RelPath{
		EncryptedRelPathStripped: "222222",
		DecryptedRelPath:         "file2",
		EncryptedChunkNames:      map[string]int64{"222222": 0},
		IsDeleted:                false,
	}
	file3RelPath2020 := RelPath{
		EncryptedRelPathStripped: "333333",
		DecryptedRelPath:         "file3",
		EncryptedChunkNames:      map[string]int64{"333333": 0},
		IsDeleted:                false,
	}
	file4RelPath2020 := RelPath{
		EncryptedRelPathStripped: "444444",
		DecryptedRelPath:         "file4",
		EncryptedChunkNames:      map[string]int64{"444444.001": 0, "444444.002": 0},
		IsDeleted:                false,
	}
	file1RelPath2021 := RelPath{
		EncryptedRelPathStripped: "111111",
		DecryptedRelPath:         "file1",
		EncryptedChunkNames:      map[string]int64{"##111111": 0},
		IsDeleted:                true,
	}
	file2RelPath2021 := RelPath{
		EncryptedRelPathStripped: "222222",
		DecryptedRelPath:         "file2",
		EncryptedChunkNames:      map[string]int64{"222222.001": 0, "222222.002": 0},
		IsDeleted:                false,
	}
	file1RelPath2022 := RelPath{
		EncryptedRelPathStripped: "111111",
		DecryptedRelPath:         "file1",
		EncryptedChunkNames:      map[string]int64{"##111111": 0},
		IsDeleted:                false,
	}
	file2RelPath2022 := RelPath{
		EncryptedRelPathStripped: "222222",
		DecryptedRelPath:         "file2",
		EncryptedChunkNames:      map[string]int64{"222222.001": 0, "222222.002": 0},
		IsDeleted:                false,
	}
	snapshot2020 := Snapshot{
		RelPaths: map[string]RelPath{"file1": file1RelPath2020, "file2": file2RelPath2020, "file3": file3RelPath2020, "file4": file4RelPath2020},
	}
	snapshot2021 := Snapshot{
		RelPaths: map[string]RelPath{"file1": file1RelPath2021, "file2": file2RelPath2021},
	}
	snapshot2022 := Snapshot{
		RelPaths: map[string]RelPath{"file1": file1RelPath2022, "file2": file2RelPath2022},
	}
	snapshots = map[string]Snapshot{
		"2020-01-01_01.02.03": snapshot2020,
		"2021-01-01_01.02.03": snapshot2021,
		"2022-01-01_01.02.03": snapshot2022,
	}
	return snapshots
}

func TestContainsRelPath(t *testing.T) {
	snapshots := SetupSnapshots()

	result := containsRelPath(snapshots["2020-01-01_01.02.03"], "file1")
	assert.Equal(t, true, result)
	result = containsRelPath(snapshots["2020-01-01_01.02.03"], "file2")
	assert.Equal(t, true, result)
	result = containsRelPath(snapshots["2020-01-01_01.02.03"], "file3")
	assert.Equal(t, false, result)

	result = containsRelPath(snapshots["2021-01-01_01.02.03"], "file1")
	assert.Equal(t, true, result)
	result = containsRelPath(snapshots["2021-01-01_01.02.03"], "file2")
	assert.Equal(t, true, result)
	result = containsRelPath(snapshots["2021-01-01_01.02.03"], "file3")
	assert.Equal(t, false, result)

	result = containsRelPath(snapshots["2021-01-01_01.02.03"], "file1")
	assert.Equal(t, true, result)
	result = containsRelPath(snapshots["2021-01-01_01.02.03"], "file2")
	assert.Equal(t, true, result)
	result = containsRelPath(snapshots["2021-01-01_01.02.03"], "file3")
	assert.Equal(t, false, result)
}

func TestRenameAllChunksIntoNextSnapshot(t *testing.T) {
	snapshots := SetupSnapshots()

	renameObjs := renameAllChunksIntoNextSnapshot(snapshots, "file3", "2020-01-01_01.02.03", "2021-01-01_01.02.03")
	assert.Equal(t, 1, len(renameObjs))
	assert.Equal(t, renameObjs[0].RelPath, "333333")
	assert.Equal(t, renameObjs[0].OldSnapshot, "2020-01-01_01.02.03")
	assert.Equal(t, renameObjs[0].NewSnapshot, "2021-01-01_01.02.03")

	renameObjs = renameAllChunksIntoNextSnapshot(snapshots, "file4", "2020-01-01_01.02.03", "2021-01-01_01.02.03")
	assert.Equal(t, 2, len(renameObjs))
	assert.Equal(t, renameObjs[0].RelPath, "444444.001")
	assert.Equal(t, renameObjs[0].OldSnapshot, "2020-01-01_01.02.03")
	assert.Equal(t, renameObjs[0].NewSnapshot, "2021-01-01_01.02.03")
	assert.Equal(t, renameObjs[1].RelPath, "444444.002")
	assert.Equal(t, renameObjs[1].OldSnapshot, "2020-01-01_01.02.03")
	assert.Equal(t, renameObjs[1].NewSnapshot, "2021-01-01_01.02.03")
}

func TestDeleteAllKeysInSnapshot(t *testing.T) {
	snapshots := SetupSnapshots()

	deleteObjs := deleteAllKeysInSnapshot(snapshots, "2020-01-01_01.02.03")
	assert.Equal(t, 5, len(deleteObjs))
	deleteObjs = deleteAllKeysInSnapshot(snapshots, "2021-01-01_01.02.03")
	assert.Equal(t, 3, len(deleteObjs))
}
