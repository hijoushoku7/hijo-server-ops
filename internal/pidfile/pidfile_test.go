package pidfile

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
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

func TestRunningTreatsIncompletePIDFileAsStoppedWithoutRemovingIt(t *testing.T) {
	for _, test := range []struct {
		name     string
		contents string
	}{
		{name: "空", contents: ""},
		{name: "壊れた内容", contents: "123 書きかけ"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "server.pid")
			if err := os.WriteFile(path, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}

			pid, running, err := (Checker{
				Directory: directory,
				ProcRoot:  t.TempDir(),
			}).Running("server")
			if err != nil || running || pid != 0 {
				t.Fatalf("pid = %d, running = %t, err = %v", pid, running, err)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("書きかけのpidfileが消えた: %v", err)
			}
			if string(contents) != test.contents {
				t.Fatalf("contents = %q, want %q", contents, test.contents)
			}
		})
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
	lock := lockPIDFile(t, path)
	defer lock.Close()
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

func TestCreateRejectsLockedIncompletePIDFile(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	path := filepath.Join(directory, "server.pid")
	lock := lockPIDFile(t, path)
	defer lock.Close()
	writeProcStat(t, procRoot, 123, "new hso", 456)

	file, err := create("server", directory, procRoot, 123, time.Hour)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := AlreadyRunningPID(err); ok {
		t.Fatal("空のpidfileからPIDを取得した")
	}
	if file != nil {
		file.Close()
		t.Fatal("書き込み途中のpidfileがロックされていても追跡を始めた")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 0 {
		t.Fatalf("contents = %q, want empty", contents)
	}
}

func TestConcurrentCreateOverStalePIDFileHasOneWinner(t *testing.T) {
	const iterations = 50
	type result struct {
		file *File
		err  error
	}

	directory := t.TempDir()
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 123, "first hso", 456)
	writeProcStat(t, procRoot, 789, "second hso", 1011)

	for iteration := range iterations {
		writePIDFile(t, directory, "server", 100, 10)
		start := make(chan struct{})
		results := make(chan result, 2)
		for _, pid := range []int{123, 789} {
			go func() {
				<-start
				file, err := create("server", directory, procRoot, pid, time.Hour)
				results <- result{file: file, err: err}
			}()
		}
		close(start)

		got := []result{<-results, <-results}
		var winner *File
		successes := 0
		for _, result := range got {
			if result.err == nil {
				successes++
				winner = result.file
				continue
			}
			if !errors.Is(result.err, ErrAlreadyRunning) {
				t.Fatalf("iteration %d: err = %v", iteration, result.err)
			}
		}
		if successes != 1 {
			for _, result := range got {
				if result.file != nil {
					result.file.Close()
				}
			}
			t.Fatalf("iteration %d: successes = %d, want 1", iteration, successes)
		}
		winner.Close()
	}
}

func TestCreateReopensPIDFileRemovedBeforeLock(t *testing.T) {
	type result struct {
		file *File
		err  error
	}

	directory := t.TempDir()
	procRoot := t.TempDir()
	writeProcStat(t, procRoot, 123, "first hso", 456)
	writeProcStat(t, procRoot, 789, "second hso", 1011)

	first, err := create("server", directory, procRoot, 123, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	path := filepath.Join(directory, "server.pid")

	opened := make(chan struct{})
	releaseOpen := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseOpen)
		}
	}()
	openCalls := 0
	openFile := func(name string, flag int, permission os.FileMode) (*os.File, error) {
		pidFile, err := os.OpenFile(name, flag, permission)
		if err != nil {
			return nil, err
		}
		openCalls++
		if openCalls == 1 {
			close(opened)
			<-releaseOpen
		}
		return pidFile, nil
	}
	results := make(chan result, 1)
	go func() {
		file, err := createWithOpenFile(
			"server", directory, procRoot, 789, time.Hour, openFile,
		)
		results <- result{file: file, err: err}
	}()

	select {
	case <-opened:
		// 2つ目の作成処理は、最初の処理がロックしている inode を開いた状態。
	case result := <-results:
		t.Fatalf("古いinodeを開く前にCreateが終了した: %v", result.err)
	}
	first.Close()
	assertMissing(t, path)
	close(releaseOpen)
	released = true

	second := <-results
	if second.err != nil {
		t.Fatalf("pidfileを開き直したCreate: %v", second.err)
	}
	if second.file == nil {
		t.Fatal("pidfileを開き直した後に追跡を始めなかった")
	}
	defer second.file.Close()
	if openCalls != 2 {
		t.Fatalf("open calls = %d, want 2", openCalls)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("作成後のpidfileがパス上にない: %v", err)
	}
	pid, running, err := (Checker{Directory: directory, ProcRoot: procRoot}).Running("server")
	if err != nil || !running || pid != 789 {
		t.Fatalf("pid = %d, running = %t, err = %v", pid, running, err)
	}
}

func TestCreateSucceedsAfterLockHolderProcessExits(t *testing.T) {
	directory := t.TempDir()
	procRoot := t.TempDir()
	path := writePIDFile(t, directory, "server", 100, 10)
	writeProcStat(t, procRoot, 123, "new hso", 456)

	command := exec.Command(os.Args[0], "-test.run=^TestPIDFileLockHolderHelper$")
	command.Env = append(os.Environ(),
		"HSO_PIDFILE_LOCK_HELPER=1",
		"HSO_PIDFILE_LOCK_PATH="+path,
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("ロック待機プロセスの準備: line = %q, err = %v", line, err)
	}

	file, err := create("server", directory, procRoot, 123, time.Hour)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("ロック中の err = %v", err)
	}
	if file != nil {
		file.Close()
		t.Fatal("別プロセスがロック中でも追跡を始めた")
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("ロック待機プロセスがkill後も正常終了した")
	}

	file, err = create("server", directory, procRoot, 123, time.Hour)
	if err != nil {
		t.Fatalf("ロック解放後のCreate: %v", err)
	}
	file.Close()
}

func TestPIDFileLockHolderHelper(t *testing.T) {
	if os.Getenv("HSO_PIDFILE_LOCK_HELPER") != "1" {
		return
	}
	file, err := os.OpenFile(os.Getenv("HSO_PIDFILE_LOCK_PATH"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	fmt.Println("locked")
	select {}
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

func lockPIDFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	return file
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s が残っている: %v", path, err)
	}
}
