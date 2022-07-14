package backup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"os"
	"path/filepath"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
)

var (
	ErrSocket    = fmt.Errorf("the file is a socket")
	ErrDevice    = fmt.Errorf("the file is a device")
	ErrIrregular = fmt.Errorf("the file is an irregular filesystem entry")
	ErrFifo      = fmt.Errorf("the file is a named pipe (FIFO)")
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

func Backup(ctx context.Context, key []byte, rootDirName string, relPath string, backupDirPath string, snapshotName string, objst *objstore.ObjStore, bucket string, vlog *util.VLog, cp *chunkPacker, bjt *database.BackupJournalTask) (chunkExtents []snapshots.ChunkExtent, pendingInChunkPacker bool, err error) {
	chunkExtents = make([]snapshots.ChunkExtent, 0)
	pendingInChunkPacker = false

	backupDirPath = util.StripTrailingSlashes(backupDirPath)

	absPath := filepath.Join(backupDirPath, relPath)

	info, err := os.Lstat(absPath)
	if err != nil {
		log.Printf("error: Backup: could not stat '%s'\n", absPath)
		return nil, false, err
	}

	//
	// filter out socket, FIFO and device files
	//
	if info.Mode()&fs.ModeDevice != 0 {
		return nil, false, ErrDevice
	}
	if info.Mode()&fs.ModeSocket != 0 {
		return nil, false, ErrSocket
	}
	if info.Mode()&fs.ModeIrregular != 0 {
		return nil, false, ErrIrregular
	}
	if info.Mode()&fs.ModeNamedPipe != 0 {
		return nil, false, ErrFifo
	}

	//
	// get the metadata on dirent
	//

	// get symlink origin if it's a symlink
	symlinkOrigin, err := getSymlinkOriginIfSymlink(absPath)
	if err != nil {
		log.Printf("error: Backup: could not get symlink info on '%s'\n", absPath)
		return nil, false, err
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

	//
	// serialize metadata into buffer with 8-byte length prefix
	//
	buf, err := serializeMetadataStruct(metadata)
	if err != nil {
		log.Printf("error: Backup(): serializeMetadata failed: %v\n", err)
		return nil, false, err
	}

	// If dir or small file (<ChunkSize bytes), process as single chunk.
	// If large file (>ChunkSize bytes), apply chunk processing logic.
	size := info.Size() + int64(len(buf))
	if info.IsDir() || (size < ChunkSize) || isSymlink {
		// Contents smaller than ChunkSize; if file just read entire file into
		// rest of buffer after metadata
		if !info.IsDir() && !isSymlink {
			buf, err = cryptography.AppendEntireFileToBuffer(absPath, buf)
			if err != nil {
				log.Printf("error: Backup: AppendEntireFileToBuffer failed: %v\n", err)
				return nil, false, err
			}
		}
		didSucceed := cp.AddDirEntry(relPath, buf, bjt)
		if !didSucceed {
			cp.Complete()
			didSucceedNow := cp.AddDirEntry(relPath, buf, bjt)
			if !didSucceedNow {
				log.Printf("error: Backup: failed to add dir entry to chunk packer even after clearing it (%s)", relPath)
				return nil, false, err
			}
		}
		pendingInChunkPacker = true

		vlog.Printf("Backed up %s (pending in chunkPacker)\n", relPath)

		return chunkExtents, pendingInChunkPacker, nil

	} else {
		// File is larger than ChunkSize

		// Generate initial nonce randomly; subsequent chunks will increment this value
		nonce := make([]byte, 12)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return nil, false, err
		}

		// Open the file for reading
		f, err := os.Open(absPath)
		if err != nil {
			log.Printf("error: could not open '%s': %v", absPath, err)
			return nil, false, err
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
				return nil, false, err
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
				return nil, false, err
			}

			// Generate chunk name
			chunkName := generateRandomChunkName()

			// Save the current chunk extent to return
			chunkExtents = append(chunkExtents, snapshots.ChunkExtent{
				ChunkName: chunkName,
				Offset:    0,
				Len:       int64(len(readBuf)),
			})

			// Upload encrypted readBuf
			objName := "chunks/" + chunkName
			err = objst.UploadObjFromBuffer(ctx, bucket, objName, ciphertextReadBuf, objstore.ComputeETag(ciphertextReadBuf))
			if err != nil {
				log.Printf("error: Backup: failed while backing up file: %v\n", err)
				return nil, false, err
			}

			// For next iteration:  increment nonce
			i += 1
			nonce = incrementNonce(nonce)
		}

		vlog.Printf("Backed up %s (chunkExtents: %v)\n", relPath, chunkExtents)

		return chunkExtents, pendingInChunkPacker, nil
	}
}

func incrementNonce(nonce []byte) []byte {
	z := new(big.Int)
	z.SetBytes(nonce)
	z.Add(z, big.NewInt(1))
	buf := make([]byte, 12)
	return z.FillBytes(buf)
}

func generateRandomChunkName() string {
	randBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, randBytes); err != nil {
		log.Fatalf("error: Backup: rand.Reader.Read failed: %v\n", err)
	}
	chunkName := base64.URLEncoding.EncodeToString([]byte(randBytes))
	return chunkName
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
