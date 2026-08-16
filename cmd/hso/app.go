package main

import (
	"context"
	"path/filepath"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/ui"
)

const (
	actionQueueSize   = 4
	initialGeneration = 1
)

func runTUI(configPath string, cfg config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actions := make(chan ui.Action, actionQueueSize)
	save := func(settings ui.Settings) error {
		return saveSettings(configPath, cfg, settings)
	}
	model := ui.New(
		actions,
		save,
		initialGeneration,
		settingsFrom(cfg),
		ui.ServerInfo{
			Name:    serverDisplayName(configPath, cfg, registeredName),
			Version: version,
		},
	)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	controller := newServerController(ctx, cfg, program)
	if err := controller.start(initialGeneration, false); err != nil {
		return err
	}
	go controller.handleActions(actions)

	_, programErr := program.Run()
	cancel()
	stopErr := controller.shutdown()

	if model.Err() != nil {
		return model.Err()
	}
	if stopErr != nil {
		return stopErr
	}
	if ui.IsExpectedExit(programErr) {
		return nil
	}
	return programErr
}

// displayNameRunes は画面に出す名前の上限。サーバー一覧の登録名と同じ長さに
// 揃える。
const displayNameRunes = 30

func serverDisplayName(
	configPath string,
	cfg config.Config,
	lookup func(string) (string, bool, error),
) string {
	if name, found, err := lookup(configPath); err == nil && found {
		return sanitizeDisplayName(name)
	}
	if cfg.Server.WorkDir == "" {
		return ""
	}
	return sanitizeDisplayName(filepath.Base(cfg.Server.WorkDir))
}

// sanitizeDisplayName は制御文字を落として長さを切る。ディレクトリ名は
// registry.ValidateName を通っておらず、ESC や BEL も入りうる。この名前は
// 端末のウィンドウタイトルとして OSC シーケンスの中へそのまま置かれるので、
// 落とさないとファイル名でシーケンスを閉じて端末を操作できてしまう。
// 一覧の登録名も、手で書き換えられた設定ファイルから来るため同じ扱いにする。
func sanitizeDisplayName(name string) string {
	cleaned := strings.TrimSpace(strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, name))

	runes := []rune(cleaned)
	if len(runes) > displayNameRunes {
		return string(runes[:displayNameRunes])
	}
	return cleaned
}

func settingsFrom(cfg config.Config) ui.Settings {
	settings := ui.DefaultSettings()
	if value := cfg.UI.Theme.Background; value != "" {
		settings.BackgroundPreset = value
	}
	if value := cfg.UI.Theme.Frame; value != "" {
		settings.FramePreset = value
	}
	if value := cfg.UI.Theme.Graph; value != "" {
		settings.GraphPreset = value
	}
	if value := cfg.UI.Theme.Meter; value != "" {
		settings.MeterPreset = value
	}
	if value := cfg.UI.Theme.Title; value != "" {
		settings.TitlePreset = value
	}
	if value := cfg.UI.Theme.Selection; value != "" {
		settings.SelectionPreset = value
	}
	if value := cfg.UI.Theme.Log; value != "" {
		settings.LogPreset = value
	}
	settings.AutoRestart = cfg.Server.AutoRestart
	settings.TimeOffsetMinutes = cfg.UI.Time.OffsetMinutes
	return settings
}

func saveSettings(configPath string, cfg config.Config, settings ui.Settings) error {
	cfg.UI.Theme = config.Theme{
		Background: settings.BackgroundPreset,
		Frame:      settings.FramePreset,
		Graph:      settings.GraphPreset,
		Meter:      settings.MeterPreset,
		Title:      settings.TitlePreset,
		Selection:  settings.SelectionPreset,
		Log:        settings.LogPreset,
	}
	cfg.Server.AutoRestart = settings.AutoRestart
	cfg.UI.Time.OffsetMinutes = settings.TimeOffsetMinutes
	return config.Save(configPath, cfg)
}
