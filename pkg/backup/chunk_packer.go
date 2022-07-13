package backup

import (
	"context"
	"log"
	"sync"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
)

type runWhileUploadingFuncType func(runWhileUploadingFinished chan bool, goodTime bool, forcePersist bool)

type chunkPackerItem struct {
	relPath string
	Offset  int
	Len     int
	bjt     *database.BackupJournalTask
}

type chunkPacker struct {
	items                 []chunkPackerItem
	plaintextChunkBuf     []byte
	posInPlaintextChunk   int
	db                    *database.DB
	dbLock                *sync.Mutex
	ctx                   context.Context
	objst                 *objstore.ObjStore
	bucket                string
	key                   []byte
	vlog                  *util.VLog
	runWhileUploadingFunc runWhileUploadingFuncType
	totalCntJournal       *int64
	finishedCountJournal  *int64
	stats                 *BackupStats
}

func (cp *chunkPacker) AddDirEntry(relPath string, buf []byte, bjt *database.BackupJournalTask) (succeeded bool) {
	//cp.vlog.Printf("chunkPacker: AddDirEntry: asked to add %d byte buffer", len(buf))

	// If adding this buffer would exceed chunk's capacity, return false immediately
	if int64(len(buf))+int64(cp.posInPlaintextChunk) > ChunkSize {
		//cp.vlog.Printf("chunkPacker: AddDirEntry: declining (%d+%d kb = %d kb would exceed ChunkSize = %d kb)", int64(len(buf))/1024, int64(cp.posInPlaintextChunk)/1024, (int64(len(buf))+int64(cp.posInPlaintextChunk))/1024, ChunkSize/1024)
		return false
	}

	// Otherwise, add the chunk
	//cp.vlog.Printf("chunkPacker: AddDirEntry: adding dir entry")
	newItem := chunkPackerItem{
		relPath: relPath,
		Offset:  cp.posInPlaintextChunk,
		Len:     len(buf),
		bjt:     bjt,
	}
	cp.plaintextChunkBuf = append(cp.plaintextChunkBuf, buf...)
	cp.posInPlaintextChunk += len(buf)
	cp.items = append(cp.items, newItem)
	if cp.stats != nil {
		cp.stats.AddBytes(int64(len(buf)))
	}
	return true
}

func (cp *chunkPacker) Complete() (isJournalComplete bool) {
	isJournalComplete = false

	//
	// Commit the chunk to the cloud if there's anything in it
	//
	chunkName := generateRandomChunkName()
	//cp.vlog.Printf("chunkPacker: Complete: completing the current chunk as %s", chunkName)
	if len(cp.plaintextChunkBuf) > 0 {
		//cp.vlog.Printf("chunkPacker: Complete: found %d byte plaintextChunkBuf with posInChunkBuf = %d", len(cp.plaintextChunkBuf), cp.posInPlaintextChunk)

		// Encrypt the chunk
		ciphertextChunkBuf, err := cryptography.EncryptBuffer(cp.key, cp.plaintextChunkBuf)
		if err != nil {
			log.Fatalf("error: chunkPacker.Complete: EncryptBuffer failed: %v\n", err)
			return
		}

		// Set up runWhileUploadingFunc
		runWhileUploadingFinished := make(chan bool, 1)
		if cp.runWhileUploadingFunc != nil {
			go cp.runWhileUploadingFunc(runWhileUploadingFinished, true, false)
		} else {
			runWhileUploadingFinished <- true
		}

		// Upload chunk (and run unrelated parallel func during upload)
		objName := "chunks/" + chunkName
		cp.vlog.Printf("chunkPacker: Complete: writing object '%s' to cloud (%s)", objName, util.FormatBytesAsString(int64(len(ciphertextChunkBuf))))
		err = cp.objst.UploadObjFromBuffer(cp.ctx, cp.bucket, objName, ciphertextChunkBuf, objstore.ComputeETag(ciphertextChunkBuf))
		if err != nil {
			log.Printf("error: chunkPacker.Complete: failed while uploading '%s': %v\n", chunkName, err)
			return
		}

		// Wait for runWhileUploadingFunc to finish
		cp.vlog.Println("RUN WHILE UPLOAD> Waiting for 'runWhileUploadingFunc' to finish...")
		<-runWhileUploadingFinished
		cp.vlog.Println("RUN WHILE UPLOAD> Has finished: 'runWhileUploadingFunc'")
	}

	//
	// Finalize each item
	//
	for _, item := range cp.items {
		// sanity check
		if isJournalComplete {
			log.Println("error: chunkPacker.Complete: something's wrong; journal should never be exhausted before end of last item in this loop")
		}
		crp := &snapshots.CloudRelPath{
			RelPath: item.relPath,
			ChunkExtents: []snapshots.ChunkExtent{
				{
					ChunkName: chunkName,
					Offset:    int64(item.Offset),
					Len:       int64(item.Len),
				},
			},
		}
		cp.vlog.Printf("chunkPacker: Complete: finalizing '%s' with offset=%d, len=%d", crp.RelPath, crp.ChunkExtents[0].Offset, crp.ChunkExtents[0].Len)
		updateLastBackupTime(cp.db, cp.dbLock, item.bjt.DirEntId)
		isJournalComplete = completeTask(cp.db, cp.dbLock, item.bjt, crp, cp.totalCntJournal, cp.finishedCountJournal)
	}

	// Reset struct to initial state so it can be reused for next chunk
	cp.items = make([]chunkPackerItem, 0)
	cp.plaintextChunkBuf = make([]byte, 0)
	cp.posInPlaintextChunk = 0

	//cp.vlog.Printf("chunkPacker: Complete: return isJournalComplete=%s", isJournalComplete)
	return isJournalComplete
}

func newChunkPacker(ctx context.Context, objst *objstore.ObjStore, bucket string, db *database.DB, dbLock *sync.Mutex, key []byte, vlog *util.VLog, runWhileUploadingFunc runWhileUploadingFuncType, totalCntJournal *int64, finishedCountJournal *int64, stats *BackupStats) *chunkPacker {
	return &chunkPacker{
		items:                 make([]chunkPackerItem, 0),
		plaintextChunkBuf:     make([]byte, 0),
		posInPlaintextChunk:   0,
		db:                    db,
		dbLock:                dbLock,
		ctx:                   ctx,
		objst:                 objst,
		bucket:                bucket,
		key:                   key,
		vlog:                  vlog,
		runWhileUploadingFunc: runWhileUploadingFunc,
		totalCntJournal:       totalCntJournal,
		finishedCountJournal:  finishedCountJournal,
		stats:                 stats,
	}
}
