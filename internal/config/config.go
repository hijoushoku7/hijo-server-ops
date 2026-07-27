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
}

func Load(path string) (Config, error) {
	var cfg Config

	metadata, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("設定ファイルを読む: %w", err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("不明な設定項目: %s", joinKeys(undecoded))
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

func joinKeys(keys []toml.Key) string {
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, key.String())
	}
	return strings.Join(names, ", ")
}
