package ui

import "github.com/charmbracelet/x/ansi"

type lineBuffer struct {
	lines []string
	start int
	count int
	// offset は最新行から何行遡って表示しているか。0 なら最新に追従する。
	offset int
}

func (buffer *lineBuffer) Add(line string) {
	if len(buffer.lines) == 0 {
		return
	}
	if buffer.count < len(buffer.lines) {
		position := (buffer.start + buffer.count) % len(buffer.lines)
		buffer.lines[position] = line
		buffer.count++
		// 遡って読んでいる間は、新着で表示が流れないよう位置を保つ。
		if buffer.offset > 0 {
			buffer.offset++
		}
		buffer.clampOffset()
		return
	}
	buffer.lines[buffer.start] = line
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
	lines := make([]string, limit)
	for index := 0; index < keep; index++ {
		lines[index] = buffer.At(buffer.count - keep + index)
	}
	buffer.lines = lines
	buffer.start = 0
	buffer.count = keep
	buffer.clampOffset()
}

// Scroll は表示位置を delta 行ぶん遡らせる（負なら最新方向へ戻す）。
func (buffer *lineBuffer) Scroll(delta, viewport int) {
	buffer.offset += delta
	buffer.clampViewport(viewport)
}

// ScrollToEnd は最新行に追従する状態へ戻す。
func (buffer *lineBuffer) ScrollToEnd() {
	buffer.offset = 0
}

// Window は現在の表示位置から viewport 行ぶんの表示内容を返す。
func (buffer *lineBuffer) Window(viewport int) []string {
	if viewport <= 0 || buffer.count == 0 {
		return nil
	}
	buffer.clampViewport(viewport)
	end := buffer.count - buffer.offset
	start := max(0, end-viewport)
	window := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		window = append(window, buffer.At(index))
	}
	return window
}

// Offset は最新行から遡っている行数。0 なら最新に追従している。
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

func (buffer *lineBuffer) Truncate(width int) {
	for index := 0; index < buffer.count; index++ {
		position := (buffer.start + index) % len(buffer.lines)
		buffer.lines[position] = truncate(buffer.lines[position], width)
	}
}

func (buffer *lineBuffer) At(index int) string {
	if index < 0 || index >= buffer.count {
		return ""
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
