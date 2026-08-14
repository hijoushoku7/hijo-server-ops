package javaenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateHomeNormalizesBinJavaWithoutResolvingSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "jdk-real")
	makeJava(t, real)
	link := filepath.Join(dir, "java-21")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	got, err := ValidateHome(filepath.Join(link, "bin", "java"))
	if err != nil {
		t.Fatal(err)
	}
	if got != link {
		t.Fatalf("ValidateHome() = %q, want symlink %q", got, link)
	}
}

func TestValidateHomeRejectsRelativeAndNonExecutable(t *testing.T) {
	if _, err := ValidateHome("java-21"); err == nil {
		t.Fatal("相対パスを受理した")
	}
	dir := t.TempDir()
	java := filepath.Join(dir, "bin", "java")
	if err := os.MkdirAll(filepath.Dir(java), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(java, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateHome(dir); err == nil {
		t.Fatal("実行不能な java を受理した")
	}
}

func TestPrependPath(t *testing.T) {
	home := "/opt/jdk"
	if got := PrependPath("", home); got != "/opt/jdk/bin" {
		t.Fatalf("PATH無し = %q", got)
	}
	if got := PrependPath("/bin:/opt/jdk/bin:/usr/bin:/opt/jdk/bin", home); got != "/opt/jdk/bin:/bin:/usr/bin" {
		t.Fatalf("重複PATH = %q", got)
	}
}

func makeJava(t *testing.T, home string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", "java"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
}
