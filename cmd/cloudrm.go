package cmd

import (
	"context"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/fsctl/tless/pkg/snapshots"
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

	tless cloudrm --snapshot=Documents/2020-01-01_04.56.01

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
	cloudrmCmd.Flags().StringVarP(&cloudrmCfgSnapshot, "snapshot", "S", "", "snapshot to delete (eg, 'Documents/2020-01-01_01.02.03')")
	rootCmd.AddCommand(cloudrmCmd)
}

func cloudrmMain() {
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)

	backupDirName, snapshotTimestamp, err := snapshots.SplitSnapshotName(cloudrmCfgSnapshot)
	if err != nil {
		log.Fatalf("Cannot split '%s' into backupDirName/snapshotTimestamp", cloudrmCfgSnapshot)
	}

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctx, objst, encKey, cfgBucket)
	if err != nil {
		log.Fatalf("Could not get grouped snapshots: %v", err)
	}

	err = snapshots.DeleteSnapshot(ctx, encKey, groupedObjects, backupDirName, snapshotTimestamp, objst, cfgBucket)
	if err != nil {
		log.Fatalf("Failed to delete snapshot: %v", err)
	}
}
