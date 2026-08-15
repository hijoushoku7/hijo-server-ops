package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/gclog"
	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/javaenv"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

func TestModelBoundsLogHistoryAndStoredRecords(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	for index := 0; index < historyLines+2; index++ {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind: serverlog.KindOther,
			Message: strings.Repeat("x", maxLogRecordWidth+100) +
				string(rune('a'+index%26)),
		}})
	}

	if model.logs.Len() != historyLines {
		t.Fatalf("logs = %d, limit = %d", model.logs.Len(), historyLines)
	}
	if window := model.logs.Window(modelLogViewport(model)); len(window) !=
		model.layout.logLines() {
		t.Fatalf("window = %d, viewport = %d", len(window), model.layout.logLines())
	}
	if width := stringWidth(model.logs.At(0).line()); width != maxLogRecordWidth {
		t.Fatalf("line width = %d, want %d", width, maxLogRecordWidth)
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
	timestamp := time.Date(0, time.January, 1, 12, 34, 56, 0, time.UTC)
	entries := []serverlog.Entry{
		{
			Kind:            serverlog.KindChat,
			Timestamp:       timestamp,
			TimestampSource: serverlog.TimestampLog,
			Player:          "alice",
			Chat:            "hello",
		},
		{Kind: serverlog.KindCommand, Player: "alice", Command: "/time set day"},
		{Kind: serverlog.KindPlayerJoin, Player: "alice", Message: "alice joined the game"},
		{Kind: serverlog.KindLag, Message: "Can't keep up!"},
	}
	for _, entry := range entries {
		_, _ = model.Update(LogMsg{Entry: entry})
	}

	if model.chat.At(0).line() != "<alice> hello" {
		t.Fatalf("chat = %q", model.chat.At(0).line())
	}
	chat := model.chat.At(0)
	if chat.timestamp != timestamp ||
		chat.timestampSource != serverlog.TimestampLog ||
		chat.kind != serverlog.KindChat || chat.player != "alice" ||
		chat.text != "hello" {
		t.Fatalf("chat record = %#v", chat)
	}
	// コマンドは専用ペインをやめて Log に流している。
	if model.logs.At(0).line() != "alice: /time set day" {
		t.Fatalf("command = %q", model.logs.At(0).line())
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

func TestModelKeepsHistoryWhenTerminalBecomesTooSmall(t *testing.T) {
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

	if model.logs.Len() != 1 || model.logs.At(0).line() != "log" {
		t.Fatalf("log history was discarded: %#v", model.logs)
	}
	if model.logs.Limit() != historyLines || model.chat.Limit() != historyLines {
		t.Fatalf("history limits = %d, %d", model.logs.Limit(), model.chat.Limit())
	}
	if model.samples.samples != nil {
		t.Fatalf("graph cache was retained: %#v", model.samples)
	}

	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(model.View().Content, "log") {
		t.Fatalf("restored view does not contain retained history")
	}
}

func TestModelRestoresFullLogLineAfterResize(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	line := "start-" + strings.Repeat("x", 50) + "-restored"
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind:    serverlog.KindOther,
		Message: line,
	}})

	_, _ = model.Update(tea.WindowSizeMsg{Width: minimumWidth, Height: 24})
	if strings.Contains(model.View().Content, line) {
		t.Fatalf("narrow view unexpectedly contains the full line")
	}
	_, _ = model.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	if !strings.Contains(model.View().Content, line) {
		t.Fatalf("expanded view did not restore the full line")
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
		!strings.Contains(model.logs.At(1).line(), "heap recovered") {
		t.Fatalf("logs = %q, %q", model.logs.At(0).line(), model.logs.At(1).line())
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
		model.logs.At(0).line(), "public IPv4 unavailable",
	) {
		t.Fatalf("logs = %q", model.logs.At(0).line())
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
	startedAt := time.Date(2026, time.August, 6, 10, 0, 0, 0, time.UTC)
	exitedAt := startedAt.Add(90 * time.Second)
	_, command := model.Update(ProcessExitedMsg{
		Err:       want,
		ExitCode:  1,
		StartedAt: startedAt,
		ExitedAt:  exitedAt,
	})

	if !errors.Is(model.Err(), want) {
		t.Fatalf("Err = %v", model.Err())
	}
	if command != nil {
		t.Fatalf("unexpected command = %T", command())
	}
	if model.exit == nil || !model.exit.crashed || model.exit.exitCode != 1 ||
		!model.exit.uptime.Available ||
		model.exit.uptime.Value != 90*time.Second || model.exit.button != 0 {
		t.Fatalf("exit = %#v", model.exit)
	}
}

func TestModelSnapshotsExitMetricsAndErrorLines(t *testing.T) {
	model := newTestModel()
	model.resize(80, 24)
	_, _ = model.Update(MetricsMsg{
		JVM: hsperfdata.Metrics{Heap: hsperfdata.Memory{
			Used:      hsperfdata.Number{Value: 10, Available: true},
			Committed: hsperfdata.Number{Value: 20, Available: true},
		}},
		Memory: procstats.Memory{
			RSS: procstats.Number{Value: 30, Available: true},
		},
	})
	model.gcStats.Collections = gclog.Count{Value: 4, Available: true}
	for _, line := range []string{
		"old ERROR one",
		"ignored info",
		"Exception two",
		"FATAL three",
		"Caused by four",
	} {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind: serverlog.KindOther, Message: line,
		}})
	}

	wantErr := errors.New("process failed")
	_, _ = model.Update(ProcessExitedMsg{Err: wantErr, ExitCode: 7})
	state := model.exit
	if state.snapshot.heap.Used.Value != 10 ||
		state.snapshot.heap.Committed.Value != 20 ||
		state.snapshot.rss.Value != 30 || state.snapshot.gc.Collections.Value != 4 {
		t.Fatalf("snapshot = %#v", state.snapshot)
	}
	wantLines := []string{
		"process failed", "Exception two", "FATAL three", "Caused by four",
	}
	if !slices.Equal(state.errorLines, wantLines) {
		t.Fatalf("errorLines = %#v, want %#v", state.errorLines, wantLines)
	}

	// 終了後に元の状態が変わっても、モーダルのスナップショットは変えない。
	_, _ = model.Update(MetricsMsg{Memory: procstats.Memory{
		RSS: procstats.Number{Value: 99, Available: true},
	}})
	if state.snapshot.rss.Value != 30 {
		t.Fatalf("snapshot RSS = %d", state.snapshot.rss.Value)
	}
}

func TestModelNormalExitCountsDownAndAnyKeyCancelsIt(t *testing.T) {
	model := newTestModel()
	_, command := model.Update(ProcessExitedMsg{ExitCode: 0})
	if command == nil || model.exit == nil || model.exit.crashed ||
		model.exit.autoQuitAt.IsZero() {
		t.Fatalf("exit = %#v, command = %T", model.exit, command)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !model.exit.autoQuitAt.IsZero() || model.settingsOpen || model.exit.closed {
		t.Fatalf("key did not only cancel countdown: %#v", model.exit)
	}
	_, command = model.Update(exitCountdownMsg{state: model.exit})
	if command != nil {
		t.Fatalf("cancelled countdown returned command %T", command)
	}
}

func TestModelFatalMessageOpensCrashModalWithoutQuitting(t *testing.T) {
	model := newTestModel()
	model.busy = true
	want := errors.New("fatal monitor error")
	_, command := model.Update(FatalMsg{Err: want})
	if command != nil || model.exit == nil || !model.exit.crashed ||
		model.exit.exitCode != unknownExitCode || model.busy ||
		!errors.Is(model.Err(), want) {
		t.Fatalf("exit = %#v, Err = %v, command = %T",
			model.exit, model.Err(), command)
	}
}

func TestModelNormalExitCountdownQuitsAfterDeadline(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(ProcessExitedMsg{})
	model.exit.autoQuitAt = time.Now().Add(-time.Second)
	_, command := model.Update(exitCountdownMsg{state: model.exit})
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("command = %T", command())
	}
}

func TestModelExitModalKeysAndStoppedMode(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 1, DefaultSettings())
	model.resize(80, 24)
	_, _ = model.Update(ProcessExitedMsg{Err: errors.New("crashed"), ExitCode: 1})

	// モーダルの既定はログで、G を含む無関係なキーは設定へ流さない。
	if model.exit.button != 0 {
		t.Fatalf("button = %d", model.exit.button)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if model.exit.button != 1 {
		t.Fatalf("button = %d", model.exit.button)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if model.settingsOpen || model.exit.button != 1 {
		t.Fatalf("modal leaked key: settings = %t, button = %d",
			model.settingsOpen, model.exit.button)
	}

	model.exit.button = 0
	_, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if command != nil || !model.exit.closed || model.panel != panelLog {
		t.Fatalf("exit = %#v, panel = %d, command = %T",
			model.exit, model.panel, command)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if action := <-actions; action.Kind != ActionRestart {
		t.Fatalf("action = %#v", action)
	}
	_, command = model.Update(tea.KeyPressMsg{Code: 'Q', Text: "Q"})
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("command = %T", command())
	}
}

func TestModelExitViewIsFullScreenLogAndRectangular(t *testing.T) {
	model := newTestModel()
	model.resize(72, 21)
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind: serverlog.KindOther, Message: "last useful log",
	}})
	_, _ = model.Update(ProcessExitedMsg{Err: errors.New("crashed"), ExitCode: 1})

	content := stripANSI(model.View().Content)
	if !strings.Contains(content, "Log · ") ||
		!strings.Contains(content, "exit 1") ||
		!strings.Contains(content, "last useful log") {
		t.Fatalf("stopped view:\n%s", content)
	}
	for _, title := range []string{"Meters", "Players", "Chat", "Console"} {
		if strings.Contains(content, title) {
			t.Fatalf("stopped view still contains %s:\n%s", title, content)
		}
	}
	lines := strings.Split(model.View().Content, "\n")
	if len(lines) != 21 {
		t.Fatalf("height = %d", len(lines))
	}
	for index, line := range lines {
		if width := stringWidth(line); width != 72 {
			t.Fatalf("line %d width = %d: %q", index, width, stripANSI(line))
		}
	}
}

func TestModelServerStartedClearsExitAndUpdatesRestartTracker(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(ProcessExitedMsg{Err: errors.New("crashed"), ExitCode: 1})
	startedAt := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	_, _ = model.Update(ServerStartedMsg{Generation: 2, StartedAt: startedAt})
	if model.exit != nil || model.restart.startedAt != startedAt || model.generation != 2 {
		t.Fatalf("model = %#v", model)
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
	logViewport := modelLogViewport(model)
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
	if model.logs.Offset(logViewport) != viewport {
		t.Fatalf("offset = %d, viewport = %d", model.logs.Offset(logViewport), viewport)
	}
	top := model.logs.Window(logViewport)[0].record.line()

	// 遡っている間は新着で表示が流れない。
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind:    serverlog.KindOther,
		Message: "newest",
	}})
	if got := model.logs.Window(logViewport)[0].record.line(); got != top {
		t.Fatalf("window shifted: %q -> %q", top, got)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if model.logs.Offset(logViewport) != 0 {
		t.Fatalf("offset = %d", model.logs.Offset(logViewport))
	}
	window := model.logs.Window(logViewport)
	if window[len(window)-1].record.line() != "newest" {
		t.Fatalf("tail = %q", window[len(window)-1].record.line())
	}
}

func TestModelReturnsToLatestWhenFocusIsReleased(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	viewport := model.layout.logLines()
	logViewport := modelLogViewport(model)
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
	if model.logs.Offset(logViewport) == 0 {
		t.Fatalf("buffer did not scroll back")
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.mode != modeSelect {
		t.Fatalf("mode = %d", model.mode)
	}
	if model.logs.Offset(logViewport) != 0 {
		t.Fatalf("offset = %d, want 0", model.logs.Offset(logViewport))
	}
	if !strings.Contains(model.View().Content, "line "+fmt.Sprint(viewport*3-1)) {
		t.Fatalf("view does not show the latest line")
	}
}

func TestBufferPanelIndicatorCountsWrappedDisplayLines(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for index := 0; index < 6; index++ {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:    serverlog.KindOther,
			Message: strings.Repeat(fmt.Sprintf("record%d ", index), 20),
		}})
	}
	viewport := modelLogViewport(model)
	model.logs.ScrollToStart(viewport)
	offset := model.logs.Offset(viewport)
	if offset <= model.logs.Len() {
		t.Fatalf("display-line offset = %d, records = %d", offset, model.logs.Len())
	}

	panel := stripANSI(model.renderBufferPanel(
		panelLog,
		&model.logs,
		model.layout.rightWidth,
		model.layout.bodyHeight,
	))
	if !strings.Contains(panel, fmt.Sprintf("Log ↑%d", offset)) {
		t.Fatalf("panel title does not contain display-line offset %d:\n%s", offset, panel)
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

// TestKeybarFitsMinimumWidth はキー説明が最小端末幅に収まるかを見る。
// keybar は fitLine で切り詰めるので、はみ出してもエラーにはならず末尾が
// 黙って欠けるだけになる。日本語版はラベルが全角で幅が伸びるため、
// 文言を足したり訳し直したりしたときにここで気付けるようにする。
func TestKeybarFitsMinimumWidth(t *testing.T) {
	// 切り詰めが起きない広さで測り、fitLine が右へ足した空白は落とす。
	const wide = 200

	focusPlayerCommands := func(t *testing.T, model *Model) {
		t.Helper()
		for _, name := range []string{"alice", "bob"} {
			_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
				Kind:   serverlog.KindPlayerJoin,
				Player: name,
			}})
		}
		focusPlayers(t, model)
		_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if model.playerStage != playerStageCommands {
			t.Fatalf("stage = %d", model.playerStage)
		}
	}

	cases := map[string]func(*testing.T, *Model){
		"console": func(*testing.T, *Model) {},
		"select": func(t *testing.T, model *Model) {
			_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		},
		"logs": func(t *testing.T, model *Model) {
			_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		},
		"players":        focusPlayers,
		"playerCommands": focusPlayerCommands,
		"settings": func(t *testing.T, model *Model) {
			_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			_, _ = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
			if !model.settingsOpen {
				t.Fatal("settings did not open")
			}
		},
		"timeModal": func(_ *testing.T, model *Model) {
			model.settingsOpen = true
			model.timeModal = &timeModalState{}
		},
		"exitModal": func(_ *testing.T, model *Model) {
			model.exit = &exitState{}
		},
		"exitLogs": func(_ *testing.T, model *Model) {
			model.exit = &exitState{closed: true}
		},
	}

	for name, focus := range cases {
		model := newTestModel()
		_, _ = model.Update(tea.WindowSizeMsg{Width: wide, Height: 40})
		focus(t, model)

		bar := strings.TrimRight(model.keybar(), " ")
		if width := stringWidth(bar); width > minimumWidth {
			t.Errorf(
				"%s のキー説明が %d 桁で、最小端末幅 %d に収まらない: %q",
				name,
				width,
				minimumWidth,
				stripANSI(bar),
			)
		}
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
	if model.logs.Len() != 1 || model.logs.At(0).line() != "say hello" {
		t.Fatalf("logs = %q", model.logs.At(0).line())
	}
}

func TestModelRecordsSentWhisperInChatAndLog(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	action := Action{Kind: ActionSendCommand, Command: "tell bob hi"}

	_, _ = model.Update(ActionResultMsg{Action: action})

	if model.chat.Len() != 1 || model.logs.Len() != 1 {
		t.Fatalf("chat = %d, logs = %d", model.chat.Len(), model.logs.Len())
	}
	chat := model.chat.At(0)
	log := model.logs.At(0)
	if chat.line() != "<→ bob> hi" {
		t.Fatalf("chat = %q", chat.line())
	}
	if log.line() != "tell bob hi" {
		t.Fatalf("log = %q", log.line())
	}
	if !chat.timestamp.Equal(log.timestamp) ||
		chat.timestampSource != log.timestampSource {
		t.Fatalf("timestamps differ: chat = %#v, log = %#v", chat, log)
	}
}

func TestModelDoesNotRecordServerCommandInChat(t *testing.T) {
	lines := []string{
		"[12:34:56] [Server thread/INFO]: [alice: Stopping the server]",
		"[12:34:56] [Server thread/INFO]: " +
			"alice issued server command: tell bob hi",
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			model := newTestModel()
			_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

			_, _ = model.Update(LogMsg{Entry: serverlog.Parse(line)})

			if model.chat.Len() != 0 {
				t.Fatalf("chat = %q", model.chat.At(0).line())
			}
			if model.logs.Len() != 1 {
				t.Fatalf("logs = %d", model.logs.Len())
			}
		})
	}
}

func TestModelDoesNotRecordSentWhisperWithoutBodyInChat(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	action := Action{Kind: ActionSendCommand, Command: "tell bob"}

	_, _ = model.Update(ActionResultMsg{Action: action})

	if model.chat.Len() != 0 {
		t.Fatalf("chat = %q", model.chat.At(0).line())
	}
	if model.logs.Len() != 1 || model.logs.At(0).line() != "tell bob" {
		t.Fatalf("logs = %#v", model.logs)
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

func modelLogViewport(model *Model) bufferViewport {
	return bufferViewport{
		width:  model.layout.rightContentWidth(),
		height: model.layout.logLines(),
	}
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

func TestModelRestartAnimatesUntilServerStarts(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 1, DefaultSettings())
	model.resize(80, 24)
	model.consoleFocus = consoleRestart

	_, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action := <-actions; action.Kind != ActionRestart {
		t.Fatalf("action = %#v", action)
	}
	if model.restartPhase == 0 || command == nil {
		t.Fatalf("restartPhase = %d, command = %T", model.restartPhase, command)
	}
	// 点は 3 桁に揃え、コンソールのボタン幅を揺らさない。
	if console := stripANSI(model.consoleLine()); !strings.Contains(console, "[restarting.  ]") {
		t.Fatalf("console = %q", console)
	}
	_, _ = model.Update(restartTickMsg{})
	if console := stripANSI(model.consoleLine()); !strings.Contains(console, "[restarting.. ]") {
		t.Fatalf("console = %q", console)
	}

	_, _ = model.Update(ServerStartedMsg{Generation: 2, StartedAt: time.Now()})
	if model.restartPhase != 0 {
		t.Fatal("animation outlived the restart")
	}
	if console := stripANSI(model.consoleLine()); !strings.Contains(console, "[restart]") {
		t.Fatalf("console = %q", console)
	}
	// 止まった後の tick では再武装しない。
	if _, command := model.Update(restartTickMsg{}); command != nil {
		t.Fatalf("command = %T", command)
	}
}

func TestModelExitModalShowsRestartProgressAndFailure(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 1, DefaultSettings())
	model.resize(80, 24)
	_, _ = model.Update(ProcessExitedMsg{Err: errors.New("crashed"), ExitCode: 1})

	model.exit.button = 1
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action := <-actions; action.Kind != ActionRestart {
		t.Fatalf("action = %#v", action)
	}
	box, _, _ := model.exitModal()
	if !strings.Contains(stripANSI(box), msg.ExitStateRestarting(model.restartDots())) {
		t.Fatalf("modal = %q", stripANSI(box))
	}

	// 失敗はモーダルへ出す。status 行がないので、ここで出さないと消える。
	_, _ = model.Update(ActionResultMsg{
		Action: Action{Kind: ActionRestart},
		Err:    errors.New("no server"),
	})
	if model.restartPhase != 0 {
		t.Fatal("animation outlived the failure")
	}
	box, _, _ = model.exitModal()
	if !strings.Contains(stripANSI(box), "no server") {
		t.Fatalf("modal = %q", stripANSI(box))
	}
}

func TestModelExitErrorLinesStayWithinCurrentGeneration(t *testing.T) {
	model := newTestModel()
	model.resize(80, 24)
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind: serverlog.KindOther, Message: "[Server thread/ERROR]: 前世代の残骸",
	}})

	// 再起動でログは消さないので、世代の境目を覚えていないと前の
	// クラッシュの ERROR を今回の原因として出してしまう。
	_, _ = model.Update(ServerStartedMsg{Generation: 2, StartedAt: time.Now()})
	_, _ = model.Update(LogMsg{Generation: 2, Entry: serverlog.Entry{
		Kind: serverlog.KindOther, Message: "Done (12.114s)!",
	}})
	_, _ = model.Update(ProcessExitedMsg{Generation: 2, ExitCode: 1})

	joined := strings.Join(model.exit.errorLines, "\n")
	if strings.Contains(joined, "前世代の残骸") {
		t.Fatalf("errorLines = %q", joined)
	}
	if !strings.Contains(joined, "Done (12.114s)!") {
		t.Fatalf("errorLines = %q", joined)
	}
}

func TestModelGuidesJavaMismatchFromCurrentGeneration(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 1))
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind: serverlog.KindOther,
		Message: "UnsupportedClassVersionError: old class file version 69.0, " +
			"class file versions up to 65.0",
	}})
	_, _ = model.Update(ServerStartedMsg{Generation: 2, StartedAt: time.Now()})

	for _, line := range []string{
		"java.lang.UnsupportedClassVersionError: Example has been compiled by a more recent version",
		"of the Java Runtime (class file version 65.0),",
		"this version only recognizes class file versions up to 61.0",
	} {
		_, _ = model.Update(LogMsg{Generation: 2, Entry: serverlog.Entry{
			Kind: serverlog.KindOther, Message: line,
		}})
	}
	crash(model, errors.New("some other exit error"), time.Second)

	if !model.exit.javaMismatch {
		t.Fatalf("exit = %#v", model.exit)
	}
	if model.exit.autoRestart || model.exit.notice != msg.ExitAutoRestartJavaMismatch {
		t.Fatalf("auto restart was not suppressed: %#v", model.exit)
	}
	if len(model.exit.errorLines) < 3 ||
		!strings.Contains(model.exit.errorLines[0], "21") ||
		!strings.Contains(model.exit.errorLines[0], "17") ||
		!strings.Contains(model.exit.errorLines[1], "hso java change") ||
		model.exit.errorLines[2] != "some other exit error" {
		t.Fatalf("errorLines = %q", model.exit.errorLines)
	}
	joined := strings.Join(model.exit.errorLines, "\n")
	if strings.Contains(joined, "Java 25") {
		t.Fatalf("previous generation was included: %q", joined)
	}

	model.resize(40, 20)
	box, _, _ := model.exitModal()
	for index, line := range strings.Split(box, "\n") {
		if width := stringWidth(line); width > 36 {
			t.Fatalf("wrapped modal line %d width = %d: %q", index, width, stripANSI(line))
		}
	}
}

func TestJavaMismatchGuidanceUsesInstalledGeneration(t *testing.T) {
	mismatch := javaenv.ClassVersionError{Required: 21}
	installed := javaMismatchGuidance(mismatch, []javaenv.Installation{{Major: 21}})
	if len(installed) != 2 || !strings.Contains(installed[0], msg.JavaVersionMismatch(21, 0)) ||
		installed[1] != msg.JavaVersionChange(21) {
		t.Fatalf("installed guidance = %q", installed)
	}
	if strings.Contains(strings.Join(installed, "\n"), msg.JavaVersionInstall(21)) {
		t.Fatalf("installed guidance asks for installation: %q", installed)
	}

	missing := javaMismatchGuidance(mismatch, nil)
	if len(missing) != 2 || missing[1] != msg.JavaVersionInstall(21) {
		t.Fatalf("missing guidance = %q", missing)
	}
}

func TestModelIgnoresJavaMismatchFromPreviousGeneration(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind:    serverlog.KindOther,
		Message: "UnsupportedClassVersionError: class file version 65.0, class file versions up to 61.0",
	}})
	_, _ = model.Update(ServerStartedMsg{Generation: 2, StartedAt: time.Now()})
	_, _ = model.Update(ProcessExitedMsg{Generation: 2, Err: errors.New("crashed"), ExitCode: 1})
	if model.exit.javaMismatch {
		t.Fatalf("previous generation caused mismatch: %#v", model.exit)
	}
}

func TestModelFatalExitDoesNotReuseStaleUptimeAndMemory(t *testing.T) {
	model := newTestModel()
	model.resize(80, 24)
	model.restart.startedAt = time.Now().Add(-3 * time.Hour)
	model.memory.RSS = procstats.Number{Value: 5 << 30, Available: true}

	// 起動に失敗した世代には稼働時間もメモリもない。前の値を流用すると、
	// ログを読んでいた時間まで uptime に足された嘘になる。
	_, _ = model.Update(FatalMsg{Err: errors.New("start the start script")})
	if model.exit.uptime.Available || model.exit.snapshot.rss.Available {
		t.Fatalf("exit = %#v", model.exit)
	}
	box, _, _ := model.exitModal()
	if strings.Count(stripANSI(box), "n/a") < 2 {
		t.Fatalf("modal = %q", stripANSI(box))
	}
}

func TestModelClearsRunErrorAfterSuccessfulRestart(t *testing.T) {
	model := newTestModel()
	model.resize(80, 24)
	_, _ = model.Update(ProcessExitedMsg{Err: errors.New("crashed"), ExitCode: 1})
	if model.Err() == nil {
		t.Fatal("crash did not set the run error")
	}

	// 立ち上がり直した時点で復旧。以後の正常停止で hso は成功で終わる。
	_, _ = model.Update(ServerStartedMsg{Generation: 2, StartedAt: time.Now()})
	if model.Err() != nil {
		t.Fatalf("err = %v", model.Err())
	}
	_, _ = model.Update(ProcessExitedMsg{Generation: 2})
	if model.Err() != nil {
		t.Fatalf("err after normal stop = %v", model.Err())
	}
}

func TestModelExitKeepsLastKnownMemoryAndSkipsFallbackOnNormalStop(t *testing.T) {
	model := newTestModel()
	model.resize(80, 24)
	_, _ = model.Update(MetricsMsg{
		JVM: hsperfdata.Metrics{Heap: hsperfdata.Memory{
			Used:      hsperfdata.Number{Value: 3 << 30, Available: true},
			Committed: hsperfdata.Number{Value: 4 << 30, Available: true},
		}},
		Memory: procstats.Memory{
			RSS: procstats.Number{Value: 5 << 30, Available: true},
		},
	})
	// プロセスが消えてから終了を検知するまでの隙間で走る、全部 n/a の採取。
	_, _ = model.Update(MetricsMsg{})
	_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
		Kind: serverlog.KindOther, Message: "[Server thread/INFO]: Saving worlds",
	}})
	_, _ = model.Update(ProcessExitedMsg{})

	if !model.exit.snapshot.rss.Available || !model.exit.snapshot.heap.Used.Available {
		t.Fatalf("snapshot = %#v", model.exit.snapshot)
	}
	// 正常停止では、拾えるエラーがないのに末尾で埋めない。
	if len(model.exit.errorLines) != 0 {
		t.Fatalf("errorLines = %q", model.exit.errorLines)
	}
	if box := stripANSI(mustModal(t, model)); strings.Contains(box, msg.ExitErrorLines) {
		t.Fatalf("modal = %q", box)
	}
}

func mustModal(t *testing.T, model *Model) string {
	t.Helper()
	box, _, _ := model.exitModal()
	return box
}

func TestModelExitKeepsFatalCauseOverLaterProcessExit(t *testing.T) {
	model := newTestModel()
	model.resize(80, 24)

	// java を見つけられないと hso が原因を出してから SIGTERM を送る。
	// 続けて届く終了通知で、その原因を上書きしてはいけない。
	_, _ = model.Update(FatalMsg{Err: errors.New("java プロセスを見つけられません")})
	_, _ = model.Update(ProcessExitedMsg{
		Err:      errors.New("exit status 143"),
		ExitCode: 143,
	})

	if !model.exit.crashed {
		t.Fatalf("exit = %#v", model.exit)
	}
	joined := strings.Join(model.exit.errorLines, "\n")
	if !strings.Contains(joined, "見つけられません") {
		t.Fatalf("errorLines = %q", joined)
	}
	// 終了コードや稼働時間は新しい通知の値を使う。
	if model.exit.exitCode != 143 {
		t.Fatalf("exitCode = %d", model.exit.exitCode)
	}
}

func TestModelKeepsPartiallyAvailableHeapForExitModal(t *testing.T) {
	model := newTestModel()
	model.resize(80, 24)
	_, _ = model.Update(MetricsMsg{JVM: hsperfdata.Metrics{Heap: hsperfdata.Memory{
		Used:      hsperfdata.Number{Value: 3 << 30, Available: true},
		Committed: hsperfdata.Number{Value: 4 << 30, Available: true},
	}}})
	// カウンタの一部だけ読めない採取。まとめて置き換えると committed が消える。
	_, _ = model.Update(MetricsMsg{JVM: hsperfdata.Metrics{Heap: hsperfdata.Memory{
		Used: hsperfdata.Number{Value: 2 << 30, Available: true},
	}}})
	_, _ = model.Update(ProcessExitedMsg{ExitCode: 1})

	heap := model.exit.snapshot.heap
	if !heap.Committed.Available || heap.Committed.Value != 4<<30 {
		t.Fatalf("committed = %#v", heap.Committed)
	}
	if heap.Used.Value != 2<<30 {
		t.Fatalf("used = %#v", heap.Used)
	}
}

func newAutoRestartModel(actions chan Action) *Model {
	settings := DefaultSettings()
	settings.AutoRestart = true
	model := New(actions, nil, 0, settings)
	model.resize(80, 24)
	return model
}

func crash(model *Model, err error, uptime time.Duration) {
	exitedAt := time.Now()
	_, _ = model.Update(ProcessExitedMsg{
		Err:       err,
		ExitCode:  1,
		StartedAt: exitedAt.Add(-uptime),
		ExitedAt:  exitedAt,
	})
}

func stopNormally(model *Model, uptime time.Duration) {
	exitedAt := time.Now()
	_, _ = model.Update(ProcessExitedMsg{
		StartedAt: exitedAt.Add(-uptime),
		ExitedAt:  exitedAt,
	})
}

func TestModelAutoRestartKeepsDashboardAndRequestsRestart(t *testing.T) {
	actions := make(chan Action, 1)
	model := newAutoRestartModel(actions)
	crash(model, errors.New("crashed"), 2*time.Hour)

	if !model.exit.autoRestart || model.exit.autoRestartAt.IsZero() {
		t.Fatalf("exit = %#v", model.exit)
	}
	// 勝手に戻ってくる画面なので、背景はダッシュボードのまま残す。
	content := stripANSI(model.View().Content)
	if !strings.Contains(content, "Console") ||
		!strings.Contains(content, msg.ExitTitleCrashed) ||
		strings.Contains(content, "[ "+msg.ExitButtonRestart+" ]") {
		t.Fatalf("view:\n%s", content)
	}

	model.exit.autoRestartAt = time.Now().Add(-time.Second)
	state := model.exit
	_, command := model.Update(autoRestartMsg{state: state})
	if command == nil {
		t.Fatal("no restart command")
	}
	if action := <-actions; action.Kind != ActionRestart {
		t.Fatalf("action = %#v", action)
	}
	if model.restartPhase == 0 {
		t.Fatalf("restartPhase = %d", model.restartPhase)
	}

	// 立ち直っても、落ちたことを読ませるためモーダルは Enter まで残す。
	_, _ = model.Update(ServerStartedMsg{Generation: 1, StartedAt: time.Now()})
	if model.exit == nil || !model.exit.restarted {
		t.Fatalf("exit = %#v", model.exit)
	}
	content = stripANSI(model.View().Content)
	if !strings.Contains(content, msg.ExitAutoRestartDone) ||
		!strings.Contains(content, "Console") {
		t.Fatalf("view:\n%s", content)
	}
	// Enter 以外では消えない。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.exit == nil {
		t.Fatal("escape closed the modal")
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.exit != nil {
		t.Fatalf("exit = %#v", model.exit)
	}
}

func TestModelAutoRestartGivesUpOnCrashLoop(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 4))
	for attempt := range shortRunGiveUp - 1 {
		crash(model, errors.New("crashed"), time.Second)
		if !model.exit.autoRestart {
			t.Fatalf("attempt %d did not auto restart: %#v", attempt, model.exit)
		}
		_, _ = model.Update(ServerStartedMsg{Generation: 1, StartedAt: time.Now()})
	}

	crash(model, errors.New("crashed"), time.Second)
	if model.exit.autoRestart || model.exit.notice != msg.ExitAutoRestartStopped {
		t.Fatalf("exit = %#v", model.exit)
	}
	// 打ち切った後はログ全面へ戻し、三択を出す。
	content := stripANSI(model.View().Content)
	if !strings.Contains(content, "[ "+msg.ExitButtonRestart+" ]") ||
		!strings.Contains(content, msg.ExitAutoRestartStopped) {
		t.Fatalf("view:\n%s", content)
	}
}

func TestModelAutoRestartForgetsCrashesAfterLongRun(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 4))
	for range shortRunGiveUp - 1 {
		crash(model, errors.New("crashed"), time.Second)
		_, _ = model.Update(ServerStartedMsg{Generation: 1, StartedAt: time.Now()})
	}
	crash(model, errors.New("crashed"), shortRunLimit+time.Second)
	if len(model.restart.attempts) != 0 || !model.exit.autoRestart {
		t.Fatalf("attempts = %d, exit = %#v",
			len(model.restart.attempts), model.exit)
	}
}

// 立ち直りの判定は終了の種類を見ない。長くもった世代を正常停止で挟んでも、
// その前のクラッシュが次の 1 回目と合算されてはいけない。
func TestModelAutoRestartForgetsCrashesAfterLongNormalRun(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 4))
	for range shortRunGiveUp - 1 {
		crash(model, errors.New("crashed"), time.Second)
		_, _ = model.Update(ServerStartedMsg{Generation: 1, StartedAt: time.Now()})
	}

	stopNormally(model, shortRunLimit+time.Second)
	if len(model.restart.attempts) != 0 {
		t.Fatalf("attempts = %d", len(model.restart.attempts))
	}
	_, _ = model.Update(ServerStartedMsg{Generation: 2, StartedAt: time.Now()})

	crash(model, errors.New("crashed"), time.Second)
	if !model.exit.autoRestart || model.exit.notice != "" {
		t.Fatalf("exit = %#v", model.exit)
	}
}

// 短時間で止めた正常停止は、クラッシュの回数を増やしも減らしもしない。
func TestModelShortNormalStopDoesNotCountAsCrash(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 4))
	crash(model, errors.New("crashed"), time.Second)
	_, _ = model.Update(ServerStartedMsg{Generation: 1, StartedAt: time.Now()})

	stopNormally(model, time.Second)
	if len(model.restart.attempts) != 1 {
		t.Fatalf("attempts = %d", len(model.restart.attempts))
	}
}

// 起動スクリプトが java を立てないのは設定の誤りなので、待って叩き直しても
// 直らない。1 回で人へ渡す。
func TestModelAutoRestartSkipsScriptWithoutJava(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 1))
	crash(model, msg.ErrScriptExitedWithoutJava, time.Second)
	if model.exit.autoRestart || model.exit.notice != msg.ExitAutoRestartSkipped {
		t.Fatalf("exit = %#v", model.exit)
	}
}

func TestModelAutoRestartIgnoredWhenDisabledOrFatal(t *testing.T) {
	model := newTestModel()
	crash(model, errors.New("crashed"), time.Second)
	if model.exit.autoRestart || model.exit.notice != "" {
		t.Fatalf("disabled setting auto restarted: %#v", model.exit)
	}

	model = newAutoRestartModel(make(chan Action, 1))
	_, command := model.Update(FatalMsg{Err: errors.New("hso failed")})
	if command != nil || model.exit.autoRestart {
		t.Fatalf("exit = %#v, command = %T", model.exit, command)
	}
}

func TestModelAutoRestartStopsOnEscapeOnlyWhileWaiting(t *testing.T) {
	actions := make(chan Action, 1)
	model := newAutoRestartModel(actions)
	crash(model, errors.New("crashed"), time.Minute*10)

	// 待っている間は他のキーを受けない。
	_, _ = model.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if len(actions) != 0 || !model.exit.autoRestart {
		t.Fatalf("modal leaked key: actions = %d, exit = %#v",
			len(actions), model.exit)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.exit.autoRestart || !model.exit.autoRestartAt.IsZero() ||
		model.exit.notice != msg.ExitAutoRestartCanceled {
		t.Fatalf("exit = %#v", model.exit)
	}
	// やめた後は元の三択に戻る。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if model.exit.button != 1 {
		t.Fatalf("button = %d", model.exit.button)
	}
}

// 再起動を頼んだ後は取り消せない。取り消せたつもりで裏の起動が進むと、
// 画面と実際がずれる。
func TestModelAutoRestartCannotBeStoppedAfterRequest(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 1))
	crash(model, errors.New("crashed"), time.Minute*10)
	model.exit.autoRestartAt = time.Now().Add(-time.Second)
	_, _ = model.Update(autoRestartMsg{state: model.exit})

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !model.exit.autoRestart || model.exit.notice != "" {
		t.Fatalf("exit = %#v", model.exit)
	}
}

func TestModelAutoRestartReportsRejectedRequest(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 1))
	crash(model, errors.New("crashed"), time.Minute*10)
	model.exit.autoRestartAt = time.Now().Add(-time.Second)
	model.busy = true
	_, command := model.Update(autoRestartMsg{state: model.exit})
	if command != nil || model.exit.autoRestart ||
		model.exit.notice != msg.ExitAutoRestartRejected {
		t.Fatalf("exit = %#v, command = %T", model.exit, command)
	}
}

// hso 自身の失敗は自動再起動の対象外。findJava の失敗のように hso が
// SIGTERM を送る経路では、後から同じ世代の終了通知が届いて状態を
// 作り直すので、そこで普通のクラッシュに化けてはいけない。
func TestModelFatalExitIsNeverAutoRestarted(t *testing.T) {
	actions := make(chan Action, 1)
	model := newAutoRestartModel(actions)
	_, _ = model.Update(FatalMsg{Err: errors.New("java not found")})

	exitedAt := time.Now()
	_, command := model.Update(ProcessExitedMsg{
		ExitCode:  143,
		StartedAt: exitedAt.Add(-time.Second),
		ExitedAt:  exitedAt,
	})
	if command != nil || model.exit.autoRestart ||
		model.exit.notice != msg.ExitAutoRestartFatal {
		t.Fatalf("exit = %#v, command = %T", model.exit, command)
	}
	if len(actions) != 0 || len(model.restart.attempts) != 0 {
		t.Fatalf("actions = %d, attempts = %d",
			len(actions), len(model.restart.attempts))
	}
}

// 手動 restart では controller が runtime を切り離すので終了通知が届かない。
// 畳まれる世代の稼働時間はここで拾わないと、履歴が残ったままになる。
func TestModelManualRestartForgetsCrashesAfterLongRun(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 4))
	crash(model, errors.New("crashed"), time.Second)
	if len(model.restart.attempts) != 1 {
		t.Fatalf("attempts = %d", len(model.restart.attempts))
	}

	_, _ = model.Update(ServerStartedMsg{
		Generation: 1,
		StartedAt:  time.Now().Add(-2 * shortRunLimit),
	})
	_, _ = model.Update(ServerRestartingMsg{})
	if len(model.restart.attempts) != 0 {
		t.Fatalf("attempts = %d", len(model.restart.attempts))
	}
}

// 終了通知で記録済みの世代を、その後の再起動でもう一度数えない。
func TestModelRestartingDoesNotRecordTwice(t *testing.T) {
	model := newAutoRestartModel(make(chan Action, 4))
	crash(model, errors.New("crashed"), time.Second)
	_, _ = model.Update(ServerRestartingMsg{})
	if len(model.restart.attempts) != 1 {
		t.Fatalf("attempts = %d", len(model.restart.attempts))
	}
}

func TestRestartTrackerShortRunBoundary(t *testing.T) {
	startedAt := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	tracker := restartTracker{attempts: []restartAttempt{{}, {}}}

	tracker.record(startedAt, startedAt.Add(shortRunLimit-time.Nanosecond), true)
	if len(tracker.attempts) != 3 {
		t.Fatalf("attempts = %d", len(tracker.attempts))
	}
	// ちょうど shortRunLimit もった世代は立ち直り側に入れる。
	tracker.record(startedAt, startedAt.Add(shortRunLimit), true)
	if len(tracker.attempts) != 0 {
		t.Fatalf("attempts = %d", len(tracker.attempts))
	}

	// 稼働時間が分からない世代は履歴に触らない。
	tracker.attempts = []restartAttempt{{}}
	tracker.record(time.Time{}, startedAt, true)
	if len(tracker.attempts) != 1 {
		t.Fatalf("attempts = %d", len(tracker.attempts))
	}
}

// モーダルは幅 64 で、はみ出した行は黙って切られる。ja / en どちらの
// ビルドでも切れないことを、文言を足したときに気付ける形で見張る。
func TestExitModalMessagesFitTheModal(t *testing.T) {
	lines := []string{
		msg.ExitStateCrashed,
		msg.ExitStateStopped,
		msg.ExitAutoRestartHint,
		msg.ExitAutoRestartCanceled,
		msg.ExitAutoRestartStopped,
		msg.ExitAutoRestartSkipped,
		msg.ExitAutoRestartRejected,
		msg.ExitAutoRestartFatal,
		msg.ExitAutoRestartIn(999),
		msg.ExitAutoQuit(999),
		msg.ExitStateRestarting("..."),
	}
	for _, line := range lines {
		if width := stringWidth(line); width > exitModalWidth-2 {
			t.Errorf("width %d > %d: %q", width, exitModalWidth-2, line)
		}
	}
}

func TestModelShowsGCCollectionsAsUnavailableUntilFirstEvent(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	stats := strings.Join(model.statsLines(), "\n")
	if !strings.Contains(stats, "GC   n/a  total") {
		t.Fatalf("stats = %q", stats)
	}

	_, _ = model.Update(GCMsg{Event: gclog.Event{
		ID:    1,
		Pause: gclog.Duration{Value: time.Millisecond, Available: true},
	}})

	stats = strings.Join(model.statsLines(), "\n")
	if !strings.Contains(stats, "GC   1 collections  total") {
		t.Fatalf("stats = %q", stats)
	}
}
