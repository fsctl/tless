package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var (
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

func init() {
	rootCmd.AddCommand(daemonCmd)
}

func daemonMain() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan bool, 1)

	go func() {
		// TODO: spawn a go routine to listen on socket

		// Go into a blocking wait for the requested signal notifications
		<-signals
		fmt.Println() // line break after ^C

		// TODO: do cleanup here before terminating

		done <- true
	}()

	fmt.Println("Press Ctrl+C to exit")
	<-done
}
