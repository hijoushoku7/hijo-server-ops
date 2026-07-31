package ui

import (
	"math"
	"runtime"
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

func TestFormatCPUDividesByCoreCount(t *testing.T) {
	// コア数はマシン依存なので、期待値もコア数から作る。
	cores := float64(runtime.NumCPU())
	if got := formatCPU(cores*100, true); got != "100%" {
		t.Fatalf("formatCPU = %q", got)
	}
	if got := formatCPU(cores*50, true); got != "50%" {
		t.Fatalf("formatCPU = %q", got)
	}
	if got := formatCPU(0, false); got != "n/a" {
		t.Fatalf("formatCPU = %q", got)
	}
}

func TestFormatRSSPercent(t *testing.T) {
	memory := procstats.Memory{
		RSS:       procstats.Number{Value: 4 << 30, Available: true},
		HostTotal: procstats.Number{Value: 16 << 30, Available: true},
	}
	if got := formatRSSPercent(memory); got != "25%" {
		t.Fatalf("formatRSSPercent = %q", got)
	}

	memory.CgroupLimit = procstats.Limit{Value: 8 << 30, Available: true}
	if got := formatRSSPercent(memory); got != "50%" {
		t.Fatalf("formatRSSPercent = %q", got)
	}

	if got := formatRSSPercent(procstats.Memory{}); got != "n/a" {
		t.Fatalf("formatRSSPercent = %q", got)
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

func TestHighlightColorsOnlyAboveThresholds(t *testing.T) {
	// 平常時は素のまま。色が付いていること自体を警告として使うため。
	if got := highlight("68%", 0.68); got != "68%" {
		t.Fatalf("normal = %q", stripANSI(got))
	}
	if got := highlight("n/a", math.NaN()); got != "n/a" {
		t.Fatalf("unavailable = %q", stripANSI(got))
	}

	high := highlight("81%", 0.81)
	over := highlight("94%", 0.94)
	if high == "81%" || over == "94%" || high == over {
		t.Fatalf("high = %q, over = %q", stripANSI(high), stripANSI(over))
	}
	// 色を落としても数値そのものは変わらない。
	if stripANSI(high) != "81%" || stripANSI(over) != "94%" {
		t.Fatalf("text = %q, %q", stripANSI(high), stripANSI(over))
	}
}

func TestRatiosReportUnavailableAsNaN(t *testing.T) {
	if !math.IsNaN(rssRatio(procstats.Memory{})) {
		t.Fatal("rssRatio is not NaN")
	}
	if !math.IsNaN(cpuRatio(0, false)) {
		t.Fatal("cpuRatio is not NaN")
	}
	memory := procstats.Memory{
		RSS:       procstats.Number{Value: 4 << 30, Available: true},
		HostTotal: procstats.Number{Value: 16 << 30, Available: true},
	}
	if ratio := rssRatio(memory); ratio != 0.25 {
		t.Fatalf("rssRatio = %f", ratio)
	}
}
