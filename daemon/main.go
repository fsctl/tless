package daemon

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/fsctl/tless/rpc"
	"google.golang.org/grpc"
)

const (
	unixSocketPath         = "/tmp/tless.sock"
	standardConfigFileName = "config.toml"
)

// server is used to implement helloworld.GreeterServer.
type server struct {
	pb.UnimplementedDaemonCtlServer
}

// Callback for rpc.DaemonCtlServer.Hello requests
func (s *server) Hello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloResponse, error) {
	log.Printf("Received Hello from: '%v' (with homedir '%v')", in.GetUsername(), in.GetUserHomeDir())
	gUsername = in.GetUsername()
	gUserHomeDir = in.GetUserHomeDir()
	initConfig()
	return &pb.HelloResponse{Message: "Hello there, " + in.GetUsername() + " (with homedir '" + in.GetUserHomeDir() + "')"}, nil
}

func DaemonMain() {
	// clean up old socket file from any preceding unclean shutdown
	err := os.Remove(unixSocketPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("error removing old socket: %T", err)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan bool, 1)

	go func() {
		// setup listener and grpc server instance
		lis, err := net.Listen("unix", unixSocketPath)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		err = os.Chmod(unixSocketPath, 0777)
		if err != nil {
			log.Fatalf("failed to chmod the socket: %v", err)
		}
		s := grpc.NewServer()

		// spawn a go routine to listen on socket
		go func() {
			pb.RegisterDaemonCtlServer(s, &server{})
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
