package ui

import (
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

const maxLogRecordWidth = 1024

type logRecord struct {
	timestamp       time.Time
	timestampSource serverlog.TimestampSource
	kind            serverlog.Kind
	player          string
	text            string
}

func (record logRecord) line() string {
	switch record.kind {
	case serverlog.KindChat:
		return "<" + record.player + "> " + record.text
	case serverlog.KindCommand:
		if record.player != "" {
			return record.player + ": " + record.text
		}
	}
	return record.text
}

// bounded はレコード 1 件が抱える文字列量に上限を設ける。端末幅とは無関係の
// メモリ上限で、画面幅がここに達することはないため表示には影響しない。
func (record logRecord) bounded(width int) logRecord {
	record.player = truncate(record.player, width)
	record.text = truncate(record.text, width)
	return record
}

type lineBuffer struct {
	lines []logRecord
	start int
	count int
	// offset は最新行から何行遡って表示しているか。0 なら最新に追従する。
	offset int
}

func (buffer *lineBuffer) Add(record logRecord) {
	if len(buffer.lines) == 0 {
		return
	}
	record = record.bounded(maxLogRecordWidth)
	if buffer.count < len(buffer.lines) {
		position := (buffer.start + buffer.count) % len(buffer.lines)
		buffer.lines[position] = record
		buffer.count++
		// 遡って読んでいる間は、新着で表示が流れないよう位置を保つ。
		if buffer.offset > 0 {
			buffer.offset++
		}
		buffer.clampOffset()
		return
	}
	buffer.lines[buffer.start] = record
	buffer.start = (buffer.start + 1) % len(buffer.lines)
	// 満杯のときは最古行が押し出されて全体が 1 行ずれるので、空きがある
	// ときと同じく offset も進めないと遡り位置が流れてしまう。
	if buffer.offset > 0 {
		buffer.offset++
	}
	buffer.clampOffset()
}

func (buffer *lineBuffer) SetLimit(limit int) {
	if limit < 0 {
		limit = 0
	}
	if limit == len(buffer.lines) {
		return
	}
	if limit == 0 {
		buffer.lines = nil
		buffer.start = 0
		buffer.count = 0
		buffer.offset = 0
		return
	}

	keep := min(buffer.count, limit)
	lines := make([]logRecord, limit)
	for index := 0; index < keep; index++ {
		lines[index] = buffer.At(buffer.count - keep + index)
	}
	buffer.lines = lines
	buffer.start = 0
	buffer.count = keep
	buffer.clampOffset()
}

func (buffer *lineBuffer) Scroll(delta, viewport int) {
	buffer.offset += delta
	buffer.clampViewport(viewport)
}

func (buffer *lineBuffer) ScrollToEnd() {
	buffer.offset = 0
}

func (buffer *lineBuffer) Window(viewport int) []logRecord {
	if viewport <= 0 || buffer.count == 0 {
		return nil
	}
	buffer.clampViewport(viewport)
	end := buffer.count - buffer.offset
	start := max(0, end-viewport)
	window := make([]logRecord, 0, end-start)
	for index := start; index < end; index++ {
		window = append(window, buffer.At(index))
	}
	return window
}

func (buffer *lineBuffer) Offset() int {
	return buffer.offset
}

func (buffer *lineBuffer) clampOffset() {
	if buffer.offset > buffer.count {
		buffer.offset = buffer.count
	}
	if buffer.offset < 0 {
		buffer.offset = 0
	}
}

func (buffer *lineBuffer) clampViewport(viewport int) {
	buffer.clampOffset()
	if limit := max(0, buffer.count-viewport); buffer.offset > limit {
		buffer.offset = limit
	}
}

func (buffer *lineBuffer) At(index int) logRecord {
	if index < 0 || index >= buffer.count {
		return logRecord{}
	}
	return buffer.lines[(buffer.start+index)%len(buffer.lines)]
}

func (buffer *lineBuffer) Len() int {
	return buffer.count
}

func (buffer *lineBuffer) Limit() int {
	return len(buffer.lines)
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "")
}
