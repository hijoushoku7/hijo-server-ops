package ui

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

const timestampGutterWidth = 6

type textSegment struct {
	text   string
	offset int
}

type logLine struct {
	record     logRecord
	text       string
	bodyOffset int
	prefix     string
	timestamp  bool
}

func wrapLogRecord(record logRecord, width, offsetMinutes int) []logLine {
	body := strings.ReplaceAll(record.line(), "\t", " ")
	firstWidth := width
	continuationWidth := width
	indent := 0
	prefix := ""
	if width >= timestampGutterWidth {
		firstWidth -= timestampGutterWidth
		indent = timestampGutterWidth
		prefix = formatLogTimestamp(record, offsetMinutes) + " "
	}

	segments := wrapPlainText(body, firstWidth, continuationWidth, indent)
	lines := make([]logLine, len(segments))
	for index, segment := range segments {
		linePrefix := strings.Repeat(" ", indent)
		showTimestamp := false
		if index == 0 {
			linePrefix = prefix
			showTimestamp = prefix != ""
		}
		lines[index] = logLine{
			record:     record,
			text:       segment.text,
			bodyOffset: segment.offset,
			prefix:     linePrefix,
			timestamp:  showTimestamp,
		}
	}
	return lines
}

// wrapPlainText は ANSI を含まない本文だけを折り、元文字列内の位置も残す。
// 空白で折れるときは区切りを表示せず、長い単語はセル幅で切る。
func wrapPlainText(value string, firstWidth, continuationWidth, indent int) []textSegment {
	firstWidth = max(0, firstWidth)
	continuationWidth = max(0, continuationWidth-indent)
	if value == "" || firstWidth == 0 {
		return []textSegment{{}}
	}

	segments := make([]textSegment, 0, 1)
	start := 0
	width := firstWidth
	for start < len(value) {
		end := hardWrapEnd(value, start, width)
		if end >= len(value) {
			segments = append(segments, textSegment{text: value[start:], offset: start})
			break
		}

		breakAt, next := whitespaceBreak(value, start, end)
		if breakAt < 0 {
			breakAt, next = end, end
			for next < len(value) {
				character, size := utf8.DecodeRuneInString(value[next:])
				if !unicode.IsSpace(character) {
					break
				}
				next += size
			}
		}
		segments = append(segments, textSegment{
			text:   value[start:breakAt],
			offset: start,
		})
		start = next
		width = continuationWidth
		if width == 0 {
			break
		}
	}
	if len(segments) == 0 {
		return []textSegment{{}}
	}
	return segments
}

func hardWrapEnd(value string, start, width int) int {
	end := start
	used := 0
	for end < len(value) {
		cluster, characterWidth := ansi.FirstGraphemeCluster(
			value[end:],
			ansi.GraphemeWidth,
		)
		if end > start && used+characterWidth > width {
			break
		}
		end += len(cluster)
		used += characterWidth
		if used >= width {
			break
		}
	}
	return end
}

func whitespaceBreak(value string, start, end int) (int, int) {
	breakAt := -1
	next := -1
	for index, character := range value[start:end] {
		position := start + index
		if position > start && unicode.IsSpace(character) {
			breakAt = position
			next = position + utf8.RuneLen(character)
		}
	}
	if breakAt < 0 {
		return -1, -1
	}
	for next < len(value) {
		character, size := utf8.DecodeRuneInString(value[next:])
		if !unicode.IsSpace(character) {
			break
		}
		next += size
	}
	return breakAt, next
}

func renderLogRecord(record logRecord, width int) string {
	lines := wrapLogRecord(record, width, 0)
	return renderLogLine(lines[0], width)
}

func renderLogLine(line logLine, width int) string {
	if width <= 0 {
		return ""
	}

	prefix := truncate(line.prefix, width)
	bodyWidth := max(0, width-stringWidth(prefix))
	body := truncate(line.text, bodyWidth)

	var result strings.Builder
	if line.timestamp {
		result.WriteString(timestampStyle(line.record.timestampSource).Render(prefix))
	} else {
		result.WriteString(prefix)
	}

	playerStart := -1
	normalized := strings.ReplaceAll(line.record.line(), "\t", " ")
	if line.record.player != "" {
		playerStart = strings.Index(normalized, line.record.player)
	}
	segmentStart := line.bodyOffset
	segmentEnd := segmentStart + len(body)
	playerEnd := playerStart + len(line.record.player)
	kindStyle := styleForLogKind(line.record.kind)
	if playerStart < 0 || playerEnd <= segmentStart || playerStart >= segmentEnd {
		result.WriteString(kindStyle.Render(body))
	} else {
		start := clamp(playerStart-segmentStart, 0, len(body))
		end := clamp(playerEnd-segmentStart, start, len(body))
		result.WriteString(kindStyle.Render(body[:start]))
		result.WriteString(styleForPlayer(line.record.player).Render(body[start:end]))
		result.WriteString(kindStyle.Render(body[end:]))
	}

	if padding := width - stringWidth(prefix+body); padding > 0 {
		result.WriteString(strings.Repeat(" ", padding))
	}
	return result.String()
}

func formatLogTimestamp(record logRecord, offsetMinutes int) string {
	if record.timestamp.IsZero() || record.timestampSource == serverlog.TimestampUnknown {
		return "n/a  "
	}
	return record.timestamp.Add(time.Duration(offsetMinutes) * time.Minute).Format("15:04")
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
