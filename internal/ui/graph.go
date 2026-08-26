package ui

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// 初期値は theme.go の init() が既定プリセットから流し込む。
var overlapStyle lipgloss.Style

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

func heapValue(sample memorySample) (uint64, bool) {
	return sample.heap, sample.heapKnown
}

func rssValue(sample memorySample) (uint64, bool) {
	return sample.rss, sample.rssKnown
}

type chartSeries struct {
	value sampleValue
	style lipgloss.Style
}

// renderChart は複数の系列を 1 枚の braille グラフに重ね、左端に Y 軸
// ラベルを付ける。縦軸は low..high で、0 起点にはしない。ヒープの増減は
// 総量に対して小さく、0 起点だと GC のノコギリ波が 1 ドットに潰れるため。
// 系列が重なったセルは overlapStyle で描き、どちらの線か分からない箇所を
// 塗り分けたように見せない。
func renderChart(
	samples sampleBuffer,
	series []chartSeries,
	width int,
	height int,
	low uint64,
	high uint64,
) []string {
	plotWidth := width - axisWidth
	if plotWidth <= 0 || height <= 0 || high <= low {
		return nil
	}

	grids := make([][]byte, len(series))
	for index, item := range series {
		grids[index] = plotSeries(samples, item.value, plotWidth, height, low, high)
	}

	lines := make([]string, height)
	for row := 0; row < height; row++ {
		var line strings.Builder
		line.WriteString(axisLabel(row, height, low, high))
		writeChartRow(&line, grids, series, row, plotWidth)
		lines[row] = line.String()
	}
	return lines
}

func plotSeries(
	samples sampleBuffer,
	value sampleValue,
	width int,
	height int,
	low uint64,
	high uint64,
) []byte {
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
		if !available {
			hasPoint = false
			continue
		}

		x := firstX + index
		if number < low {
			number = low
		}
		ratio := math.Min(1, float64(number-low)/float64(high-low))
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
	return cells
}

func writeChartRow(
	line *strings.Builder,
	grids [][]byte,
	series []chartSeries,
	row int,
	width int,
) {
	var run strings.Builder
	runStyle := -1
	flush := func() {
		if run.Len() == 0 {
			return
		}
		text := run.String()
		run.Reset()
		switch {
		case runStyle < 0:
			line.WriteString(text)
		case runStyle < len(series):
			line.WriteString(series[runStyle].style.Render(text))
		default:
			line.WriteString(overlapStyle.Render(text))
		}
	}

	for column := 0; column < width; column++ {
		bits := byte(0)
		style := -1
		for index, cells := range grids {
			if cells[row*width+column] == 0 {
				continue
			}
			bits |= cells[row*width+column]
			if style < 0 {
				style = index
			} else {
				style = len(series)
			}
		}
		if style != runStyle {
			flush()
			runStyle = style
		}
		if bits == 0 {
			run.WriteByte(' ')
			continue
		}
		run.WriteRune(rune(0x2800 + int(bits)))
	}
	flush()
}

func axisLabel(row, height int, low, high uint64) string {
	value := uint64(0)
	switch row {
	case 0:
		value = high
	case height - 1:
		value = low
	case height / 2:
		value = low + (high-low)/2
	default:
		return dimStyle.Render(strings.Repeat(" ", axisWidth-1) + "│")
	}
	label := formatAxisBytes(value)
	if stringWidth(label) > axisWidth-1 {
		label = truncate(label, axisWidth-1)
	}
	return dimStyle.Render(
		strings.Repeat(" ", axisWidth-1-stringWidth(label)) + label + "┤",
	)
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
