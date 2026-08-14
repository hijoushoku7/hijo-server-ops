// Package procstat は Linux の /proc/<pid>/stat を読む。
package procstat

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Stat は hso がプロセスの探索と PID 再利用の検出に使う項目だけを持つ。
type Stat struct {
	Command      string
	ParentPID    int
	ProcessGroup int
	StartTime    uint64
}

// Parse は /proc/<pid>/stat から必要な項目を読む。comm には空白や括弧が
// 入りうるので、最後の ')' より後ろにある固定位置の項目を数える。
func Parse(data []byte) (Stat, error) {
	line := strings.TrimSpace(string(data))
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close <= open {
		return Stat{}, errors.New("malformed /proc stat")
	}

	fields := strings.Fields(line[close+1:])
	if len(fields) < 20 {
		return Stat{}, errors.New("malformed /proc stat fields")
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return Stat{}, fmt.Errorf("read PPID: %w", err)
	}
	pgrp, err := strconv.Atoi(fields[2])
	if err != nil {
		return Stat{}, fmt.Errorf("read process group ID: %w", err)
	}
	startTime, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return Stat{}, fmt.Errorf("read start time: %w", err)
	}

	return Stat{
		Command:      line[open+1 : close],
		ParentPID:    ppid,
		ProcessGroup: pgrp,
		StartTime:    startTime,
	}, nil
}
