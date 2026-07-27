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

func TestTailerRejectsEmptyPath(t *testing.T) {
	err := (Tailer{}).Run(context.Background(), make(chan Event))
	if err == nil {
		t.Fatal("Run succeeded")
	}
}
