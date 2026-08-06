package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

// 初期値を既定プリセットと揃え、New を通さないテストでも同じ配色にする。
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8BE9FD")).
			Bold(true)
	heapStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#50FA7B"))
	rssStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8BE9FD"))
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#777777"))
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#282A36")).
			Background(lipgloss.Color("#F1FA8C")).
			Bold(true)
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#282A36")).
			Background(lipgloss.Color("#BBBBBB"))
	// 終了モーダルの枠。異常終了と正常終了の区別は配色プリセットに
	// 左右させない。単色プリセットで両者が同じ色になると意味が壊れる。
	exitCrashStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")).
			Bold(true)
	exitStoppedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#50FA7B")).
				Bold(true)
	logTimestampStyle = color("#777777")
	logReceivedStyle  = color("#5A5A5A").Faint(true)
	logKindStyles     = map[serverlog.Kind]lipgloss.Style{
		serverlog.KindPlayerJoin:  color("#50FA7B"),
		serverlog.KindPlayerLeave: color("#FF5555"),
		serverlog.KindChat:        color("#F8F8F2"),
		serverlog.KindCommand:     color("#BD93F9"),
		serverlog.KindLag:         color("#FFB86C"),
		serverlog.KindOther:       color("#BBBBBB"),
	}
	logPlayerStyles = playerStyles([]string{
		"#8BE9FD", "#FF79C6", "#F1FA8C", "#50FA7B", "#BD93F9", "#FFB86C",
	})
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
}

// 配色をグループ単位に限定し、反転表示や警告色の意味を壊さない。
func applyTheme(settings Settings) {
	defaults := DefaultSettings()

	frame := preset(framePresets, settings.FramePreset, defaults.FramePreset)
	plainFrame.style = color(frame.plain)
	selectedFrame.style = color(frame.selected).Bold(true)
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
