package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

func TestReadServerOutputParsesLines(t *testing.T) {
	logs := make(chan serverlog.Entry, 4)
	readServerOutput(strings.NewReader(
		"[12:00:00] [Server thread/INFO]: <alice> hello\n"+
			"[12:00:01] [Server thread/INFO]: alice joined the game\n",
	), logs)

	first := <-logs
	second := <-logs
	if first.Kind != serverlog.KindChat || first.Chat != "hello" {
		t.Fatalf("first = %#v", first)
	}
	if second.Kind != serverlog.KindPlayerJoin || second.Player != "alice" {
		t.Fatalf("second = %#v", second)
	}
	if first.Raw != "" || second.Raw != "" {
		t.Fatalf("raw log was retained")
	}
}

func TestReadServerOutputBoundsLongLinesAndContinues(t *testing.T) {
	logs := make(chan serverlog.Entry, 4)
	input := bytes.Repeat([]byte("x"), maxOutputLineBytes+100)
	input = append(input, '\n')
	input = append(input, []byte("next\n")...)
	readServerOutput(bytes.NewReader(input), logs)

	first := <-logs
	second := <-logs
	if len(first.Message) > maxOutputLineBytes+len("…") {
		t.Fatalf("line length = %d", len(first.Message))
	}
	if !strings.HasSuffix(first.Message, "…") {
		t.Fatalf("first = %q", first.Message)
	}
	if second.Message != "next" {
		t.Fatalf("second = %q", second.Message)
	}
}

func TestOfferLogDropsOldestWhenQueueIsFull(t *testing.T) {
	logs := make(chan serverlog.Entry, 2)
	offerLog(logs, serverlog.Entry{Message: "one"})
	offerLog(logs, serverlog.Entry{Message: "two"})
	offerLog(logs, serverlog.Entry{Message: "three"})

	first := <-logs
	second := <-logs
	if first.Message != "two" || second.Message != "three" {
		t.Fatalf("logs = %q, %q", first.Message, second.Message)
	}
}
