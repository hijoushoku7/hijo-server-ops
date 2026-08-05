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
