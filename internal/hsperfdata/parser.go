package hsperfdata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	prologueSize = 32
	entrySize    = 20
	magic        = 0xcafec0c0
)

var (
	ErrNotAccessible = errors.New("hsperfdata is not readable yet")
	ErrUnsupported   = errors.New("unsupported hsperfdata format")
)

type Counter struct {
	long     int64
	text     string
	isString bool
}

func (c Counter) Long() (int64, bool) {
	return c.long, !c.isString
}

func (c Counter) String() (string, bool) {
	return c.text, c.isString
}

type Snapshot struct {
	Counters map[string]Counter
	Overflow int64
}

func (s Snapshot) Long(name string) (int64, bool) {
	counter, ok := s.Counters[name]
	if !ok {
		return 0, false
	}
	return counter.Long()
}

func (s Snapshot) String(name string) (string, bool) {
	counter, ok := s.Counters[name]
	if !ok {
		return "", false
	}
	return counter.String()
}

func Parse(data []byte) (Snapshot, error) {
	if len(data) < prologueSize {
		return Snapshot{}, errors.New("hsperfdata prologue too short")
	}

	order, err := byteOrder(data[4])
	if err != nil {
		return Snapshot{}, err
	}
	if binary.BigEndian.Uint32(data[0:4]) != magic {
		return Snapshot{}, errors.New("invalid hsperfdata magic")
	}
	if data[5] != 2 {
		return Snapshot{}, fmt.Errorf("%w: version %d.%d", ErrUnsupported, data[5], data[6])
	}
	if data[7] == 0 {
		return Snapshot{}, ErrNotAccessible
	}

	used := int(order.Uint32(data[8:12]))
	overflow := int64(int32(order.Uint32(data[12:16])))
	entryOffset := int(order.Uint32(data[24:28]))
	numEntries := int(order.Uint32(data[28:32]))
	if used < prologueSize || used > len(data) {
		return Snapshot{}, fmt.Errorf("invalid hsperfdata used size: %d", used)
	}
	if entryOffset < prologueSize || entryOffset > used || entryOffset%4 != 0 {
		return Snapshot{}, fmt.Errorf("invalid hsperfdata entry offset: %d", entryOffset)
	}
	if numEntries < 0 || numEntries > (used-entryOffset)/entrySize {
		return Snapshot{}, fmt.Errorf("invalid hsperfdata entry count: %d", numEntries)
	}

	snapshot := Snapshot{
		Counters: make(map[string]Counter, numEntries),
		Overflow: overflow,
	}
	offset := entryOffset
	for index := 0; index < numEntries; index++ {
		name, counter, next, err := parseEntry(data[:used], offset, order)
		if err != nil {
			return Snapshot{}, fmt.Errorf("hsperfdata entry %d: %w", index, err)
		}
		snapshot.Counters[name] = counter
		offset = next
	}
	return snapshot, nil
}

func parseEntry(data []byte, offset int, order binary.ByteOrder) (string, Counter, int, error) {
	if offset < 0 || offset%4 != 0 || offset+entrySize > len(data) {
		return "", Counter{}, 0, fmt.Errorf("invalid offset: %d", offset)
	}

	entryLength := int(order.Uint32(data[offset : offset+4]))
	if entryLength < entrySize || offset+entryLength > len(data) {
		return "", Counter{}, 0, fmt.Errorf("invalid entry length: %d", entryLength)
	}

	nameOffset := int(order.Uint32(data[offset+4 : offset+8]))
	vectorLength := int(order.Uint32(data[offset+8 : offset+12]))
	dataType := data[offset+12]
	dataOffset := int(order.Uint32(data[offset+16 : offset+20]))
	if nameOffset < entrySize || nameOffset >= entryLength {
		return "", Counter{}, 0, fmt.Errorf("invalid name offset: %d", nameOffset)
	}
	if dataOffset <= nameOffset || dataOffset >= entryLength {
		return "", Counter{}, 0, fmt.Errorf("invalid data offset: %d", dataOffset)
	}

	nameBytes := data[offset+nameOffset : offset+dataOffset]
	terminator := bytes.IndexByte(nameBytes, 0)
	if terminator <= 0 {
		return "", Counter{}, 0, errors.New("name is not NUL terminated")
	}
	name := string(nameBytes[:terminator])

	switch {
	case vectorLength == 0 && dataType == 'J':
		if dataOffset+8 > entryLength {
			return "", Counter{}, 0, errors.New("long value crosses the entry boundary")
		}
		value := int64(order.Uint64(data[offset+dataOffset : offset+dataOffset+8]))
		return name, Counter{long: value}, offset + entryLength, nil
	case vectorLength > 0 && dataType == 'B':
		if dataOffset+vectorLength > entryLength {
			return "", Counter{}, 0, errors.New("string crosses the entry boundary")
		}
		value := data[offset+dataOffset : offset+dataOffset+vectorLength]
		if end := bytes.IndexByte(value, 0); end >= 0 {
			value = value[:end]
		}
		return name, Counter{text: string(value), isString: true}, offset + entryLength, nil
	default:
		return "", Counter{}, 0, fmt.Errorf(
			"%w: type=%q vector length=%d",
			ErrUnsupported,
			dataType,
			vectorLength,
		)
	}
}

func byteOrder(value byte) (binary.ByteOrder, error) {
	switch value {
	case 0:
		return binary.BigEndian, nil
	case 1:
		return binary.LittleEndian, nil
	default:
		return nil, fmt.Errorf("invalid hsperfdata byte order: %d", value)
	}
}
