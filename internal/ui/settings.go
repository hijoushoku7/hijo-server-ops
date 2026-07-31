package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Settings は設定モーダルで変更できる値。項目を増やすときは、ここへ
// フィールドを 1 つ足して settingItems に 1 エントリ足すだけでよい。
// モーダル側は項目の中身を知らない。
type Settings struct {
	FrameColor         string
	SelectedFrameColor string
	FocusedFrameColor  string
}

func DefaultSettings() Settings {
	return Settings{
		FrameColor:         "#777777",
		SelectedFrameColor: "#8BE9FD",
		FocusedFrameColor:  "#F1FA8C",
	}
}

// settingOption は選択肢 1 つ。value がそのまま設定値になる。
type settingOption struct {
	label string
	value string
}

// settingItem は設定 1 項目。値の出し入れだけを持たせることで、モーダルは
// ↑↓ で項目、←→ で値を動かす操作だけを知っていればよくなる。
type settingItem struct {
	label   string
	options []settingOption
	get     func(Settings) string
	set     func(*Settings, string)
}

var colorOptions = []settingOption{
	{label: "グレー", value: "#777777"},
	{label: "白", value: "#F8F8F2"},
	{label: "シアン", value: "#8BE9FD"},
	{label: "緑", value: "#50FA7B"},
	{label: "黄", value: "#F1FA8C"},
	{label: "紫", value: "#BD93F9"},
	{label: "桃", value: "#FF79C6"},
	{label: "橙", value: "#FFB86C"},
	{label: "赤", value: "#FF5555"},
}

var settingItems = []settingItem{
	{
		label:   "枠（通常）",
		options: colorOptions,
		get:     func(settings Settings) string { return settings.FrameColor },
		set: func(settings *Settings, value string) {
			settings.FrameColor = value
		},
	},
	{
		label:   "枠（選択中）",
		options: colorOptions,
		get: func(settings Settings) string {
			return settings.SelectedFrameColor
		},
		set: func(settings *Settings, value string) {
			settings.SelectedFrameColor = value
		},
	},
	{
		label:   "枠（フォーカス中）",
		options: colorOptions,
		get: func(settings Settings) string {
			return settings.FocusedFrameColor
		},
		set: func(settings *Settings, value string) {
			settings.FocusedFrameColor = value
		},
	},
}

// optionIndex は今の値が選択肢の何番目か。設定ファイルに選択肢外の値が
// 書かれていた場合は -1 を返し、値そのものを表示に使う。
func (item settingItem) optionIndex(settings Settings) int {
	current := item.get(settings)
	for index, option := range item.options {
		if option.value == current {
			return index
		}
	}
	return -1
}

func (item settingItem) valueLabel(settings Settings) string {
	if index := item.optionIndex(settings); index >= 0 {
		return item.options[index].label
	}
	return item.get(settings)
}

// shift は選択肢を step だけ動かす。端は反対側へ回り込む。
func (item settingItem) shift(settings *Settings, step int) {
	if len(item.options) == 0 {
		return
	}
	index := item.optionIndex(*settings)
	if index < 0 {
		index = 0
	} else {
		index = (index + step + len(item.options)) % len(item.options)
	}
	item.set(settings, item.options[index].value)
}

// handleSettingsKey は設定モーダル表示中。変更はその場で画面に反映し、
// 閉じるときにまとめて保存する。
func (model *Model) handleSettingsKey(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape, tea.KeyEnter, tea.KeyKpEnter:
		model.settingsOpen = false
		model.saveSettings()
	case tea.KeyUp:
		model.settingCursor = max(0, model.settingCursor-1)
	case tea.KeyDown:
		model.settingCursor = min(len(settingItems)-1, model.settingCursor+1)
	case tea.KeyLeft:
		settingItems[model.settingCursor].shift(&model.settings, -1)
	case tea.KeyRight:
		settingItems[model.settingCursor].shift(&model.settings, 1)
	}
	return model, nil
}

// saveSettings は保存をアプリ層へ渡す。サーバー操作ではないので busy には
// せず、キューが詰まっていれば諦める（次に閉じたときに再送される）。
func (model *Model) saveSettings() {
	if model.actions == nil {
		return
	}
	select {
	case model.actions <- Action{
		Kind:     ActionSaveSettings,
		Settings: model.settings,
	}:
	default:
	}
}

// settingsModal は設定一覧を画面中央に重ねて出す。
func (model *Model) settingsModal() (string, int, int) {
	labelWidth := 0
	valueWidth := 0
	for _, item := range settingItems {
		labelWidth = max(labelWidth, stringWidth(item.label))
		valueWidth = max(valueWidth, stringWidth(item.valueLabel(model.settings)))
	}
	// " ラベル  ‹ 値 › " の飾りと枠の 2 列を足した幅。
	width := labelWidth + valueWidth + 10
	width = min(width, model.layout.width)
	height := len(settingItems) + 2

	lines := make([]string, 0, len(settingItems))
	for index, item := range settingItems {
		value := item.valueLabel(model.settings)
		line := " " + fitLine(item.label, labelWidth) + "  ‹ " +
			strings.Repeat(" ", max(0, valueWidth-stringWidth(value))) +
			value + " ›"
		line = fitLine(line, width-2)
		if index == model.settingCursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	x := max(0, (model.layout.width-width)/2)
	y := max(0, (model.layout.height-height)/2)
	box := renderPanel(
		"Settings",
		lines,
		width,
		height,
		false,
		model.styled(modalFrame, model.settings.FocusedFrameColor),
	)
	return box, x, y
}
