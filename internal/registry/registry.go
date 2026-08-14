// Package registry は hso.toml の場所とサーバー名の一覧を管理する。
package registry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

type Registry struct {
	Servers []Server `toml:"servers"`
}

type Server struct {
	Name   string `toml:"name"`
	Config string `toml:"config"`
}

// Add は重複を検査してサーバーを一覧へ加える。
func (r *Registry) Add(server Server) error {
	if err := ValidateName(server.Name); err != nil {
		return err
	}
	if _, found := r.Find(server.Name); found {
		return msg.DuplicateServerName(server.Name)
	}
	r.Servers = append(r.Servers, server)
	return nil
}

// Find は大文字小文字を区別せず登録名を探す。
func (r Registry) Find(name string) (Server, bool) {
	for _, server := range r.Servers {
		if strings.EqualFold(server.Name, name) {
			return server, true
		}
	}
	return Server{}, false
}

// Path はサーバー一覧の設定ファイルの場所を返す。
func Path() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", msg.RegistryPathFailed(err)
	}
	return filepath.Join(configDir, "hso", "config.toml"), nil
}

// ValidateName は一覧に登録できる名前かどうかを返す。名前は pidfile の
// ファイル名になるので、通す文字を列挙してバイト単位で検査する。
func ValidateName(name string) error {
	if len(name) == 0 || len(name) > 30 {
		return msg.InvalidServerName(name)
	}
	for i := 0; i < len(name); i++ {
		character := name[i]
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9':
			continue
		case character == '-' || character == '_' || character == '.':
			if i == 0 {
				return msg.InvalidServerName(name)
			}
		default:
			return msg.InvalidServerName(name)
		}
	}
	return nil
}

// Load はサーバー一覧を読む。まだファイルが無ければ空の一覧を返す。
func Load(path string) (Registry, error) {
	var registry Registry
	metadata, err := toml.DecodeFile(path, &registry)
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return registry, msg.ReadRegistryFailed(err, path)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return registry, msg.UnknownRegistryKeys(joinKeys(undecoded), path)
	}
	if err := validate(registry); err != nil {
		return registry, err
	}
	return registry, nil
}

// Save はサーバー一覧を一時ファイルへ書いてから置き換える。
func Save(path string, registry Registry) error {
	return save(path, registry, os.WriteFile, os.Rename)
}

func save(
	path string,
	registry Registry,
	writeFile func(string, []byte, os.FileMode) error,
	rename func(string, string) error,
) error {
	if err := validate(registry); err != nil {
		return err
	}
	var contents strings.Builder
	if err := toml.NewEncoder(&contents).Encode(registry); err != nil {
		return msg.EncodeRegistryFailed(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return msg.CreateRegistryDirectoryFailed(err)
	}

	permission := permissionOf(path)
	temporary := path + ".tmp"
	if err := writeFile(temporary, []byte(contents.String()), permission); err != nil {
		_ = os.Remove(temporary)
		return msg.WriteRegistryFailed(err)
	}
	if err := os.Chmod(temporary, permission); err != nil {
		_ = os.Remove(temporary)
		return msg.RegistryPermissionFailed(err)
	}
	if err := rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return msg.ReplaceRegistryFailed(err)
	}
	return nil
}

// NameForConfig は指定された hso.toml が一覧にあれば、その登録名を返す。
func (r Registry) NameForConfig(path string) (string, bool) {
	target, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	for _, server := range r.Servers {
		candidate, err := filepath.Abs(server.Config)
		if err == nil && filepath.Clean(candidate) == filepath.Clean(target) {
			return server.Name, true
		}
	}
	return "", false
}

func validate(registry Registry) error {
	var validated Registry
	for _, server := range registry.Servers {
		if err := validated.Add(server); err != nil {
			return err
		}
	}
	return nil
}

func permissionOf(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return 0o644
	}
	return info.Mode().Perm()
}

func joinKeys(keys []toml.Key) string {
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, key.String())
	}
	return strings.Join(names, ", ")
}
