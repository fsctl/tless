package cmd

import (
	"context"
	"log"
	"time"

	pb "github.com/fsctl/trustlessbak/rpc"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	// Flags
	cfgDaemonHost string

	daemonClientCmd = &cobra.Command{
		Use:   "daemon-client",
		Short: "Simple demonstration client for the daemon",
		Long: `If the daemon is running in another process on this machine, this simple command will
connect to it and send an RPC.

Example:

	trustlessbak daemon-client --daemon-host localhost:50051
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			daemonClientMain()
		},
	}
)

func init() {
	daemonClientCmd.Flags().StringVarP(&cfgDaemonHost, "daemon-host", "D", "127.0.0.1:50051", "host:port on which daemon is listening")

	extrasCmd.AddCommand(daemonClientCmd)
}

func daemonClientMain() {
	// Set up a connection to the server.
	conn, err := grpc.Dial(cfgDaemonHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewGreeterClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := c.SayHello(ctx, &pb.HelloRequest{Name: "Daemon Client"})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
	log.Printf("Greeting reply received: %s", r.GetMessage())
}
