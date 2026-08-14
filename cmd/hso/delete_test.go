package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

func TestDeleteFromRegistry(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "hso.toml")
	if err := os.WriteFile(configPath, []byte("[server]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := registry.Server{Name: "Survival", Config: configPath}

	tests := []struct {
		name       string
		options    deleteOptions
		input      string
		terminal   bool
		running    bool
		missing    bool
		wantSaved  bool
		wantError  string
		wantOutput string
	}{
		{name: "名前指定で削除する", options: deleteOptions{name: "survival"}, input: "y\n", terminal: true, wantSaved: true, wantOutput: msg.ServerDeleted(server.Name)},
		{name: "確認を拒否する", options: deleteOptions{name: "survival"}, input: "n\n", terminal: true, wantOutput: msg.Aborted},
		{name: "確認を省略する", options: deleteOptions{name: "survival", yes: true}, wantSaved: true, wantOutput: msg.ServerDeleted(server.Name)},
		{name: "起動中は拒否する", options: deleteOptions{name: "survival", yes: true}, running: true, wantError: "PID 4321"},
		{name: "未登録名を拒否する", options: deleteOptions{name: "creative", yes: true}, wantError: "creative"},
		{name: "設定ファイルがなくても削除する", options: deleteOptions{name: "survival", yes: true}, missing: true, wantSaved: true, wantOutput: msg.ServerDeleted(server.Name)},
		{name: "設定ファイルがなく起動中なら拒否する", options: deleteOptions{name: "survival", yes: true}, running: true, missing: true, wantError: "PID 4321"},
		{name: "端末なしでは確認できない", options: deleteOptions{name: "survival"}, wantError: msg.DeleteRequiresConfirmation().Error()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := configPath
			if test.missing {
				path = filepath.Join(directory, "missing.toml")
			}
			servers := registry.Registry{Servers: []registry.Server{{Name: server.Name, Config: path}}}
			var output bytes.Buffer
			saved := false
			err := deleteFromRegistry(test.options, func() (registry.Registry, error) { return servers, nil }, test.terminal, strings.NewReader(test.input), &output,
				func(registry.Registry) (registry.Server, bool, error) {
					t.Fatal("名前指定時にピッカーを呼んではいけない")
					return registry.Server{}, false, nil
				},
				func(string) (int, bool, error) { return 4321, test.running, nil },
				func(got registry.Registry) error {
					saved = true
					if len(got.Servers) != 0 {
						t.Fatalf("保存する一覧 = %#v", got.Servers)
					}
					return nil
				})
			if test.wantError == "" && err != nil || test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError)) {
				t.Fatalf("err = %v", err)
			}
			if saved != test.wantSaved {
				t.Fatalf("saved = %t, want %t", saved, test.wantSaved)
			}
			if test.wantOutput != "" && !strings.Contains(output.String(), test.wantOutput) {
				t.Fatalf("output = %q", output.String())
			}
			if test.wantError == "" && !strings.Contains(output.String(), msg.DeleteTarget(server.Name, path)) {
				t.Fatalf("output = %q", output.String())
			}
		})
	}
}

func TestDeleteFromRegistryRejectsEmptyList(t *testing.T) {
	err := deleteFromRegistry(deleteOptions{yes: true}, func() (registry.Registry, error) { return registry.Registry{}, nil }, false, strings.NewReader(""), &bytes.Buffer{}, nil, nil, nil)
	if err == nil || err.Error() != msg.NoRegisteredServers().Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteFromRegistryRequiresTerminalToChoose(t *testing.T) {
	servers := registry.Registry{Servers: []registry.Server{{Name: "survival", Config: "/missing/hso.toml"}}}
	err := deleteFromRegistry(deleteOptions{yes: true}, func() (registry.Registry, error) { return servers, nil }, false, strings.NewReader(""), &bytes.Buffer{}, nil, nil, nil)
	if err == nil || err.Error() != msg.DeleteRequiresTerminal().Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestParseDeleteOptions(t *testing.T) {
	for _, args := range [][]string{{"one", "two"}, {"--unknown"}} {
		if _, err := parseDeleteOptions(args); err == nil {
			t.Fatalf("args = %#v はエラーになるべき", args)
		}
	}
	for _, args := range [][]string{{"survival", "--yes"}, {"--yes", "survival"}} {
		options, err := parseDeleteOptions(args)
		if err != nil || !options.yes || options.name != "survival" {
			t.Fatalf("args = %#v, options = %#v, err = %v", args, options, err)
		}
	}
}

func TestDeleteFromRegistryReturnsSaveError(t *testing.T) {
	want := errors.New("save failed")
	servers := registry.Registry{Servers: []registry.Server{{Name: "survival", Config: "/missing/hso.toml"}}}
	err := deleteFromRegistry(deleteOptions{name: "survival", yes: true}, func() (registry.Registry, error) { return servers, nil }, false, strings.NewReader(""), &bytes.Buffer{}, nil,
		func(string) (int, bool, error) { return 0, false, nil }, func(registry.Registry) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteFromRegistryReloadsAfterConfirmation(t *testing.T) {
	target := registry.Server{Name: "survival", Config: "/servers/survival/hso.toml"}
	added := registry.Server{Name: "creative", Config: "/servers/creative/hso.toml"}
	tests := []struct {
		name          string
		reloaded      registry.Registry
		runningAfter  bool
		wantSaved     bool
		wantError     string
		wantRemaining []registry.Server
	}{
		{name: "対象が削除済み", reloaded: registry.Registry{}, wantError: msg.ServerNotRegistered(target.Name).Error()},
		{name: "対象が起動済み", reloaded: registry.Registry{Servers: []registry.Server{target}}, runningAfter: true, wantError: "PID 4321"},
		{name: "別のサーバーが追加済み", reloaded: registry.Registry{Servers: []registry.Server{target, added}}, wantSaved: true, wantRemaining: []registry.Server{added}},
		{
			name:      "同じ名前で別の設定へ差し替え済み",
			reloaded:  registry.Registry{Servers: []registry.Server{{Name: target.Name, Config: "/servers/other/hso.toml"}}},
			wantError: msg.DeleteTargetChanged(target.Name).Error(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loads := 0
			load := func() (registry.Registry, error) {
				loads++
				if loads == 1 {
					return registry.Registry{Servers: []registry.Server{target}}, nil
				}
				return test.reloaded, nil
			}
			runningChecks := 0
			running := func(string) (int, bool, error) {
				runningChecks++
				return 4321, test.runningAfter && runningChecks == 2, nil
			}
			var saved registry.Registry
			savedCalled := false
			err := deleteFromRegistry(deleteOptions{name: target.Name, yes: true}, load, false, strings.NewReader(""), &bytes.Buffer{}, nil, running,
				func(got registry.Registry) error {
					savedCalled = true
					saved = got
					return nil
				})
			if test.wantError == "" && err != nil || test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError)) {
				t.Fatalf("err = %v", err)
			}
			if savedCalled != test.wantSaved {
				t.Fatalf("saved = %t, want %t", savedCalled, test.wantSaved)
			}
			if test.wantSaved && len(saved.Servers) != len(test.wantRemaining) {
				t.Fatalf("保存する一覧 = %#v, want %#v", saved.Servers, test.wantRemaining)
			}
			for index := range test.wantRemaining {
				if saved.Servers[index] != test.wantRemaining[index] {
					t.Fatalf("保存する一覧 = %#v, want %#v", saved.Servers, test.wantRemaining)
				}
			}
		})
	}
}
