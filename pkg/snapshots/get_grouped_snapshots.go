package snapshots

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
)

type ChunkExtent struct {
	ChunkName string
	Offset    int64
	Len       int64
}

type CloudRelPath struct {
	RelPath      string
	ChunkExtents []ChunkExtent
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

func (bd BackupDir) GetMostRecentSnapshot() *Snapshot {
	snapshotKeys := make([]string, 0)
	for ssName := range bd.Snapshots {
		snapshotKeys = append(snapshotKeys, ssName)
	}
	sort.Strings(snapshotKeys)

	if len(snapshotKeys) == 0 {
		return nil
	} else {
		mostRecentSnapshotName := snapshotKeys[len(snapshotKeys)-1]
		mostRecentSnapshot := bd.Snapshots[mostRecentSnapshotName]
		return &mostRecentSnapshot
	}
}

type SetInitialGetGroupedSnapshotsProgress func(finished int64, total int64)
type UpdateGetGroupedSnapshotsProgress func(finished int64, total int64)

func GetGroupedSnapshots(ctx context.Context, objst *objstore.ObjStore, key []byte, bucket string, vlog *util.VLog, setInitialGGSProgressFunc SetInitialGetGroupedSnapshotsProgress, updateGGSProgressFunc UpdateGetGroupedSnapshotsProgress) (map[string]BackupDir, error) {
	// setup return map
	ret := make(map[string]BackupDir)

	// get all backup names from cloud (top level paths)
	topLevelObjs, err := objst.GetObjListTopLevel(ctx, bucket, []string{"salt-", "version", "chunks", "keys"})
	if err != nil {
		log.Println("error: GetGroupedSnapshots: objst.GetObjListTopLevel: ", err)
		return nil, err
	}

	// Set initial progress if callback was supplied
	var totalSnapshotIndices int64 = 0
	var finishedSnapshotIndices int64 = 0
	var mObjs map[string]int64 = nil
	if setInitialGGSProgressFunc != nil {
		for _, encBackupName := range topLevelObjs {
			mObjs, err = objst.GetObjList(ctx, bucket, encBackupName+"/@", false, nil)
			if err != nil {
				log.Printf("error: GetGroupedSnapshots: could not get list of snapshot index files for '%s': %v\n", encBackupName, err)
				return nil, err
			}
			for range mObjs {
				totalSnapshotIndices += 1
			}
		}
		setInitialGGSProgressFunc(finishedSnapshotIndices, totalSnapshotIndices)
	}

	// Loop over every backup
	for _, encBackupName := range topLevelObjs {
		//vlog.Printf("GetGroupedSnapshots: working on backup '%s'", encBackupName)

		// get decrypted backup name
		backupName, err := cryptography.DecryptFilename(key, encBackupName)
		if err != nil {
			log.Printf("error: GetGroupedSnapshots: could not decrypt backup dir name (%s): %v\n", encBackupName, err)
			return nil, err
		}

		// add an object to ret map for this backup
		ret[backupName] = BackupDir{
			EncryptedName: encBackupName,
			DecryptedName: backupName,
			Snapshots:     make(map[string]Snapshot),
		}

		// loop over every snapshot index for curr backup
		mObjs, err = objst.GetObjList(ctx, bucket, encBackupName+"/@", false, nil)
		if err != nil {
			log.Printf("error: GetGroupedSnapshots: could not get list of snapshot index files for '%s': %v\n", encBackupName, err)
			return nil, err
		}
		for encObjName := range mObjs {
			// strip off the prefix "encBackupName/@"
			encSsName := strings.TrimPrefix(encObjName, encBackupName+"/@")

			//vlog.Printf("GetGroupedSnapshots: working on snapshot '%s' / @'%s'", encBackupName, encSsName)

			// decrypt snapshot name
			ssName, err := cryptography.DecryptFilename(key, encSsName)
			if err != nil {
				log.Printf("error: GetGroupedSnapshots: could not decrypt snapshot name (%s) - skipping: %v\n", encSsName, err)
				continue
			}

			plaintextIndexFileBuf, err := GetSnapshotIndexFile(ctx, objst, bucket, key, encObjName)
			if err != nil {
				log.Printf("error: GetGroupedSnapshots: could not retrieve snapshot index file (%s) - skipping: %v\n", ssName, err)
				return nil, err
			}

			// reconstruct object hierarchy for this snapshot, placing into BackupDir objects in map
			ssObj, err := UnmarshalSnapshotObj(plaintextIndexFileBuf)
			if err != nil {
				log.Printf("error: GetGroupedSnapshots: could not get snapshot obj for '%s': %v\n", ssName, err)
				return nil, err
			}
			ret[backupName].Snapshots[ssName] = *ssObj

			// update progress if callback supplied
			finishedSnapshotIndices += 1
			if updateGGSProgressFunc != nil {
				updateGGSProgressFunc(finishedSnapshotIndices, totalSnapshotIndices)
			}
		}
	}

	// return map
	return ret, nil
}

func UnmarshalSnapshotObj(indexFileJsonBuf []byte) (*Snapshot, error) {
	ssObj := Snapshot{}
	err := json.Unmarshal(indexFileJsonBuf, &ssObj)
	if err != nil {
		log.Println("error: GetSnapshotObj unmarshal failed: ", err)
		return nil, err
	}
	return &ssObj, nil
}

func GetSnapshotIndexFile(ctx context.Context, objst *objstore.ObjStore, bucket string, key []byte, encObjName string) ([]byte, error) {
	// download actual snapshot file
	buf, err := objst.DownloadObjToBuffer(ctx, bucket, encObjName)
	if err != nil {
		log.Printf("error: getAllSnapshotIndices: could not download snapshot index file '%s': %v\n", encObjName, err)
		return nil, err
	}

	// decrypt and uncompress snapshot file
	plaintextIndexFileBuf, err := decryptAndUncompressIndexFile(key, buf)
	if err != nil {
		log.Printf("error: getAllSnapshotIndices: could not decrypt+uncompress snapshot index file '%s': %v\n", encObjName, err)
		return nil, err
	}

	return plaintextIndexFileBuf, nil
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
