package daemon

import (
	"context"
	"log"
	"os"
	"time"

	pb "github.com/fsctl/tless/rpc"
)

const (
	wakeEveryNSeconds            int = 1
	automaticBackupEveryNSeconds int = 1 * 60 * 60
	automaticPruneEveryNSeconds  int = 6 * 60 * 60
)

func timerLoop(signals chan os.Signal, server *server) {
	// temporary
	isVerbose := false

	var (
		secondsCnt                 int = 0
		lastAutomaticBackupStarted int = 0
		lastAutomaticPruneStarted  int = 0
	)

	for {
		// Loop wakes up periodically to do automatic tasks
		if isVerbose {
			log.Println("PERIODIC> going to sleep")
		}
		time.Sleep(time.Second * time.Duration(wakeEveryNSeconds))

		// Wake up
		secondsCnt += wakeEveryNSeconds

		if isVerbose {
			log.Println("PERIODIC> woke up")
		}

		// Check the signals channel to see if we should exit routine
		select {
		case sig := <-signals:
			log.Printf("PERIODIC> exiting (rcvd signal %v)", sig)
			return
		default:
		}

		// Has gCfg, gUsername, etc been set yet?  Cannot do anything until that is done
		gGlobalsLock.Lock()
		isReadyForPeriodics := gCfg != nil && gUsername != "" && gUserHomeDir != "" && gDb != nil && gKey != nil
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
				log.Println("PERIODIC> cannot start backup b/c we're not in Idle state")
			}
		}

		//
		// Automatic snapshot prune
		//
		if secondsCnt-lastAutomaticPruneStarted >= automaticPruneEveryNSeconds {
			// Attempt to start a prune of snapshots, updating lastAutomaticPruneStarted if successful
			gGlobalsLock.Lock()
			isIdle := gStatus.state == Idle
			gGlobalsLock.Unlock()

			if isIdle {
				//
				// TODO TODO TODO
				//

				lastAutomaticPruneStarted = secondsCnt
			} else {
				log.Println("PERIODIC> cannot prune snapshots b/c we're not in Idle state")
			}
		}
	}
}
