package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if !isTerminal(file) {
		t.Fatal("/dev/null はキャラクタデバイス")
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
