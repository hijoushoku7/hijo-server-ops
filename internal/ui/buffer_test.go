package ui

import (
	"reflect"
	"strings"
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
	viewport := bufferViewport{width: 40, height: 2}
	for _, line := range []string{"l0", "l1", "l2", "l3"} {
		buffer.Add(testLogRecord(line))
	}
	buffer.Scroll(2, viewport)

	if got, want := windowTexts(buffer.Window(viewport)), []string{"l0", "l1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window = %v, want %v", got, want)
	}

	// 満杯なので最古行が押し出されるが、見えている内容は動かない。
	buffer.Add(testLogRecord("l4"))
	if got, want := windowTexts(buffer.Window(viewport)), []string{"l1", "l2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window after add = %v, want %v", got, want)
	}

	// 履歴から落ちた先へは遡れないので、最古行に張り付く。
	buffer.Add(testLogRecord("l5"))
	buffer.Add(testLogRecord("l6"))
	if got, want := windowTexts(buffer.Window(viewport)), []string{"l3", "l4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window at the oldest line = %v, want %v", got, want)
	}

	buffer.ScrollToEnd()
	if got, want := windowTexts(buffer.Window(viewport)), []string{"l5", "l6"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window after ScrollToEnd = %v, want %v", got, want)
	}
}

func TestLineBufferKeepsRecordAnchorAcrossWidthChange(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(10)
	for _, line := range []string{
		"first record wraps",
		"anchored record wraps",
		"third record wraps",
		"fourth record wraps",
		"fifth record wraps",
	} {
		buffer.Add(testLogRecord(line))
	}

	narrow := bufferViewport{width: 12, height: 3}
	buffer.ScrollToStart(narrow)
	firstLines := len(wrapLogRecord(buffer.At(0), narrow.width))
	buffer.Scroll(-firstLines-1, narrow)
	wantAnchor := buffer.itemAt(1).number
	want := logAnchor{
		record: wantAnchor,
		offset: wrapLogRecord(buffer.At(1), narrow.width)[1].bodyOffset,
	}
	if buffer.anchor != want {
		t.Fatalf("anchor = %#v, want %#v", buffer.anchor, want)
	}

	wide := bufferViewport{width: 18, height: 3}
	window := buffer.Window(wide)
	if buffer.anchor != want {
		t.Fatalf("anchor after resize = %#v, want %#v", buffer.anchor, want)
	}
	// 幅が広がるとセグメントの切れ目は変わるので、先頭が同じセグメント
	// 番号になるとは限らない。覚えた本文位置を含んでいればよい。
	top := window[0]
	if len(window) == 0 || top.record.line() != "anchored record wraps" ||
		top.bodyOffset > want.offset ||
		want.offset >= top.bodyOffset+len(top.text) {
		t.Fatalf("window after resize = %#v", window)
	}
}

// 1 レコードが何十セグメントにも折れるとき、アンカーをセグメント番号で
// 持つと幅を変えた時点でセグメント総数が変わり、まったく別の場所へ飛ぶ。
// 本文位置で覚えていれば、広げても狭めても同じ本文が先頭に残る。
func TestLineBufferKeepsBodyPositionAcrossWidthRoundTrip(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(2)
	buffer.Add(testLogRecord(strings.Repeat("あa1", 200)))

	narrow := bufferViewport{width: 12, height: 3}
	buffer.ScrollToStart(narrow)
	buffer.Scroll(-30, narrow)
	want := buffer.Window(narrow)[0].bodyOffset
	if want == 0 {
		t.Fatalf("scroll did not move into the record")
	}

	for _, viewport := range []bufferViewport{
		{width: 60, height: 3},
		{width: 8, height: 3},
		narrow,
	} {
		line := buffer.Window(viewport)[0]
		length := len(line.text)
		if line.bodyOffset > want || want >= line.bodyOffset+max(1, length) {
			t.Fatalf(
				"width %d: top segment covers [%d,%d), want it to contain %d",
				viewport.width, line.bodyOffset, line.bodyOffset+length, want,
			)
		}
	}
}

func TestLineBufferKeepsFollowingTailAcrossWidthChange(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(4)
	for _, line := range []string{"one long record", "two long record", "last long record"} {
		buffer.Add(testLogRecord(line))
	}

	narrow := bufferViewport{width: 10, height: 2}
	_ = buffer.Window(narrow)
	wide := bufferViewport{width: 16, height: 2}
	window := buffer.Window(wide)
	lastRecord := buffer.At(buffer.Len() - 1)
	lastLines := wrapLogRecord(lastRecord, wide.width)
	if buffer.anchor.record != 0 {
		t.Fatalf("tail following anchor = %#v", buffer.anchor)
	}
	if got, want := window[len(window)-1].text, lastLines[len(lastLines)-1].text; got != want {
		t.Fatalf("tail = %q, want %q", got, want)
	}
}

func TestLineBufferShowsTailOfRecordTallerThanViewport(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(1)
	buffer.Add(testLogRecord("abcdefghijklmnopqrst"))
	viewport := bufferViewport{width: 10, height: 2}

	window := buffer.Window(viewport)
	if len(window) != viewport.height {
		t.Fatalf("window height = %d, want %d", len(window), viewport.height)
	}
	if window[0].bodyOffset == 0 {
		t.Fatalf("window starts at the first segment: %#v", window)
	}
	if got, want := windowTexts(window), []string{"mnop", "qrst"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window = %v, want %v", got, want)
	}

	buffer.ScrollToStart(viewport)
	if got := buffer.Window(viewport)[0].bodyOffset; got != 0 {
		t.Fatalf("first segment offset = %d", got)
	}
}

func TestLineBufferScrollsByWrappedDisplayLine(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(1)
	buffer.Add(testLogRecord("abcdefghijklmnopqrst"))
	viewport := bufferViewport{width: 10, height: 2}

	buffer.Scroll(1, viewport)
	if got, want := windowTexts(buffer.Window(viewport)), []string{"ijkl", "mnop"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window after up = %v, want %v", got, want)
	}
	if got := buffer.Offset(viewport); got != 1 {
		t.Fatalf("offset after up = %d, want 1", got)
	}

	buffer.Scroll(-1, viewport)
	if got, want := windowTexts(buffer.Window(viewport)), []string{"mnop", "qrst"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window after down = %v, want %v", got, want)
	}
	if got := buffer.Offset(viewport); got != 0 {
		t.Fatalf("offset after down = %d, want 0", got)
	}
}

func TestLineBufferWrapCacheAddsOnlyTailAtSameWidth(t *testing.T) {
	var buffer lineBuffer
	buffer.SetLimit(3)
	buffer.Add(testLogRecord("first record wraps"))
	viewport := bufferViewport{width: 12, height: 2}
	_ = buffer.Window(viewport)
	firstLine := &buffer.wrapped[0].lines[0]

	buffer.Add(testLogRecord("second record wraps"))
	if got := &buffer.wrapped[0].lines[0]; got != firstLine {
		t.Fatalf("cached first record was rebuilt: %p -> %p", firstLine, got)
	}
	if got, want := len(buffer.wrapped), 2; got != want {
		t.Fatalf("wrapped records = %d, want %d", got, want)
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

func windowTexts(lines []logLine) []string {
	values := make([]string, len(lines))
	for index, line := range lines {
		values[index] = line.text
	}
	return values
}

func testLogRecord(text string) logRecord {
	return logRecord{text: text}
}
