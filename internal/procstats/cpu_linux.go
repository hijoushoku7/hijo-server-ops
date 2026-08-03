package procstats

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func ReadCPUTime(pid int) (Duration, error) {
	return readCPUTimeAt("/proc", pid)
}

func readCPUTimeAt(procRoot string, pid int) (Duration, error) {
	if pid <= 0 {
		return Duration{}, fmt.Errorf("invalid PID: %d", pid)
	}
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return Duration{}, fmt.Errorf("read process stat: %w", err)
	}
	return parseCPUTime(data)
}

func parseCPUTime(data []byte) (Duration, error) {
	line := strings.TrimSpace(string(data))
	close := strings.LastIndexByte(line, ')')
	if close < 0 {
		return Duration{}, errors.New("malformed process stat")
	}
	fields := strings.Fields(line[close+1:])
	if len(fields) < 13 {
		return Duration{}, errors.New("process stat has too few fields")
	}
	user, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return Duration{}, fmt.Errorf("read process user CPU time: %w", err)
	}
	system, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return Duration{}, fmt.Errorf("read process system CPU time: %w", err)
	}
	if user > ^uint64(0)-system {
		return Duration{}, errors.New("process CPU time is too large")
	}

	// 対応対象の Linux/amd64・arm64 では /proc の CPU 時間は USER_HZ=100。
	const clockTicksPerSecond = uint64(100)
	ticks := user + system
	seconds := ticks / clockTicksPerSecond
	if seconds > uint64((1<<63-1)/int64(time.Second)) {
		return Duration{}, errors.New("process CPU time is too large")
	}
	value := time.Duration(seconds)*time.Second +
		time.Duration(ticks%clockTicksPerSecond)*time.Second/
			time.Duration(clockTicksPerSecond)
	return Duration{Value: value, Available: true}, nil
}
