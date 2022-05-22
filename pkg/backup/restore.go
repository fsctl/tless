package backup

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"math/big"
	"os"
	"path/filepath"

	"github.com/fsctl/trustlessbak/pkg/cryptography"
	"github.com/fsctl/trustlessbak/pkg/objstore"
	"github.com/fsctl/trustlessbak/pkg/util"
)

func RestoreDirEntry(ctx context.Context, key []byte, restoreIntoDirPath string, encryptedRootDirName string, encryptedRelPath string, objst *objstore.ObjStore, bucket string) error {
	// Strip any trailing slashes on destination path
	restoreIntoDirPath = util.StripTrailingSlashes(restoreIntoDirPath)

	// Retrieve the ciphertext
	ciphertextBuf, err := objst.DownloadObjToBuffer(ctx, bucket, encryptedRootDirName+"/"+encryptedRelPath)
	if err != nil {
		log.Fatalf("Error retrieving file: %v", err)
	}

	// decrypt the root dir name and relative path
	decryptedRootDirName, err := cryptography.DecryptFilename(key, encryptedRootDirName)
	if err != nil {
		log.Fatalf("error: could not decrypt root dir name: %v", err)
	}
	decryptedRelPath, err := cryptography.DecryptFilename(key, encryptedRelPath)
	if err != nil {
		log.Fatalf("error: could not decrypt root dir name: %v", err)
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

	// create the full path (for dirs) or path up but excluding last component (for files)
	// and restore contents (for files)
	if metadataPtr.IsDir {
		// TODO: fix hardcoded mode
		err = createFullPath(restoreIntoDirPath, decryptedRootDirName, decryptedRelPath, 0755)
		if err != nil {
			log.Printf("error: could not create dir '%s': %v\n",
				filepath.Join(restoreIntoDirPath, decryptedRootDirName, decryptedRelPath), err)
			return err
		}
	} else {
		dir, filename := filepath.Split(decryptedRelPath)
		// TODO: fix hardcoded mode
		err = createFullPath(restoreIntoDirPath, decryptedRootDirName, dir, 0755)
		if err != nil {
			log.Printf("error: could not create dir '%s': %v\n",
				filepath.Join(restoreIntoDirPath, decryptedRootDirName, dir), err)
			return err
		}

		filenameAbsPath := filepath.Join(restoreIntoDirPath, decryptedRootDirName, dir, filename)
		err = writeBufferToFile(fileContents, filenameAbsPath)
		if err != nil {
			log.Printf("error: could not write file '%s': %v\n", filenameAbsPath, err)
			return err
		}
	}
	return nil
}

func createFullPath(basePath string, backupRootDirName string, relPath string, mode os.FileMode) error {
	joinedDirs := filepath.Join(basePath, backupRootDirName, relPath)

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

func writeBufferToFile(buf []byte, path string) error {
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

func RestoreDirEntryFromChunks(ctx context.Context, key []byte, restoreIntoDirPath string, encryptedRootDirName string, encryptedRelPathChunks []string, objst *objstore.ObjStore, bucket string) error {
	// Strip any trailing slashes on destination path
	restoreIntoDirPath = util.StripTrailingSlashes(restoreIntoDirPath)

	// decrypt the root dir name and relative path
	decryptedRootDirName, err := cryptography.DecryptFilename(key, encryptedRootDirName)
	if err != nil {
		log.Fatalf("error: could not decrypt root dir name: %v", err)
	}
	firstRelPathChunk := encryptedRelPathChunks[0]
	decryptedRelPath, err := cryptography.DecryptFilename(key, firstRelPathChunk[0:len(firstRelPathChunk)-4])
	if err != nil {
		log.Fatalf("error: could not decrypt root dir name: %v", err)
	}

	// download the first chunk
	ciphertextFirstChunkBuf, err := objst.DownloadObjToBuffer(ctx, bucket, encryptedRootDirName+"/"+firstRelPathChunk)
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
	dir, filename := filepath.Split(decryptedRelPath)
	err = createFullPath(restoreIntoDirPath, decryptedRootDirName, dir, 0755) // TODO: fix hardcoded mode
	if err != nil {
		log.Printf("error: could not create dir '%s': %v\n",
			filepath.Join(restoreIntoDirPath, decryptedRootDirName, dir), err)
		return err
	}

	// create the file and write first chunk to it
	_ = metadataPtr // not currently using metadata to create file
	filenameAbsPath := filepath.Join(restoreIntoDirPath, decryptedRootDirName, dir, filename)
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
	for _, encryptedRelPathChunk := range encryptedRelPathChunks[1:] {
		// download the chunk
		ciphertextNextChunkBuf, err := objst.DownloadObjToBuffer(ctx, bucket, encryptedRootDirName+"/"+encryptedRelPathChunk)
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

	return nil
}
