package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/user"
	"time"

	pb "github.com/fsctl/tless/rpc"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	// Flags
	cfgDaemonSocket string

	daemonClientCmd = &cobra.Command{
		Use:   "daemon-client",
		Short: "Simple demonstration client for the daemon",
		Long: `If the daemon is running in another process on this machine, this simple command will
connect to it and send an RPC.

Example:

	tless daemon-client --socket unix:///tmp/tless.sock

Note the 3 slashes in the socket address.  It's "unix://" plus the absolute path of the socket, which
also begins with a slash.
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			daemonClientMain()
		},
	}
)

func init() {
	daemonClientCmd.Flags().StringVarP(&cfgDaemonSocket, "socket", "o", "unix:///tmp/tless.sock", "unix socket on which daemon is listening")

	extrasCmd.AddCommand(daemonClientCmd)
}

func daemonClientMain() {
	// Set up a connection to the server.
	conn, err := grpc.Dial(cfgDaemonSocket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewDaemonCtlClient(conn)

	// Get information on user running this process
	user, err := user.Current()
	if err != nil {
		log.Fatalln("error: Could not get current user name: ", err)
	}

	// Contact the server and print out its response.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp, err := client.Hello(ctx, &pb.HelloRequest{
		Username:    user.Username,
		UserHomeDir: user.HomeDir})
	if err != nil {
		log.Fatalf("error: could not initiate connection: %v", err)
	}
	log.Printf("Hello response received: didSucceed=%v (%v)", resp.GetDidSucceed(), resp.GetErrMsg())

	go func() {
		for {
			resp, err := client.Status(ctx, &pb.DaemonStatusRequest{})
			if err != nil {
				log.Fatalf("error: could not get daemon status: %v", err)
			}
			log.Printf("Status response: %v\n", resp.GetStatus())
			if resp.GetMsg() != "" {
				log.Printf("  Message: %s\n\n", resp.GetMsg())
			}

			time.Sleep(time.Second)
		}
	}()

	checkConnResp, err := client.CheckConn(ctx, &pb.CheckConnRequest{
		Endpoint:   cfgEndpoint,
		AccessKey:  cfgAccessKeyId,
		SecretKey:  cfgSecretAccessKey,
		BucketName: cfgBucket,
	})
	if err != nil {
		log.Fatalf("error: could not get daemon status: %v", err)
	}
	if checkConnResp.Result == pb.CheckConnResponse_SUCCESS {
		log.Printf("Connection check SUCCESSFUL\n")
	} else {
		log.Printf("Connection check FAILED (with error '%s')\n", checkConnResp.ErrorMsg)
	}

	// Wait 2 seconds (idle check is still running)
	time.Sleep(time.Second * 2)

	// Now try a streaming RPC
	stream, err := client.ReadAllSnapshots(ctx, &pb.ReadAllSnapshotsRequest{})
	if err != nil {
		log.Fatalf("error: cannot open stream %v", err)
	}
	allRelPaths := make([]string, 0)
	for {
		partialResp, err := stream.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatalf("error: stream.Recv() failed: %v", err)
		}
		log.Printf("Partial response rcvd: for %s with %d rel paths\n", partialResp.PartialSnapshot.SnapshotRawName, len(partialResp.PartialSnapshot.RawRelPaths))
		allRelPaths = append(allRelPaths, partialResp.PartialSnapshot.RawRelPaths...)
	}
	fmt.Printf("Got streaming response with %d total rel paths\n", len(allRelPaths))
}
