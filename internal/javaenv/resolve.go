package javaenv

import (
	"path/filepath"
	"regexp"
	"strconv"
)

// ResolutionKind は設定した Java を起動時にどう扱うかを表す。
type ResolutionKind int

const (
	UseConfigured ResolutionKind = iota
	UseReplacement
	DoNotInject
)

// Resolution は設定値と、実際に PATH へ注入する JAVA_HOME を表す。
// Home が空なら Java の PATH を注入せず、親プロセスの PATH を使う。
type Resolution struct {
	Kind       ResolutionKind
	Configured string
	Home       string
}

var javaHomeMajorPattern = regexp.MustCompile(`(?i)(java|jdk)[-_]?([0-9]+)`)

// Resolve は設定値を検査し、使えなければ root を再走査して同世代の JVM を探す。
// 設定や走査に問題があってもエラーにはせず、Java を注入しない結果を返す。
func Resolve(configured, root string) Resolution {
	result := Resolution{Kind: DoNotInject, Configured: configured}
	if home, err := ValidateHome(configured); err == nil {
		result.Kind, result.Configured, result.Home = UseConfigured, home, home
		return result
	}
	major, ok := configuredMajor(configured)
	if !ok {
		return result
	}
	installations, err := Installed(root)
	if err != nil {
		return result
	}
	for _, installation := range installations {
		if installation.Major == major {
			result.Kind, result.Home = UseReplacement, installation.Home
			return result
		}
	}
	return result
}

func configuredMajor(path string) (int, bool) {
	if home, err := NormalizeHome(path); err == nil {
		path = home
	}
	match := javaHomeMajorPattern.FindStringSubmatch(filepath.Base(filepath.Clean(path)))
	if len(match) != 3 {
		return 0, false
	}
	major, err := strconv.Atoi(match[2])
	return major, err == nil && major > 0
}
