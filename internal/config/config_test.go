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
	// 何をすればいいか分かるよう、直し方とファイルの場所も添える。
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v", err)
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSaveWritesReloadableConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	cfg := Config{
		Server: Server{Command: "./run.sh", WorkDir: dir},
		UI:     UI{Theme: Theme{Frame: "neon", Graph: "safe", Log: "cool"}},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Command != cfg.Server.Command ||
		loaded.UI.Theme != cfg.UI.Theme {
		t.Fatalf("loaded = %#v", loaded)
	}
	// 途中で失敗しても元の設定を壊さないよう一時ファイル経由で書くが、
	// 成功した後に残っていてはいけない。
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file was left: %v", err)
	}
}

// 配色を変えただけで、ユーザーが絞っていた権限が広がってはいけない。
func TestSaveKeepsExistingPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	writeConfig(t, path, "[server]\ncommand = \"./run.sh\"\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.UI.Theme.Frame = "neon"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permission = %o", info.Mode().Perm())
	}
}

// 配色を変えただけで、相対指定の workdir が絶対パスに化けてはいけない。
func TestSaveKeepsRelativeWorkDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	writeConfig(t, path, "[server]\ncommand = \"./run.sh\"\nworkdir = \"server\"\n")
	if err := os.Mkdir(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.UI.Theme.Frame = "neon"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), `workdir = "server"`) {
		t.Fatalf("saved = %s", saved)
	}
}

// workdir を書いていなかった設定ファイルには、Load が入れた既定値を
// 書き足さない。
func TestSaveOmitsDefaultedWorkDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	writeConfig(t, path, "[server]\ncommand = \"./run.sh\"\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "workdir") {
		t.Fatalf("saved = %s", saved)
	}
}
