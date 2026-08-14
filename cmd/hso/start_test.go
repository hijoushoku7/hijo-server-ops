package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

func TestStartWithNameLaunchesRegisteredConfig(t *testing.T) {
	configPath := writeStartConfig(t, "survival.toml")
	servers := registry.Registry{Servers: []registry.Server{
		{Name: "Survival", Config: configPath},
	}}
	launched := ""
	err := startFromRegistry(
		"survival",
		servers,
		func(registry.Registry) (registry.Server, bool, error) {
			t.Fatal("名前があるときに選択 UI を呼んではいけない")
			return registry.Server{}, false, nil
		},
		func(string) (int, bool, error) { return 0, false, nil },
		func(path string) error {
			launched = path
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if launched != configPath {
		t.Fatalf("launched = %q, want %q", launched, configPath)
	}
}

func TestStartWithUnknownNameReturnsError(t *testing.T) {
	configPath := writeStartConfig(t, "survival.toml")
	err := startFromRegistry(
		"creative",
		registry.Registry{Servers: []registry.Server{{Name: "survival", Config: configPath}}},
		nil,
		func(string) (int, bool, error) { return 0, false, nil },
		func(string) error {
			t.Fatal("一覧にない名前を起動してはいけない")
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "creative") {
		t.Fatalf("err = %v", err)
	}
}

func TestStartWithoutNameLaunchesSelection(t *testing.T) {
	firstPath := writeStartConfig(t, "survival.toml")
	secondPath := writeStartConfig(t, "creative.toml")
	servers := registry.Registry{Servers: []registry.Server{
		{Name: "survival", Config: firstPath},
		{Name: "creative", Config: secondPath},
	}}
	launched := ""
	err := startFromRegistry(
		"",
		servers,
		func(got registry.Registry) (registry.Server, bool, error) {
			if len(got.Servers) != 2 {
				t.Fatalf("servers = %#v", got.Servers)
			}
			return got.Servers[1], true, nil
		},
		func(string) (int, bool, error) { return 0, false, nil },
		func(path string) error {
			launched = path
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if launched != secondPath {
		t.Fatalf("launched = %q, want %q", launched, secondPath)
	}
}

func TestStartRejectsRunningSelection(t *testing.T) {
	configPath := writeStartConfig(t, "survival.toml")
	server := registry.Server{Name: "survival", Config: configPath}
	launched := false
	err := startFromRegistry(
		"",
		registry.Registry{Servers: []registry.Server{server}},
		func(registry.Registry) (registry.Server, bool, error) { return server, true, nil },
		func(string) (int, bool, error) { return 4321, true, nil },
		func(string) error {
			launched = true
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "4321") {
		t.Fatalf("err = %v", err)
	}
	if launched {
		t.Fatal("起動中のサーバーを再び起動してはいけない")
	}
}

func TestStartReportsExclusiveCreateConflictAsAlreadyRunning(t *testing.T) {
	configPath := writeStartConfig(t, "survival.toml")
	server := registry.Server{Name: "survival", Config: configPath}
	err := startFromRegistry(
		"survival",
		registry.Registry{Servers: []registry.Server{server}},
		nil,
		func(string) (int, bool, error) { return 0, false, nil },
		func(string) error { return fmt.Errorf("track server: %w", pidfile.ErrAlreadyRunning) },
	)
	if err == nil || err.Error() != msg.ServerAlreadyRunningWithoutPID(server.Name).Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestStartRejectsMissingSelectedConfig(t *testing.T) {
	server := registry.Server{Name: "missing", Config: filepath.Join(t.TempDir(), "hso.toml")}
	launched := false
	err := startFromRegistry(
		"",
		registry.Registry{Servers: []registry.Server{server}},
		func(registry.Registry) (registry.Server, bool, error) { return server, true, nil },
		func(string) (int, bool, error) {
			t.Fatal("設定がないエントリでは pidfile を調べるべきではない")
			return 0, false, nil
		},
		func(string) error {
			launched = true
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), server.Config) {
		t.Fatalf("err = %v", err)
	}
	if launched {
		t.Fatal("設定がないエントリを起動してはいけない")
	}
}

func TestRunStartWithoutNameRejectsNonTerminal(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	configPath := writeStartConfig(t, "survival.toml")
	path, err := registry.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Save(path, registry.Registry{Servers: []registry.Server{
		{Name: "survival", Config: configPath},
	}}); err != nil {
		t.Fatal(err)
	}

	err = runStartWithTerminal("", false)
	if err == nil || err.Error() != msg.StartRequiresTerminal().Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestStartModelSelectsWithArrowAndEnter(t *testing.T) {
	model := &startModel{rows: []startRow{
		{server: registry.Server{Name: "survival"}, status: serverStatus{state: serverRunning, pid: 123}},
		{server: registry.Server{Name: "creative"}, status: serverStatus{state: serverStopped}},
		{server: registry.Server{Name: "missing"}, status: serverStatus{state: serverConfigMissing}},
	}}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !model.chosen || model.selected.Name != "creative" || command == nil {
		t.Fatalf("chosen = %t, selected = %#v, command = %v", model.chosen, model.selected, command)
	}
	view := model.View().Content
	for _, want := range []string{
		"survival", msg.ServerRunning(123), "creative", msg.ServerStopped, "missing", msg.ConfigNotFound,
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view = %q に %q がない", view, want)
		}
	}
}

func TestDispatchStartRejectsExtraArguments(t *testing.T) {
	handled, err := dispatchCommand([]string{"start", "one", "two"}, &strings.Builder{})
	if !handled || err == nil {
		t.Fatalf("handled = %t, err = %v", handled, err)
	}
}

func writeStartConfig(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("[server]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
