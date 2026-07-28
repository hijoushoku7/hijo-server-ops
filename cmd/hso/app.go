package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/gclog"
	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
	"github.com/hijoushoku7/hijo-server-ops/internal/ui"
)

const (
	logQueueSize       = 16
	outputBufferSize   = 4 << 10
	maxOutputLineBytes = 16 << 10
	metricsInterval    = time.Second
	gracefulStopWait   = 60 * time.Second
)

type stoppableServer interface {
	Done() <-chan struct{}
	Send(string) error
	Signal(os.Signal) error
}

func runTUI(cfg config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := ui.New()
	program := tea.NewProgram(model, tea.WithContext(ctx))
	logs := make(chan serverlog.Entry, logQueueSize)
	go pumpLogs(ctx, program, logs)

	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	go readServerOutput(stdoutReader, logs)
	go readServerOutput(stderrReader, logs)

	server, err := process.Start(process.Options{
		Command: cfg.Server.Command,
		WorkDir: cfg.Server.WorkDir,
		Stdout:  stdoutWriter,
		Stderr:  stderrWriter,
	})
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		_ = stderrReader.Close()
		_ = stderrWriter.Close()
		return err
	}

	var javaFound atomic.Bool
	go waitForServer(server, stdoutWriter, stderrWriter, &javaFound, program)
	go findJava(ctx, server, &javaFound, program)
	go streamGC(ctx, server.GCLogPath(), program)

	_, programErr := program.Run()
	cancel()

	stopErr := stopServer(server, javaFound.Load(), gracefulStopWait)
	_ = stdoutReader.Close()
	_ = stderrReader.Close()

	if model.Err() != nil {
		return model.Err()
	}
	if stopErr != nil {
		return stopErr
	}
	if ui.IsExpectedExit(programErr) {
		return nil
	}
	return programErr
}

func stopServer(server stoppableServer, javaFound bool, wait time.Duration) error {
	select {
	case <-server.Done():
		return nil
	default:
	}

	if javaFound {
		if err := server.Send("stop"); err == nil {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-server.Done():
				return nil
			case <-timer.C:
			}
		}
	}

	if err := server.Signal(syscall.SIGTERM); err != nil {
		select {
		case <-server.Done():
			return nil
		default:
			return fmt.Errorf("サーバーを停止する: %w", err)
		}
	}
	<-server.Done()
	return nil
}

func waitForServer(
	server *process.Process,
	stdout *io.PipeWriter,
	stderr *io.PipeWriter,
	javaFound *atomic.Bool,
	program *tea.Program,
) {
	waitErr := server.Wait()
	_ = stdout.Close()
	_ = stderr.Close()
	if !javaFound.Load() && waitErr == nil {
		waitErr = errors.New("起動スクリプトがjavaプロセスを開始せずに終了しました")
	}
	program.Send(ui.ProcessExitedMsg{Err: waitErr})
}

func findJava(
	ctx context.Context,
	server *process.Process,
	javaFound *atomic.Bool,
	program *tea.Program,
) {
	finder := process.JavaFinder{
		ProcessGroup:      server.ServerPID(),
		ExpectedStartTime: server.RootStartTime(),
	}
	pid, err := finder.Wait(ctx, server.PID())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		program.Send(ui.FatalMsg{
			Err: fmt.Errorf("javaプロセスの特定: %w", err),
		})
		_ = server.Signal(syscall.SIGTERM)
		return
	}

	javaFound.Store(true)
	program.Send(ui.JavaFoundMsg{PID: pid})
	go collectMetrics(ctx, pid, program)
}

func collectMetrics(ctx context.Context, pid int, program *tea.Program) {
	ticker := time.NewTicker(metricsInterval)
	defer ticker.Stop()

	var reader *hsperfdata.Reader
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()

	for {
		var metrics hsperfdata.Metrics
		if reader == nil {
			opened, err := hsperfdata.Open(pid)
			if err == nil {
				reader = opened
			}
		}
		if reader != nil {
			snapshot, err := reader.Sample()
			if err != nil {
				_ = reader.Close()
				reader = nil
			} else {
				metrics = snapshot.Metrics()
			}
		}

		memory, _ := procstats.ReadMemory(pid)
		program.Send(ui.MetricsMsg{JVM: metrics, Memory: memory})

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func streamGC(ctx context.Context, path string, program *tea.Program) {
	events := make(chan gclog.Event, 4)
	go func() {
		_ = (gclog.Tailer{Path: path}).Run(ctx, events)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			program.Send(ui.GCMsg{Event: event})
		}
	}
}

func pumpLogs(
	ctx context.Context,
	program *tea.Program,
	logs <-chan serverlog.Entry,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-logs:
			program.Send(ui.LogMsg{Entry: entry})
		}
	}
}

func readServerOutput(input io.Reader, logs chan serverlog.Entry) {
	reader := bufio.NewReaderSize(input, outputBufferSize)
	line := make([]byte, 0, outputBufferSize)
	truncated := false

	for {
		fragment, err := reader.ReadSlice('\n')
		if remaining := maxOutputLineBytes - len(line); remaining > 0 {
			line = append(line, fragment[:min(len(fragment), remaining)]...)
			if len(fragment) > remaining {
				truncated = true
			}
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
	line = bytes.TrimRight(line, "\r\n")
	if truncated {
		line = append(line, []byte("…")...)
	}
	entry := serverlog.Parse(string(line))
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
