package main

import (
	"context"

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
	model := ui.New(actions, save, initialGeneration, settingsFrom(cfg))
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

func settingsFrom(cfg config.Config) ui.Settings {
	settings := ui.DefaultSettings()
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
	return settings
}

func saveSettings(configPath string, cfg config.Config, settings ui.Settings) error {
	cfg.UI.Theme = config.Theme{
		Frame:     settings.FramePreset,
		Graph:     settings.GraphPreset,
		Meter:     settings.MeterPreset,
		Title:     settings.TitlePreset,
		Selection: settings.SelectionPreset,
	}
	return config.Save(configPath, cfg)
}
