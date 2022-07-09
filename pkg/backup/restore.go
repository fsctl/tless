package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
)

type DirChmodQueueItem struct {
	AbsPath   string
	FinalMode fs.FileMode
}

func RestoreDirEntry(ctx context.Context, key []byte, restoreIntoDirPath string, crp snapshots.CloudRelPath, rootDirName string, snapshotName string, relPath string, objst *objstore.ObjStore, bucket string, vlog *util.VLog, dirChmodQueue *[]DirChmodQueueItem, uid int, gid int, cc *ChunkCache) error {
	// Strip any trailing slashes on destination path
	restoreIntoDirPath = util.StripTrailingSlashes(restoreIntoDirPath)

	// Retrieve the first chunk of ciphertext
	if len(crp.ChunkExtents) == 0 {
		msg := fmt.Sprintf("error: crp.ChunkExtents has no elements on '%s'", relPath)
		log.Println(msg)
		return fmt.Errorf(msg)
	}
	objName := "chunks/" + crp.ChunkExtents[0].ChunkName
	offset := crp.ChunkExtents[0].Offset
	len := crp.ChunkExtents[0].Len
	plaintextBuf, prevNonce, err := cc.FetchExtentIntoBuffer(ctx, bucket, objName, offset, len)
	if err != nil {
		log.Fatalf("error: RestoreDirEntry: failed to retrieve obj '%s': %v", objName, err)
	}

	// read metadata header from plaintext
	metadataPtr, fileContents, err := deserializeMetadataStruct(plaintextBuf)
	if err != nil {
		log.Printf("error: deserializeMetadataStruct failed: %v\n", err)
		return err
	}

	// create the full path (for dirs) or path up but excluding last component (for symlinks/files)
	// and restore contents (for files)
	if metadataPtr.IsSymlink {
		dir, linkName := filepath.Split(relPath)
		// We can create the dir(s) initially as 0755 b/c they'll get fixed later when we process
		// the dirs' entry themselves.
		err = createFullPath(restoreIntoDirPath, rootDirName, snapshotName, dir, 0755, uid, gid)
		if err != nil {
			log.Printf("error: could not create dir '%s': %v\n",
				filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, dir), err)
			return err
		}

		linkNameAbsPath := filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, dir, linkName)
		if err := os.Symlink(metadataPtr.SymlinkOrigin, linkNameAbsPath); err != nil {
			log.Printf("error: could not create symlink '%s': %v\n", linkNameAbsPath, err)
		}

		// We don't worry about xattrs on symlink entries
	} else if metadataPtr.IsDir {
		// We initially create dirs with mode=0755 (because real mode might be too restrictive for child
		// restores) and enqueue an item to set dir to its correct, final mode after all files restored.
		dirFullPath := filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, relPath)
		err = createFullPath(restoreIntoDirPath, rootDirName, snapshotName, relPath, 0755, uid, gid)
		if err != nil {
			log.Printf("error: could not create dir '%s': %v\n", dirFullPath, err)
			return err
		}
		*dirChmodQueue = append(*dirChmodQueue, DirChmodQueueItem{AbsPath: dirFullPath, FinalMode: fs.FileMode(metadataPtr.Mode)})
		if err = deserializeAndSetXAttrs(filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, relPath), metadataPtr.XAttrs); err != nil {
			log.Printf("error: could not set xattrs on dir '%s': %v\n", dirFullPath, err)
			return err
		}
	} else {
		// create the directory containing this file in case it does not exist yet
		// (We can create dirs initially as 0755 b/c they'll get fixed later when we process
		// the dirs' entries themselves.)
		dir, filename := filepath.Split(relPath)
		err = createFullPath(restoreIntoDirPath, rootDirName, snapshotName, dir, 0755, uid, gid)
		if err != nil {
			log.Printf("error: could not create dir '%s': %v\n",
				filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, dir), err)
			return err
		}

		// Create the file and write first + all subsequent chunks to it
		filenameAbsPath := filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, dir, filename)
		file, err := os.Create(filenameAbsPath)
		if err != nil {
			log.Fatalf("error: RestoreDirEntry: %v", err)
			return err
		}
		defer file.Close()

		// Fix the mode bits on new file
		if err := os.Chmod(filenameAbsPath, fs.FileMode(metadataPtr.Mode)); err != nil {
			log.Fatalf("error:  RestoreDirEntry: could not chmod newly created file (desired mode %#o)", fs.FileMode(metadataPtr.Mode))
			return errors.New("could not chmod file")
		}

		_, err = file.Write(fileContents)
		if err != nil {
			log.Fatalf("RestoreDirEntry: Write() failed: %v", err)
			return err
		}

		// for each of the remaining chunks
		for _, chunkExtent := range crp.ChunkExtents[1:] {
			// download the chunk
			objName := "chunks/" + chunkExtent.ChunkName
			offset := chunkExtent.Offset
			len := chunkExtent.Len
			plaintextBuf, nonce, err := cc.FetchExtentIntoBuffer(ctx, bucket, objName, offset, len)
			if err != nil {
				log.Fatalf("error: RestoreDirEntry: failed to retrieve obj '%s': %v", objName, err)
			}

			// check that the nonce is one larger than prev nonce
			isNonceOneMore := isNonceOneMoreThanPrev(nonce, prevNonce)
			if !isNonceOneMore {
				log.Println("error: RestoreDirEntry: nonce ordering expectation violated, data may have been tampered with (chunk reordering)")
			}

			// append plaintext to file
			_, err = file.Write(plaintextBuf)
			if err != nil {
				log.Fatalf("error: RestoreDirEntry: Write() failed: %v", err)
				return err
			}

			// save nonce as new prevNonce
			prevNonce = nonce
		}

		if err = deserializeAndSetXAttrs(filenameAbsPath, metadataPtr.XAttrs); err != nil {
			log.Printf("error: could not set xattrs on file '%s': %v\n", filenameAbsPath, err)
			return err
		}
		if err := os.Chown(filenameAbsPath, uid, gid); err != nil {
			log.Printf("error: could not chown file '%s' to '%d/%d': %v", filenameAbsPath, uid, gid, err)
			return err
		}
	}

	vlog.Printf("Restored %s\n", filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, relPath))

	return nil
}

// If uid and gid are -1, we don't try to set an owner/group and just let it default to whoever is running the program
func createFullPath(basePath string, backupRootDirName string, snapshotName string, relPath string, mode os.FileMode, uid int, gid int) error {
	joinedDirs := filepath.Join(basePath, backupRootDirName, snapshotName, relPath)

	// If directory does not exist, create it.  If it does exist, correct its mode bits if necessary.
	if info, err := os.Stat(joinedDirs); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(joinedDirs, mode); err != nil {
			log.Printf("error: could not create directories '%s' with mode %#o: %v\n", joinedDirs, mode, err)
			return err
		}
	} else {
		currentMode := info.Mode()
		if currentMode&0x0000ffff != mode {
			if err := os.Chmod(joinedDirs, mode); err != nil {
				log.Printf("error: could not chmod '%s' with mode %#o: %v\n", joinedDirs, mode, err)
				return err
			}
		}
	}

	if uid != -1 && gid != -1 {
		// This one really may fail depending on basepath, so silence the error.
		_ = os.Chown(basePath, uid, gid)
		if err := os.Chown(filepath.Join(basePath, backupRootDirName), uid, gid); err != nil {
			log.Printf("error: os.Chown failed to set uid/gid (%d/%d) on %s", uid, gid, filepath.Join(basePath, backupRootDirName))
		}
		if err := os.Chown(filepath.Join(basePath, backupRootDirName, snapshotName), uid, gid); err != nil {
			log.Printf("error: os.Chown failed to set uid/gid (%d/%d) on %s", uid, gid, filepath.Join(basePath, backupRootDirName, snapshotName))
		}
		if err := os.Chown(filepath.Join(basePath, backupRootDirName, snapshotName, relPath), uid, gid); err != nil {
			log.Printf("error: os.Chown failed to set uid/gid (%d/%d) on %s", uid, gid, filepath.Join(basePath, backupRootDirName, snapshotName, relPath))
		}
	}

	return nil
}

func isNonceOneMoreThanPrev(nonce []byte, prevNonce []byte) bool {
	// set z = prevNonce+1
	z := new(big.Int)
	z.SetBytes(prevNonce)
	z.Add(z, big.NewInt(1))

	// set y = nonce
	y := new(big.Int)
	y.SetBytes(nonce)

	// are they equal?
	return z.Cmp(y) == 0
}

// Filters the relpaths in snapshotObj returning every matching rel path.
// If specificRelPaths is non-nil, only returns rel paths that match exactly an element in the
// specificRelPaths slice.
// If specificRelPaths is nil, but selectedRelPathPrefixes is non-nil, returns every rel path that
// matches any prefix in selectedRelPathPrefixes.
// If both are nil or empty slices, we just return everything.
func FilterRelPaths(snapshotObj *snapshots.Snapshot, specificRelPaths []string, selectedRelPathPrefixes []string) map[string]snapshots.CloudRelPath {
	ret := make(map[string]snapshots.CloudRelPath)
	for relPath := range snapshotObj.RelPaths {
		if len(specificRelPaths) == 0 && len(selectedRelPathPrefixes) == 0 {
			// accept everything
			ret[relPath] = snapshotObj.RelPaths[relPath]
		} else if len(specificRelPaths) > 0 {
			for _, srp := range specificRelPaths {
				//log.Printf("RESTORE: relPath='%s'=='%s'=srp", relPath, srp)
				if relPath == srp {
					// accept it because it matched exactly an element in specific rel paths
					ret[relPath] = snapshotObj.RelPaths[relPath]
				}
			}
		} else if len(selectedRelPathPrefixes) > 0 {
			for _, prefix := range selectedRelPathPrefixes {
				if strings.HasPrefix(relPath, prefix) {
					ret[relPath] = snapshotObj.RelPaths[relPath]
				}
			}
		}
	}
	return ret
}
