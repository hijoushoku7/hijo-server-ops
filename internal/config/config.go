package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

type Config struct {
	Server Server `toml:"server"`
	UI     UI     `toml:"ui"`

	// rawWorkDir は設定ファイルに書かれていたままの workdir。Server.WorkDir
	// は Load で絶対パスにするので、書き戻すときの元の表現をここに残す。
	rawWorkDir string
}

type Server struct {
	Command string `toml:"command"`
	WorkDir string `toml:"workdir"`
	// AutoRestart は異常終了したサーバーを hso が自動で立て直すか。配色では
	// なく挙動なので [ui.theme] ではなくここに置く。既定は無効。
	AutoRestart bool `toml:"auto_restart"`
}

type UI struct {
	Panes []string `toml:"panes"`
	Theme Theme    `toml:"theme"`
	Time  Time     `toml:"time"`
}

type Time struct {
	OffsetMinutes int `toml:"offset_minutes"`
}

// Theme は設定モーダルで選んだ配色プリセットの名前。空なら既定を使う。
type Theme struct {
	Frame     string `toml:"frame"`
	Graph     string `toml:"graph"`
	Meter     string `toml:"meter"`
	Title     string `toml:"title"`
	Selection string `toml:"selection"`
	Log       string `toml:"log"`
}

func Load(path string) (Config, error) {
	var cfg Config

	metadata, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, msg.ReadConfigFailed(err, path)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return cfg, msg.UnknownConfigKeys(joinKeys(undecoded), path)
	}
	cfg.UI.Time.OffsetMinutes = normalizeTimeOffset(cfg.UI.Time.OffsetMinutes)

	cfg.Server.Command = strings.TrimSpace(cfg.Server.Command)
	if cfg.Server.Command == "" {
		return cfg, msg.CommandRequired()
	}

	configPath, err := filepath.Abs(path)
	if err != nil {
		return cfg, msg.ConfigAbsPathFailed(err)
	}
	configDir := filepath.Dir(configPath)

	cfg.rawWorkDir = cfg.Server.WorkDir
	if cfg.Server.WorkDir == "" {
		cfg.Server.WorkDir = configDir
	} else if !filepath.IsAbs(cfg.Server.WorkDir) {
		cfg.Server.WorkDir = filepath.Join(configDir, cfg.Server.WorkDir)
	}
	cfg.Server.WorkDir = filepath.Clean(cfg.Server.WorkDir)

	info, err := os.Stat(cfg.Server.WorkDir)
	if err != nil {
		return cfg, msg.WorkDirCheckFailed(err)
	}
	if !info.IsDir() {
		return cfg, msg.WorkDirNotDirectory(cfg.Server.WorkDir)
	}

	return cfg, nil
}

// Save は設定ファイルを書き直す。設定モーダルで変えた値を次の起動へ
// 残すため。書き出すのは hso が解釈する項目だけなので、ユーザーが書いた
// コメントは残らない。書き込みは一時ファイル経由にして、途中で失敗しても
// 元の設定ファイルを壊さないようにする。
func Save(path string, cfg Config) error {
	// 設定ファイルに書かれていた workdir は、その表現のまま書き戻す。
	// Load が絶対化した値を書くと、相対指定がディレクトリごとの移動に
	// 追従しなくなる。書かれていなかったなら、Load が既定値として入れた
	// 設定ファイルと同じ場所を省いて、書いていない状態を保つ。
	workDir := cfg.rawWorkDir
	if workDir == "" {
		if directory, err := filepath.Abs(filepath.Dir(path)); err != nil ||
			cfg.Server.WorkDir != directory {
			workDir = cfg.Server.WorkDir
		}
	}
	cfg.Server.WorkDir = workDir

	// 一時ファイルは新規作成なので、rename しても元の権限が残るよう
	// 既存ファイルの mode に合わせる。
	permission := permissionOf(path)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(render(cfg)), permission); err != nil {
		return msg.WriteConfigFailed(err)
	}
	if err := os.Chmod(temporary, permission); err != nil {
		os.Remove(temporary)
		return msg.ConfigPermissionFailed(err)
	}
	if err := os.Rename(temporary, path); err != nil {
		os.Remove(temporary)
		return msg.ReplaceConfigFailed(err)
	}
	return nil
}

// permissionOf は既存の設定ファイルの権限を返す。まだ無ければセットアップ
// が作るときと同じ 0644 にする。
func permissionOf(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return 0o644
	}
	return info.Mode().Perm()
}

func render(cfg Config) string {
	var out strings.Builder
	out.WriteString("[server]\n")
	out.WriteString("command = " + quote(cfg.Server.Command) + "\n")
	if cfg.Server.WorkDir != "" {
		out.WriteString("workdir = " + quote(cfg.Server.WorkDir) + "\n")
	}
	if cfg.Server.AutoRestart {
		out.WriteString("auto_restart = true\n")
	}

	if len(cfg.UI.Panes) > 0 {
		names := make([]string, 0, len(cfg.UI.Panes))
		for _, pane := range cfg.UI.Panes {
			names = append(names, quote(pane))
		}
		out.WriteString("\n[ui]\n")
		out.WriteString("panes = [" + strings.Join(names, ", ") + "]\n")
	}

	if cfg.UI.Theme != (Theme{}) {
		out.WriteString("\n[ui.theme]\n")
		for _, preset := range [][2]string{
			{"frame", cfg.UI.Theme.Frame},
			{"graph", cfg.UI.Theme.Graph},
			{"meter", cfg.UI.Theme.Meter},
			{"title", cfg.UI.Theme.Title},
			{"selection", cfg.UI.Theme.Selection},
			{"log", cfg.UI.Theme.Log},
		} {
			if preset[1] != "" {
				out.WriteString(preset[0] + " = " + quote(preset[1]) + "\n")
			}
		}
	}

	if cfg.UI.Time.OffsetMinutes != 0 {
		out.WriteString("\n[ui.time]\n")
		out.WriteString("offset_minutes = " +
			strconv.Itoa(cfg.UI.Time.OffsetMinutes) + "\n")
	}
	return out.String()
}

// normalizeTimeOffset は手書きの値を表示で扱える 30 分刻みへ寄せる。
// -720 は有効範囲 (-720, 720] の外なので、下限側の -690 にクランプする。
// 丸める前に範囲へ収めるのは、int の端に近い値で足し算が回り込んで
// 反対側へ振れるのを避けるため。
func normalizeTimeOffset(minutes int) int {
	minutes = max(-690, min(720, minutes))
	if minutes >= 0 {
		return ((minutes + 15) / 30) * 30
	}
	return -(((-minutes + 15) / 30) * 30)
}

// quote は TOML の基本文字列にする。次の起動で読めない設定ファイルを
// 作らないよう、制御文字も含めてエスケープする。
func quote(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return `"` + replacer.Replace(value) + `"`
}

func joinKeys(keys []toml.Key) string {
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, key.String())
	}
	return strings.Join(names, ", ")
}
