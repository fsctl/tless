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
	"path/filepath"
	"sync"
	"syscall"

	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
	"google.golang.org/grpc"
)

const (
	unixSocketPath         = "/tmp/tless.sock"
	standardConfigFileName = "config.toml"
)

var (
	gGlobalsLock sync.Mutex
	gDb          *database.DB
)

// server is used to implement helloworld.GreeterServer.
type server struct {
	pb.UnimplementedDaemonCtlServer
}

// Callback for rpc.DaemonCtlServer.Hello requests
func (s *server) Hello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloResponse, error) {
	log.Printf("Received Hello from: '%v' (with homedir '%v')", in.GetUsername(), in.GetUserHomeDir())

	// Set up global state
	gGlobalsLock.Lock()
	gUsername = in.GetUsername()
	gUserHomeDir = in.GetUserHomeDir()
	gGlobalsLock.Unlock()
	initConfig()
	initDbConn()

	// Replay dirty journals
	go func() {
		for {
			gGlobalsLock.Lock()
			hasDirtyBackupJournal, err := gDb.HasDirtyBackupJournal()
			gGlobalsLock.Unlock()
			if err != nil {
				log.Println("error: gDb.HasDirtyBackupJournal: ", err)
			}
			if hasDirtyBackupJournal {
				replayBackupJournal()
			} else {
				return
			}
		}
	}()

	// Return useless response
	return &pb.HelloResponse{Message: "Hello there, " + in.GetUsername() + " (with homedir '" + in.GetUserHomeDir() + "')"}, nil
}

func initDbConn() {
	gGlobalsLock.Lock()
	username := gUsername
	userHomeDir := gUserHomeDir
	gGlobalsLock.Unlock()

	// open and prepare sqlite database
	sqliteDir, err := util.MkdirUserConfig(username, userHomeDir)
	if err != nil {
		log.Fatalf("error: making sqlite dir: %v", err)
	}
	gGlobalsLock.Lock()
	gDb, err = database.NewDB(filepath.Join(sqliteDir, "state.db"))
	gGlobalsLock.Unlock()
	if err != nil {
		log.Fatalf("error: cannot open database: %v", err)
	}
	gGlobalsLock.Lock()
	err = gDb.CreateTablesIfNotExist()
	gGlobalsLock.Unlock()
	if err != nil {
		log.Fatalf("error: cannot initialize database: %v", err)
	}

	// Get the last completed backup time
	lastBackupTimeFormatted := getLastBackupTimeFormatted(&gGlobalsLock)

	// Set status message to last backup time if status is Idle
	gGlobalsLock.Lock()
	if gStatus.state == Idle {
		gStatus.msg = "Last backup: " + lastBackupTimeFormatted
	}
	gGlobalsLock.Unlock()
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

		// TODO:  go routine that wakes up periodically and checks if its time for a backup
		//....

		// Go into a blocking wait for the requested signal notifications
		<-signals
		fmt.Println() // line break after ^C

		// do cleanup here before terminating
		s.GracefulStop()

		// other cleanup
		gGlobalsLock.Lock()
		if gDb != nil {
			gDb.Close()
		}
		gGlobalsLock.Unlock()

		// tell main routine we are ready to exit
		done <- true
	}()

	fmt.Println("Press Ctrl+C to exit")
	<-done
}
