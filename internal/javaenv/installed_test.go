package javaenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstalled(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "jdk-21.0.4-long-real-name")
	makeInstalled(t, real, "21.0.4", "Eclipse Adoptium")
	link := filepath.Join(root, "java-21")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(root, "default-java")); err != nil {
		t.Fatal(err)
	}
	java8 := filepath.Join(root, "java-8")
	makeInstalled(t, java8, "1.8.0_422", "Debian")
	makeJava(t, filepath.Join(root, "release-less"))
	if err := os.Mkdir(filepath.Join(root, "broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken", "release"), []byte("JAVA_VERSION=oops\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Installed(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("Installed = %#v", got)
	}
	if got[0].Home != link || got[0].Major != 21 {
		t.Fatalf("symlink representative = %#v", got[0])
	}
	if got[1].Major != 8 {
		t.Fatalf("Java 8 = %#v", got[1])
	}
	for _, installation := range got {
		if filepath.Base(installation.Home) == "default-java" {
			t.Fatal("default-java が候補に含まれた")
		}
	}
}

func TestInstalledPrefersLongerSymlinkToShortRealDirectory(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "jdk-21")
	makeInstalled(t, real, "21.0.4", "Eclipse Adoptium")
	link := filepath.Join(root, "java-21-openjdk")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	got, err := Installed(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Home != link {
		t.Fatalf("Installed = %#v, want symlink %q", got, link)
	}
}

func TestInstalledMissingRoot(t *testing.T) {
	got, err := Installed(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(got) != 0 {
		t.Fatalf("Installed = %#v, %v", got, err)
	}
}

func makeInstalled(t *testing.T, home, version, implementor string) {
	t.Helper()
	makeJava(t, home)
	release := "JAVA_VERSION=\"" + version + "\"\nIMPLEMENTOR=\"" + implementor + "\"\nOS_ARCH=\"x86_64\"\n"
	if err := os.WriteFile(filepath.Join(home, "release"), []byte(release), 0o644); err != nil {
		t.Fatal(err)
	}
}
