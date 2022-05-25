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
	IsDir         bool
	MTime         int64
	XAttrs        string
	Mode          uint32
	IsSymlink     bool
	SymlinkOrigin string
}

func Backup(ctx context.Context, key []byte, db *database.DB, backupDirPath string, snapshotName string, dirEntId int, objst *objstore.ObjStore, bucket string, showNameOnSuccess bool) error {
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
	// get symlink origin if it's a symlink
	symlinkOrigin, err := getSymlinkOriginIfSymlink(absPath)
	if err != nil {
		log.Printf("error: Backup(): could not get symlink info on '%s'\n", absPath)
		return err
	}
	var isSymlink bool = false
	if symlinkOrigin != "" {
		isSymlink = true
	}
	// get the xattrs if any
	xattrs, err := serializeXAttrsToHex(absPath)
	if err != nil {
		log.Printf("error: Backup(): could not serialize xattrs for '%s'\n", absPath)
		return err
	}
	metadata := dirEntMetadata{
		IsDir:         info.IsDir(),
		MTime:         info.ModTime().Unix(),
		XAttrs:        xattrs,
		Mode:          uint32(info.Mode()),
		SymlinkOrigin: symlinkOrigin,
		IsSymlink:     isSymlink,
	}

	// serialize metadata into buffer with 8-byte length prefix
	buf, err := serializeMetadataStruct(metadata)
	if err != nil {
		log.Printf("error: Backup(): serializeMetadata failed: %v\n", err)
		return err
	}

	// encrypt dir entry name, snapshot name and root dir name
	encryptedRelPath, err := cryptography.EncryptFilename(key, relPath)
	if err != nil {
		log.Printf("error: Backup(): could not encrypt rel path: %v\n", err)
		return err
	}
	encryptedSnapshotName, err := cryptography.EncryptFilename(key, snapshotName)
	if err != nil {
		log.Printf("error: Backup(): could not encrypt snapshot name: %v\n", err)
		return err
	}
	encryptedRootDirName, err := cryptography.EncryptFilename(key, rootDirName)
	if err != nil {
		log.Printf("error: Backup(): could not encrypt root dir name: %v\n", err)
		return err
	}

	// If dir or small file (<ChunkSize bytes), process as single chunk.
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

		// Insert a slash in the middle of encrypted relPath b/c server won't
		// allow path components > 255 characters
		encryptedRelPath = InsertSlashIntoEncRelPath(encryptedRelPath)

		// try to put the (encrypted filename, encrypted snapshot name, encrypted relPath) tuple to obj store
		objName := encryptedRootDirName + "/" + encryptedSnapshotName + "/" + encryptedRelPath
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

		// Insert a slash in the middle of encrypted relPath b/c server won't
		// allow path components > 255 characters
		encryptedRelPath = InsertSlashIntoEncRelPath(encryptedRelPath)

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
			objName := encryptedRootDirName + "/" + encryptedSnapshotName + "/" + encryptedRelPath + fmt.Sprintf(".%03d", i)

			// Upload encrypted readBuf
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

	if showNameOnSuccess {
		fmt.Printf("Backed up %s\n", relPath)
	}

	return nil
}

func InsertSlashIntoEncRelPath(encryptedRelPath string) string {
	encryptedRelPath1 := encryptedRelPath[:len(encryptedRelPath)/2]
	encryptedRelPath2 := encryptedRelPath[len(encryptedRelPath)/2:]
	encryptedRelPath = encryptedRelPath1 + "/" + encryptedRelPath2
	return encryptedRelPath
}

func incrementNonce(nonce []byte) []byte {
	z := new(big.Int)
	z.SetBytes(nonce)
	z.Add(z, big.NewInt(1))
	buf := make([]byte, 12)
	return z.FillBytes(buf)
}

func getSymlinkOriginIfSymlink(pathToInspect string) (string, error) {
	// get the file info
	fileInfo, err := os.Lstat(pathToInspect)
	if err != nil {
		fmt.Printf("error: getSymlinkOriginIfSymlink: %v", err)
		return "", err
	}

	// is it a sym link?
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		linkOrigin, err := os.Readlink(pathToInspect)
		if err != nil {
			fmt.Printf("error: getSymlinkOriginIfSymlink: %v", err)
			return "", err
		}
		return linkOrigin, nil
	} else {
		return "", nil
	}
}
