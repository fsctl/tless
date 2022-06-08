package daemon

import (
	"context"
	"fmt"
	"log"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/fsctl/tless/pkg/snapshots"
)

func PruneSnapshots() error {
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
		lastBackupTimeFormatted := getLastBackupTimeFormatted(&gGlobalsLock)
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
	copy(encKey, gKey)
	gGlobalsLock.Unlock()
	objst := objstore.NewObjStore(ctx, endpoint, accessKey, secretKey)

	mSnapshots, err := snapshots.GetAllSnapshotInfos(ctx, encKey, objst, bucket)
	if err != nil {
		fmt.Println("AUTOPRUNE> error: prune: ", err)
		return err
	}

	groupedObjects, err := objstorefs.GetGroupedSnapshots(ctx, objst, encKey, bucket)
	if err != nil {
		log.Printf("AUTOPRUNE> error: could not get grouped snapshots: %v", err)
		return err
	}

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
				snapshots.DeleteSnapshot(ctx, encKey, groupedObjects, backupName, ss.Name, objst, bucket)
			} else {
				log.Printf("AUTOPRUNE> Keeping snapshot '%s'\n", ss.RawSnapshotName)
			}
		}
	}

	return nil
}