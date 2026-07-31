package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if command, ok := SupervisorCommand(os.Args); ok {
		os.Exit(RunSupervisor(command))
	}
	os.Exit(m.Run())
}

func TestProcessConnectsStreamsAndInjectsGCLog(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	content := `#!/bin/sh
printf '%s\n' "$JAVA_TOOL_OPTIONS"
IFS= read -r line
printf 'got:%s\n' "$line"
`
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	server, err := Start(Options{
		Command: script,
		WorkDir: dir,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env:     []string{"JAVA_TOOL_OPTIONS=-Dexisting=true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	gcLogPath := server.GCLogPath()

	if err := server.Send("say hello"); err != nil {
		t.Fatal(err)
	}
	if err := server.Wait(); err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "-Dexisting=true -Xlog:gc:file="+gcLogPath) {
		t.Fatalf("JAVA_TOOL_OPTIONS was %q", output)
	}
	if !strings.Contains(output, ":time,uptime,level,tags:filesize=8M,filecount=2") {
		t.Fatalf("JAVA_TOOL_OPTIONS was %q", output)
	}
	if !strings.Contains(output, "got:say hello") {
		t.Fatalf("stdout = %q", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Dir(gcLogPath)); !os.IsNotExist(err) {
		t.Fatalf("GC log directory was not removed: %v", err)
	}
}

func TestProcessReportsStartError(t *testing.T) {
	_, err := Start(Options{
		Command: filepath.Join(t.TempDir(), "missing"),
		WorkDir: t.TempDir(),
	})
	// 起動スクリプトが見つからないことが呼び出し側まで伝わるかどうか。
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}

func TestProcessRunsRelativeCommandWithoutSlash(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	server, err := Start(Options{Command: "run.sh", WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteAfterExitFails(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	server, err := Start(Options{Command: script, WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Write([]byte("list\n")); err == nil {
		t.Fatal("Write succeeded after process exit")
	}
}

func TestProcessWaitsForBackgroundProcessInGroup(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 0.15 &\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	server, err := Start(Options{Command: script, WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Wait(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
		t.Fatalf("Wait returned before background process exited: %s", elapsed)
	}
}

func TestSupervisorKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	content := "#!/bin/sh\ntrap '' INT TERM HUP\n: > ready\nwhile :; do sleep 10; done\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}

	server, err := Start(Options{Command: script, WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitForFile(t, filepath.Join(dir, "ready"))
	if err := server.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	waited := make(chan struct{})
	go func() {
		_ = server.Wait()
		close(waited)
	}()

	select {
	case <-waited:
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not terminate the process group")
	}

	alive, err := processGroupAlive("/proc", server.ServerPID())
	if err != nil {
		t.Fatal(err)
	}
	if alive {
		t.Fatal("process group is still alive")
	}
}

func TestSupervisorKillsProcessGroupWhenParentExits(t *testing.T) {
	dir := t.TempDir()
	worker := filepath.Join(dir, "worker.sh")
	workerContent := "#!/bin/sh\ntrap '' INT TERM HUP\nwhile :; do sleep 10; done\n"
	if err := os.WriteFile(worker, []byte(workerContent), 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "run.sh")
	content := "#!/bin/sh\n./worker.sh &\necho $! > child.pid\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}

	helper := exec.Command(os.Args[0], "-test.run=^TestPdeathHelper$")
	helper.Env = append(os.Environ(), "HSO_PDEATH_HELPER="+dir)
	if output, err := helper.CombinedOutput(); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, output)
	}

	data, err := os.ReadFile(filepath.Join(dir, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child process %d survived its hso parent", pid)
}

func TestSupervisorDetectsAndStopsDetachedTmux(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	dir := t.TempDir()
	socket := filepath.Join(dir, fmt.Sprintf("tmux-%d.sock", os.Getpid()))
	defer exec.Command(tmux, "-S", socket, "kill-server").Run()

	script := filepath.Join(dir, "run.sh")
	content := fmt.Sprintf(
		"#!/bin/sh\nunset TMUX TMUX_PANE\nexec %s -S %s new-session -d /bin/sleep 30\n",
		tmux,
		socket,
	)
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	server, err := Start(Options{
		Command: script,
		WorkDir: dir,
		Stderr:  &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	finder := JavaFinder{
		ProcessGroup:      server.ServerPID(),
		ExpectedStartTime: server.RootStartTime(),
		PollInterval:      10 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = finder.Wait(ctx, server.PID())
	if !errors.Is(err, ErrDetachedTerminal) {
		_ = server.Signal(syscall.SIGTERM)
		if strings.Contains(stderr.String(), "Operation not permitted") {
			t.Skip("sandbox does not permit starting a tmux server")
		}
		t.Fatalf("finder error = %v; stderr = %q", err, stderr.String())
	}
	if err := server.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := server.Wait(); err == nil {
		t.Fatal("supervisor unexpectedly reported a clean exit")
	}

	if err := exec.Command(tmux, "-S", socket, "has-session").Run(); err == nil {
		t.Fatal("detached tmux server survived supervisor shutdown")
	}
}

func TestPdeathHelper(t *testing.T) {
	dir := os.Getenv("HSO_PDEATH_HELPER")
	if dir == "" {
		t.Skip("helper process only")
	}

	server, err := Start(Options{
		Command: filepath.Join(dir, "run.sh"),
		WorkDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForFile(t, filepath.Join(dir, "child.pid"))
	_ = server
	os.Exit(0)
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
