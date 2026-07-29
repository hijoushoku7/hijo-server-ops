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

func TestLineBufferKeepsScrollPositionWhenFull(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(4)
	for _, line := range []string{"l0", "l1", "l2", "l3"} {
		buffer.Add(line)
	}
	buffer.Scroll(2, 2)

	if got, want := buffer.Window(2), []string{"l0", "l1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window = %v, want %v", got, want)
	}

	// 満杯なので最古行が押し出されるが、見えている内容は動かない。
	buffer.Add("l4")
	if got, want := buffer.Window(2), []string{"l1", "l2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window after add = %v, want %v", got, want)
	}

	// 履歴から落ちた先へは遡れないので、最古行に張り付く。
	buffer.Add("l5")
	buffer.Add("l6")
	if got, want := buffer.Window(2), []string{"l3", "l4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window at the oldest line = %v, want %v", got, want)
	}

	buffer.ScrollToEnd()
	if got, want := buffer.Window(2), []string{"l5", "l6"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window after ScrollToEnd = %v, want %v", got, want)
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
