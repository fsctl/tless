package daemon

import (
	"context"
	"log"

	pb "github.com/fsctl/tless/rpc"
)

// Callback for rpc.DaemonCtlServer.Hello requests
func (s *server) Version(ctx context.Context, in *pb.VersionRequest) (*pb.VersionResponse, error) {
	log.Printf("VERSION> tless daemon %s (%s)", gConstVersion, gConstCommitHash)
	return &pb.VersionResponse{Version: gConstVersion, CommitHash: gConstCommitHash}, nil
}
