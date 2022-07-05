package backup

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/fsctl/tless/pkg/snapshots"
	"github.com/fsctl/tless/pkg/util"
)

type BackupStats struct {
	cntFiles      int64
	cntBytes      int64
	startTimeUnix int64
}

func NewBackupStats() *BackupStats {
	return &BackupStats{
		cntFiles:      0,
		cntBytes:      0,
		startTimeUnix: time.Now().Unix(),
	}
}

func (bs *BackupStats) AddFile() {
	atomic.AddInt64(&bs.cntFiles, 1)
}

func (bs *BackupStats) AddBytes(n int64) {
	atomic.AddInt64(&bs.cntBytes, n)
}

func (bs *BackupStats) AddBytesFromChunkExtents(chunkExtents []snapshots.ChunkExtent) {
	for _, chunkExtent := range chunkExtents {
		bs.AddBytes(chunkExtent.Len)
	}
}

func (bs *BackupStats) FinalReport() string {
	durationSeconds := time.Now().Unix() - bs.startTimeUnix
	humanReadableDuration := util.FormatSecondsAsString(durationSeconds)
	humanReadableBytes := util.FormatBytesAsString(atomic.LoadInt64(&bs.cntBytes))
	humanFilesCount := util.FormatNumberAsString(atomic.LoadInt64(&bs.cntFiles))
	humanDataRate := util.FormatDataRateAsString(atomic.LoadInt64(&bs.cntBytes), durationSeconds)
	return fmt.Sprintf("%s files (%s) in %s ~= %s", humanFilesCount, humanReadableBytes, humanReadableDuration, humanDataRate)
}
