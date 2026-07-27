package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestJavaFinderFindsDescendant(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 100, "start script", 1, 100, 10)
	writeProcStat(t, procRoot, 101, "bash", 100, 100, 11)
	writeProcStat(t, procRoot, 102, "java", 101, 100, 12)

	pid, err := (JavaFinder{ProcRoot: procRoot}).Find(100)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 102 {
		t.Fatalf("pid = %d, want 102", pid)
	}
}

func TestJavaFinderFindsExecedRoot(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 100, "java", 1, 100, 10)

	pid, err := (JavaFinder{ProcRoot: procRoot}).Find(100)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 100 {
		t.Fatalf("pid = %d, want 100", pid)
	}
}

func TestJavaFinderFindsReparentedProcessGroupMember(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 102, "java", 1, 100, 12)

	pid, err := (JavaFinder{ProcRoot: procRoot}).Find(100)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 102 {
		t.Fatalf("pid = %d, want 102", pid)
	}
}

func TestJavaFinderRejectsDetachedTerminal(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 100, "bash", 1, 100, 10)
	writeProcStat(t, procRoot, 101, "tmux: server", 100, 100, 11)
	writeProcStat(t, procRoot, 102, "java", 101, 100, 12)

	_, err := (JavaFinder{ProcRoot: procRoot}).Find(100)
	if !errors.Is(err, ErrDetachedTerminal) {
		t.Fatalf("err = %v", err)
	}
}

func TestJavaFinderWaitCanBeCanceled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := (JavaFinder{
		ProcRoot:     t.TempDir(),
		PollInterval: time.Millisecond,
	}).Wait(ctx, 100)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseProcStatAllowsSpacesAndParenthesesInComm(t *testing.T) {
	entry, err := parseProcStat([]byte(procStatLine(42, "odd ) name", 7, 42, 99)))
	if err != nil {
		t.Fatal(err)
	}
	if entry.comm != "odd ) name" || entry.ppid != 7 || entry.pgrp != 42 || entry.startTime != 99 {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestFindJavaRejectsReusedRootPID(t *testing.T) {
	processes := map[int]procEntry{
		100: {comm: "bash", ppid: 1, pgrp: 100, startTime: 20},
		101: {comm: "java", ppid: 100, pgrp: 100, startTime: 21},
	}

	_, err := findJava(processes, 100, 10)
	if !errors.Is(err, ErrRootPIDReused) {
		t.Fatalf("err = %v", err)
	}
}

func writeProcStat(t *testing.T, root string, pid int, comm string, ppid, pgrp int, startTime uint64) {
	t.Helper()
	dir := filepath.Join(root, fmt.Sprint(pid))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := procStatLine(pid, comm, ppid, pgrp, startTime)
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func procStatLine(pid int, comm string, ppid, pgrp int, startTime uint64) string {
	fields := make([]string, 20)
	for index := range fields {
		fields[index] = "0"
	}
	fields[0] = "S"
	fields[1] = strconv.Itoa(ppid)
	fields[2] = strconv.Itoa(pgrp)
	fields[19] = strconv.FormatUint(startTime, 10)
	return fmt.Sprintf("%d (%s) %s\n", pid, comm, strings.Join(fields, " "))
}
