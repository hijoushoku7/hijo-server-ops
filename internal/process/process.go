package process

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type Options struct {
	Command string
	WorkDir string
	Stdout  io.Writer
	Stderr  io.Writer
	Env     []string
}

type Process struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	done      chan struct{}
	waitErr   error
	gcLogDir  string
	gcLogPath string
	serverPID int
	rootStart uint64

	writeMu sync.Mutex
}

func Start(options Options) (*Process, error) {
	command, err := resolveCommand(options.Command, options.WorkDir)
	if err != nil {
		return nil, err
	}

	gcLogDir, err := os.MkdirTemp("", "hso-")
	if err != nil {
		return nil, fmt.Errorf("GCログ用ディレクトリを作る: %w", err)
	}
	gcLogPath := filepath.Join(gcLogDir, "gc.log")

	executable, err := os.Executable()
	if err != nil {
		_ = os.RemoveAll(gcLogDir)
		return nil, fmt.Errorf("hsoの実行ファイルを確認する: %w", err)
	}
	cmd := exec.Command(executable, supervisorArgument, command)
	cmd.Dir = options.WorkDir
	cmd.Env = withJavaToolOptions(options.Env, gcLogPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		_ = os.RemoveAll(gcLogDir)
		return nil, fmt.Errorf("サーバーstdinを開く: %w", err)
	}
	controlReader, controlWriter, err := os.Pipe()
	if err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		_ = os.RemoveAll(gcLogDir)
		return nil, fmt.Errorf("supervisor制御パイプを開く: %w", err)
	}
	cmd.Stdin = stdinReader
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr
	cmd.ExtraFiles = []*os.File{controlWriter}

	if err := cmd.Start(); err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		_ = controlReader.Close()
		_ = controlWriter.Close()
		_ = os.RemoveAll(gcLogDir)
		return nil, fmt.Errorf("supervisorを開始する: %w", err)
	}
	_ = stdinReader.Close()
	_ = controlWriter.Close()

	rootStart, err := processStartTime("/proc", cmd.Process.Pid)
	if err != nil {
		_ = stdinWriter.Close()
		_ = controlReader.Close()
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
		_ = os.RemoveAll(gcLogDir)
		return nil, fmt.Errorf("supervisorの開始時刻を読む: %w", err)
	}

	serverPID, err := readSupervisorPID(controlReader)
	_ = controlReader.Close()
	if err != nil {
		_ = stdinWriter.Close()
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
		_ = os.RemoveAll(gcLogDir)
		return nil, err
	}

	process := &Process{
		cmd:       cmd,
		stdin:     stdinWriter,
		done:      make(chan struct{}),
		gcLogDir:  gcLogDir,
		gcLogPath: gcLogPath,
		serverPID: serverPID,
		rootStart: rootStart,
	}
	go process.reap()

	return process, nil
}

func resolveCommand(command, workDir string) (string, error) {
	path := command
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("起動スクリプトの絶対パスを求める: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("起動スクリプトを確認する: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("起動スクリプトはディレクトリです: %s", path)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("起動スクリプトに実行権限がありません: %s", path)
	}
	return path, nil
}

func (p *Process) PID() int {
	return p.cmd.Process.Pid
}

func (p *Process) ServerPID() int {
	return p.serverPID
}

func (p *Process) RootStartTime() uint64 {
	return p.rootStart
}

func (p *Process) GCLogPath() string {
	return p.gcLogPath
}

func (p *Process) Done() <-chan struct{} {
	return p.done
}

func (p *Process) Wait() error {
	<-p.done
	return p.waitErr
}

func (p *Process) Write(data []byte) (int, error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	select {
	case <-p.done:
		return 0, errors.New("サーバープロセスは終了しています")
	default:
		return p.stdin.Write(data)
	}
}

func (p *Process) Send(command string) error {
	command = strings.TrimRight(command, "\r\n") + "\n"
	_, err := io.WriteString(p, command)
	return err
}

func (p *Process) Signal(signal os.Signal) error {
	value, ok := signal.(syscall.Signal)
	if !ok {
		return fmt.Errorf("未対応のシグナル: %v", signal)
	}
	if err := p.cmd.Process.Signal(value); err != nil {
		return fmt.Errorf("supervisorへ%sを送る: %w", signal, err)
	}
	return nil
}

func (p *Process) reap() {
	p.waitErr = p.cmd.Wait()
	_ = p.stdin.Close()
	_ = os.RemoveAll(p.gcLogDir)
	close(p.done)
}

func readSupervisorPID(control *os.File) (int, error) {
	line, err := bufio.NewReader(control).ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("supervisorの応答を読む: %w", err)
	}
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "ERR ") {
		return 0, errors.New(strings.TrimPrefix(line, "ERR "))
	}
	if !strings.HasPrefix(line, "PID ") {
		return 0, fmt.Errorf("不正なsupervisor応答: %q", line)
	}
	pid, err := strconv.Atoi(strings.TrimPrefix(line, "PID "))
	if err != nil {
		return 0, fmt.Errorf("supervisor応答のPIDを読む: %w", err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("supervisor応答のPIDが不正です: %d", pid)
	}
	return pid, nil
}

func processStartTime(procRoot string, pid int) (uint64, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	entry, err := parseProcStat(data)
	if err != nil {
		return 0, err
	}
	return entry.startTime, nil
}

func withJavaToolOptions(extraEnv []string, gcLogPath string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, extraEnv...)

	const key = "JAVA_TOOL_OPTIONS"
	flag := "-Xlog:gc:file=" + gcLogPath +
		":time,uptime,level,tags:filesize=8M,filecount=2"

	value := ""
	filtered := env[:0]
	for _, item := range env {
		if strings.HasPrefix(item, key+"=") {
			value = strings.TrimPrefix(item, key+"=")
			continue
		}
		filtered = append(filtered, item)
	}
	if value != "" {
		value += " "
	}
	value += flag

	return append(filtered, key+"="+value)
}
