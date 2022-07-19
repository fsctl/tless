package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsctl/tless/pkg/backup"
	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

var (
	// Flags
	cfgDirs         []string
	cfgExcludePaths []string
	cfgResumeBackup bool

	// Command
	backupCmd = &cobra.Command{
		Use:   "backup",
		Short: "Performs an incremental backup",
		Long: `Performs an incremental backup. The directories to recursively back up are listed
in the config file, or can be specified on the command line (-d). It is recommended that you create
a configuration file rather than use the command line flags.

Example:

	tless backup

This will read your config.toml configuration file for all information about the backup,
cloud provider credentials, and master password. It will then perform an incremental backup to
your cloud provider, uploading only those files that have changed since the last backup. Files
on the cloud provider will be overwritten by these newer local copies.
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			backupMain()
		},
	}
)

func init() {
	backupCmd.Flags().StringArrayVarP(&cfgDirs, "dir", "d", nil, "directories to backup (can use multiple times)")
	backupCmd.Flags().StringArrayVarP(&cfgExcludePaths, "exclude", "x", nil, "paths prefixes to exclude from backup (can use multiple times)")
	backupCmd.Flags().BoolVar(&cfgResumeBackup, "resume-backup", true, "resume (vs rollback) any previous interrupted run")
	rootCmd.AddCommand(backupCmd)
}

func backupMain() {
	ctx := context.Background()

	vlog := util.NewVLog(nil, func() bool { return cfgVerbose })

	// check that cfgDirs is set, and allow cfgExcludePaths to be set from toml file if no arg
	if err := validateDirs(); err != nil {
		log.Fatalln("no valid dirs to back up: ", err)
	}
	if cfgExcludePaths == nil {
		cfgExcludePaths = viper.GetStringSlice("backups.excludes")
	}

	// open and prepare sqlite database
	sqliteDir, err := util.MkdirUserConfig("", "")
	if err != nil {
		log.Fatalf("error: making sqlite dir: %v", err)
	}
	db, err := database.NewDB(filepath.Join(sqliteDir, "state.db"))
	if err != nil {
		log.Fatalf("error: cannot open database: %v", err)
	}
	defer db.Close()
	if err := db.PerformDbMigrations(vlog); err != nil {
		log.Fatalf("error: cannot initialize database: %v", err)
	}

	// open connection to cloud server
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey, cfgTrustSelfSignedCerts)
	if ok, err := objst.IsReachable(ctx, cfgBucket, vlog); !ok {
		log.Fatalln("error: exiting because server not reachable: ", err)
	}

	onDone := func() {
		// On finished, log the new total space usage
		persistUsage(db, true, true, vlog)
	}

	// initialize progress bar container and its callbacks
	progressBarContainer := mpb.New()
	var progressBar *mpb.Bar = nil
	setBackupInitialProgressFunc := func(finished int64, total int64, backupDirName string, vlog *util.VLog) {
		if !cfgVerbose {
			progressBar = progressBarContainer.New(
				total,
				mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Rbound("]"),
				mpb.PrependDecorators(
					decor.Name(backupDirName, decor.WC{W: len(backupDirName) + 1, C: decor.DidentRight}),
					// replace ETA decorator with "done" message on OnComplete event
					decor.OnComplete(
						decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 4}), "done",
					),
				),
				mpb.AppendDecorators(decor.Percentage()),
			)
		}
	}
	updateBackupProgressFunc := func(finished int64, total int64, vlog *util.VLog) {
		if !cfgVerbose {
			progressBar.SetCurrent(finished)
		}
	}

	// replay/rollback check - if there's an interrupted previous backup, deal with it and exit
	if didResumeOrRollback := handleReplay(ctx, objst, db, vlog, setBackupInitialProgressFunc, updateBackupProgressFunc); didResumeOrRollback {
		onDone()
		return
	}

	// main loop through backup dirs
	for _, backupDirPath := range cfgDirs {
		// log what iteration of the loop we're in
		vlog.Printf("Inspecting %s...\n", backupDirPath)

		// init the progress bar to nil
		progressBar = nil

		// Traverse the FS for changed files and do the journaled backup
		stats := backup.NewBackupStats()
		backupReportedEvents, breakFromLoop, continueLoop, fatalError := backup.DoJournaledBackup(ctx, encKey, objst, cfgBucket, nil, db, backupDirPath, cfgExcludePaths, vlog, nil, nil, setBackupInitialProgressFunc, updateBackupProgressFunc, stats, cfgResourceUtilization)
		for _, e := range backupReportedEvents {
			if e.Kind == util.ERR_OP_NOT_PERMITTED {
				log.Printf("warning:  insufficient permissions to process path '%s'", e.Path)
			}
		}
		if fatalError {
			goto done
		}
		if continueLoop {
			continue
		}
		if breakFromLoop {
			break
		}
	}

	if cfgVerbose {
		fmt.Printf("done\n")
	} else {
		// Give progress bar 0.1 sec to draw itself for final time
		time.Sleep(1e8)
	}

done:
	onDone()
}

func handleReplay(ctx context.Context, objst *objstore.ObjStore, db *database.DB, vlog *util.VLog, setBackupInitialProgressFunc backup.SetReplayInitialProgressFuncType, updateBackupProgressFunc backup.UpdateProgressFuncType) bool {
	hasDirtyBackupJournal, err := db.HasDirtyBackupJournal()
	if err != nil {
		log.Println("error: could not determine if previous backup was interrupted: ", err)
		return false
	}

	if hasDirtyBackupJournal {
		if cfgResumeBackup {
			fmt.Println("Resuming previous interrupted backup... (--resume-backup=false to roll back)")
			backup.ReplayBackupJournal(ctx, encKey, objst, cfgBucket, nil, db, vlog, setBackupInitialProgressFunc, nil, updateBackupProgressFunc, cfgResourceUtilization)
		} else {
			fmt.Println("Rolling back previous interrupted backup...")

			// get the backupName and snapshotName
			backupDirPath, snapshotUnixTime, err := db.GetJournaledBackupInfo()
			if err != nil {
				log.Fatalf("error: handleReplay: could not get the journal info: %v", err)
			}
			snapshotName := time.Unix(snapshotUnixTime, 0).UTC().Format("2006-01-02_15.04.05")

			// Delete the snapshot if it exists
			ssDel := snapshots.SnapshotForDeletion{
				BackupDirName: filepath.Base(backupDirPath),
				SnapshotName:  snapshotName,
			}
			err = snapshots.DeleteSnapshots(ctx, encKey, []snapshots.SnapshotForDeletion{ssDel}, objst, cfgBucket, vlog, nil, nil)
			if err != nil {
				// This is ok and just means snapshot index file wasn't writetn to cloud yet
				vlog.Printf("warning: handleReplay: could not delete partially created snapshot index (probably does not exist yet): %v", err)

				// Garbage collect any orphaned chunks that were written while creating unused snapshot index file
				if err = snapshots.GCChunks(ctx, objst, cfgBucket, encKey, vlog, nil, nil); err != nil {
					log.Println("error: handleReplay: could not garbage collect chunks: ", err)
				}
			}

			// reset the last backup times in db
			err = db.CancelationResetLastBackupTime()
			if err != nil {
				log.Fatalf("error: handleReplay: db.CancelationResetLastBackupTime failed")
			}

			// clean up the journal
			err = db.CancelationCleanupJournal()
			if err != nil {
				log.Fatalf("error: handleReplay: db.CancelationCleanupJournal failed")
			}
		}
		return true
	}
	return false
}

func validateDirs() error {
	if len(cfgDirs) == 0 {
		return fmt.Errorf("backup dirs invalid (value='%v')", cfgDirs)
	}
	for _, dir := range cfgDirs {
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("backup dir '%s' does not exist)", dir)
		}
	}
	return nil
}
