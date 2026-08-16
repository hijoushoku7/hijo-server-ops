package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

func TestCdWithNameOpensRegisteredServerDirectory(t *testing.T) {
	dir := t.TempDir()
	server := registry.Server{Name: "Survival", Config: filepath.Join(dir, "hso.toml")}
	opened := ""
	err := cdFromRegistry("survival", registry.Registry{Servers: []registry.Server{server}}, nil,
		func(got registry.Server, gotDir string) error {
			if got != server {
				t.Fatalf("server = %#v, want %#v", got, server)
			}
			opened = gotDir
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if opened != dir {
		t.Fatalf("opened = %q, want %q", opened, dir)
	}
}

func TestCdWithUnknownNameReturnsError(t *testing.T) {
	server := registry.Server{Name: "survival", Config: filepath.Join(t.TempDir(), "hso.toml")}
	err := cdFromRegistry("creative", registry.Registry{Servers: []registry.Server{server}}, nil,
		func(registry.Server, string) error {
			t.Fatal("一覧にないサーバーのディレクトリを開いてはいけない")
			return nil
		})
	if err == nil || err.Error() != msg.ServerNotRegistered("creative").Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestCdWithInvalidNameReturnsError(t *testing.T) {
	err := cdFromRegistry("../invalid", registry.Registry{Servers: []registry.Server{{Name: "survival"}}}, nil,
		func(registry.Server, string) error {
			t.Fatal("不正な名前でディレクトリを開いてはいけない")
			return nil
		})
	if err == nil || !strings.Contains(err.Error(), "../invalid") {
		t.Fatalf("err = %v", err)
	}
}

func TestCdWithEmptyRegistryReturnsError(t *testing.T) {
	err := cdFromRegistry("", registry.Registry{}, nil, nil)
	if err == nil || err.Error() != msg.NoRegisteredServers().Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestCdWithoutNameOpensSelectedServerDirectory(t *testing.T) {
	first := registry.Server{Name: "survival", Config: filepath.Join(t.TempDir(), "hso.toml")}
	secondDir := t.TempDir()
	second := registry.Server{Name: "creative", Config: filepath.Join(secondDir, "hso.toml")}
	servers := registry.Registry{Servers: []registry.Server{first, second}}
	opened := ""
	err := cdFromRegistry("", servers,
		func(got registry.Registry) (registry.Server, bool, error) {
			return got.Servers[1], true, nil
		},
		func(_ registry.Server, dir string) error {
			opened = dir
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if opened != secondDir {
		t.Fatalf("opened = %q, want %q", opened, secondDir)
	}
}

func TestCdWithoutNameDoesNothingWhenSelectionIsCanceled(t *testing.T) {
	server := registry.Server{Name: "survival", Config: filepath.Join(t.TempDir(), "hso.toml")}
	err := cdFromRegistry("", registry.Registry{Servers: []registry.Server{server}},
		func(registry.Registry) (registry.Server, bool, error) {
			return registry.Server{}, false, nil
		},
		func(registry.Server, string) error {
			t.Fatal("キャンセル後にディレクトリを開いてはいけない")
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
}

func TestServerDirectoryDoesNotRequireConfigFile(t *testing.T) {
	dir := t.TempDir()
	got, err := serverDirectory(registry.Server{Name: "survival", Config: filepath.Join(dir, "hso.toml")})
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("dir = %q, want %q", got, dir)
	}
}

func TestServerDirectoryRejectsMissingDirectory(t *testing.T) {
	server := registry.Server{Name: "survival", Config: filepath.Join(t.TempDir(), "missing", "hso.toml")}
	dir := filepath.Dir(server.Config)
	_, err := serverDirectory(server)
	if err == nil || err.Error() != msg.ServerDirectoryNotFound(server.Name, dir).Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestServerDirectoryRejectsNonDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "server")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	server := registry.Server{Name: "survival", Config: filepath.Join(file, "hso.toml")}
	_, err := serverDirectory(server)
	if err == nil || err.Error() != msg.ServerDirectoryNotFound(server.Name, file).Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestShellPath(t *testing.T) {
	t.Run("設定あり", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/bash")
		if got := shellPath(); got != "/bin/bash" {
			t.Fatalf("shellPath() = %q", got)
		}
	})
	t.Run("空", func(t *testing.T) {
		t.Setenv("SHELL", "")
		if got := shellPath(); got != "/bin/sh" {
			t.Fatalf("shellPath() = %q", got)
		}
	})
	t.Run("未設定", func(t *testing.T) {
		original, found := os.LookupEnv("SHELL")
		if err := os.Unsetenv("SHELL"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if found {
				_ = os.Setenv("SHELL", original)
			} else {
				_ = os.Unsetenv("SHELL")
			}
		})
		if got := shellPath(); got != "/bin/sh" {
			t.Fatalf("shellPath() = %q", got)
		}
	})
}

func TestDispatchCdRejectsExtraArguments(t *testing.T) {
	handled, err := dispatchCommand([]string{"cd", "one", "two"}, &strings.Builder{})
	if !handled || err == nil || err.Error() != msg.CdArgumentsNotAllowed().Error() {
		t.Fatalf("handled = %t, err = %v", handled, err)
	}
}
