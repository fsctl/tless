package daemon

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/fsctl/tless/pkg/util"
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
	reportedEvents []util.ReportedEvent
}

var (
	// protected by gGlobalsLock
	gStatus = &Status{
		state:          Idle,
		msg:            "",
		percentage:     -1.0,
		reportedEvents: make([]util.ReportedEvent, 0),
	}
)

// Callback for rpc.DaemonCtlServer.Status requests
func (s *server) Status(ctx context.Context, in *pb.DaemonStatusRequest) (*pb.DaemonStatusResponse, error) {
	//log.Println(">> GOT & COMPLETED COMMAND: Status")

	// If daemon has restarted we need to tell the client we need a new Hello to boot us up
	gGlobalsLock.Lock()
	isNeedingHello := gUsername == "" || gUserHomeDir == "" || gCfg == nil || gEncKey == nil
	gGlobalsLock.Unlock()
	gDbLock.Lock()
	isNeedingHello = isNeedingHello || gDb == nil
	gDbLock.Unlock()
	if isNeedingHello {
		log.Println(">> Status: we responded that we need a Hello")
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_NEED_HELLO,
			Msg:            "",
			Percentage:     0,
			ReportedEvents: nil}, nil
	}

	// Normal status responses
	gGlobalsLock.Lock()
	defer gGlobalsLock.Unlock()

	if gStatus.state == Idle {
		// Move the reported events over to the return pb struct and clear them here
		pbReportedEvents := make([]*pb.ReportedEvent, 0)
		for _, e := range gStatus.reportedEvents {
			switch e.Kind {
			case util.ERR_OP_NOT_PERMITTED:
				pbReportedEvents = append(pbReportedEvents, &pb.ReportedEvent{
					Kind:     pb.ReportedEvent_ErrOperationNotPermitted,
					Path:     e.Path,
					IsDir:    e.IsDir,
					Datetime: e.Datetime,
					Msg:      e.Msg,
				})
			case util.INFO_BACKUP_COMPLETED:
				pbReportedEvents = append(pbReportedEvents, &pb.ReportedEvent{
					Kind:     pb.ReportedEvent_InfoBackupCompleted,
					Path:     e.Path,
					IsDir:    e.IsDir,
					Datetime: e.Datetime,
					Msg:      e.Msg,
				})
			case util.ERR_INCOMPATIBLE_BUCKET_VERSION:
				pbReportedEvents = append(pbReportedEvents, &pb.ReportedEvent{
					Kind:     pb.ReportedEvent_ErrIncompatibleBucketVersion,
					Path:     e.Path,
					IsDir:    e.IsDir,
					Datetime: e.Datetime,
					Msg:      e.Msg,
				})
			case util.INFO_BACKUP_COMPLETED_WITH_ERRORS:
				pbReportedEvents = append(pbReportedEvents, &pb.ReportedEvent{
					Kind:     pb.ReportedEvent_InfoBackupCompletedWithErrors,
					Path:     e.Path,
					IsDir:    e.IsDir,
					Datetime: e.Datetime,
					Msg:      e.Msg,
				})
			case util.INFO_BACKUP_CANCELED:
				pbReportedEvents = append(pbReportedEvents, &pb.ReportedEvent{
					Kind:     pb.ReportedEvent_InfoBackupCanceled,
					Path:     e.Path,
					IsDir:    e.IsDir,
					Datetime: e.Datetime,
					Msg:      e.Msg,
				})
			case util.INFO_AUTOPRUNE_COMPLETED:
				pbReportedEvents = append(pbReportedEvents, &pb.ReportedEvent{
					Kind:     pb.ReportedEvent_InfoAutopruneCompleted,
					Path:     e.Path,
					IsDir:    e.IsDir,
					Datetime: e.Datetime,
					Msg:      e.Msg,
				})
			}
		}
		gStatus.reportedEvents = make([]util.ReportedEvent, 0)

		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_IDLE,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedEvents: pbReportedEvents}, nil
	} else if gStatus.state == CheckingConn {
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_CHECKING_CONN,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedEvents: nil}, nil
	} else if gStatus.state == BackingUp {
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_BACKING_UP,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedEvents: nil}, nil
	} else if gStatus.state == Restoring {
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_RESTORING,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedEvents: nil}, nil
	} else if gStatus.state == CleaningUp {
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_CLEANING_UP,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedEvents: nil}, nil
	} else {
		// We need a default return
		return &pb.DaemonStatusResponse{
			Status:         pb.DaemonStatusResponse_IDLE,
			Msg:            gStatus.msg,
			Percentage:     gStatus.percentage,
			ReportedEvents: nil}, nil
	}
}

func getLastBackupTimeFormatted(dbLock *sync.Mutex) string {
	dbLock.Lock()
	lastBackupUnixtime, err := gDb.GetLastCompletedBackupUnixTime()
	dbLock.Unlock()
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
