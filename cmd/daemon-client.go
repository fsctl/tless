package cmd

import (
	"context"
	"log"
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
