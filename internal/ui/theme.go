package ui

import "charm.land/lipgloss/v2"

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
