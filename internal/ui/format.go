package ui

import (
	"fmt"
	"math"
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

func formatCPU(value float64, available bool) string {
	if !available || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", value)
}

func fitLine(value string, width int) string {
	value = truncate(value, width)
	visible := stringWidth(value)
	if visible < width {
		value += strings.Repeat(" ", width-visible)
	}
	return value
}
