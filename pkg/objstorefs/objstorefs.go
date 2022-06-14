// Package objstorefs is a filesystem-like abstraction on top of the objstore package that
// can determine the full list of files in a snapshot, or the full list of snapshots for a
// backup name group.
package objstorefs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
)

type CloudRelPath struct {
	// Encrypted relative path, stripped of ## prefix and .NNN suffix. Contains '/'.
	// Ex:    -hK9_DlG_AFMIE1PKHZaBUebiFvnvV/MjTo0Vx771kwwMbjivWed65LkTSq8A
	// Ex 2:  0lQjIGkeLOlUMnekodf0alyD1Xq9cXrGD0/9cI0DMhON8b6i0cVsDt9kVePqrvFw4d6d1
	// Ex 3:  U7j1GWpdrvxmOOGpStOZCJfaRuqkF6Zv5xaTjnTe/E9QzmBJ-_UF54dmvOyybTgtzA2xuxNDfsaNc1xT-
	EncryptedRelPathStripped string

	// The plaintext rel path
	// Ex:    file4
	// Ex 2:  subdir2/subdir2a
	// Ex 3:  will-delete-dir/file
	DecryptedRelPath string

	// Map of all chunk names onto their (encrypted & compressed) size
	// Ex:    map[-hK9_DlG_AFMIE1PKHZaBUebiFvnvV/MjTo0Vx771kwwMbjivWed65LkTSq8A.000:134217879 -hK9_DlG_AFMIE1PKHZaBUebiFvnvV/MjTo0Vx771kwwMbjivWed65LkTSq8A.001:2097180]
	// Ex 2:  map[0lQjIGkeLOlUMnekodf0alyD1Xq9cXrGD0/9cI0DMhON8b6i0cVsDt9kVePqrvFw4d6d1:168]
	// Ex 3:  map[##U7j1GWpdrvxmOOGpStOZCJfaRuqkF6Zv5xaTjnTe/E9QzmBJ-_UF54dmvOyybTgtzA2xuxNDfsaNc1xT-:0]  (even though file had .000 and .001 before deletion)
	EncryptedChunkNames map[string]int64

	// Bool indicating whether the rel path is deleted in parent snapshot
	// Ex:    false
	// Ex 2:  false
	// Ex 3:  true
	IsDeleted bool
}

type Snapshot struct {
	EncryptedName string
	DecryptedName string
	Datetime      time.Time
	RelPaths      map[string]CloudRelPath
}

type BackupDir struct {
	EncryptedName string
	DecryptedName string
	Snapshots     map[string]Snapshot
}

func (crp *CloudRelPath) ToJson() []byte {
	buf, err := json.Marshal(crp)
	if err != nil {
		log.Println("error: CloudRelPath.ToJson marshal failed: ", err)
		return []byte{}
	}
	return buf
}

func NewCloudRelPathFromJson(jsonBuf []byte) *CloudRelPath {
	obj := CloudRelPath{}
	err := json.Unmarshal(jsonBuf, &obj)
	if err != nil {
		log.Println("error: NewCloudRelPathFromJson unmarshal failed: ", err)
		return nil
	}
	return &obj
}

func GetGroupedSnapshots(ctx context.Context, objst *objstore.ObjStore, key []byte, bucket string) (map[string]BackupDir, error) {
	retBackupDirs := make(map[string]BackupDir)

	allObjects, err := objst.GetObjList(ctx, bucket, "", nil)
	if err != nil {
		log.Printf("error: GetGroupedSnapshots: GetObjList failed: %v", err)
		return nil, err
	}

	reDot, _ := regexp.Compile(`\.`)

	for objName, size := range allObjects {
		// skip the salt and version object keys
		if objName[:5] == "SALT-" || objName[:7] == "VERSION" {
			continue
		}
		// skip snapshot index files
		if strings.Contains(objName, "@") {
			continue
		}

		// We expect everything after this to be backupName/snapshotName/relP/ath.
		// (relPath is split in half by a slash to overcome server limitations.) So,
		// if objName is not splittable by slashes into 4 parts, ignore it.
		parts := strings.Split(objName, "/")
		if len(parts) != 4 {
			log.Printf("error: object name not in proper format '%s' (skipping)", objName)
			continue
		}
		encBackupName := parts[0]
		encSnapshotName := parts[1]
		encRelPath := parts[2] + "/" + parts[3]
		encRelPathWithoutSlash := parts[2] + parts[3]

		// Get the prefix and suffix stripped versions of encRelPath
		isDeleted := false
		encRelPathStripped := encRelPath
		encRelPathWithoutSlashStripped := encRelPathWithoutSlash
		if strings.HasPrefix(encRelPath, "##") {
			encRelPathStripped = strings.TrimPrefix(encRelPathStripped, "##")
			encRelPathWithoutSlashStripped = strings.TrimPrefix(encRelPathWithoutSlashStripped, "##")
			isDeleted = true
		}
		hasDot := reDot.FindAllString(encRelPathStripped, -1) != nil
		if hasDot {
			// strip trailing .NNN if present
			encRelPathStripped = encRelPathStripped[:len(encRelPathStripped)-4]
			encRelPathWithoutSlashStripped = encRelPathWithoutSlashStripped[:len(encRelPathWithoutSlashStripped)-4]
		}

		backupName, snapshotName, relPath, err := decryptNamesTriplet(key, encBackupName, encSnapshotName, encRelPathWithoutSlashStripped)
		if err != nil {
			continue
		}

		// Parse the snapshot datetime string
		snapShotDateTime, err := time.Parse("2006-01-02_15.04.05", snapshotName)
		if err != nil {
			log.Printf("error: GetGroupedSnapshots: time.Parse failed on '%s': %v", snapshotName, err)
			continue
		}

		if _, ok := retBackupDirs[backupName]; !ok {
			// backupName key is new, so add a map k/v pair for it
			retBackupDirs[backupName] = BackupDir{
				EncryptedName: encBackupName,
				DecryptedName: backupName,
				Snapshots:     make(map[string]Snapshot),
			}
		}

		if _, ok := retBackupDirs[backupName].Snapshots[snapshotName]; !ok {
			// snapshotName is new, so add a k/v pair for it
			retBackupDirs[backupName].Snapshots[snapshotName] = Snapshot{
				EncryptedName: encSnapshotName,
				DecryptedName: snapshotName,
				Datetime:      snapShotDateTime,
				RelPaths:      make(map[string]CloudRelPath),
			}
		}

		if _, ok := retBackupDirs[backupName].Snapshots[snapshotName].RelPaths[relPath]; !ok {
			// relPath is new, so add a k/v pair for it
			retBackupDirs[backupName].Snapshots[snapshotName].RelPaths[relPath] = CloudRelPath{
				EncryptedRelPathStripped: encRelPathStripped,
				DecryptedRelPath:         relPath,
				EncryptedChunkNames:      make(map[string]int64),
				IsDeleted:                isDeleted,
			}
		}

		retBackupDirs[backupName].Snapshots[snapshotName].RelPaths[relPath].EncryptedChunkNames[encRelPath] = size
	}

	return retBackupDirs, nil
}

////////////////////////////////  /new code for GetGroupedSnapshots2  //////////////////////////////////////////////

// TODO:  take a vlog (optional)
func GetGroupedSnapshots2(ctx context.Context, objst *objstore.ObjStore, key []byte, bucket string) (map[string]BackupDir, error) {
	// setup return map
	ret := make(map[string]BackupDir)

	// get all backup names from cloud (top level paths)
	topLevelObjs, err := objst.GetObjListTopLevel(ctx, bucket, []string{"SALT-", "VERSION"})
	if err != nil {
		log.Println("error: GetGroupedSnapshots2: objst.GetObjListTopLevel: ", err)
		return nil, err
	}

	for _, encBackupName := range topLevelObjs {
		// get decrypted backup name
		backupName, err := cryptography.DecryptFilename(key, encBackupName)
		if err != nil {
			log.Printf("error: GetGroupedSnapshots2: could not decrypt backup dir name (%s): %v\n", encBackupName, err)
			return nil, err
		}

		// add an object to ret map for this backup
		ret[backupName] = BackupDir{
			EncryptedName: encBackupName,
			DecryptedName: backupName,
			Snapshots:     make(map[string]Snapshot),
		}

		// get all snapshot index files for this backup
		indexFiles, err := getAllSnapshotIndexFiles(ctx, objst, key, encBackupName, bucket)
		if err != nil {
			log.Printf("error: GetGroupedSnapshots2: could not get snapshot index files for '%s': %v\n", backupName, err)
			return nil, err
		}

		for ssName, indexFileJsonBuf := range indexFiles {
			// reconstruct object hierarchy for this snapshot, placing into BackupDir objects in map
			ssObj := Snapshot{}
			err := json.Unmarshal(indexFileJsonBuf, &ssObj)
			if err != nil {
				log.Println("error: GetGroupedSnapshots2 unmarshal failed: ", err)
				return nil, err
			}

			ret[backupName].Snapshots[ssName] = ssObj
		}
	}

	// return map
	return ret, nil
}

// Returns a map of [decrypted snapshot index obj names] onto [slices of (encrypted, compressed) bytes] for that index file
// TODO:  cache decrypted/uncompressed files, and then look in cache first
func getAllSnapshotIndexFiles(ctx context.Context, objst *objstore.ObjStore, key []byte, encBackupName string, bucket string) (map[string][]byte, error) {
	ret := make(map[string][]byte)

	mObjs, err := objst.GetObjList2(ctx, bucket, encBackupName+"/@", false, nil)
	if err != nil {
		log.Printf("error: getAllSnapshotIndices: could not get list of snapshot index files for '%s': %v\n", encBackupName, err)
		return nil, err
	}

	for encObjName := range mObjs {
		log.Printf("Working on index file:  %s", encObjName) //vlog

		// strip off the prefix "encBackupName/@"
		encSsName := strings.TrimPrefix(encObjName, encBackupName+"/@")

		// decrypt snapshot name
		ssName, err := cryptography.DecryptFilename(key, encSsName)
		if err != nil {
			log.Printf("error: getAllSnapshotIndexFiles: could not decrypt snapshot name (%s): %v\n", encSsName, err)
			return nil, err
		}
		log.Printf("While is for snapshot:  %s", ssName) //vlog

		// download actual snapshot file
		buf, err := objst.DownloadObjToBuffer(ctx, bucket, encObjName)
		if err != nil {
			log.Printf("error: getAllSnapshotIndices: could not download snapshot index file '%s': %v\n", encObjName, err)
			return nil, err
		}

		// decrypt and uncompress snapshot file
		plaintextIndexFileBuf, err := decryptAndUncompressIndexFile(key, buf)
		if err != nil {
			log.Printf("error: getAllSnapshotIndices: could not decrypt+uncompress snapshot index file '%s': %v\n", ssName, err)
			return nil, err
		}

		ret[ssName] = plaintextIndexFileBuf
	}

	return ret, nil
}

func decryptAndUncompressIndexFile(key []byte, encBuf []byte) ([]byte, error) {
	// Decrypt
	decBuf, err := cryptography.DecryptBuffer(key, encBuf)
	if err != nil {
		log.Println("error: decryptAndUncompressIndexFile: could not decrypt snapshot index file: ", err)
		return nil, err
	}

	// Uncompress
	uncompressedBuf, err := util.XZUncompress(decBuf)
	if err != nil {
		log.Println("error: decryptAndUncompressIndexFile: could not decompress snapshot index file: ", err)
		return nil, err
	}

	return uncompressedBuf, nil
}

/////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func decryptNamesTriplet(key []byte, encBackupName string, encSnapshotName string, encRelPath string) (backupName string, snapshotName string, relPath string, err error) {
	backupName, err = cryptography.DecryptFilename(key, encBackupName)
	if err != nil {
		log.Printf("error: skipping b/c could not decrypt encrypted backup name '%s'", encBackupName)
		return "", "", "", err
	}
	snapshotName, err = cryptography.DecryptFilename(key, encSnapshotName)
	if err != nil {
		log.Printf("error: skipping b/c could not decrypt encrypted snapshot name '%s'", encSnapshotName)
		return "", "", "", err
	}
	relPath, err = cryptography.DecryptFilename(key, encRelPath)
	if err != nil {
		log.Printf("error: skipping b/c could not decrypt encrypted rel path '%s'", encRelPath)
		return "", "", "", err
	}
	return backupName, snapshotName, relPath, nil
}

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
func ReconstructSnapshotFileList(ctx context.Context, objst *objstore.ObjStore, bucket string, key []byte, backupName string, snapshotName string, partialRestorePath string, preselectedRelPaths map[string]int, groupedObjects map[string]BackupDir) (map[string][]string, error) {
	m := make(map[string][]string)
	var err error

	if groupedObjects == nil {
		groupedObjects, err = GetGroupedSnapshots(ctx, objst, key, bucket)
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

func ComputeSnapshotDelete(snapshots map[string]Snapshot, snapshotToDelete string) (deleteObjs []map[string]int64, renameObjs []RenameObj, err error) {
	// Make sure snapshotToDelete is actually a real snapshot
	if _, ok := snapshots[snapshotToDelete]; !ok {
		return nil, nil, fmt.Errorf("error: snapshot '%s' not in list of snapshots", snapshotToDelete)
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
		return deleteObjs, nil, nil
	}

	// Get the next snapshot forward in time after snapshotToDelete
	nextSnapshot := getNextSnapshot(snapshotKeys, snapshotToDelete)

	// loop over each rel path in snapshotToDelete
	for relPath := range snapshots[snapshotToDelete].RelPaths {
		// If relpath in the current snapshot is just a ## deletion marker, we either need to
		// delete it or roll it forward to the next snapshot.
		// - Delete if relPath shows up again in the next snapshot
		// - Else rename and roll it forward
		if snapshots[snapshotToDelete].RelPaths[relPath].IsDeleted {
			deletionMarker := "##" + snapshots[snapshotToDelete].RelPaths[relPath].EncryptedRelPathStripped

			// If rel path is in next snapshot...
			if containsRelPath(snapshots[nextSnapshot], relPath) {
				// If rel path is in next snapshot as a non-deleted entry, we can delete relpath in snapshotToDelete
				deleteObjs = append(deleteObjs, map[string]int64{deletionMarker: 0})
			} else {
				// If relpath is NOT in next snapshot change set at all, then we must rename relpath objects in
				// snapshotToDelete to next snapshot.
				renameObjs = append(renameObjs, renameDeletionMarkerIntoNextSnapshot(snapshots, relPath, snapshotToDelete, nextSnapshot)...)
			}
		} else {
			// relPath in snapshotToDelete is a real file.  Delete it if it shows up in the next
			// snapshot, otherwise roll it forward
			if containsRelPath(snapshots[nextSnapshot], relPath) {
				deleteObjs = append(deleteObjs, snapshots[snapshotToDelete].RelPaths[relPath].EncryptedChunkNames)
			} else {
				renameObjs = append(renameObjs, renameAllChunksIntoNextSnapshot(snapshots, relPath, snapshotToDelete, nextSnapshot)...)
			}
		}
	}

	return deleteObjs, renameObjs, nil
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
