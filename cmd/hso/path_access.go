package main

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

// targetDirectoryAccess はファイルの削除・置換に必要な、親ディレクトリへの
// 書き込み権限と検索権限があるかを access(2) で確認する。
func targetDirectoryAccess(target string) error {
	return unix.Access(filepath.Dir(target), unix.W_OK|unix.X_OK)
}
