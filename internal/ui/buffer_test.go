package ui

import (
	"reflect"
	"testing"
)

func TestLineBufferDropsLinesOutsideLimit(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(2)
	buffer.Add("one")
	buffer.Add("two")
	buffer.Add("three")

	if got, want := bufferValues(buffer), []string{"two", "three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}
}

func TestLineBufferResizeReleasesOldLines(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(3)
	buffer.Add("one")
	buffer.Add("two")
	buffer.Add("three")
	buffer.SetLimit(1)

	if got, want := bufferValues(buffer), []string{"three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}

	buffer.SetLimit(0)
	if buffer.Len() != 0 || buffer.Limit() != 0 || buffer.lines != nil {
		t.Fatalf("buffer = %#v", buffer)
	}
}

func TestLineBufferTruncatesWideCharacters(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(1)
	buffer.Add("abc日本語")
	buffer.Truncate(5)

	if got := buffer.At(0); got != "abc日" {
		t.Fatalf("line = %q", got)
	}
}

func TestLineBufferAddDoesNotAllocateAfterInitialization(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(4)

	allocations := testing.AllocsPerRun(100, func() {
		buffer.Add("line")
	})
	if allocations != 0 {
		t.Fatalf("allocations per Add = %f", allocations)
	}
}

func bufferValues(buffer lineBuffer) []string {
	values := make([]string, buffer.Len())
	for index := range values {
		values[index] = buffer.At(index)
	}
	return values
}
