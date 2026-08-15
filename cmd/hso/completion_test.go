package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompleteWritesTabSeparatedCandidates(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var output bytes.Buffer
	handled, err := dispatchCommand([]string{"__complete", "hso", "java", ""}, &output)
	if err != nil || !handled {
		t.Fatalf("handled = %t, err = %v", handled, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if !strings.Contains(line, "\t") {
			t.Errorf("候補がタブ区切りでない: %q", line)
		}
	}
}

func TestCompleteIgnoresBrokenRegistry(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	path := configHome + "/hso/config.toml"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	handled, err := dispatchCommand([]string{"__complete", "hso", ""}, &output)
	if err != nil || !handled {
		t.Fatalf("handled = %t, err = %v", handled, err)
	}
	if !strings.Contains(output.String(), "setup\t") || strings.Contains(output.String(), "broken") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestCompletionCommandPrintsEmbeddedScript(t *testing.T) {
	var output bytes.Buffer
	handled, err := dispatchCommand([]string{"completion", "bash"}, &output)
	if err != nil || !handled {
		t.Fatalf("handled = %t, err = %v", handled, err)
	}
	if !strings.Contains(output.String(), "hso __complete") {
		t.Fatalf("output = %q", output.String())
	}
}
