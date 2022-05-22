package backup

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"

	"github.com/fsctl/trustlessbak/pkg/cryptography"
	"github.com/fsctl/trustlessbak/pkg/database"
	"github.com/fsctl/trustlessbak/pkg/objstore"
	"github.com/fsctl/trustlessbak/pkg/util"
)

const (
	ChunkSize int64 = 134217728 // 128mb
)

type dirEntMetadata struct {
	IsDir  bool
	MTime  int64
	XAttrs string
}

func Backup(ctx context.Context, key []byte, db *database.DB, backupDirPath string, dirEntId int, objst *objstore.ObjStore, bucket string) error {
	// strip any trailing slashes on destination path
	backupDirPath = util.StripTrailingSlashes(backupDirPath)

	// get the path for the dirent
	rootDirName, relPath, err := db.GetDirEntPaths(dirEntId)
	if err != nil {
		log.Printf("error: Backup(): could not get dirent id '%d'\n", dirEntId)
		return err
	}
	absPath := filepath.Join(backupDirPath, relPath)

	// get the os.stat metadata on file
	info, err := os.Stat(absPath)
	if err != nil {
		log.Printf("error: Backup(): could not stat '%s'\n", absPath)
		return err
	}
	metadata := dirEntMetadata{
		IsDir:  info.IsDir(),
		MTime:  info.ModTime().Unix(),
		XAttrs: "(not implemented yet)",
	}

	// serialize metadata into buffer with 8-byte length prefix
	buf, err := serializeMetadataStruct(metadata)
	if err != nil {
		log.Printf("error: Backup(): serializeMetadata failed: %v\n", err)
		return err
	}

	// encrypt dir entry name and root dir name
	encryptedRelPath, err := cryptography.EncryptFilename(key, relPath)
	if err != nil {
		log.Printf("error: Backup(): could not encrypt rel path: %v\n", err)
		return err
	}
	encryptedRootDirName, err := cryptography.EncryptFilename(key, rootDirName)
	if err != nil {
		log.Printf("error: Backup(): could not encrypt root dir name: %v\n", err)
		return err
	}

	// If dir or small file (<ChhunkSize bytes), process as single chunk.
	// If large file (>ChunkSize bytes), apply chunk processing logic.
	if info.IsDir() || info.Size() < ChunkSize {
		// Contents smaller than ChunkSize; if file just read entire file into
		// rest of buffer after metadata
		if !info.IsDir() {
			buf, err = cryptography.AppendEntireFileToBuffer(absPath, buf)
			if err != nil {
				log.Printf("error: Backup(): AppendEntireFileToBuffer failed: %v\n", err)
				return err
			}
		}

		// encrypt buffer
		ciphertextBuf, err := cryptography.EncryptBuffer(key, buf)
		if err != nil {
			log.Printf("error: Backup(): EncryptBuffer failed: %v\n", err)
			return err
		}

		// try to put the (encrypted filename, encrypted buffer) pair to obj store
		objName := encryptedRootDirName + "/" + encryptedRelPath
		if len(objName) > 320 { // 320 seems to be the magic key length limit for minio
			log.Printf("WARN: skipping path b/c too long: '%s' (server max is 320 chars, len(objName) is %d chars)", relPath, len(objName))
			return fmt.Errorf("skipped path '%s'", relPath)
		}
		err = objst.UploadObjFromBuffer(ctx, bucket, objName, ciphertextBuf)
		if err != nil {
			log.Printf("error: Backup(): backing up file: %v\n", err)
			return err
		}

	} else {
		// File is larger than ChunkSize

		// Generate initial nonce randomly; subsequent chunks will increment this value
		nonce := make([]byte, 12)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return err
		}

		// Open the file for reading
		f, err := os.Open(absPath)
		if err != nil {
			log.Printf("error: could not open '%s': %v", absPath, err)
			return err
		}
		defer f.Close()

		// Loop until last partial chunk is processed
		i := 0
		for {
			// Read next ChunkSize bytes
			readBuf := make([]byte, ChunkSize)
			bytesRead, err := f.Read(readBuf)
			if errors.Is(err, io.EOF) && bytesRead == 0 {
				break
			}
			if err != nil {
				log.Printf("error: could not read from '%s': %v", absPath, err)
				return err
			}
			if bytesRead < len(readBuf) {
				readBuf = readBuf[0:bytesRead]
			}

			// On first iterationn only: prepend buf (containing header) to readBuf
			if i == 0 {
				readBuf = append(buf, readBuf...)
			}

			// Encrypt readBuf with key and current nonce
			ciphertextReadBuf, err := cryptography.EncryptBufferWithNonce(key, readBuf, nonce)
			if err != nil {
				log.Printf("error: could not encrypt buffer: %v", err)
				return err
			}

			// Form object name with .N appended
			objName := encryptedRootDirName + "/" + encryptedRelPath + fmt.Sprintf(".%03d", i)

			// Upload encrypted readBuf
			if len(objName) > 320 { // 320 seems to be the magic key length limit for minio
				log.Printf("WARN: skipping path b/c too long: '%s' (server max is 320 chars, len(objName) is %d chars)", relPath, len(objName))
				return fmt.Errorf("skipped path '%s'", relPath)
			}
			//TODO:  send this md5 as E-Tag, it works for minio.
			//But note that to compute it on the client, you need to know the nonce, which is
			//bad for the case when you're just checking local file against what's on the server.
			//Would need to retrieve first 24 bytes of server file to compute ciphertext correctly.
			//fmt.Printf("MD5 of ciphertext buffer: %x (%d bytes)\n", md5.Sum(ciphertextReadBuf), len(ciphertextReadBuf))
			err = objst.UploadObjFromBuffer(ctx, bucket, objName, ciphertextReadBuf)
			if err != nil {
				log.Printf("error: Backup(): backing up file: %v\n", err)
				return err
			}

			// For next iteration:  increment counter, increment nonce
			i += 1
			nonce = incrementNonce(nonce)
		}
	}

	log.Printf("Backed up %s\n", relPath)

	return nil
}

func incrementNonce(nonce []byte) []byte {
	z := new(big.Int)
	z.SetBytes(nonce)
	z.Add(z, big.NewInt(1))
	buf := make([]byte, 12)
	return z.FillBytes(buf)
}
