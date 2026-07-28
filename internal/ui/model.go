package ui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/gclog"
	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

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
)

const maxInputRunes = 512

type ActionKind uint8

const (
	ActionSendCommand ActionKind = iota
	ActionRestart
)

type Action struct {
	Kind    ActionKind
	Command string
}

type LogMsg struct {
	Generation uint64
	Entry      serverlog.Entry
}

type MetricsMsg struct {
	Generation uint64
	JVM        hsperfdata.Metrics
	Memory     procstats.Memory
}

type GCMsg struct {
	Generation uint64
	Event      gclog.Event
}

type JavaFoundMsg struct {
	Generation uint64
	PID        int
}

type ProcessExitedMsg struct {
	Generation uint64
	Err        error
}

type FatalMsg struct {
	Generation uint64
	Err        error
}

type ServerRestartingMsg struct{}

type ServerStartedMsg struct {
	Generation uint64
}

type ActionResultMsg struct {
	Action Action
	Err    error
}

type focus uint8

const (
	focusInput focus = iota
	focusRestart
	focusStop
)

type Model struct {
	layout     layout
	status     string
	runErr     error
	actions    chan<- Action
	input      []rune
	focus      focus
	busy       bool
	generation uint64
	metrics    hsperfdata.Metrics
	memory     procstats.Memory
	gcStats    gclog.Stats
	tracker    serverlog.Tracker
	players    string
	chat       lineBuffer
	commands   lineBuffer
	logs       lineBuffer
	samples    sampleBuffer
}

func New(actions chan<- Action, generation uint64) *Model {
	return &Model{
		status:     "starting",
		actions:    actions,
		generation: generation,
	}
}

func (model *Model) Init() tea.Cmd {
	return nil
}

func (model *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyPressMsg:
		return model.handleKey(message)
	case tea.WindowSizeMsg:
		model.resize(message.Width, message.Height)
	case LogMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.addLog(message.Entry)
	case MetricsMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.metrics = hsperfdata.Metrics{
			Heap:    message.JVM.Heap,
			Uptime:  message.JVM.Uptime,
			Threads: message.JVM.Threads,
		}
		model.memory = message.Memory
		model.addSample(message)
	case GCMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.gcStats.Add(message.Event)
	case JavaFoundMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.status = fmt.Sprintf("java pid %d", message.PID)
	case ServerRestartingMsg:
		model.status = "restarting"
		model.busy = true
	case ServerStartedMsg:
		model.generation = message.Generation
		model.resetServerState()
		model.status = "starting"
		model.busy = false
	case ActionResultMsg:
		model.busy = false
		if message.Err != nil {
			model.status = "操作失敗: " + message.Err.Error()
			break
		}
		if message.Action.Kind == ActionSendCommand {
			model.addLog(serverlog.SentCommand(message.Action.Command))
		}
	case ProcessExitedMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.status = "stopped"
		if model.runErr == nil {
			model.runErr = message.Err
		}
		return model, tea.Quit
	case FatalMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.status = "error"
		if model.runErr == nil {
			model.runErr = message.Err
		}
		return model, tea.Quit
	}
	return model, nil
}

func (model *Model) accepts(generation uint64) bool {
	return generation == 0 || generation == model.generation
}

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
			model.layout.width,
			statsHeight,
			false,
		)
		chat := renderBufferPanel(
			"Chat",
			model.chat,
			model.layout.leftWidth,
			model.layout.chatHeight,
		)
		commands := renderBufferPanel(
			"Commands",
			model.commands,
			model.layout.leftWidth,
			model.layout.commandHeight,
		)
		left := chat + "\n" + commands
		logs := renderBufferPanel(
			"Log",
			model.logs,
			model.layout.rightWidth,
			model.layout.bodyHeight,
		)
		body := joinColumns(left, logs)
		footer := renderPanel(
			"Console",
			[]string{model.consoleLine()},
			model.layout.width,
			footerHeight,
			false,
		)
		content = stats + "\n" + body + "\n" + footer
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "hijo-server-ops"
	return view
}

func (model *Model) Err() error {
	return model.runErr
}

func (model *Model) handleKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := message.Key()
	if message.String() == "ctrl+c" {
		return model, tea.Quit
	}

	switch key.Code {
	case tea.KeyUp, tea.KeyEscape:
		model.focus = focusInput
	case tea.KeyDown:
		if model.focus == focusInput {
			model.focus = focusRestart
		}
	case tea.KeyLeft:
		model.focus = focusRestart
	case tea.KeyRight:
		model.focus = focusStop
	case tea.KeyTab:
		model.focus = (model.focus + 1) % 3
	case tea.KeyBackspace:
		if model.focus == focusInput && len(model.input) > 0 {
			model.input = model.input[:len(model.input)-1]
		}
	case tea.KeyEnter, tea.KeyKpEnter:
		if model.focus == focusStop {
			return model, tea.Quit
		}
		if model.focus == focusRestart {
			model.offer(Action{Kind: ActionRestart})
			break
		}
		model.sendInput()
	default:
		if model.focus == focusInput && key.Text != "" {
			model.appendInput(key.Text)
		}
	}
	return model, nil
}

func (model *Model) appendInput(text string) {
	for _, character := range text {
		if len(model.input) >= maxInputRunes {
			return
		}
		if character >= ' ' && character != '\x7f' {
			model.input = append(model.input, character)
		}
	}
}

func (model *Model) sendInput() {
	command := strings.TrimSpace(string(model.input))
	if command == "" {
		return
	}
	if model.offer(Action{Kind: ActionSendCommand, Command: command}) {
		model.input = model.input[:0]
	}
}

func (model *Model) offer(action Action) bool {
	if model.busy || model.actions == nil {
		return false
	}
	select {
	case model.actions <- action:
		model.busy = true
		if action.Kind == ActionRestart {
			model.status = "restarting"
		}
		return true
	default:
		model.status = "操作待ち"
		return false
	}
}

func (model *Model) resetServerState() {
	model.metrics = hsperfdata.Metrics{}
	model.memory = procstats.Memory{}
	model.gcStats = gclog.Stats{}
	model.tracker = serverlog.Tracker{}
	model.players = ""
	model.samples.SetLimit(0)
	model.samples.SetLimit(model.layout.graphWidth * 2)
}

func (model *Model) resize(width, height int) {
	model.layout = calculateLayout(width, height)
	model.chat.SetLimit(model.layout.chatLines())
	model.commands.SetLimit(model.layout.commandLines())
	model.logs.SetLimit(model.layout.logLines())
	model.chat.Truncate(model.layout.leftContentWidth())
	model.commands.Truncate(model.layout.leftContentWidth())
	model.logs.Truncate(model.layout.rightContentWidth())
	model.samples.SetLimit(model.layout.graphWidth * 2)
}

func (model *Model) consoleLine() string {
	restart := "[restart]"
	stop := "[stop]"
	cursor := ""
	if model.focus == focusInput {
		cursor = "█"
	}
	if model.focus == focusRestart {
		restart = selectedStyle.Render(restart)
	} else {
		restart = dimStyle.Render(restart)
	}
	if model.focus == focusStop {
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

func (model *Model) addLog(entry serverlog.Entry) {
	if entry.Kind == serverlog.KindIgnored {
		return
	}
	model.tracker.Apply(entry)
	if entry.Kind == serverlog.KindPlayerJoin ||
		entry.Kind == serverlog.KindPlayerLeave {
		model.players = strings.Join(model.tracker.Players(), " ")
	}

	switch entry.Kind {
	case serverlog.KindChat:
		line := fmt.Sprintf("<%s> %s", entry.Player, entry.Chat)
		model.chat.Add(truncate(line, model.layout.leftContentWidth()))
	case serverlog.KindCommand:
		line := entry.Command
		if entry.Player != "" {
			line = entry.Player + ": " + entry.Command
		}
		model.commands.Add(truncate(line, model.layout.leftContentWidth()))
	default:
		model.logs.Add(truncate(entry.Message, model.layout.rightContentWidth()))
	}
}

func (model *Model) addSample(message MetricsMsg) {
	sample := memorySample{}
	if message.JVM.Heap.Used.Available && message.JVM.Heap.Used.Value >= 0 {
		sample.heap = uint64(message.JVM.Heap.Used.Value)
		sample.heapKnown = true
	}
	if message.Memory.RSS.Available {
		sample.rss = message.Memory.RSS.Value
		sample.rssKnown = true
	}
	model.samples.Add(sample)
}

func (model *Model) statsTitle() string {
	return fmt.Sprintf(
		"hijo-server-ops · %s · uptime %s",
		model.status,
		formatUptime(model.metrics.Uptime),
	)
}

func (model *Model) statsLines() []string {
	postGC := "n/a"
	if model.gcStats.PostGC.Available {
		postGC = formatBytes(model.gcStats.PostGC.Value)
	}
	frequency := model.gcStats.Frequency()
	threads := "n/a"
	if model.metrics.Threads.Available {
		threads = fmt.Sprintf("%d", model.metrics.Threads.Value)
	}

	lines := []string{
		fmt.Sprintf(
			"Heap %s / %s committed (max %s)  post-GC %s",
			formatJVMBytes(model.metrics.Heap.Used),
			formatJVMBytes(model.metrics.Heap.Committed),
			formatJVMBytes(model.metrics.Heap.Max),
			postGC,
		),
		fmt.Sprintf(
			"RSS  %s / %s limit  Δ %s",
			formatProcBytes(model.memory.RSS),
			formatLimit(model.memory.CgroupLimit),
			formatDelta(model.memory.RSS, model.metrics.Heap.Committed),
		),
		fmt.Sprintf(
			"GC   %d collections  total %s  last %s  freq %s  threads %s",
			model.gcStats.Collections,
			formatPause(
				model.gcStats.TotalPause,
				model.gcStats.LastPause.Available,
			),
			formatPause(
				model.gcStats.LastPause.Value,
				model.gcStats.LastPause.Available,
			),
			formatFrequency(frequency.PerMinute, frequency.Available),
			threads,
		),
		fmt.Sprintf(
			"Players %d [%s]  Lag events: %d",
			model.tracker.PlayerCount(),
			model.players,
			model.tracker.LagEvents(),
		),
	}

	maximum := model.graphMaximum()
	heap := renderBraille(
		model.samples,
		func(sample memorySample) (uint64, bool) {
			return sample.heap, sample.heapKnown
		},
		model.layout.graphWidth,
		2,
		maximum,
	)
	rss := renderBraille(
		model.samples,
		func(sample memorySample) (uint64, bool) {
			return sample.rss, sample.rssKnown
		},
		model.layout.graphWidth,
		2,
		maximum,
	)
	for index, graphLine := range heap {
		label := "     "
		if index == 0 {
			label = "Heap "
		}
		lines = append(lines, heapStyle.Render(label+graphLine))
	}
	for index, graphLine := range rss {
		label := "     "
		if index == 0 {
			label = "RSS  "
		}
		lines = append(lines, rssStyle.Render(label+graphLine))
	}
	return lines
}

func (model *Model) graphMaximum() uint64 {
	var maximum uint64
	if model.metrics.Heap.Max.Available && model.metrics.Heap.Max.Value > 0 {
		maximum = uint64(model.metrics.Heap.Max.Value)
	}
	for index := 0; index < model.samples.Len(); index++ {
		sample := model.samples.At(index)
		if sample.heapKnown && sample.heap > maximum {
			maximum = sample.heap
		}
		if sample.rssKnown && sample.rss > maximum {
			maximum = sample.rss
		}
	}
	return maximum
}

func renderBufferPanel(title string, buffer lineBuffer, width, height int) string {
	contentHeight := max(0, height-2)
	lines := make([]string, contentHeight)
	padding := max(0, contentHeight-buffer.Len())
	for index := 0; index < buffer.Len() && padding+index < len(lines); index++ {
		lines[padding+index] = buffer.At(index)
	}
	return renderPanel(title, lines, width, height, true)
}

func renderPanel(
	title string,
	lines []string,
	width int,
	height int,
	alignBottom bool,
) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if width < 2 || height < 2 {
		return strings.Repeat(" ", width)
	}

	innerWidth := width - 2
	contentHeight := height - 2
	title = truncate(title, max(0, width-4))
	top := "┌─ " + titleStyle.Render(title) + " "
	top += strings.Repeat("─", max(0, width-stringWidth(top)-1)) + "┐"

	var result strings.Builder
	result.WriteString(top)
	start := 0
	if alignBottom && len(lines) > contentHeight {
		start = len(lines) - contentHeight
	}
	for row := 0; row < contentHeight; row++ {
		result.WriteByte('\n')
		result.WriteString("│")
		line := ""
		position := start + row
		if position >= 0 && position < len(lines) {
			line = lines[position]
		}
		result.WriteString(fitLine(line, innerWidth))
		result.WriteString("│")
	}
	result.WriteByte('\n')
	result.WriteString("└")
	result.WriteString(strings.Repeat("─", innerWidth))
	result.WriteString("┘")
	return result.String()
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

func IsExpectedExit(err error) bool {
	return err == nil || errors.Is(err, tea.ErrInterrupted)
}
