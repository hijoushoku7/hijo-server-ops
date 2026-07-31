package ui

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
)

func formatJVMBytes(number hsperfdata.Number) string {
	if !number.Available || number.Value < 0 {
		return "n/a"
	}
	return formatBytes(uint64(number.Value))
}

func formatProcBytes(number procstats.Number) string {
	if !number.Available {
		return "n/a"
	}
	return formatBytes(number.Value)
}

func formatLimit(limit procstats.Limit) string {
	switch {
	case !limit.Available:
		return "n/a"
	case limit.Unlimited:
		return "unlimited"
	default:
		return formatBytes(limit.Value)
	}
}

func formatBytes(value uint64) string {
	const unit = uint64(1024)
	if value < unit {
		return fmt.Sprintf("%dB", value)
	}

	divisor := unit
	exponent := 0
	for quotient := value / unit; quotient >= unit && exponent < 4; quotient /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf(
		"%.1f%c",
		float64(value)/float64(divisor),
		"KMGTPE"[exponent],
	)
}

// formatAxisBytes は Y 軸ラベル用に桁を詰めた表記。10 以上は小数を落とし、
// "512M" "5.4G" のように 4 桁以内へ収める。
func formatAxisBytes(value uint64) string {
	text := formatBytes(value)
	unit := text[len(text)-1]
	whole, _, found := strings.Cut(text[:len(text)-1], ".")
	if found && len(whole) > 1 {
		return whole + string(unit)
	}
	return text
}

func formatDelta(rss procstats.Number, committed hsperfdata.Number) string {
	if !rss.Available || !committed.Available || committed.Value < 0 ||
		rss.Value > math.MaxInt64 {
		return "n/a"
	}
	delta := int64(rss.Value) - committed.Value
	sign := "+"
	if delta < 0 {
		sign = "-"
		delta = -delta
	}
	return sign + formatBytes(uint64(delta))
}

func formatUptime(duration hsperfdata.Duration) string {
	if !duration.Available || duration.Value < 0 {
		return "n/a"
	}
	value := duration.Value.Round(time.Second)
	days := value / (24 * time.Hour)
	value %= 24 * time.Hour
	hours := value / time.Hour
	value %= time.Hour
	minutes := value / time.Minute
	seconds := value % time.Minute / time.Second
	if days > 0 {
		return fmt.Sprintf("%dd %02d:%02d:%02d", days, hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func formatPause(duration time.Duration, available bool) string {
	if !available {
		return "n/a"
	}
	switch {
	case duration < time.Millisecond:
		return fmt.Sprintf("%.1fµs", float64(duration)/float64(time.Microsecond))
	case duration < time.Second:
		return fmt.Sprintf("%.1fms", float64(duration)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.2fs", duration.Seconds())
	}
}

func formatFrequency(value float64, available bool) string {
	if !available {
		return "n/a"
	}
	return fmt.Sprintf("%.2f/min", value)
}

// formatCPU は全コア合計で数えた使用率を、分母 100% に直して出す。
func formatCPU(value float64, available bool) string {
	if !available || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", normalizeCPU(value))
}

// normalizeCPU は「コア数 × 100%」を満目とする値を 0..100% に直す。
// 収集側は従来どおりコア数ぶんを合計しており、ここで割るだけ。
func normalizeCPU(value float64) float64 {
	cores := float64(runtime.NumCPU())
	if cores <= 0 {
		return value
	}
	return value / cores
}

// formatRSSPercent は RSS が分母に占める割合。分母は rssDenominator と同じで、
// cgroup 制限があればそれ、なければ OS の総メモリ。
func formatRSSPercent(memory procstats.Memory) string {
	limit, _ := rssDenominator(memory)
	if !memory.RSS.Available || limit == 0 ||
		memory.RSS.Value > math.MaxInt64 || limit > math.MaxInt64 {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", percent(int64(memory.RSS.Value), int64(limit)))
}

// rssRatio は RSS が分母に占める割合の 0..1 表現。取れなければ NaN。
func rssRatio(memory procstats.Memory) float64 {
	limit, _ := rssDenominator(memory)
	if !memory.RSS.Available || limit == 0 {
		return math.NaN()
	}
	return float64(memory.RSS.Value) / float64(limit)
}

// cpuRatio は CPU 使用率の 0..1 表現。取れなければ NaN。
func cpuRatio(value float64, available bool) float64 {
	if !available || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return math.NaN()
	}
	return normalizeCPU(value) / 100
}

// highlight は割合が高いときだけ数値に色を付ける。平常時は素のままにする
// ことで、色が付いていること自体が警告になる。閾値は Meters の棒と同じ
// 75% / 90% で、色もメーターのプリセットから引く。
func highlight(text string, ratio float64) string {
	switch {
	case math.IsNaN(ratio):
		return text
	case ratio >= meterOverRatio:
		return meterOverStyle.Bold(true).Render(text)
	case ratio >= meterHighRatio:
		return meterHighStyle.Render(text)
	default:
		return text
	}
}

func fitLine(value string, width int) string {
	value = truncate(value, width)
	visible := stringWidth(value)
	if visible < width {
		value += strings.Repeat(" ", width-visible)
	}
	return value
}
