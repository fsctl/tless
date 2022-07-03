package daemon

import (
	"fmt"
	"testing"
	"time"

	"github.com/fsctl/tless/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestReadFileIntoLines(t *testing.T) {
	// Set up
	logPath := "/var/log/tless.log"
	vlog := util.NewVLog(nil, func() bool { return true })
	startingOffset := int64(0)
	nextOffset := int64(0)

	returnSomeClosure := func(didSucceed bool, errMsg string, logLines []string, nxtOffset int64, percentDone float64) error {
		assert.True(t, didSucceed)
		assert.Empty(t, errMsg)
		nextOffset = nxtOffset
		for _, line := range logLines {
			fmt.Printf("> '%s' (len=%d)\n", line, len(line))
		}
		return nil
	}

	// Test
	fmt.Println("------------------------------------------------------------")
	readFileIntoLines(returnSomeClosure, startingOffset, logPath, vlog)
	fmt.Println("------------------------------------------------------------")

	time.Sleep(time.Second * 5)

	// Test with more lines
	readFileIntoLines(returnSomeClosure, nextOffset, logPath, vlog)
	fmt.Println("------------------------------------------------------------")
}
