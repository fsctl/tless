package cmd

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/fsctl/trustlessbak/rpc"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

var (
	// Flags
	cfgPort int

	daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Runs as a background daemon",
		Long: `Starts the program in a mode where it just sits and waits for SIGINT or SIGTERM. 
While waiting, the daemon will listen on the socket /tmp/trustlessbak.sock for commands and 
return the results of running those commands through the socket. Logging output will still 
be written to STDERR.

Example:

	trustlessbak daemon
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			daemonMain()
		},
	}
)

// server is used to implement helloworld.GreeterServer.
type server struct {
	pb.UnimplementedGreeterServer
}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("Received: '%v'", in.GetName())
	return &pb.HelloReply{Message: "Hello from " + in.GetName()}, nil
}

func init() {
	daemonCmd.Flags().IntVarP(&cfgPort, "port", "P", 50051, "localhost port that daemon listens on")

	rootCmd.AddCommand(daemonCmd)
}

func daemonMain() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan bool, 1)

	go func() {
		// setup listener and grpc server instance
		lis, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", cfgPort))
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		s := grpc.NewServer()

		// spawn a go routine to listen on socket
		go func() {
			pb.RegisterGreeterServer(s, &server{})
			log.Printf("server listening at %v", lis.Addr())
			if err := s.Serve(lis); err != nil {
				log.Fatalf("failed to serve: %v", err)
			}
		}()

		// Go into a blocking wait for the requested signal notifications
		<-signals
		fmt.Println() // line break after ^C

		// do cleanup here before terminating
		s.GracefulStop()

		// tell main routine we are ready to exit
		done <- true
	}()

	fmt.Println("Press Ctrl+C to exit")
	<-done
}
