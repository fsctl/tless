package daemon

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/objstore"
	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
	"google.golang.org/grpc"
)

const (
	unixSocketPath         = "/tmp/tless.sock"
	standardConfigFileName = "config.toml"
)

var (
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
	log.Printf("Received Hello from: '%v' (with homedir '%v')", in.GetUsername(), in.GetUserHomeDir())

	// Set up global state
	gGlobalsLock.Lock()
	gUsername = in.GetUsername()
	gUserHomeDir = in.GetUserHomeDir()
	gGlobalsLock.Unlock()
	initConfig(&gGlobalsLock)
	initDbConn(&gGlobalsLock)

	// Do crypto config check and report any failures back to user rather than continuing
	gGlobalsLock.Lock()
	endpoint := gCfg.Endpoint
	accessKey := gCfg.AccessKeyId
	secretKey := gCfg.SecretAccessKey
	bucket := gCfg.Bucket
	salt := gCfg.Salt
	key := gKey
	gGlobalsLock.Unlock()
	ctxBkg := context.Background()
	objst := objstore.NewObjStore(ctxBkg, endpoint, accessKey, secretKey)
	if ok, err := objst.IsReachableWithRetries(ctxBkg, 5, bucket); !ok {
		errMsg := fmt.Sprintf("server not reachable: %v", err)
		log.Println(errMsg)
		return &pb.HelloResponse{DidSucceed: false, ErrMsg: errMsg}, nil
	}
	if ok, err := objst.CheckCryptoConfigMatchesServerDaemon(ctx, key, bucket, salt); !ok {
		errMsg := fmt.Sprintf("cannot continue due to cryptographic config error: %v", err)
		log.Println(errMsg)
		return &pb.HelloResponse{DidSucceed: false, ErrMsg: errMsg}, nil
	}

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
	globalsLock.Lock()
	sqliteFilePath := filepath.Join(sqliteDir, "state.db")
	gDb, err = database.NewDB(sqliteFilePath)
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
		s := grpc.NewServer(grpc.MaxSendMsgSize(math.MaxInt32 * 1024))

		// spawn a go routine to listen on socket
		go func() {
			pb.RegisterDaemonCtlServer(s, &server{})
			log.Printf("server listening at %v", lis.Addr())
			if err := s.Serve(lis); err != nil {
				log.Fatalf("failed to serve: %v", err)
			}
		}()

		// go routine that wakes up periodically and checks if its time for a backup
		go timerLoop(signals, &server{})

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
