package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

func (model *Model) View() tea.View {
	var content string
	if !model.layout.ready {
		content = fmt.Sprintf(
			"hijo-server-ops\n\nterminal is too small: %dx%d\nminimum: %dx%d",
			model.layout.width,
			model.layout.height,
			minimumWidth,
			minimumHeight,
		)
	} else {
		stats := renderPanel(
			model.statsTitle(),
			model.statsLines(),
			model.layout.statsWidth,
			statsHeight,
			false,
			plainFrame,
		)
		top := joinColumns(
			joinColumns(stats, model.renderMetersPanel()),
			model.renderPlayersPanel(),
		)
		chat := model.renderBufferPanel(
			panelChat,
			&model.chat,
			model.layout.leftWidth,
			model.layout.chatHeight,
		)
		left := model.renderGraphPanel() + "\n" + chat
		logs := model.renderBufferPanel(
			panelLog,
			&model.logs,
			model.layout.rightWidth,
			model.layout.bodyHeight,
		)
		body := joinColumns(left, logs)
		footer := renderPanel(
			panelConsole.title(),
			[]string{model.consoleLine()},
			model.layout.width,
			footerHeight,
			false,
			model.frameFor(panelConsole),
		)
		content = top + "\n" + body + "\n" + footer + "\n" + model.keybar()
		switch {
		case model.settingsOpen:
			box, x, y := model.settingsModal()
			content = overlay(content, box, x, y)
		case model.mode == modeFocus && model.panel == panelPlayers &&
			model.playerStage == playerStageCommands:
			box, x, y := model.commandModal()
			content = overlay(content, box, x, y)
		}
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "hijo-server-ops"
	return view
}

func (model *Model) consoleLine() string {
	restart := "[restart]"
	stop := "[stop]"
	focused := model.mode == modeFocus && model.panel == panelConsole
	cursor := ""
	if focused && model.consoleFocus == consoleInput {
		cursor = "█"
	}
	if focused && model.consoleFocus == consoleRestart {
		restart = selectedStyle.Render(restart)
	} else {
		restart = dimStyle.Render(restart)
	}
	if focused && model.consoleFocus == consoleStop {
		stop = selectedStyle.Render(stop)
	} else {
		stop = dimStyle.Render(stop)
	}

	buttons := restart + " " + stop
	inputWidth := max(
		0,
		model.layout.width-2-stringWidth(buttons)-3,
	)
	input := tail(string(model.input), max(0, inputWidth-stringWidth(cursor)))
	return fitLine("> "+input+cursor+" "+buttons, model.layout.width-2)
}

func tail(value string, width int) string {
	if width <= 0 {
		return ""
	}
	valueWidth := stringWidth(value)
	if valueWidth <= width {
		return value
	}
	for remove := valueWidth - width; remove < valueWidth; remove++ {
		result := ansi.TruncateLeft(value, remove, "")
		if stringWidth(result) <= width {
			return result
		}
	}
	return ""
}

func (model *Model) renderBufferPanel(
	target panel,
	buffer *lineBuffer,
	width, height int,
) string {
	contentHeight := max(0, height-2)
	window := buffer.Window(contentHeight)
	lines := make([]string, contentHeight)
	padding := max(0, contentHeight-len(window))
	for index := 0; index < len(window) && padding+index < len(lines); index++ {
		lines[padding+index] = window[index].line()
	}

	title := target.title()
	if offset := buffer.Offset(); offset > 0 {
		title = fmt.Sprintf("%s ↑%d", title, offset)
	}
	return renderPanel(title, lines, width, height, true, model.frameFor(target))
}

func renderPanel(
	title string,
	lines []string,
	width int,
	height int,
	alignBottom bool,
	box frame,
) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if width < 2 || height < 2 {
		return strings.Repeat(" ", width)
	}

	innerWidth := width - 2
	contentHeight := height - 2
	title = truncate(title, max(0, width-5))
	top := box.render(box.topLeft+box.horizontal) + " " +
		titleStyle.Render(title) + " "
	top += box.render(
		strings.Repeat(box.horizontal, max(0, width-stringWidth(top)-1)) +
			box.topRight,
	)

	vertical := box.render(box.vertical)
	var result strings.Builder
	result.WriteString(top)
	start := 0
	if alignBottom && len(lines) > contentHeight {
		start = len(lines) - contentHeight
	}
	for row := 0; row < contentHeight; row++ {
		result.WriteByte('\n')
		result.WriteString(vertical)
		line := ""
		position := start + row
		if position >= 0 && position < len(lines) {
			line = lines[position]
		}
		result.WriteString(fitLine(line, innerWidth))
		result.WriteString(vertical)
	}
	result.WriteByte('\n')
	result.WriteString(box.render(
		box.bottomLeft +
			strings.Repeat(box.horizontal, innerWidth) +
			box.bottomRight,
	))
	return result.String()
}

func (model *Model) keybar() string {
	var keys [][2]string
	switch {
	case model.settingsOpen:
		keys = [][2]string{
			{"↑↓", msg.BarItem},
			{"←→", msg.BarValue},
			{"Enter/Esc", msg.BarClose},
			{"^C", msg.BarExit},
		}
	case model.mode == modeSelect:
		keys = [][2]string{
			{"←↑↓→", msg.BarSelectPanel},
			{"Enter", msg.BarFocus},
			{"G", msg.BarSettings},
			{"^C", msg.BarExit},
		}
	case model.panel == panelConsole:
		keys = [][2]string{
			{"Esc", msg.BarBackToSelect},
			{"Tab", msg.BarConsoleTab},
			{"Enter", msg.BarExecute},
			{"^C", msg.BarExit},
		}
	case model.panel == panelPlayers &&
		model.playerStage == playerStageCommands:
		keys = [][2]string{
			{"Esc", msg.BarBack},
			{"↑↓", msg.BarCommand},
			{"Enter", msg.BarPutInConsole},
			{"^C", msg.BarExit},
		}
	case model.panel == panelPlayers:
		keys = [][2]string{
			{"Esc", msg.BarBackToSelect},
			{"↑↓", msg.BarPlayer},
			{"Enter", msg.BarCommandList},
			{"^C", msg.BarExit},
		}
	default:
		keys = [][2]string{
			{"Esc", msg.BarBackToSelect},
			{"↑↓", msg.BarScroll},
			{"PgUp/PgDn", msg.BarPage},
			{"End", msg.BarLatest},
			{"^C", msg.BarExit},
		}
	}

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, keyStyle.Render(" "+key[0]+" ")+" "+key[1])
	}
	return fitLine(strings.Join(parts, "  "), model.layout.width)
}

func joinColumns(left, right string) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	height := max(len(leftLines), len(rightLines))
	var result strings.Builder
	for row := 0; row < height; row++ {
		if row > 0 {
			result.WriteByte('\n')
		}
		if row < len(leftLines) {
			result.WriteString(leftLines[row])
		}
		if row < len(rightLines) {
			result.WriteString(rightLines[row])
		}
	}
	return result.String()
}

func stringWidth(value string) int {
	return ansi.StringWidth(value)
}
