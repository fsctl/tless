package daemon

import (
	"context"
	"log"
	"time"

	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

const (
	wakeEveryNSeconds            int   = 60
	dontDoAnythingFirstNSeconds  int64 = 15 * 60
	automaticBackupEveryNSeconds int64 = 24 * 60 * 60
	automaticPruneEveryNSeconds  int64 = 24 * 60 * 60
	persistUsageEveryNSeconds    int64 = 6 * 60 * 60
)

func timerLoop(server *server) {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	// When we first start, pretend some tasks just ran so we don't run one too soon
	var (
		startedAtUnixtime     int64 = time.Now().Unix()
		lastAutopruneUnixtime int64 = time.Now().Unix()
		lastPersistUsage      int64 = time.Now().Unix()
	)

	for {
		time.Sleep(time.Second * time.Duration(wakeEveryNSeconds))
		nowUnixtime := time.Now().Unix()

		// Check if config is ready
		gGlobalsLock.Lock()
		isReadyForPeriodics := gCfg != nil && gUsername != "" && gUserHomeDir != "" && gEncKey != nil
		gGlobalsLock.Unlock()
		gDbLock.Lock()
		isReadyForPeriodics = isReadyForPeriodics && gDb != nil
		gDbLock.Unlock()
		if !isReadyForPeriodics {
			continue
		}

		// We don't do anything until a certain amount of time since startup has passed
		if nowUnixtime-startedAtUnixtime < dontDoAnythingFirstNSeconds {
			continue
		}

		//
		// When was last backup? If it's been long enough, start one.
		//
		gDbLock.Lock()
		lastBackupUnixtime, err := gDb.GetLastCompletedBackupUnixTime()
		gDbLock.Unlock()
		if err != nil {
			log.Printf("error: could not get last completed backup time: %v", err)
		}
		secondsSinceLastBackup := nowUnixtime - lastBackupUnixtime
		if secondsSinceLastBackup > automaticBackupEveryNSeconds {
			// Attempt to start a backup
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
				}
			} else {
				vlog.Println("PERIODIC> cannot start backup b/c we're not in Idle state")
			}
		}

		//
		// When was last autoprune?  Start one if it's been long enough
		//
		secondsSinceLastAutoprune := nowUnixtime - lastAutopruneUnixtime
		if secondsSinceLastAutoprune > automaticPruneEveryNSeconds {
			// Attempt to start an autoprune
			gGlobalsLock.Lock()
			isIdle := gStatus.state == Idle
			gGlobalsLock.Unlock()
			if isIdle {
				if err := PruneSnapshots(); err != nil {
					log.Println("PERIODIC> failed to run autoprune: ", err)
				} else {
					lastAutopruneUnixtime = time.Now().Unix()
					persistUsage(true, true, vlog)
					lastPersistUsage = time.Now().Unix()
				}
			}
		}

		//
		// When did we last persist usage? If it's been long enough, do it now.
		//
		secondsSinceLastPersistUsage := nowUnixtime - lastPersistUsage
		if secondsSinceLastPersistUsage > persistUsageEveryNSeconds {
			gGlobalsLock.Lock()
			isIdle := gStatus.state == Idle
			gGlobalsLock.Unlock()
			if isIdle {
				persistUsage(true, true, vlog)
				lastPersistUsage = time.Now().Unix()
			} else {
				vlog.Printf("error: timerLoop: cannot persist usage because we are not in Idle state")
			}
		}
	}
}
