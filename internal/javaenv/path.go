package javaenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NormalizeHome は JAVA_HOME またはその bin/java を JAVA_HOME の絶対パスへ整える。
// symlink は更新後も同じ名前を保存するため解決しない。
func NormalizeHome(path string) (string, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("JAVA_HOME must be an absolute path: %q", path)
	}
	path = filepath.Clean(path)
	if filepath.Base(path) == "java" && filepath.Base(filepath.Dir(path)) == "bin" {
		path = filepath.Dir(filepath.Dir(path))
	}
	return path, nil
}

// ValidateHome は JAVA_HOME 配下の bin/java が実行可能か検査する。
func ValidateHome(path string) (string, error) {
	home, err := NormalizeHome(path)
	if err != nil {
		return "", err
	}
	java := filepath.Join(home, "bin", "java")
	info, err := os.Stat(java)
	if err != nil {
		return "", fmt.Errorf("check Java executable: %w", err)
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("Java executable is not executable: %s", java)
	}
	return home, nil
}

// PrependPath は JAVA_HOME/bin を PATH の先頭へ一度だけ置く。
func PrependPath(pathValue, javaHome string) string {
	bin := filepath.Join(javaHome, "bin")
	parts := []string{bin}
	for _, part := range filepath.SplitList(pathValue) {
		if part != bin {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}
