package ui

import (
	stdcolor "image/color"

	"charm.land/lipgloss/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

// 既定プリセットを起動時に流し込み、New を通さないテストでも同じ配色にする。
// 初期値をリテラルで二重に持つと既定を変えたときに片方だけ古くなる。
func init() { applyTheme(DefaultSettings()) }

var (
	backgroundColor   stdcolor.Color
	titleStyle        lipgloss.Style
	heapStyle         lipgloss.Style
	rssStyle          lipgloss.Style
	dimStyle          lipgloss.Style
	selectedStyle     lipgloss.Style
	keyStyle          lipgloss.Style
	logTimestampStyle lipgloss.Style
	logReceivedStyle  lipgloss.Style
	logKindStyles     map[serverlog.Kind]lipgloss.Style
	logPlayerStyles   []lipgloss.Style

	// 終了モーダルの枠。異常終了と正常終了の区別は配色プリセットに
	// 左右させない。単色プリセットで両者が同じ色になると意味が壊れる。
	exitCrashStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")).
			Bold(true)
	exitStoppedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#50FA7B")).
				Bold(true)
)

type framePalette struct {
	plain    string
	selected string
	focused  string
	dim      string
}

type graphPalette struct {
	heap    string
	rss     string
	overlap string
}

type meterPalette struct {
	full  string
	high  string
	over  string
	empty string
}

type selectionPalette struct {
	foreground    string
	background    string
	keyForeground string
	keyBackground string
}

type logPalette struct {
	timestamp string
	received  string
	join      string
	leave     string
	chat      string
	command   string
	lag       string
	other     string
	players   []string
}

type themeBundle struct {
	background string
	frame      string
	graph      string
	meter      string
	title      string
	selection  string
	log        string
}

var themeBundles = map[string]themeBundle{
	"dracula": {
		background: "terminal", frame: "dracula", graph: "dracula",
		meter: "signal", title: "cyan", selection: "amber", log: "dracula",
	},
	"mono": {
		background: "terminal", frame: "mono", graph: "mono",
		meter: "mono", title: "white", selection: "mono", log: "mono",
	},
	"safe": {
		background: "terminal", frame: "mono", graph: "safe",
		meter: "safe", title: "white", selection: "mono", log: "safe",
	},
	"sunset": {
		background: "terminal", frame: "sunset", graph: "sunset",
		meter: "sunset", title: "sunset", selection: "sunset", log: "sunset",
	},
	"sakura": {
		background: "deep", frame: "sakura", graph: "sakura",
		meter: "sakura", title: "sakura", selection: "sakura", log: "sakura",
	},
	"nord": {
		background: "night", frame: "nord", graph: "nord",
		meter: "nord", title: "nord", selection: "nord", log: "nord",
	},
}

func (bundle themeBundle) apply(settings *Settings) {
	settings.BackgroundPreset = bundle.background
	settings.FramePreset = bundle.frame
	settings.GraphPreset = bundle.graph
	settings.MeterPreset = bundle.meter
	settings.TitlePreset = bundle.title
	settings.SelectionPreset = bundle.selection
	settings.LogPreset = bundle.log
}

func (bundle themeBundle) matches(settings Settings) bool {
	return settings.BackgroundPreset == bundle.background &&
		settings.FramePreset == bundle.frame &&
		settings.GraphPreset == bundle.graph &&
		settings.MeterPreset == bundle.meter &&
		settings.TitlePreset == bundle.title &&
		settings.SelectionPreset == bundle.selection &&
		settings.LogPreset == bundle.log
}

func themeName(settings Settings) string {
	for name, bundle := range themeBundles {
		if bundle.matches(settings) {
			return name
		}
	}
	return ""
}

var framePresets = map[string]framePalette{
	"dracula": {plain: "#777777", selected: "#8BE9FD", focused: "#F1FA8C", dim: "#777777"},
	"mono":    {plain: "#5A5A5A", selected: "#AAAAAA", focused: "#F8F8F2", dim: "#8A8A8A"},
	"neon":    {plain: "#6272A4", selected: "#FF79C6", focused: "#8BE9FD", dim: "#6272A4"},
	"ocean":   {plain: "#44506B", selected: "#6FB3D2", focused: "#A0E9FF", dim: "#5A6E8C"},
	"forest":  {plain: "#4E6151", selected: "#8FBC8F", focused: "#D7E8A0", dim: "#6B7F6B"},
	"sunset":  {plain: "#7A5360", selected: "#FF8A65", focused: "#FFB4A2", dim: "#87616B"},
	"sakura":  {plain: "#70556E", selected: "#F38BA8", focused: "#CBA6F7", dim: "#80677E"},
	"nord":    {plain: "#4C566A", selected: "#81A1C1", focused: "#88C0D0", dim: "#616E88"},
}

var graphPresets = map[string]graphPalette{
	"dracula": {heap: "#50FA7B", rss: "#8BE9FD", overlap: "#F1FA8C"},
	"mono":    {heap: "#F8F8F2", rss: "#909090", overlap: "#FFFFFF"},
	"warm":    {heap: "#FFB86C", rss: "#FF79C6", overlap: "#F1FA8C"},
	"cool":    {heap: "#8BE9FD", rss: "#BD93F9", overlap: "#F8F8F2"},
	// Okabe-Ito の青と橙。色覚型によらず区別できる組み合わせ。
	"safe":   {heap: "#56B4E9", rss: "#E69F00", overlap: "#F0F0F0"},
	"sunset": {heap: "#FF8A65", rss: "#C77DFF", overlap: "#FFD166"},
	"sakura": {heap: "#F38BA8", rss: "#89B4FA", overlap: "#CBA6F7"},
	"nord":   {heap: "#A3BE8C", rss: "#81A1C1", overlap: "#D08770"},
}

var meterPresets = map[string]meterPalette{
	"signal": {full: "#50FA7B", high: "#FFB86C", over: "#FF5555", empty: "#444444"},
	"mono":   {full: "#8A8A8A", high: "#C8C8C8", over: "#F8F8F2", empty: "#333333"},
	"safe":   {full: "#56B4E9", high: "#F0E442", over: "#D55E00", empty: "#333333"},
	// 段階で色を変えず、長さだけで読ませる。
	"flat":   {full: "#8BE9FD", high: "#8BE9FD", over: "#8BE9FD", empty: "#333333"},
	"sunset": {full: "#FFB86C", high: "#FF8A65", over: "#FF5555", empty: "#49343A"},
	"sakura": {full: "#A6E3A1", high: "#FAB387", over: "#F38BA8", empty: "#423442"},
	"nord":   {full: "#A3BE8C", high: "#EBCB8B", over: "#BF616A", empty: "#3B4252"},
}

var titlePresets = map[string]string{
	"cyan":   "#8BE9FD",
	"white":  "#F8F8F2",
	"amber":  "#F1FA8C",
	"violet": "#BD93F9",
	"quiet":  "#999999",
	"sunset": "#FF8A65",
	"sakura": "#F38BA8",
	"nord":   "#88C0D0",
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
	"sunset": {
		foreground: "#2B1B20", background: "#FF8A65",
		keyForeground: "#2B1B20", keyBackground: "#FFB4A2",
	},
	"sakura": {
		foreground: "#241824", background: "#F38BA8",
		keyForeground: "#241824", keyBackground: "#CBA6F7",
	},
	"nord": {
		foreground: "#2E3440", background: "#88C0D0",
		keyForeground: "#2E3440", keyBackground: "#81A1C1",
	},
}

var logPresets = map[string]logPalette{
	"dracula": {
		timestamp: "#777777", received: "#5A5A5A",
		join: "#50FA7B", leave: "#FF5555", chat: "#F8F8F2",
		command: "#BD93F9", lag: "#FFB86C", other: "#BBBBBB",
		players: []string{"#8BE9FD", "#FF79C6", "#F1FA8C", "#50FA7B", "#BD93F9", "#FFB86C"},
	},
	"mono": {
		timestamp: "#777777", received: "#555555",
		join: "#E0E0E0", leave: "#A0A0A0", chat: "#F8F8F2",
		command: "#C8C8C8", lag: "#FFFFFF", other: "#909090",
		players: []string{"#FFFFFF", "#E0E0E0", "#C8C8C8", "#B0B0B0"},
	},
	"warm": {
		timestamp: "#9B7B6B", received: "#70584D",
		join: "#F1FA8C", leave: "#FF5555", chat: "#FFF1DC",
		command: "#FF79C6", lag: "#FFB86C", other: "#C9A58A",
		players: []string{"#FFD166", "#FF9F68", "#FF79C6", "#F1FA8C", "#FFB86C"},
	},
	"cool": {
		timestamp: "#6272A4", received: "#44506B",
		join: "#8BE9FD", leave: "#BD93F9", chat: "#E6F7FF",
		command: "#7AA2F7", lag: "#FF79C6", other: "#8FA9B8",
		players: []string{"#8BE9FD", "#56B4E9", "#BD93F9", "#A0E9FF", "#7DCFFF"},
	},
	"safe": {
		timestamp: "#777777", received: "#555555",
		join: "#009E73", leave: "#D55E00", chat: "#F0F0F0",
		command: "#CC79A7", lag: "#E69F00", other: "#999999",
		players: []string{"#56B4E9", "#E69F00", "#CC79A7", "#009E73", "#F0E442"},
	},
	"sunset": {
		timestamp: "#A77B7F", received: "#76565E",
		join: "#FFD166", leave: "#FF5555", chat: "#FFF0E6",
		command: "#F06292", lag: "#FF8A65", other: "#D8A0A8",
		players: []string{"#FF8A65", "#F06292", "#FFD166", "#FFB4A2", "#C77DFF"},
	},
	"sakura": {
		timestamp: "#94738F", received: "#685163",
		join: "#A6E3A1", leave: "#F38BA8", chat: "#FCEAF3",
		command: "#CBA6F7", lag: "#FAB387", other: "#B99AAF",
		players: []string{"#F38BA8", "#CBA6F7", "#89B4FA", "#A6E3A1", "#F9E2AF"},
	},
	"nord": {
		timestamp: "#6B7894", received: "#4C566A",
		join: "#A3BE8C", leave: "#BF616A", chat: "#ECEFF4",
		command: "#B48EAD", lag: "#EBCB8B", other: "#9AA7BD",
		players: []string{"#88C0D0", "#81A1C1", "#A3BE8C", "#EBCB8B", "#B48EAD"},
	},
}

// 明るい背景は暗背景向けの既定前景を読みにくくするため設けず、暗色を充実させる。
var backgroundPresets = map[string]string{
	"terminal": "",
	"dark":     "#1E1E24",
	"night":    "#111827",
	"deep":     "#09090B",
	"charcoal": "#20242B",
}

// 配色をグループ単位に限定し、反転表示や警告色の意味を壊さない。
func applyTheme(settings Settings) {
	defaults := DefaultSettings()
	background := preset(
		backgroundPresets,
		settings.BackgroundPreset,
		defaults.BackgroundPreset,
	)
	backgroundColor = nil
	if background != "" {
		backgroundColor = lipgloss.Color(background)
	}

	frame := preset(framePresets, settings.FramePreset, defaults.FramePreset)
	plainFrame.style = color(frame.plain)
	selectedFrame.style = color(frame.selected).Bold(true)
	hoverFrame.style = color(frame.selected)
	focusedFrame.style = color(frame.focused).Bold(true)
	modalFrame.style = focusedFrame.style
	dimStyle = color(frame.dim)

	graph := preset(graphPresets, settings.GraphPreset, defaults.GraphPreset)
	heapStyle = color(graph.heap)
	rssStyle = color(graph.rss)
	overlapStyle = color(graph.overlap)

	meter := preset(meterPresets, settings.MeterPreset, defaults.MeterPreset)
	meterFullStyle = color(meter.full)
	meterHighStyle = color(meter.high)
	meterOverStyle = color(meter.over)
	meterEmptyStyle = color(meter.empty)

	title := preset(titlePresets, settings.TitlePreset, defaults.TitlePreset)
	titleStyle = color(title).Bold(true)

	selection := preset(
		selectionPresets,
		settings.SelectionPreset,
		defaults.SelectionPreset,
	)
	selectedStyle = color(selection.foreground).
		Background(lipgloss.Color(selection.background)).
		Bold(true)
	keyStyle = color(selection.keyForeground).
		Background(lipgloss.Color(selection.keyBackground))

	logs := preset(logPresets, settings.LogPreset, defaults.LogPreset)
	logTimestampStyle = color(logs.timestamp)
	// 受信時刻はログ由来の時刻より暗くし、推定値であることを区別する。
	logReceivedStyle = color(logs.received).Faint(true)
	logKindStyles = map[serverlog.Kind]lipgloss.Style{
		serverlog.KindPlayerJoin:  color(logs.join),
		serverlog.KindPlayerLeave: color(logs.leave),
		serverlog.KindChat:        color(logs.chat),
		serverlog.KindCommand:     color(logs.command),
		serverlog.KindLag:         color(logs.lag),
		serverlog.KindOther:       color(logs.other),
	}
	logPlayerStyles = playerStyles(logs.players)
}

func preset[T any](presets map[string]T, selected, fallback string) T {
	if value, ok := presets[selected]; ok {
		return value
	}
	return presets[fallback]
}

func color(value string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(value))
}

func playerStyles(colors []string) []lipgloss.Style {
	styles := make([]lipgloss.Style, len(colors))
	for index, value := range colors {
		styles[index] = color(value).Bold(true)
	}
	return styles
}
