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

	"github.com/fsctl/trustlessbak/pkg/cryptography"
	"github.com/fsctl/trustlessbak/pkg/objstore"
	"github.com/fsctl/trustlessbak/pkg/util"
)

func RestoreDirEntry(ctx context.Context, key []byte, restoreIntoDirPath string, objName string, rootDirName string, snapshotName string, relPath string, objst *objstore.ObjStore, bucket string, printOnSuccess bool) error {
	// Strip any trailing slashes on destination path
	restoreIntoDirPath = util.StripTrailingSlashes(restoreIntoDirPath)

	// Retrieve the ciphertext
	ciphertextBuf, err := objst.DownloadObjToBuffer(ctx, bucket, objName)
	if err != nil {
		log.Fatalf("Error retrieving file: %v", err)
	}

	// decrypt the ciphertext
	plaintextBuf, err := cryptography.DecryptBuffer(key, ciphertextBuf)
	if err != nil {
		log.Printf("error: DecryptBuffer failed: %v\n", err)
		return err
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
		err = createFullPath(restoreIntoDirPath, rootDirName, snapshotName, dir, 0755)
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
		err = createFullPath(restoreIntoDirPath, rootDirName, snapshotName, relPath, fs.FileMode(metadataPtr.Mode))
		if err != nil {
			log.Printf("error: could not create dir '%s': %v\n",
				filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, relPath), err)
			return err
		}
		if err = deserializeAndSetXAttrs(filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, relPath), metadataPtr.XAttrs); err != nil {
			log.Printf("error: could not set xattrs on dir '%s': %v\n",
				filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, relPath), err)
			return err
		}
	} else {
		dir, filename := filepath.Split(relPath)
		// We can create the dir(s) initially as 0755 b/c they'll get fixed later when we process
		// the dirs' entry themselves.
		err = createFullPath(restoreIntoDirPath, rootDirName, snapshotName, dir, 0755)
		if err != nil {
			log.Printf("error: could not create dir '%s': %v\n",
				filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, dir), err)
			return err
		}

		filenameAbsPath := filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, dir, filename)
		err = writeBufferToFile(fileContents, filenameAbsPath, fs.FileMode(metadataPtr.Mode))
		if err != nil {
			log.Printf("error: could not write file '%s': %v\n", filenameAbsPath, err)
			return err
		}
		if err = deserializeAndSetXAttrs(filenameAbsPath, metadataPtr.XAttrs); err != nil {
			log.Printf("error: could not set xattrs on file '%s': %v\n", filenameAbsPath, err)
			return err
		}
	}

	if printOnSuccess {
		fmt.Printf("Restored %s\n", filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, relPath))
	}

	return nil
}

func createFullPath(basePath string, backupRootDirName string, snapshotName string, relPath string, mode os.FileMode) error {
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
	return nil
}

func writeBufferToFile(buf []byte, path string, mode fs.FileMode) error {
	file, err := os.Create(path)
	if err != nil {
		log.Fatalf("writeEntireFile: %v", err)
		return err
	}
	defer file.Close()

	n, err := file.Write(buf)
	if err != nil {
		log.Fatalf("writeEntireFile: Write() failed: %v", err)
		return err
	}
	if n != len(buf) {
		log.Fatalf("writeEntireFile: wrote %d bytes but buffer is %d bytes", n, len(buf))
		return errors.New("wrote wrong number of bytes")
	}

	// Fix the mode bits
	if err := os.Chmod(path, mode); err != nil {
		log.Fatalf("writeEntireFile: could not chmod newly created file (desired mode %#o)", mode)
		return errors.New("could not chmod file")
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

func RestoreDirEntryFromChunks(ctx context.Context, key []byte, restoreIntoDirPath string, objNames []string, rootDirName string, snapshotName string, relPath string, objst *objstore.ObjStore, bucket string, printOnSuccess bool) error {
	// Strip any trailing slashes on destination path
	restoreIntoDirPath = util.StripTrailingSlashes(restoreIntoDirPath)

	// download the first chunk
	ciphertextFirstChunkBuf, err := objst.DownloadObjToBuffer(ctx, bucket, objNames[0])
	if err != nil {
		log.Fatalf("Error retrieving file: %v", err)
	}

	// decrypt the first chunk, saving nonce as prevNonce for comparison with later chunks
	plaintextFirstChunkBuf, prevNonce, err := cryptography.DecryptBufferReturningNonce(key, ciphertextFirstChunkBuf)
	if err != nil {
		log.Printf("error: DecryptBufferReturningNonce failed: %v\n", err)
		return err
	}

	// extract the first chunk's metadata from plaintext
	metadataPtr, fileContents, err := deserializeMetadataStruct(plaintextFirstChunkBuf)
	if err != nil {
		log.Printf("error: deserializeMetadataStruct failed: %v\n", err)
		return err
	}

	// create the directory containing this file in case it does not exist yet
	dir, filename := filepath.Split(relPath)
	err = createFullPath(restoreIntoDirPath, rootDirName, snapshotName, dir, 0755) // TODO: fix hardcoded mode
	if err != nil {
		log.Printf("error: could not create dir '%s': %v\n",
			filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, dir), err)
		return err
	}

	// create the file and write first chunk to it
	_ = metadataPtr // not currently using metadata to create file
	filenameAbsPath := filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, dir, filename)
	file, err := os.Create(filenameAbsPath)
	if err != nil {
		log.Fatalf("RestoreDirEntryFromChunks: %v", err)
		return err
	}
	defer file.Close()

	_, err = file.Write(fileContents)
	if err != nil {
		log.Fatalf("RestoreDirEntryFromChunks: Write() failed: %v", err)
		return err
	}

	// for each of the remaining chunks
	for _, objName := range objNames[1:] {
		// download the chunk
		ciphertextNextChunkBuf, err := objst.DownloadObjToBuffer(ctx, bucket, objName)
		if err != nil {
			log.Fatalf("Error retrieving file: %v", err)
		}

		// decrypt the chunk
		plaintextNextChunkBuf, nonce, err := cryptography.DecryptBufferReturningNonce(key, ciphertextNextChunkBuf)
		if err != nil {
			log.Printf("error: DecryptBufferReturningNonce failed: %v\n", err)
			return err
		}

		// check that the nonce is one larger than prev nonce
		isNonceOneMore := isNonceOneMoreThanPrev(nonce, prevNonce)
		if !isNonceOneMore {
			log.Println("error: nonce ordering expectation violated, data may have been tampered with (chunk reordering)")
		}

		// append plaintext to file
		_, err = file.Write(plaintextNextChunkBuf)
		if err != nil {
			log.Fatalf("RestoreDirEntryFromChunks: Write() failed: %v", err)
			return err
		}

		// save nonce as new prevNonce
		prevNonce = nonce
	}

	if err = deserializeAndSetXAttrs(filenameAbsPath, metadataPtr.XAttrs); err != nil {
		log.Printf("error: could not set xattrs on multi-chunk file '%s': %v\n", filenameAbsPath, err)
		return err
	}

	if printOnSuccess {
		fmt.Printf("Restored %s\n", filepath.Join(restoreIntoDirPath, rootDirName, snapshotName, relPath))
	}

	return nil
}
