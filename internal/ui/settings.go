package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Settings は設定モーダルで変更できる値。項目を増やすときは、ここへ
// フィールドを 1 つ足して settingItems に 1 エントリ足すだけでよい。
// モーダル側は項目の中身を知らない。
type Settings struct {
	FramePreset     string
	GraphPreset     string
	MeterPreset     string
	TitlePreset     string
	SelectionPreset string
}

func DefaultSettings() Settings {
	return Settings{
		FramePreset:     "dracula",
		GraphPreset:     "dracula",
		MeterPreset:     "signal",
		TitlePreset:     "cyan",
		SelectionPreset: "amber",
	}
}

// 色は 5 つのプリセットにまとめて選ばせる。1 色ずつ変えられるようにすると、
// 反転表示の前景と背景が同色になったり、メーターの緑＝正常・赤＝危険という
// 意味が壊れたりする組み合わせを作れてしまうため。

// framePalette は枠と、枠まわりの地味な文字（Y 軸ラベル、[restart]、
// メトリクスが取れない理由）の色。
type framePalette struct {
	plain    string
	selected string
	focused  string
	dim      string
}

// graphPalette はグラフの線。overlap は heap と rss が同じセルに乗った色。
type graphPalette struct {
	heap    string
	rss     string
	overlap string
}

// meterPalette はメーターの棒。high は 75%、over は 90% から切り替わる。
type meterPalette struct {
	full  string
	high  string
	over  string
	empty string
}

// selectionPalette は反転表示。選択行とキーバーで前景・背景の組を揃える。
type selectionPalette struct {
	foreground    string
	background    string
	keyForeground string
	keyBackground string
}

var framePresets = map[string]framePalette{
	"dracula": {plain: "#777777", selected: "#8BE9FD", focused: "#F1FA8C", dim: "#777777"},
	"mono":    {plain: "#5A5A5A", selected: "#AAAAAA", focused: "#F8F8F2", dim: "#8A8A8A"},
	"neon":    {plain: "#6272A4", selected: "#FF79C6", focused: "#8BE9FD", dim: "#6272A4"},
	"ocean":   {plain: "#44506B", selected: "#6FB3D2", focused: "#A0E9FF", dim: "#5A6E8C"},
	"forest":  {plain: "#4E6151", selected: "#8FBC8F", focused: "#D7E8A0", dim: "#6B7F6B"},
}

var graphPresets = map[string]graphPalette{
	"dracula": {heap: "#50FA7B", rss: "#8BE9FD", overlap: "#F1FA8C"},
	"mono":    {heap: "#F8F8F2", rss: "#909090", overlap: "#FFFFFF"},
	"warm":    {heap: "#FFB86C", rss: "#FF79C6", overlap: "#F1FA8C"},
	"cool":    {heap: "#8BE9FD", rss: "#BD93F9", overlap: "#F8F8F2"},
	// Okabe-Ito の青と橙。色覚型によらず区別できる組み合わせ。
	"safe": {heap: "#56B4E9", rss: "#E69F00", overlap: "#F0F0F0"},
}

var meterPresets = map[string]meterPalette{
	"signal": {full: "#50FA7B", high: "#FFB86C", over: "#FF5555", empty: "#444444"},
	"mono":   {full: "#8A8A8A", high: "#C8C8C8", over: "#F8F8F2", empty: "#333333"},
	"safe":   {full: "#56B4E9", high: "#F0E442", over: "#D55E00", empty: "#333333"},
	// 段階で色を変えず、長さだけで読ませる。
	"flat": {full: "#8BE9FD", high: "#8BE9FD", over: "#8BE9FD", empty: "#333333"},
}

var titlePresets = map[string]string{
	"cyan":   "#8BE9FD",
	"white":  "#F8F8F2",
	"amber":  "#F1FA8C",
	"violet": "#BD93F9",
	"quiet":  "#999999",
}

var selectionPresets = map[string]selectionPalette{
	"amber": {
		foreground: "#282A36", background: "#F1FA8C",
		keyForeground: "#282A36", keyBackground: "#BBBBBB",
	},
	"cyan": {
		foreground: "#282A36", background: "#8BE9FD",
		keyForeground: "#282A36", keyBackground: "#8FA9B8",
	},
	"violet": {
		foreground: "#F8F8F2", background: "#6272A4",
		keyForeground: "#F8F8F2", keyBackground: "#44475A",
	},
	"mono": {
		foreground: "#282A36", background: "#BBBBBB",
		keyForeground: "#282A36", keyBackground: "#888888",
	},
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

var settingItems = []settingItem{
	{
		label: "枠",
		options: []settingOption{
			{label: "既定", value: "dracula"},
			{label: "モノクロ", value: "mono"},
			{label: "ネオン", value: "neon"},
			{label: "海", value: "ocean"},
			{label: "森", value: "forest"},
		},
		get: func(settings Settings) string { return settings.FramePreset },
		set: func(settings *Settings, value string) {
			settings.FramePreset = value
		},
	},
	{
		label: "グラフの線",
		options: []settingOption{
			{label: "既定", value: "dracula"},
			{label: "モノクロ", value: "mono"},
			{label: "暖色", value: "warm"},
			{label: "寒色", value: "cool"},
			{label: "シンプル", value: "safe"},
		},
		get: func(settings Settings) string { return settings.GraphPreset },
		set: func(settings *Settings, value string) {
			settings.GraphPreset = value
		},
	},
	{
		label: "メーターの棒",
		options: []settingOption{
			{label: "信号", value: "signal"},
			{label: "モノクロ", value: "mono"},
			{label: "シンプル", value: "safe"},
			{label: "単色", value: "flat"},
		},
		get: func(settings Settings) string { return settings.MeterPreset },
		set: func(settings *Settings, value string) {
			settings.MeterPreset = value
		},
	},
	{
		label: "タイトル",
		options: []settingOption{
			{label: "シアン", value: "cyan"},
			{label: "白", value: "white"},
			{label: "黄", value: "amber"},
			{label: "紫", value: "violet"},
			{label: "控えめ", value: "quiet"},
		},
		get: func(settings Settings) string { return settings.TitlePreset },
		set: func(settings *Settings, value string) {
			settings.TitlePreset = value
		},
	},
	{
		label: "選択行",
		options: []settingOption{
			{label: "黄", value: "amber"},
			{label: "シアン", value: "cyan"},
			{label: "紫", value: "violet"},
			{label: "モノクロ", value: "mono"},
		},
		get: func(settings Settings) string { return settings.SelectionPreset },
		set: func(settings *Settings, value string) {
			settings.SelectionPreset = value
		},
	},
}

// applyTheme は選んだプリセットを実際のスタイルへ反映する。TUI はプロセスに
// 1 つなので、描画側は設定を持ち回らずパッケージ変数を見るだけでよい。
// 設定ファイルに知らない名前が書かれていたら既定へ落とす。
func applyTheme(settings Settings) {
	defaults := DefaultSettings()

	frame, ok := framePresets[settings.FramePreset]
	if !ok {
		frame = framePresets[defaults.FramePreset]
	}
	plainFrame.style = color(frame.plain)
	selectedFrame.style = color(frame.selected).Bold(true)
	focusedFrame.style = color(frame.focused).Bold(true)
	modalFrame.style = focusedFrame.style
	dimStyle = color(frame.dim)

	graph, ok := graphPresets[settings.GraphPreset]
	if !ok {
		graph = graphPresets[defaults.GraphPreset]
	}
	heapStyle = color(graph.heap)
	rssStyle = color(graph.rss)
	overlapStyle = color(graph.overlap)

	meter, ok := meterPresets[settings.MeterPreset]
	if !ok {
		meter = meterPresets[defaults.MeterPreset]
	}
	meterFullStyle = color(meter.full)
	meterHighStyle = color(meter.high)
	meterOverStyle = color(meter.over)
	meterEmptyStyle = color(meter.empty)

	title, ok := titlePresets[settings.TitlePreset]
	if !ok {
		title = titlePresets[defaults.TitlePreset]
	}
	titleStyle = color(title).Bold(true)

	selection, ok := selectionPresets[settings.SelectionPreset]
	if !ok {
		selection = selectionPresets[defaults.SelectionPreset]
	}
	selectedStyle = color(selection.foreground).
		Background(lipgloss.Color(selection.background)).
		Bold(true)
	keyStyle = color(selection.keyForeground).
		Background(lipgloss.Color(selection.keyBackground))
}

func color(value string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(value))
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
		applyTheme(model.settings)
	case tea.KeyRight:
		settingItems[model.settingCursor].shift(&model.settings, 1)
		applyTheme(model.settings)
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
	box := renderPanel("Settings", lines, width, height, false, modalFrame)
	return box, x, y
}
