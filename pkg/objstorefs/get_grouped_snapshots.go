package objstorefs

import (
	"context"
	"encoding/json"
	"log"
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

func GetGroupedSnapshots2(ctx context.Context, objst *objstore.ObjStore, key []byte, bucket string, vlog *util.VLog) (map[string]BackupDir, error) {
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
		indexFiles, err := getAllSnapshotIndexFiles(ctx, objst, key, encBackupName, bucket, vlog)
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
func getAllSnapshotIndexFiles(ctx context.Context, objst *objstore.ObjStore, key []byte, encBackupName string, bucket string, vlog *util.VLog) (map[string][]byte, error) {
	ret := make(map[string][]byte)

	mObjs, err := objst.GetObjList2(ctx, bucket, encBackupName+"/@", false, nil)
	if err != nil {
		log.Printf("error: getAllSnapshotIndices: could not get list of snapshot index files for '%s': %v\n", encBackupName, err)
		return nil, err
	}

	for encObjName := range mObjs {
		vlog.Printf("Working on index file:  %s", encObjName)

		// strip off the prefix "encBackupName/@"
		encSsName := strings.TrimPrefix(encObjName, encBackupName+"/@")

		// decrypt snapshot name
		ssName, err := cryptography.DecryptFilename(key, encSsName)
		if err != nil {
			log.Printf("error: getAllSnapshotIndexFiles: could not decrypt snapshot name (%s): %v\n", encSsName, err)
			return nil, err
		}
		vlog.Printf("Which is for snapshot:  %s", ssName)

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
