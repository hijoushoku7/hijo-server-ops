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
	model := New()
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
	model := New()
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
	model := New()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = model.Update(MetricsMsg{
		JVM: hsperfdata.Metrics{Heap: hsperfdata.Memory{
			Used: hsperfdata.Number{Value: 2 << 30, Available: true},
			Max:  hsperfdata.Number{Value: 4 << 30, Available: true},
		}},
		Memory: procstats.Memory{
			RSS: procstats.Number{Value: 3 << 30, Available: true},
		},
	})

	content := model.View().Content
	if !strings.Contains(content, "Heap") || !strings.Contains(content, "RSS") {
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
	model := New()
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

func TestModelReturnsProcessError(t *testing.T) {
	model := New()
	want := errors.New("server failed")
	_, command := model.Update(ProcessExitedMsg{Err: want})

	if !errors.Is(model.Err(), want) {
		t.Fatalf("Err = %v", model.Err())
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("command = %T", command())
	}
}

func containsBraille(value string) bool {
	for _, character := range value {
		if character >= 0x2800 && character <= 0x28ff {
			return true
		}
	}
	return false
}
