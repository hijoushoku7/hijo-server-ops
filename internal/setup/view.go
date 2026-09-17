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

	titleStyle    = palette.Title
	dimStyle      = palette.Dim
	selectedStyle = palette.Selected
	keyStyle      = palette.Key

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555"))
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
	case stepInstallKind:
		return append([]string{msg.SetupStepInstallKind, ""}, selectionLines(installKindLabels(), m.cursor)...)
	case stepInstallVersion:
		versions := m.visibleVersions()
		if m.versions == nil {
			return []string{msg.SetupStepInstallVersion, "", "  " + msg.SetupLoading, "", dimStyle.Render("  " + msg.SetupCacheTime(m.fetchedAt))}
		}
		labels := make([]string, len(versions))
		for i, version := range versions {
			labels[i] = version.Version
		}
		lines := append([]string{msg.SetupStepInstallVersion, ""}, selectionLines(labels, m.cursor)...)
		return append(lines, "", dimStyle.Render("  "+msg.SetupCacheTime(m.fetchedAt)))
	case stepInstallVersionInput:
		return []string{msg.SetupStepInstallVersionInput, "", "  " + string(m.input) + "█"}
	case stepInstallLoader:
		if m.loaders == nil {
			return []string{msg.SetupStepInstallLoader, "", "  " + msg.SetupLoading, "", dimStyle.Render("  " + msg.SetupCacheTime(m.fetchedAt))}
		}
		labels := make([]string, len(m.loaders))
		for i, loader := range m.loaders {
			labels[i] = loader.Version
			if loader.Recommended {
				labels[i] += " " + msg.SetupLoaderRecommended
			}
		}
		lines := append([]string{msg.SetupStepInstallLoader, ""}, selectionLines(labels, m.cursor)...)
		return append(lines, "", dimStyle.Render("  "+msg.SetupCacheTime(m.fetchedAt)))
	case stepInstallConfirm:
		return []string{
			msg.SetupStepInstallConfirm, "",
			"  " + msg.SetupInstallDirectory(m.workDir),
			"  " + msg.SetupInstallSelection(m.installDescription()), "",
			"  " + msg.SetupEULA,
			dimStyle.Render("  https://aka.ms/MinecraftEULA"),
		}
	case stepInstalling:
		done, total := m.downloaded.Load(), m.downloadTotal.Load()
		progress := msg.SetupDownloadingBytes(done)
		if total > 0 {
			progress = msg.SetupDownloadingProgress(done, total)
		}
		lines := []string{msg.SetupInstalling, "", "  " + progress}
		for _, line := range m.installerOutputLines() {
			lines = append(lines, "  "+line)
		}
		return lines
	case stepInstallJava:
		labels := make([]string, 1, len(m.installations)+1)
		labels[0] = msg.SetupJavaNone
		for _, installation := range m.installations {
			labels = append(labels, fmt.Sprintf("Java %d  %s  %s", installation.Major, installation.Implementor, installation.Home))
		}
		return append([]string{msg.SetupStepInstallJava, ""}, selectionLines(labels, m.cursor)...)
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
	labels := make([]string, 0, len(m.candidates)+2)
	for _, item := range m.candidates {
		labels = append(labels, item.label())
	}
	labels = append(labels, msg.SetupManualEntry)
	labels = append(labels, msg.SetupInstallEntry)
	return selectionLines(labels, m.cursor)
}

func selectionLines(labels []string, cursor int) []string {
	start := windowStart(cursor, len(labels), listViewport)
	end := min(start+listViewport, len(labels))
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		number := "    "
		if index < 9 {
			number = fmt.Sprintf("(%d) ", index+1)
		}
		label := number + labels[index]
		if index == cursor {
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
	case stepInstallJava:
		keys = append(keys, [2]string{"↑↓ / 1-9", msg.KeySelect}, [2]string{"Enter", msg.KeyConfirm})
	case stepInstallKind, stepInstallLoader:
		keys = append(keys, [2]string{"↑↓ / 1-9", msg.KeySelect}, [2]string{"Enter", msg.KeyConfirm}, [2]string{"Esc", msg.KeyBack})
		if m.step == stepInstallLoader && m.installKind == "forge" {
			keys = append(keys, [2]string{"a", msg.KeyAllForgeBuilds})
		}
	case stepInstallVersion:
		keys = append(keys, [2]string{"↑↓ / 1-9", msg.KeySelect}, [2]string{"s", msg.KeyToggleSnapshots}, [2]string{"Enter", msg.KeyConfirm}, [2]string{"Esc", msg.KeyBack})
	case stepInstallVersionInput:
		keys = append(keys, [2]string{"Enter", msg.KeyNext}, [2]string{"Esc", msg.KeyBack})
	case stepInstallConfirm:
		keys = append(keys, [2]string{"Enter", msg.KeyAgreeInstall}, [2]string{"Esc", msg.KeyBack})
	case stepInstalling:
		keys = nil
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
