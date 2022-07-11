package cmd

import (
	"context"
	"fmt"
	"log"
	"sort"
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
	cloudlsCfgShowChunks         bool
	cloudlsCfgSnapshot           string
	cloudlsCfgGreppableSnapshots bool

	// Command
	cloudlsCmd = &cobra.Command{
		Use:   "cloudls",
		Short: "Lists all data stored in cloud",
		Long: `Lists everything you have stored in the cloud. File and directory names are automatically
decrypted in the display.  Without --verbose, just shows all snapshots for each backup name group.
With --verbose, shows the files in each snapshot.

Example:

	tless cloudls
	tless cloudls --verbose
	tless cloudls --snapshot=Documents/2020-01-01_04.56.01

The available snapshot times are displayed in 'tless cloudls' with no arguments.
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if cloudlsCfgGreppableSnapshots || cloudlsCfgSnapshot == "" {
				cloudlsMain()
			} else {
				cloudlsMainShowSnapshot()
			}
		},
	}
)

func init() {
	cloudlsCmd.Flags().BoolVar(&cloudlsCfgShowChunks, "show-chunks", false, "show the chunk(s) making up each file; implies -v (default: false)")
	cloudlsCmd.Flags().BoolVar(&cloudlsCfgGreppableSnapshots, "grep", false, "show a grep-friendly snapshot list (default: false)")
	cloudlsCmd.Flags().StringVar(&cloudlsCfgSnapshot, "snapshot", "", "snapshot to display (eg, 'Documents/2020-01-01_01.02.03'); implies -v")
	rootCmd.AddCommand(cloudlsCmd)
}

func cloudlsMain() {
	vlog := util.NewVLog(nil, func() bool { return cfgVerbose })

	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)

	// initialize progress bar container and its callbacks
	progressBarContainer := mpb.New()
	var progressBar *mpb.Bar = nil
	setGGSInitialProgress := func(finished int64, total int64) {
		if !cfgVerbose && !cloudlsCfgGreppableSnapshots {
			name := "Reading snapshots"
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
		if !cfgVerbose && !cloudlsCfgGreppableSnapshots {
			progressBar.SetCurrent(finished)
		}
	}

	groupedObjects, err := snapshots.GetGroupedSnapshots(ctx, objst, encKey, cfgBucket, vlog, setGGSInitialProgress, updateGGSProgress)
	if err != nil {
		log.Fatalf("Could not get grouped snapshots: %v", err)
	}
	time.Sleep(time.Millisecond * 100) // let the bar finish drawing

	// print out each backup name group
	if len(groupedObjects) == 0 {
		fmt.Println("No objects found in cloud")
	}

	groupNameKeys := make([]string, 0, len(groupedObjects))
	for groupName := range groupedObjects {
		groupNameKeys = append(groupNameKeys, groupName)
	}
	sort.Strings(groupNameKeys)

	for _, groupName := range groupNameKeys {
		if !cloudlsCfgGreppableSnapshots {
			fmt.Printf("Backup '%s':\n", groupName)
		}

		snapshotKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots))
		for snapshotName := range groupedObjects[groupName].Snapshots {
			snapshotKeys = append(snapshotKeys, snapshotName)
		}
		sort.Strings(snapshotKeys)

		for _, snapshotName := range snapshotKeys {
			if cloudlsCfgGreppableSnapshots {
				fmt.Printf("%s/%s\n", groupName, snapshotName)
			} else {
				fmt.Printf("  %s\n", snapshotName)

				if cfgVerbose || cloudlsCfgShowChunks {
					relPathKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots[snapshotName].RelPaths))
					for relPath := range groupedObjects[groupName].Snapshots[snapshotName].RelPaths {
						relPathKeys = append(relPathKeys, relPath)
					}
					sort.Strings(relPathKeys)

					for _, relPath := range relPathKeys {
						val := groupedObjects[groupName].Snapshots[snapshotName].RelPaths[relPath]
						fmt.Printf("    %s\n", relPath)

						if cloudlsCfgShowChunks {
							for _, chunkExtent := range val.ChunkExtents {
								fmt.Printf("      Chunk: %s (offset: %d, len: %d)\n", chunkExtent.ChunkName, chunkExtent.Offset, chunkExtent.Len)
							}
						}
					}
				}
			}
		}
	}
}

func cloudlsMainShowSnapshot() {
	vlog := util.NewVLog(nil, func() bool { return cfgVerbose })

	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)

	backupName, snapshotName, err := util.SplitSnapshotName(cloudlsCfgSnapshot)
	if err != nil {
		log.Fatalf("Cannot split '%s' into backupDirName/snapshotTimestamp", cloudrmCfgSnapshot)
	}

	groupedObjects, err := snapshots.GetGroupedSnapshots(ctx, objst, encKey, cfgBucket, vlog, nil, nil)
	if err != nil {
		log.Fatalf("Could not get grouped snapshots: %v", err)
	}
	mRelPathsObjsMap := groupedObjects[backupName].Snapshots[snapshotName].RelPaths
	for relPath := range groupedObjects[backupName].Snapshots[snapshotName].RelPaths {
		fmt.Printf("\nFILE> %s\n", relPath)

		if len(mRelPathsObjsMap[relPath].ChunkExtents) == 1 {
			fmt.Printf("  +- %s (offset: %d, len: %d)\n", mRelPathsObjsMap[relPath].ChunkExtents[0].ChunkName, mRelPathsObjsMap[relPath].ChunkExtents[0].Offset, mRelPathsObjsMap[relPath].ChunkExtents[0].Len)
		} else if len(mRelPathsObjsMap[relPath].ChunkExtents) > 1 {
			for _, chunkExtent := range mRelPathsObjsMap[relPath].ChunkExtents {
				fmt.Printf("  |- %s (offset: %d, len: %d)\n", chunkExtent.ChunkName, chunkExtent.Offset, chunkExtent.Len)
			}
		} else {
			log.Fatalf("error: invalid number of chunks planned for %s", relPath)
		}
	}
}
