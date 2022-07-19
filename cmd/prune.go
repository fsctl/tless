package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
	"github.com/spf13/cobra"
)

var (
	// Flags
	cfgPruneDryRun bool

	// Command
	pruneCmd = &cobra.Command{
		Use:   "prune",
		Short: "Prunes snapshots from a backup",
		Long: `Prunes snapshots on server by deleting intermediate snapshots that are no longer necessary.
Prune keeps every snapshot from the past 24 hours, the oldest and newest from the past 2-7 days,
the oldest and newest from past 7-30 days and the oldest and newest from the past 2-12 months.

The prune command is specific to a particular backup and will only look at snapshots from that 
backup. In the examples below, it is imagined that you have backups with names like "Documents"
and "home".

Example:

	tless prune Documents
	tless prune home --dry-run

The --dry-run flag will cause prune to simply print what snapshots it would delete and preserve, 
but not do any actual deletion.
`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 1 {
				pruneMain(args[0], cfgPruneDryRun)
			} else {
				fmt.Printf("Please specify one argument (the backup name)")
			}
		},
	}
)

func init() {
	pruneCmd.Flags().BoolVar(&cfgPruneDryRun, "dry-run", false, "list snapshots to be deleted, but don't make changes")
	rootCmd.AddCommand(pruneCmd)
}

func pruneMain(backupName string, isDryRun bool) {
	vlog := util.NewVLog(nil, func() bool { return cfgVerbose })

	// Record peak usage before prune
	persistUsage(nil, true, false, vlog)

	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)
	mSnapshots, err := snapshots.GetAllSnapshotInfos(ctx, encKey, objst, cfgBucket)
	if err != nil {
		fmt.Println("error: prune: ", err)
		return
	}

	if _, ok := mSnapshots[backupName]; !ok {
		fmt.Printf("error: backup name invalid '%s'\n", backupName)
	}

	fmt.Printf("Backup '%s'\n", backupName)

	// Mark what is to be kept
	keeps := snapshots.GetPruneKeepsList(mSnapshots[backupName])

	for _, ss := range mSnapshots[backupName] {
		if isDryRun {
			verb := "DELE"
			for _, k := range keeps {
				if ss == k {
					verb = "KEEP"
					break
				}
			}

			tm := time.Unix(ss.TimestampUnix, 0).UTC()
			formattedTimestamp := tm.Format("Jan 2, 2006 at 3:04pm UTC")

			fmt.Printf("  %s: '%s' from %s\n", verb, ss.Name, formattedTimestamp)
		} else {
			keepCurr := false
			for _, k := range keeps {
				if ss == k {
					keepCurr = true
					break
				}
			}

			if !keepCurr {
				fmt.Printf("  Deleting snapshot '%s'\n", ss.RawSnapshotName)
				ssDel := snapshots.SnapshotForDeletion{
					BackupDirName: backupName,
					SnapshotName:  ss.Name,
				}
				if err = snapshots.DeleteSnapshots(ctx, encKey, []snapshots.SnapshotForDeletion{ssDel}, objst, cfgBucket, vlog, nil, nil); err != nil {
					fmt.Printf("error: could not delete '%s': %v\n", ss.RawSnapshotName, err)
				}
			} else {
				fmt.Printf("  Keeping snapshot '%s'\n", ss.RawSnapshotName)
			}
		}
	}

	persistUsage(nil, true, true, vlog)
}
