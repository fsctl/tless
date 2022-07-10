package database

import (
	"log"
	"time"
)

type DbAccum struct {
	Accum     int64
	startMark int64
}

func NewDbAccum() *DbAccum {
	return &DbAccum{
		Accum:     0,
		startMark: 0,
	}
}

func (a *DbAccum) Start() {
	a.startMark = time.Now().UnixMicro()
}

func (a *DbAccum) Stop() {
	a.Accum += time.Now().UnixMicro() - a.startMark
}

type DbAccums struct {
	Overall       *DbAccum
	Update        *DbAccum
	CountTotal    *DbAccum
	CountFinished *DbAccum
	Iterations    int64
}

func NewDbAccums() *DbAccums {
	return &DbAccums{
		Overall:       NewDbAccum(),
		Update:        NewDbAccum(),
		CountTotal:    NewDbAccum(),
		CountFinished: NewDbAccum(),
		Iterations:    0,
	}
}

func (a *DbAccums) Print() {
	percentUpdate := 100 * float64(a.Update.Accum) / float64(a.Overall.Accum)
	percentCountTotal := 100 * float64(a.CountTotal.Accum) / float64(a.Overall.Accum)
	percentCountFinished := 100 * float64(a.CountFinished.Accum) / float64(a.Overall.Accum)
	log.Printf("DB Accumulators:\n%2.0f%%  update query\n%2.0f%%  getCountTotalBackupJournal\n%2.0f%%  getCountFinishedBackupJournal\n", percentUpdate, percentCountTotal, percentCountFinished)
}
