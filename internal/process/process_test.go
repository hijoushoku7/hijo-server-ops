package process

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	if err == nil || !strings.Contains(err.Error(), "起動スクリプトを開始する") {
		t.Fatalf("err = %v", err)
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
