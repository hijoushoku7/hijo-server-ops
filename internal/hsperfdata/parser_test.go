package hsperfdata

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestParseLittleEndian(t *testing.T) {
	data := perfData(t, binary.LittleEndian,
		longEntry("sun.gc.generation.0.space.0.used", 1234),
		stringEntry("sun.gc.generation.0.name", "young"),
	)

	snapshot, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := snapshot.Long("sun.gc.generation.0.space.0.used"); !ok || value != 1234 {
		t.Fatalf("long = %d, %v", value, ok)
	}
	if value, ok := snapshot.String("sun.gc.generation.0.name"); !ok || value != "young" {
		t.Fatalf("string = %q, %v", value, ok)
	}
}

func TestParseBigEndian(t *testing.T) {
	data := perfData(t, binary.BigEndian, longEntry("java.threads.live", 42))

	snapshot, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := snapshot.Long("java.threads.live"); !ok || value != 42 {
		t.Fatalf("long = %d, %v", value, ok)
	}
}

func TestParseRejectsInaccessibleData(t *testing.T) {
	data := perfData(t, binary.LittleEndian, longEntry("counter", 1))
	data[7] = 0

	_, err := Parse(data)
	if !errors.Is(err, ErrNotAccessible) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseRejectsUnsupportedVersion(t *testing.T) {
	data := perfData(t, binary.LittleEndian, longEntry("counter", 1))
	data[5] = 3

	_, err := Parse(data)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseRejectsEntryOutsideUsedArea(t *testing.T) {
	data := perfData(t, binary.LittleEndian, longEntry("counter", 1))
	binary.LittleEndian.PutUint32(data[32:36], uint32(len(data)))

	if _, err := Parse(data); err == nil {
		t.Fatal("Parse succeeded")
	}
}

type testEntry struct {
	name   string
	long   int64
	text   string
	isText bool
}

func longEntry(name string, value int64) testEntry {
	return testEntry{name: name, long: value}
}

func stringEntry(name, value string) testEntry {
	return testEntry{name: name, text: value, isText: true}
}

func perfData(t *testing.T, order binary.ByteOrder, entries ...testEntry) []byte {
	t.Helper()

	data := make([]byte, prologueSize)
	if order == binary.LittleEndian {
		data[4] = 1
	}
	data[5] = 2
	data[7] = 1
	binary.BigEndian.PutUint32(data[0:4], magic)
	order.PutUint32(data[24:28], prologueSize)
	order.PutUint32(data[28:32], uint32(len(entries)))

	for _, item := range entries {
		name := append([]byte(item.name), 0)
		dataOffset := align(entrySize+len(name), 8)
		valueLength := 8
		vectorLength := 0
		dataType := byte('J')
		if item.isText {
			valueLength = len(item.text) + 1
			vectorLength = valueLength
			dataType = 'B'
		}
		entryLength := align(dataOffset+valueLength, 4)
		entry := make([]byte, entryLength)
		order.PutUint32(entry[0:4], uint32(entryLength))
		order.PutUint32(entry[4:8], entrySize)
		order.PutUint32(entry[8:12], uint32(vectorLength))
		entry[12] = dataType
		order.PutUint32(entry[16:20], uint32(dataOffset))
		copy(entry[entrySize:], name)
		if item.isText {
			copy(entry[dataOffset:], item.text)
		} else {
			order.PutUint64(entry[dataOffset:dataOffset+8], uint64(item.long))
		}
		data = append(data, entry...)
	}
	order.PutUint32(data[8:12], uint32(len(data)))
	return data
}

func align(value, alignment int) int {
	return (value + alignment - 1) &^ (alignment - 1)
}
