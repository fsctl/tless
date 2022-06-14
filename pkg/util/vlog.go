package util

import (
	"log"
	"sync"
)

type VLog struct {
	lock          *sync.Mutex
	isVerboseFunc func() bool
}

func NewVLog(lock *sync.Mutex, checkIfVerboseFunc func() bool) *VLog {
	return &VLog{
		lock:          lock,
		isVerboseFunc: checkIfVerboseFunc,
	}
}

func (v *VLog) Println(x ...any) {
	LockIf(v.lock)
	isVerbose := v.isVerboseFunc()
	UnlockIf(v.lock)
	if isVerbose {
		log.Println(x...)
	}
}

func (v *VLog) Printf(format string, x ...any) {
	LockIf(v.lock)
	isVerbose := v.isVerboseFunc()
	UnlockIf(v.lock)
	if isVerbose {
		log.Printf(format, x...)
	}
}
