package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hijoushoku7/hijo-server-ops/internal/procstat"
)

var (
	ErrJavaNotFound     = errors.New("java process not found yet")
	ErrDetachedTerminal = errors.New("start scripts using screen/tmux are not supported")
	ErrRootPIDReused    = errors.New("start script PID was reused")
)

type JavaFinder struct {
	ProcRoot          string
	ProcessGroup      int
	ExpectedStartTime uint64
	PollInterval      time.Duration
}

func (f JavaFinder) Find(rootPID int) (int, error) {
	processes, err := readProcesses(f.procRoot())
	if err != nil {
		return 0, err
	}
	return findJava(processes, rootPID, f.processGroup(rootPID), f.ExpectedStartTime)
}

func readProcesses(procRoot string) (map[int]procEntry, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", procRoot, err)
	}
	processes := make(map[int]procEntry)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(procRoot, entry.Name(), "stat"))
		if err != nil {
			continue
		}
		item, err := parseProcStat(data)
		if err != nil {
			continue
		}
		processes[pid] = item
	}
	return processes, nil
}

func findJava(
	processes map[int]procEntry,
	rootPID int,
	processGroup int,
	expectedStartTime uint64,
) (int, error) {
	if root, ok := processes[rootPID]; ok && expectedStartTime != 0 && root.startTime != expectedStartTime {
		return 0, ErrRootPIDReused
	}

	children := make(map[int][]int)
	for pid, item := range processes {
		children[item.ppid] = append(children[item.ppid], pid)
	}

	queue := []int{rootPID}
	visited := make(map[int]bool)
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if visited[pid] {
			continue
		}
		visited[pid] = true

		queue = append(queue, children[pid]...)
	}

	// バックグラウンド化されて親子関係が切れたプロセスも、hsoが作った
	// プロセスグループ内にいる限り対象に含める。
	for pid, item := range processes {
		if item.pgrp == processGroup {
			visited[pid] = true
		}
	}

	for pid := range visited {
		switch processes[pid].comm {
		case "screen", "tmux", "tmux: server":
			return 0, ErrDetachedTerminal
		}
	}
	for pid := range visited {
		if processes[pid].comm == "java" {
			return pid, nil
		}
	}

	return 0, ErrJavaNotFound
}

func (f JavaFinder) Wait(ctx context.Context, rootPID int) (int, error) {
	interval := f.PollInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}

	timer := time.NewTimer(0)
	defer timer.Stop()

	rootStartTime := f.ExpectedStartTime
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-timer.C:
			processes, err := readProcesses(f.procRoot())
			if err != nil {
				return 0, err
			}
			if root, ok := processes[rootPID]; ok && rootStartTime == 0 {
				rootStartTime = root.startTime
			}

			pid, err := findJava(processes, rootPID, f.processGroup(rootPID), rootStartTime)
			if err == nil {
				return pid, nil
			}
			if !errors.Is(err, ErrJavaNotFound) {
				return 0, err
			}
			timer.Reset(interval)
		}
	}
}

func (f JavaFinder) procRoot() string {
	if f.ProcRoot != "" {
		return f.ProcRoot
	}
	return "/proc"
}

func (f JavaFinder) processGroup(rootPID int) int {
	if f.ProcessGroup != 0 {
		return f.ProcessGroup
	}
	return rootPID
}

type procEntry struct {
	comm      string
	ppid      int
	pgrp      int
	startTime uint64
}

func parseProcStat(data []byte) (procEntry, error) {
	stat, err := procstat.Parse(data)
	if err != nil {
		return procEntry{}, err
	}
	return procEntry{
		comm:      stat.Command,
		ppid:      stat.ParentPID,
		pgrp:      stat.ProcessGroup,
		startTime: stat.StartTime,
	}, nil
}

func processGroupAlive(procRoot string, processGroup int) (bool, error) {
	processes, err := readProcesses(procRoot)
	if err != nil {
		return false, err
	}
	for _, item := range processes {
		if item.pgrp == processGroup {
			return true, nil
		}
	}
	return false, nil
}
