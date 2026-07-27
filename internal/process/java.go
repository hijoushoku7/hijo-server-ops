package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	ErrJavaNotFound     = errors.New("javaプロセスがまだ見つかりません")
	ErrDetachedTerminal = errors.New("screen/tmuxを使う起動スクリプトには対応していません")
)

type JavaFinder struct {
	ProcRoot     string
	PollInterval time.Duration
}

func (f JavaFinder) Find(rootPID int) (int, error) {
	procRoot := f.procRoot()
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return 0, fmt.Errorf("%sを読む: %w", procRoot, err)
	}

	processes := make(map[int]procEntry)
	children := make(map[int][]int)
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

		item, ok := processes[pid]
		if ok {
			switch item.comm {
			case "screen", "tmux", "tmux: server":
				return 0, ErrDetachedTerminal
			case "java":
				return pid, nil
			}
		}
		queue = append(queue, children[pid]...)
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

	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-timer.C:
			pid, err := f.Find(rootPID)
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

type procEntry struct {
	comm string
	ppid int
}

func parseProcStat(data []byte) (procEntry, error) {
	line := strings.TrimSpace(string(data))
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close <= open {
		return procEntry{}, errors.New("不正な/proc stat形式")
	}

	fields := strings.Fields(line[close+1:])
	if len(fields) < 2 {
		return procEntry{}, errors.New("不正な/proc statフィールド")
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return procEntry{}, fmt.Errorf("PPIDを読む: %w", err)
	}

	return procEntry{
		comm: line[open+1 : close],
		ppid: ppid,
	}, nil
}
