package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/gclog"
	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serveraddr"
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

func runTUI(configPath string, cfg config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actions := make(chan ui.Action, actionQueueSize)
	save := func(settings ui.Settings) error {
		return saveSettings(configPath, cfg, settings)
	}
	model := ui.New(actions, save, initialGeneration, settingsFrom(cfg))
	program := tea.NewProgram(model, tea.WithContext(ctx))

	controller := newServerController(ctx, configPath, cfg, program)
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
	expectedExit atomic.Bool
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
	ctx        context.Context
	configPath string
	cfg        config.Config
	program    *tea.Program

	operation sync.Mutex
	currentMu sync.Mutex
	current   *serverRuntime
}

func newServerController(
	ctx context.Context,
	configPath string,
	cfg config.Config,
	program *tea.Program,
) *serverController {
	return &serverController{
		ctx:        ctx,
		configPath: configPath,
		cfg:        cfg,
		program:    program,
	}
}

// settingsFrom は設定ファイルの配色プリセットを UI へ渡す。空欄は既定の
// ままにする。
func settingsFrom(cfg config.Config) ui.Settings {
	settings := ui.DefaultSettings()
	if value := cfg.UI.Theme.Frame; value != "" {
		settings.FramePreset = value
	}
	if value := cfg.UI.Theme.Graph; value != "" {
		settings.GraphPreset = value
	}
	if value := cfg.UI.Theme.Meter; value != "" {
		settings.MeterPreset = value
	}
	if value := cfg.UI.Theme.Title; value != "" {
		settings.TitlePreset = value
	}
	if value := cfg.UI.Theme.Selection; value != "" {
		settings.SelectionPreset = value
	}
	return settings
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

// saveSettings は設定モーダルの変更を設定ファイルへ書き戻す。画面を描く
// goroutine から直接呼ばれるので、ここではファイルを書くだけにする。
func saveSettings(
	configPath string,
	cfg config.Config,
	settings ui.Settings,
) error {
	cfg.UI.Theme = config.Theme{
		Frame:     settings.FramePreset,
		Graph:     settings.GraphPreset,
		Meter:     settings.MeterPreset,
		Title:     settings.TitlePreset,
		Selection: settings.SelectionPreset,
	}
	return config.Save(configPath, cfg)
}

func (controller *serverController) sendCommand(action ui.Action) {
	controller.operation.Lock()
	defer controller.operation.Unlock()

	runtime := controller.currentRuntime()
	if runtime == nil {
		controller.program.Send(ui.ActionResultMsg{
			Action: action,
			Err:    msg.ErrServerStopped,
		})
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
		controller.program.Send(ui.ActionResultMsg{
			Action: ui.Action{Kind: ui.ActionRestart},
			Err:    msg.ErrServerStopped,
		})
		return
	}
	if !runtime.javaFound.Load() {
		controller.program.Send(ui.ActionResultMsg{
			Action: ui.Action{Kind: ui.ActionRestart},
			Err:    msg.ErrRestartBeforeJava,
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
			Err: msg.RestartFailed(err),
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
	waitErr = serverExitError(
		waitErr,
		runtime.javaFound.Load(),
		runtime.expectedExit.Load(),
	)
	controller.program.Send(ui.ProcessExitedMsg{
		Generation: runtime.generation,
		Err:        waitErr,
	})
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
			Err:        msg.FindJavaFailed(err),
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
	var previousCPU procstats.Duration
	var previousSample time.Time
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()

	for {
		var metrics hsperfdata.Metrics
		var jvmErr error
		if reader == nil {
			opened, err := hsperfdata.Open(pid)
			if err == nil {
				reader = opened
			} else {
				jvmErr = err
			}
		}
		if reader != nil {
			snapshot, err := reader.Sample()
			if err != nil {
				_ = reader.Close()
				reader = nil
				jvmErr = err
			} else {
				metrics = snapshot.Metrics()
				if !metrics.Heap.Used.Available ||
					!metrics.Heap.Committed.Available {
					jvmErr = msg.ErrHeapCountersUnavailable
				}
			}
		}

		memory, memoryErr := procstats.ReadMemory(pid)
		if memoryErr == nil && !memory.RSS.Available {
			memoryErr = msg.ErrRSSUnavailable
		}
		cpuTime, _ := procstats.ReadCPUTime(pid)
		sampledAt := time.Now()
		cpu, cpuAvailable := calculateCPU(
			previousCPU,
			cpuTime,
			sampledAt.Sub(previousSample),
		)
		previousCPU = cpuTime
		previousSample = sampledAt
		program.Send(ui.MetricsMsg{
			Generation:   generation,
			JVM:          metrics,
			Memory:       memory,
			CPU:          cpu,
			CPUAvailable: cpuAvailable,
			JVMError:     errorText(jvmErr),
			MemoryError:  errorText(memoryErr),
		})

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func calculateCPU(
	previous procstats.Duration,
	current procstats.Duration,
	elapsed time.Duration,
) (float64, bool) {
	if !previous.Available || !current.Available ||
		current.Value < previous.Value || elapsed <= 0 {
		return 0, false
	}
	used := current.Value - previous.Value
	return float64(used) / float64(elapsed) * 100, true
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func resolveServerAddress(
	ctx context.Context,
	workDir string,
	program *tea.Program,
	generation uint64,
) {
	result := serveraddr.Resolve(ctx, workDir)
	if ctx.Err() != nil {
		return
	}
	ip := ""
	if result.IP.IsValid() {
		ip = result.IP.String()
	}
	program.Send(ui.ServerAddressMsg{
		Generation: generation,
		IP:         ip,
		Port:       result.Port,
		IPErr:      errorText(result.IPErr),
		PortErr:    errorText(result.PortErr),
	})
}

func streamGC(
	ctx context.Context,
	path string,
	program *tea.Program,
	generation uint64,
) {
	events := make(chan gclog.Event, 4)
	result := make(chan error, 1)
	go func() {
		result <- (gclog.Tailer{Path: path}).Run(ctx, events)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-result:
			if err != nil && !errors.Is(err, context.Canceled) {
				program.Send(ui.LogMsg{
					Generation: generation,
					Entry: serverlog.Entry{
						Kind:    serverlog.KindOther,
						Message: "GC metrics unavailable: " + err.Error(),
					},
				})
			}
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
