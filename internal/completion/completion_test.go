package completion

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

func TestCandidatesByPosition(t *testing.T) {
	servers := []registry.Server{
		{Name: "survival", Config: "/srv/minecraft/hso.toml"},
		{Name: "creative", Config: "/srv/creative/hso.toml"},
	}
	serverValues := []string{"survival", "creative"}
	tests := []struct {
		name  string
		words []string
		want  []string
	}{
		{name: "コマンド", words: []string{"hso", ""}, want: []string{"setup", "start", "cd", "list", "ls", "delete", "java", "completion", "version", "update", "uninstall", "help", "-config"}},
		{name: "start", words: []string{"hso", "start", ""}, want: serverValues},
		{name: "cd", words: []string{"hso", "cd", ""}, want: serverValues},
		{name: "delete", words: []string{"hso", "delete", ""}, want: []string{"survival", "creative", "-y", "--yes"}},
		{name: "java", words: []string{"hso", "java", ""}, want: []string{"change", "list"}},
		{name: "java change", words: []string{"hso", "java", "change", ""}, want: serverValues},
		{name: "uninstall", words: []string{"hso", "uninstall", ""}, want: []string{"--purge", "-y", "--yes"}},
		{name: "completion", words: []string{"hso", "completion", ""}, want: []string{"bash", "zsh", "fish"}},
		{name: "config", words: []string{"hso", "-config", ""}, want: []string{":files"}},
		{name: "その他", words: []string{"hso", "list", ""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotCandidates := Candidates(test.words, servers)
			var got []string
			for _, candidate := range gotCandidates {
				got = append(got, candidate.Value)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Candidates(%q) = %q, want %q", test.words, got, test.want)
			}
		})
	}
}

func TestServerCandidateDescriptionIsConfigPath(t *testing.T) {
	got := Candidates([]string{"hso", "start", ""}, []registry.Server{{Name: "survival", Config: "/srv/hso.toml"}})
	if len(got) != 1 || got[0].Description != "/srv/hso.toml" {
		t.Fatalf("candidates = %#v", got)
	}
}

func TestEmbeddedScriptsHaveValidSyntax(t *testing.T) {
	tests := []struct {
		shell string
		args  []string
		file  string
	}{
		{shell: "bash", args: []string{"-n"}, file: "hso.bash"},
		{shell: "zsh", args: []string{"-n"}, file: "_hso"},
		{shell: "fish", args: []string{"--no-execute"}, file: "hso.fish"},
	}
	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			if _, err := exec.LookPath(test.shell); err != nil {
				t.Skipf("%s がないためスキップします", test.shell)
			}
			path := filepath.Join("scripts", test.file)
			if output, err := exec.Command(test.shell, append(test.args, path)...).CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", path, err, output)
			}
		})
	}
}
