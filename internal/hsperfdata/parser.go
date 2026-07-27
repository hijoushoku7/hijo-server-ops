package hsperfdata

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const (
	prologueSize = 32
	entrySize    = 20
	magic        = 0xcafec0c0
)

var (
	ErrNotAccessible = errors.New("hsperfdataはまだ読み取り可能ではありません")
	ErrUnsupported   = errors.New("未対応のhsperfdata形式です")
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
		return Snapshot{}, errors.New("hsperfdataのプロローグが短すぎます")
	}

	order, err := byteOrder(data[4])
	if err != nil {
		return Snapshot{}, err
	}
	if order.Uint32(data[0:4]) != magic {
		return Snapshot{}, errors.New("hsperfdataのマジック値が不正です")
	}
	if data[5] != 2 {
		return Snapshot{}, fmt.Errorf("%w: バージョン%d.%d", ErrUnsupported, data[5], data[6])
	}
	if data[7] == 0 {
		return Snapshot{}, ErrNotAccessible
	}

	used := int(order.Uint32(data[8:12]))
	overflow := int64(int32(order.Uint32(data[12:16])))
	entryOffset := int(order.Uint32(data[24:28]))
	numEntries := int(order.Uint32(data[28:32]))
	if used < prologueSize || used > len(data) {
		return Snapshot{}, fmt.Errorf("hsperfdataの使用量が不正です: %d", used)
	}
	if entryOffset < prologueSize || entryOffset > used || entryOffset%4 != 0 {
		return Snapshot{}, fmt.Errorf("hsperfdataのエントリ開始位置が不正です: %d", entryOffset)
	}
	if numEntries < 0 || numEntries > (used-entryOffset)/entrySize {
		return Snapshot{}, fmt.Errorf("hsperfdataのエントリ数が不正です: %d", numEntries)
	}

	snapshot := Snapshot{
		Counters: make(map[string]Counter, numEntries),
		Overflow: overflow,
	}
	offset := entryOffset
	for index := 0; index < numEntries; index++ {
		name, counter, next, err := parseEntry(data[:used], offset, order)
		if err != nil {
			return Snapshot{}, fmt.Errorf("hsperfdataエントリ%d: %w", index, err)
		}
		snapshot.Counters[name] = counter
		offset = next
	}
	return snapshot, nil
}

func parseEntry(data []byte, offset int, order binary.ByteOrder) (string, Counter, int, error) {
	if offset < 0 || offset%4 != 0 || offset+entrySize > len(data) {
		return "", Counter{}, 0, fmt.Errorf("開始位置が不正です: %d", offset)
	}

	entryLength := int(order.Uint32(data[offset : offset+4]))
	if entryLength < entrySize || offset+entryLength > len(data) {
		return "", Counter{}, 0, fmt.Errorf("長さが不正です: %d", entryLength)
	}

	nameOffset := int(order.Uint32(data[offset+4 : offset+8]))
	vectorLength := int(order.Uint32(data[offset+8 : offset+12]))
	dataType := data[offset+12]
	dataOffset := int(order.Uint32(data[offset+16 : offset+20]))
	if nameOffset < entrySize || nameOffset >= entryLength {
		return "", Counter{}, 0, fmt.Errorf("名前の位置が不正です: %d", nameOffset)
	}
	if dataOffset <= nameOffset || dataOffset >= entryLength {
		return "", Counter{}, 0, fmt.Errorf("データの位置が不正です: %d", dataOffset)
	}

	nameBytes := data[offset+nameOffset : offset+dataOffset]
	terminator := strings.IndexByte(string(nameBytes), 0)
	if terminator <= 0 {
		return "", Counter{}, 0, errors.New("名前がNUL終端されていません")
	}
	name := string(nameBytes[:terminator])

	switch {
	case vectorLength == 0 && dataType == 'J':
		if dataOffset+8 > entryLength {
			return "", Counter{}, 0, errors.New("long値がエントリ境界を越えています")
		}
		value := int64(order.Uint64(data[offset+dataOffset : offset+dataOffset+8]))
		return name, Counter{long: value}, offset + entryLength, nil
	case vectorLength > 0 && dataType == 'B':
		if dataOffset+vectorLength > entryLength {
			return "", Counter{}, 0, errors.New("文字列がエントリ境界を越えています")
		}
		value := data[offset+dataOffset : offset+dataOffset+vectorLength]
		if end := strings.IndexByte(string(value), 0); end >= 0 {
			value = value[:end]
		}
		return name, Counter{text: string(value), isString: true}, offset + entryLength, nil
	default:
		return "", Counter{}, 0, fmt.Errorf(
			"%w: 型=%q ベクタ長=%d",
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
		return nil, fmt.Errorf("hsperfdataのバイトオーダーが不正です: %d", value)
	}
}
