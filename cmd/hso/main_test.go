package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
)

func TestDispatchCommandKeepsTUIArguments(t *testing.T) {
	for _, args := range [][]string{
		{"-config", "/srv/minecraft/hso.toml"},
		{"-config=/srv/minecraft/hso.toml"},
	} {
		handled, err := dispatchCommand(args, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("args=%q: %v", args, err)
		}
		if handled {
			t.Errorf("args=%q がサブコマンドとして処理された", args)
		}
	}
}

// 引数なしの hso とヘルプの呼び方は、どれもコマンド一覧を出して終わる。
// ここが TUI 経路へ落ちると、打ち間違いでセットアップが始まる。
func TestDispatchCommandWritesHelp(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"help"},
		{"-h"},
		{"-help"},
		{"--help"},
		{"help", "setup"},
	} {
		var output bytes.Buffer
		handled, err := dispatchCommand(args, &output)
		if err != nil {
			t.Fatalf("args=%q: %v", args, err)
		}
		if !handled {
			t.Errorf("args=%q がヘルプとして処理されなかった", args)
		}
		if output.String() != msg.CommandHelp+"\n" {
			t.Errorf("args=%q: output = %q", args, output.String())
		}
	}
}

func TestRunExistingConfigBranches(t *testing.T) {
	loadErr := errors.New("設定読み込み失敗")
	tests := []struct {
		name         string
		setupCommand bool
		terminal     bool
		registered   bool
		promptName   string
		canceled     bool
		loadErr      error
		wantPrompt   int
		wantLaunch   int
		wantErr      bool
		wantAbort    bool
	}{
		{name: "未登録を登録して起動", terminal: true, promptName: "survival", wantPrompt: 1, wantLaunch: 1},
		{name: "未登録の追加を断っても通常起動", terminal: true, wantPrompt: 1, wantLaunch: 1},
		{name: "setupで追加を断ると起動しない", setupCommand: true, terminal: true, wantPrompt: 1, wantAbort: true},
		{name: "Ctrl+Cで通常起動を中止", terminal: true, canceled: true, wantPrompt: 1, wantAbort: true},
		{name: "Ctrl+Cでsetupを中止", setupCommand: true, terminal: true, canceled: true, wantPrompt: 1, wantAbort: true},
		{name: "setupで登録済みはエラー", setupCommand: true, terminal: true, registered: true, wantErr: true},
		{name: "非端末の通常起動はプロンプトなし", wantLaunch: 1},
		{name: "非端末のsetupはエラー", setupCommand: true, wantErr: true},
		{name: "設定読み込み失敗では登録しない", terminal: true, loadErr: loadErr, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompted, launched := 0, 0
			var output bytes.Buffer
			err := runExistingConfig("hso.toml", tt.setupCommand, tt.terminal, &output,
				func(string) (config.Config, error) { return config.Config{}, tt.loadErr },
				func(string) (string, bool, error) { return "survival", tt.registered, nil },
				func(string, config.Config) (string, bool, error) {
					prompted++
					return tt.promptName, tt.canceled, nil
				},
				func(string, config.Config) error {
					launched++
					return nil
				})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if prompted != tt.wantPrompt || launched != tt.wantLaunch {
				t.Fatalf("prompted = %d, launched = %d", prompted, launched)
			}
			if strings.Contains(output.String(), msg.Aborted) != tt.wantAbort {
				t.Fatalf("output = %q", output.String())
			}
		})
	}
}

// ヘルプは打てるコマンドを全部載せる。ja / en のどちらかで書き落とすと、
// そのバイナリだけコマンドの存在が読めなくなる。
func TestCommandHelpListsEveryCommand(t *testing.T) {
	for _, command := range []string{
		"setup", "start", "list", "ls", "delete",
		"java change", "java list", "version", "update", "uninstall", "help",
	} {
		if !strings.Contains(msg.CommandHelp, command) {
			t.Errorf("ヘルプに %q がない", command)
		}
	}
}

func TestDispatchCommandRunsVersion(t *testing.T) {
	previousVersion := version
	previousURL := latestReleaseURL
	version = "v1.2.3"
	latestReleaseURL = "://通信失敗"
	t.Cleanup(func() {
		version = previousVersion
		latestReleaseURL = previousURL
	})

	// `-` 付きの書き方もヘルプと同じく version と同じものを出す。--help が
	// 効くなら --version も効くと思うのが自然で、flag の使い方表示に落ちる
	// と探しているものが出ない。
	for _, args := range [][]string{
		{"version"},
		{"-v"},
		{"--v"},
		{"-version"},
		{"--version"},
		{"--version", "extra"},
	} {
		var output bytes.Buffer
		handled, err := dispatchCommand(args, &output)
		if err != nil {
			t.Fatalf("args=%q: %v", args, err)
		}
		if !handled {
			t.Fatalf("args=%q がサブコマンドとして処理されなかった", args)
		}
		want := msg.VersionOutput("v1.2.3", msg.Lang, runtime.GOARCH) + "\n"
		if output.String() != want {
			t.Fatalf("args=%q: output = %q, want %q", args, output.String(), want)
		}
	}
}

func TestDispatchCommandRejectsUnknownCommand(t *testing.T) {
	handled, err := dispatchCommand([]string{"unknown"}, &bytes.Buffer{})
	if !handled {
		t.Fatal("未知のサブコマンドが TUI 経路へ渡された")
	}
	if err == nil {
		t.Fatal("未知のサブコマンドがエラーにならなかった")
	}
	for _, want := range []string{"unknown", "hso help"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q に %q がない", err, want)
		}
	}
}

func TestDispatchVersionRejectsArguments(t *testing.T) {
	handled, err := dispatchCommand([]string{"version", "extra"}, &bytes.Buffer{})
	if !handled || err == nil {
		t.Fatalf("handled = %t, err = %v", handled, err)
	}
}

func TestDispatchUpdateRejectsArguments(t *testing.T) {
	handled, err := dispatchCommand([]string{"update", "extra"}, &bytes.Buffer{})
	if !handled || err == nil {
		t.Fatalf("handled = %t, err = %v", handled, err)
	}
}

func TestDispatchSetupRejectsArguments(t *testing.T) {
	handled, err := dispatchCommand([]string{"setup", "extra"}, &bytes.Buffer{})
	if !handled || err == nil {
		t.Fatalf("handled = %t, err = %v", handled, err)
	}
}

func TestMissingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	if !missingConfig(path) {
		t.Fatal("ないファイルは missing")
	}
	if err := os.WriteFile(path, []byte("[server]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if missingConfig(path) {
		t.Fatal("あるファイルは missing ではない")
	}
	// 読めないディレクトリの下など、存在有無を判断できないものは
	// missing 扱いにしない（そのまま config.Load のエラーを出す）。
	if missingConfig(filepath.Join(path, "hso.toml")) {
		t.Fatal("ファイル配下のパスは missing ではない")
	}
}

func TestIsTerminal(t *testing.T) {
	// /dev/null はキャラクタデバイスだが端末ではない。ここを取り違えると
	// hso </dev/null でウィザードが開いて入力待ちのまま止まる。
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if isTerminal(file) {
		t.Fatal("/dev/null は端末ではない")
	}

	regular, err := os.Create(filepath.Join(t.TempDir(), "log"))
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	if isTerminal(regular) {
		t.Fatal("通常ファイルは端末ではない")
	}
}

func TestRunTrackedTUIStopsForUnsafePIDDirectory(t *testing.T) {
	launched := false
	unsafe := fmt.Errorf("pidfileディレクトリ: %w", pidfile.ErrUnsafeDirectory)
	err := runTrackedTUI("hso.toml", config.Config{},
		func(string) (*pidfile.File, error) { return nil, unsafe },
		func(string, config.Config) error {
			launched = true
			return nil
		})
	if !errors.Is(err, pidfile.ErrUnsafeDirectory) {
		t.Fatalf("err = %v", err)
	}
	if launched {
		t.Fatal("安全でないpidfileディレクトリでもTUIが起動された")
	}
}

func TestRunTrackedTUIStopsWhenRegisteredServerPIDFileCannotBeWritten(t *testing.T) {
	launched := false
	writeErr := syscall.EACCES
	err := runTrackedTUI("hso.toml", config.Config{},
		func(string) (*pidfile.File, error) { return nil, writeErr },
		func(string, config.Config) error {
			launched = true
			return nil
		})
	if !errors.Is(err, syscall.EACCES) {
		t.Fatalf("err = %v", err)
	}
	if launched {
		t.Fatal("pidfileを書けないのにTUIが起動された")
	}
}

func TestRunTrackedTUILaunchesUnregisteredConfigWithoutPIDFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	runtimeDirectory := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDirectory)
	configPath := filepath.Join(t.TempDir(), "hso.toml")
	launched := false

	err := runTrackedTUI(configPath, config.Config{}, trackRegisteredServer,
		func(path string, _ config.Config) error {
			launched = true
			if path != configPath {
				t.Fatalf("path = %q, want %q", path, configPath)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !launched {
		t.Fatal("一覧にない設定でTUIが起動されなかった")
	}
	if _, err := os.Stat(filepath.Join(runtimeDirectory, "hso")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("一覧にない設定でpidfileディレクトリが作られた: %v", err)
	}
}
