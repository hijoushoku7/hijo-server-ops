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
	// LastPlayed は最後に起動したサーバーの登録名。次の hso start で
	// カーソルを合わせるためだけに使う。servers より前に置くのは、
	// TOML では配列テーブルの後ろに書いた素のキーがその表に属して
	// しまうため。
	LastPlayed string   `toml:"last_played"`
	Servers    []Server `toml:"servers"`
}

type Server struct {
	Name   string `toml:"name"`
	Config string `toml:"config"`
}

// Add は重複を検査してサーバーを一覧へ加える。
func (r *Registry) Add(server Server) error {
	if name, found := r.NameForConfig(server.Config); found {
		return msg.DuplicateServerConfig(name, server.Config)
	}
	return r.addWithoutConfigCheck(server)
}

func (r *Registry) addWithoutConfigCheck(server Server) error {
	if err := ValidateName(server.Name); err != nil {
		return err
	}
	if _, found := r.Find(server.Name); found {
		return msg.DuplicateServerName(server.Name)
	}
	r.Servers = append(r.Servers, server)
	return nil
}

// Remove は大文字小文字を区別せず登録名を探して一覧から取り除く。
func (r *Registry) Remove(name string) (Server, bool) {
	for index, server := range r.Servers {
		if strings.EqualFold(server.Name, name) {
			r.Servers = append(r.Servers[:index], r.Servers[index+1:]...)
			return server, true
		}
	}
	return Server{}, false
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
	target, err := resolveConfig(path)
	if err != nil {
		return "", false
	}
	for _, server := range r.Servers {
		candidate, err := resolveConfig(server.Config)
		if err == nil && candidate == target {
			return server.Name, true
		}
	}
	return "", false
}

// resolveConfig は hso.toml のパスを比較できる形にする。symlink を張った
// サーバーディレクトリ（/srv/mc/current -> /srv/mc/1.21 など）越しに同じ
// ファイルを指されても同一と判定できるよう実体まで辿る。
//
// hso.toml がまだ無い（setup 直前）／消えた登録でもディレクトリの symlink は
// 辿れるので、そのときは親だけ解決してファイル名を繋ぎ直す。ここで諦めると
// alias 越しの setup が既存の登録を見落とし、ウィザードを終えてから重複で
// 弾かれる。親も辿れなければ絶対パスのまま比べる。
func resolveConfig(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved, nil
	}
	directory, base := filepath.Split(filepath.Clean(absolute))
	resolvedDirectory, err := filepath.EvalSymlinks(filepath.Clean(directory))
	if err != nil {
		return filepath.Clean(absolute), nil
	}
	return filepath.Join(resolvedDirectory, base), nil
}

func validate(registry Registry) error {
	var validated Registry
	for _, server := range registry.Servers {
		// 既存のパス重複を Load で拒むと delete で修復できないため、
		// ここでは名前に関する不変条件だけを検査する。
		if err := validated.addWithoutConfigCheck(server); err != nil {
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
