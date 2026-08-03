package ui

import (
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
)

var (
	meterFullStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#50FA7B"))
	meterHighStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB86C"))
	meterOverStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555"))
	meterEmptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#444444"))
)

const (
	meterHighRatio = 0.75
	meterOverRatio = 0.9
)

// meter は 1 行ぶんのメーター。ratio は 0..1、text は元のパーセント表示。
// available が false のときはメーターを描かず n/a と出す（推測で埋めない）。
type meter struct {
	label     string
	ratio     float64
	text      string
	available bool
}

func renderMeter(ratio float64, width int) string {
	if width <= 0 {
		return ""
	}
	if math.IsNaN(ratio) || ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	style := meterFullStyle
	switch {
	case ratio >= meterOverRatio:
		style = meterOverStyle
	case ratio >= meterHighRatio:
		style = meterHighStyle
	}

	eighths := int(math.Round(ratio * float64(width) * 8))
	full := eighths / 8
	remainder := eighths % 8
	if full > width {
		full = width
		remainder = 0
	}

	var bar strings.Builder
	bar.WriteString(strings.Repeat("█", full))
	if remainder > 0 && full < width {
		// U+2588 の先が左寄せの部分ブロック（▏▎▍…）。手前へ遡ると
		// 下寄せ（▁▂▃…）になり、横棒には使えない。
		bar.WriteRune(rune('█' + (8 - remainder)))
		full++
	}
	return style.Render(bar.String()) +
		meterEmptyStyle.Render(strings.Repeat("░", max(0, width-full)))
}

// meterLines は 1 本を 2 行に組む。上が "ラベル … パーセント"、
// 下が幅いっぱいのバー。Meters 列は狭いので、バーに全幅を使う。
// 値が取れないメーターはバーを描かず、パーセント欄に n/a と出す（原則4）。
func meterLines(meters []meter, width int) []string {
	lines := make([]string, 0, len(meters)*3)
	for index, item := range meters {
		// メーター同士が地続きに見えないよう 1 行空ける。
		if index > 0 {
			lines = append(lines, "")
		}
		gap := max(1, width-stringWidth(item.label)-stringWidth(item.text))
		lines = append(lines, fitLine(
			item.label+strings.Repeat(" ", gap)+item.text,
			width,
		))
		if !item.available {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, renderMeter(item.ratio, width))
	}
	return lines
}

// cpuMeter はマシン全体を 100% とする。Minecraft の主要処理は単スレッド
// なので満目には届きにくいが、マシン全体に対する使用量として読める。
func cpuMeter(value float64, available bool) meter {
	if !available || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return meter{label: "CPU", text: "n/a"}
	}
	return meter{
		label:     "CPU",
		ratio:     normalizeCPU(value) / 100,
		text:      formatCPU(value, true),
		available: true,
	}
}

func heapMeter(heap hsperfdata.Memory) meter {
	used := heap.Used
	limit := heap.Max
	if !used.Available || !limit.Available || used.Value < 0 || limit.Value <= 0 {
		return meter{label: "Heap", text: "n/a"}
	}
	return meter{
		label:     "Heap",
		ratio:     float64(used.Value) / float64(limit.Value),
		text:      fmt.Sprintf("%.0f%%", percent(used.Value, limit.Value)),
		available: true,
	}
}

// rssMeter は cgroup 制限を分母にし、制限がなければホスト総メモリに落とす。
// どちらも取れなければ n/a。使った分母は second の戻り値で示す。
func rssMeter(memory procstats.Memory) (meter, string) {
	limit, source := rssDenominator(memory)
	if !memory.RSS.Available || limit == 0 {
		return meter{label: "RSS", text: "n/a"}, ""
	}

	return meter{
		label:     "RSS",
		ratio:     float64(memory.RSS.Value) / float64(limit),
		text:      formatRSSPercent(memory),
		available: true,
	}, source
}

func rssDenominator(memory procstats.Memory) (uint64, string) {
	if memory.CgroupLimit.Available && !memory.CgroupLimit.Unlimited &&
		memory.CgroupLimit.Value > 0 {
		return memory.CgroupLimit.Value, "cgroup"
	}
	if memory.HostTotal.Available && memory.HostTotal.Value > 0 {
		return memory.HostTotal.Value, "host"
	}
	return 0, ""
}

func percent(value, limit int64) float64 {
	if limit <= 0 {
		return 0
	}
	return float64(value) / float64(limit) * 100
}
