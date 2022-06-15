// Package objstorefs is a filesystem-like abstraction on top of the objstore package that
// can determine the full list of files in a snapshot, or the full list of snapshots for a
// backup name group.
package objstorefs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
)

// Walks all snapshots from the earliest to snapshotName using the deltas in each to figure out
// what files the snapshot snapshotName consisted of, and what object key to use to retrieve them.
//
// Returns map[string][]string of relPaths mapped onto a slice of either 1 or multiple object names:
//  - One object key in the slice is the case where the entire file fits in a single chunk.
//  - Multiple object keys are of the form xxxxxxxxxxxx.000, xxxxxxxxxxxx.001, xxxxxxxxxxxx.002, etc.
// and need to be concatenated after decryption.
//
// partialRestorePath is to restore a single prefix (like docs/October). Pass "" to ignore it.
// preselectedRelPaths is when you have an exact list of the rel paths you want to restore. Pass nil to ignore it.
// It is recommended to use one or the other but not both at the same time.
//
// groupedObjects argument can be nil and it will be computed for you.  However, because compueting
// it is slow, if caller already has a copy cached, then you can pass it in and your cached copy will
// be used (read-only).
func ReconstructSnapshotFileList(ctx context.Context, objst *objstore.ObjStore, bucket string, key []byte, backupName string, snapshotName string, partialRestorePath string, preselectedRelPaths map[string]int, groupedObjects map[string]BackupDir, vlog *util.VLog) (map[string][]string, error) {
	m := make(map[string][]string)
	var err error

	if groupedObjects == nil {
		groupedObjects, err = GetGroupedSnapshots(ctx, objst, key, bucket, vlog)
		if err != nil {
			log.Fatalf("Could not get grouped snapshots: %v", err)
		}
	}

	// Get the BackupDir struct for this backup name group
	if _, ok := groupedObjects[backupName]; !ok {
		return nil, fmt.Errorf("error: '%s' is not a key into the backup names group", backupName)
	}
	backupDir := groupedObjects[backupName]

	// Get encrypted version of backupName
	encBackupName, err := cryptography.EncryptFilename(key, backupName)
	if err != nil {
		log.Printf("error: skipping b/c could not encrypt backup name '%s'", backupName)
		return nil, err
	}

	// Get an ascending order list of snapshots from the earliest onward
	snapshotKeys := make([]string, 0, len(backupDir.Snapshots))
	for snapshot := range backupDir.Snapshots {
		snapshotKeys = append(snapshotKeys, snapshot)
	}
	sort.Strings(snapshotKeys)

	// Loop through list from earliest until we hit one equal to snapshotName
	for _, currSnapshot := range snapshotKeys {
		// Get encrypted version of currSnapshot
		encCurrSnapshotName, err := cryptography.EncryptFilename(key, currSnapshot)
		if err != nil {
			log.Printf("error: skipping b/c could not encrypt snapshot name '%s'", currSnapshot)
			return nil, err
		}

		// Loop through all the rel paths in current snapshot (order not important)
		for relPath := range backupDir.Snapshots[currSnapshot].RelPaths {
			// For a partial restore, check for prefix on each rel path and skip if no match.
			if partialRestorePath != "" {
				if !strings.HasPrefix(relPath, partialRestorePath) {
					continue
				}
			}

			// For a restore of specific rel paths from a list, skip if the list is non-empty and rel path
			// isn't in it.
			if preselectedRelPaths != nil {
				if _, ok := preselectedRelPaths[relPath]; !ok {
					continue
				}
			}

			if backupDir.Snapshots[currSnapshot].RelPaths[relPath].IsDeleted {
				delete(m, relPath)
			} else {
				m[relPath] = make([]string, 0)

				// Make an ordered list of all chunks for this rel path
				chunkNameKeys := make([]string, 0, len(backupDir.Snapshots[currSnapshot].RelPaths[relPath].EncryptedChunkNames))
				for chunkName := range backupDir.Snapshots[currSnapshot].RelPaths[relPath].EncryptedChunkNames {
					chunkNameKeys = append(chunkNameKeys, encBackupName+"/"+encCurrSnapshotName+"/"+chunkName)
				}
				sort.Strings(chunkNameKeys)

				// Add the ordered list of chunk names to return map
				m[relPath] = append(m[relPath], chunkNameKeys...)
			}
		}

		// Terminate loop once we've completed snapshotName iteration
		if currSnapshot == snapshotName {
			break
		}
	}

	return m, nil
}

type RenameObj struct {
	RelPath     string
	OldSnapshot string
	NewSnapshot string
}

func ComputeSnapshotDelete(key []byte, encBackupDirName string, snapshots map[string]Snapshot, snapshotToDelete string, objst *objstore.ObjStore, ctx context.Context, bucket string) (deleteObjs []map[string]int64, renameObjs []RenameObj, newNextSnapshot *Snapshot, err error) {
	// Make sure snapshotToDelete is actually a real snapshot
	if _, ok := snapshots[snapshotToDelete]; !ok {
		return nil, nil, nil, fmt.Errorf("error: snapshot '%s' not in list of snapshots", snapshotToDelete)
	}

	// Get an ordered list of all snapshots from earliest to most recent
	snapshotKeys := make([]string, 0, len(snapshots))
	for k := range snapshots {
		snapshotKeys = append(snapshotKeys, k)
	}
	sort.Strings(snapshotKeys)

	// Check whether snapshotToDelete is the most recent snapshot.  If it is, just delete everything
	// since there's no snapshot to merge forward into.
	if snapshotKeys[len(snapshotKeys)-1] == snapshotToDelete {
		deleteObjs = deleteAllKeysInSnapshot(snapshots, snapshotToDelete)
		return deleteObjs, nil, nil, nil
	}

	// Get the next snapshot forward in time after snapshotToDelete
	nextSnapshot := getNextSnapshot(snapshotKeys, snapshotToDelete)

	// Create a Snapshot struct for new NEXT snapshot (one we're merging into)
	encNextSnapshot, err := cryptography.EncryptFilename(key, nextSnapshot)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error: ComputeSnapshotDelete: could not encrypt snapshot name '%s'", nextSnapshot)
	}
	newNextSnapshot, err = ReadIndexFileIntoSnapshotObj(key, encBackupDirName, encNextSnapshot, objst, ctx, bucket)
	if err != nil {
		log.Printf("error: ComputeSnapshotDelete: could not read next snapshot's index '%s'", nextSnapshot)
		return nil, nil, nil, err
	}

	// loop over each rel path in snapshotToDelete
	for relPath := range snapshots[snapshotToDelete].RelPaths {
		crp := CloudRelPath{
			EncryptedRelPathStripped: snapshots[snapshotToDelete].RelPaths[relPath].EncryptedRelPathStripped,
			DecryptedRelPath:         relPath,
			EncryptedChunkNames:      snapshots[snapshotToDelete].RelPaths[relPath].EncryptedChunkNames,
		}

		// If relpath in the current snapshot is just a ## deletion marker, we either need to
		// delete it or roll it forward to the next snapshot.
		// - Delete if relPath shows up again in the next snapshot
		// - Else rename and roll it forward
		if snapshots[snapshotToDelete].RelPaths[relPath].IsDeleted {
			crp.IsDeleted = true
			deletionMarker := "##" + snapshots[snapshotToDelete].RelPaths[relPath].EncryptedRelPathStripped

			// If rel path is in next snapshot...
			if containsRelPath(snapshots[nextSnapshot], relPath) {
				// If rel path is in next snapshot as a non-deleted entry, we can delete relpath in snapshotToDelete
				deleteObjs = append(deleteObjs, map[string]int64{deletionMarker: 0})
			} else {
				// If relpath is NOT in next snapshot change set at all, then we must rename relpath objects in
				// snapshotToDelete to next snapshot.
				renameObjs = append(renameObjs, renameDeletionMarkerIntoNextSnapshot(snapshots, relPath, snapshotToDelete, nextSnapshot)...)
				newNextSnapshot.RelPaths[relPath] = crp
			}
		} else {
			// relPath in snapshotToDelete is a real file.  Delete it if it shows up in the next
			// snapshot, otherwise roll it forward
			crp.IsDeleted = false

			if containsRelPath(snapshots[nextSnapshot], relPath) {
				deleteObjs = append(deleteObjs, snapshots[snapshotToDelete].RelPaths[relPath].EncryptedChunkNames)
			} else {
				renameObjs = append(renameObjs, renameAllChunksIntoNextSnapshot(snapshots, relPath, snapshotToDelete, nextSnapshot)...)
				newNextSnapshot.RelPaths[relPath] = crp
			}
		}
	}

	return deleteObjs, renameObjs, newNextSnapshot, nil
}

func renameDeletionMarkerIntoNextSnapshot(snapshots map[string]Snapshot, relPath, snapshotToDelete, nextSnapshot string) (renameObjs []RenameObj) {
	renameObjs = make([]RenameObj, 0)
	renameObj := RenameObj{
		RelPath:     relPath,
		OldSnapshot: snapshotToDelete,
		NewSnapshot: nextSnapshot,
	}
	renameObjs = append(renameObjs, renameObj)
	return renameObjs
}

func deleteAllKeysInSnapshot(snapshots map[string]Snapshot, snapshotToDelete string) (deleteObjs []map[string]int64) {
	deleteObjs = make([]map[string]int64, 0)
	for relPath := range snapshots[snapshotToDelete].RelPaths {
		for chunk, size := range snapshots[snapshotToDelete].RelPaths[relPath].EncryptedChunkNames {
			delObj := make(map[string]int64)
			delObj[chunk] = size
			deleteObjs = append(deleteObjs, delObj)
		}
	}
	return deleteObjs
}

func renameAllChunksIntoNextSnapshot(snapshots map[string]Snapshot, relPath string, snapshotToDelete string, nextSnapshot string) (renameObjs []RenameObj) {
	renameObjs = make([]RenameObj, 0)
	for chunk := range snapshots[snapshotToDelete].RelPaths[relPath].EncryptedChunkNames {
		renameObjs = append(renameObjs, RenameObj{RelPath: chunk, OldSnapshot: snapshotToDelete, NewSnapshot: nextSnapshot})
	}
	return renameObjs
}

// Returns true if snapshot's RelPaths member contains relPath. False otherwise.
func containsRelPath(snapshot Snapshot, relPath string) bool {
	_, ok := snapshot.RelPaths[relPath]
	return ok
}

// Gets the next snapshot forward in time after snapshotToDelete.
// Assumes snapshotKeys are in ascending order.
// Assumes it is never called on the last snapshot in snapshotKeys.
func getNextSnapshot(snapshotKeys []string, snapshotToDelete string) (nextSnapshot string) {
	isNext := false
	for _, val := range snapshotKeys {
		if isNext {
			return val
		}
		if val == snapshotToDelete {
			isNext = true
		}
	}
	log.Fatalln("error: getNextSnapshot couldn't find next snapshot")
	return ""
}

func ReadIndexFileIntoSnapshotObj(key []byte, encryptedBackupDirName string, encryptedSnapshotName string, objst *objstore.ObjStore, ctx context.Context, bucket string) (snapshotObj *Snapshot, err error) {
	// form the obj name of index file: encBackupDir/@encSnapshhotName
	objName := encryptedBackupDirName + "/" + "@" + encryptedSnapshotName

	// retrieve it from cloud
	encCompressedBuf, err := objst.DownloadObjToBuffer(ctx, bucket, objName)
	if err != nil {
		log.Println("error: ReadIndexFileIntoSnapshotObj: UploadObjFromBuffer: ", err)
		return nil, err
	}

	// decrypt
	compressedBuf, err := cryptography.DecryptBuffer(key, encCompressedBuf)
	if err != nil {
		log.Println("error: ReadIndexFileIntoSnapshotObj: DecryptBuffer: ", err)
		return nil, err
	}

	// decompress
	buf, err := util.XZUncompress(compressedBuf)
	if err != nil {
		log.Println("error: ReadIndexFileIntoSnapshotObj: xzUncompress failed")
		return nil, err
	}

	// deserialize to json
	var retObj Snapshot
	err = json.Unmarshal(buf, &retObj)
	if err != nil {
		log.Println("error: ReadIndexFileIntoSnapshotObj: json.Unmarshall failed")
		return nil, err
	}

	return &retObj, nil
}
