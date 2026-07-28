package ui

import (
	"testing"
	"time"

	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
)

func TestFormatBytesAndUnavailableValues(t *testing.T) {
	if got := formatBytes(1536 << 20); got != "1.5G" {
		t.Fatalf("formatBytes = %q", got)
	}
	if got := formatJVMBytes(hsperfdata.Number{}); got != "n/a" {
		t.Fatalf("formatJVMBytes = %q", got)
	}
	if got := formatProcBytes(procstats.Number{}); got != "n/a" {
		t.Fatalf("formatProcBytes = %q", got)
	}
}

func TestFormatDelta(t *testing.T) {
	got := formatDelta(
		procstats.Number{Value: 5 << 30, Available: true},
		hsperfdata.Number{Value: 3 << 30, Available: true},
	)
	if got != "+2.0G" {
		t.Fatalf("delta = %q", got)
	}
}

func TestFormatUptime(t *testing.T) {
	got := formatUptime(hsperfdata.Duration{
		Value:     28*time.Hour + 12*time.Minute + 3*time.Second,
		Available: true,
	})
	if got != "1d 04:12:03" {
		t.Fatalf("uptime = %q", got)
	}
}
