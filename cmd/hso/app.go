package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
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
			Name:           serverDisplayName(configPath, cfg, registeredName),
			Version:        version,
			ServerVersion:  cfg.Server.Version,
			PropertiesPath: filepath.Join(cfg.Server.WorkDir, "server.properties"),
		},
	)
	program := tea.NewProgram(model, tea.WithContext(ctx))
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	go func() {
		<-signals
		// hso が先に死ぬと Pdeathsig で supervisor も終了し、ワールドを
		// 保存する前にサーバーが畳まれるため、通常の終了経路へ合流させる。
		// ここで購読を解除しない。解除すると 2 回目の SIGTERM が既定動作に
		// 戻り、停止を待っている最中の hso をその場で殺してしまう。
		program.Quit()
	}()

	// 更新の確認は 1 回だけ。失敗しても黙って諦める。開発ビルドはタグと
	// 比べようがないので確認しない。
	if version != "dev" {
		go func() {
			if latest, err := latestRelease(versionHTTPClient); err == nil && latest.Tag != version {
				program.Send(ui.UpdateAvailableMsg{Version: latest.Tag})
			}
		}()
	}

	controller := newServerController(ctx, cfg, program)
	if err := controller.start(initialGeneration, false); err != nil {
		return err
	}
	go controller.handleActions(actions)

	_, programErr := program.Run()
	cancel()
	// 停止を待つ間に固まったと誤解して ^C を押させないため、alt screen を
	// 畳んだ直後に待機中であることを端末へ残す。すでに止まっているなら
	// 待ちは無いので出さない。
	if controller.currentRuntime() != nil {
		fmt.Fprintln(os.Stdout, msg.ServerStoppingNotice)
	}
	stopErr := controller.shutdown()

	if model.Err() != nil {
		return model.Err()
	}
	if stopErr != nil {
		return stopErr
	}
	if !ui.IsExpectedExit(programErr) {
		return programErr
	}
	// 終了モーダルを出さずに終わる道（^C）があるので、止まったことは端末に
	// 残す。alt screen を畳んだ後なので画面には最後の 1 行として残る。
	// 失敗を返すときは hso: のエラーだけを出す。
	fmt.Fprintln(os.Stdout, msg.ServerStoppedNotice)
	return nil
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
