package main

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

const startListViewport = 10

var (
	startTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8BE9FD")).
			Bold(true)
	startSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#282A36")).
				Background(lipgloss.Color("#F1FA8C")).
				Bold(true)
	startDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#777777"))
	startKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#282A36")).
			Background(lipgloss.Color("#BBBBBB"))
)

type startRow struct {
	server registry.Server
	status serverStatus
}

type startModel struct {
	rows     []startRow
	title    string
	cursor   int
	selected registry.Server
	chosen   bool
}

func (m *startModel) Init() tea.Cmd {
	return nil
}

func (m *startModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.Code {
	case tea.KeyEscape:
		return m, tea.Quit
	case tea.KeyEnter, tea.KeyKpEnter:
		m.selected = m.rows[m.cursor].server
		m.chosen = true
		return m, tea.Quit
	case tea.KeyUp:
		m.cursor--
	case tea.KeyDown:
		m.cursor++
	case tea.KeyHome:
		m.cursor = 0
	case tea.KeyEnd:
		m.cursor = len(m.rows) - 1
	default:
		if key.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if len(key.Text) == 1 && key.Text[0] >= '1' && key.Text[0] <= '9' {
			index := int(key.Text[0] - '1')
			if index < len(m.rows) {
				m.cursor = index
			}
		}
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	return m, nil
}

func (m *startModel) View() tea.View {
	lines := []string{startTitleStyle.Render(m.title), ""}
	nameWidth := 0
	for _, row := range m.rows {
		nameWidth = max(nameWidth, ansi.StringWidth(row.server.Name))
	}
	start := startWindowStart(m.cursor, len(m.rows), startListViewport)
	end := min(start+startListViewport, len(m.rows))
	for index := start; index < end; index++ {
		row := m.rows[index]
		number := "  "
		if index < 9 {
			number = fmt.Sprintf("%d ", index+1)
		}
		label := number + row.server.Name + strings.Repeat(" ", nameWidth-ansi.StringWidth(row.server.Name)+2) +
			row.status.label()
		if index == m.cursor {
			lines = append(lines, "  "+startSelectedStyle.Render(" "+label+" "))
			continue
		}
		lines = append(lines, "   "+label)
	}
	if end < len(m.rows) {
		lines = append(lines, startDimStyle.Render("   …"))
	}
	keys := startKeyStyle.Render(" ↑↓ / 1-9 ") + " " + msg.KeySelect + "  " +
		startKeyStyle.Render(" Enter ") + " " + msg.KeyConfirm + "  " +
		startKeyStyle.Render(" Esc / Ctrl+C ") + " " + msg.KeyAbort
	lines = append(lines, "", keys)
	return tea.NewView(strings.Join(lines, "\n") + "\n")
}

func startWindowStart(cursor, count, viewport int) int {
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

type serverChooser func(registry.Registry) (registry.Server, bool, error)

func runStart(name string) error {
	return runStartWithTerminal(name, interactive())
}

func runStartWithTerminal(name string, terminal bool) error {
	path, err := registry.Path()
	if err != nil {
		return err
	}
	servers, err := registry.Load(path)
	if err != nil {
		return err
	}
	if name == "" && len(servers.Servers) > 0 && !terminal {
		return msg.StartRequiresTerminal()
	}
	choose := func(servers registry.Registry) (registry.Server, bool, error) {
		return chooseServer(servers, pidfile.Running, msg.StartTitle)
	}
	return startFromRegistry(name, servers, choose, pidfile.Running, launchConfig)
}

func startFromRegistry(
	name string,
	servers registry.Registry,
	choose serverChooser,
	running func(string) (int, bool, error),
	launch func(string) error,
) error {
	if len(servers.Servers) == 0 {
		return msg.NoRegisteredServers()
	}

	var server registry.Server
	if name == "" {
		var chosen bool
		var err error
		server, chosen, err = choose(servers)
		if err != nil || !chosen {
			return err
		}
	} else {
		if err := registry.ValidateName(name); err != nil {
			return err
		}
		var found bool
		server, found = servers.Find(name)
		if !found {
			return msg.ServerNotRegistered(name)
		}
	}

	status, err := inspectServer(server, running)
	if err != nil {
		return err
	}
	switch status.state {
	case serverRunning:
		return msg.ServerAlreadyRunning(server.Name, status.pid)
	case serverConfigMissing:
		return msg.RegisteredConfigNotFound(server.Name, server.Config)
	default:
		err := launch(server.Config)
		if !errors.Is(err, pidfile.ErrAlreadyRunning) {
			return err
		}
		if pid, ok := pidfile.AlreadyRunningPID(err); ok {
			return msg.ServerAlreadyRunning(server.Name, pid)
		}
		return msg.ServerAlreadyRunningWithoutPID(server.Name)
	}
}

func chooseServer(
	servers registry.Registry,
	running func(string) (int, bool, error),
	title string,
) (registry.Server, bool, error) {
	rows := make([]startRow, 0, len(servers.Servers))
	for _, server := range servers.Servers {
		status, err := inspectServer(server, running)
		if err != nil {
			return registry.Server{}, false, err
		}
		rows = append(rows, startRow{server: server, status: status})
	}
	model := &startModel{rows: rows, title: title}
	if _, err := tea.NewProgram(model).Run(); err != nil {
		return registry.Server{}, false, err
	}
	return model.selected, model.chosen, nil
}
