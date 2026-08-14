// Package pidfile は登録済みサーバーを動かす hso プロセスの生存情報を管理する。
package pidfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstat"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

const refreshInterval = time.Hour

var (
	// ErrAlreadyRunning は同じサーバーの pidfile がすでに使われていることを表す。
	ErrAlreadyRunning = msg.AlreadyRunning()
	// ErrUnsafeDirectory は /tmp 側のディレクトリを安全と確認できなかったことを表す。
	ErrUnsafeDirectory = msg.UnsafePIDDirectory()
)

const (
	accessExecute = 1
	accessWrite   = 2
)

// File は実行中の hso が作った pidfile と mtime 更新処理を持つ。
type File struct {
	path      string
	pid       int
	startTime uint64
	done      chan struct{}
	once      sync.Once
	wait      sync.WaitGroup
}

// Checker は pidfile と /proc を照合する。空の項目には通常の場所を使う。
type Checker struct {
	Directory string
	ProcRoot  string
}

// Directory は pidfile を置くディレクトリを作って返す。
func Directory() (string, error) {
	return runtimeDirectory(os.Getenv("XDG_RUNTIME_DIR"), "/tmp", os.Getuid(), syscall.Access)
}

// Create は現在の hso の PID と起動時刻を書き、mtime の更新を始める。
func Create(name string) (*File, error) {
	directory, err := Directory()
	if err != nil {
		return nil, err
	}
	file, err := create(name, directory, "/proc", os.Getpid(), refreshInterval)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func create(name, directory, procRoot string, pid int, interval time.Duration) (*File, error) {
	return createWithOpenFile(name, directory, procRoot, pid, interval, os.OpenFile)
}

func createWithOpenFile(
	name, directory, procRoot string,
	pid int,
	interval time.Duration,
	openFile func(string, int, os.FileMode) (*os.File, error),
) (*File, error) {
	if err := registry.ValidateName(name); err != nil {
		return nil, err
	}
	stat, err := readProcStat(procRoot, pid)
	if err != nil {
		return nil, msg.ReadPIDStartTimeFailed(err)
	}
	path := filepath.Join(directory, name+".pid")
	contents := fmt.Sprintf("%d %d\n", pid, stat.StartTime)
	pidFile, err := openFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		runningPID, running, checkErr := (Checker{
			Directory: directory,
			ProcRoot:  procRoot,
		}).Running(name)
		if checkErr != nil {
			return nil, checkErr
		}
		if running {
			return nil, &alreadyRunningError{name: name, pid: runningPID}
		}
		pidFile, err = openFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			return nil, ErrAlreadyRunning
		}
	}
	if err != nil {
		return nil, msg.WritePIDFileFailed(err, path)
	}
	if _, err := pidFile.WriteString(contents); err != nil {
		_ = pidFile.Close()
		return nil, msg.WritePIDFileFailed(err, path)
	}
	if err := pidFile.Close(); err != nil {
		return nil, msg.WritePIDFileFailed(err, path)
	}

	file := &File{path: path, pid: pid, startTime: stat.StartTime, done: make(chan struct{})}
	file.wait.Add(1)
	go file.refresh(interval)
	return file, nil
}

type alreadyRunningError struct {
	name string
	pid  int
}

func (e *alreadyRunningError) Error() string {
	return msg.ServerAlreadyRunning(e.name, e.pid).Error()
}

func (e *alreadyRunningError) Unwrap() error {
	return ErrAlreadyRunning
}

// AlreadyRunningPID は競合した pidfile から PID を確認できたときだけ返す。
func AlreadyRunningPID(err error) (int, bool) {
	var runningErr *alreadyRunningError
	if !errors.As(err, &runningErr) {
		return 0, false
	}
	return runningErr.pid, true
}

// Close は mtime の更新を止めて pidfile を消す。実行中にファイルが消されて
// いても hso の終了処理には影響させない。
func (f *File) Close() {
	if f == nil {
		return
	}
	f.once.Do(func() {
		close(f.done)
		f.wait.Wait()
		_ = removeIfMatches(f.path, f.pid, f.startTime)
	})
}

func (f *File) refresh(interval time.Duration) {
	defer f.wait.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			// 起動後は runtime directory ごと pidfile が消えていても、
			// 動作中の hso は止めない。
			_ = os.Chtimes(f.path, now, now)
		case <-f.done:
			return
		}
	}
}

// Running は既定の場所にある pidfile と /proc を照合する。
func Running(name string) (int, bool, error) {
	return (Checker{}).Running(name)
}

// Running は PID と起動時刻が現在の /proc と一致するときだけ true を返す。
func (c Checker) Running(name string) (int, bool, error) {
	if err := registry.ValidateName(name); err != nil {
		return 0, false, err
	}
	directory := c.Directory
	if directory == "" {
		var err error
		directory, err = Directory()
		if err != nil {
			return 0, false, err
		}
	}
	procRoot := c.ProcRoot
	if procRoot == "" {
		procRoot = "/proc"
	}
	path := filepath.Join(directory, name+".pid")
	pid, expectedStartTime, err := read(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, msg.ReadPIDFileFailed(err, path)
	}

	procDirectory := filepath.Join(procRoot, strconv.Itoa(pid))
	if _, err := os.Stat(procDirectory); errors.Is(err, os.ErrNotExist) {
		return stale(path, pid, expectedStartTime)
	} else if err != nil {
		return 0, false, msg.CheckProcessFailed(err, pid)
	}
	stat, err := readProcStat(procRoot, pid)
	if errors.Is(err, os.ErrNotExist) {
		return stale(path, pid, expectedStartTime)
	}
	if err != nil {
		return 0, false, msg.CheckProcessFailed(err, pid)
	}
	if stat.StartTime != expectedStartTime {
		return stale(path, pid, expectedStartTime)
	}
	return pid, true, nil
}

func runtimeDirectory(
	xdgRuntimeDir string,
	temporaryRoot string,
	uid int,
	access func(string, uint32) error,
) (string, error) {
	if xdgRuntimeDir != "" {
		info, err := os.Stat(xdgRuntimeDir)
		if err == nil && info.IsDir() && access(xdgRuntimeDir, accessWrite|accessExecute) == nil {
			directory := filepath.Join(xdgRuntimeDir, "hso")
			if err := os.MkdirAll(directory, 0o700); err != nil {
				return "", msg.CreatePIDDirectoryFailed(err)
			}
			return directory, nil
		}
	}

	directory := filepath.Join(temporaryRoot, "hso-"+strconv.Itoa(uid))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", msg.CreatePIDDirectoryFailed(err)
	}
	if err := validateTemporaryDirectory(directory, uid); err != nil {
		return "", err
	}
	return directory, nil
}

func validateTemporaryDirectory(path string, uid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnsafeDirectory, msg.CheckPIDDirectoryFailed(err))
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %w", ErrUnsafeDirectory, msg.PIDDirectoryIsSymlink(path))
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: %w", ErrUnsafeDirectory,
			msg.CheckPIDDirectoryFailed(errors.New("file stat has no syscall.Stat_t")))
	}
	if stat.Uid != uint32(uid) {
		return fmt.Errorf("%w: %w", ErrUnsafeDirectory, msg.PIDDirectoryWrongOwner(path))
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("%w: %w", ErrUnsafeDirectory,
			msg.PIDDirectoryWrongMode(path, uint32(info.Mode().Perm())))
	}
	return nil
}

func read(path string) (int, uint64, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(contents))
	if len(fields) != 2 {
		return 0, 0, errors.New("malformed pidfile")
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return 0, 0, errors.New("malformed pidfile PID")
	}
	startTime, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, 0, errors.New("malformed pidfile start time")
	}
	return pid, startTime, nil
}

func readProcStat(procRoot string, pid int) (procstat.Stat, error) {
	contents, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return procstat.Stat{}, err
	}
	return procstat.Parse(contents)
}

func stale(path string, pid int, startTime uint64) (int, bool, error) {
	if err := removeIfMatches(path, pid, startTime); err != nil {
		return 0, false, msg.RemoveStalePIDFileFailed(err, path)
	}
	return 0, false, nil
}

func removeIfMatches(path string, pid int, startTime uint64) error {
	currentPID, currentStartTime, err := read(path)
	if err != nil || currentPID != pid || currentStartTime != startTime {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
