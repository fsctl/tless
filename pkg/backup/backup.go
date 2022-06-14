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

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
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

func Backup(ctx context.Context, key []byte, rootDirName string, relPath string, backupDirPath string, snapshotName string, objst *objstore.ObjStore, bucket string, showNameOnSuccess bool) (encryptedRelPath string, encryptedChunks map[string]int64, err error) {
	encryptedChunks = make(map[string]int64)

	// strip any trailing slashes on destination path
	backupDirPath = util.StripTrailingSlashes(backupDirPath)

	// get the path for the dirent
	absPath := filepath.Join(backupDirPath, relPath)

	// get the os.stat metadata on file
	info, err := os.Lstat(absPath)
	if err != nil {
		log.Printf("error: Backup(): could not stat '%s'\n", absPath)
		return "", nil, err
	}
	// get symlink origin if it's a symlink
	symlinkOrigin, err := getSymlinkOriginIfSymlink(absPath)
	if err != nil {
		log.Printf("error: Backup(): could not get symlink info on '%s'\n", absPath)
		return "", nil, err
	}
	var isSymlink bool = false
	if symlinkOrigin != "" {
		isSymlink = true
	}
	// get the xattrs if any
	xattrs, err := serializeXAttrsToHex(absPath)
	if err != nil {
		// fs may validly not support xattrs, so if serialization fails just set xattrs to blank
		xattrs = ""
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
		return "", nil, err
	}

	// encrypt dir entry name, snapshot name and root dir name
	encryptedRelPath, err = cryptography.EncryptFilename(key, relPath)
	if err != nil {
		log.Printf("error: Backup(): could not encrypt rel path: %v\n", err)
		return "", nil, err
	}
	encryptedSnapshotName, err := cryptography.EncryptFilename(key, snapshotName)
	if err != nil {
		log.Printf("error: Backup(): could not encrypt snapshot name: %v\n", err)
		return "", nil, err
	}
	encryptedRootDirName, err := cryptography.EncryptFilename(key, rootDirName)
	if err != nil {
		log.Printf("error: Backup(): could not encrypt root dir name: %v\n", err)
		return "", nil, err
	}

	// If dir or small file (<ChunkSize bytes), process as single chunk.
	// If large file (>ChunkSize bytes), apply chunk processing logic.
	if info.IsDir() || info.Size() < ChunkSize {
		// Contents smaller than ChunkSize; if file just read entire file into
		// rest of buffer after metadata
		if !info.IsDir() && !isSymlink {
			buf, err = cryptography.AppendEntireFileToBuffer(absPath, buf)
			if err != nil {
				log.Printf("error: Backup(): AppendEntireFileToBuffer failed: %v\n", err)
				return "", nil, err
			}
		}

		// encrypt buffer
		ciphertextBuf, err := cryptography.EncryptBuffer(key, buf)
		if err != nil {
			log.Printf("error: Backup(): EncryptBuffer failed: %v\n", err)
			return "", nil, err
		}

		// Insert a slash in the middle of encrypted relPath b/c server won't
		// allow path components > 255 characters
		encryptedRelPath = InsertSlashIntoEncRelPath(encryptedRelPath)

		// Save the single chunk name to return
		encryptedChunks[encryptedRelPath] = int64(len(ciphertextBuf))

		// try to put the (encrypted filename, encrypted snapshot name, encrypted relPath) tuple to obj store
		objName := encryptedRootDirName + "/" + encryptedSnapshotName + "/" + encryptedRelPath
		err = objst.UploadObjFromBuffer(ctx, bucket, objName, ciphertextBuf, objstore.ComputeETag(ciphertextBuf))
		if err != nil {
			log.Printf("error: Backup(): backing up file: %v\n", err)
			return "", nil, err
		}

	} else {
		// File is larger than ChunkSize

		// Generate initial nonce randomly; subsequent chunks will increment this value
		nonce := make([]byte, 12)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return "", nil, err
		}

		// Open the file for reading
		f, err := os.Open(absPath)
		if err != nil {
			log.Printf("error: could not open '%s': %v", absPath, err)
			return "", nil, err
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
				return "", nil, err
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
				return "", nil, err
			}

			// Form object name with .N appended
			objName := encryptedRootDirName + "/" + encryptedSnapshotName + "/" + encryptedRelPath + fmt.Sprintf(".%03d", i)

			// Save the current chunk name to return
			encryptedChunks[encryptedRelPath+fmt.Sprintf(".%03d", i)] = int64(len(ciphertextReadBuf))

			// Upload encrypted readBuf
			err = objst.UploadObjFromBuffer(ctx, bucket, objName, ciphertextReadBuf, objstore.ComputeETag(ciphertextReadBuf))
			if err != nil {
				log.Printf("error: Backup(): backing up file: %v\n", err)
				return "", nil, err
			}

			// For next iteration:  increment counter, increment nonce
			i += 1
			nonce = incrementNonce(nonce)
		}
	}

	if showNameOnSuccess {
		fmt.Printf("Backed up %s\n", relPath)
	}

	return encryptedRelPath, encryptedChunks, nil
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
