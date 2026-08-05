package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

func TestRenderLogRecordAddsSixColumnTimestampGutter(t *testing.T) {
	record := logRecord{
		timestamp:       time.Date(2026, time.August, 5, 12, 34, 56, 0, time.UTC),
		timestampSource: serverlog.TimestampLog,
		kind:            serverlog.KindOther,
		text:            "server ready",
	}

	got := stripANSI(renderLogRecord(record, 30))
	if !strings.HasPrefix(got, "12:34 server ready") {
		t.Fatalf("line = %q", got)
	}
	gutter := got[:timestampGutterWidth]
	if gutter != "12:34 " || stringWidth(gutter) != timestampGutterWidth {
		t.Fatalf("gutter = %q, width = %d", gutter, stringWidth(gutter))
	}
}

func TestRenderLogRecordDropsTimestampWhenWidthIsNarrow(t *testing.T) {
	record := logRecord{
		timestamp:       time.Date(2026, time.August, 5, 12, 34, 56, 0, time.UTC),
		timestampSource: serverlog.TimestampLog,
		kind:            serverlog.KindOther,
		text:            "hello",
	}

	got := stripANSI(renderLogRecord(record, timestampGutterWidth-1))
	if got != "hello" {
		t.Fatalf("line = %q", got)
	}
}

func TestPlayerColorIsStableForSameName(t *testing.T) {
	applyTheme(DefaultSettings())
	want := styleForPlayer("alice").GetForeground()
	for range 10 {
		if got := styleForPlayer("alice").GetForeground(); got != want {
			t.Fatalf("player color = %v, want %v", got, want)
		}
	}
}

// lipgloss はタブを空白 4 個へ展開するので、残したままだと幅計算より実際の
// 描画が広くなり枠がずれる。スタックトレースのような本文で起きる。
func TestRenderLogRecordFlattensTabs(t *testing.T) {
	record := logRecord{
		timestamp:       time.Date(2026, time.August, 5, 12, 34, 56, 0, time.UTC),
		timestampSource: serverlog.TimestampLog,
		kind:            serverlog.KindOther,
		text:            "\tat java.base/java.lang.Thread.run",
	}

	for width := 1; width <= 40; width++ {
		line := renderLogRecord(record, width)
		if strings.Contains(line, "\t") {
			t.Fatalf("width %d: line still contains a tab: %q", width, stripANSI(line))
		}
		if got := stringWidth(line); got != width {
			t.Fatalf("width %d: line width = %d, text = %q", width, got, stripANSI(line))
		}
	}
}

func TestRenderLogRecordKeepsWidthAfterColoring(t *testing.T) {
	record := logRecord{
		timestamp:       time.Date(2026, time.August, 5, 12, 34, 56, 0, time.UTC),
		timestampSource: serverlog.TimestampReceived,
		kind:            serverlog.KindChat,
		player:          "alice",
		text:            "こんにちは world",
	}

	for width := 1; width <= 30; width++ {
		line := renderLogRecord(record, width)
		if got := stringWidth(line); got != width {
			t.Fatalf("width %d: line width = %d, text = %q", width, got, stripANSI(line))
		}
	}
}
