package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
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

var escape = tea.KeyPressMsg{Code: tea.KeyEscape}

var ctrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

func TestRegisterModelFlow(t *testing.T) {
	cfg := config.Config{Server: config.Server{Command: "./run.sh", WorkDir: "/srv/minecraft"}}
	model := newRegisterModel("/srv/minecraft/hso.toml", cfg, registry.Registry{})
	if model.step != stepRegisterNotice {
		t.Fatalf("step = %d", model.step)
	}
	view := model.View().Content
	if !strings.Contains(view, cfg.Server.Command) || !strings.Contains(view, cfg.Server.WorkDir) {
		t.Fatalf("既存設定の内容が表示されていない: %q", view)
	}
	// 登録ウィザードは hso.toml を作らないので、作成ウィザードの見出しを
	// そのまま出すと嘘になる。
	if !strings.Contains(view, msg.SetupRegisterTarget("/srv/minecraft/hso.toml")) {
		t.Fatalf("登録ウィザードの見出しが違う: %q", view)
	}
	if strings.Contains(view, msg.SetupTarget("/srv/minecraft/hso.toml")) {
		t.Fatalf("作成ウィザードの見出しが出ている: %q", view)
	}

	press(t, model, enter)
	if model.step != stepName || string(model.input) != "minecraft" {
		t.Fatalf("step = %d, input = %q", model.step, model.input)
	}
	press(t, model, enter)
	if !model.created || model.name != "minecraft" {
		t.Fatalf("created = %v, name = %q", model.created, model.name)
	}
}

func TestRegisterModelCanDeclineAndGoBack(t *testing.T) {
	cfg := config.Config{Server: config.Server{Command: "./run.sh", WorkDir: "/srv/minecraft"}}
	declined := newRegisterModel("/srv/minecraft/hso.toml", cfg, registry.Registry{})
	press(t, declined, escape)
	if declined.created {
		t.Fatal("追加を断ったのに登録済みになった")
	}

	model := newRegisterModel("/srv/minecraft/hso.toml", cfg, registry.Registry{})
	press(t, model, enter, escape)
	if model.step != stepRegisterNotice {
		t.Fatalf("step = %d", model.step)
	}
}

func TestRegisterModelCtrlCCancels(t *testing.T) {
	cfg := config.Config{Server: config.Server{Command: "./run.sh", WorkDir: "/srv/minecraft"}}
	for _, step := range []string{"案内画面", "名前入力画面"} {
		t.Run(step, func(t *testing.T) {
			model := newRegisterModel("/srv/minecraft/hso.toml", cfg, registry.Registry{})
			if step == "名前入力画面" {
				press(t, model, enter)
			}
			press(t, model, ctrlC)
			if !model.canceled || model.created {
				t.Fatalf("canceled = %v, created = %v", model.canceled, model.created)
			}
		})
	}
}

func TestRegisterModelRejectsDuplicateNameIgnoringCase(t *testing.T) {
	cfg := config.Config{Server: config.Server{Command: "./run.sh", WorkDir: "/srv/minecraft"}}
	model := newRegisterModel("/srv/minecraft/hso.toml", cfg, registry.Registry{Servers: []registry.Server{
		{Name: "Minecraft", Config: "/srv/other/hso.toml"},
	}})
	press(t, model, enter, enter)
	if model.step != stepName || model.message == "" || model.created {
		t.Fatalf("step = %d, message = %q, created = %v", model.step, model.message, model.created)
	}
}

func TestModelCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "run.sh"), 0o644)
	path := filepath.Join(dir, "hso.toml")

	model := newModel(path, registry.Registry{})
	// 初期値のディレクトリと名前を確定し、一覧から run.sh を選ぶ。
	press(t, model, enter)
	if model.step != stepName || string(model.input) != filepath.Base(dir) {
		t.Fatalf("step = %d, input = %q", model.step, string(model.input))
	}
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

func TestModelSelectsCommandWithNumber(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "run.sh"), 0o755)
	writeFile(t, filepath.Join(dir, "start.sh"), 0o755)

	model := newModel(filepath.Join(dir, "hso.toml"), registry.Registry{})
	press(t, model, enter, enter)
	if model.step != stepCommand || len(model.candidates) != 2 {
		t.Fatalf("step = %d, candidates = %#v", model.step, model.candidates)
	}
	press(t, model, typeText("3"))
	if model.cursor != 2 || model.step != stepCommand {
		t.Fatalf("cursor = %d, step = %d", model.cursor, model.step)
	}
	press(t, model, typeText("9"))
	if model.cursor != 2 {
		t.Fatalf("範囲外の番号で cursor = %d", model.cursor)
	}
	view := model.View().Content
	if !strings.Contains(view, "3 "+msg.SetupManualEntry) || !strings.Contains(view, "↑↓ / 1-9") {
		t.Fatalf("view = %q", view)
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

	model := newModel(path, registry.Registry{})
	// サーバーディレクトリを打ち直してから、手入力でパスを指定する。
	press(t, model, typeText("/server"), enter)
	if model.workDir != server {
		t.Fatalf("workDir = %q (%s)", model.workDir, model.message)
	}
	press(t, model, enter, tea.KeyPressMsg{Code: tea.KeyEnd}, enter)
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
	model := newModel(filepath.Join(t.TempDir(), "hso.toml"), registry.Registry{})
	press(t, model, typeText("/no-such-directory"), enter)
	if model.step != stepWorkDir || model.message == "" {
		t.Fatalf("step = %d, message = %q", model.step, model.message)
	}
}

// 手入力から進んだ確認画面の Esc は、一覧ではなく入力画面へ戻る。
func TestModelEscapeReturnsToManualEntry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "run.sh"), 0o755)

	model := newModel(filepath.Join(dir, "hso.toml"), registry.Registry{})
	press(t, model, enter, enter)
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

	model := newModel(path, registry.Registry{})
	press(t, model, enter, enter, enter)
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

	model := newModel(filepath.Join(dir, "hso.toml"), registry.Registry{})
	press(t, model, enter, enter)
	press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.step != stepName || string(model.input) != filepath.Base(dir) {
		t.Fatalf("step = %d, input = %q", model.step, string(model.input))
	}
}

func TestDefaultServerName(t *testing.T) {
	if got := defaultServerName("/srv/survival-1"); got != "survival-1" {
		t.Fatalf("有効なディレクトリ名の初期値 = %q", got)
	}
	for _, path := range []string{"/srv/日本語", "/srv/my server"} {
		if got := defaultServerName(path); got != "" {
			t.Errorf("defaultServerName(%q) = %q, want empty", path, got)
		}
	}
}

func TestModelLeavesInvalidDirectoryNameEmpty(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "日本語 サーバー")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	model := newModel(filepath.Join(dir, "hso.toml"), registry.Registry{})
	press(t, model, enter)
	if model.step != stepName || len(model.input) != 0 {
		t.Fatalf("step = %d, input = %q", model.step, string(model.input))
	}
}

func TestModelRejectsDuplicateNameIgnoringCase(t *testing.T) {
	dir := t.TempDir()
	model := newModel(filepath.Join(dir, "hso.toml"), registry.Registry{Servers: []registry.Server{
		{Name: "Survival", Config: "/srv/one/hso.toml"},
	}})
	press(t, model, enter)
	model.input = []rune("survival")
	press(t, model, enter)
	if model.step != stepName || model.message == "" {
		t.Fatalf("step = %d, message = %q", model.step, model.message)
	}
}

func TestRegisterServerRejectsDuplicateIgnoringCase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := registry.Save(path, registry.Registry{Servers: []registry.Server{
		{Name: "Survival", Config: "/srv/one/hso.toml"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := registerServer(path, "survival", "/srv/two/hso.toml"); err == nil {
		t.Fatal("保存直前にも重複を拒否するべき")
	}
}

func TestRegisterServerSavesNameAndConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	configPath := "/srv/minecraft/hso.toml"
	if err := registerServer(path, "survival", configPath); err != nil {
		t.Fatal(err)
	}
	servers, err := registry.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers.Servers) != 1 || servers.Servers[0] != (registry.Server{
		Name: "survival", Config: configPath,
	}) {
		t.Fatalf("servers = %#v", servers.Servers)
	}
}

func TestRunRejectsRegisteredConfigBeforeWizard(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
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

	created, err := Run(configPath)
	if err == nil || !strings.Contains(err.Error(), "hso delete survival") {
		t.Fatalf("created = %q, err = %v", created, err)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("設定ファイルを作ってはいけない: %v", statErr)
	}
}

func TestRegisterRejectsRegisteredConfigBeforeWizard(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
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

	name, canceled, err := Register(configPath, config.Config{})
	if err == nil || !strings.Contains(err.Error(), "hso delete survival") {
		t.Fatalf("name = %q, canceled = %v, err = %v", name, canceled, err)
	}
}

func TestRegisterSavesNameAndConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "hso.toml")
	cfg := config.Config{Server: config.Server{Command: "./run.sh", WorkDir: filepath.Dir(configPath)}}

	name, canceled, err := register(configPath, cfg, func(model *model) error {
		model.name = "survival"
		model.created = true
		return nil
	})
	if err != nil || canceled || name != "survival" {
		t.Fatalf("name = %q, canceled = %v, err = %v", name, canceled, err)
	}
	registryPath, err := registry.Path()
	if err != nil {
		t.Fatal(err)
	}
	servers, err := registry.Load(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	wantPath, err := filepath.Abs(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers.Servers) != 1 || servers.Servers[0] != (registry.Server{Name: "survival", Config: wantPath}) {
		t.Fatalf("servers = %#v", servers.Servers)
	}
}
