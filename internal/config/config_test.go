package config

import (
	"math"
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
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "hso setup") {
		t.Fatalf("err = %v", err)
	}
}

// 設定ファイルがまだ無いだけのときに、削除を促す案内を出してはいけない。
// 引数なしの hso はヘルプを出すだけなので、案内先は hso setup になる。
func TestLoadMissingFileGuidesToSetup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hso.toml")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "hso setup") {
		t.Fatalf("err = %v", err)
	}
	// ja / en どちらの古い案内も残っていないことを見る。
	for _, stale := range []string{"削除", "delete", "hso を起動", "start hso"} {
		if strings.Contains(err.Error(), stale) {
			t.Fatalf("案内に %q が残っている: %v", stale, err)
		}
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
		UI: UI{Theme: Theme{
			Background: "night", Frame: "neon", Graph: "safe", Log: "cool",
		}},
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

func TestSaveRoundTripsAutoRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	cfg := Config{Server: Server{
		Command:     "./run.sh",
		WorkDir:     dir,
		AutoRestart: true,
	}}

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Server.AutoRestart {
		t.Fatalf("loaded = %#v", loaded.Server)
	}

	// 既定の無効は書かない。書いていない設定ファイルをそのまま保つ。
	loaded.Server.AutoRestart = false
	if err := Save(path, loaded); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "auto_restart") {
		t.Fatalf("written:\n%s", written)
	}
}

func TestNormalizeTimeOffset(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{input: 14, want: 0},
		{input: 15, want: 30},
		{input: -14, want: 0},
		{input: -15, want: -30},
		{input: 734, want: 720},
		{input: 735, want: 720},
		{input: -704, want: -690},
		{input: -705, want: -690},
		{input: -720, want: -690},
		{input: -1000, want: -690},
		// 手書きの設定ファイルには int の端の値も書ける。丸めを先に
		// 掛けると足し算が回り込んで反対側へ振れる。
		{input: math.MaxInt, want: 720},
		{input: math.MinInt, want: -690},
	}
	for _, test := range tests {
		if got := normalizeTimeOffset(test.input); got != test.want {
			t.Errorf("normalizeTimeOffset(%d) = %d, want %d", test.input, got, test.want)
		}
	}
	// 正規化した値をもう一度通しても動かない（Load を繰り返しても安定する）。
	for offset := -690; offset <= 720; offset += 30 {
		if got := normalizeTimeOffset(offset); got != offset {
			t.Errorf("normalizeTimeOffset(%d) = %d, 不動点でない", offset, got)
		}
	}
}

func TestLoadNormalizesTimeOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	writeConfig(t, path, "[server]\ncommand = \"./run.sh\"\n\n"+
		"[ui.time]\noffset_minutes = 94\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.Time.OffsetMinutes != 90 {
		t.Fatalf("OffsetMinutes = %d", cfg.UI.Time.OffsetMinutes)
	}
}

func TestSaveRoundTripsTimeOffsetAndOmitsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	cfg := Config{
		Server: Server{Command: "./run.sh", WorkDir: dir},
		UI:     UI{Time: Time{OffsetMinutes: 90}},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UI.Time.OffsetMinutes != 90 {
		t.Fatalf("loaded = %#v", loaded.UI.Time)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "[ui.time]\noffset_minutes = 90") {
		t.Fatalf("written:\n%s", written)
	}

	loaded.UI.Time.OffsetMinutes = 0
	if err := Save(path, loaded); err != nil {
		t.Fatal(err)
	}
	written, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "[ui.time]") {
		t.Fatalf("written:\n%s", written)
	}
}
