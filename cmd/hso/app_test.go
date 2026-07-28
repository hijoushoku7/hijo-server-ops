package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

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

func TestStopServerSendsMinecraftStop(t *testing.T) {
	server := newFakeStoppableServer()
	server.finishOnSend = true

	if err := stopServer(server, true, time.Second); err != nil {
		t.Fatal(err)
	}
	if len(server.commands) != 1 || server.commands[0] != "stop" {
		t.Fatalf("commands = %v", server.commands)
	}
	if len(server.signals) != 0 {
		t.Fatalf("signals = %v", server.signals)
	}
}

func TestStopServerFallsBackToSIGTERMAfterTimeout(t *testing.T) {
	server := newFakeStoppableServer()
	server.finishOnSignal = true

	if err := stopServer(server, true, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(server.commands) != 1 || server.commands[0] != "stop" {
		t.Fatalf("commands = %v", server.commands)
	}
	if len(server.signals) != 1 || server.signals[0] != syscall.SIGTERM {
		t.Fatalf("signals = %v", server.signals)
	}
}

func TestStopServerUsesSIGTERMBeforeJavaIsFound(t *testing.T) {
	server := newFakeStoppableServer()
	server.finishOnSignal = true

	if err := stopServer(server, false, time.Second); err != nil {
		t.Fatal(err)
	}
	if len(server.commands) != 0 {
		t.Fatalf("commands = %v", server.commands)
	}
	if len(server.signals) != 1 || server.signals[0] != syscall.SIGTERM {
		t.Fatalf("signals = %v", server.signals)
	}
}

func TestStopServerReturnsSignalError(t *testing.T) {
	server := newFakeStoppableServer()
	server.signalErr = errors.New("signal failed")

	if err := stopServer(server, false, time.Second); err == nil {
		t.Fatal("expected an error")
	}
}

type fakeStoppableServer struct {
	done           chan struct{}
	commands       []string
	signals        []os.Signal
	finishOnSend   bool
	finishOnSignal bool
	signalErr      error
}

func newFakeStoppableServer() *fakeStoppableServer {
	return &fakeStoppableServer{done: make(chan struct{})}
}

func (s *fakeStoppableServer) Done() <-chan struct{} {
	return s.done
}

func (s *fakeStoppableServer) Send(command string) error {
	s.commands = append(s.commands, command)
	if s.finishOnSend {
		close(s.done)
	}
	return nil
}

func (s *fakeStoppableServer) Signal(signal os.Signal) error {
	s.signals = append(s.signals, signal)
	if s.signalErr != nil {
		return s.signalErr
	}
	if s.finishOnSignal {
		close(s.done)
	}
	return nil
}
