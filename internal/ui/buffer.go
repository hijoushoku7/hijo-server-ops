package ui

import "github.com/charmbracelet/x/ansi"

type lineBuffer struct {
	lines []string
	start int
	count int
}

func (buffer *lineBuffer) Add(line string) {
	if len(buffer.lines) == 0 {
		return
	}
	if buffer.count < len(buffer.lines) {
		position := (buffer.start + buffer.count) % len(buffer.lines)
		buffer.lines[position] = line
		buffer.count++
		return
	}
	buffer.lines[buffer.start] = line
	buffer.start = (buffer.start + 1) % len(buffer.lines)
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
