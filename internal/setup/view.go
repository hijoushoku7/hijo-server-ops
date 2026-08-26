package setup

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/ui"
)

// 選択画面は hso.toml を読む前に描くので配色を持てず、本体の既定と同じ色で揃える。
var (
	palette = ui.StartupColors()

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(palette.Title)).
			Bold(true)
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(palette.Dim))
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(palette.Foreground)).
			Background(lipgloss.Color(palette.Background)).
			Bold(true)
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555"))
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(palette.KeyForeground)).
			Background(lipgloss.Color(palette.KeyBackground))
)

func (m *model) View() tea.View {
	title := msg.SetupTitle
	target := msg.SetupTarget(m.configPath)
	if m.register {
		title = msg.SetupRegisterTitle
		target = msg.SetupRegisterTarget(m.configPath)
	}
	lines := []string{
		titleStyle.Render(title),
		dimStyle.Render(target),
		"",
	}
	lines = append(lines, m.body()...)
	if m.message != "" {
		lines = append(lines, "", errorStyle.Render(m.message))
	}
	lines = append(lines, "", m.keybar())
	return tea.NewView(strings.Join(lines, "\n") + "\n")
}

func (m *model) body() []string {
	switch m.step {
	case stepRegisterNotice:
		lines := []string{msg.SetupRegisterNotice, ""}
		for _, line := range strings.Split(strings.TrimRight(m.preview(), "\n"), "\n") {
			lines = append(lines, "  "+line)
		}
		return lines
	case stepWorkDir:
		return []string{
			msg.SetupStepWorkDir,
			"",
			"  " + string(m.input) + "█",
		}
	case stepName:
		step := msg.SetupStepName
		if m.register {
			step = msg.SetupRegisterStepName
		}
		return []string{
			step,
			"",
			"  " + string(m.input) + "█",
		}
	case stepCommand:
		return append([]string{msg.SetupStepCommand, ""}, m.candidateLines()...)
	case stepCommandInput:
		return []string{
			msg.SetupStepCommandInput,
			dimStyle.Render("  " + msg.SetupRelativeHint(m.workDir)),
			"",
			"  " + string(m.input) + "█",
		}
	default:
		lines := []string{
			msg.SetupStepConfirm,
			"",
			"  " + msg.SetupServerName(m.name),
			"",
		}
		for _, line := range strings.Split(strings.TrimRight(m.preview(), "\n"), "\n") {
			lines = append(lines, "  "+line)
		}
		if m.needsChmod {
			lines = append(
				lines,
				"",
				"  "+m.chmodLine(),
				dimStyle.Render("  "+m.commandAbs),
			)
		}
		return lines
	}
}

func (m *model) chmodLine() string {
	if m.grantChmod {
		return msg.SetupChmodGrant
	}
	return errorStyle.Render(msg.SetupChmodDeny)
}

func (m *model) candidateLines() []string {
	labels := make([]string, 0, len(m.candidates)+1)
	for _, item := range m.candidates {
		labels = append(labels, item.label())
	}
	labels = append(labels, msg.SetupManualEntry)

	start := windowStart(m.cursor, len(labels), listViewport)
	end := min(start+listViewport, len(labels))
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		number := "  "
		if index < 9 {
			number = fmt.Sprintf("%d ", index+1)
		}
		label := number + labels[index]
		if index == m.cursor {
			lines = append(lines, "  "+selectedStyle.Render(" "+label+" "))
			continue
		}
		lines = append(lines, "   "+label)
	}
	if end < len(labels) {
		lines = append(lines, dimStyle.Render("   …"))
	}
	return lines
}

func windowStart(cursor, count, viewport int) int {
	if count <= viewport {
		return 0
	}
	start := cursor - viewport/2
	if start < 0 {
		start = 0
	}
	if start > count-viewport {
		start = count - viewport
	}
	return start
}

func (m *model) keybar() string {
	var keys [][2]string
	switch m.step {
	case stepRegisterNotice:
		return renderKeys([][2]string{
			{"Enter", msg.KeyAddConfig},
			{"Esc", msg.KeyDoNotAddConfig},
			{"Ctrl+C", msg.KeyAbort},
		})
	case stepWorkDir:
		return renderKeys([][2]string{
			{"Enter", msg.KeyNext},
			{"Esc / Ctrl+C", msg.KeyAbort},
		})
	case stepName:
		if m.register {
			keys = append(keys,
				[2]string{"Enter", msg.KeyRegister},
				[2]string{"Esc", msg.KeyBack},
			)
			break
		}
		keys = append(keys,
			[2]string{"Enter", msg.KeyNext},
			[2]string{"Esc", msg.KeyBack},
		)
	case stepCommand:
		keys = append(keys,
			[2]string{"↑↓ / 1-9", msg.KeySelect},
			[2]string{"Enter", msg.KeyConfirm},
			[2]string{"Esc", msg.KeyBack},
		)
	case stepCommandInput:
		keys = append(keys,
			[2]string{"Enter", msg.KeyNext},
			[2]string{"Esc", msg.KeyBack},
		)
	default:
		keys = append(keys, [2]string{"Enter", msg.KeyCreate})
		if m.needsChmod {
			keys = append(keys, [2]string{"c", msg.KeyToggleChmod})
		}
		keys = append(keys, [2]string{"Esc", msg.KeyBack})
	}
	keys = append(keys, [2]string{"Ctrl+C", msg.KeyAbort})
	return renderKeys(keys)
}

func renderKeys(keys [][2]string) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, keyStyle.Render(" "+key[0]+" ")+" "+key[1])
	}
	return strings.Join(parts, "  ")
}
