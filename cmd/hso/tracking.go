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
	return pidfile.Create(name)
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
