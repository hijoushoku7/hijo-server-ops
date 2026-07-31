package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSettingsModalOpensWithGAndChangesFrameColor(t *testing.T) {
	// スタイルはパッケージ変数なので、後続のテストへ持ち越さない。
	t.Cleanup(func() { applyTheme(DefaultSettings()) })
	var saved []Settings
	save := func(settings Settings) error {
		saved = append(saved, settings)
		return nil
	}
	model := New(make(chan Action, 1), save, 0, DefaultSettings())
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

	// 先頭の項目（枠のプリセット）を 1 つ進める。
	before := model.settings.FramePreset
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if model.settings.FramePreset == before {
		t.Fatalf("frame preset unchanged: %q", model.settings.FramePreset)
	}
	// 選んだプリセットは即座に描画へ反映する。
	if plainFrame.style.GetForeground() !=
		color(framePresets[model.settings.FramePreset].plain).GetForeground() {
		t.Fatalf("theme was not applied: %#v", plainFrame.style)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.settingsOpen {
		t.Fatal("settings did not close")
	}
	if len(saved) != 1 || saved[0].FramePreset != model.settings.FramePreset {
		t.Fatalf("saved = %#v", saved)
	}
}

// 保存はサーバー操作のキューを通さないので、操作待ちでも取りこぼさない。
func TestSettingsAreSavedEveryTimeTheModalCloses(t *testing.T) {
	t.Cleanup(func() { applyTheme(DefaultSettings()) })
	var saved []Settings
	save := func(settings Settings) error {
		saved = append(saved, settings)
		return nil
	}
	// 容量 0 の相当として、詰まったままのキューを渡す。
	actions := make(chan Action, 1)
	actions <- Action{Kind: ActionRestart}
	model := New(actions, save, 0, DefaultSettings())
	_, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model.busy = true
	// Console の入力欄から離れないと G は文字入力になる。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	for i := 0; i < 3; i++ {
		_, _ = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
		_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	}
	if len(saved) != 3 {
		t.Fatalf("saved = %#v", saved)
	}
	if saved[2].FramePreset != model.settings.FramePreset {
		t.Fatalf("last save = %q", saved[2].FramePreset)
	}
}

// 保存に失敗したら気付けるよう、ステータス行に理由を出す。
func TestSettingsSaveFailureShowsStatus(t *testing.T) {
	t.Cleanup(func() { applyTheme(DefaultSettings()) })
	save := func(Settings) error { return errors.New("書けません") }
	model := New(make(chan Action, 1), save, 0, DefaultSettings())
	_, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	_, _ = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !strings.Contains(model.status, "書けません") {
		t.Fatalf("status = %q", model.status)
	}
}

func TestSettingItemsWrapAroundAndKeepUnknownValues(t *testing.T) {
	item := settingItems[0]
	settings := Settings{FramePreset: item.options[0].value}

	item.shift(&settings, -1)
	if settings.FramePreset != item.options[len(item.options)-1].value {
		t.Fatalf("wrap = %q", settings.FramePreset)
	}

	// 設定ファイルに知らないプリセット名が書かれていても捨てず表示はする。
	settings.FramePreset = "unknown"
	if label := item.valueLabel(settings); label != "unknown" {
		t.Fatalf("label = %q", label)
	}
}

func TestApplyThemeFallsBackToDefaultsForUnknownPresets(t *testing.T) {
	t.Cleanup(func() { applyTheme(DefaultSettings()) })
	applyTheme(Settings{})

	defaults := DefaultSettings()
	want := framePresets[defaults.FramePreset].plain
	if plainFrame.style.GetForeground() != color(want).GetForeground() {
		t.Fatalf("frame = %#v, want %s", plainFrame.style, want)
	}
	if meterOverStyle.GetForeground() !=
		color(meterPresets[defaults.MeterPreset].over).GetForeground() {
		t.Fatalf("meter = %#v", meterOverStyle)
	}
}
