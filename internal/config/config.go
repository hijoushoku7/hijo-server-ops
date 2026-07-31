package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server Server `toml:"server"`
	UI     UI     `toml:"ui"`
}

type Server struct {
	Command string `toml:"command"`
	WorkDir string `toml:"workdir"`
}

type UI struct {
	Panes []string `toml:"panes"`
	Theme Theme    `toml:"theme"`
}

// Theme は設定モーダルで選んだ配色プリセットの名前。空なら既定を使う。
type Theme struct {
	Frame     string `toml:"frame"`
	Graph     string `toml:"graph"`
	Meter     string `toml:"meter"`
	Title     string `toml:"title"`
	Selection string `toml:"selection"`
}

func Load(path string) (Config, error) {
	var cfg Config

	metadata, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("設定ファイルを読む: %w%s", err, reinitialize(path))
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf(
			"不明な設定項目: %s%s",
			joinKeys(undecoded),
			reinitialize(path),
		)
	}

	cfg.Server.Command = strings.TrimSpace(cfg.Server.Command)
	if cfg.Server.Command == "" {
		return cfg, errors.New("server.command は必須です")
	}

	configPath, err := filepath.Abs(path)
	if err != nil {
		return cfg, fmt.Errorf("設定ファイルの絶対パスを求める: %w", err)
	}
	configDir := filepath.Dir(configPath)

	if cfg.Server.WorkDir == "" {
		cfg.Server.WorkDir = configDir
	} else if !filepath.IsAbs(cfg.Server.WorkDir) {
		cfg.Server.WorkDir = filepath.Join(configDir, cfg.Server.WorkDir)
	}
	cfg.Server.WorkDir = filepath.Clean(cfg.Server.WorkDir)

	info, err := os.Stat(cfg.Server.WorkDir)
	if err != nil {
		return cfg, fmt.Errorf("server.workdir を確認する: %w", err)
	}
	if !info.IsDir() {
		return cfg, fmt.Errorf("server.workdir はディレクトリではありません: %s", cfg.Server.WorkDir)
	}

	return cfg, nil
}

// Save は設定ファイルを書き直す。設定モーダルで変えた値を次の起動へ
// 残すため。書き出すのは hso が解釈する項目だけなので、ユーザーが書いた
// コメントは残らない。書き込みは一時ファイル経由にして、途中で失敗しても
// 元の設定ファイルを壊さないようにする。
func Save(path string, cfg Config) error {
	// Load が既定値として入れた workdir をそのまま書き戻すと、書いていな
	// かったユーザーの設定に絶対パスが増える。設定ファイルと同じ場所なら
	// 省いて、ディレクトリごと移動しても壊れないままにする。
	if directory, err := filepath.Abs(filepath.Dir(path)); err == nil &&
		cfg.Server.WorkDir == directory {
		cfg.Server.WorkDir = ""
	}

	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(render(cfg)), 0o644); err != nil {
		return fmt.Errorf("設定ファイルを書く: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		os.Remove(temporary)
		return fmt.Errorf("設定ファイルを置き換える: %w", err)
	}
	return nil
}

func render(cfg Config) string {
	var out strings.Builder
	out.WriteString("[server]\n")
	out.WriteString("command = " + quote(cfg.Server.Command) + "\n")
	if cfg.Server.WorkDir != "" {
		out.WriteString("workdir = " + quote(cfg.Server.WorkDir) + "\n")
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
		} {
			if preset[1] != "" {
				out.WriteString(preset[0] + " = " + quote(preset[1]) + "\n")
			}
		}
	}
	return out.String()
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

// reinitialize は設定ファイルが読めないときの直し方を添える。古い形式の
// 項目が残っているのが主な原因で、消して起動し直せばセットアップから
// 作り直せる、というところまで書かないと何をすればいいか分からない。
func reinitialize(path string) string {
	return fmt.Sprintf(
		"\nhso.toml を初期化してください: %s を削除して hso を起動すると"+
			"セットアップから作り直せます",
		path,
	)
}

func joinKeys(keys []toml.Key) string {
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, key.String())
	}
	return strings.Join(names, ", ")
}
