package main

import (
	"fmt"
	"os"

	"github.com/fsctl/tless/cmd"
	"github.com/fsctl/tless/daemon"
)

var (
	// Compile-time variables
	Version        string
	CommitHash     string
	BuildTimestamp string
)

func main() {
	// We have to trap "--version" out here because linker won't write to cmd package
	// during `go build`
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("tless v%s-%s (built at %s)\n", Version, CommitHash, BuildTimestamp)
	} else if len(os.Args) == 2 && (os.Args[1] == "daemon" || os.Args[1] == "-d") {
		// We also trap 'daemon'/'-d' out here because daemon mode bypasses all the viper/cobra
		// and toml config stuff because in this mode we receive all our input on a grpc unix socket.
		daemon.DaemonMain()
	} else {
		cmd.Execute()
	}
}
