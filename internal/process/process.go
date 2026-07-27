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
	"time"
)

const supervisorScript = `
shutdown() {
	trap '' INT TERM HUP
	kill -TERM -$$ 2>/dev/null
	(sleep 1; kill -KILL -$$ 2>/dev/null) &
	killer=$!
	wait "$child" 2>/dev/null
	kill -KILL "$killer" 2>/dev/null
	exit 143
}
trap shutdown INT TERM HUP
exec 3<&0
"$@" <&3 &
child=$!
wait "$child"
status=$?
trap - INT TERM HUP
exit "$status"
`

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
	if err := validateCommand(options.Command, options.WorkDir); err != nil {
		return nil, err
	}

	gcLogDir, err := os.MkdirTemp("", "hso-")
	if err != nil {
		return nil, fmt.Errorf("GCログ用ディレクトリを作る: %w", err)
	}
	gcLogPath := filepath.Join(gcLogDir, "gc.log")

	cmd := exec.Command("/bin/sh", "-c", supervisorScript, "hso-supervisor", options.Command)
	cmd.Dir = options.WorkDir
	cmd.Env = withJavaToolOptions(options.Env, gcLogPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		_ = os.RemoveAll(gcLogDir)
		return nil, fmt.Errorf("サーバーstdinを開く: %w", err)
	}
	cmd.Stdin = stdinReader
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr

	if err := cmd.Start(); err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		_ = os.RemoveAll(gcLogDir)
		return nil, fmt.Errorf("起動スクリプトを開始する: %w", err)
	}
	_ = stdinReader.Close()

	process := &Process{
		cmd:       cmd,
		stdin:     stdinWriter,
		done:      make(chan struct{}),
		gcLogDir:  gcLogDir,
		gcLogPath: gcLogPath,
	}
	go process.reap()

	return process, nil
}

func validateCommand(command, workDir string) error {
	path := command
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("起動スクリプトを確認する: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("起動スクリプトはディレクトリです: %s", path)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("起動スクリプトに実行権限がありません: %s", path)
	}
	return nil
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

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		alive, err := processGroupAlive("/proc", p.PID())
		if err != nil {
			if p.waitErr == nil {
				p.waitErr = fmt.Errorf("プロセスグループを確認する: %w", err)
			}
			break
		}
		if !alive {
			break
		}
		<-ticker.C
	}

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
