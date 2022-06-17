package backup

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
)

const (
	MaxCacheSizeOnDisk int64 = 1 * 1024 * 1024 * 1024 // 1 Gb
	//MaxCacheSizeOnDisk int64  = 512 * 1024 * 1024 // 512 Mb
	CacheDirectory string = "/tmp/tless-cache"
)

type CachedChunk struct {
	absPath          string
	size             int64
	lastUsedUnixTime int64
	plaintext        []byte
	nonce            []byte
}

type CacheStatistics struct {
	hits  int
	total int
}

type ChunkCache struct {
	objst  *objstore.ObjStore
	key    []byte
	vlog   *util.VLog
	chunks map[string]CachedChunk
	stats  *CacheStatistics
}

func NewChunkCache(objst *objstore.ObjStore, key []byte, vlog *util.VLog) *ChunkCache {
	// Construct cache obj
	cc := &ChunkCache{
		objst:  objst,
		key:    key,
		vlog:   vlog,
		chunks: make(map[string]CachedChunk, 0),
		stats: &CacheStatistics{
			hits:  0,
			total: 0,
		},
	}

	// Make cache directory if it doesn't already exist
	if err := os.MkdirAll(CacheDirectory, 0755); err != nil {
		log.Printf("error: NewChunkCache: cannot create cache directory '%s': %v", CacheDirectory, err)
		return cc
	}

	// Iterate over cache directory in case we already have objects stored in it
	err := filepath.WalkDir(CacheDirectory, func(path string, dirent fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("error: NewChunkCache: WalkDirFunc: %v (skipping)", err)
			return fs.SkipDir
		}

		finfo, err := dirent.Info()
		if err != nil {
			log.Println("error: NewChunkCache: WalkDirFunc: dirent.Info failed: ", err)
			return err
		}
		size := finfo.Size()
		objName := filepath.Base(path)

		if size > 0 && objName != "" && objName != filepath.Base(CacheDirectory) {
			// Read the ciphertext on disk and decrypt it into memory
			ciphertextChunkBuf, err := readEntireFile(objName)
			if err != nil {
				log.Printf("error: NewChunkCache: WalkDirFunc: failed to read file cache of obj '%s': %v", objName, err)
				return err
			}
			plaintextBuf, nonce, err := cryptography.DecryptBufferReturningNonce(key, ciphertextChunkBuf)
			if err != nil {
				log.Printf("error: NewChunkCache: WalkDirFunc: DecryptBufferReturningNonce failed: %v\n", err)
				return err
			}

			cached := CachedChunk{
				absPath:          filepath.Join(CacheDirectory, objName),
				size:             size,
				lastUsedUnixTime: time.Now().Unix(),
				plaintext:        plaintextBuf,
				nonce:            nonce,
			}
			cc.chunks[objName] = cached
		}

		return nil
	})
	if err != nil {
		log.Println("error: NewChunkCache: error returned from WalkDir function: ", err)
	}

	return cc
}

func (cc *ChunkCache) FetchExtentIntoBuffer(ctx context.Context, bucket string, objectName string, offset int64, len int64) (extent []byte, nonce []byte, err error) {
	chunkName := strings.TrimPrefix(objectName, "chunks/")

	cc.vlog.Printf("FetchObjIntoBuffer: searching for chunk '%s'", chunkName)
	cc.stats.total += 1

	if cc.isObjCached(chunkName) {
		cc.vlog.Printf("FetchObjIntoBuffer: found in cache '%s'", chunkName)
		cc.stats.hits += 1
	} else {
		cc.vlog.Printf("FetchObjIntoBuffer: not in cache '%s'; downloading", chunkName)
		ciphertextChunkBuf, err := cc.objst.DownloadObjToBuffer(ctx, bucket, objectName)
		if err != nil {
			log.Printf("error: FetchObjToBuffer: failed to retrieve object '%s': %v", objectName, err)
			return nil, nil, err
		}
		cc.saveObjToCache(chunkName, ciphertextChunkBuf)
	}

	// Extract just [offset:offset+len] from plaintext in memory
	plaintextChunkBuf := cc.chunks[chunkName].plaintext
	extent = plaintextChunkBuf[offset : offset+len]

	nonce = cc.chunks[chunkName].nonce

	return extent, nonce, nil
}

func readEntireFile(objName string) ([]byte, error) {
	path := filepath.Join(CacheDirectory, objName)
	f, err := os.Open(path)
	if err != nil {
		log.Printf("error: readEntireFile: failed to open file '%s': %v", objName, err)
		return nil, err
	}
	defer f.Close()

	fInfo, err := os.Stat(path)
	if err != nil {
		log.Printf("error: readEntireFile: failed to stat file '%s': %v", objName, err)
		return nil, err
	}

	size := fInfo.Size()
	ret := make([]byte, size)
	n, err := f.Read(ret)
	if err != nil {
		log.Printf("error: readEntireFile: failed to read file '%s': %v", objName, err)
		return nil, err
	}
	if int64(n) < size {
		msg := fmt.Sprintf("error: readEntireFile: failed to read expected %d bytes (only read %d)", size, n)
		return nil, fmt.Errorf(msg)
	}

	return ret, nil
}

func (cc *ChunkCache) saveObjToCache(objName string, ciphertextBuf []byte) {
	objName = strings.TrimPrefix(objName, "chunks/")
	path := filepath.Join(CacheDirectory, objName)

	// Will added size cause cache to exceed max size?  If so, evict until there is enough space.
	for {
		if cc.totalSizeOnDisk()+int64(len(ciphertextBuf)) > MaxCacheSizeOnDisk {
			cc.evictLeastRecentlyUsed()
		} else {
			break
		}
	}

	// Write out the ciphertext buffer to disk
	f, err := os.Create(path)
	if err != nil {
		log.Println("error: saveObjToCache: ", err)
		return
	}
	defer f.Close()

	n, err := f.Write(ciphertextBuf)
	if err != nil {
		log.Println("error: saveObjToCache: Write failed: ", err)
		return
	}
	if n != len(ciphertextBuf) {
		log.Printf("error: saveObjToCache: wrote only %d bytes (expected to write %d): ", n, len(ciphertextBuf))
		return
	}

	// Decrypt the ciphertext buffer and save its plaintext in memory
	plaintextBuf, nonce, err := cryptography.DecryptBufferReturningNonce(cc.key, ciphertextBuf)
	if err != nil {
		log.Printf("error: saveObjToCache: DecryptBufferReturningNonce failed: %v\n", err)
		return
	}

	// Save in cc struct
	cached := CachedChunk{
		absPath:          path,
		size:             int64(len(ciphertextBuf)),
		lastUsedUnixTime: time.Now().Unix(),
		plaintext:        plaintextBuf,
		nonce:            nonce,
	}
	cc.chunks[objName] = cached
}

func (cc *ChunkCache) isObjCached(objName string) bool {
	if _, ok := cc.chunks[objName]; ok {
		return true
	} else {
		return false
	}
}

func (cc *ChunkCache) totalSizeOnDisk() int64 {
	var accum int64 = 0
	for objName := range cc.chunks {
		accum += cc.chunks[objName].size
	}
	return accum
}

func (cc *ChunkCache) evictLeastRecentlyUsed() {
	lruUnixTime := time.Now().Unix()
	lruObjName := ""
	for objName := range cc.chunks {
		lastUsedUnix := cc.chunks[objName].lastUsedUnixTime
		if lastUsedUnix < lruUnixTime {
			lruObjName = objName
			lruUnixTime = lastUsedUnix
		}
	}
	if lruObjName != "" {
		path := filepath.Join(CacheDirectory, lruObjName)
		if err := os.Remove(path); err != nil {
			log.Printf("error: evictLeastRecentlyUsed: could not remove file '%s': %v\n", path, err)
		}
		delete(cc.chunks, lruObjName)
	}
}

func (cc *ChunkCache) PrintCacheStatistics() {
	if cc.stats.total == 0 {
		cc.vlog.Println("ChunkCache: cache hit rate undefined (cache unused")
	} else {
		percentageHitRate := float64(100) * (float64(cc.stats.hits) / float64(cc.stats.total))
		cc.vlog.Printf("ChunkCache: cache hit rate %d / %d (%02f%%)", cc.stats.hits, cc.stats.total, percentageHitRate)
	}
}
