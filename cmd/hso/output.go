package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"time"

	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

const (
	outputBufferSize   = 4 << 10
	maxOutputLineBytes = 16 << 10
)

func readAndCloseServerOutput(input *io.PipeReader, logs chan serverlog.Entry) {
	defer input.Close()
	readServerOutput(input, logs)
}

func readServerOutput(input io.Reader, logs chan serverlog.Entry) {
	reader := bufio.NewReaderSize(input, outputBufferSize)
	line := make([]byte, 0, outputBufferSize)
	truncated := false

	for {
		fragment, err := reader.ReadSlice('\n')
		if remaining := maxOutputLineBytes - len(line); remaining > 0 {
			line = append(line, fragment[:min(len(fragment), remaining)]...)
			truncated = truncated || len(fragment) > remaining
		} else if len(fragment) > 0 {
			truncated = true
		}

		switch {
		case err == nil:
			offerLog(logs, parseOutputLine(line, truncated))
			line = line[:0]
			truncated = false
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(line) > 0 {
				offerLog(logs, parseOutputLine(line, truncated))
			}
			return
		default:
			return
		}
	}
}

func parseOutputLine(line []byte, truncated bool) serverlog.Entry {
	receivedAt := time.Now()
	line = bytes.TrimRight(line, "\r\n")
	if truncated {
		line = append(line, []byte("…")...)
	}
	entry := serverlog.Parse(string(line))
	if entry.Timestamp.IsZero() {
		entry.Timestamp = receivedAt
		entry.TimestampSource = serverlog.TimestampReceived
	}
	entry.Raw = ""
	return entry
}

func offerLog(logs chan serverlog.Entry, entry serverlog.Entry) {
	select {
	case logs <- entry:
		return
	default:
	}
	select {
	case <-logs:
	default:
	}
	select {
	case logs <- entry:
	default:
	}
}
