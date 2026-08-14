package pidfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunningWithoutPIDFileIsStopped(t *testing.T) {
	pid, running, err := (Checker{Directory: t.TempDir(), ProcRoot: t.TempDir()}).Running("server")
	if err != nil || running || pid != 0 {
		t.Fatalf("pid = %d, running = %t, err = %v", pid, running, err)
	}
}

func TestRunningRemovesPIDFileWhenProcessIsGone(t *testing.T) {
	directory := t.TempDir()
	path := writePIDFile(t, directory, "server", 100, 10)

	pid, running, err := (Checker{Directory: directory, ProcRoot: t.TempDir()}).Running("server")
	if err != nil || running || pid != 0 {
		t.Fatalf("pid = %d, running = %t, err = %v", pid, running, err)
	}
	assertMissing(t, path)
}

func TestRunningRemovesPIDFileWhenPIDWasReused(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	path := writePIDFile(t, directory, "server", 100, 10)
	writeProcStat(t, procRoot, 100, "unrelated process", 20)

	pid, running, err := (Checker{Directory: directory, ProcRoot: procRoot}).Running("server")
	if err != nil || running || pid != 0 {
		t.Fatalf("pid = %d, running = %t, err = %v", pid, running, err)
	}
	assertMissing(t, path)
}

func TestRunningMatchesPIDAndStartTime(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	writePIDFile(t, directory, "server", 100, 10)
	writeProcStat(t, procRoot, 100, "hso (server)", 10)

	pid, running, err := (Checker{Directory: directory, ProcRoot: procRoot}).Running("server")
	if err != nil || !running || pid != 100 {
		t.Fatalf("pid = %d, running = %t, err = %v", pid, running, err)
	}
}

func TestTemporaryDirectoryRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "link")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := validateTemporaryDirectory(path, os.Getuid()); !errors.Is(err, ErrUnsafeDirectory) {
		t.Fatalf("err = %v", err)
	}
}

func TestTemporaryDirectoryRejectsWrongOwner(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateTemporaryDirectory(path, os.Getuid()+1); !errors.Is(err, ErrUnsafeDirectory) {
		t.Fatalf("err = %v", err)
	}
}

func TestTemporaryDirectoryRejectsWrongMode(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateTemporaryDirectory(path, os.Getuid()); !errors.Is(err, ErrUnsafeDirectory) {
		t.Fatalf("err = %v", err)
	}
}

func TestRuntimeDirectoryRejectsUnsafeTemporaryDirectory(t *testing.T) {
	temporaryRoot := t.TempDir()
	directory := filepath.Join(temporaryRoot, "hso-"+strconv.Itoa(os.Getuid()))
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := runtimeDirectory("", temporaryRoot, os.Getuid(),
		func(string, uint32) error { return nil })
	if !errors.Is(err, ErrUnsafeDirectory) {
		t.Fatalf("err = %v", err)
	}
}

func TestRuntimeDirectoryFallsBackWhenXDGIsNotWritable(t *testing.T) {
	temporaryRoot := t.TempDir()
	directory, err := runtimeDirectory(t.TempDir(), temporaryRoot, os.Getuid(),
		func(string, uint32) error { return syscall.EACCES })
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(temporaryRoot, "hso-"+strconv.Itoa(os.Getuid()))
	if directory != want {
		t.Fatalf("directory = %q, want %q", directory, want)
	}
}

func TestRuntimeDirectoryUsesWritableXDG(t *testing.T) {
	xdgRuntimeDir := t.TempDir()
	directory, err := runtimeDirectory(xdgRuntimeDir, t.TempDir(), os.Getuid(),
		func(string, uint32) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdgRuntimeDir, "hso")
	if directory != want {
		t.Fatalf("directory = %q, want %q", directory, want)
	}
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		t.Fatalf("pidfile directory: info=%v, err=%v", info, err)
	}
}

func TestCreateWritesAndCloseRemovesPIDFile(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 123, "hso", 456)
	file, err := create("server", directory, procRoot, 123, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "server.pid")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "123 456\n" {
		t.Fatalf("contents = %q", contents)
	}
	file.Close()
	assertMissing(t, path)
	file.Close()
}

func TestCreateRejectsRunningPIDFile(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	path := writePIDFile(t, directory, "server", 100, 10)
	writeProcStat(t, procRoot, 100, "running hso", 10)
	writeProcStat(t, procRoot, 123, "new hso", 456)

	file, err := create("server", directory, procRoot, 123, time.Hour)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("err = %v", err)
	}
	if runningPID, ok := AlreadyRunningPID(err); !ok || runningPID != 100 {
		t.Fatalf("running PID = %d, ok = %t", runningPID, ok)
	}
	if file != nil {
		file.Close()
		t.Fatal("起動中のpidfileを上書きして追跡を始めた")
	}
	pid, startTime, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 100 || startTime != 10 {
		t.Fatalf("pid = %d, startTime = %d", pid, startTime)
	}
}

func TestCreateReplacesStalePIDFile(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	writePIDFile(t, directory, "server", 100, 10)
	writeProcStat(t, procRoot, 100, "reused process", 20)
	writeProcStat(t, procRoot, 123, "new hso", 456)

	file, err := create("server", directory, procRoot, 123, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	pid, startTime, err := read(filepath.Join(directory, "server.pid"))
	if err != nil {
		t.Fatal(err)
	}
	if pid != 123 || startTime != 456 {
		t.Fatalf("pid = %d, startTime = %d", pid, startTime)
	}
}

func TestCreateRejectsSecondExclusiveCreateConflict(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	path := writePIDFile(t, directory, "server", 100, 10)
	writeProcStat(t, procRoot, 100, "reused process", 20)
	writeProcStat(t, procRoot, 123, "new hso", 456)

	openCalls := 0
	openFile := func(name string, flag int, permission os.FileMode) (*os.File, error) {
		openCalls++
		if openCalls == 2 {
			competitor, err := os.OpenFile(name, flag, permission)
			if err != nil {
				return nil, err
			}
			if _, err := competitor.WriteString("789 1011\n"); err != nil {
				_ = competitor.Close()
				return nil, err
			}
			if err := competitor.Close(); err != nil {
				return nil, err
			}
		}
		return os.OpenFile(name, flag, permission)
	}
	file, err := createWithOpenFile(
		"server", directory, procRoot, 123, time.Hour, openFile,
	)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("err = %v", err)
	}
	if file != nil {
		file.Close()
		t.Fatal("2度目の排他的作成が競合しても追跡を始めた")
	}
	if openCalls != 2 {
		t.Fatalf("open calls = %d, want 2", openCalls)
	}
	pid, startTime, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 789 || startTime != 1011 {
		t.Fatalf("pid = %d, startTime = %d", pid, startTime)
	}
}

func TestCloseDoesNotFailAfterPIDFileWasRemoved(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 123, "hso", 456)
	file, err := create("server", directory, procRoot, 123, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(directory, "server.pid")); err != nil {
		t.Fatal(err)
	}
	// mtime 更新が消えたファイルへ少なくとも一度失敗しても、更新処理と
	// 終了処理がそのエラーを hso の動作へ返さないことを確かめる。
	time.Sleep(5 * time.Millisecond)
	file.Close()
}

func TestCloseDoesNotRemoveReplacedPIDFile(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 123, "hso", 456)
	file, err := create("server", directory, procRoot, 123, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	path := writePIDFile(t, directory, "server", 789, 1011)
	file.Close()
	pid, startTime, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 789 || startTime != 1011 {
		t.Fatalf("pid = %d, startTime = %d", pid, startTime)
	}
}

func TestStaleRemovesOnlyMatchingPIDFile(t *testing.T) {
	for _, test := range []struct {
		name             string
		currentPID       int
		currentStartTime uint64
		removed          bool
	}{
		{name: "自分の内容", currentPID: 123, currentStartTime: 456, removed: true},
		{name: "別プロセスの内容", currentPID: 789, currentStartTime: 1011, removed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := writePIDFile(t, directory, "server", test.currentPID, test.currentStartTime)
			if _, _, err := stale(path, 123, 456); err != nil {
				t.Fatal(err)
			}
			_, err := os.Stat(path)
			if test.removed && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("一致するpidfileが残った: %v", err)
			}
			if !test.removed && err != nil {
				t.Fatalf("別プロセスのpidfileが消えた: %v", err)
			}
		})
	}
}

func TestCreateRefreshesModificationTime(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 123, "hso", 456)
	file, err := create("server", directory, procRoot, 123, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	path := filepath.Join(directory, "server.pid")
	old := time.Unix(1, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.ModTime().After(old) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pidfile の mtime が更新されなかった")
		}
		time.Sleep(time.Millisecond)
	}
}

func writePIDFile(t *testing.T, directory, name string, pid int, startTime uint64) string {
	t.Helper()
	path := filepath.Join(directory, name+".pid")
	contents := fmt.Sprintf("%d %d\n", pid, startTime)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeProcStat(t *testing.T, procRoot string, pid int, command string, startTime uint64) {
	t.Helper()
	directory := filepath.Join(procRoot, strconv.Itoa(pid))
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	fields := make([]string, 20)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "S"
	fields[1] = "1"
	fields[2] = strconv.Itoa(pid)
	fields[19] = strconv.FormatUint(startTime, 10)
	contents := fmt.Sprintf("%d (%s) %s\n", pid, command, strings.Join(fields, " "))
	if err := os.WriteFile(filepath.Join(directory, "stat"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s が残っている: %v", path, err)
	}
}
