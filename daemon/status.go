package daemon

import (
	"context"
	"log"
	"sync"

	pb "github.com/fsctl/tless/rpc"
)

type state int

const (
	Idle state = iota
	CheckingConn
	BackingUp
	Restoring
)

type StatusInfo struct {
	lock       sync.Mutex
	state      state
	msg        string
	percentage float32
}

var (
	Status = &StatusInfo{
		state:      Idle,
		msg:        "",
		percentage: -1.0,
	}
)

// Callback for rpc.DaemonCtlServer.Status requests
func (s *server) Status(ctx context.Context, in *pb.DaemonStatusRequest) (*pb.DaemonStatusResponse, error) {
	log.Println(">> GOT & COMPLETED COMMAND: Status")

	// If daemon has restarted we need to tell the client we need a new Hello to boot us up
	if gUsername == "" || gUserHomeDir == "" || gCfg == nil {
		return &pb.DaemonStatusResponse{
			Status:     pb.DaemonStatusResponse_NEED_HELLO,
			Msg:        "",
			Percentage: 0}, nil
	}

	// Normal status responses
	Status.lock.Lock()
	defer Status.lock.Unlock()

	if Status.state == Idle {
		return &pb.DaemonStatusResponse{
			Status:     pb.DaemonStatusResponse_IDLE,
			Msg:        Status.msg,
			Percentage: Status.percentage}, nil
	} else if Status.state == CheckingConn {
		return &pb.DaemonStatusResponse{
			Status:     pb.DaemonStatusResponse_CHECKING_CONN,
			Msg:        Status.msg,
			Percentage: Status.percentage}, nil
	}
	//
	// TODO:  more else if's....
	//

	// For now we need a default return:
	return &pb.DaemonStatusResponse{
		Status:     pb.DaemonStatusResponse_IDLE,
		Msg:        Status.msg + "(default)",
		Percentage: 55.0}, nil
}
