package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

var (
	// Flags
	cloudrmCfgSnapshot []string

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
			if len(cloudrmCfgSnapshot) != 0 {
				cloudrmMain()
			} else {
				log.Fatalln("error: --snapshot is required")
			}
		},
	}
)

func init() {
	cloudrmCmd.Flags().StringArrayVarP(&cloudrmCfgSnapshot, "snapshot", "S", []string{}, "snapshot to delete (eg, 'Documents/2020-01-01_01.02.03')")
	rootCmd.AddCommand(cloudrmCmd)
}

func cloudrmMain() {
	vlog := util.NewVLog(nil, func() bool { return cfgVerbose })

	// Record peak usage before delete
	persistUsage(nil, true, false, vlog)

	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)

	ssDeletes := make([]snapshots.SnapshotForDeletion, 0)
	for _, snapshotRawName := range cloudrmCfgSnapshot {
		backupDirName, snapshotTimestamp, err := util.SplitSnapshotName(snapshotRawName)
		if err != nil {
			log.Fatalf("Cannot split '%s' into backupDirName/snapshotTimestamp", cloudrmCfgSnapshot)
		}

		ssDeletes = append(ssDeletes, snapshots.SnapshotForDeletion{
			BackupDirName: backupDirName,
			SnapshotName:  snapshotTimestamp,
		})
	}

	// initialize progress bar container and its callbacks
	progressBarContainer := mpb.New()
	var progressBar *mpb.Bar = nil
	setGGSInitialProgress := func(finished int64, total int64) {
		if !cfgVerbose {
			name := "Garbage collecting orphaned chunks"
			progressBar = progressBarContainer.New(
				total,
				mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Rbound("]"),
				mpb.PrependDecorators(
					decor.Name(name, decor.WC{W: len(name) + 1, C: decor.DidentRight}),
					// replace ETA decorator with "done" message on OnComplete event
					decor.OnComplete(
						decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 4}), "done",
					),
				),
				mpb.AppendDecorators(decor.Percentage()),
			)
		}
	}
	updateGGSProgress := func(finished int64, total int64) {
		if !cfgVerbose {
			progressBar.SetCurrent(finished)
		}
	}

	for _, ssDel := range ssDeletes {
		fmt.Printf("Deleting %s/%s\n", ssDel.BackupDirName, ssDel.SnapshotName)
	}
	err := snapshots.DeleteSnapshots(ctx, encKey, ssDeletes, objst, cfgBucket, vlog, setGGSInitialProgress, updateGGSProgress)
	if err != nil {
		log.Fatalf("Failed to delete snapshot: %v", err)
	}

	persistUsage(nil, true, true, vlog)

	time.Sleep(time.Millisecond * 100) // let the bar finish drawing
}
