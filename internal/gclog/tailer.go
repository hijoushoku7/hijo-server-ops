package gclog

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Tailer struct {
	Path         string
	PollInterval time.Duration
}

func (t Tailer) Run(ctx context.Context, events chan<- Event) error {
	if strings.TrimSpace(t.Path) == "" {
		return errors.New("GCログのパスが空です")
	}

	file, err := t.waitForFile(ctx)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var partial string
	for {
		line, readErr := reader.ReadString('\n')
		partial += line
		switch {
		case readErr == nil:
			if event, ok := Parse(partial); ok {
				select {
				case events <- event:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			partial = ""
		case errors.Is(readErr, io.EOF):
			if err := wait(ctx, t.pollInterval()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("GCログを読む: %w", readErr)
		}
	}
}

func (t Tailer) waitForFile(ctx context.Context) (*os.File, error) {
	for {
		file, err := os.Open(t.Path)
		switch {
		case err == nil:
			return file, nil
		case !errors.Is(err, os.ErrNotExist):
			return nil, fmt.Errorf("GCログを開く: %w", err)
		}
		if err := wait(ctx, t.pollInterval()); err != nil {
			return nil, err
		}
	}
}

func (t Tailer) pollInterval() time.Duration {
	if t.PollInterval <= 0 {
		return 100 * time.Millisecond
	}
	return t.PollInterval
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
