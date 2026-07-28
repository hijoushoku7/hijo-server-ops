package gclog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailerWaitsForFileAndReadsCompleteLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gc.log")
	events := make(chan Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		result <- (Tailer{
			Path:         path,
			PollInterval: time.Millisecond,
		}).Run(ctx, events)
	}()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(
		"[1.000s][info][gc] GC(0) Pause Young 10M->",
	); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	select {
	case event := <-events:
		t.Fatalf("received partial event: %#v", event)
	default:
	}
	if _, err := file.WriteString("2M(100M) 1.000ms\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-events:
		assertBytes(t, event.After, 2<<20)
		cancel()
	case <-time.After(time.Second):
		t.Fatal("event was not received")
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
}

func TestTailerCanBeCanceledBeforeFileExists(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := (Tailer{
		Path:         filepath.Join(t.TempDir(), "missing.log"),
		PollInterval: time.Millisecond,
	}).Run(ctx, make(chan Event))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
}

func TestTailerFollowsRotatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gc.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		result <- (Tailer{
			Path:         path,
			PollInterval: time.Millisecond,
		}).Run(ctx, events)
	}()

	appendGCLine(t, path, "[1.000s][info][gc] GC(0) Pause Young 10M->2M(100M) 1.000ms\n")
	waitForEvent(t, events, 0)

	if err := os.Rename(path, path+".0"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		path,
		[]byte("[2.000s][info][gc] GC(1) Pause Young 20M->3M(100M) 2.000ms\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	waitForEvent(t, events, 1)
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
}

func TestTailerRejectsEmptyPath(t *testing.T) {
	err := (Tailer{}).Run(context.Background(), make(chan Event))
	if err == nil {
		t.Fatal("Run succeeded")
	}
}

func appendGCLine(t *testing.T, path, line string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(line); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func waitForEvent(t *testing.T, events <-chan Event, id uint64) {
	t.Helper()
	select {
	case event := <-events:
		if event.ID != id {
			t.Fatalf("event ID = %d, want %d", event.ID, id)
		}
	case <-time.After(time.Second):
		t.Fatalf("event %d was not received", id)
	}
}
