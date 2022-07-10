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
	"time"

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
	gConstVersion    string // not protected by lock
	gConstCommitHash string // not protected by lock
	gGlobalsLock     sync.Mutex
	gDb              *database.DB
	gCancelRequested bool
)

// server is used to implement helloworld.GreeterServer.
type server struct {
	pb.UnimplementedDaemonCtlServer
}

// Callback for rpc.DaemonCtlServer.Hello requests
func (s *server) Hello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloResponse, error) {
	log.Printf("HELLO> Rcvd Hello from '%v' (with homedir '%v')", in.GetUsername(), in.GetUserHomeDir())

	// Set up global state
	gGlobalsLock.Lock()
	gUsername = in.GetUsername()
	gUserHomeDir = in.GetUserHomeDir()
	gGlobalsLock.Unlock()
	initDbConn(&gGlobalsLock)
	if err := initConfig(&gGlobalsLock); err != nil {
		return &pb.HelloResponse{DidSucceed: false, ErrMsg: err.Error()}, nil
	}

	// Replay dirty journals
	go func() {
		for {
			gGlobalsLock.Lock()
			isIdle := gStatus.state == Idle
			hasDirtyBackupJournal, err := gDb.HasDirtyBackupJournal()
			gGlobalsLock.Unlock()
			if err != nil {
				log.Println("error: gDb.HasDirtyBackupJournal: ", err)
			}
			if hasDirtyBackupJournal {
				if isIdle {
					replayBackupJournal()
				} else {
					time.Sleep(time.Second * 30)
				}
			} else {
				return
			}
		}
	}()

	// Return useless response
	return &pb.HelloResponse{DidSucceed: true, ErrMsg: ""}, nil
}

func initDbConn(globalsLock *sync.Mutex) {
	globalsLock.Lock()
	username := gUsername
	userHomeDir := gUserHomeDir
	globalsLock.Unlock()

	// open and prepare sqlite database
	sqliteDir, err := util.MkdirUserConfig(username, userHomeDir)
	if err != nil {
		log.Fatalf("error: making sqlite dir: %v", err)
	}

	// This code is intended to call Close on gDb in cases where it's not nil.  That might
	// occur if the client disconnects and then reconnects, sending a new hello.
	// Originally, we simply called gDB.Close() whenever gDb was non-nill. However, it turns
	// out that some operations like backup and restore make copies of gDb and use the copies,
	// so they were crashing with a closed handle.  To solve that, we only call gDb.Close() when
	// we're in an Idle state, meaning no one is holding a long-lived copy of gDb.
	// It's not perfect because we still leak some DB handles, but it's less leakage than before.
	sqliteFilePath := filepath.Join(sqliteDir, "state.db")
	var dbTemp *database.DB
	globalsLock.Lock()
	dbTemp, err = database.NewDB(sqliteFilePath)
	if err == nil {
		// Only try to close current db conn if we're in an idle state
		if gStatus.state == Idle {
			if gDb != nil {
				gDb.Close()
			}
			gDb = dbTemp
		} else {
			gDb = dbTemp
		}
	}
	globalsLock.Unlock()
	if err != nil {
		log.Fatalf("error: cannot open database: %v", err)
	}

	globalsLock.Lock()
	err = gDb.CreateTablesIfNotExist()
	globalsLock.Unlock()
	if err != nil {
		log.Fatalf("error: cannot initialize database: %v", err)
	}

	// in case we just created a new db file, set its permissions to console user as owner
	uid, gid, err := util.GetUidGid(username)
	if err != nil {
		log.Println("error: cannot get user's UID/GID: ", err)
	}
	if err := os.Chown(sqliteFilePath, uid, gid); err != nil {
		log.Printf("error: could not chown db file '%s' to '%d/%d': %v", sqliteFilePath, uid, gid, err)
	}

	// Get the last completed backup time
	lastBackupTimeFormatted := getLastBackupTimeFormatted(&gGlobalsLock)

	// Set status message to last backup time if status is Idle
	globalsLock.Lock()
	if gStatus.state == Idle {
		gStatus.msg = "Last backup: " + lastBackupTimeFormatted
	}
	globalsLock.Unlock()
}

func DaemonMain(version string, commitHash string) {
	// Save the program version in global "constants"
	gConstVersion = version
	gConstCommitHash = commitHash

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
		err = os.Chmod(unixSocketPath, 0600)
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

		// go routine that wakes up periodically and checks if its time for a backup
		go timerLoop(&server{})

		// Go into a blocking wait for the requested signal notifications
		<-signals
		fmt.Println() // line break after ^C

		// This is commented out because otherwise the gRPC listener panics when SIGTERM is
		// received during the processing of an RPC.
		//s.GracefulStop()

		// Give go routines a chance to gracefully terminate
		time.Sleep(time.Second * 15)

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
