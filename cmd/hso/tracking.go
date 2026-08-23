package main

import (
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

// trackRegisteredServer は TUI が開く hso.toml の登録名を引き、見つかった
// ときだけ pidfile を作る。一覧にない設定には状態の読み手がいないため作らない。
func trackRegisteredServer(configPath string) (*pidfile.File, error) {
	name, found, err := registeredName(configPath)
	if err != nil || !found {
		return nil, err
	}
	file, err := pidfile.Create(name)
	if err != nil {
		return nil, err
	}
	// 記録は pidfile を取れてから。二重起動で弾かれた呼び出しは TUI を開かない
	// ので、最後に起動したサーバーとして数えない。
	recordLastPlayed(name)
	return file, nil
}

// recordLastPlayed は起動したサーバーを覚えて、次の hso start でカーソルを
// 合わせられるようにする。一覧に書けなくても起動は止めない。
func recordLastPlayed(name string) {
	path, err := registry.Path()
	if err != nil {
		return
	}
	_ = registry.Update(path, func(servers *registry.Registry) error {
		if servers.LastPlayed == name {
			return nil
		}
		servers.LastPlayed = name
		return nil
	})
}

func registeredName(configPath string) (string, bool, error) {
	path, err := registry.Path()
	if err != nil {
		return "", false, err
	}
	servers, err := registry.Load(path)
	if err != nil {
		return "", false, err
	}
	name, found := servers.NameForConfig(configPath)
	return name, found, nil
}
