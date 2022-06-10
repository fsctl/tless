package cmd

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/spf13/cobra"
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
			if cloudlsCfgGreppableSnapshots {
				cloudlsMainGreppableSnapshots()
			} else if cloudlsCfgSnapshot == "" {
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
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctx, objst, encKey, cfgBucket)
	if err != nil {
		log.Fatalf("Could not get grouped snapshots: %v", err)
	}

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
		fmt.Printf("Backup '%s':\n", groupName)

		snapshotKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots))
		for snapshotName := range groupedObjects[groupName].Snapshots {
			snapshotKeys = append(snapshotKeys, snapshotName)
		}
		sort.Strings(snapshotKeys)

		for _, snapshotName := range snapshotKeys {
			fmt.Printf("  %s\n", snapshotName)

			if cfgVerbose || cloudlsCfgShowChunks {
				relPathKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots[snapshotName].RelPaths))
				for relPath := range groupedObjects[groupName].Snapshots[snapshotName].RelPaths {
					relPathKeys = append(relPathKeys, relPath)
				}
				sort.Strings(relPathKeys)

				for _, relPath := range relPathKeys {
					val := groupedObjects[groupName].Snapshots[snapshotName].RelPaths[relPath]
					deletedMsg := ""
					if val.IsDeleted {
						deletedMsg = " (deleted)"
					}
					fmt.Printf("    %s%s\n", relPath, deletedMsg)

					if cloudlsCfgShowChunks {
						chunkNameKeys := make([]string, 0, len(val.EncryptedChunkNames))
						for chunkName := range val.EncryptedChunkNames {
							chunkNameKeys = append(chunkNameKeys, chunkName)
						}
						sort.Strings(chunkNameKeys)

						for _, chunkName := range chunkNameKeys {
							fmt.Printf("      Chunk: %s (%d bytes)\n", chunkName, val.EncryptedChunkNames[chunkName])
						}
					}
				}
			}
		}
	}
}

func cloudlsMainGreppableSnapshots() {
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctx, objst, encKey, cfgBucket)
	if err != nil {
		log.Fatalf("Could not get grouped snapshots: %v", err)
	}

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
		snapshotKeys := make([]string, 0, len(groupedObjects[groupName].Snapshots))
		for snapshotName := range groupedObjects[groupName].Snapshots {
			snapshotKeys = append(snapshotKeys, snapshotName)
		}
		sort.Strings(snapshotKeys)

		for _, snapshotName := range snapshotKeys {
			fmt.Printf("%s/%s\n", groupName, snapshotName)
		}
	}
}

func cloudlsMainShowSnapshot() {
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)

	snapshotFlagParts := strings.Split(cloudlsCfgSnapshot, "/")
	if len(snapshotFlagParts) != 2 {
		log.Printf("Malformed: '%s'", cloudlsCfgSnapshot)
		return
	}
	backupName := snapshotFlagParts[0]
	snapshotName := snapshotFlagParts[1]
	// TODO: check both parts for regex validity

	mRelPathsObjsMap, err := objstorefs.ReconstructSnapshotFileList(ctx, objst, cfgBucket, encKey, backupName, snapshotName, "", nil, nil)
	if err != nil {
		log.Fatalln("error: reconstructSnapshotFileList failed: ", err)
	}
	for relPath := range mRelPathsObjsMap {
		fmt.Printf("\nFILE> %s\n", relPath)

		if len(mRelPathsObjsMap[relPath]) == 1 {
			fmt.Printf("  +- %s\n", mRelPathsObjsMap[relPath][0])
		} else if len(mRelPathsObjsMap[relPath]) > 1 {
			for _, objName := range mRelPathsObjsMap[relPath] {
				fmt.Printf("  |- %s\n", objName)
			}
		} else {
			log.Fatalf("error: invalid number of chunks planned for %s", relPath)
		}
	}
}
