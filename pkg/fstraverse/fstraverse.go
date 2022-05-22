// Package fstraverse traverses the local filesystem recursively looking for new,
// changed and deleted files.
package fstraverse

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/fsctl/trustlessbak/pkg/database"
	"github.com/fsctl/trustlessbak/pkg/util"
)

func relativizePath(path string, prefix string) string {
	relPath := strings.TrimPrefix(path, util.StripTrailingSlashes(prefix)+"/")
	return relPath
}

func getMTimeUnix(dirent fs.DirEntry) (int64, error) {
	info, err := dirent.Info()
	if err != nil {
		return 0, fmt.Errorf("error: can't get info: %v", err)
	}
	return info.ModTime().Unix(), nil
}

func Traverse(rootPath string, knownPaths map[string]int, db *database.DB) error {
	rootPath = util.StripTrailingSlashes(rootPath)
	rootDirName := filepath.Base(rootPath)

	err := filepath.WalkDir(rootPath, func(path string, dirent fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath := relativizePath(path, rootPath)
		if relPath == rootPath {
			return nil
		}

		mtimeUnix, err := getMTimeUnix(dirent)
		if err != nil {
			log.Printf("Error: getMTimeUnix: %v", err)
			return nil // log the error and skip this entry
		}

		// // Debugging only
		// if dirent.IsDir() {
		// 	fmt.Printf("DIR> %s (mtime=%v)\n", rootDirName+"/"+relPath, mtimeUnix)
		// } else {
		// 	fmt.Printf("FILE> %s (mtime=%v)\n", rootDirName+"/"+relPath, mtimeUnix)
		// }

		// Remove path from knownPaths so that at the end
		// knownPaths will be a list of all files recently deleted
		delete(knownPaths, rootDirName+"/"+relPath)

		// Is dirent already in our list of previously seen dirents?
		// If so:
		//	 - Check whether mtime is newer than last_backup?
		//   - If yes, enqueue it for backup.
		//   - If no, do nothing with this dirent.
		// If not:
		//   - Insert it into dirents with last_backup set to 0
		//   - Enqueue it for backup.
		hasDirEnt, lastBackupUnix, id, err := db.HasDirEnt(rootDirName, relPath)
		if err != nil {
			log.Printf("Error while searching for %s/%s, skipping this dirent", rootDirName, relPath)
			return nil
		}

		// // Debugging
		// if mtimeUnix > lastBackupUnix {
		// 	log.Printf("CHANGED> %s\n", relPath)
		// }

		if hasDirEnt {
			if mtimeUnix > lastBackupUnix {
				if err = db.EnqueueBackupItem(id); err != nil {
					log.Printf("Could not enqueue backup of %s/%s, skipping", rootDirName, relPath)
					return nil
				}
			}
		} else {
			id, err = db.InsertDirEnt(rootDirName, relPath, 0)
			if err != nil {
				log.Printf("Could not insert %s/%s, so skipping enqueue", rootDirName, relPath)
				return nil
			}

			if err = db.EnqueueBackupItem(id); err != nil {
				log.Printf("Could not enqueue backup of %s/%s, skipping", rootDirName, relPath)
				return nil
			}
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Error in Traverse: %v\n", err)
	}

	// Any paths remaining in knownPaths represent files that have been deleted locally.
	// Enqueue a work item to delete them from the cloud as well.
	for missingPath := range knownPaths {
		if err = db.EnqueueDeleteItem(missingPath); err != nil {
			log.Printf("Could not enqueue delete of %s, skipping", missingPath)
			return nil
		}
	}

	return nil
}
