package main

import (
	"fmt"
	"os"

	"github.com/fsctl/trustlessbak/cmd"
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
		fmt.Printf("trustlessbak v%s-%s (built at %s)\n", Version, CommitHash, BuildTimestamp)
	} else {
		cmd.Execute()
	}
}
