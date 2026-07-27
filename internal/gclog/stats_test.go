package gclog

import (
	"math"
	"testing"
	"time"
)

func TestStatsTracksPostGCAndPauses(t *testing.T) {
	var stats Stats
	stats.Add(Event{
		ID:     1,
		Uptime: Duration{Value: 10 * time.Second, Available: true},
		After:  Bytes{Value: 500, Available: true},
		Pause:  Duration{Value: 10 * time.Millisecond, Available: true},
	})
	stats.Add(Event{
		ID:     2,
		Uptime: Duration{Value: 40 * time.Second, Available: true},
		After:  Bytes{Value: 300, Available: true},
		Pause:  Duration{Value: 20 * time.Millisecond, Available: true},
	})

	if stats.Collections != 2 {
		t.Fatalf("Collections = %d", stats.Collections)
	}
	assertBytes(t, stats.PostGC, 300)
	assertDuration(t, stats.LastPause, 20*time.Millisecond)
	if stats.TotalPause != 30*time.Millisecond {
		t.Fatalf("TotalPause = %s", stats.TotalPause)
	}
	rate := stats.Frequency()
	if !rate.Available || math.Abs(rate.PerMinute-2) > 0.0001 {
		t.Fatalf("Frequency = %#v", rate)
	}
}

func TestStatsDoesNotCountSameCollectionTwice(t *testing.T) {
	var stats Stats
	stats.Add(Event{
		ID:    1,
		After: Bytes{Value: 500, Available: true},
	})
	stats.Add(Event{
		ID:    1,
		After: Bytes{Value: 400, Available: true},
		Pause: Duration{Value: time.Millisecond, Available: true},
	})

	if stats.Collections != 1 {
		t.Fatalf("Collections = %d", stats.Collections)
	}
	assertBytes(t, stats.PostGC, 400)
	assertDuration(t, stats.LastPause, time.Millisecond)
	if stats.Frequency().Available {
		t.Fatalf("Frequency = %#v", stats.Frequency())
	}
}

func TestStatsCountsPauseOnlyCollection(t *testing.T) {
	var stats Stats
	stats.Add(Event{
		ID:     1,
		Uptime: Duration{Value: time.Second, Available: true},
		Pause:  Duration{Value: time.Millisecond, Available: true},
	})

	if stats.Collections != 1 {
		t.Fatalf("Collections = %d", stats.Collections)
	}
	if stats.PostGC.Available {
		t.Fatalf("PostGC = %#v", stats.PostGC)
	}
}
