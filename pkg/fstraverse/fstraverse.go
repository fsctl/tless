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
	"time"

	"github.com/aybabtme/uniplot/histogram"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/util"
)

type BackupIdsQueueItem struct {
	Id         int
	ChangeType database.ChangeType
}

type BackupIdsQueue struct {
	Items []BackupIdsQueueItem
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

func Traverse(rootPath string, knownPaths map[string]int, db *database.DB, dbLock *sync.Mutex, backupIdsQueue *BackupIdsQueue, excludePathPrefixes []string) ([]util.ReportedEvent, error) {
	rootPath = util.StripTrailingSlashes(rootPath)
	rootDirName := filepath.Base(rootPath)

	pendingDirEntryInserts := make([]dirEntryInsert, 0, 10000)

	reportedEvents := make([]util.ReportedEvent, 0)

	// For sizes histogram, count, mean and median
	fileSizesMb := make([]float64, 0)
	var filesCnt int64 = 0
	var dirsCnt int64 = 0

	err := filepath.WalkDir(rootPath, func(path string, dirent fs.DirEntry, err error) error {
		if isInExcludePathPrefixes(path, excludePathPrefixes) {
			return nil
		}
		if err != nil {
			log.Println("error: WalkDirFunc: ", err)

			// We can get here in two ways:
			// 1. This is a dir and readdirnames failed on it, in which case dirent is set to describe this dir (at path).
			// 2. This is any directory entry and os.Lstat failed on it, in which case err is set to Lstat's error.
			// We're mainly concerned with whole directories we can't traverse, so we'll look for signs of that and if so
			// queue it as a serious error to report to the user at the end.

			if dirent.IsDir() && strings.Contains(err.Error(), "operation not permitted") {
				reportedEvents = append(reportedEvents, util.ReportedEvent{
					Kind:     util.ERR_OP_NOT_PERMITTED,
					Path:     path,
					IsDir:    true,
					Datetime: time.Now().Unix(),
				})
			}

			return fs.SkipDir
		}

		relPath := relativizePath(path, rootPath)
		if relPath == rootPath {
			return nil
		}

		mtimeUnix, err := getMTimeUnix(dirent)
		if err != nil {
			log.Printf("error: getMTimeUnix: %v", err)
			return nil // log the error and skip this entry
		}

		// For summary statistics only
		finfo, err := dirent.Info()
		if err == nil {
			size := finfo.Size()
			sizeMb := float64(size) / float64(1024*1024)
			fileSizesMb = append(fileSizesMb, sizeMb)
		}
		if dirent.IsDir() {
			dirsCnt += 1
			//fmt.Printf("DIR> %s (mtime=%v)\n", rootDirName+"/"+relPath, mtimeUnix)
		} else {
			filesCnt += 1
			//fmt.Printf("FILE> %s (mtime=%v)\n", rootDirName+"/"+relPath, mtimeUnix)
		}
		// end - summary stats

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
				backupIdsQueue.Items = append(backupIdsQueue.Items, BackupIdsQueueItem{
					Id:         id,
					ChangeType: database.Updated,
				})
			} else {
				backupIdsQueue.Items = append(backupIdsQueue.Items, BackupIdsQueueItem{
					Id:         id,
					ChangeType: database.Unchanged,
				})
			}
		} else {
			pendingDirEntryInserts = append(pendingDirEntryInserts, dirEntryInsert{rootPath: rootDirName, relPath: relPath, lastBackupUnixtime: 0})
		}

		return nil
	})
	if err != nil {
		log.Printf("error: during traverse: %v\n", err)
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
		backupIdsQueue.Items = append(backupIdsQueue.Items, BackupIdsQueueItem{
			Id:         id,
			ChangeType: database.Updated,
		})
	}

	// print summary statistics
	p := message.NewPrinter(language.English)
	_ = p
	//s := p.Sprintf("%d files, %d dirs", filesCnt, dirsCnt)
	//log.Println("~~~ path traversal summary stats ~~~")
	//log.Println(s)
	//log.Printf("\nHistogram of size in Mb (%d files):\n", len(fileSizesMb))
	hist := histogram.Hist(30, fileSizesMb)
	_ = hist
	//if err := histogram.Fprint(os.Stdout, hist, histogram.Linear(80)); err != nil {
	//	log.Println("error: uniplot.Fprint: ", err)
	//}
	//log.Println("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
	// end - summary statistics

	return reportedEvents, nil
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
