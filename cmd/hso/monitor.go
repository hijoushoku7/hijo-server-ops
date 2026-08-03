package main

import (
	"context"
	"errors"
	"sync/atomic"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/gclog"
	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serveraddr"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
	"github.com/hijoushoku7/hijo-server-ops/internal/ui"
)

const metricsInterval = time.Second

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
		program.Send(ui.FatalMsg{Generation: generation, Err: msg.FindJavaFailed(err)})
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
		metrics, jvmErr := sampleJVM(pid, &reader)
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

func sampleJVM(pid int, reader **hsperfdata.Reader) (hsperfdata.Metrics, error) {
	if *reader == nil {
		opened, err := hsperfdata.Open(pid)
		if err != nil {
			return hsperfdata.Metrics{}, err
		}
		*reader = opened
	}

	snapshot, err := (*reader).Sample()
	if err != nil {
		_ = (*reader).Close()
		*reader = nil
		return hsperfdata.Metrics{}, err
	}
	metrics := snapshot.Metrics()
	if !metrics.Heap.Used.Available || !metrics.Heap.Committed.Available {
		return metrics, msg.ErrHeapCountersUnavailable
	}
	return metrics, nil
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
	return float64(current.Value-previous.Value) / float64(elapsed) * 100, true
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
			program.Send(ui.LogMsg{Generation: generation, Entry: entry})
		}
	}
}
