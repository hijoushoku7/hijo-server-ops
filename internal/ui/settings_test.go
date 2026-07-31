package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSettingsModalOpensWithGAndChangesFrameColor(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, 0, DefaultSettings())
	_, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	// Console フォーカス中の g は文字入力。
	_, _ = model.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if model.settingsOpen || string(model.input) != "g" {
		t.Fatalf("settings opened while typing: %q", model.input)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, _ = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !model.settingsOpen {
		t.Fatal("settings did not open")
	}
	if !strings.Contains(stripANSI(model.View().Content), "Settings") {
		t.Fatal("view does not contain the settings modal")
	}

	// 先頭の項目（枠の通常色）を 1 つ進める。
	before := model.settings.FrameColor
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if model.settings.FrameColor == before {
		t.Fatalf("frame color unchanged: %q", model.settings.FrameColor)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.settingsOpen {
		t.Fatal("settings did not close")
	}
	action := <-actions
	if action.Kind != ActionSaveSettings ||
		action.Settings.FrameColor != model.settings.FrameColor {
		t.Fatalf("action = %#v", action)
	}
}

func TestSettingItemsWrapAroundAndKeepUnknownValues(t *testing.T) {
	item := settingItems[0]
	settings := Settings{FrameColor: colorOptions[0].value}

	item.shift(&settings, -1)
	if settings.FrameColor != colorOptions[len(colorOptions)-1].value {
		t.Fatalf("wrap = %q", settings.FrameColor)
	}

	// 設定ファイルに選択肢外の色が書かれていても捨てず、そのまま表示する。
	settings.FrameColor = "#123456"
	if label := item.valueLabel(settings); label != "#123456" {
		t.Fatalf("label = %q", label)
	}
}
