package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndRenderJavaKeepSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "jdk-real")
	makeConfigJava(t, real)
	link := filepath.Join(dir, "java-21")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hso.toml")
	writeConfig(t, path, "[server]\ncommand = \"./run.sh\"\njava = \""+filepath.Join(link, "bin", "java")+"\"\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Java != link {
		t.Fatalf("Java = %q, want symlink %q", cfg.Server.Java, link)
	}
	if got := render(cfg); !strings.Contains(got, `java = "`+link+`"`) {
		t.Fatalf("render:\n%s", got)
	}
}

func TestSetJavaPreservesDocument(t *testing.T) {
	dir := t.TempDir()
	java := filepath.Join(dir, "jdk")
	makeConfigJava(t, java)
	path := filepath.Join(dir, "hso.toml")
	original := "# top\r\n[server]\r\ncommand = \"./run.sh#java = fake\"\r\njava = \"/old\" # keep\r\n\r\n[ui.theme]\r\nframe = \"\"\"\r\n[server]\r\njava = fake\r\n\"\"\""
	// 壊れた旧設定からでも局所更新できることを確かめる。
	old := filepath.Join(dir, "old")
	original = strings.Replace(original, "/old", old, 1)
	writeConfig(t, path, original)
	if err := SetJava(path, filepath.Join(java, "bin", "java")); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, `java = "`+old+`" # keep`, `java = "`+java+`" # keep`, 1)
	if string(written) != want {
		t.Fatalf("written:\n%q\nwant:\n%q", written, want)
	}
}

func TestSetJavaAddsAndKeepsTrailingNewlineState(t *testing.T) {
	dir := t.TempDir()
	java := filepath.Join(dir, "jdk")
	makeConfigJava(t, java)
	path := filepath.Join(dir, "hso.toml")
	writeConfig(t, path, "[server]\ncommand = \"./run.sh\"\n\n[ui.theme]\nframe = \"x\"")
	if err := SetJava(path, java); err != nil {
		t.Fatal(err)
	}
	written, _ := os.ReadFile(path)
	if !strings.Contains(string(written), "java = \""+java+"\"\n[ui.theme]") {
		t.Fatalf("written:\n%s", written)
	}
	if strings.HasSuffix(string(written), "\n") {
		t.Fatal("末尾改行が追加された")
	}
}

func TestSetJavaFailureLeavesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	original := "[server]\ncommand = \"./run.sh\"\n"
	writeConfig(t, path, original)
	if err := SetJava(path, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("存在しない JVM を受理した")
	}
	written, _ := os.ReadFile(path)
	if string(written) != original {
		t.Fatalf("written = %q", written)
	}
}

func makeConfigJava(t *testing.T, home string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", "java"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestLoadContinuesWithBrokenJavaAndSetJavaRepairsIt(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "空", value: "", want: ""},
		{name: "形が不正", value: "java-21", want: "java-21"},
		{name: "指す先がない", value: "/missing/java-21", want: "/missing/java-21"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "hso.toml")
			writeConfig(t, path, "[server]\ncommand = \"./run.sh\"\njava = \""+test.value+"\"\n")
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load() が起動を妨げた: %v", err)
			}
			if cfg.Server.Java != test.want {
				t.Fatalf("Java = %q, want %q", cfg.Server.Java, test.want)
			}
			replacement := filepath.Join(dir, "jdk-21")
			makeConfigJava(t, replacement)
			if err := SetJava(path, replacement); err != nil {
				t.Fatalf("SetJava() で修復できない: %v", err)
			}
			repaired, err := Load(path)
			if err != nil || repaired.Server.Java != replacement {
				t.Fatalf("修復後 = %#v, %v", repaired.Server, err)
			}
		})
	}
}

func TestSetJavaReplacesMultilineJavaAndPreservesDocument(t *testing.T) {
	dir := t.TempDir()
	java := filepath.Join(dir, "jdk-21")
	makeConfigJava(t, java)
	path := filepath.Join(dir, "hso.toml")
	original := "# top\n[server]\ncommand = \"./run.sh\"\njava = \"\"\"\n/missing/java-21\n\"\"\" # keep\n\n[ui.theme]\nframe = '''\njava = fake\n'''\n"
	writeConfig(t, path, original)
	if err := SetJava(path, java); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, "java = \"\"\"\n/missing/java-21\n\"\"\" # keep", `java = "`+java+`" # keep`, 1)
	if string(written) != want {
		t.Fatalf("written:\n%s\nwant:\n%s", written, want)
	}
}

func TestLoadJavaUpdateErrorHidesTemporaryPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	temporaryPath := filepath.Join(dir, ".hso-java-temporary")
	writeConfig(t, temporaryPath, "not valid TOML =")
	_, err := loadJavaUpdate(path, temporaryPath)
	if err == nil {
		t.Fatal("不正な更新内容を受理した")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want original path %q", err, path)
	}
	if strings.Contains(err.Error(), temporaryPath) {
		t.Errorf("error leaked temporary path: %q", err)
	}
}

// server = { ... } の形は hso が生成しないので書き換えに対応しない。ただし
// 意味の分かるエラーで断り、元のファイルは触らないこと。
func TestSetJavaRejectsInlineServerTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "server = { command = \"./run.sh\", java = \"/missing/jdk-17\" }\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "jvm")
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", "java"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := SetJava(path, home)
	if err == nil {
		t.Fatal("インラインテーブルを黙って書き換えた")
	}
	if strings.Contains(err.Error(), "already been defined") {
		t.Fatalf("TOML の内部エラーがそのまま出ている: %v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != body {
		t.Fatalf("元のファイルが変わった:\n%s", after)
	}
}
