package ui

import (
	"strings"
	"testing"

	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
)

func TestRenderMeterFillsProportionally(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
		full  int
	}{
		{name: "empty", ratio: 0, full: 0},
		{name: "half", ratio: 0.5, full: 4},
		{name: "full", ratio: 1, full: 8},
		{name: "clamped above", ratio: 3, full: 8},
		{name: "clamped below", ratio: -1, full: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bar := stripANSI(renderMeter(test.ratio, 8))
			if got := strings.Count(bar, "█"); got != test.full {
				t.Fatalf("filled = %d, want %d: %q", got, test.full, bar)
			}
			if width := stringWidth(bar); width != 8 {
				t.Fatalf("width = %d: %q", width, bar)
			}
		})
	}
}

func TestRenderMeterUsesLeftAlignedPartialBlocks(t *testing.T) {
	// 端数セルは左寄せの部分ブロック（▏▎▍…）。下寄せ（▁▂▃…）だと
	// 横棒の途中に細い横線が出てしまう。
	bar := stripANSI(renderMeter(0.5, 5))

	if width := stringWidth(bar); width != 5 {
		t.Fatalf("width = %d: %q", width, bar)
	}
	if !strings.ContainsAny(bar, "▏▎▍▌▋▊▉") {
		t.Fatalf("bar has no partial block: %q", bar)
	}
	if strings.ContainsAny(bar, "▁▂▃▄▅▆▇") {
		t.Fatalf("bar uses bottom-aligned blocks: %q", bar)
	}
}

func TestHeapMeterIsUnavailableWithoutMax(t *testing.T) {
	item := heapMeter(hsperfdata.Memory{
		Used: hsperfdata.Number{Value: 1 << 30, Available: true},
	})

	if item.available || item.text != "n/a" {
		t.Fatalf("meter = %#v", item)
	}
}

func TestRSSMeterFallsBackToHostTotal(t *testing.T) {
	memory := procstats.Memory{
		RSS:       procstats.Number{Value: 4 << 30, Available: true},
		HostTotal: procstats.Number{Value: 16 << 30, Available: true},
	}

	item, source := rssMeter(memory)
	if !item.available || source != "host" || item.text != "25%" {
		t.Fatalf("meter = %#v, source = %q", item, source)
	}

	// cgroup 制限があるならそちらを優先する。
	memory.CgroupLimit = procstats.Limit{Value: 8 << 30, Available: true}
	item, source = rssMeter(memory)
	if !item.available || source != "cgroup" || item.text != "50%" {
		t.Fatalf("meter = %#v, source = %q", item, source)
	}

	// unlimited は制限なしなのでホスト総メモリへ落とす。
	memory.CgroupLimit = procstats.Limit{Available: true, Unlimited: true}
	if _, source = rssMeter(memory); source != "host" {
		t.Fatalf("source = %q", source)
	}

	// どちらも取れなければ推測せず n/a。
	memory.HostTotal = procstats.Number{}
	item, source = rssMeter(memory)
	if item.available || item.text != "n/a" || source != "" {
		t.Fatalf("meter = %#v, source = %q", item, source)
	}
}

func TestMeterLinesFitTheGivenWidth(t *testing.T) {
	lines := meterLines([]meter{
		{label: "CPU", ratio: 0.5, text: "125%", available: true},
		{label: "Heap", ratio: 0.2, text: "20%", available: true},
		{label: "RSS", text: "n/a"},
	}, 20)

	if len(lines) != 3 {
		t.Fatalf("lines = %d", len(lines))
	}
	for index, line := range lines {
		if width := stringWidth(line); width != 20 {
			t.Fatalf("line %d width = %d: %q", index, width, stripANSI(line))
		}
	}
	if !strings.Contains(stripANSI(lines[2]), "n/a") {
		t.Fatalf("unavailable meter = %q", stripANSI(lines[2]))
	}
}
