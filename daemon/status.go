package daemon

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/fsctl/tless/pkg/fstraverse"
	pb "github.com/fsctl/tless/rpc"
)

type state int

const (
	Idle state = iota
	CheckingConn
	BackingUp
	Restoring
	CleaningUp
)

type Status struct {
	state          state
	msg            string
	percentage     float32
	reportedErrors []fstraverse.SeriousError
}

var (
	gStatus = &Status{
		state:          Idle,
		msg:            "",
		percentage:     -1.0,
		reportedErrors: make([]fstraverse.SeriousError, 0),
	}
	//gTempCntr = 0
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
			Status:         pb.DaemonStatusResponse_NEED_HELLO,
			Msg:            "",
			Percentage:     0,
			ReportedErrors: nil}, nil
	}

	// Normal status responses
	gGlobalsLock.Lock()
	defer gGlobalsLock.Unlock()

	// TEMP CODE FOR TESTING ONLY
	// if gTempCntr == 0 {
	// 	gTempCntr += 1
	//
	// 	gStatus.reportedErrors = append(gStatus.reportedErrors, fstraverse.SeriousError{
	// 		Kind:     fstraverse.OP_NOT_PERMITTED,
	// 		Path:     "/test/path",
	// 		IsDir:    true,
	// 		Datetime: time.Now().Unix(),
	// 	})
	// }

	if gStatus.state == Idle {
		// move the reported errors over to the return pb struct and clear them here
		pbReportedErrors := make([]*pb.ReportedError, 0)
		for _, e := range gStatus.reportedErrors {
			switch e.Kind {
			case fstraverse.OP_NOT_PERMITTED:
				pbReportedErrors = append(pbReportedErrors, &pb.ReportedError{
					Kind:     pb.ReportedError_OperationNotPermitted,
					Path:     e.Path,
					IsDir:    e.IsDir,
					Datetime: e.Datetime,
				})
			}
		}
		gStatus.reportedErrors = make([]fstraverse.SeriousError, 0)

		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_IDLE,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedErrors: pbReportedErrors}, nil
	} else if gStatus.state == CheckingConn {
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_CHECKING_CONN,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedErrors: nil}, nil
	} else if gStatus.state == BackingUp {
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_BACKING_UP,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedErrors: nil}, nil
	} else if gStatus.state == Restoring {
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_RESTORING,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedErrors: nil}, nil
	} else if gStatus.state == CleaningUp {
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_CLEANING_UP,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedErrors: nil}, nil
	} else {
		// We need a default return
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_IDLE,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedErrors: nil}, nil
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
