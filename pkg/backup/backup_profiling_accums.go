package backup

import (
	"log"
	"time"
)

type BackupAccum struct {
	Accum     int64
	startMark int64
}

func NewAccum() *BackupAccum {
	return &BackupAccum{
		Accum:     0,
		startMark: 0,
	}
}

func (a *BackupAccum) Start() {
	a.startMark = time.Now().UnixMicro()
}

func (a *BackupAccum) Stop() {
	a.Accum += time.Now().UnixMicro() - a.startMark
}

type BackupAccums struct {
	Overall              *BackupAccum
	CancelCheck          *BackupAccum
	GetDirEnt            *BackupAccum
	Updated              *BackupAccum
	Unchanged            *BackupAccum
	Deleted              *BackupAccum
	FinishTask           *BackupAccum
	UpdateLastBackupTime *BackupAccum
	CompleteTask         *BackupAccum
	ProgressUpdate       *BackupAccum
	Iterations           int64
}

func NewBackupAccums() *BackupAccums {
	return &BackupAccums{
		Overall:              NewAccum(),
		CancelCheck:          NewAccum(),
		GetDirEnt:            NewAccum(),
		Updated:              NewAccum(),
		Unchanged:            NewAccum(),
		Deleted:              NewAccum(),
		FinishTask:           NewAccum(),
		UpdateLastBackupTime: NewAccum(),
		CompleteTask:         NewAccum(),
		ProgressUpdate:       NewAccum(),
		Iterations:           0,
	}
}

func (a *BackupAccums) Print() {
	percentCancelCheck := 100 * float64(a.CancelCheck.Accum) / float64(a.Overall.Accum)
	percentGetDirEnt := 100 * float64(a.GetDirEnt.Accum) / float64(a.Overall.Accum)
	percentUpdated := 100 * float64(a.Updated.Accum) / float64(a.Overall.Accum)
	percentUnchanged := 100 * float64(a.Unchanged.Accum) / float64(a.Overall.Accum)
	percentDeleted := 100 * float64(a.Deleted.Accum) / float64(a.Overall.Accum)
	percentFinishTask := 100 * float64(a.FinishTask.Accum) / float64(a.Overall.Accum)
	percentUpdateLastBackup := 100 * float64(a.UpdateLastBackupTime.Accum) / float64(a.FinishTask.Accum)
	percentCompleteTask := 100 * float64(a.CompleteTask.Accum) / float64(a.FinishTask.Accum)
	percentProgressUpdate := 100 * float64(a.ProgressUpdate.Accum) / float64(a.Overall.Accum)
	log.Printf("Accumulators:\n%2.0f%%  cancel check\n%2.0f%%  get dir ent\n%2.0f%%  updated\n%2.0f%%  unchanged\n%2.0f%%  deleted\n%2.0f%%  finish task\n%2.0f%%    updatelastbackup\n%2.0f%%    completetask\n%2.0f%%  progress update\n", percentCancelCheck, percentGetDirEnt, percentUpdated, percentUnchanged, percentDeleted, percentFinishTask, percentUpdateLastBackup, percentCompleteTask, percentProgressUpdate)
}
