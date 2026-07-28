package ui

import (
	"math"
	"strings"
)

type memorySample struct {
	heap      uint64
	heapKnown bool
	rss       uint64
	rssKnown  bool
}

type sampleBuffer struct {
	samples []memorySample
	start   int
	count   int
}

func (buffer *sampleBuffer) Add(sample memorySample) {
	if len(buffer.samples) == 0 {
		return
	}
	if buffer.count < len(buffer.samples) {
		position := (buffer.start + buffer.count) % len(buffer.samples)
		buffer.samples[position] = sample
		buffer.count++
		return
	}
	buffer.samples[buffer.start] = sample
	buffer.start = (buffer.start + 1) % len(buffer.samples)
}

func (buffer *sampleBuffer) SetLimit(limit int) {
	if limit < 0 {
		limit = 0
	}
	if limit == len(buffer.samples) {
		return
	}
	if limit == 0 {
		buffer.samples = nil
		buffer.start = 0
		buffer.count = 0
		return
	}

	keep := min(buffer.count, limit)
	samples := make([]memorySample, limit)
	for index := 0; index < keep; index++ {
		samples[index] = buffer.At(buffer.count - keep + index)
	}
	buffer.samples = samples
	buffer.start = 0
	buffer.count = keep
}

func (buffer *sampleBuffer) At(index int) memorySample {
	if index < 0 || index >= buffer.count {
		return memorySample{}
	}
	return buffer.samples[(buffer.start+index)%len(buffer.samples)]
}

func (buffer *sampleBuffer) Len() int {
	return buffer.count
}

type sampleValue func(memorySample) (uint64, bool)

func renderBraille(
	samples sampleBuffer,
	value sampleValue,
	width int,
	height int,
	maximum uint64,
) []string {
	if width <= 0 || height <= 0 {
		return nil
	}

	cells := make([]byte, width*height)
	dotWidth := width * 2
	dotHeight := height * 4
	count := min(samples.Len(), dotWidth)
	firstX := dotWidth - count

	var (
		previousX int
		previousY int
		hasPoint  bool
	)
	for index := 0; index < count; index++ {
		sample := samples.At(samples.Len() - count + index)
		number, available := value(sample)
		if !available || maximum == 0 {
			hasPoint = false
			continue
		}

		x := firstX + index
		ratio := math.Min(1, float64(number)/float64(maximum))
		y := dotHeight - 1 - int(math.Round(ratio*float64(dotHeight-1)))
		if hasPoint {
			drawLine(cells, width, height, previousX, previousY, x, y)
		} else {
			setBrailleDot(cells, width, height, x, y)
		}
		previousX = x
		previousY = y
		hasPoint = true
	}

	lines := make([]string, height)
	for row := 0; row < height; row++ {
		var line strings.Builder
		line.Grow(width * 3)
		for column := 0; column < width; column++ {
			bits := cells[row*width+column]
			if bits == 0 {
				line.WriteByte(' ')
				continue
			}
			line.WriteRune(rune(0x2800 + int(bits)))
		}
		lines[row] = line.String()
	}
	return lines
}

func drawLine(
	cells []byte,
	width int,
	height int,
	fromX int,
	fromY int,
	toX int,
	toY int,
) {
	deltaX := abs(toX - fromX)
	stepX := -1
	if fromX < toX {
		stepX = 1
	}
	deltaY := -abs(toY - fromY)
	stepY := -1
	if fromY < toY {
		stepY = 1
	}
	lineError := deltaX + deltaY

	for {
		setBrailleDot(cells, width, height, fromX, fromY)
		if fromX == toX && fromY == toY {
			return
		}
		twiceError := 2 * lineError
		if twiceError >= deltaY {
			lineError += deltaY
			fromX += stepX
		}
		if twiceError <= deltaX {
			lineError += deltaX
			fromY += stepY
		}
	}
}

func setBrailleDot(cells []byte, width, height, x, y int) {
	if x < 0 || y < 0 || x >= width*2 || y >= height*4 {
		return
	}

	bits := [4][2]byte{
		{1 << 0, 1 << 3},
		{1 << 1, 1 << 4},
		{1 << 2, 1 << 5},
		{1 << 6, 1 << 7},
	}
	cell := (y/4)*width + x/2
	cells[cell] |= bits[y%4][x%2]
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
