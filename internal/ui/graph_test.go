package ui

import (
	"reflect"
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

func TestRenderBraillePlacesLatestSampleAtRightEdge(t *testing.T) {
	var samples sampleBuffer
	samples.SetLimit(2)
	samples.Add(memorySample{heap: 0, heapKnown: true})

	lines := renderBraille(
		samples,
		func(sample memorySample) (uint64, bool) {
			return sample.heap, sample.heapKnown
		},
		1,
		1,
		10,
	)

	if got, want := lines, []string{"⢀"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

func TestRenderBrailleLeavesUnavailableGap(t *testing.T) {
	var samples sampleBuffer
	samples.SetLimit(2)
	samples.Add(memorySample{})

	lines := renderBraille(
		samples,
		func(sample memorySample) (uint64, bool) {
			return sample.heap, sample.heapKnown
		},
		1,
		1,
		10,
	)

	if got, want := lines, []string{" "}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
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
