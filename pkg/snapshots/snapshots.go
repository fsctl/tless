package snapshots

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/fsctl/tless/pkg/util"
)

func DeleteSnapshot(ctx context.Context, key []byte, groupedObjects map[string]objstorefs.BackupDir, backupDirName string, snapshotTimestamp string, objst *objstore.ObjStore, bucket string) error {
	backupDirSnapshotsOnly := groupedObjects[backupDirName].Snapshots

	// get the encrypted representation of backupDirName and snapshotName
	encryptedSnapshotName, err := cryptography.EncryptFilename(key, snapshotTimestamp)
	if err != nil {
		return fmt.Errorf("error: DeleteSnapshot: could not encrypt snapshot name (%s): %v", snapshotTimestamp, err)
	}
	encryptedBackupDirName, err := cryptography.EncryptFilename(key, backupDirName)
	if err != nil {
		return fmt.Errorf("error: DeleteSnapshot: could not encrypt backup dir name (%s): %v", backupDirName, err)
	}

	// Compute the plan for deleting this snapshot and merging forward
	deleteObjs, renameObjs, newNextSnapshotObj, err := objstorefs.ComputeSnapshotDelete(key, encryptedBackupDirName, backupDirSnapshotsOnly, snapshotTimestamp, objst, ctx, bucket)
	if err != nil {
		return fmt.Errorf("error: DeleteSnapshot: could not compute plan for deleting snapshot: %v", err)
	}

	// Object deletes
	for _, m := range deleteObjs {
		for encRelPath := range m {
			objName := encryptedBackupDirName + "/" + encryptedSnapshotName + "/" + encRelPath

			err = objst.DeleteObj(ctx, bucket, objName)
			if err != nil {
				return fmt.Errorf("error: DeleteSnapshot: could not delete object (%s): %v", objName, err)
			}
		}
	}

	// Object renames
	for _, renObj := range renameObjs {
		encryptedOldSnapshotName, err := cryptography.EncryptFilename(key, renObj.OldSnapshot)
		if err != nil {
			log.Fatalf("error: DeleteSnapshot: could not encrypt snapshot name (%s): %v\n", renObj.OldSnapshot, err)
		}
		encryptedNewSnapshotName, err := cryptography.EncryptFilename(key, renObj.NewSnapshot)
		if err != nil {
			log.Fatalf("error: DeleteSnapshot: could not encrypt snapshot name (%s): %v\n", renObj.NewSnapshot, err)
		}
		oldObjName := encryptedBackupDirName + "/" + encryptedOldSnapshotName + "/" + renObj.RelPath
		newObjName := encryptedBackupDirName + "/" + encryptedNewSnapshotName + "/" + renObj.RelPath

		err = objst.RenameObj(ctx, bucket, oldObjName, newObjName)
		if err != nil {
			log.Fatalf("error: DeleteSnapshot: could not rename object (%s): %v\n", oldObjName, err)
		}
	}

	// Overwrite the new snapshot index for NEXT snapshot (one we merged into)
	if newNextSnapshotObj != nil {
		if err = SerializeAndWriteSnapshotObj(newNextSnapshotObj, key, encryptedBackupDirName, newNextSnapshotObj.EncryptedName, objst, ctx, bucket); err != nil {
			log.Printf("error: DeleteSnapshot: failed in SerializeAndSaveSnapshotObj: %v\n", err)
			return err
		}
	}

	// Delete index file for deleted snapshot
	indexObjName := encryptedBackupDirName + "/@" + encryptedSnapshotName
	err = objst.DeleteObj(ctx, bucket, indexObjName)
	if err != nil {
		return fmt.Errorf("error: DeleteSnapshot: could not delete old snapshot's index file (%s): %v", indexObjName, err)
	}

	return nil
}

func SplitSnapshotName(snapshotName string) (backupDirName string, snapshotTime string, err error) {
	snapshotNameParts := strings.Split(snapshotName, "/")
	if len(snapshotNameParts) != 2 {
		return "", "", fmt.Errorf("should be slash-splitable into 2 parts")
	}

	backupDirName = snapshotNameParts[0]
	snapshotTime = snapshotNameParts[1]
	return backupDirName, snapshotTime, nil
}

// Returns true if the specified backupName+snapshotName is the most recent snapshot existing for backup backupName
func IsMostRecentSnapshotForBackup(ctx context.Context, objst *objstore.ObjStore, bucket string, groupedObjects map[string]objstorefs.BackupDir, backupDirName string, snapshotTimestamp string) bool {
	backupDirSnapshotsOnly := groupedObjects[backupDirName].Snapshots

	// Get an ordered list of all snapshots from earliest to most recent
	snapshotKeys := make([]string, 0, len(backupDirSnapshotsOnly))
	for k := range backupDirSnapshotsOnly {
		snapshotKeys = append(snapshotKeys, k)
	}
	sort.Strings(snapshotKeys)

	mostRecentSnapshotName := snapshotKeys[len(snapshotKeys)-1]

	return snapshotTimestamp == mostRecentSnapshotName
}

type SnapshotInfo struct {
	Name            string
	RawSnapshotName string
	TimestampUnix   int64
}

// Returns a map of backup:[]SnapshotInfo, where the snapshot info structs are sorted by timestamp ascending
// Used by prune (cmd/prune.go) and autoprune (daemon/timer.go)
func GetAllSnapshotInfos(ctx context.Context, key []byte, objst *objstore.ObjStore, bucket string) (map[string][]SnapshotInfo, error) {
	// Get the backup:snapshots map with encrypted names
	encryptedSnapshotsMap, err := objst.GetObjListTopTwoLevels(ctx, bucket, []string{"SALT-", "VERSION"}, []string{"@"})
	if err != nil {
		log.Println("error: GetAllSnapshotInfos: ", err)
		return nil, err
	}

	// Loop over decrypting all names
	mRet := make(map[string][]SnapshotInfo)
	for encBackupName := range encryptedSnapshotsMap {
		backupName, err := cryptography.DecryptFilename(key, encBackupName)
		if err != nil {
			log.Println("error: GetAllSnapshotInfos: DecryptFilename: ", err)
			return nil, err
		}
		mRet[backupName] = make([]SnapshotInfo, 0)

		for _, encSnapshotName := range encryptedSnapshotsMap[encBackupName] {
			snapshotName, err := cryptography.DecryptFilename(key, encSnapshotName)
			if err != nil {
				log.Println("error: GetAllSnapshotInfos: DecryptFilename: ", err)
				return nil, err
			}
			mRet[backupName] = append(mRet[backupName], SnapshotInfo{
				Name:            snapshotName,
				RawSnapshotName: backupName + "/" + snapshotName,
				TimestampUnix:   util.GetUnixTimeFromSnapshotName(snapshotName),
			})
		}
		sort.Slice(mRet[backupName], func(i, j int) bool {
			return mRet[backupName][i].TimestampUnix < mRet[backupName][j].TimestampUnix
		})
	}
	return mRet, nil
}
