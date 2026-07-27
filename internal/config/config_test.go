package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	writeConfig(t, path, `
[server]
command = "./run.sh"
workdir = "server"

[ui]
panes = ["stats", "log"]
`)
	if err := os.Mkdir(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Command != "./run.sh" {
		t.Fatalf("Command = %q", cfg.Server.Command)
	}
	if cfg.Server.WorkDir != filepath.Join(dir, "server") {
		t.Fatalf("WorkDir = %q", cfg.Server.WorkDir)
	}
	if len(cfg.UI.Panes) != 2 || cfg.UI.Panes[1] != "log" {
		t.Fatalf("Panes = %#v", cfg.UI.Panes)
	}
}

func TestLoadDefaultsWorkDirToConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	writeConfig(t, path, "[server]\ncommand = \"./run.sh\"\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.WorkDir != dir {
		t.Fatalf("WorkDir = %q, want %q", cfg.Server.WorkDir, dir)
	}
}

func TestLoadRejectsMissingCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hso.toml")
	writeConfig(t, path, "[server]\nworkdir = \".\"\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "server.command") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hso.toml")
	writeConfig(t, path, "[server]\ncommand = \"./run.sh\"\ncommnad = \"typo\"\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "server.commnad") {
		t.Fatalf("err = %v", err)
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
