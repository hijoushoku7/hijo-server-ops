package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

func TestModelBoundsLogsAndSamplesToScreen(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	for index := 0; index < model.layout.logLines()+2; index++ {
		_, _ = model.Update(LogMsg{Entry: serverlog.Entry{
			Kind:    serverlog.KindOther,
			Message: strings.Repeat("x", 200) + string(rune('a'+index)),
		}})
	}

	if model.logs.Len() != model.layout.logLines() {
		t.Fatalf("logs = %d, limit = %d", model.logs.Len(), model.layout.logLines())
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
	if model.commands.At(0) != "alice: /time set day" {
		t.Fatalf("command = %q", model.commands.At(0))
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
		!strings.Contains(content, "CPU 125%") {
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

	if model.chat.lines != nil || model.commands.lines != nil ||
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
	if !strings.Contains(strings.Join(model.statsLines(), "\n"), failure.JVMError) {
		t.Fatalf("stats = %q", model.statsLines())
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
	model := New(actions, 0)

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

func TestModelSelectsRestartAndStopWithArrowKeys(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, 0)

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if model.focus != focusRestart {
		t.Fatalf("focus = %d", model.focus)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action := <-actions; action.Kind != ActionRestart {
		t.Fatalf("action = %#v", action)
	}

	_, _ = model.Update(ActionResultMsg{Action: Action{Kind: ActionRestart}})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	_, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.focus != focusStop {
		t.Fatalf("focus = %d", model.focus)
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("command = %T", command())
	}
}

func TestModelRecordsOnlySuccessfullySentCommands(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	action := Action{Kind: ActionSendCommand, Command: "say hello"}

	_, _ = model.Update(ActionResultMsg{Action: action, Err: errors.New("failed")})
	if model.commands.Len() != 0 {
		t.Fatalf("failed command was recorded")
	}
	_, _ = model.Update(ActionResultMsg{Action: action})
	if model.commands.Len() != 1 || model.commands.At(0) != "say hello" {
		t.Fatalf("commands = %q", model.commands.At(0))
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
	model := New(make(chan Action, 1), 2)
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

func newTestModel() *Model {
	return New(make(chan Action, 8), 0)
}

func containsBraille(value string) bool {
	for _, character := range value {
		if character >= 0x2800 && character <= 0x28ff {
			return true
		}
	}
	return false
}
