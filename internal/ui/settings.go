package ui

import (
	"os"
	"os/exec"
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

// 既定は sunset バンドルそのもの。選択画面（hso start・セットアップ）は配色を
// 持てないので固定色を焼き込むしかなく、本体の既定をそこへ寄せて両者を揃える。
// バンドルと一致させておかないと設定モーダルが「カスタム」始まりになり、
// テーマ欄から起動時の見た目へ戻せなくなる。
func DefaultSettings() Settings {
	return Settings{
		BackgroundPreset: "terminal",
		FramePreset:      "sunset",
		GraphPreset:      "sunset",
		MeterPreset:      "sunset",
		TitlePreset:      "sunset",
		SelectionPreset:  "sunset",
		LogPreset:        "sunset",
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
	button  string
	options []settingOption
	get     func(Settings) string
	set     func(*Settings, string)
	// options が空の項目では、get は現在値の表示だけに使う。Enter で
	// open を呼び、設定モーダルを残したまま追加のモーダルを開く。
	open func(*Model) tea.Cmd
}

var settingItems = []settingItem{
	{
		section: msg.SectionPreferences,
		label:   msg.LabelTheme,
		options: []settingOption{
			{label: msg.OptDracula, value: "dracula"},
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
			{label: msg.OptDracula, value: "dracula"},
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
			{label: msg.OptDracula, value: "dracula"},
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
			{label: msg.OptDracula, value: "dracula"},
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
		label:   msg.LabelServerProperties,
		button:  msg.EditButton,
		get:     func(Settings) string { return "server.properties" },
		open: func(model *Model) tea.Cmd {
			parts := resolveEditor()
			command := exec.Command(parts[0], append(parts[1:], model.info.PropertiesPath)...)
			return tea.ExecProcess(command, func(err error) tea.Msg {
				return editorFinishedMsg{err: err}
			})
		},
	},
	{
		label: msg.LabelAutoRestart,
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
		label:  msg.LabelTimezone,
		button: msg.TimeSettingButton,
		get: func(settings Settings) string {
			if settings.TimeOffsetMinutes == 0 {
				return msg.OptSystemTime
			}
			return formatTimeOffset(settings.TimeOffsetMinutes)
		},
		open: func(model *Model) tea.Cmd {
			model.openTimeModal()
			return nil
		},
	},
}

type editorFinishedMsg struct{ err error }

func resolveEditor() []string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		// ponytail: 引用符を含む空白入りパスは解釈しない。
		if parts := strings.Fields(os.Getenv(name)); len(parts) > 0 {
			return parts
		}
	}
	return []string{"vi"}
}

const (
	settingOn  = "on"
	settingOff = "off"
	// 設定モーダルの左右の余白と、広げすぎないための上限幅。
	settingsPad      = 2
	settingsMaxWidth = 60
)

func pad(width int) string { return strings.Repeat(" ", max(0, width)) }

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
		return model, item.open(model)
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
			actionWidth = max(actionWidth, stringWidth(item.button)+4)
		}
	}
	// 1 項目をラベル行と値行の 2 行に割るので、値は行いっぱいの
	// "‹ 値 ›" として置ける。幅はラベル・値・見出しのうち一番広いものに
	// 左右の余白 2 列ずつと枠の 2 列を足す。
	inner := max(labelWidth+actionWidth+2, max(valueWidth+6, sectionWidth))
	// 中身に合わせるだけだと細く貧相になる。画面の 3/5 を目安に広げ、
	// 中身がそれより広ければ中身を優先する。
	width := max(inner+settingsPad*2+2, min(model.layout.width*3/5, settingsMaxWidth))
	width = min(width, model.layout.width)
	contentWidth := width - 2
	field := max(0, contentWidth-settingsPad*2)

	lines := make([]string, 0, len(settingItems)*2+6)
	lines = append(lines, "")
	// 選択中の項目の値行。画面に収まらないときはここを中心に窓を取る。
	cursorLine := 0
	for index, item := range settingItems {
		if item.section != "" {
			// 先頭の見出し以外は、区切りとして 1 行空ける。
			if len(lines) > 1 {
				lines = append(lines, "")
			}
			lines = append(lines,
				titleStyle.Render(fitLine(pad(settingsPad)+item.section, contentWidth)))
		}

		// ラベルは値行の "‹ 値 ›" と中心を揃える。
		label := pad(settingsPad+max(0, (field-stringWidth(item.label))/2)) + item.label
		if item.open != nil {
			button := "[" + item.button + "]"
			label = fitLine(label, max(0, contentWidth-settingsPad-stringWidth(button))) +
				button + pad(settingsPad)
		}
		label = fitLine(label, contentWidth)
		// 選択中はラベル行と値行の 2 行まとめて反転させ、項目 1 つが
		// 選ばれていることを見せる。
		if index == model.settingCursor {
			label = selectedStyle.Render(label)
		}
		lines = append(lines, label)

		value := item.valueLabel(model.settings)
		left := max(0, (field-2-stringWidth(value))/2)
		line := pad(settingsPad) + "‹" + pad(left) + value
		line = fitLine(line, contentWidth-settingsPad-1) + "›" + pad(settingsPad)
		// 値行はラベル行と見分けが付くよう灰色に落とす。選択中だけ反転する。
		if index == model.settingCursor {
			cursorLine = len(lines)
			line = selectedStyle.Render(fitLine(line, contentWidth))
		} else {
			line = dimStyle.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")

	height := min(len(lines)+2, model.layout.height)
	// 端末が低いと全項目は入らない。選択行を中心に窓を切り、カーソルが
	// 常に見えるようにする。
	if contentHeight := height - 2; contentHeight < len(lines) {
		start := min(max(0, cursorLine-contentHeight/2), len(lines)-contentHeight)
		lines = lines[start : start+contentHeight]
	}

	x := max(0, (model.layout.width-width)/2)
	y := max(0, (model.layout.height-height)/2)
	box := renderPanel("Settings", lines, width, height, false, modalFrame)
	return box, x, y
}
