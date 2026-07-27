package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

	writeMu sync.Mutex
}

func Start(options Options) (*Process, error) {
	gcLogDir, err := os.MkdirTemp("", "hso-")
	if err != nil {
		return nil, fmt.Errorf("GCログ用ディレクトリを作る: %w", err)
	}
	gcLogPath := filepath.Join(gcLogDir, "gc.log")

	cmd := exec.Command(options.Command)
	cmd.Dir = options.WorkDir
	cmd.Env = withJavaToolOptions(options.Env, gcLogPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = os.RemoveAll(gcLogDir)
		return nil, fmt.Errorf("サーバーstdinを開く: %w", err)
	}
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = os.RemoveAll(gcLogDir)
		return nil, fmt.Errorf("起動スクリプトを開始する: %w", err)
	}

	process := &Process{
		cmd:       cmd,
		stdin:     stdin,
		done:      make(chan struct{}),
		gcLogDir:  gcLogDir,
		gcLogPath: gcLogPath,
	}
	go process.reap()

	return process, nil
}

func (p *Process) PID() int {
	return p.cmd.Process.Pid
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
	if err := syscall.Kill(-p.PID(), value); err != nil {
		return fmt.Errorf("プロセスグループへ%sを送る: %w", signal, err)
	}
	return nil
}

func (p *Process) reap() {
	p.waitErr = p.cmd.Wait()
	_ = p.stdin.Close()
	_ = os.RemoveAll(p.gcLogDir)
	close(p.done)
}

func withJavaToolOptions(extraEnv []string, gcLogPath string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, extraEnv...)

	const key = "JAVA_TOOL_OPTIONS"
	flag := "-Xlog:gc:file=" + gcLogPath + ":time,uptime,level,tags"

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
