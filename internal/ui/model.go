package ui

import (
	"errors"
	"fmt"
	"math"
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
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#282A36")).
			Background(lipgloss.Color("#BBBBBB"))
)

const (
	maxInputRunes       = 512
	maxMetricErrorRunes = 256
)

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
	Generation   uint64
	JVM          hsperfdata.Metrics
	Memory       procstats.Memory
	CPU          float64
	CPUAvailable bool
	JVMError     string
	MemoryError  string
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

type Model struct {
	layout            layout
	status            string
	runErr            error
	actions           chan<- Action
	input             []rune
	mode              mode
	panel             panel
	consoleFocus      consoleFocus
	busy              bool
	generation        uint64
	metrics           hsperfdata.Metrics
	memory            procstats.Memory
	cpu               float64
	cpuAvailable      bool
	jvmMetricError    string
	memoryMetricError string
	gcStats           gclog.Stats
	tracker           serverlog.Tracker
	playerList        []string
	playerCursor      int
	playerStage       playerStage
	playerTarget      string
	commandCursor     int
	chat              lineBuffer
	logs              lineBuffer
	samples           sampleBuffer
}

func New(actions chan<- Action, generation uint64) *Model {
	// 起動直後からコマンドを打てるよう、Console にフォーカスした状態で始める。
	return &Model{
		status:     "starting",
		actions:    actions,
		generation: generation,
		mode:       modeFocus,
		panel:      panelConsole,
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
		model.cpu = message.CPU
		model.cpuAvailable = message.CPUAvailable
		model.updateMetricError("heap", &model.jvmMetricError, message.JVMError)
		model.updateMetricError("RSS", &model.memoryMetricError, message.MemoryError)
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
		left := chat + "\n" + model.renderGraphPanel()
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
		if model.mode == modeFocus && model.panel == panelPlayers &&
			model.playerStage == playerStageCommands {
			box, x, y := model.commandModal()
			content = overlay(content, box, x, y)
		}
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
	if model.mode == modeSelect {
		return model.handleSelectKey(key)
	}
	if model.panel == panelConsole {
		return model.handleConsoleKey(key)
	}
	if model.panel == panelPlayers {
		return model.handlePlayersKey(key)
	}
	return model.handleBufferKey(key)
}

// handlePlayersKey はプレイヤー一覧フォーカス中。プレイヤーを選ぶ段階と、
// そのプレイヤーへのコマンドを選ぶ段階の 2 段階で動く。
func (model *Model) handlePlayersKey(key tea.Key) (tea.Model, tea.Cmd) {
	if model.playerStage == playerStageCommands {
		return model.handlePlayerCommandKey(key)
	}

	cursor := moveCursor(key, model.playerCursor, len(model.playerList),
		model.layout.playerLines())
	switch key.Code {
	case tea.KeyEscape:
		model.playerCursor = 0
		model.mode = modeSelect
	case tea.KeyEnter, tea.KeyKpEnter:
		if model.playerCursor < len(model.playerList) {
			model.playerTarget = model.playerList[model.playerCursor]
			model.playerStage = playerStageCommands
			model.commandCursor = 0
		}
	default:
		model.playerCursor = cursor
	}
	return model, nil
}

// handlePlayerCommandKey はコマンド選択中。選んだコマンドは即実行せず、
// Console 入力欄に組み立てて置く。ban / kick の誤操作を Enter の
// もう一押しで止められ、tell の本文や理由もそのまま続けて書ける。
func (model *Model) handlePlayerCommandKey(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		model.playerStage = playerStagePlayers
	case tea.KeyEnter, tea.KeyKpEnter:
		command := playerCommands[model.commandCursor]
		model.input = []rune(fmt.Sprintf(command.template, model.playerTarget))
		model.playerStage = playerStagePlayers
		model.panel = panelConsole
		model.consoleFocus = consoleInput
	default:
		model.commandCursor = moveCursor(key, model.commandCursor,
			len(playerCommands), model.layout.playerLines())
	}
	return model, nil
}

// moveCursor は縦リスト共通のカーソル移動。
func moveCursor(key tea.Key, cursor, count, viewport int) int {
	switch key.Code {
	case tea.KeyUp:
		cursor--
	case tea.KeyDown:
		cursor++
	case tea.KeyPgUp:
		cursor -= viewport
	case tea.KeyPgDown:
		cursor += viewport
	case tea.KeyHome:
		cursor = 0
	case tea.KeyEnd:
		cursor = count - 1
	}
	return clamp(cursor, 0, max(0, count-1))
}

// windowStart はカーソルが見える範囲の表示開始位置。スクロール位置を
// 状態として持たず毎回導出するので、リストが変わってもずれない。
func windowStart(cursor, count, viewport int) int {
	if viewport <= 0 || count <= viewport {
		return 0
	}
	return clamp(cursor-viewport/2, 0, count-viewport)
}

// handleSelectKey は選択モード。矢印でパネルを移動し、Enter でフォーカスする。
func (model *Model) handleSelectKey(key tea.Key) (tea.Model, tea.Cmd) {
	move := neighbors[model.panel]
	switch key.Code {
	case tea.KeyUp:
		model.panel = move.up
	case tea.KeyDown:
		model.panel = move.down
	case tea.KeyLeft:
		model.panel = move.left
	case tea.KeyRight:
		model.panel = move.right
	case tea.KeyEnter, tea.KeyKpEnter:
		model.mode = modeFocus
		model.consoleFocus = consoleInput
	}
	return model, nil
}

// handleConsoleKey は Console フォーカス中。左右キーは入力に使うため、
// restart / stop の選択は Tab で巡回する。
func (model *Model) handleConsoleKey(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		model.mode = modeSelect
	case tea.KeyTab:
		model.consoleFocus = (model.consoleFocus + 1) % consoleFocusCount
	case tea.KeyBackspace:
		if model.consoleFocus == consoleInput && len(model.input) > 0 {
			model.input = model.input[:len(model.input)-1]
		}
	case tea.KeyEnter, tea.KeyKpEnter:
		switch model.consoleFocus {
		case consoleStop:
			return model, tea.Quit
		case consoleRestart:
			model.offer(Action{Kind: ActionRestart})
		default:
			model.sendInput()
		}
	default:
		if model.consoleFocus == consoleInput && key.Text != "" {
			model.appendInput(key.Text)
		}
	}
	return model, nil
}

// handleBufferKey は Chat / Commands / Log フォーカス中のスクロール操作。
func (model *Model) handleBufferKey(key tea.Key) (tea.Model, tea.Cmd) {
	buffer, viewport := model.focusedBuffer()
	if buffer == nil {
		return model, nil
	}

	switch key.Code {
	case tea.KeyEscape:
		// フォーカスを外したら最新行の追従に戻す。遡ったまま放置すると
		// 新着ログが見えなくなる。
		buffer.ScrollToEnd()
		model.mode = modeSelect
	case tea.KeyUp:
		buffer.Scroll(1, viewport)
	case tea.KeyDown:
		buffer.Scroll(-1, viewport)
	case tea.KeyPgUp:
		buffer.Scroll(viewport, viewport)
	case tea.KeyPgDown:
		buffer.Scroll(-viewport, viewport)
	case tea.KeyHome:
		buffer.Scroll(buffer.Len(), viewport)
	case tea.KeyEnd:
		buffer.ScrollToEnd()
	}
	return model, nil
}

func (model *Model) focusedBuffer() (*lineBuffer, int) {
	switch model.panel {
	case panelChat:
		return &model.chat, model.layout.chatLines()
	case panelLog:
		return &model.logs, model.layout.logLines()
	default:
		return nil, 0
	}
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
	model.cpu = 0
	model.cpuAvailable = false
	model.jvmMetricError = ""
	model.memoryMetricError = ""
	model.gcStats = gclog.Stats{}
	model.tracker = serverlog.Tracker{}
	model.playerList = nil
	model.playerCursor = 0
	model.playerStage = playerStagePlayers
	model.samples.SetLimit(0)
	model.samples.SetLimit(model.layout.graphWidth * 2)
}

func (model *Model) resize(width, height int) {
	model.layout = calculateLayout(width, height)
	// 表示できないサイズでは保持もやめる。表示できる間は画面高に関わらず
	// 一定の履歴を持ち、スクロールで遡れるようにする。
	history := 0
	if model.layout.ready {
		history = historyLines
	}
	model.chat.SetLimit(history)
	model.logs.SetLimit(history)
	model.chat.Truncate(model.layout.leftContentWidth())
	model.logs.Truncate(model.layout.rightContentWidth())
	model.samples.SetLimit(model.layout.graphWidth * 2)
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

func (model *Model) addLog(entry serverlog.Entry) {
	if entry.Kind == serverlog.KindIgnored {
		return
	}
	model.tracker.Apply(entry)
	if entry.Kind == serverlog.KindPlayerJoin ||
		entry.Kind == serverlog.KindPlayerLeave {
		model.playerList = model.tracker.Players()
		// 退出などで一覧が縮んでもカーソルが範囲外に残らないようにする。
		model.playerCursor = clamp(
			model.playerCursor,
			0,
			max(0, len(model.playerList)-1),
		)
	}

	switch entry.Kind {
	case serverlog.KindChat:
		line := fmt.Sprintf("<%s> %s", entry.Player, entry.Chat)
		model.chat.Add(truncate(line, model.layout.leftContentWidth()))
	case serverlog.KindCommand:
		// 専用ペインは Graph に譲ったので、コマンドの実行記録は Log に混ぜる。
		line := entry.Command
		if entry.Player != "" {
			line = entry.Player + ": " + entry.Command
		}
		model.logs.Add(truncate(line, model.layout.rightContentWidth()))
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

func (model *Model) updateMetricError(source string, current *string, next string) {
	if next == *current {
		return
	}
	next = truncateRunes(next, maxMetricErrorRunes)
	if next == *current {
		return
	}
	if next == "" {
		if *current != "" {
			model.addLog(serverlog.Entry{
				Kind:    serverlog.KindOther,
				Message: "metrics: " + source + " recovered",
			})
		}
	} else {
		model.addLog(serverlog.Entry{
			Kind:    serverlog.KindOther,
			Message: "metrics: " + source + " unavailable: " + next,
		})
	}
	*current = next
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (model *Model) statsTitle() string {
	title := fmt.Sprintf(
		"hijo-server-ops · %s · uptime %s",
		model.status,
		formatUptime(model.metrics.Uptime),
	)
	if model.jvmMetricError != "" || model.memoryMetricError != "" {
		title += " · metrics degraded"
	}
	return title
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

	// グラフは Graph パネルへ移したので、ここは数値だけ。行間を空けて読む。
	rows := []string{
		fmt.Sprintf(
			"Heap %s / %s committed (max %s)  post-GC %s",
			formatJVMBytes(model.metrics.Heap.Used),
			formatJVMBytes(model.metrics.Heap.Committed),
			formatJVMBytes(model.metrics.Heap.Max),
			postGC,
		),
		fmt.Sprintf(
			"RSS  %s / %s total (%s)  limit %s  Δ %s",
			formatProcBytes(model.memory.RSS),
			formatProcBytes(model.memory.HostTotal),
			formatRSSPercent(model.memory),
			formatLimit(model.memory.CgroupLimit),
			formatDelta(model.memory.RSS, model.metrics.Heap.Committed),
		),
		fmt.Sprintf(
			"GC   %d collections  total %s  last %s  freq %s",
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
		),
		// 名前の列挙は Players パネルに移した。ここは他の指標と並べて
		// 見たい人数だけを残す。
		fmt.Sprintf(
			"Players %d  Lag events: %d  CPU %s  threads %s",
			model.tracker.PlayerCount(),
			model.tracker.LagEvents(),
			formatCPU(model.cpu, model.cpuAvailable),
			threads,
		),
	}

	lines := make([]string, 0, len(rows)*2-1)
	for index, row := range rows {
		if index > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, row)
	}
	return lines
}

// renderGraphPanel は heap と rss を 1 枚に重ねた braille グラフ。
// 表示専用でフォーカス対象ではない。
func (model *Model) renderGraphPanel() string {
	width := model.layout.leftContentWidth()
	var lines []string
	if model.jvmMetricError != "" {
		lines = append(lines, dimStyle.Render(truncate(
			"heap unavailable: "+model.jvmMetricError, width)))
	}
	if model.memoryMetricError != "" {
		lines = append(lines, dimStyle.Render(truncate(
			"rss unavailable: "+model.memoryMetricError, width)))
	}

	low, high, ok := model.graphRange()
	if ok {
		lines = append(lines, renderChart(
			model.samples,
			[]chartSeries{
				{value: heapValue, style: heapStyle},
				{value: rssValue, style: rssStyle},
			},
			width,
			model.layout.graphLines()-len(lines),
			low,
			high,
		)...)
	}
	return renderPanel(
		model.graphTitle(),
		lines,
		model.layout.leftWidth,
		model.layout.graphHeight,
		false,
		plainFrame,
	)
}

// graphTitle には凡例と、画面に写っている時間の幅を出す。
// サンプルは 1 秒に 1 つなので、保持数がそのまま秒数になる。
func (model *Model) graphTitle() string {
	title := "Graph · " +
		heapStyle.Render("heap") + dimStyle.Render(" / ") +
		rssStyle.Render("rss")
	if model.samples.Len() > 0 {
		title += fmt.Sprintf(" · %ds", model.samples.Len())
	}
	return title
}

// graphRange は縦軸の範囲を保持中のサンプルから決める。0 起点にすると
// GC のノコギリ波も RSS のじわ増えも潰れるので、実測の min/max に
// 上下 5% の余白を付けた範囲を使う（原則6）。
func (model *Model) graphRange() (uint64, uint64, bool) {
	low := uint64(math.MaxUint64)
	high := uint64(0)
	for index := 0; index < model.samples.Len(); index++ {
		sample := model.samples.At(index)
		for _, value := range []sampleValue{heapValue, rssValue} {
			number, available := value(sample)
			if !available {
				continue
			}
			low = min(low, number)
			high = max(high, number)
		}
	}
	if high == 0 {
		return 0, 0, false
	}

	margin := max((high-low)/20, high/100)
	if margin == 0 {
		margin = 1
	}
	return low - min(low, margin), high + margin, true
}

// renderMetersPanel は CPU / Heap / RSS を横棒メーターで出す。
// 表示専用でフォーカス対象ではない。
func (model *Model) renderMetersPanel() string {
	rss, source := rssMeter(model.memory)
	meters := []meter{
		cpuMeter(model.cpu, model.cpuAvailable),
		heapMeter(model.metrics.Heap),
		rss,
	}

	// RSS の分母に cgroup 制限とホスト総メモリのどちらを使ったかを明示する。
	title := "Meters"
	if source != "" {
		title += " · RSS/" + source
	}
	return renderPanel(
		title,
		meterLines(meters, model.layout.metersContentWidth()),
		model.layout.metersWidth,
		statsHeight,
		false,
		plainFrame,
	)
}

// renderPlayersPanel はオンラインのプレイヤー一覧。フォーカス中は
// カーソル行を反転させる。行全体を埋めてから着色するので帯状になる。
func (model *Model) renderPlayersPanel() string {
	focused := model.mode == modeFocus && model.panel == panelPlayers
	viewport := model.layout.playerLines()
	width := model.layout.playersContentWidth()
	start := windowStart(model.playerCursor, len(model.playerList), viewport)

	lines := make([]string, viewport)
	for index := 0; index < viewport; index++ {
		position := start + index
		if position >= len(model.playerList) {
			continue
		}
		line := fitLine(truncate(model.playerList[position], width), width)
		if focused && position == model.playerCursor {
			line = selectedStyle.Render(line)
		}
		lines[index] = line
	}

	return renderPanel(
		fmt.Sprintf("%s %d", panelPlayers.title(), len(model.playerList)),
		lines,
		model.layout.playersWidth,
		statsHeight,
		false,
		model.frameFor(panelPlayers),
	)
}

// commandModal は選択中のプレイヤーへのコマンド一覧を、その行の左下に
// 重ねて出すためのモーダルと位置を返す。一覧を差し替えず重ねることで、
// どのプレイヤーを選んだのかが背後に見えたままになる。
func (model *Model) commandModal() (string, int, int) {
	width := stringWidth(model.playerTarget) + 5
	for _, command := range playerCommands {
		width = max(width, stringWidth(command.label)+4)
	}
	height := len(playerCommands) + 2

	lines := make([]string, 0, len(playerCommands))
	for index, command := range playerCommands {
		line := fitLine(" "+command.label, width-2)
		if index == model.commandCursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	// 押したプレイヤー行の左下に出す。画面からはみ出す場合だけ内側へ寄せる。
	start := windowStart(
		model.playerCursor,
		len(model.playerList),
		model.layout.playerLines(),
	)
	x := model.layout.statsWidth + model.layout.metersWidth
	y := model.playerCursor - start + 2
	x = clamp(x, 0, max(0, model.layout.width-width))
	y = clamp(y, 0, max(0, model.layout.height-height))

	box := renderPanel(model.playerTarget, lines, width, height, false, modalFrame)
	return box, x, y
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
		lines[padding+index] = window[index]
	}

	// 最新に追従していないことが分かるよう、遡り行数をタイトルに出す。
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
	// 枠 2 + 前後の空白 2 + 右上の角 1 を除いた分しかタイトルは置けない。
	// width が 6 未満だとこれでも 1 セルはみ出すが、パネルは最小 18 列、
	// モーダルは最小 17 列なのでそこには到達しない。
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

// keybar は nano 風のキー説明行。モードとフォーカス先で内容が変わる。
func (model *Model) keybar() string {
	var keys [][2]string
	switch {
	case model.mode == modeSelect:
		keys = [][2]string{
			{"←↑↓→", "select"},
			{"Enter", "focus"},
			{"^C", "exit"},
		}
	case model.panel == panelConsole:
		keys = [][2]string{
			{"Esc", "select"},
			{"Tab", "input/restart/stop"},
			{"Enter", "execute"},
			{"^C", "exit"},
		}
	case model.panel == panelPlayers &&
		model.playerStage == playerStageCommands:
		keys = [][2]string{
			{"Esc", "back"},
			{"↑↓", "command"},
			{"Enter", "put in console"},
			{"^C", "exit"},
		}
	case model.panel == panelPlayers:
		keys = [][2]string{
			{"Esc", "select"},
			{"↑↓", "player"},
			{"Enter", "commands"},
			{"^C", "exit"},
		}
	default:
		keys = [][2]string{
			{"Esc", "select"},
			{"↑↓", "scroll"},
			{"PgUp/PgDn", "page"},
			{"End", "latest"},
			{"^C", "exit"},
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

func IsExpectedExit(err error) bool {
	return err == nil || errors.Is(err, tea.ErrInterrupted)
}
