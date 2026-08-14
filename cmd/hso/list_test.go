package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

func TestPrintServerListShowsEmptyHint(t *testing.T) {
	var output bytes.Buffer
	if err := printServerList(&output, registry.Registry{}, pidfile.Running); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "hso setup") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestPrintServerListShowsAllStatuses(t *testing.T) {
	directory := t.TempDir()
	stoppedConfig := filepath.Join(directory, "stopped.toml")
	runningConfig := filepath.Join(directory, "running.toml")
	for _, path := range []string{stoppedConfig, runningConfig} {
		if err := os.WriteFile(path, []byte("[server]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	servers := registry.Registry{Servers: []registry.Server{
		{Name: "missing", Config: filepath.Join(directory, "missing.toml")},
		{Name: "stopped", Config: stoppedConfig},
		{Name: "running", Config: runningConfig},
	}}
	check := func(name string) (int, bool, error) {
		return 4321, name == "running", nil
	}

	var output bytes.Buffer
	if err := printServerList(&output, servers, check); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		msg.ListNameHeader,
		msg.ListStatusHeader,
		msg.ListPathHeader,
		msg.ConfigNotFound,
		msg.ServerStopped,
		msg.ServerRunning(4321),
		stoppedConfig,
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output = %q に %q がない", output.String(), want)
		}
	}
}

func TestPrintServerListAlignsColumnsWithFullWidthValues(t *testing.T) {
	directory := t.TempDir()
	stoppedConfig := filepath.Join(directory, "stopped.toml")
	if err := os.WriteFile(stoppedConfig, []byte("[server]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	servers := registry.Registry{Servers: []registry.Server{
		{Name: "survival", Config: stoppedConfig},
		{Name: "creative", Config: filepath.Join(directory, "missing.toml")},
	}}

	var output bytes.Buffer
	if err := printServerList(&output, servers, func(string) (int, bool, error) {
		return 0, false, nil
	}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("出力行数 = %d, want 3: %q", len(lines), output.String())
	}
	statusColumn := -1
	pathColumn := -1
	for index, line := range lines {
		status := []string{msg.ListStatusHeader, msg.ServerStopped, msg.ConfigNotFound}[index]
		path := []string{msg.ListPathHeader, stoppedConfig, filepath.Join(directory, "missing.toml")}[index]
		statusIndex := strings.Index(line, status)
		pathIndex := strings.Index(line, path)
		if statusIndex < 0 || pathIndex < 0 {
			t.Fatalf("必要な値が出力されていない: %q", line)
		}
		if index == 0 {
			statusColumn = ansi.StringWidth(line[:statusIndex])
			pathColumn = ansi.StringWidth(line[:pathIndex])
			continue
		}
		if got := ansi.StringWidth(line[:statusIndex]); got != statusColumn {
			t.Errorf("状態列の開始位置 = %d, want %d: %q", got, statusColumn, line)
		}
		if got := ansi.StringWidth(line[:pathIndex]); got != pathColumn {
			t.Errorf("パス列の開始位置 = %d, want %d: %q", got, pathColumn, line)
		}
	}
}

func TestDispatchCommandRunsListAliases(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, command := range []string{"list", "ls"} {
		var output bytes.Buffer
		handled, err := dispatchCommand([]string{command}, &output)
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if !handled || !strings.Contains(output.String(), "hso setup") {
			t.Fatalf("%s: handled=%t, output=%q", command, handled, output.String())
		}
	}
}

func TestDispatchListRejectsArguments(t *testing.T) {
	handled, err := dispatchCommand([]string{"list", "extra"}, &bytes.Buffer{})
	if !handled || err == nil {
		t.Fatalf("handled = %t, err = %v", handled, err)
	}
}

func TestTrackRegisteredServerCreatesPIDFile(t *testing.T) {
	configHome := t.TempDir()
	runtimeDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDirectory)
	configPath := filepath.Join(t.TempDir(), "hso.toml")
	registryPath, err := registry.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Save(registryPath, registry.Registry{Servers: []registry.Server{
		{Name: "survival", Config: configPath},
	}}); err != nil {
		t.Fatal(err)
	}

	file, err := trackRegisteredServer(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if file == nil {
		t.Fatal("登録済みの設定に pidfile が作られなかった")
	}
	t.Cleanup(file.Close)
	pid, running, err := pidfile.Running("survival")
	if err != nil {
		t.Fatal(err)
	}
	if !running || pid != os.Getpid() {
		t.Fatalf("pid = %d, running = %t", pid, running)
	}
}

func TestTrackUnregisteredServerDoesNotCreatePIDFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	file, err := trackRegisteredServer("/srv/unregistered/hso.toml")
	if err != nil {
		t.Fatal(err)
	}
	if file != nil {
		file.Close()
		t.Fatal("一覧にない設定へ pidfile が作られた")
	}
}
