package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
	"github.com/hijoushoku7/hijo-server-ops/internal/ui"
)

const (
	logQueueSize     = 16
	gracefulStopWait = 60 * time.Second
)

type stoppableServer interface {
	Done() <-chan struct{}
	Send(string) error
	Signal(os.Signal) error
}

type outputPipe struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func newOutputPipe() outputPipe {
	reader, writer := io.Pipe()
	return outputPipe{reader: reader, writer: writer}
}

func (pipe outputPipe) close() {
	_ = pipe.reader.Close()
	_ = pipe.writer.Close()
}

type serverRuntime struct {
	server       *process.Process
	generation   uint64
	startedAt    time.Time
	javaFound    atomic.Bool
	expectedExit atomic.Bool
	cancel       context.CancelFunc
	stdout       outputPipe
	stderr       outputPipe
}

func (runtime *serverRuntime) close() {
	runtime.cancel()
	runtime.stdout.close()
	runtime.stderr.close()
}

type serverController struct {
	ctx     context.Context
	cfg     config.Config
	program *tea.Program

	operation sync.Mutex
	currentMu sync.Mutex
	current   *serverRuntime
	// runtime を手放した後も、次の起動世代を決められるよう保持する。
	lastGeneration uint64
}

func newServerController(
	ctx context.Context,
	cfg config.Config,
	program *tea.Program,
) *serverController {
	return &serverController{ctx: ctx, cfg: cfg, program: program}
}

func (controller *serverController) start(generation uint64, announce bool) error {
	if err := controller.ctx.Err(); err != nil {
		return err
	}
	stdout := newOutputPipe()
	stderr := newOutputPipe()
	server, err := process.Start(process.Options{
		Command: controller.cfg.Server.Command,
		WorkDir: controller.cfg.Server.WorkDir,
		Stdout:  stdout.writer,
		Stderr:  stderr.writer,
	})
	if err != nil {
		stdout.close()
		stderr.close()
		return err
	}

	runtimeCtx, runtimeCancel := context.WithCancel(controller.ctx)
	runtime := &serverRuntime{
		server:     server,
		generation: generation,
		startedAt:  time.Now(),
		cancel:     runtimeCancel,
		stdout:     stdout,
		stderr:     stderr,
	}
	controller.currentMu.Lock()
	controller.current = runtime
	controller.lastGeneration = generation
	controller.currentMu.Unlock()

	if announce {
		controller.program.Send(ui.ServerStartedMsg{
			Generation: generation,
			StartedAt:  runtime.startedAt,
		})
	}

	logs := make(chan serverlog.Entry, logQueueSize)
	go pumpLogs(runtimeCtx, controller.program, logs, generation)
	go readAndCloseServerOutput(stdout.reader, logs)
	go readAndCloseServerOutput(stderr.reader, logs)
	go controller.waitForServer(runtime)
	go findJava(runtimeCtx, server, &runtime.javaFound, controller.program, generation)
	go streamGC(runtimeCtx, server.GCLogPath(), controller.program, generation)
	go resolveServerAddress(
		runtimeCtx,
		controller.cfg.Server.WorkDir,
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
		controller.program.Send(ui.ActionResultMsg{Action: action, Err: msg.ErrServerStopped})
		return
	}
	isStop := strings.EqualFold(strings.TrimSpace(action.Command), "stop")
	if isStop {
		runtime.expectedExit.Store(true)
	}
	err := runtime.server.Send(action.Command)
	if err != nil && isStop {
		runtime.expectedExit.Store(false)
	}
	controller.program.Send(ui.ActionResultMsg{Action: action, Err: err})
}

func (controller *serverController) restart() {
	controller.operation.Lock()
	defer controller.operation.Unlock()

	runtime := controller.currentRuntime()
	if runtime == nil {
		controller.program.Send(ui.ServerRestartingMsg{})
		if err := controller.start(controller.latestGeneration()+1, true); err != nil {
			controller.program.Send(ui.FatalMsg{Err: msg.RestartFailed(err)})
		}
		return
	}
	if !runtime.javaFound.Load() {
		controller.sendRestartResult(msg.ErrRestartBeforeJava)
		return
	}
	if !controller.detach(runtime) {
		return
	}

	runtime.cancel()
	controller.program.Send(ui.ServerRestartingMsg{})
	if err := stopServer(runtime.server, true, gracefulStopWait); err != nil {
		runtime.close()
		controller.program.Send(ui.FatalMsg{Err: err})
		return
	}
	runtime.close()
	if controller.ctx.Err() != nil {
		return
	}

	if err := controller.start(runtime.generation+1, true); err != nil {
		controller.program.Send(ui.FatalMsg{Err: msg.RestartFailed(err)})
	}
}

func (controller *serverController) sendRestartResult(err error) {
	controller.program.Send(ui.ActionResultMsg{
		Action: ui.Action{Kind: ui.ActionRestart},
		Err:    err,
	})
}

func (controller *serverController) shutdown() error {
	controller.operation.Lock()
	defer controller.operation.Unlock()

	runtime := controller.detachCurrent()
	if runtime == nil {
		return nil
	}
	runtime.cancel()
	err := stopServer(runtime.server, runtime.javaFound.Load(), gracefulStopWait)
	runtime.close()
	return err
}

func (controller *serverController) waitForServer(runtime *serverRuntime) {
	waitErr := runtime.server.Wait()
	_ = runtime.stdout.writer.Close()
	_ = runtime.stderr.writer.Close()
	runtime.cancel()
	if !controller.detach(runtime) {
		return
	}
	controller.program.Send(ui.ProcessExitedMsg{
		Generation: runtime.generation,
		ExitCode:   processExitCode(waitErr),
		StartedAt:  runtime.startedAt,
		ExitedAt:   time.Now(),
		Err: serverExitError(
			waitErr,
			runtime.javaFound.Load(),
			runtime.expectedExit.Load(),
		),
	})
}

func processExitCode(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func serverExitError(waitErr error, javaFound, expected bool) error {
	if waitErr != nil || expected {
		return waitErr
	}
	if !javaFound {
		return msg.ErrScriptExitedWithoutJava
	}
	return msg.ErrServerExited
}

func (controller *serverController) currentRuntime() *serverRuntime {
	controller.currentMu.Lock()
	defer controller.currentMu.Unlock()
	return controller.current
}

func (controller *serverController) latestGeneration() uint64 {
	controller.currentMu.Lock()
	defer controller.currentMu.Unlock()
	return controller.lastGeneration
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
			return msg.StopFailed(err)
		}
	}
	<-server.Done()
	return nil
}
