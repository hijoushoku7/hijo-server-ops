package javaenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Run("設定値をそのまま使う", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "jdk-21.0.4")
		makeJava(t, home)
		got := Resolve(filepath.Join(home, "bin", "java"), filepath.Join(t.TempDir(), "missing"))
		if got.Kind != UseConfigured || got.Home != home || got.Configured != home {
			t.Fatalf("Resolve() = %#v", got)
		}
	})

	t.Run("同世代をバージョン降順で代替する", func(t *testing.T) {
		root := t.TempDir()
		older := filepath.Join(root, "jdk-21.0.3")
		newer := filepath.Join(root, "jdk-21.0.10")
		makeInstalled(t, older, "21.0.3", "test")
		makeInstalled(t, newer, "21.0.10", "test")
		configured := filepath.Join(t.TempDir(), "java-21-openjdk-amd64")
		got := Resolve(configured, root)
		if got.Kind != UseReplacement || got.Home != newer || got.Configured != configured {
			t.Fatalf("Resolve() = %#v", got)
		}
	})

	t.Run("代替せず起動を続ける", func(t *testing.T) {
		root := t.TempDir()
		makeInstalled(t, filepath.Join(root, "jdk-17.0.12"), "17.0.12", "test")
		configured := filepath.Join(t.TempDir(), "jdk-21.0.4")
		got := Resolve(configured, root)
		if got.Kind != DoNotInject || got.Home != "" || got.Configured != configured {
			t.Fatalf("Resolve() = %#v", got)
		}
	})

	t.Run("走査エラーでも起動を続ける", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(root, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		got := Resolve("/missing/jdk-21.0.4", root)
		if got.Kind != DoNotInject || got.Home != "" {
			t.Fatalf("Resolve() = %#v", got)
		}
	})
}

func TestConfiguredMajor(t *testing.T) {
	for _, path := range []string{"/usr/lib/jvm/java-21-openjdk-amd64", "/opt/jdk-21.0.4", "/opt/jdk-21.0.4/bin/java"} {
		if got, ok := configuredMajor(path); !ok || got != 21 {
			t.Errorf("configuredMajor(%q) = %d, %v", path, got, ok)
		}
	}
}
