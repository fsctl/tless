// Package objstorefs is a filesystem-like abstraction on top of the objstore package that
// can determine the full list of files in a snapshot, or the full list of snapshots for a
// backup name group.
package objstorefs

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
)

type RelPath struct {
	EncryptedRelPathStripped string
	DecryptedRelPath         string
	EncryptedChunkNames      map[string]int64
	IsDeleted                bool
}

type Snapshot struct {
	EncryptedName string
	DecryptedName string
	Datetime      time.Time
	RelPaths      map[string]RelPath
}

type BackupDir struct {
	EncryptedName string
	DecryptedName string
	Snapshots     map[string]Snapshot
}

func GetGroupedSnapshots(ctx context.Context, objst *objstore.ObjStore, key []byte, bucket string) (map[string]BackupDir, error) {
	retBackupDirs := make(map[string]BackupDir)

	allObjects, err := objst.GetObjList(ctx, bucket, "")
	if err != nil {
		log.Printf("error: getGroupedSnapshots: GetObjList failed: %v", err)
		return nil, err
	}

	reDot, _ := regexp.Compile(`\.`)

	for objName, size := range allObjects {
		// skip the salt object key
		if objName[:5] == "SALT-" {
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
		snapShotDateTime, err := time.Parse("2006-01-02_15:04:05", snapshotName)
		if err != nil {
			log.Printf("error: getGroupedSnapshots: time.Parse failed on '%s': %v", snapshotName, err)
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
				RelPaths:      make(map[string]RelPath),
			}
		}

		if _, ok := retBackupDirs[backupName].Snapshots[snapshotName].RelPaths[relPath]; !ok {
			// relPath is new, so add a k/v pair for it
			retBackupDirs[backupName].Snapshots[snapshotName].RelPaths[relPath] = RelPath{
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
func ReconstructSnapshotFileList(ctx context.Context, objst *objstore.ObjStore, bucket string, key []byte, backupName string, snapshotName string, partialRestorePath string) (map[string][]string, error) {
	m := make(map[string][]string)

	groupedObjects, err := GetGroupedSnapshots(ctx, objst, key, bucket)
	if err != nil {
		log.Fatalf("Could not get grouped snapshots: %v", err)
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
