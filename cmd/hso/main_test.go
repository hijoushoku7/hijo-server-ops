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
		nil,
		{"-config", "/srv/minecraft/hso.toml"},
		{"-help"},
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

func TestDispatchCommandRunsVersion(t *testing.T) {
	previousVersion := version
	version = "v1.2.3"
	t.Cleanup(func() { version = previousVersion })

	var output bytes.Buffer
	handled, err := dispatchCommand([]string{"version"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("version がサブコマンドとして処理されなかった")
	}
	want := msg.VersionOutput("v1.2.3", msg.Lang, runtime.GOARCH) + "\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
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
	for _, want := range []string{"unknown", "version"} {
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
