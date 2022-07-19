package daemon

import (
	"context"
	"fmt"
	"log"

	pb "github.com/fsctl/tless/rpc"
)

// Callback for rpc.DaemonCtlServer.ReadAllSnapshotsMetadata requests
func (s *server) GetUsageHistory(context.Context, *pb.GetUsageHistoryRequest) (*pb.GetUsageHistoryResponse, error) {
	//vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	log.Println(">> GOT COMMAND: GetUsageHistory")
	defer log.Println(">> COMPLETED COMMAND: GetUsageHistory")

	doneWithError := func(msg string) (*pb.GetUsageHistoryResponse, error) {
		log.Println(msg)
		return &pb.GetUsageHistoryResponse{
			DidSucceed:          false,
			ErrMsg:              msg,
			PeakSpaceUsage:      []*pb.DailyUsage{},
			TotalBandwidthUsage: []*pb.DailyUsage{},
		}, nil
	}

	// Make sure the global config we need is initialized
	gGlobalsLock.Lock()
	isDbReady := gDb != nil
	gGlobalsLock.Unlock()
	if !isDbReady {
		msg := fmt.Sprintln("error: GetUsageHistory: database not yet initialized")
		return doneWithError(msg)
	}

	gDbLock.Lock()
	peakDailySpaceUsages, err := gDb.GetPeakDailySpaceUsage(3)
	gDbLock.Unlock()
	if err != nil {
		msg := fmt.Sprintln("error: GetUsageHistory: ", err)
		return doneWithError(msg)
	}

	retPeakSpace := make([]*pb.DailyUsage, 0, 3*31)
	for _, peakDailySpaceUsage := range peakDailySpaceUsages {
		retPeakSpace = append(retPeakSpace, &pb.DailyUsage{
			DayYmd:    peakDailySpaceUsage.DateYMD,
			ByteCount: peakDailySpaceUsage.ByteCnt,
		})
	}

	gDbLock.Lock()
	totalDailyBandwidthUsages, err := gDb.GetTotalDailyBandwidthUsage(3)
	gDbLock.Unlock()
	if err != nil {
		msg := fmt.Sprintln("error: GetUsageHistory: ", err)
		return doneWithError(msg)
	}

	retTotalBandwidth := make([]*pb.DailyUsage, 0, 3*31)
	for _, totalDailyBandwidthUsage := range totalDailyBandwidthUsages {
		retTotalBandwidth = append(retTotalBandwidth, &pb.DailyUsage{
			DayYmd:    totalDailyBandwidthUsage.DateYMD,
			ByteCount: totalDailyBandwidthUsage.ByteCnt,
		})
	}

	return &pb.GetUsageHistoryResponse{
		DidSucceed:          true,
		ErrMsg:              "",
		PeakSpaceUsage:      retPeakSpace,
		TotalBandwidthUsage: retTotalBandwidth,
	}, nil
}
