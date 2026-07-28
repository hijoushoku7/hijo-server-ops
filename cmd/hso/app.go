package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
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
	actionQueueSize    = 4
	initialGeneration  = 1
)

type stoppableServer interface {
	Done() <-chan struct{}
	Send(string) error
	Signal(os.Signal) error
}

func runTUI(cfg config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actions := make(chan ui.Action, actionQueueSize)
	model := ui.New(actions, initialGeneration)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	controller := newServerController(ctx, cfg, program)
	if err := controller.start(initialGeneration, false); err != nil {
		return err
	}
	go controller.handleActions(actions)

	_, programErr := program.Run()
	cancel()

	stopErr := controller.shutdown()

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

type serverRuntime struct {
	server       *process.Process
	generation   uint64
	javaFound    atomic.Bool
	cancel       context.CancelFunc
	stdoutReader *io.PipeReader
	stdoutWriter *io.PipeWriter
	stderrReader *io.PipeReader
	stderrWriter *io.PipeWriter
}

func (runtime *serverRuntime) close() {
	runtime.cancel()
	_ = runtime.stdoutReader.Close()
	_ = runtime.stdoutWriter.Close()
	_ = runtime.stderrReader.Close()
	_ = runtime.stderrWriter.Close()
}

type serverController struct {
	ctx     context.Context
	cfg     config.Config
	program *tea.Program

	operation sync.Mutex
	currentMu sync.Mutex
	current   *serverRuntime
}

func newServerController(
	ctx context.Context,
	cfg config.Config,
	program *tea.Program,
) *serverController {
	return &serverController{
		ctx:     ctx,
		cfg:     cfg,
		program: program,
	}
}

func (controller *serverController) start(
	generation uint64,
	announce bool,
) error {
	if err := controller.ctx.Err(); err != nil {
		return err
	}
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	server, err := process.Start(process.Options{
		Command: controller.cfg.Server.Command,
		WorkDir: controller.cfg.Server.WorkDir,
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

	runtimeCtx, runtimeCancel := context.WithCancel(controller.ctx)
	runtime := &serverRuntime{
		server:       server,
		generation:   generation,
		cancel:       runtimeCancel,
		stdoutReader: stdoutReader,
		stdoutWriter: stdoutWriter,
		stderrReader: stderrReader,
		stderrWriter: stderrWriter,
	}
	controller.currentMu.Lock()
	controller.current = runtime
	controller.currentMu.Unlock()

	if announce {
		controller.program.Send(ui.ServerStartedMsg{Generation: generation})
	}

	logs := make(chan serverlog.Entry, logQueueSize)
	go pumpLogs(runtimeCtx, controller.program, logs, generation)
	go readAndCloseServerOutput(stdoutReader, logs)
	go readAndCloseServerOutput(stderrReader, logs)
	go controller.waitForServer(runtime)
	go findJava(
		runtimeCtx,
		server,
		&runtime.javaFound,
		controller.program,
		generation,
	)
	go streamGC(
		runtimeCtx,
		server.GCLogPath(),
		controller.program,
		generation,
	)
	return nil
}

func (controller *serverController) handleActions(actions <-chan ui.Action) {
	for {
		select {
		case <-controller.ctx.Done():
			return
		case action := <-actions:
			switch action.Kind {
			case ui.ActionRestart:
				controller.restart()
			case ui.ActionSendCommand:
				controller.sendCommand(action)
			}
		}
	}
}

func (controller *serverController) sendCommand(action ui.Action) {
	controller.operation.Lock()
	defer controller.operation.Unlock()

	runtime := controller.currentRuntime()
	if runtime == nil {
		controller.program.Send(ui.ActionResultMsg{
			Action: action,
			Err:    errors.New("サーバーは停止しています"),
		})
		return
	}
	controller.program.Send(ui.ActionResultMsg{
		Action: action,
		Err:    runtime.server.Send(action.Command),
	})
}

func (controller *serverController) restart() {
	controller.operation.Lock()
	defer controller.operation.Unlock()

	runtime := controller.currentRuntime()
	if runtime == nil {
		controller.program.Send(ui.ActionResultMsg{
			Action: ui.Action{Kind: ui.ActionRestart},
			Err:    errors.New("サーバーは停止しています"),
		})
		return
	}
	if !runtime.javaFound.Load() {
		controller.program.Send(ui.ActionResultMsg{
			Action: ui.Action{Kind: ui.ActionRestart},
			Err:    errors.New("javaプロセスの起動完了後に再起動できます"),
		})
		return
	}
	if !controller.detach(runtime) {
		return
	}

	runtime.cancel()
	controller.program.Send(ui.ServerRestartingMsg{})
	if err := stopServer(
		runtime.server,
		runtime.javaFound.Load(),
		gracefulStopWait,
	); err != nil {
		runtime.close()
		controller.program.Send(ui.FatalMsg{Err: err})
		return
	}
	runtime.close()
	if controller.ctx.Err() != nil {
		return
	}

	nextGeneration := runtime.generation + 1
	if err := controller.start(nextGeneration, true); err != nil {
		controller.program.Send(ui.FatalMsg{
			Err: fmt.Errorf("サーバーを再起動する: %w", err),
		})
	}
}

func (controller *serverController) shutdown() error {
	controller.operation.Lock()
	defer controller.operation.Unlock()

	runtime := controller.detachCurrent()
	if runtime == nil {
		return nil
	}
	runtime.cancel()
	err := stopServer(
		runtime.server,
		runtime.javaFound.Load(),
		gracefulStopWait,
	)
	runtime.close()
	return err
}

func (controller *serverController) waitForServer(runtime *serverRuntime) {
	waitErr := runtime.server.Wait()
	_ = runtime.stdoutWriter.Close()
	_ = runtime.stderrWriter.Close()
	runtime.cancel()
	if !controller.detach(runtime) {
		return
	}
	if !runtime.javaFound.Load() && waitErr == nil {
		waitErr = errors.New("起動スクリプトがjavaプロセスを開始せずに終了しました")
	}
	controller.program.Send(ui.ProcessExitedMsg{
		Generation: runtime.generation,
		Err:        waitErr,
	})
}

func (controller *serverController) currentRuntime() *serverRuntime {
	controller.currentMu.Lock()
	defer controller.currentMu.Unlock()
	return controller.current
}

func (controller *serverController) detachCurrent() *serverRuntime {
	controller.currentMu.Lock()
	defer controller.currentMu.Unlock()
	runtime := controller.current
	controller.current = nil
	return runtime
}

func (controller *serverController) detach(runtime *serverRuntime) bool {
	controller.currentMu.Lock()
	defer controller.currentMu.Unlock()
	if controller.current != runtime {
		return false
	}
	controller.current = nil
	return true
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

func readAndCloseServerOutput(
	input *io.PipeReader,
	logs chan serverlog.Entry,
) {
	defer input.Close()
	readServerOutput(input, logs)
}

func findJava(
	ctx context.Context,
	server *process.Process,
	javaFound *atomic.Bool,
	program *tea.Program,
	generation uint64,
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
			Generation: generation,
			Err:        fmt.Errorf("javaプロセスの特定: %w", err),
		})
		_ = server.Signal(syscall.SIGTERM)
		return
	}

	javaFound.Store(true)
	program.Send(ui.JavaFoundMsg{Generation: generation, PID: pid})
	go collectMetrics(ctx, pid, program, generation)
}

func collectMetrics(
	ctx context.Context,
	pid int,
	program *tea.Program,
	generation uint64,
) {
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
		program.Send(ui.MetricsMsg{
			Generation: generation,
			JVM:        metrics,
			Memory:     memory,
		})

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func streamGC(
	ctx context.Context,
	path string,
	program *tea.Program,
	generation uint64,
) {
	events := make(chan gclog.Event, 4)
	go func() {
		_ = (gclog.Tailer{Path: path}).Run(ctx, events)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			program.Send(ui.GCMsg{Generation: generation, Event: event})
		}
	}
}

func pumpLogs(
	ctx context.Context,
	program *tea.Program,
	logs <-chan serverlog.Entry,
	generation uint64,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-logs:
			program.Send(ui.LogMsg{
				Generation: generation,
				Entry:      entry,
			})
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
