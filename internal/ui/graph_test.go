package ui

import (
	"reflect"
	"strings"
	"testing"
)

func TestSampleBufferKeepsOnlyVisibleSamples(t *testing.T) {
	var buffer sampleBuffer
	buffer.SetLimit(2)
	buffer.Add(memorySample{heap: 1, heapKnown: true})
	buffer.Add(memorySample{heap: 2, heapKnown: true})
	buffer.Add(memorySample{heap: 3, heapKnown: true})

	got := []memorySample{buffer.At(0), buffer.At(1)}
	want := []memorySample{
		{heap: 2, heapKnown: true},
		{heap: 3, heapKnown: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("samples = %#v, want %#v", got, want)
	}
}

func TestSampleBufferResizeKeepsNewestAndReleasesStorage(t *testing.T) {
	var buffer sampleBuffer
	buffer.SetLimit(3)
	buffer.Add(memorySample{heap: 1})
	buffer.Add(memorySample{heap: 2})
	buffer.Add(memorySample{heap: 3})
	buffer.SetLimit(1)

	if buffer.Len() != 1 || buffer.At(0).heap != 3 {
		t.Fatalf("buffer = %#v", buffer)
	}

	buffer.SetLimit(0)
	if buffer.Len() != 0 || buffer.samples != nil {
		t.Fatalf("buffer = %#v", buffer)
	}
}

func TestPlotSeriesPlacesLatestSampleAtRightEdge(t *testing.T) {
	var samples sampleBuffer
	samples.SetLimit(2)
	samples.Add(memorySample{heap: 0, heapKnown: true})

	cells := plotSeries(samples, heapValue, 1, 1, 0, 10)

	// 最新の 1 点は右下のドット。
	if got, want := cells, []byte{1 << 7}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cells = %v, want %v", got, want)
	}
}

func TestPlotSeriesLeavesUnavailableGap(t *testing.T) {
	var samples sampleBuffer
	samples.SetLimit(2)
	samples.Add(memorySample{})

	cells := plotSeries(samples, heapValue, 1, 1, 0, 10)

	if got, want := cells, []byte{0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cells = %v, want %v", got, want)
	}
}

func TestRenderChartDrawsAxisLabelsAndBothSeries(t *testing.T) {
	var samples sampleBuffer
	samples.SetLimit(4)
	samples.Add(memorySample{
		heap: 1 << 30, heapKnown: true,
		rss: 3 << 30, rssKnown: true,
	})
	samples.Add(memorySample{
		heap: 2 << 30, heapKnown: true,
		rss: 3 << 30, rssKnown: true,
	})

	lines := renderChart(
		samples,
		[]chartSeries{
			{value: heapValue, style: heapStyle},
			{value: rssValue, style: rssStyle},
		},
		axisWidth+2,
		2,
		1<<30,
		4<<30,
	)

	if len(lines) != 2 {
		t.Fatalf("lines = %d", len(lines))
	}
	// 上端に最大値、下端に最小値のラベルが出る。
	if !strings.Contains(stripANSI(lines[0]), "4.0G┤") ||
		!strings.Contains(stripANSI(lines[1]), "1.0G┤") {
		t.Fatalf("axis labels = %q", stripANSI(lines[0]+lines[1]))
	}
	if !containsBraille(strings.Join(lines, "")) {
		t.Fatalf("chart has no braille: %q", stripANSI(strings.Join(lines, "")))
	}
	for index, line := range lines {
		if width := stringWidth(stripANSI(line)); width != axisWidth+2 {
			t.Fatalf("line %d width = %d", index, width)
		}
	}
}

func TestSampleBufferAddDoesNotAllocateAfterInitialization(t *testing.T) {
	var buffer sampleBuffer
	buffer.SetLimit(4)

	allocations := testing.AllocsPerRun(100, func() {
		buffer.Add(memorySample{heap: 1, heapKnown: true})
	})
	if allocations != 0 {
		t.Fatalf("allocations per Add = %f", allocations)
	}
}
