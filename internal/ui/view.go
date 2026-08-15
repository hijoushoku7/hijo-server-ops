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
	} else if model.exit != nil && !model.exit.autoRestart {
		content = model.renderBufferPanelWithTitle(
			msg.StoppedLogTitle(formatExitCode(model.exit.exitCode)),
			panelLog,
			&model.logs,
			model.layout.width,
			model.layout.height-keybarHeight,
		) + "\n" + model.keybar()
		if !model.exit.closed {
			box, x, y := model.exitModal()
			content = overlay(content, box, x, y)
		}
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
		// 自動再起動の最中はダッシュボードを背景のまま残す。ログ全面に
		// 切り替えると、勝手に戻ってくる画面で操作を促すことになる。
		case model.exit != nil:
			box, x, y := model.exitModal()
			content = overlay(content, box, x, y)
		case model.timeModal != nil:
			// 時刻入力は設定モーダルを閉じず、その上へ重ねる。
			box, x, y := model.settingsModal()
			content = overlay(content, box, x, y)
			box, x, y = model.timeSettingsModal()
			content = overlay(content, box, x, y)
		case model.settingsOpen:
			box, x, y := model.settingsModal()
			content = overlay(content, box, x, y)
		case model.mode == modeFocus && model.panel == panelPlayers &&
			model.playerStage == playerStageCommands:
			box, x, y := model.commandModal()
			content = overlay(content, box, x, y)
		case model.editingConsole() && model.completionOpen:
			box, x, y := model.completionModal()
			content = overlay(content, box, x, y)
		}
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = model.windowTitle()
	return view
}

func (model *Model) windowTitle() string {
	if model.info.Name == "" {
		return "hijo-server-ops"
	}
	return model.info.Name + " · hijo-server-ops"
}

func (model *Model) consoleLine() string {
	restart := "[restart]"
	if model.restartPhase != 0 {
		restart = "[restarting" + model.restartDots() + "]"
	}
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
	// 候補は装飾なので、入力とカーソルを引いた残りに収まるぶんだけ出す。
	hint := ""
	if model.editingConsole() {
		hintWidth := max(0, inputWidth-stringWidth(input)-stringWidth(cursor))
		if text := truncate(model.completionHint(), hintWidth); text != "" {
			hint = dimStyle.Render(text)
		}
	}
	return fitLine("> "+input+cursor+hint+" "+buttons, model.layout.width-2)
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
	return model.renderBufferPanelWithTitle(
		target.title(), target, buffer, width, height,
	)
}

func (model *Model) renderBufferPanelWithTitle(
	title string,
	target panel,
	buffer *lineBuffer,
	width, height int,
) string {
	contentHeight := max(0, height-2)
	innerWidth := max(0, width-2)
	viewport := bufferViewport{width: innerWidth, height: contentHeight}
	window := buffer.Window(viewport)
	lines := make([]string, contentHeight)
	for index := range lines {
		lines[index] = strings.Repeat(" ", innerWidth)
	}
	padding := max(0, contentHeight-len(window))
	for index := 0; index < len(window) && padding+index < len(lines); index++ {
		lines[padding+index] = renderLogLine(window[index], innerWidth)
	}

	if offset := buffer.Offset(viewport); offset > 0 {
		title = fmt.Sprintf("%s ↑%d", title, offset)
	}
	return renderFittedPanel(title, lines, width, height, true, model.frameFor(target))
}

func renderPanel(
	title string,
	lines []string,
	width int,
	height int,
	alignBottom bool,
	box frame,
) string {
	return renderPanelLines(title, lines, width, height, alignBottom, box, false)
}

// renderFittedPanel は素テキストで切り詰めてから色を付けた行をそのまま描く。
// ANSI 付きの行へ再度 fitLine を通さず、ログの色と枠幅を保つ。
func renderFittedPanel(
	title string,
	lines []string,
	width int,
	height int,
	alignBottom bool,
	box frame,
) string {
	return renderPanelLines(title, lines, width, height, alignBottom, box, true)
}

func renderPanelLines(
	title string,
	lines []string,
	width int,
	height int,
	alignBottom bool,
	box frame,
	linesFitted bool,
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
		if !linesFitted {
			line = fitLine(line, innerWidth)
		}
		result.WriteString(line)
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
	case model.exit != nil && model.exit.restarted:
		keys = [][2]string{{"Enter", msg.BarClose}, {"^C", msg.BarExit}}
	case model.exit != nil && model.exit.autoRestart:
		// 再起動を頼んだ後はやめられないので、案内も消す。
		keys = [][2]string{{"^C", msg.BarExit}}
		if !model.exit.autoRestartAt.IsZero() {
			keys = append([][2]string{{"Esc", msg.BarStopAuto}}, keys...)
		}
	case model.exit != nil && !model.exit.closed:
		keys = [][2]string{
			{"hl/←→/Tab", msg.BarExitButton},
			{"Enter", msg.BarConfirm},
			{"Esc", msg.BarReadLogs},
			{"^C", msg.BarExit},
		}
	case model.exit != nil:
		keys = [][2]string{
			{"kj/↑↓/Pg", msg.BarScroll},
			{"Home/End", msg.BarEnds},
			{"R", msg.BarRestart},
			{"Q/Enter", msg.BarExit},
		}
	case model.timeModal != nil:
		// 時刻入力だけは hjkl を併記しない。en の説明が長く、足すと最小幅で
		// 末尾が切れる。Enter / Esc を ↵ / ^[ に縮めてまで載せる案内ではない。
		keys = [][2]string{
			{"←→", msg.BarTimeField},
			{"↑↓", msg.BarTimeAdjust},
			{"Enter", msg.BarConfirm},
			{"Esc", msg.BarCancel},
			{"^C", msg.BarExit},
		}
	case model.settingsOpen:
		keys = [][2]string{
			{"kj/↑↓", msg.BarItem},
			{"hl/←→", msg.BarValue},
			{"Enter/Esc", msg.BarClose},
			{"^C", msg.BarExit},
		}
	case model.mode == modeSelect:
		keys = [][2]string{
			{"hjkl/←↑↓→", msg.BarSelectPanel},
			{"Enter", msg.BarFocus},
			{"G", msg.BarSettings},
			{"^C", msg.BarExit},
		}
	case model.completionOpen:
		keys = [][2]string{
			{"Tab/↑↓", msg.BarCandidate},
			{"Enter", msg.BarConfirm},
			{"Esc", msg.BarClose},
			{"^C", msg.BarExit},
		}
	case model.panel == panelConsole:
		tab := msg.BarConsoleTab
		if model.editingConsole() && len(model.completions()) > 0 {
			tab = msg.BarComplete
		}
		keys = [][2]string{
			{"Esc", msg.BarBackToSelect},
			{"Tab", tab},
			{"Enter", msg.BarExecute},
			{"^C", msg.BarExit},
		}
	case model.panel == panelPlayers &&
		model.playerStage == playerStageCommands:
		keys = [][2]string{
			{"Esc", msg.BarBack},
			{"kj/↑↓", msg.BarCommand},
			{"Enter", msg.BarPutInConsole},
			{"^C", msg.BarExit},
		}
	case model.panel == panelPlayers:
		keys = [][2]string{
			{"Esc", msg.BarBackToSelect},
			{"kj/↑↓", msg.BarPlayer},
			{"Enter", msg.BarCommandList},
			{"^C", msg.BarExit},
		}
	default:
		keys = [][2]string{
			{"Esc", msg.BarBackToSelect},
			{"kj/↑↓", msg.BarScroll},
			{"Pg", msg.BarPage},
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
