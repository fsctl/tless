package snapshots

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/objstorefs"
	"github.com/fsctl/tless/pkg/util"
)

func WriteIndexFile(ctx context.Context, dbLock *sync.Mutex, db *database.DB, objst *objstore.ObjStore, bucket string, key []byte, backupDirName string, snapshotName string) error {
	// Get encrypted snapshot name and backup dir
	encryptedSnapshotName, err := cryptography.EncryptFilename(key, snapshotName)
	if err != nil {
		log.Printf("error: writeIndexFile(): could not encrypt snapshot name (%s): %v\n", snapshotName, err)
		return err
	}
	encryptedBackupDirName, err := cryptography.EncryptFilename(key, backupDirName)
	if err != nil {
		log.Printf("error: createDeletedPathKeyAndPurgeFromDb(): could not encrypt backup dir name (%s): %v\n", backupDirName, err)
		return err
	}

	// Get snapshot time from name and parse into time.Time struct
	// Parse the snapshot datetime string
	snapShotDateTime, err := time.Parse("2006-01-02_15.04.05", snapshotName)
	if err != nil {
		log.Printf("error: writeIndexFile: time.Parse failed on '%s': %v", snapshotName, err)
	}

	// Construct the objstorefs.Snapshot object
	snapshotObj := objstorefs.Snapshot{
		EncryptedName: encryptedSnapshotName,
		DecryptedName: snapshotName,
		Datetime:      snapShotDateTime,
		RelPaths:      make(map[string]objstorefs.CloudRelPath),
	}

	// Add to snapshot:  every journal row's index_entry
	util.LockIf(dbLock)
	indexEntries, err := db.GetAllBackupJournalRowIndexEntries()
	util.UnlockIf(dbLock)
	if err != nil {
		log.Println("error: writeIndexFile: db.GetAllBackupJournalRowIndexEntries() failed")
		return fmt.Errorf("error: writeIndexFile failed")
	}

	for _, indexEntry := range indexEntries {
		// reconstruct the crp object from json
		crp := objstorefs.NewCloudRelPathFromJson(indexEntry)

		// add object to snapshot obj's map
		snapshotObj.RelPaths[crp.DecryptedRelPath] = *crp
	}

	if err = SerializeAndWriteSnapshotObj(&snapshotObj, key, encryptedBackupDirName, encryptedSnapshotName, objst, ctx, bucket); err != nil {
		log.Println("error: writeIndexFile: SerializeAndSaveSnapshotObj failed: ", err)
		return err
	}

	return nil
}

func SerializeAndWriteSnapshotObj(snapshotObj *objstorefs.Snapshot, key []byte, encryptedBackupDirName string, encryptedSnapshotName string, objst *objstore.ObjStore, ctx context.Context, bucket string) error {
	// Serialize fully linked snapshot obj to json bytes
	buf, err := json.Marshal(snapshotObj)
	if err != nil {
		log.Println("error: SerializeAndSaveSnapshotObj: marshal failed: ", err)
		return err
	}

	// lzma compress the json
	compressedBuf, err := util.XZCompress(buf)
	if err != nil {
		log.Println("error: SerializeAndSaveSnapshotObj: xzCompress failed")
		return err
	}

	// encrypt the compressed json
	encCompressedBuf, err := cryptography.EncryptBuffer(key, compressedBuf)
	if err != nil {
		log.Println("error: SerializeAndSaveSnapshotObj: EncryptBuffer: ", err)
		return err
	}

	// form the obj name of index file (enc backup dir / '@' + enc snapshhot name)
	objName := encryptedBackupDirName + "/" + "@" + encryptedSnapshotName

	// put the index object to server
	err = objst.UploadObjFromBuffer(ctx, bucket, objName, encCompressedBuf, objstore.ComputeETag(encCompressedBuf))
	if err != nil {
		log.Println("error: SerializeAndSaveSnapshotObj: UploadObjFromBuffer: ", err)
		return err
	}

	return nil
}
