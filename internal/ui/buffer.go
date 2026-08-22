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

type storedLogRecord struct {
	number uint64
	record logRecord
}

type wrappedLogRecord struct {
	number uint64
	lines  []logLine
}

// logAnchor は閲覧位置を本文のバイト位置で覚える。セグメント番号で持つと
// 幅を変えたときにセグメントの総数自体が変わり、同じ番号が本文の別の場所を
// 指してしまう（長い JSON やスタックトレースで顕著）。
type logAnchor struct {
	record uint64
	offset int
}

type bufferViewport struct {
	width  int
	height int
}

type lineBuffer struct {
	lines      []storedLogRecord
	start      int
	count      int
	nextNumber uint64
	// record が 0 の間は、幅が変わっても末尾への追従を続ける。
	anchor logAnchor

	wrapWidth     int
	wrapValid     bool
	wrapped       []wrappedLogRecord
	offsetMinutes int
}

// Following は最新行へ追従しているかを返す。
func (buffer *lineBuffer) Following() bool {
	return buffer.anchor.record == 0
}

func (buffer *lineBuffer) Add(record logRecord) {
	if len(buffer.lines) == 0 {
		return
	}
	record = record.bounded(maxLogRecordWidth)
	buffer.nextNumber++
	item := storedLogRecord{number: buffer.nextNumber, record: record}
	full := buffer.count == len(buffer.lines)

	if !full {
		position := (buffer.start + buffer.count) % len(buffer.lines)
		buffer.lines[position] = item
		buffer.count++
	} else {
		buffer.lines[buffer.start] = item
		buffer.start = (buffer.start + 1) % len(buffer.lines)
	}

	if !buffer.wrapValid {
		return
	}
	wrapped := wrappedLogRecord{
		number: item.number,
		lines:  wrapLogRecord(item.record, buffer.wrapWidth, buffer.offsetMinutes),
	}
	if !full {
		buffer.wrapped = append(buffer.wrapped, wrapped)
		return
	}
	copy(buffer.wrapped, buffer.wrapped[1:])
	buffer.wrapped[len(buffer.wrapped)-1] = wrapped
}

// SetTimeOffset は保存済みレコードを変えず、表示用の折り返しだけを作り直す。
func (buffer *lineBuffer) SetTimeOffset(minutes int) {
	if buffer.offsetMinutes == minutes {
		return
	}
	buffer.offsetMinutes = minutes
	buffer.invalidateWrap()
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
		buffer.nextNumber = 0
		buffer.anchor = logAnchor{}
		buffer.invalidateWrap()
		return
	}

	keep := min(buffer.count, limit)
	lines := make([]storedLogRecord, limit)
	for index := 0; index < keep; index++ {
		lines[index] = buffer.itemAt(buffer.count - keep + index)
	}
	buffer.lines = lines
	buffer.start = 0
	buffer.count = keep
	buffer.invalidateWrap()
}

func (buffer *lineBuffer) Scroll(delta int, viewport bufferViewport) {
	if delta == 0 || viewport.width <= 0 || viewport.height <= 0 || buffer.count == 0 {
		return
	}
	buffer.ensureWrapped(viewport.width)
	start := buffer.viewportStart(viewport)
	tail := max(0, buffer.displayLineCount()-viewport.height)
	target := clamp(start-delta, 0, tail)
	if target == tail {
		buffer.anchor = logAnchor{}
		return
	}
	buffer.anchor = buffer.anchorAt(target)
}

func (buffer *lineBuffer) ScrollToStart(viewport bufferViewport) {
	if viewport.width <= 0 || viewport.height <= 0 || buffer.count == 0 {
		return
	}
	buffer.ensureWrapped(viewport.width)
	if buffer.displayLineCount() <= viewport.height {
		buffer.anchor = logAnchor{}
		return
	}
	buffer.anchor = buffer.anchorAt(0)
}

func (buffer *lineBuffer) ScrollToEnd() {
	buffer.anchor = logAnchor{}
}

func (buffer *lineBuffer) Window(viewport bufferViewport) []logLine {
	if viewport.width <= 0 || viewport.height <= 0 || buffer.count == 0 {
		return nil
	}
	buffer.ensureWrapped(viewport.width)
	buffer.clampViewport(viewport)
	start := buffer.viewportStart(viewport)
	end := min(buffer.displayLineCount(), start+viewport.height)
	window := make([]logLine, 0, end-start)
	lineNumber := 0
	for _, record := range buffer.wrapped {
		for _, line := range record.lines {
			if lineNumber >= start && lineNumber < end {
				window = append(window, line)
			}
			lineNumber++
			if lineNumber >= end {
				return window
			}
		}
	}
	return window
}

// Offset は現在の表示領域より下にある表示行数を返す。
func (buffer *lineBuffer) Offset(viewport bufferViewport) int {
	if viewport.width <= 0 || viewport.height <= 0 || buffer.count == 0 {
		return 0
	}
	buffer.ensureWrapped(viewport.width)
	buffer.clampViewport(viewport)
	if buffer.anchor.record == 0 {
		return 0
	}
	start := buffer.viewportStart(viewport)
	return max(0, buffer.displayLineCount()-min(buffer.displayLineCount(), start+viewport.height))
}

func (buffer *lineBuffer) clampViewport(viewport bufferViewport) {
	if len(buffer.wrapped) == 0 || buffer.anchor.record == 0 {
		return
	}
	// 末尾より下を指してしまったときだけ追従へ戻す。それ以外で書き戻すと
	// 覚えた本文位置が現在の幅のセグメント先頭へ丸められ、幅を往復した
	// ときに位置がずれていく。
	tail := max(0, buffer.displayLineCount()-viewport.height)
	if buffer.anchorLine() >= tail {
		buffer.anchor = logAnchor{}
	}
}

func (buffer *lineBuffer) viewportStart(viewport bufferViewport) int {
	if buffer.anchor.record == 0 {
		return max(0, buffer.displayLineCount()-viewport.height)
	}
	return max(0, buffer.anchorLine())
}

func (buffer *lineBuffer) anchorLine() int {
	lineNumber := 0
	for _, record := range buffer.wrapped {
		if record.number == buffer.anchor.record {
			// 覚えた本文位置を含むセグメント、つまり先頭がその位置を
			// 追い越さない最後のセグメントへ写す。
			segment := 0
			for index, line := range record.lines {
				if line.bodyOffset > buffer.anchor.offset {
					break
				}
				segment = index
			}
			return lineNumber + segment
		}
		lineNumber += len(record.lines)
	}
	// アンカーのレコードが履歴から押し出されたときは最古行へ寄せる。
	return 0
}

func (buffer *lineBuffer) anchorAt(lineNumber int) logAnchor {
	position := 0
	for _, record := range buffer.wrapped {
		end := position + len(record.lines)
		if lineNumber < end {
			line := record.lines[lineNumber-position]
			return logAnchor{record: record.number, offset: line.bodyOffset}
		}
		position = end
	}
	return logAnchor{}
}

func (buffer *lineBuffer) ensureWrapped(width int) {
	if buffer.wrapValid && buffer.wrapWidth == width {
		return
	}
	wrapped := make([]wrappedLogRecord, 0, len(buffer.lines))
	for index := 0; index < buffer.count; index++ {
		item := buffer.itemAt(index)
		wrapped = append(wrapped, wrappedLogRecord{
			number: item.number,
			lines:  wrapLogRecord(item.record, width, buffer.offsetMinutes),
		})
	}
	buffer.wrapWidth = width
	buffer.wrapValid = true
	buffer.wrapped = wrapped
}

func (buffer *lineBuffer) invalidateWrap() {
	buffer.wrapWidth = 0
	buffer.wrapValid = false
	buffer.wrapped = nil
}

func (buffer *lineBuffer) displayLineCount() int {
	count := 0
	for _, record := range buffer.wrapped {
		count += len(record.lines)
	}
	return count
}

func (buffer *lineBuffer) itemAt(index int) storedLogRecord {
	if index < 0 || index >= buffer.count {
		return storedLogRecord{}
	}
	return buffer.lines[(buffer.start+index)%len(buffer.lines)]
}

func (buffer *lineBuffer) At(index int) logRecord {
	return buffer.itemAt(index).record
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
