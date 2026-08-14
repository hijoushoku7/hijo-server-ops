package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/javaenv"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

func TestJavaCommandHelpDoesNotRequireTerminal(t *testing.T) {
	var output bytes.Buffer
	if err := runJava(nil, &output, &bytes.Buffer{}, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"hso java change [name]", "hso java list"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output = %q に %q がない", output.String(), want)
		}
	}
}

func TestJavaCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"other"}, {"list", "extra"}, {"change", "one", "two"}} {
		if err := runJava(args, &bytes.Buffer{}, &bytes.Buffer{}, false); err == nil {
			t.Errorf("runJava(%q) が成功した", args)
		}
	}
}

func TestChangeJavaSelectsServer(t *testing.T) {
	servers := registry.Registry{Servers: []registry.Server{{Name: "one", Config: "one.toml"}, {Name: "two", Config: "two.toml"}}}
	installation := javaenv.Installation{Home: "/usr/lib/jvm/jdk-21", Major: 21}
	for _, test := range []struct {
		name       string
		argument   string
		wantConfig string
		wantChoose bool
	}{
		{name: "名前あり", argument: "two", wantConfig: "two.toml"},
		{name: "名前なし", wantConfig: "one.toml", wantChoose: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			choseServer := false
			chooseServer := func(registry.Registry) (registry.Server, bool, error) {
				choseServer = true
				return servers.Servers[0], true, nil
			}
			setPath := ""
			err := changeJava(test.argument, servers, true, chooseServer,
				func([]javaenv.Installation, string, bool) (javaenv.Installation, bool, error) {
					return installation, true, nil
				},
				func(string) ([]javaenv.Installation, error) { return []javaenv.Installation{installation}, nil },
				func(string) (config.Config, error) { return config.Config{}, nil },
				func(path, _ string) error { setPath = path; return nil },
				func(string) (int, bool, error) { return 0, false, nil }, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			if setPath != test.wantConfig || choseServer != test.wantChoose {
				t.Fatalf("setPath = %q, choseServer = %t", setPath, choseServer)
			}
		})
	}
}

func TestChangeJavaSingleServerSkipsServerChooser(t *testing.T) {
	servers := registry.Registry{Servers: []registry.Server{{Name: "only", Config: "only.toml"}}}
	installation := javaenv.Installation{Home: "/usr/lib/jvm/jdk-21"}
	called := false
	err := changeJava("", servers, true,
		func(registry.Registry) (registry.Server, bool, error) {
			called = true
			return registry.Server{}, false, nil
		},
		func([]javaenv.Installation, string, bool) (javaenv.Installation, bool, error) {
			return installation, true, nil
		},
		func(string) ([]javaenv.Installation, error) { return []javaenv.Installation{installation}, nil },
		func(string) (config.Config, error) { return config.Config{}, nil },
		func(string, string) error { return nil }, func(string) (int, bool, error) { return 0, false, nil }, &bytes.Buffer{})
	if err != nil || called {
		t.Fatalf("err = %v, chooser called = %t", err, called)
	}
}

func TestChangeJavaErrorsAndCancellationDoNotSave(t *testing.T) {
	servers := registry.Registry{Servers: []registry.Server{{Name: "known", Config: "known.toml"}}}
	installation := javaenv.Installation{Home: "/usr/lib/jvm/jdk-21"}
	base := func(name string, terminal bool, load func(string) (config.Config, error), installed func(string) ([]javaenv.Installation, error), choose javaChooser) (error, bool) {
		saved := false
		err := changeJava(name, servers, terminal, nil, choose, installed, load,
			func(string, string) error { saved = true; return nil }, func(string) (int, bool, error) { return 0, false, nil }, &bytes.Buffer{})
		return err, saved
	}
	goodLoad := func(string) (config.Config, error) { return config.Config{}, nil }
	goodInstalled := func(string) ([]javaenv.Installation, error) { return []javaenv.Installation{installation}, nil }
	choose := func([]javaenv.Installation, string, bool) (javaenv.Installation, bool, error) {
		return installation, true, nil
	}
	for _, test := range []struct {
		name string
		err  error
		save bool
	}{
		{"未登録", func() error { err, _ := base("missing", true, goodLoad, goodInstalled, choose); return err }(), false},
		{"非端末", func() error { err, _ := base("known", false, goodLoad, goodInstalled, choose); return err }(), false},
		{"設定エラー", func() error {
			err, _ := base("known", true, func(string) (config.Config, error) { return config.Config{}, errors.New("broken") }, goodInstalled, choose)
			return err
		}(), false},
	} {
		if test.err == nil {
			t.Errorf("%s が成功した", test.name)
		}
	}
	err, saved := base("known", true, goodLoad, goodInstalled,
		func([]javaenv.Installation, string, bool) (javaenv.Installation, bool, error) {
			return javaenv.Installation{}, false, nil
		})
	if err != nil || saved {
		t.Fatalf("キャンセル: err = %v, saved = %t", err, saved)
	}
}

func TestChangeJavaNoInstallationsShowsScopedNote(t *testing.T) {
	servers := registry.Registry{Servers: []registry.Server{{Name: "known", Config: "known.toml"}}}
	var output bytes.Buffer
	saved := false
	err := changeJava("known", servers, true, nil, nil, func(string) ([]javaenv.Installation, error) { return nil, nil },
		func(string) (config.Config, error) { return config.Config{}, nil },
		func(string, string) error { saved = true; return nil }, nil, &output)
	if err != nil || saved {
		t.Fatalf("err = %v, saved = %t", err, saved)
	}
	assertJavaScopeNote(t, output.String())
}

func TestJavaModelCurrentSelectionCancellationAndRunningNotice(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "jdk-17")
	other := filepath.Join(root, "jdk-21")
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	model := newJavaModel([]javaenv.Installation{{Home: other, Major: 21}, {Home: current, Major: 17}}, current, true)
	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", model.cursor)
	}
	view := model.View().Content
	if !strings.Contains(view, msg.JavaCurrentMark) || !strings.Contains(view, msg.JavaRunningNotice) {
		t.Fatalf("view = %q", view)
	}
	assertJavaScopeNote(t, view)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.(*javaModel).chosen {
		t.Fatal("キャンセル後に選択済みになった")
	}
}

func TestPrintJavaListAssociatesSortedRowsAndContinuesWarnings(t *testing.T) {
	root := t.TempDir()
	java21 := filepath.Join(root, "jdk-21")
	java17 := filepath.Join(root, "jdk-17")
	for _, path := range []string{java21, java17} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	installations := []javaenv.Installation{
		{Home: java21, Major: 21, Implementor: "Adoptium"},
		{Home: java17, Major: 17, Implementor: "Debian"},
	}
	servers := registry.Registry{Servers: []registry.Server{
		{Name: "survival", Config: "survival"}, {Name: "modded", Config: "modded"},
		{Name: "test", Config: "test"}, {Name: "unset", Config: "unset"}, {Name: "broken", Config: "broken"},
	}}
	load := func(path string) (config.Config, error) {
		switch path {
		case "survival":
			return config.Config{Server: config.Server{Java: java21}}, nil
		case "modded", "test":
			return config.Config{Server: config.Server{Java: java17}}, nil
		case "unset":
			return config.Config{}, nil
		default:
			return config.Config{}, errors.New("broken config")
		}
	}
	var output, warnings bytes.Buffer
	if err := printJavaList(&output, &warnings, installations, servers, load); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Index(text, "21") > strings.Index(text, "17") || !strings.Contains(text, "modded, test") || !strings.Contains(text, "survival") {
		t.Fatalf("output = %q", text)
	}
	if !strings.Contains(warnings.String(), "unset") || !strings.Contains(warnings.String(), "broken") {
		t.Fatalf("warnings = %q", warnings.String())
	}
	assertJavaScopeNote(t, text)
}

func TestPrintJavaListNoInstallationsStillShowsNote(t *testing.T) {
	var output bytes.Buffer
	if err := printJavaList(&output, &bytes.Buffer{}, nil, registry.Registry{}, func(string) (config.Config, error) { return config.Config{}, nil }); err != nil {
		t.Fatal(err)
	}
	assertJavaScopeNote(t, output.String())
}

func assertJavaScopeNote(t *testing.T, text string) {
	t.Helper()
	for _, want := range []string{"/usr/lib/jvm", "SDKMAN", "asdf", "/opt", "hso.toml", "JAVA_HOME"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q に %q がない", text, want)
		}
	}
}

func TestDispatchCommandRunsJavaHelp(t *testing.T) {
	var output bytes.Buffer
	handled, err := dispatchCommand([]string{"java"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || !strings.Contains(output.String(), "hso java change") {
		t.Fatalf("handled = %t, output = %q", handled, output.String())
	}
}
