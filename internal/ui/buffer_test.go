package ui

import (
	"reflect"
	"testing"
)

func TestLineBufferDropsLinesOutsideLimit(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(2)
	buffer.Add(testLogRecord("one"))
	buffer.Add(testLogRecord("two"))
	buffer.Add(testLogRecord("three"))

	if got, want := bufferValues(buffer), []string{"two", "three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}
}

func TestLineBufferResizeReleasesOldLines(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(3)
	buffer.Add(testLogRecord("one"))
	buffer.Add(testLogRecord("two"))
	buffer.Add(testLogRecord("three"))
	buffer.SetLimit(1)

	if got, want := bufferValues(buffer), []string{"three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}

	buffer.SetLimit(0)
	if buffer.Len() != 0 || buffer.Limit() != 0 || buffer.lines != nil {
		t.Fatalf("buffer = %#v", buffer)
	}
}

func TestLineBufferKeepsScrollPositionWhenFull(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(4)
	for _, line := range []string{"l0", "l1", "l2", "l3"} {
		buffer.Add(testLogRecord(line))
	}
	buffer.Scroll(2, 2)

	if got, want := recordLines(buffer.Window(2)), []string{"l0", "l1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window = %v, want %v", got, want)
	}

	// 満杯なので最古行が押し出されるが、見えている内容は動かない。
	buffer.Add(testLogRecord("l4"))
	if got, want := recordLines(buffer.Window(2)), []string{"l1", "l2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window after add = %v, want %v", got, want)
	}

	// 履歴から落ちた先へは遡れないので、最古行に張り付く。
	buffer.Add(testLogRecord("l5"))
	buffer.Add(testLogRecord("l6"))
	if got, want := recordLines(buffer.Window(2)), []string{"l3", "l4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window at the oldest line = %v, want %v", got, want)
	}

	buffer.ScrollToEnd()
	if got, want := recordLines(buffer.Window(2)), []string{"l5", "l6"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window after ScrollToEnd = %v, want %v", got, want)
	}
}

func TestLineBufferAddDoesNotAllocateAfterInitialization(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(4)

	allocations := testing.AllocsPerRun(100, func() {
		buffer.Add(testLogRecord("line"))
	})
	if allocations != 0 {
		t.Fatalf("allocations per Add = %f", allocations)
	}
}

func bufferValues(buffer lineBuffer) []string {
	values := make([]string, buffer.Len())
	for index := range values {
		values[index] = buffer.At(index).line()
	}
	return values
}

func recordLines(records []logRecord) []string {
	values := make([]string, len(records))
	for index, record := range records {
		values[index] = record.line()
	}
	return values
}

func testLogRecord(text string) logRecord {
	return logRecord{text: text}
}
