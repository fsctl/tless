package cmd

import (
	"context"
	"log"
	"strings"

	"github.com/fsctl/trustlessbak/pkg/backup"
	"github.com/fsctl/trustlessbak/pkg/objstore"
	"github.com/fsctl/trustlessbak/pkg/objstorefs"
	"github.com/spf13/cobra"
)

var (
	restoreCmd = &cobra.Command{
		Use:   "restore [name] [/restore/into/dir]",
		Short: "Restores a snapshot into a specified directory",
		Long: `Restores the named snapshot into the specified path. Usage:

trustlessbak restore [snapshot] [/restore/into/dir]

(Use 'trustlessbak cloudls' to get the names of your stored snapshots.)

Example:

	trustlessbak restore Documents/2020-01-15_04:56:00 /home/myname/Recovered-Documents

The available snapshot times are displayed in 'unbackupcloud cloudls' with no arguments.
`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			restoreMain(args[0], args[1])
		},
	}
)

func init() {
	rootCmd.AddCommand(restoreCmd)
}

func restoreMain(backupAndSnapshotName string, pathToRestoreInto string) {
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)

	backupAndSnapshotNameParts := strings.Split(backupAndSnapshotName, "/")
	if len(backupAndSnapshotNameParts) != 2 {
		log.Fatalf("Malformed snapshot name: '%s'", backupAndSnapshotName)
	}
	backupName := backupAndSnapshotNameParts[0]
	snapshotName := backupAndSnapshotNameParts[1]
	// TODO: validate both of these further to make sure argument is well-formed

	mRelPathsObjsMap, err := objstorefs.ReconstructSnapshotFileList(ctx, objst, cfgBucket, encKey, backupName, snapshotName)
	if err != nil {
		log.Fatalln("error: reconstructSnapshotFileList failed: ", err)
	}
	for relPath := range mRelPathsObjsMap {
		if len(mRelPathsObjsMap[relPath]) > 1 {
			relPathChunks := mRelPathsObjsMap[relPath]

			err = backup.RestoreDirEntryFromChunks(ctx, encKey, pathToRestoreInto, relPathChunks, backupName, snapshotName, relPath, objst, cfgBucket, cfgVerbose)
			if err != nil {
				log.Printf("error: could not restore a dir entry '%s'", relPath)
			}
		} else if len(mRelPathsObjsMap[relPath]) == 1 {
			objName := mRelPathsObjsMap[relPath][0]

			err = backup.RestoreDirEntry(ctx, encKey, pathToRestoreInto, objName, backupName, snapshotName, relPath, objst, cfgBucket, cfgVerbose)
			if err != nil {
				log.Printf("error: could not restore a dir entry '%s'", relPath)
			}
		} else {
			log.Fatalf("error: invalid number of chunks planned for %s", relPath)
		}
	}
}
