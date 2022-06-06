package daemon

import (
	"context"
	"log"
	"sync"
	"time"

	pb "github.com/fsctl/tless/rpc"
)

type state int

const (
	Idle state = iota
	CheckingConn
	BackingUp
	Restoring
)

type Status struct {
	state      state
	msg        string
	percentage float32
}

var (
	gStatus = &Status{
		state:      Idle,
		msg:        "",
		percentage: -1.0,
	}
)

// Callback for rpc.DaemonCtlServer.Status requests
func (s *server) Status(ctx context.Context, in *pb.DaemonStatusRequest) (*pb.DaemonStatusResponse, error) {
	//log.Println(">> GOT & COMPLETED COMMAND: Status")

	// If daemon has restarted we need to tell the client we need a new Hello to boot us up
	gGlobalsLock.Lock()
	isNeedingHello := gUsername == "" || gUserHomeDir == "" || gCfg == nil || gDb == nil || gKey == nil
	gGlobalsLock.Unlock()
	if isNeedingHello {
		log.Println(">> Status: we responded that we need a Hello")
		return &pb.DaemonStatusResponse{
			Status:     pb.DaemonStatusResponse_NEED_HELLO,
			Msg:        "",
			Percentage: 0}, nil
	}

	// Normal status responses
	gGlobalsLock.Lock()
	defer gGlobalsLock.Unlock()
	if gStatus.state == Idle {
		return &pb.DaemonStatusResponse{
			Status:     pb.DaemonStatusResponse_IDLE,
			Msg:        gStatus.msg,
			Percentage: gStatus.percentage}, nil
	} else if gStatus.state == CheckingConn {
		return &pb.DaemonStatusResponse{
			Status:     pb.DaemonStatusResponse_CHECKING_CONN,
			Msg:        gStatus.msg,
			Percentage: gStatus.percentage}, nil
	} else if gStatus.state == BackingUp {
		return &pb.DaemonStatusResponse{
			Status:     pb.DaemonStatusResponse_BACKING_UP,
			Msg:        gStatus.msg,
			Percentage: gStatus.percentage}, nil
	} else if gStatus.state == Restoring {
		return &pb.DaemonStatusResponse{
			Status:     pb.DaemonStatusResponse_RESTORING,
			Msg:        gStatus.msg,
			Percentage: gStatus.percentage}, nil
	} else {
		// We need a default return
		return &pb.DaemonStatusResponse{
			Status:     pb.DaemonStatusResponse_IDLE,
			Msg:        gStatus.msg,
			Percentage: gStatus.percentage}, nil
	}
}

func getLastBackupTimeFormatted(globalsLock *sync.Mutex) string {
	globalsLock.Lock()
	lastBackupUnixtime, err := gDb.GetLastCompletedBackupUnixTime()
	globalsLock.Unlock()
	if err != nil {
		log.Printf("error: could not get last completed backup time: %v", err)
	}

	var lastBackupTimeFormatted string
	if lastBackupUnixtime <= int64(0) {
		lastBackupTimeFormatted = "none"
	} else {
		tmUnixUTC := time.Unix(lastBackupUnixtime, 0)
		lastBackupTimeFormatted = tmUnixUTC.Local().Format("Jan 2, 2006 3:04pm")
	}

	return lastBackupTimeFormatted
}
