package gclog

import (
	"testing"
	"time"
)

func TestParseG1Pause(t *testing.T) {
	line := "[2026-07-27T12:00:00.000+0000][12.345s][info][gc] " +
		"GC(7) Pause Young (Normal) (G1 Evacuation Pause) " +
		"2048M->512M(4096M) 12.345ms"

	event, ok := Parse(line)
	if !ok {
		t.Fatal("Parse did not recognize line")
	}
	if event.ID != 7 {
		t.Fatalf("ID = %d", event.ID)
	}
	assertDuration(t, event.Uptime, 12_345*time.Millisecond)
	assertBytes(t, event.Before, 2048<<20)
	assertBytes(t, event.After, 512<<20)
	assertBytes(t, event.Capacity, 4096<<20)
	assertDuration(t, event.Pause, 12_345*time.Microsecond)
}

func TestParseSerialPauseWithDifferentUnits(t *testing.T) {
	line := "[1.500s][info][gc] GC(1) Pause Full (System.gc()) " +
		"1.5G->768M(2G) 0.25s"

	event, ok := Parse(line)
	if !ok {
		t.Fatal("Parse did not recognize line")
	}
	assertBytes(t, event.Before, 1536<<20)
	assertBytes(t, event.After, 768<<20)
	assertBytes(t, event.Capacity, 2<<30)
	assertDuration(t, event.Pause, 250*time.Millisecond)
}

func TestParseZGCTransitionWithoutCapacity(t *testing.T) {
	line := "[3.000s][info][gc] GC(2) Garbage Collection (Warmup) " +
		"14M(1%)->2M(0%)"

	event, ok := Parse(line)
	if !ok {
		t.Fatal("Parse did not recognize line")
	}
	assertBytes(t, event.Before, 14<<20)
	assertBytes(t, event.After, 2<<20)
	if event.Capacity.Available || event.Pause.Available {
		t.Fatalf("Event = %#v", event)
	}
}

func TestParsePauseWithoutHeapTransition(t *testing.T) {
	line := "[4.000s][info][gc] GC(3) Pause Final Mark 0.321ms"

	event, ok := Parse(line)
	if !ok {
		t.Fatal("Parse did not recognize line")
	}
	if event.After.Available {
		t.Fatalf("After = %#v", event.After)
	}
	assertDuration(t, event.Pause, 321*time.Microsecond)
}

func TestParseIgnoresConcurrentDurationAsPause(t *testing.T) {
	line := "[4.000s][info][gc] GC(3) Concurrent Mark 12.000ms"
	if _, ok := Parse(line); ok {
		t.Fatal("Parse recognized concurrent duration")
	}
}

func TestParseIgnoresUnrelatedLine(t *testing.T) {
	if _, ok := Parse("[info][gc] Using G1"); ok {
		t.Fatal("Parse recognized unrelated line")
	}
}

func assertBytes(t *testing.T, got Bytes, want uint64) {
	t.Helper()
	if !got.Available || got.Value != want {
		t.Fatalf("Bytes = %#v, want %d", got, want)
	}
}

func assertDuration(t *testing.T, got Duration, want time.Duration) {
	t.Helper()
	if !got.Available || got.Value != want {
		t.Fatalf("Duration = %#v, want %s", got, want)
	}
}
