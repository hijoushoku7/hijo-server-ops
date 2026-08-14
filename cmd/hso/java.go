package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/javaenv"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

const javaRoot = "/usr/lib/jvm"

var javaCurrentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))

type javaModel struct {
	installations []javaenv.Installation
	cursor        int
	selected      javaenv.Installation
	chosen        bool
	running       bool
}

func (m *javaModel) Init() tea.Cmd { return nil }

func (m *javaModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.Code {
	case tea.KeyEscape:
		return m, tea.Quit
	case tea.KeyEnter, tea.KeyKpEnter:
		m.selected, m.chosen = m.installations[m.cursor], true
		return m, tea.Quit
	case tea.KeyUp:
		m.cursor--
	case tea.KeyDown:
		m.cursor++
	case tea.KeyHome:
		m.cursor = 0
	case tea.KeyEnd:
		m.cursor = len(m.installations) - 1
	default:
		if key.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	m.cursor = max(0, min(m.cursor, len(m.installations)-1))
	return m, nil
}

func (m *javaModel) View() tea.View {
	lines := []string{startTitleStyle.Render(msg.JavaChangeTitle), ""}
	for index, installation := range m.installations {
		marker := " "
		if index == m.cursor {
			marker = ">"
		}
		current := ""
		if javaHomesMatch(installation.Home, m.selected.Home) {
			current = " " + javaCurrentStyle.Render(msg.JavaCurrentMark)
		}
		lines = append(lines, fmt.Sprintf(" %s Java %d  %s  %s%s", marker, installation.Major,
			installation.Implementor, installation.Home, current))
	}
	if m.running {
		lines = append(lines, "", msg.JavaRunningNotice)
	}
	lines = append(lines, "", msg.JavaDetectionNote, "",
		startKeyStyle.Render(" ↑↓ ")+" "+msg.KeySelect+"  "+
			startKeyStyle.Render(" Enter ")+" "+msg.KeyConfirm+"  "+
			startKeyStyle.Render(" Esc / Ctrl+C ")+" "+msg.KeyAbort)
	return tea.NewView(strings.Join(lines, "\n") + "\n")
}

type javaChooser func([]javaenv.Installation, string, bool) (javaenv.Installation, bool, error)
type registryChooser func(registry.Registry) (registry.Server, bool, error)

func runJava(args []string, output, warnings io.Writer, terminal bool) error {
	if len(args) == 0 {
		_, err := io.WriteString(output, msg.JavaCommandHelp+"\n")
		return err
	}
	switch args[0] {
	case "change":
		if len(args) > 2 {
			return msg.JavaChangeArgumentsNotAllowed()
		}
		name := ""
		if len(args) == 2 {
			name = args[1]
		}
		return runJavaChange(name, output, terminal)
	case "list":
		if len(args) != 1 {
			return msg.JavaListArgumentsNotAllowed()
		}
		return runJavaList(output, warnings)
	default:
		return msg.UnknownJavaCommand(args[0])
	}
}

func runJavaChange(name string, output io.Writer, terminal bool) error {
	path, err := registry.Path()
	if err != nil {
		return err
	}
	servers, err := registry.Load(path)
	if err != nil {
		return err
	}
	chooseRegistry := func(servers registry.Registry) (registry.Server, bool, error) {
		return chooseServer(servers, pidfile.Running)
	}
	chooseInstallation := func(installed []javaenv.Installation, current string, running bool) (javaenv.Installation, bool, error) {
		model := newJavaModel(installed, current, running)
		if _, err := tea.NewProgram(model).Run(); err != nil {
			return javaenv.Installation{}, false, err
		}
		return model.selected, model.chosen, nil
	}
	return changeJava(name, servers, terminal, chooseRegistry, chooseInstallation, javaenv.Installed,
		config.Load, config.SetJava, pidfile.Running, output)
}

func changeJava(name string, servers registry.Registry, terminal bool, chooseRegistry registryChooser,
	chooseInstallation javaChooser, installed func(string) ([]javaenv.Installation, error),
	load func(string) (config.Config, error), setJava func(string, string) error,
	running func(string) (int, bool, error), output io.Writer,
) error {
	if len(servers.Servers) == 0 {
		return msg.NoRegisteredServers()
	}
	if !terminal {
		return msg.JavaChangeRequiresTerminal()
	}
	var server registry.Server
	if name != "" {
		if err := registry.ValidateName(name); err != nil {
			return err
		}
		var found bool
		server, found = servers.Find(name)
		if !found {
			return msg.ServerNotRegistered(name)
		}
	} else if len(servers.Servers) == 1 {
		server = servers.Servers[0]
	} else {
		var chosen bool
		var err error
		server, chosen, err = chooseRegistry(servers)
		if err != nil || !chosen {
			return err
		}
	}
	cfg, err := load(server.Config)
	if err != nil {
		return err
	}
	installations, err := installed(javaRoot)
	if err != nil {
		return msg.JavaScanFailed(err)
	}
	if len(installations) == 0 {
		_, err := fmt.Fprintf(output, "%s\n%s\n", msg.JavaNotFound, msg.JavaDetectionNote)
		return err
	}
	_, alive, err := running(server.Name)
	if err != nil {
		return err
	}
	selected, chosen, err := chooseInstallation(installations, cfg.Server.Java, alive)
	if err != nil || !chosen {
		return err
	}
	if err := setJava(server.Config, selected.Home); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, msg.JavaChanged(server.Name, selected.Home))
	return err
}

func newJavaModel(installations []javaenv.Installation, current string, running bool) *javaModel {
	model := &javaModel{installations: installations, running: running}
	for index, installation := range installations {
		if javaHomesMatch(installation.Home, current) {
			model.cursor = index
			model.selected = installation
			break
		}
	}
	return model
}

func runJavaList(output, warnings io.Writer) error {
	path, err := registry.Path()
	if err != nil {
		return err
	}
	servers, err := registry.Load(path)
	if err != nil {
		return err
	}
	installations, err := javaenv.Installed(javaRoot)
	if err != nil {
		return msg.JavaScanFailed(err)
	}
	return printJavaList(output, warnings, installations, servers, config.Load)
}

func printJavaList(output, warnings io.Writer, installations []javaenv.Installation, servers registry.Registry,
	load func(string) (config.Config, error),
) error {
	usedBy := make(map[string][]string)
	for _, server := range servers.Servers {
		cfg, err := load(server.Config)
		if err != nil {
			fmt.Fprintln(warnings, msg.JavaConfigWarning(server.Name, err))
			continue
		}
		if cfg.Server.Java == "" {
			fmt.Fprintln(warnings, msg.JavaNotConfiguredWarning(server.Name))
			continue
		}
		matched := false
		for _, installation := range installations {
			if javaHomesMatch(cfg.Server.Java, installation.Home) {
				usedBy[installation.Home] = append(usedBy[installation.Home], server.Name)
				matched = true
				break
			}
		}
		if !matched {
			fmt.Fprintln(warnings, msg.JavaConfiguredNotDetectedWarning(server.Name, cfg.Server.Java))
		}
	}
	if len(installations) == 0 {
		_, err := fmt.Fprintf(output, "%s\n%s\n", msg.JavaNotFound, msg.JavaDetectionNote)
		return err
	}
	type row [4]string
	rows := []row{{msg.JavaHeader, msg.JavaImplementorHeader, msg.JavaHomeHeader, msg.JavaServersHeader}}
	widths := [3]int{ansi.StringWidth(rows[0][0]), ansi.StringWidth(rows[0][1]), ansi.StringWidth(rows[0][2])}
	for _, installation := range installations {
		names := usedBy[installation.Home]
		sort.Strings(names)
		row := row{fmt.Sprint(installation.Major), installation.Implementor, installation.Home, strings.Join(names, ", ")}
		rows = append(rows, row)
		for index := range widths {
			widths[index] = max(widths[index], ansi.StringWidth(row[index]))
		}
	}
	for _, row := range rows {
		line := row[0] + strings.Repeat(" ", widths[0]-ansi.StringWidth(row[0])+2) +
			row[1] + strings.Repeat(" ", widths[1]-ansi.StringWidth(row[1])+2) +
			row[2] + strings.Repeat(" ", widths[2]-ansi.StringWidth(row[2])+2) + row[3] + "\n"
		if _, err := io.WriteString(output, line); err != nil {
			return msg.WriteJavaListFailed(err)
		}
	}
	_, err := fmt.Fprintln(output, "\n"+msg.JavaDetectionNote)
	return err
}

func javaHomesMatch(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	left, leftErr := filepath.EvalSymlinks(filepath.Clean(left))
	right, rightErr := filepath.EvalSymlinks(filepath.Clean(right))
	return leftErr == nil && rightErr == nil && left == right
}
