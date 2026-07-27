package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJavaFinderFindsDescendant(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 100, "start script", 1)
	writeProcStat(t, procRoot, 101, "bash", 100)
	writeProcStat(t, procRoot, 102, "java", 101)

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
	writeProcStat(t, procRoot, 100, "java", 1)

	pid, err := (JavaFinder{ProcRoot: procRoot}).Find(100)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 100 {
		t.Fatalf("pid = %d, want 100", pid)
	}
}

func TestJavaFinderRejectsDetachedTerminal(t *testing.T) {
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 100, "bash", 1)
	writeProcStat(t, procRoot, 101, "tmux: server", 100)
	writeProcStat(t, procRoot, 102, "java", 101)

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
	entry, err := parseProcStat([]byte("42 (odd ) name) S 7 0 0 0"))
	if err != nil {
		t.Fatal(err)
	}
	if entry.comm != "odd ) name" || entry.ppid != 7 {
		t.Fatalf("entry = %#v", entry)
	}
}

func writeProcStat(t *testing.T, root string, pid int, comm string, ppid int) {
	t.Helper()
	dir := filepath.Join(root, fmt.Sprint(pid))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("%d (%s) S %d 0 0 0\n", pid, comm, ppid)
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
