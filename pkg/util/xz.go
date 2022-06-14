package util

import (
	"bytes"
	"fmt"
	"io"
	"log"

	"github.com/ulikunitz/xz"
)

func XZCompress(buf []byte) ([]byte, error) {
	var outbuf bytes.Buffer

	w, err := xz.NewWriter(&outbuf)
	if err != nil {
		log.Printf("error: xzCompress: %v", err)
		return nil, err
	}

	n, err := io.WriteString(w, string(buf))
	if err != nil {
		log.Printf("error: xzCompress: %v", err)
		return nil, err
	}
	if n != len(buf) {
		msg := fmt.Sprintf("error: xzCompress: wrote %d bytes but should have written %d bytes", n, len(buf))
		return nil, fmt.Errorf(msg)
	}

	if err := w.Close(); err != nil {
		log.Printf("error: xzCompress: %v", err)
		return nil, err
	}

	return outbuf.Bytes(), nil
}

func XZUncompress(buf []byte) ([]byte, error) {
	inbuf := bytes.NewBuffer(buf)
	var outbuf bytes.Buffer

	r, err := xz.NewReader(inbuf)
	if err != nil {
		log.Printf("error: xzUncmpress: %v", err)
		return nil, err
	}

	_, err = io.Copy(&outbuf, r)
	if err != nil {
		log.Printf("error: xzUncmpress: %v", err)
		return nil, err
	}

	return outbuf.Bytes(), nil
}
