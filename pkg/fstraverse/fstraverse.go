// Package fstraverse traverses the local filesystem recursively looking for new,
// changed and deleted files.
package fstraverse

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsctl/trustlessbak/pkg/database"
	"github.com/fsctl/trustlessbak/pkg/util"
)

type BackupIdsQueue struct {
	Ids  []int
	Lock sync.Mutex
}

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

func Traverse(rootPath string, knownPaths map[string]int, db *database.DB, backupIdsQueue *BackupIdsQueue) error {
	rootPath = util.StripTrailingSlashes(rootPath)
	rootDirName := filepath.Base(rootPath)

	dirEntStmt, err := database.NewInsertDirEntStmt(db)
	if err != nil {
		log.Printf("error: NewInsertDirEntStmt: %v", err)
		return err
	}

	err = filepath.WalkDir(rootPath, func(path string, dirent fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("error: WalkDirFunc: %v", err)
			return fs.SkipDir
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
				backupIdsQueue.Lock.Lock()
				backupIdsQueue.Ids = append(backupIdsQueue.Ids, id)
				backupIdsQueue.Lock.Unlock()
			}
		} else {
			id, err = dirEntStmt.InsertDirEnt(rootDirName, relPath, 0)
			if err != nil {
				log.Printf("Could not insert %s/%s, so skipping enqueue", rootDirName, relPath)
				return nil
			}
			backupIdsQueue.Lock.Lock()
			backupIdsQueue.Ids = append(backupIdsQueue.Ids, id)
			backupIdsQueue.Lock.Unlock()
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Error in Traverse: %v\n", err)
	}
	dirEntStmt.Close()
	return nil
}
