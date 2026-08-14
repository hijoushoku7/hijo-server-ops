package setup

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8BE9FD")).
			Bold(true)
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#777777"))
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#282A36")).
			Background(lipgloss.Color("#F1FA8C")).
			Bold(true)
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555"))
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#282A36")).
			Background(lipgloss.Color("#BBBBBB"))
)

func (m *model) View() tea.View {
	lines := []string{
		titleStyle.Render(msg.SetupTitle),
		dimStyle.Render(msg.SetupTarget(m.configPath)),
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
	case stepWorkDir:
		return []string{
			msg.SetupStepWorkDir,
			"",
			"  " + string(m.input) + "█",
		}
	case stepName:
		return []string{
			msg.SetupStepName,
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
		if index == m.cursor {
			lines = append(lines, "  "+selectedStyle.Render(" "+labels[index]+" "))
			continue
		}
		lines = append(lines, "   "+labels[index])
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
	case stepWorkDir:
		return renderKeys([][2]string{
			{"Enter", msg.KeyNext},
			{"Esc / Ctrl+C", msg.KeyAbort},
		})
	case stepName:
		keys = append(keys,
			[2]string{"Enter", msg.KeyNext},
			[2]string{"Esc", msg.KeyBack},
		)
	case stepCommand:
		keys = append(keys,
			[2]string{"↑↓", msg.KeySelect},
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
