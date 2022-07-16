package daemon

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
)

func PruneSnapshots() error {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	gGlobalsLock.Lock()
	isIdle := gStatus.state == Idle
	gGlobalsLock.Unlock()
	if !isIdle {
		log.Println("AUTOPRUNE> Not in Idle state, cannot prune")
		return fmt.Errorf("error: not in Idle state")
	}

	gGlobalsLock.Lock()
	gStatus.state = CleaningUp
	gStatus.msg = "Pruning snapshots"
	gStatus.percentage = 0.0
	gGlobalsLock.Unlock()

	done := func() {
		lastBackupTimeFormatted := getLastBackupTimeFormatted(&gDbLock)
		gGlobalsLock.Lock()
		gStatus.state = Idle
		gStatus.msg = "Last backup: " + lastBackupTimeFormatted
		gStatus.percentage = -1.0
		gGlobalsLock.Unlock()
	}
	defer done()

	ctx := context.Background()
	encKey := make([]byte, 32)
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	trustSelfSignedCerts := gCfg.TrustSelfSignedCerts
	copy(encKey, gEncKey)
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey, trustSelfSignedCerts)

	mSnapshots, err := snapshots.GetAllSnapshotInfos(ctx, encKey, objst, bucket)
	if err != nil {
		fmt.Println("AUTOPRUNE> error: prune: ", err)
		return err
	}

	cntDeletedSnapshots := 0
	for backupName := range mSnapshots {
		// Mark what is to be kept
		keeps := snapshots.GetPruneKeepsList(mSnapshots[backupName])

		for _, ss := range mSnapshots[backupName] {
			keepCurr := false
			for _, k := range keeps {
				if ss == k {
					keepCurr = true
					break
				}
			}

			if !keepCurr {
				log.Printf("AUTOPRUNE> Deleting snapshot '%s'\n", ss.RawSnapshotName)
				ssDel := snapshots.SnapshotForDeletion{
					BackupDirName: backupName,
					SnapshotName:  ss.Name,
				}
				if err = snapshots.DeleteSnapshots(ctx, encKey, []snapshots.SnapshotForDeletion{ssDel}, objst, bucket, vlog, nil, nil); err != nil {
					log.Printf("AUTOPRUNE> error: could not delete snapshot '%s': %v\n", ss.RawSnapshotName, err)
				} else {
					cntDeletedSnapshots += 1
				}
			} else {
				log.Printf("AUTOPRUNE> Keeping snapshot '%s'\n", ss.RawSnapshotName)
			}
		}
	}

	// Log a reported event for the autoprune
	msg := fmt.Sprintf("deleted %d snapshots", cntDeletedSnapshots)
	gGlobalsLock.Lock()
	gStatus.reportedEvents = append(gStatus.reportedEvents, util.ReportedEvent{
		Kind:     util.INFO_AUTOPRUNE_COMPLETED,
		Path:     "",
		IsDir:    false,
		Datetime: time.Now().Unix(),
		Msg:      msg,
	})
	gGlobalsLock.Unlock()

	return nil
}
