package registry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{name: "", valid: false},
		{name: "a", valid: true},
		{name: strings.Repeat("a", 30), valid: true},
		{name: strings.Repeat("a", 31), valid: false},
		{name: "-server", valid: false},
		{name: "_server", valid: false},
		{name: ".server", valid: false},
		{name: "server-1_test.local", valid: true},
		{name: "サーバー", valid: false},
	}
	for _, test := range tests {
		err := ValidateName(test.name)
		if (err == nil) != test.valid {
			t.Errorf("ValidateName(%q) = %v, valid=%t", test.name, err, test.valid)
		}
		if err != nil {
			for _, rule := range []string{"30", "-", "_", "."} {
				if !strings.Contains(err.Error(), rule) {
					t.Errorf("ValidateName(%q) のエラー %q に規則 %q がない", test.name, err, rule)
				}
			}
		}
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeRegistry(t, path, "[[servers]]\nname = \"survival\"\nconfig = \"/srv/hso.toml\"\ncommand = \"./run.sh\"\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "servers.command") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsInvalidName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeRegistry(t, path, "[[servers]]\nname = \"../server\"\nconfig = \"/srv/hso.toml\"\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "../server") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsCaseInsensitiveDuplicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeRegistry(t, path, "[[servers]]\nname = \"survival\"\nconfig = \"/srv/one/hso.toml\"\n\n"+
		"[[servers]]\nname = \"Survival\"\nconfig = \"/srv/two/hso.toml\"\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "Survival") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadKeepsMissingConfigEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeRegistry(t, path, "[[servers]]\nname = \"survival\"\nconfig = \"/missing/hso.toml\"\n")

	registry, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Servers) != 1 || registry.Servers[0].Config != "/missing/hso.toml" {
		t.Fatalf("registry = %#v", registry)
	}
}

func TestSaveRoundTripsAndRemovesTemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hso", "config.toml")
	want := Registry{Servers: []Server{{Name: "survival", Config: "/srv/minecraft/hso.toml"}}}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"[[servers]]", `name = "survival"`, `config = "/srv/minecraft/hso.toml"`} {
		if !strings.Contains(string(written), value) {
			t.Errorf("written = %q に %q がない", written, value)
		}
	}
	for _, unwanted := range []string{"command", "workdir"} {
		if strings.Contains(string(written), unwanted) {
			t.Errorf("written = %q に不要な項目 %q がある", written, unwanted)
		}
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Servers) != 1 || got.Servers[0] != want.Servers[0] {
		t.Fatalf("got = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("一時ファイルが残った: %v", err)
	}
}

func TestSaveRenameFailureKeepsOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := []byte("[[servers]]\nname = \"old\"\nconfig = \"/old/hso.toml\"\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("rename failed")
	err := save(path, Registry{Servers: []Server{{Name: "new", Config: "/new/hso.toml"}}},
		os.WriteFile,
		func(string, string) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("元の一覧が変わった:\n%s", got)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("失敗後に一時ファイルが残った: %v", err)
	}
}

func TestSaveWriteFailureRemovesTemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	wantErr := errors.New("write failed")
	writeFile := func(path string, _ []byte, permission os.FileMode) error {
		if err := os.WriteFile(path, []byte("incomplete"), permission); err != nil {
			t.Fatal(err)
		}
		return wantErr
	}
	err := save(path, Registry{Servers: []Server{{Name: "server", Config: "/srv/hso.toml"}}},
		writeFile, os.Rename)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("書き込み失敗後に一時ファイルが残った: %v", err)
	}
}

func TestPathUsesXDGConfigHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, "hso", "config.toml") {
		t.Fatalf("path = %q", path)
	}
}

func TestPathFallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(home, ".config", "hso", "config.toml") {
		t.Fatalf("path = %q", path)
	}
}

func writeRegistry(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
