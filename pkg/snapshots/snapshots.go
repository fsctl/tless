package snapshots

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
)

func DeleteSnapshot(ctx context.Context, key []byte, groupedObjects map[string]objstorefs.BackupDir, backupDirName string, snapshotTimestamp string, objst *objstore.ObjStore, bucket string) error {
	backupDirSnapshotsOnly := groupedObjects[backupDirName].Snapshots

	deleteObjs, renameObjs, err := objstorefs.ComputeSnapshotDelete(backupDirSnapshotsOnly, snapshotTimestamp)
	if err != nil {
		return fmt.Errorf("error: DeleteSnapshot(): could not compute plan for deleting snapshot")
	}

	// get the encrypted representation of backupDirName and snapshotName
	encryptedSnapshotName, err := cryptography.EncryptFilename(key, snapshotTimestamp)
	if err != nil {
		return fmt.Errorf("error: DeleteSnapshot(): could not encrypt snapshot name (%s): %v", snapshotTimestamp, err)
	}
	encryptedBackupDirName, err := cryptography.EncryptFilename(key, backupDirName)
	if err != nil {
		return fmt.Errorf("error: DeleteSnapshot(): could not encrypt backup dir name (%s): %v", backupDirName, err)
	}

	// Object deletes
	for _, m := range deleteObjs {
		for encRelPath := range m {
			objName := encryptedBackupDirName + "/" + encryptedSnapshotName + "/" + encRelPath

			err = objst.DeleteObj(ctx, bucket, objName)
			if err != nil {
				return fmt.Errorf("error: DeleteSnapshot(): could not rename object (%s): %v", objName, err)
			}
		}
	}

	// Object renames
	for _, renObj := range renameObjs {
		encryptedOldSnapshotName, err := cryptography.EncryptFilename(key, renObj.OldSnapshot)
		if err != nil {
			log.Fatalf("error: cloudrmMain(): could not encrypt snapshot name (%s): %v\n", renObj.OldSnapshot, err)
		}
		encryptedNewSnapshotName, err := cryptography.EncryptFilename(key, renObj.NewSnapshot)
		if err != nil {
			log.Fatalf("error: cloudrmMain(): could not encrypt snapshot name (%s): %v\n", renObj.NewSnapshot, err)
		}
		oldObjName := encryptedBackupDirName + "/" + encryptedOldSnapshotName + "/" + renObj.RelPath
		newObjName := encryptedBackupDirName + "/" + encryptedNewSnapshotName + "/" + renObj.RelPath

		err = objst.RenameObj(ctx, bucket, oldObjName, newObjName)
		if err != nil {
			log.Fatalf("error: cloudrmMain(): could not rename object (%s): %v\n", oldObjName, err)
		}
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
