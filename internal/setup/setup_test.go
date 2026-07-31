package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func writeFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func TestScanCommands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "start.sh"), 0o644)
	writeFile(t, filepath.Join(dir, "run.sh"), 0o755)
	writeFile(t, filepath.Join(dir, "server.jar"), 0o755)
	writeFile(t, filepath.Join(dir, "hso"), 0o755)
	if err := os.Mkdir(filepath.Join(dir, "world"), 0o755); err != nil {
		t.Fatal(err)
	}

	candidates := scanCommands(dir)
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v", candidates)
	}
	// 実行可能なものが先頭に来る。
	if candidates[0].name != "run.sh" || !candidates[0].executable {
		t.Fatalf("candidates[0] = %#v", candidates[0])
	}
	if candidates[1].name != "start.sh" || candidates[1].executable {
		t.Fatalf("candidates[1] = %#v", candidates[1])
	}
}

func TestResolveCommandRelative(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "run.sh"), 0o755)

	command, path, err := resolveCommand("run.sh", dir)
	if err != nil {
		t.Fatal(err)
	}
	if command != "./run.sh" {
		t.Fatalf("command = %q", command)
	}
	if path != filepath.Join(dir, "run.sh") {
		t.Fatalf("path = %q", path)
	}
}

func TestResolveCommandOutsideWorkDir(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	script := filepath.Join(other, "run.sh")
	writeFile(t, script, 0o755)

	command, _, err := resolveCommand(script, dir)
	if err != nil {
		t.Fatal(err)
	}
	if command != script {
		t.Fatalf("command = %q", command)
	}
}

func TestResolveCommandMissing(t *testing.T) {
	if _, _, err := resolveCommand("missing.sh", t.TempDir()); err == nil {
		t.Fatal("エラーになるべき")
	}
	if _, _, err := resolveCommand("  ", t.TempDir()); err == nil {
		t.Fatal("空入力はエラーになるべき")
	}
}

func TestResolveWorkDir(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveWorkDir(dir)
	if err != nil || got != dir {
		t.Fatalf("got = %q, err = %v", got, err)
	}

	file := filepath.Join(dir, "run.sh")
	writeFile(t, file, 0o755)
	if _, err := resolveWorkDir(file); err == nil {
		t.Fatal("ファイルはエラーになるべき")
	}
}

func TestRender(t *testing.T) {
	same := render("./run.sh", "/srv/mc", "/srv/mc")
	if strings.Contains(same, "workdir") {
		t.Fatalf("workdir を省略すべき: %q", same)
	}
	differs := render("./run.sh", "/srv/mc", "/home/user")
	if !strings.Contains(differs, `workdir = "/srv/mc"`) {
		t.Fatalf("differs = %q", differs)
	}
}

func TestScanCommandsKeepsExtensionlessExecutable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "start"), 0o755)
	writeFile(t, filepath.Join(dir, "notes.txt"), 0o755)

	candidates := scanCommands(dir)
	if len(candidates) != 1 || candidates[0].name != "start" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestQuote(t *testing.T) {
	if got := quote(`a"b\c`); got != `"a\"b\\c"` {
		t.Fatalf("quote = %s", got)
	}
	if got := quote("a\nb\tc"); got != `"a\nb\tc"` {
		t.Fatalf("quote = %s", got)
	}
}

func TestWriteConfigRefusesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hso.toml")
	if err := writeConfig(path, "[server]\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(path, "[server]\n"); err == nil {
		t.Fatal("上書きしてはいけない")
	}
}

func TestGrantExecute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.sh")
	writeFile(t, path, 0o640)

	if err := grantExecute(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// 読める相手にだけ実行を許す。0o640 なら 0o750。
	if info.Mode().Perm() != 0o750 {
		t.Fatalf("perm = %o", info.Mode().Perm())
	}
}

func press(t *testing.T, model *model, keys ...tea.KeyPressMsg) {
	t.Helper()
	for _, key := range keys {
		_, _ = model.Update(key)
	}
}

func typeText(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: text}
}

var enter = tea.KeyPressMsg{Code: tea.KeyEnter}

func TestModelCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "run.sh"), 0o644)
	path := filepath.Join(dir, "hso.toml")

	model := newModel(path)
	// 初期値のディレクトリをそのまま確定し、一覧から run.sh を選ぶ。
	press(t, model, enter)
	if model.step != stepCommand {
		t.Fatalf("step = %d (%s)", model.step, model.message)
	}
	press(t, model, enter)
	if model.step != stepConfirm || !model.needsChmod {
		t.Fatalf("step = %d, needsChmod = %v", model.step, model.needsChmod)
	}
	press(t, model, enter)

	if !model.created || model.err != nil {
		t.Fatalf("created = %v, err = %v", model.created, model.err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "[server]\ncommand = \"./run.sh\"\n" {
		t.Fatalf("content = %q", content)
	}
	info, err := os.Stat(filepath.Join(dir, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("実行権限が付いていない: %o", info.Mode().Perm())
	}
}

func TestModelManualEntry(t *testing.T) {
	dir := t.TempDir()
	server := filepath.Join(dir, "server")
	if err := os.Mkdir(server, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(server, "start.sh"), 0o755)
	path := filepath.Join(dir, "hso.toml")

	model := newModel(path)
	// サーバーディレクトリを打ち直してから、手入力でパスを指定する。
	press(t, model, typeText("/server"), enter)
	if model.workDir != server {
		t.Fatalf("workDir = %q (%s)", model.workDir, model.message)
	}
	press(t, model, tea.KeyPressMsg{Code: tea.KeyEnd}, enter)
	if model.step != stepCommandInput {
		t.Fatalf("step = %d", model.step)
	}
	press(t, model, typeText("start.sh"), enter)
	if model.step != stepConfirm || model.needsChmod {
		t.Fatalf("step = %d, needsChmod = %v", model.step, model.needsChmod)
	}
	press(t, model, enter)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "[server]\ncommand = \"./start.sh\"\nworkdir = " + quote(server) + "\n"
	if string(content) != want {
		t.Fatalf("content = %q", content)
	}
}

func TestModelRejectsMissingDirectory(t *testing.T) {
	model := newModel(filepath.Join(t.TempDir(), "hso.toml"))
	press(t, model, typeText("/no-such-directory"), enter)
	if model.step != stepWorkDir || model.message == "" {
		t.Fatalf("step = %d, message = %q", model.step, model.message)
	}
}

// 手入力から進んだ確認画面の Esc は、一覧ではなく入力画面へ戻る。
func TestModelEscapeReturnsToManualEntry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "run.sh"), 0o755)

	model := newModel(filepath.Join(dir, "hso.toml"))
	press(t, model, enter)
	press(t, model, tea.KeyPressMsg{Code: tea.KeyEnd}, enter)
	press(t, model, typeText("run.sh"), enter)
	if model.step != stepConfirm {
		t.Fatalf("step = %d (%s)", model.step, model.message)
	}
	press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.step != stepCommandInput {
		t.Fatalf("step = %d", model.step)
	}
}

// c で実行権限の付与を断ると、chmod せずに設定だけ作る。
func TestModelDeclineChmod(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	writeFile(t, script, 0o644)
	path := filepath.Join(dir, "hso.toml")

	model := newModel(path)
	press(t, model, enter, enter)
	if !model.grantChmod {
		t.Fatal("既定では実行権限を付ける")
	}
	press(t, model, typeText("c"))
	if model.grantChmod {
		t.Fatal("c で切り替わるべき")
	}
	press(t, model, enter)

	if !model.created {
		t.Fatalf("created = %v, err = %v", model.created, model.err)
	}
	info, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("実行権限を付けてはいけない: %o", info.Mode().Perm())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestModelEscapeGoesBack(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "run.sh"), 0o755)

	model := newModel(filepath.Join(dir, "hso.toml"))
	press(t, model, enter)
	press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.step != stepWorkDir || string(model.input) != dir {
		t.Fatalf("step = %d, input = %q", model.step, string(model.input))
	}
}
