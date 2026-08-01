package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

func TestModelBoundsLogsAndSamplesToScreen(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	for index := 0; index < historyLines+2; index++ {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:    serverlog.KindOther,
			Message: strings.Repeat("x", 200) + string(rune('a'+index%26)),
		}})
	}

	if model.logs.Len() != historyLines {
		t.Fatalf("logs = %d, limit = %d", model.logs.Len(), historyLines)
	}
	if window := model.logs.Window(model.layout.logLines()); len(window) !=
		model.layout.logLines() {
		t.Fatalf("window = %d, viewport = %d", len(window), model.layout.logLines())
	}
	if stringWidth(model.logs.At(0)) > model.layout.rightContentWidth() {
		t.Fatalf("line width = %d", stringWidth(model.logs.At(0)))
	}

	for index := 0; index < model.layout.graphWidth*2+2; index++ {
		_, _ = model.Update(MetricsMsg{
			JVM: hsperfdata.Metrics{Heap: hsperfdata.Memory{
				Used: hsperfdata.Number{Value: int64(index), Available: true},
			}},
		})
	}
	if model.samples.Len() != model.layout.graphWidth*2 {
		t.Fatalf("samples = %d, limit = %d", model.samples.Len(), model.layout.graphWidth*2)
	}
}

func TestModelRoutesLogsAndTracksPlayers(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	entries := []serverlog.Entry{
		{Kind: serverlog.KindChat, Player: "alice", Chat: "hello"},
		{Kind: serverlog.KindCommand, Player: "alice", Command: "/time set day"},
		{Kind: serverlog.KindPlayerJoin, Player: "alice", Message: "alice joined the game"},
		{Kind: serverlog.KindLag, Message: "Can't keep up!"},
	}
	for _, entry := range entries {
		_, _ = model.Update(LogMsg{Entry: entry})
	}

	if model.chat.At(0) != "<alice> hello" {
		t.Fatalf("chat = %q", model.chat.At(0))
	}
	// コマンドは専用ペインをやめて Log に流している。
	if model.logs.At(0) != "alice: /time set day" {
		t.Fatalf("command = %q", model.logs.At(0))
	}
	if model.tracker.PlayerCount() != 1 || model.tracker.LagEvents() != 1 {
		t.Fatalf(
			"players = %d, lag = %d",
			model.tracker.PlayerCount(),
			model.tracker.LagEvents(),
		)
	}
}

func TestModelViewContainsBrailleGraphs(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = model.Update(MetricsMsg{
		JVM: hsperfdata.Metrics{Heap: hsperfdata.Memory{
			Used: hsperfdata.Number{Value: 2 << 30, Available: true},
			Max:  hsperfdata.Number{Value: 4 << 30, Available: true},
		}},
		Memory: procstats.Memory{
			RSS: procstats.Number{Value: 3 << 30, Available: true},
		},
		CPU:          125,
		CPUAvailable: true,
	})

	content := model.View().Content
	if !strings.Contains(content, "Heap") ||
		!strings.Contains(content, "RSS") ||
		// CPU 表示はコア数で割るのでマシン依存。期待値も同じ式で作る。
		!strings.Contains(content, "CPU "+formatCPU(125, true)) {
		t.Fatalf("view does not contain metrics:\n%s", content)
	}
	if !containsBraille(content) {
		t.Fatalf("view does not contain Braille graph:\n%s", content)
	}
	lines := strings.Split(content, "\n")
	if len(lines) != 24 {
		t.Fatalf("view height = %d", len(lines))
	}
	for index, line := range lines {
		if width := stringWidth(line); width != 80 {
			t.Fatalf("line %d width = %d: %q", index, width, line)
		}
	}
}

func TestModelReleasesDisplayCachesWhenTerminalBecomesTooSmall(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind:    serverlog.KindOther,
		Message: "log",
	}})
	_, _ = model.Update(MetricsMsg{
		Memory: procstats.Memory{
			RSS: procstats.Number{Value: 1, Available: true},
		},
	})
	_, _ = model.Update(tea.WindowSizeMsg{Width: 20, Height: 5})

	if model.chat.lines != nil ||
		model.logs.lines != nil || model.samples.samples != nil {
		t.Fatalf("display caches were retained: %#v", model)
	}
}

func TestModelDoesNotRetainMetricsThatAreNotDisplayed(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(MetricsMsg{
		JVM: hsperfdata.Metrics{
			Generations: []hsperfdata.Generation{{Name: "young"}},
			Collectors:  []hsperfdata.Collector{{Name: "G1 young"}},
		},
	})

	if model.metrics.Generations != nil || model.metrics.Collectors != nil {
		t.Fatalf("unused metrics were retained: %#v", model.metrics)
	}
}

func TestModelReportsMetricFailureOnceAndRecovery(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 24})

	failure := MetricsMsg{JVMError: "hsperfdataが見つかりません"}
	_, _ = model.Update(failure)
	_, _ = model.Update(failure)

	if model.logs.Len() != 1 {
		t.Fatalf("failure logs = %d", model.logs.Len())
	}
	if !strings.Contains(model.statsTitle(), "metrics degraded") {
		t.Fatalf("title = %q", model.statsTitle())
	}
	// 取れない理由はグラフの代わりに Graph パネルへ出す。パネル幅で切られる
	// ので、頭が出ていることだけを見る。
	if !strings.Contains(
		stripANSI(model.renderGraphPanel()),
		"heap unavailable: hsperfdata",
	) {
		t.Fatalf("graph = %q", stripANSI(model.renderGraphPanel()))
	}

	_, _ = model.Update(MetricsMsg{})
	if model.logs.Len() != 2 ||
		!strings.Contains(model.logs.At(1), "heap recovered") {
		t.Fatalf("logs = %q, %q", model.logs.At(0), model.logs.At(1))
	}
	if strings.Contains(model.statsTitle(), "metrics degraded") {
		t.Fatalf("title = %q", model.statsTitle())
	}
}

func TestModelDisplaysServerAddressAndReportsFailure(t *testing.T) {
	model := newTestModel()
	model.generation = 2
	model.resize(minimumWidth, minimumHeight)

	_, _ = model.Update(ServerAddressMsg{
		Generation: 2,
		IP:         "203.0.113.10",
		Port:       25565,
	})
	if got := model.serverAddressLine(); got != "Server 203.0.113.10:25565" {
		t.Fatalf("address = %q", got)
	}
	if width := stringWidth(model.serverAddressLine()); width > model.layout.statsWidth-2 {
		t.Fatalf("address width = %d", width)
	}

	// 前の世代から遅れて届いた結果で、再起動後の表示を上書きしない。
	_, _ = model.Update(ServerAddressMsg{
		Generation: 1,
		IP:         "198.51.100.1",
		Port:       25566,
	})
	if got := model.serverAddressLine(); got != "Server 203.0.113.10:25565" {
		t.Fatalf("stale address was accepted: %q", got)
	}

	_, _ = model.Update(ServerAddressMsg{
		Generation: 2,
		IP:         "invalid IP",
		Port:       25565,
		IPErr:      "request failed",
	})
	if got := model.serverAddressLine(); got != "Server n/a" {
		t.Fatalf("failed address = %q", got)
	}
	if model.logs.Len() != 1 || !strings.Contains(
		model.logs.At(0), "public IPv4 unavailable",
	) {
		t.Fatalf("logs = %q", model.logs.At(0))
	}
}

func TestModelClearsServerAddressOnRestart(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(ServerAddressMsg{
		IP:   "203.0.113.10",
		Port: 25565,
	})
	_, _ = model.Update(ServerStartedMsg{Generation: 2})
	if got := model.serverAddressLine(); got != "Server n/a" {
		t.Fatalf("address = %q", got)
	}
}

func TestModelBoundsCurrentMetricError(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(MetricsMsg{
		JVMError: strings.Repeat("あ", maxMetricErrorRunes+10),
	})

	if len([]rune(model.jvmMetricError)) != maxMetricErrorRunes {
		t.Fatalf("error runes = %d", len([]rune(model.jvmMetricError)))
	}
}

func TestModelReturnsProcessError(t *testing.T) {
	model := newTestModel()
	want := errors.New("server failed")
	_, command := model.Update(ProcessExitedMsg{Err: want})

	if !errors.Is(model.Err(), want) {
		t.Fatalf("Err = %v", model.Err())
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("command = %T", command())
	}
}

func TestModelSendsBoundedCommandInput(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 0, DefaultSettings())

	_, _ = model.Update(tea.KeyPressMsg{Text: strings.Repeat("a", maxInputRunes+10)})
	if len(model.input) != maxInputRunes {
		t.Fatalf("input length = %d", len(model.input))
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	action := <-actions
	if action.Kind != ActionSendCommand ||
		len([]rune(action.Command)) != maxInputRunes {
		t.Fatalf("action = %#v", action)
	}
	if len(model.input) != 0 {
		t.Fatalf("input was not cleared: %q", model.input)
	}
}

func TestModelSelectsRestartAndStopWithTab(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 0, DefaultSettings())

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if model.consoleFocus != consoleRestart {
		t.Fatalf("consoleFocus = %d", model.consoleFocus)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action := <-actions; action.Kind != ActionRestart {
		t.Fatalf("action = %#v", action)
	}

	_, _ = model.Update(ActionResultMsg{Action: Action{Kind: ActionRestart}})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	_, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.consoleFocus != consoleStop {
		t.Fatalf("consoleFocus = %d", model.consoleFocus)
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("command = %T", command())
	}
}

func TestModelMovesBetweenPanelsInSelectMode(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Console フォーカス中は矢印がパネル移動に使われない。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if model.panel != panelConsole || model.mode != modeFocus {
		t.Fatalf("panel = %d, mode = %d", model.panel, model.mode)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.mode != modeSelect {
		t.Fatalf("mode = %d", model.mode)
	}

	moves := []struct {
		code rune
		want panel
	}{
		{tea.KeyUp, panelChat},
		{tea.KeyUp, panelPlayers},
		{tea.KeyDown, panelLog},
		{tea.KeyLeft, panelChat},
		{tea.KeyDown, panelConsole},
	}
	for _, move := range moves {
		_, _ = model.Update(tea.KeyPressMsg{Code: move.code})
		if model.panel != move.want {
			t.Fatalf("panel = %d, want %d", model.panel, move.want)
		}
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.mode != modeFocus || model.consoleFocus != consoleInput {
		t.Fatalf("mode = %d, consoleFocus = %d", model.mode, model.consoleFocus)
	}
}

func TestModelScrollsFocusedBufferAndKeepsPositionOnNewLines(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	viewport := model.layout.logLines()
	for index := 0; index < viewport*3; index++ {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:    serverlog.KindOther,
			Message: fmt.Sprintf("line %d", index),
		}})
	}

	// Log パネルへフォーカスする。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if model.panel != panelLog {
		t.Fatalf("panel = %d", model.panel)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if model.logs.Offset() != viewport {
		t.Fatalf("offset = %d, viewport = %d", model.logs.Offset(), viewport)
	}
	top := model.logs.Window(viewport)[0]

	// 遡っている間は新着で表示が流れない。
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind:    serverlog.KindOther,
		Message: "newest",
	}})
	if got := model.logs.Window(viewport)[0]; got != top {
		t.Fatalf("window shifted: %q -> %q", top, got)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if model.logs.Offset() != 0 {
		t.Fatalf("offset = %d", model.logs.Offset())
	}
	window := model.logs.Window(viewport)
	if window[len(window)-1] != "newest" {
		t.Fatalf("tail = %q", window[len(window)-1])
	}
}

func TestModelReturnsToLatestWhenFocusIsReleased(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	viewport := model.layout.logLines()
	for index := 0; index < viewport*3; index++ {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:    serverlog.KindOther,
			Message: fmt.Sprintf("line %d", index),
		}})
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if model.logs.Offset() == 0 {
		t.Fatalf("buffer did not scroll back")
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.mode != modeSelect {
		t.Fatalf("mode = %d", model.mode)
	}
	if model.logs.Offset() != 0 {
		t.Fatalf("offset = %d, want 0", model.logs.Offset())
	}
	if !strings.Contains(model.View().Content, "line "+fmt.Sprint(viewport*3-1)) {
		t.Fatalf("view does not show the latest line")
	}
}

func TestModelKeybarReflectsMode(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 24})

	if !strings.Contains(model.keybar(), msg.BarExecute) {
		t.Fatalf("console keybar = %q", model.keybar())
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !strings.Contains(model.keybar(), msg.BarFocus) {
		t.Fatalf("select keybar = %q", model.keybar())
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(model.keybar(), msg.BarPage) {
		t.Fatalf("buffer keybar = %q", model.keybar())
	}
}

func TestModelHighlightsFocusedPanelFrame(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	focusedBorder := model.frameFor(panelConsole).render(focusedFrame.topLeft + focusedFrame.horizontal)
	plainBorder := model.frameFor(panelLog).
		render(plainFrame.topLeft + plainFrame.horizontal)
	if focusedBorder == plainBorder {
		t.Fatalf("focused and plain borders are identical: %q", focusedBorder)
	}

	content := model.View().Content
	if !strings.Contains(content, focusedBorder) {
		t.Fatalf("view does not contain the focused border")
	}
	if !strings.Contains(content, plainBorder) {
		t.Fatalf("view does not contain the plain border")
	}

	// 選択モードへ抜けると、同じ Console の枠が別の色になる。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	selectedBorder := model.frameFor(panelConsole).
		render(selectedFrame.topLeft + selectedFrame.horizontal)
	if selectedBorder == focusedBorder {
		t.Fatalf("selected border is not distinguishable from focused")
	}
	if !strings.Contains(model.View().Content, selectedBorder) {
		t.Fatalf("view does not contain the selected border")
	}
}

func TestModelRecordsOnlySuccessfullySentCommands(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	action := Action{Kind: ActionSendCommand, Command: "say hello"}

	_, _ = model.Update(ActionResultMsg{Action: action, Err: errors.New("failed")})
	if model.logs.Len() != 0 {
		t.Fatalf("failed command was recorded")
	}
	_, _ = model.Update(ActionResultMsg{Action: action})
	if model.logs.Len() != 1 || model.logs.At(0) != "say hello" {
		t.Fatalf("logs = %q", model.logs.At(0))
	}
}

func TestModelClearsServerMetricsOnRestart(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = model.Update(MetricsMsg{
		JVM: hsperfdata.Metrics{Heap: hsperfdata.Memory{
			Used: hsperfdata.Number{Value: 10, Available: true},
		}},
		Memory: procstats.Memory{
			RSS: procstats.Number{Value: 20, Available: true},
		},
	})
	_, _ = model.Update(ServerStartedMsg{})

	if model.metrics.Heap.Used.Available || model.memory.RSS.Available ||
		model.samples.Len() != 0 {
		t.Fatalf("old server metrics were retained: %#v", model)
	}
}

func TestModelIgnoresMessagesFromPreviousServer(t *testing.T) {
	model := New(make(chan Action, 1), nil, 2, DefaultSettings())
	_, _ = model.Update(MetricsMsg{
		Generation: 1,
		Memory: procstats.Memory{
			RSS: procstats.Number{Value: 20, Available: true},
		},
	})
	_, _ = model.Update(LogMsg{
		Generation: 1,
		Entry: serverlog.Entry{
			Kind:    serverlog.KindOther,
			Message: "old server",
		},
	})

	if model.memory.RSS.Available || model.logs.Len() != 0 {
		t.Fatalf("old server state was accepted: %#v", model)
	}
}

func TestTailKeepsVisibleRightEdge(t *testing.T) {
	if got := tail("abc日本語", 5); got != "本語" {
		t.Fatalf("tail = %q", got)
	}
}

func TestModelShowsPlayersInTheirOwnPanel(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for _, name := range []string{"alice", "bob", "carol"} {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:   serverlog.KindPlayerJoin,
			Player: name,
		}})
	}

	panelContent := stripANSI(model.renderPlayersPanel())
	for _, name := range []string{"alice", "bob", "carol"} {
		if !strings.Contains(panelContent, name) {
			t.Fatalf("players panel is missing %q:\n%s", name, panelContent)
		}
	}

	// 名前の列挙は Stats 行から消え、人数だけが残る。
	stats := strings.Join(model.statsLines(), "\n")
	if !strings.Contains(stats, "Players 3") {
		t.Fatalf("stats = %q", stats)
	}
	if strings.Contains(stats, "alice") {
		t.Fatalf("stats still lists player names: %q", stats)
	}
}

func TestModelScrollsPlayersFromTheTop(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	viewport := model.layout.playerLines()
	for index := 0; index < viewport+3; index++ {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:   serverlog.KindPlayerJoin,
			Player: fmt.Sprintf("player%02d", index),
		}})
	}
	focusPlayers(t, model)

	// 一覧は先頭から並ぶので、初期状態は先頭が見えている。
	if !strings.Contains(stripANSI(model.renderPlayersPanel()), "player00") {
		t.Fatalf("players panel does not start at the top")
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if model.playerCursor != viewport+2 {
		t.Fatalf("cursor = %d, want %d", model.playerCursor, viewport+2)
	}
	content := stripANSI(model.renderPlayersPanel())
	if strings.Contains(content, "player00") {
		t.Fatalf("scrolled panel still shows the first player:\n%s", content)
	}
	if !strings.Contains(content, fmt.Sprintf("player%02d", viewport+2)) {
		t.Fatalf("panel does not show the last player:\n%s", content)
	}

	// Esc は先頭へ戻す。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.playerCursor != 0 || model.mode != modeSelect {
		t.Fatalf("cursor = %d, mode = %d", model.playerCursor, model.mode)
	}
}

func TestModelPutsPlayerCommandIntoTheConsole(t *testing.T) {
	actions := make(chan Action, 4)
	model := New(actions, nil, 0, DefaultSettings())
	_, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for _, name := range []string{"alice", "bob"} {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:   serverlog.KindPlayerJoin,
			Player: name,
		}})
	}
	focusPlayers(t, model)

	// bob を選んでコマンド一覧へ。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.playerStage != playerStageCommands || model.playerTarget != "bob" {
		t.Fatalf("stage = %d, target = %q", model.playerStage, model.playerTarget)
	}
	// コマンド一覧はモーダルとして重ねて出る。背後のプレイヤー一覧は残る。
	box, _, _ := model.commandModal()
	if !strings.Contains(stripANSI(box), "bob") ||
		!strings.Contains(stripANSI(box), "kick") {
		t.Fatalf("command modal:\n%s", stripANSI(box))
	}
	view := stripANSI(model.View().Content)
	if !strings.Contains(view, "alice") || !strings.Contains(view, "kick") {
		t.Fatalf("view does not show the modal over the player list:\n%s", view)
	}

	// kick を選ぶと、即実行ではなく Console に組み立てられる。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if string(model.input) != "kick bob " {
		t.Fatalf("input = %q", string(model.input))
	}
	if model.panel != panelConsole || model.consoleFocus != consoleInput {
		t.Fatalf("panel = %d, focus = %d", model.panel, model.consoleFocus)
	}
	select {
	case action := <-actions:
		t.Fatalf("command was sent without confirmation: %#v", action)
	default:
	}

	// もう一度 Enter を押してはじめて送信される。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	action := <-actions
	if action.Kind != ActionSendCommand || action.Command != "kick bob" {
		t.Fatalf("action = %#v", action)
	}
}

func TestModelKeepsViewRectangularWithTheCommandModal(t *testing.T) {
	for _, size := range [][2]int{{72, 21}, {80, 24}, {120, 40}} {
		model := newTestModel()
		_, _ = model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for index := 0; index < 6; index++ {
			_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
				Kind:   serverlog.KindPlayerJoin,
				Player: fmt.Sprintf("player%02d", index),
			}})
		}
		focusPlayers(t, model)

		// どのプレイヤー行から開いても、モーダルは画面内に収まる。
		for cursor := 0; cursor < 6; cursor++ {
			model.playerCursor = cursor
			_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

			lines := strings.Split(model.View().Content, "\n")
			if len(lines) != size[1] {
				t.Fatalf("%dx%d cursor %d: height = %d",
					size[0], size[1], cursor, len(lines))
			}
			for index, line := range lines {
				if width := stringWidth(line); width != size[0] {
					t.Fatalf("%dx%d cursor %d: line %d width = %d: %q",
						size[0], size[1], cursor, index, width, stripANSI(line))
				}
			}
			_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		}
	}
}

func TestOverlayReplacesTheTargetSpanOnly(t *testing.T) {
	base := "0123456789\nabcdefghij\nABCDEFGHIJ"

	got := stripANSI(overlay(base, "XX\nYY", 3, 1))

	want := "0123456789\nabcXXfghij\nABCYYFGHIJ"
	if got != want {
		t.Fatalf("overlay = %q, want %q", got, want)
	}
}

func TestOverlayKeepsWidthAcrossWideCharacters(t *testing.T) {
	// 全角文字の途中で切ると、左は文字が落ち、右は文字が残る。
	// どちらの側でも行幅が変わらないことを全位置で確かめる。
	base := "日本語テスト"
	for x := 0; x <= stringWidth(base); x++ {
		got := stripANSI(overlay(base, "XX", x, 0))
		if width := stringWidth(got); width != stringWidth(base) {
			t.Fatalf("x = %d: width = %d, want %d: %q",
				x, width, stringWidth(base), got)
		}
	}
}

func TestOverlayIgnoresRowsOutsideTheBase(t *testing.T) {
	base := "0123\n4567"

	got := stripANSI(overlay(base, "XX\nYY\nZZ", 1, 1))

	// 下地からはみ出す 2 行目以降は捨てられ、行数は増えない。
	if want := "0123\n4XX7"; got != want {
		t.Fatalf("overlay = %q, want %q", got, want)
	}
}

func TestModelLeavesPlayerCommandsWithEscape(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind:   serverlog.KindPlayerJoin,
		Player: "alice",
	}})
	focusPlayers(t, model)

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.playerStage != playerStagePlayers || model.mode != modeFocus {
		t.Fatalf("stage = %d, mode = %d", model.playerStage, model.mode)
	}

	// もう一度 Esc で選択モードへ抜ける。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.mode != modeSelect {
		t.Fatalf("mode = %d", model.mode)
	}
}

func TestModelKeepsPlayerCursorInRangeWhenPlayersLeave(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for _, name := range []string{"alice", "bob", "carol"} {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:   serverlog.KindPlayerJoin,
			Player: name,
		}})
	}
	focusPlayers(t, model)
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})

	for _, name := range []string{"carol", "bob"} {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:   serverlog.KindPlayerLeave,
			Player: name,
		}})
	}
	if model.playerCursor != 0 {
		t.Fatalf("cursor = %d, want 0", model.playerCursor)
	}
	if !strings.Contains(stripANSI(model.renderPlayersPanel()), "alice") {
		t.Fatalf("players panel lost the remaining player")
	}
}

// focusPlayers は Console から Players パネルへフォーカスを移す。
func focusPlayers(t *testing.T, model *Model) {
	t.Helper()
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	for index := 0; index < 3; index++ {
		_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if model.panel != panelPlayers {
		t.Fatalf("panel = %d", model.panel)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.mode != modeFocus {
		t.Fatalf("mode = %d", model.mode)
	}
}

func TestModelViewKeepsRectangularWithThreeTopPanels(t *testing.T) {
	for _, size := range [][2]int{{72, 21}, {80, 24}, {120, 40}} {
		model := newTestModel()
		_, _ = model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		_, _ = model.Update(MetricsMsg{
			JVM: hsperfdata.Metrics{Heap: hsperfdata.Memory{
				Used: hsperfdata.Number{Value: 2 << 30, Available: true},
				Max:  hsperfdata.Number{Value: 4 << 30, Available: true},
			}},
			Memory: procstats.Memory{
				RSS:       procstats.Number{Value: 3 << 30, Available: true},
				HostTotal: procstats.Number{Value: 16 << 30, Available: true},
			},
			CPU:          125,
			CPUAvailable: true,
		})

		content := model.View().Content
		lines := strings.Split(content, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d: height = %d", size[0], size[1], len(lines))
		}
		for index, line := range lines {
			if width := stringWidth(line); width != size[0] {
				t.Fatalf(
					"%dx%d: line %d width = %d: %q",
					size[0], size[1], index, width, stripANSI(line),
				)
			}
		}
		if !strings.Contains(content, "Meters") ||
			!strings.Contains(content, "Players") {
			t.Fatalf("%dx%d: view is missing the new panels", size[0], size[1])
		}
	}
}

func newTestModel() *Model {
	return New(make(chan Action, 8), nil, 0, DefaultSettings())
}

func stripANSI(value string) string {
	return ansi.Strip(value)
}

func containsBraille(value string) bool {
	for _, character := range value {
		if character >= 0x2800 && character <= 0x28ff {
			return true
		}
	}
	return false
}

func TestStatsRSSLineKeepsDeltaOnNarrowTerminals(t *testing.T) {
	model := newTestModel()
	model.memory = procstats.Memory{
		RSS:       procstats.Number{Value: 5400 << 20, Available: true},
		HostTotal: procstats.Number{Value: 16 << 30, Available: true},
	}
	model.metrics = hsperfdata.Metrics{Heap: hsperfdata.Memory{
		Committed: hsperfdata.Number{Value: 2100 << 20, Available: true},
	}}

	// 最小幅では分母を落としてでも Δ を残す。Δ はこのツールの核心。
	model.resize(minimumWidth, minimumHeight)
	narrow := model.rssLine()
	if stringWidth(narrow) > model.layout.statsWidth-2 {
		t.Fatalf("narrow line = %q (%d 桁)", narrow, stringWidth(narrow))
	}
	if !strings.Contains(narrow, "Δ +3.2G") {
		t.Fatalf("narrow line = %q", narrow)
	}

	// 広ければ分母も出す。
	model.resize(110, 30)
	wide := stripANSI(model.rssLine())
	if !strings.Contains(wide, "16.0G total") ||
		!strings.Contains(wide, "Δ +3.2G") {
		t.Fatalf("wide line = %q", wide)
	}

	// cgroup 制限があるときは、割合の分母をそちらに切り替えて隣に置く。
	model.memory.CgroupLimit = procstats.Limit{Value: 8 << 30, Available: true}
	limited := stripANSI(model.rssLine())
	if !strings.Contains(limited, "8.0G limit (66%)") {
		t.Fatalf("limited line = %q", limited)
	}
}

func TestGraphRangeAddsMarginAndReportsUnavailable(t *testing.T) {
	model := newTestModel()
	model.resize(110, 30)
	if _, _, ok := model.graphRange(); ok {
		t.Fatal("range is available without samples")
	}

	// 1 点しかなくても高さ 0 の範囲は返さない。
	model.samples.Add(memorySample{heap: 1 << 30, heapKnown: true})
	low, high, ok := model.graphRange()
	if !ok || low >= high {
		t.Fatalf("low = %d, high = %d, ok = %t", low, high, ok)
	}

	// 余白が値より大きくても下限は 0 で飽和する。
	model.samples.SetLimit(0)
	model.samples.SetLimit(4)
	model.samples.Add(memorySample{heap: 1, heapKnown: true})
	if low, _, ok := model.graphRange(); !ok || low != 0 {
		t.Fatalf("low = %d, ok = %t", low, ok)
	}
}
