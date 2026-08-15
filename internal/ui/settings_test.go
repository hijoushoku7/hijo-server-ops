package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

func TestSettingsModalOpensWithGAndChangesFrameColor(t *testing.T) {
	// スタイルはパッケージ変数なので、後続のテストへ持ち越さない。
	t.Cleanup(func() { applyTheme(DefaultSettings()) })
	var saved []Settings
	save := func(settings Settings) error {
		saved = append(saved, settings)
		return nil
	}
	model := New(make(chan Action, 1), save, 0, DefaultSettings(), ServerInfo{})
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
	model := New(actions, save, 0, DefaultSettings(), ServerInfo{})
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
	model := New(make(chan Action, 1), save, 0, DefaultSettings(), ServerInfo{})
	_, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	_, _ = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !strings.Contains(model.status, "書けません") {
		t.Fatalf("status = %q", model.status)
	}
}

// 見出しはカーソルの止まらない飾りなので、↓ の回数と項目の対応は
// 見出しを挟んでも変わらない。
func TestSettingsModalShowsSectionHeadings(t *testing.T) {
	t.Cleanup(func() { applyTheme(DefaultSettings()) })
	model := New(make(chan Action, 1), func(Settings) error { return nil },
		0, DefaultSettings(), ServerInfo{})
	_, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, _ = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})

	content := stripANSI(model.View().Content)
	preferences := strings.Index(content, msg.SectionPreferences)
	advanced := strings.Index(content, msg.SectionAdvanced)
	autoRestart := strings.Index(content, msg.LabelAutoRestart)
	timezone := strings.Index(content, msg.LabelTimezone)
	frame := strings.Index(content, msg.LabelFrame)
	switch {
	case preferences < 0 || advanced < 0:
		t.Fatalf("見出しがない:\n%s", content)
	case !(preferences < frame && frame < advanced && advanced < autoRestart && autoRestart < timezone):
		t.Fatalf("見出しの位置が違う:\n%s", content)
	}

	// 見出しの分だけカーソルがずれていないか。
	for range len(settingItems) - 2 {
		_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if !model.settings.AutoRestart {
		t.Fatalf("settings = %#v", model.settings)
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
	if logTimestampStyle.GetForeground() !=
		color(logPresets[defaults.LogPreset].timestamp).GetForeground() {
		t.Fatalf("log timestamp = %#v", logTimestampStyle)
	}
}

// 自動再起動は配色ではないが、同じ項目の仕組みに乗せている。
func TestSettingsToggleAutoRestart(t *testing.T) {
	t.Cleanup(func() { applyTheme(DefaultSettings()) })
	var saved []Settings
	save := func(settings Settings) error {
		saved = append(saved, settings)
		return nil
	}
	model := New(make(chan Action, 1), save, 0, DefaultSettings(), ServerInfo{})
	_, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, _ = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})

	if model.settings.AutoRestart {
		t.Fatal("auto restart is enabled by default")
	}
	for range len(settingItems) - 2 {
		_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if !model.settings.AutoRestart {
		t.Fatalf("settings = %#v", model.settings)
	}
	content := stripANSI(model.View().Content)
	if !strings.Contains(content, msg.LabelAutoRestart) ||
		!strings.Contains(content, msg.OptOn) {
		t.Fatalf("view:\n%s", content)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(saved) != 1 || saved[0].AutoRestart {
		t.Fatalf("saved = %#v", saved)
	}
}
