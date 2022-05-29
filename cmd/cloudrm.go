package cmd

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/spf13/cobra"
)

var (
	// Flags
	cloudrmCfgSnapshot string

	// Command
	cloudrmCmd = &cobra.Command{
		Use:   "cloudrm",
		Short: "Deletes a snapshot",
		Long: `Deletes a snapshot by merging it with the next snapshot forward in time. This makes it as if
you had never created the snapshot, but the next snapshot forward in time is updated to how it
would have been if you had only made one at that time.

Example:

	tless cloudrm --snapshot=Documents/2020-01-01_04:56:01

The available snapshot times are displayed in 'tless cloudls' with no arguments.
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if cloudrmCfgSnapshot != "" {
				cloudrmMain()
			} else {
				log.Fatalln("error: --snapshot is required")
			}
		},
	}
)

func init() {
	cloudrmCmd.Flags().StringVarP(&cloudrmCfgSnapshot, "snapshot", "S", "", "snapshot to delete (eg, 'Documents/2020-01-01_01:02:03')")
	rootCmd.AddCommand(cloudrmCmd)
}

func cloudrmMain() {
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)

	backupDirName, snapshotTimestamp, err := splitSnapshotName(cloudrmCfgSnapshot)
	if err != nil {
		log.Fatalf("Cannot split '%s' into backupDirName/snapshotTimestamp", cloudrmCfgSnapshot)
	}

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctx, objst, encKey, cfgBucket)
	if err != nil {
		log.Fatalf("Could not get grouped snapshots: %v", err)
	}

	backupDirSnapshotsOnly := groupedObjects[backupDirName].Snapshots

	deleteObjs, renameObjs, err := objstorefs.ComputeSnapshotDelete(backupDirSnapshotsOnly, snapshotTimestamp)
	if err != nil {
		log.Fatalln("error: could not compute plan for deleting snapshot")
	}

	// get the encrypted representation of backupDirName and snapshotName
	encryptedSnapshotName, err := cryptography.EncryptFilename(encKey, snapshotTimestamp)
	if err != nil {
		log.Fatalf("error: cloudrmMain(): could not encrypt snapshot name (%s): %v\n", snapshotTimestamp, err)
	}
	encryptedBackupDirName, err := cryptography.EncryptFilename(encKey, backupDirName)
	if err != nil {
		log.Fatalf("error: cloudrmMain(): could not encrypt backup dir name (%s): %v\n", backupDirName, err)
	}

	// Object deletes
	for _, m := range deleteObjs {
		for encRelPath := range m {
			objName := encryptedBackupDirName + "/" + encryptedSnapshotName + "/" + encRelPath
			//fmt.Printf("  %s\n", objName)
			err = objst.DeleteObj(ctx, cfgBucket, objName)
			if err != nil {
				log.Fatalf("error: cloudrmMain(): could not rename object (%s): %v\n", objName, err)
			}
		}
	}

	// Object renames
	for _, renObj := range renameObjs {
		encryptedOldSnapshotName, err := cryptography.EncryptFilename(encKey, renObj.OldSnapshot)
		if err != nil {
			log.Fatalf("error: cloudrmMain(): could not encrypt snapshot name (%s): %v\n", renObj.OldSnapshot, err)
		}
		encryptedNewSnapshotName, err := cryptography.EncryptFilename(encKey, renObj.NewSnapshot)
		if err != nil {
			log.Fatalf("error: cloudrmMain(): could not encrypt snapshot name (%s): %v\n", renObj.NewSnapshot, err)
		}
		oldObjName := encryptedBackupDirName + "/" + encryptedOldSnapshotName + "/" + renObj.RelPath
		newObjName := encryptedBackupDirName + "/" + encryptedNewSnapshotName + "/" + renObj.RelPath
		//fmt.Printf("'%s' -> '%s'\n", oldObjName, newObjName)
		err = objst.RenameObj(ctx, cfgBucket, oldObjName, newObjName)
		if err != nil {
			log.Fatalf("error: cloudrmMain(): could not rename object (%s): %v\n", oldObjName, err)
		}
	}
}

func splitSnapshotName(snapshotName string) (backupDirName string, snapshotTime string, err error) {
	snapshotNameParts := strings.Split(snapshotName, "/")
	if len(snapshotNameParts) != 2 {
		return "", "", fmt.Errorf("should be slash-splitable into 2 parts")
	}

	backupDirName = snapshotNameParts[0]
	snapshotTime = snapshotNameParts[1]
	return backupDirName, snapshotTime, nil
}
