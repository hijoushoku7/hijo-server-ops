package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

// Settings は設定モーダルで変更できる値。項目を増やすときは、ここへ
// フィールドを 1 つ足して settingItems に 1 エントリ足すだけでよい。
// モーダル側は項目の中身を知らない。
type Settings struct {
	BackgroundPreset  string
	FramePreset       string
	GraphPreset       string
	MeterPreset       string
	TitlePreset       string
	SelectionPreset   string
	LogPreset         string
	AutoRestart       bool
	TimeOffsetMinutes int
}

func DefaultSettings() Settings {
	return Settings{
		BackgroundPreset: "terminal",
		FramePreset:      "dracula",
		GraphPreset:      "dracula",
		MeterPreset:      "signal",
		TitlePreset:      "cyan",
		SelectionPreset:  "amber",
		LogPreset:        "dracula",
	}
}

type settingOption struct {
	label string
	value string
}

// settingItem は設定 1 項目。値の出し入れだけを持たせることで、モーダルは
// ↑↓ で項目、←→ で値を動かす操作だけを知っていればよくなる。
//
// section が空でない項目は、その手前に見出しを 1 行挟む。見出しはカーソルの
// 止まらない飾りなので、項目の並びとカーソル位置の対応は変わらない。
type settingItem struct {
	section string
	label   string
	options []settingOption
	get     func(Settings) string
	set     func(*Settings, string)
	// options が空の項目では、get は現在値の表示だけに使う。Enter で
	// open を呼び、設定モーダルを残したまま追加のモーダルを開く。
	open func(*Model)
}

var settingItems = []settingItem{
	{
		section: msg.SectionPreferences,
		label:   msg.LabelTheme,
		options: []settingOption{
			{label: msg.OptDefault, value: "dracula"},
			{label: msg.OptMono, value: "mono"},
			{label: msg.OptSafe, value: "safe"},
			{label: msg.OptSunset, value: "sunset"},
			{label: msg.OptSakura, value: "sakura"},
			{label: msg.OptNord, value: "nord"},
		},
		get: func(settings Settings) string {
			if name := themeName(settings); name != "" {
				return name
			}
			return msg.OptCustom
		},
		set: func(settings *Settings, value string) {
			if bundle, ok := themeBundles[value]; ok {
				bundle.apply(settings)
			}
		},
	},
	{
		label: msg.LabelFrame,
		options: []settingOption{
			{label: msg.OptDefault, value: "dracula"},
			{label: msg.OptMono, value: "mono"},
			{label: msg.OptNeon, value: "neon"},
			{label: msg.OptOcean, value: "ocean"},
			{label: msg.OptForest, value: "forest"},
			{label: msg.OptSunset, value: "sunset"},
			{label: msg.OptSakura, value: "sakura"},
			{label: msg.OptNord, value: "nord"},
		},
		get: func(settings Settings) string { return settings.FramePreset },
		set: func(settings *Settings, value string) {
			settings.FramePreset = value
		},
	},
	{
		label: msg.LabelGraphLine,
		options: []settingOption{
			{label: msg.OptDefault, value: "dracula"},
			{label: msg.OptMono, value: "mono"},
			{label: msg.OptWarm, value: "warm"},
			{label: msg.OptCool, value: "cool"},
			{label: msg.OptSafe, value: "safe"},
			{label: msg.OptSunset, value: "sunset"},
			{label: msg.OptSakura, value: "sakura"},
			{label: msg.OptNord, value: "nord"},
		},
		get: func(settings Settings) string { return settings.GraphPreset },
		set: func(settings *Settings, value string) {
			settings.GraphPreset = value
		},
	},
	{
		label: msg.LabelMeterBar,
		options: []settingOption{
			{label: msg.OptSignal, value: "signal"},
			{label: msg.OptMono, value: "mono"},
			{label: msg.OptSafe, value: "safe"},
			{label: msg.OptFlat, value: "flat"},
			{label: msg.OptSunset, value: "sunset"},
			{label: msg.OptSakura, value: "sakura"},
			{label: msg.OptNord, value: "nord"},
		},
		get: func(settings Settings) string { return settings.MeterPreset },
		set: func(settings *Settings, value string) {
			settings.MeterPreset = value
		},
	},
	{
		label: msg.LabelTitle,
		options: []settingOption{
			{label: msg.OptCyan, value: "cyan"},
			{label: msg.OptWhite, value: "white"},
			{label: msg.OptAmber, value: "amber"},
			{label: msg.OptViolet, value: "violet"},
			{label: msg.OptQuiet, value: "quiet"},
			{label: msg.OptSunset, value: "sunset"},
			{label: msg.OptSakura, value: "sakura"},
			{label: msg.OptNord, value: "nord"},
		},
		get: func(settings Settings) string { return settings.TitlePreset },
		set: func(settings *Settings, value string) {
			settings.TitlePreset = value
		},
	},
	{
		label: msg.LabelSelection,
		options: []settingOption{
			{label: msg.OptAmber, value: "amber"},
			{label: msg.OptCyan, value: "cyan"},
			{label: msg.OptViolet, value: "violet"},
			{label: msg.OptMono, value: "mono"},
			{label: msg.OptSunset, value: "sunset"},
			{label: msg.OptSakura, value: "sakura"},
			{label: msg.OptNord, value: "nord"},
		},
		get: func(settings Settings) string { return settings.SelectionPreset },
		set: func(settings *Settings, value string) {
			settings.SelectionPreset = value
		},
	},
	{
		label: msg.LabelLog,
		options: []settingOption{
			{label: msg.OptDefault, value: "dracula"},
			{label: msg.OptMono, value: "mono"},
			{label: msg.OptWarm, value: "warm"},
			{label: msg.OptCool, value: "cool"},
			{label: msg.OptSafe, value: "safe"},
			{label: msg.OptSunset, value: "sunset"},
			{label: msg.OptSakura, value: "sakura"},
			{label: msg.OptNord, value: "nord"},
		},
		get: func(settings Settings) string { return settings.LogPreset },
		set: func(settings *Settings, value string) {
			settings.LogPreset = value
		},
	},
	{
		label: msg.LabelBackground,
		options: []settingOption{
			{label: msg.OptTerminal, value: "terminal"},
			{label: msg.OptDark, value: "dark"},
			{label: msg.OptNight, value: "night"},
			{label: msg.OptDeep, value: "deep"},
			{label: msg.OptCharcoal, value: "charcoal"},
		},
		get: func(settings Settings) string { return settings.BackgroundPreset },
		set: func(settings *Settings, value string) {
			settings.BackgroundPreset = value
		},
	},
	{
		section: msg.SectionAdvanced,
		label:   msg.LabelAutoRestart,
		// 値は文字列で持たせる。項目 1 つのために get/set を型で分けると、
		// モーダルが項目の中身を知らずに済む今の形が崩れる。
		options: []settingOption{
			{label: msg.OptOff, value: settingOff},
			{label: msg.OptOn, value: settingOn},
		},
		get: func(settings Settings) string {
			if settings.AutoRestart {
				return settingOn
			}
			return settingOff
		},
		set: func(settings *Settings, value string) {
			settings.AutoRestart = value == settingOn
		},
	},
	{
		label: msg.LabelTimezone,
		get: func(settings Settings) string {
			if settings.TimeOffsetMinutes == 0 {
				return msg.OptSystemTime
			}
			return formatTimeOffset(settings.TimeOffsetMinutes)
		},
		open: func(model *Model) { model.openTimeModal() },
	},
}

const (
	settingOn  = "on"
	settingOff = "off"
)

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

func (model *Model) handleSettingsKey(key tea.Key) (tea.Model, tea.Cmd) {
	item := settingItems[model.settingCursor]
	if (key.Code == tea.KeyEnter || key.Code == tea.KeyKpEnter) && item.open != nil {
		item.open(model)
		return model, nil
	}
	switch key.Code {
	case tea.KeyEscape, tea.KeyEnter, tea.KeyKpEnter:
		model.settingsOpen = false
		model.saveSettings()
	case tea.KeyUp:
		model.settingCursor = max(0, model.settingCursor-1)
	case tea.KeyDown:
		model.settingCursor = min(len(settingItems)-1, model.settingCursor+1)
	case tea.KeyLeft:
		if item.open == nil {
			item.shift(&model.settings, -1)
			applyTheme(model.settings)
		}
	case tea.KeyRight:
		if item.open == nil {
			item.shift(&model.settings, 1)
			applyTheme(model.settings)
		}
	}
	return model, nil
}

// saveSettings は設定ファイルへの書き戻しをその場で済ませる。キュー越しに
// 頼むと、サーバー操作で詰まっている間に要求を落としたり、直後に終了して
// 書かれないまま終わったりする。設定ファイル 1 枚の書き込みなので、画面を
// 止めてでも確実に終わらせる。
func (model *Model) saveSettings() {
	if model.save == nil {
		return
	}
	if err := model.save(model.settings); err != nil {
		model.status = msg.SaveSettingsFailed(err)
	}
}

func (model *Model) settingsModal() (string, int, int) {
	labelWidth := 0
	valueWidth := 0
	sectionWidth := 0
	actionWidth := 0
	for _, item := range settingItems {
		labelWidth = max(labelWidth, stringWidth(item.label))
		valueWidth = max(valueWidth, stringWidth(item.valueLabel(model.settings)))
		sectionWidth = max(sectionWidth, stringWidth(item.section))
		if item.open != nil {
			actionWidth = stringWidth(msg.TimeSettingButton) + 4
		}
	}
	// " ラベル  ‹ 値 › " の飾りと枠の 2 列を足した幅。見出しが長ければそちらに
	// 合わせる。
	width := max(labelWidth+valueWidth+actionWidth+10, sectionWidth+4)
	width = min(width, model.layout.width)

	lines := make([]string, 0, len(settingItems)+2)
	for index, item := range settingItems {
		if item.section != "" {
			// 先頭の見出し以外は、区切りとして 1 行空ける。
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines,
				titleStyle.Render(fitLine(" "+item.section, width-2)))
		}
		value := item.valueLabel(model.settings)
		line := " " + fitLine(item.label, labelWidth) + "  ‹ " +
			strings.Repeat(" ", max(0, valueWidth-stringWidth(value))) +
			value + " ›"
		if item.open != nil {
			button := "[" + msg.TimeSettingButton + "]"
			line += strings.Repeat(" ", max(2, width-2-stringWidth(line)-stringWidth(button))) +
				button
		}
		line = fitLine(line, width-2)
		if index == model.settingCursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	height := len(lines) + 2

	x := max(0, (model.layout.width-width)/2)
	y := max(0, (model.layout.height-height)/2)
	box := renderPanel("Settings", lines, width, height, false, modalFrame)
	return box, x, y
}
