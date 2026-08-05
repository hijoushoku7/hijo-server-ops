package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

const timestampGutterWidth = 6

func renderLogRecord(record logRecord, width int) string {
	if width <= 0 {
		return ""
	}

	// タブは幅計算では 0 桁だが lipgloss が描画時に空白 4 個へ展開するので、
	// 幅を数える前に空白 1 個へ潰す。1 バイトのままなので後段の位置計算も狂わない。
	body := strings.ReplaceAll(record.line(), "\t", " ")
	prefix := ""
	if width >= timestampGutterWidth {
		prefix = formatLogTimestamp(record) + " "
	}
	// 切り詰めは ANSI を含まない状態で済ませ、色は残った範囲へ最後に付ける。
	plain := truncate(prefix+body, width)

	var result strings.Builder
	prefixEnd := min(len(prefix), len(plain))
	if prefixEnd > 0 {
		result.WriteString(timestampStyle(record.timestampSource).Render(plain[:prefixEnd]))
	}

	bodyStart := prefixEnd
	playerStart := -1
	if record.player != "" {
		if position := strings.Index(body, record.player); position >= 0 {
			playerStart = len(prefix) + position
		}
	}
	kindStyle := styleForLogKind(record.kind)
	if playerStart < bodyStart || playerStart >= len(plain) {
		result.WriteString(kindStyle.Render(plain[bodyStart:]))
	} else {
		playerEnd := min(playerStart+len(record.player), len(plain))
		result.WriteString(kindStyle.Render(plain[bodyStart:playerStart]))
		result.WriteString(styleForPlayer(record.player).Render(plain[playerStart:playerEnd]))
		result.WriteString(kindStyle.Render(plain[playerEnd:]))
	}

	if padding := width - stringWidth(plain); padding > 0 {
		result.WriteString(strings.Repeat(" ", padding))
	}
	return result.String()
}

func formatLogTimestamp(record logRecord) string {
	if record.timestamp.IsZero() || record.timestampSource == serverlog.TimestampUnknown {
		return "n/a  "
	}
	return record.timestamp.Format("15:04")
}

func timestampStyle(source serverlog.TimestampSource) lipgloss.Style {
	if source == serverlog.TimestampLog {
		return logTimestampStyle
	}
	return logReceivedStyle
}

func styleForLogKind(kind serverlog.Kind) lipgloss.Style {
	if style, ok := logKindStyles[kind]; ok {
		return style
	}
	return logKindStyles[serverlog.KindOther]
}

func styleForPlayer(name string) lipgloss.Style {
	if len(logPlayerStyles) == 0 {
		return lipgloss.NewStyle()
	}
	// FNV-1a で名前を固定の色番号へ割り当てる。
	hash := uint32(2166136261)
	for index := 0; index < len(name); index++ {
		hash ^= uint32(name[index])
		hash *= 16777619
	}
	return logPlayerStyles[int(hash%uint32(len(logPlayerStyles)))]
}
