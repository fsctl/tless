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

	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/util"
)

type BackupIdsQueue struct {
	Ids []int
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

type dirEntryInsert struct {
	rootPath           string
	relPath            string
	lastBackupUnixtime int64
}

func Traverse(rootPath string, knownPaths map[string]int, db *database.DB, dbLock *sync.Mutex, backupIdsQueue *BackupIdsQueue, excludePathPrefixes []string) error {
	rootPath = util.StripTrailingSlashes(rootPath)
	rootDirName := filepath.Base(rootPath)

	pendingDirEntryInserts := make([]dirEntryInsert, 0, 10000)

	err := filepath.WalkDir(rootPath, func(path string, dirent fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("error: WalkDirFunc: %v", err)
			return fs.SkipDir
		}
		if isInExcludePathPrefixes(path, excludePathPrefixes) {
			return nil
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
		if dbLock != nil {
			dbLock.Lock()
		}
		hasDirEnt, lastBackupUnix, id, err := db.HasDirEnt(rootDirName, relPath)
		if dbLock != nil {
			dbLock.Unlock()
		}
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
				backupIdsQueue.Ids = append(backupIdsQueue.Ids, id)
			}
		} else {
			pendingDirEntryInserts = append(pendingDirEntryInserts, dirEntryInsert{rootPath: rootDirName, relPath: relPath, lastBackupUnixtime: 0})
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Error in Traverse: %v\n", err)
	}

	// Do all the dir entry inserts, get the ids and enqueue them for backup
	err = doPendingDirEntryInserts(db, dbLock, pendingDirEntryInserts)
	if err != nil {
		log.Printf("error: Traverse: doPendingDirEntryInserts: %v\n", err)
	}
	for _, ins := range pendingDirEntryInserts {
		if dbLock != nil {
			dbLock.Lock()
		}
		_, _, id, err := db.HasDirEnt(ins.rootPath, ins.relPath)
		if dbLock != nil {
			dbLock.Unlock()
		}
		if err != nil {
			log.Printf("error: Traverse: db.HasDirEnt: %v\n", err)
		}
		backupIdsQueue.Ids = append(backupIdsQueue.Ids, id)
	}

	return nil
}

func isInExcludePathPrefixes(path string, excludePathPrefixes []string) bool {
	for _, excludePathPrefix := range excludePathPrefixes {
		if strings.HasPrefix(path, excludePathPrefix) {
			return true
		}
	}
	return false
}

func doPendingDirEntryInserts(db *database.DB, dbLock *sync.Mutex, pendingDirEntryInserts []dirEntryInsert) error {
	if dbLock != nil {
		dbLock.Lock()
	}
	dirEntStmt, err := database.NewInsertDirEntStmt(db)
	if dbLock != nil {
		dbLock.Unlock()
	}
	if err != nil {
		log.Printf("error: doPendingDirEntryInserts: %v", err)
		return err
	}

	for _, ins := range pendingDirEntryInserts {
		if dbLock != nil {
			dbLock.Lock()
		}
		err = dirEntStmt.InsertDirEnt(ins.rootPath, ins.relPath, ins.lastBackupUnixtime)
		if dbLock != nil {
			dbLock.Unlock()
		}
		if err != nil {
			log.Printf("error: doPendingDirEntryInserts: %v", err)
			return err
		}
	}

	if dbLock != nil {
		dbLock.Lock()
	}
	dirEntStmt.Close()
	if dbLock != nil {
		dbLock.Unlock()
	}
	return nil
}
