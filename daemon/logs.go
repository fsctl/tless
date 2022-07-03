package daemon

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/fsctl/tless/pkg/util"
	pb "github.com/fsctl/tless/rpc"
)

const (
	MaxLineLengthBytes           int = 300
	MaxLinesToReturnInSingleCall int = 5000
)

// Callback for rpc.DaemonCtlServer.LogStream requests
func (s *server) LogStream(in *pb.LogStreamRequest, srv pb.DaemonCtl_LogStreamServer) error {
	vlog := util.NewVLog(&gGlobalsLock, func() bool { return gCfg == nil || gCfg.VerboseDaemon })

	// log.Printf(">> GOT COMMAND: LogStream (%s starting at byte %d)", in.LogPath, in.StartingOffset)
	// defer log.Println(">> COMPLETED COMMAND: LogStream")
	logPath := in.LogPath
	startingOffset := in.StartingOffset

	returnSome := func(didSucceed bool, errMsg string, logLines []string, nextOffset int64, percentDone float64) error {
		resp := pb.LogStreamResponse{
			DidSucceed:  didSucceed,
			ErrMsg:      errMsg,
			LogLines:    logLines,
			NextOffset:  nextOffset,
			PercentDone: percentDone,
		}
		if err := srv.Send(&resp); err != nil {
			log.Printf("error: server.Send failed (sending %d strings, with nextOffset=%d): %v", len(logLines), nextOffset, err)
			return err
		}
		return nil
	}

	return readFileIntoLines(returnSome, startingOffset, logPath, vlog)
}

// Meat of LogStream() broken out to here so it can be tested
func readFileIntoLines(returnSome func(bool, string, []string, int64, float64) error, startingOffset int64, logPath string, vlog *util.VLog) error {
	// If the file has no new bytes, just return empty array and unchanged next offset.
	fInfo, err := os.Lstat(logPath)
	if err != nil {
		msg := fmt.Sprintf("lstat failed on %s: %v", logPath, err)
		log.Println("error: ", msg)
		returnSome(false, msg, []string{}, startingOffset, float64(0))
		return nil
	}
	fileLen := fInfo.Size()
	if fileLen == startingOffset {
		// File has no new bytes, return empty array
		returnSome(true, "", []string{}, startingOffset, float64(100))
		return nil
	}

	// Open the file
	f, err := os.Open(logPath)
	if err != nil {
		msg := fmt.Sprintf("open failed on %s: %v", logPath, err)
		log.Println("error: ", msg)
		returnSome(false, msg, []string{}, startingOffset, float64(0))
		return nil
	}
	defer f.Close()

	// Here file size is either greater than startingOffset (meaning new bytes appended) or
	// startingOffset is greater than filesize (meaning file was truncated to zero and restarted)
	if fileLen < startingOffset { // file was truncated and restarted
		startingOffset = 0
	}
	linesReturned := 0
	remnantBuf := make([]byte, 0, MaxLineLengthBytes)
	currOffset := startingOffset
	for {
		buf := make([]byte, MaxLineLengthBytes)
		n, err := f.ReadAt(buf, currOffset)
		if err != nil && !errors.Is(err, io.EOF) {
			msg := fmt.Sprintf("readat failed on %s: %v", logPath, err)
			log.Println("error: ", msg)
			returnSome(false, msg, []string{}, startingOffset, float64(100)*float64(startingOffset)/float64(fileLen))
			return nil
		}
		currOffset += int64(n)
		//vlog.Printf("remnantBuf(1) = <%s>", string(remnantBuf))
		fullBuf := append(remnantBuf, buf...)
		if !errors.Is(err, io.EOF) {
			//vlog.Printf("fullBuf = $%s$", string(fullBuf))
			var lines []string
			lines, remnantBuf = splitToLines(fullBuf, false, vlog)
			//vlog.Printf("remnantBuf(2) = <%s>", string(remnantBuf))
			if len(lines) > 0 {
				linesReturned += len(lines)
				e := returnSome(true, "", lines, currOffset-int64(len(remnantBuf)), float64(100)*float64(currOffset)/float64(fileLen))
				if e != nil || linesReturned > MaxLinesToReturnInSingleCall {
					return nil
				}
			}
		} else {
			// EOF
			var lines []string
			//vlog.Printf("fullBuf (EOF) = $%s$", string(fullBuf))
			lines, remnantBuf = splitToLines(fullBuf, true, vlog)
			logAssert(len(remnantBuf) == 0, "len(remnantBuf) should be zero at EOF", vlog)
			logAssert(currOffset >= fileLen, "currOffset should >= fileLen at EOF", vlog)
			returnSome(true, "", lines, currOffset, float64(100)*float64(currOffset)/float64(fileLen))
			return nil
		}
	}
}

func splitToLines(buf []byte, isEOF bool, vlog *util.VLog) (lines []string, remnantBuf []byte) {
	if len(buf) == 0 {
		return []string{}, []byte{}
	}

	s := string(buf)
	//vlog.Printf("s = '%s'", s)
	lines = strings.Split(s, "\n")

	// take back last (partial) line into remnant buf unless this is the end of the file
	if !isEOF {
		partialLine := lines[len(lines)-1]
		//vlog.Printf("partialLine = '%s'", partialLine)
		remnantBuf = []byte(partialLine)
		lines = lines[:len(lines)-1]
	} else {
		//vlog.Printf("s = '%s'", s)
		remnantBuf = make([]byte, 0)
	}

	// Strip out blank (all 0x00's) last line if present
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		if allZeros(lastLine) {
			lines = lines[:len(lines)-1]
		}
	}

	return lines, remnantBuf
}

func allZeros(s string) bool {
	buf := []byte(s)
	for _, b := range buf {
		if b != 0x00 {
			return false
		}
	}
	return true
}

func logAssert(b bool, msg string, vlog *util.VLog) {
	if !b {
		vlog.Printf("assertion failed: %s", msg)
	}
}
