package daemon

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

const (
	wakeEveryNSeconds            int = 60
	automaticBackupEveryNSeconds int = 6 * 60 * 60
	automaticPruneEveryNSeconds  int = 12 * 60 * 60
	firstAutomaticBackupAfterMin int = 15

	dbgDisableAutoprune bool = false
)

func timerLoop(server *server) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	// Monitor for SIGINT and SIGTERM and exit routine if caught
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// Start the first automatic backup 15 minutes after this loop starts running
	var (
		secondsCnt                 int = automaticBackupEveryNSeconds - (firstAutomaticBackupAfterMin * 60)
		lastAutomaticBackupStarted int = 0
		lastAutomaticPruneStarted  int = 0
	)

	for {
		// Loop wakes up periodically to do automatic tasks
		//vlog.Println("PERIODIC> going to sleep")
		for i := 0; i < wakeEveryNSeconds; i++ {
			time.Sleep(time.Second)

			// Check the signals channel to see if we should exit routine
			select {
			case sig := <-signals:
				log.Printf("PERIODIC> exiting (rcvd signal %v)", sig)
				return
			default:
			}
		}

		// Wake up
		secondsCnt += wakeEveryNSeconds

		//vlog.Println("PERIODIC> woke up")

		// Has gCfg, gUsername, etc been set yet?  Cannot do anything until that is done
		gGlobalsLock.Lock()
		isReadyForPeriodics := gCfg != nil && gUsername != "" && gUserHomeDir != "" && gDb != nil && gEncKey != nil
		gGlobalsLock.Unlock()
		if !isReadyForPeriodics {
			continue
		}

		//
		// Automatic periodic backup
		//
		if secondsCnt-lastAutomaticBackupStarted >= automaticBackupEveryNSeconds {
			// Attempt to start a backup, update lastAutomaticBackupStarted if successful
			gGlobalsLock.Lock()
			isIdle := gStatus.state == Idle
			gGlobalsLock.Unlock()

			if isIdle {
				in := &pb.BackupRequest{}
				in.ForceFullBackup = false
				response, err := server.Backup(context.Background(), in)
				if err != nil {
					log.Printf("PERIODIC> error: periodic backup failed: %v", err)
				} else if !response.IsStarting {
					log.Printf("PERIODIC> error: periodic backup failed with ErrMsg: %s", response.ErrMsg)
				} else {
					log.Println("PERIODIC> periodic backup started")
					lastAutomaticBackupStarted = secondsCnt
				}
			} else {
				vlog.Println("PERIODIC> cannot start backup b/c we're not in Idle state")
			}
		}

		//
		// Automatic snapshot prune
		//
		if !dbgDisableAutoprune && (secondsCnt-lastAutomaticPruneStarted >= automaticPruneEveryNSeconds) {
			// Attempt to start a prune of snapshots, updating lastAutomaticPruneStarted if successful
			gGlobalsLock.Lock()
			isIdle := gStatus.state == Idle
			gGlobalsLock.Unlock()

			if isIdle {
				err := PruneSnapshots()
				if err == nil {
					lastAutomaticPruneStarted = secondsCnt
				}
			} else {
				vlog.Println("PERIODIC> cannot prune snapshots b/c we're not in Idle state")
			}
		}
	}
}
