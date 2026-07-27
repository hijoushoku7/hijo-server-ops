package procstats

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Number struct {
	Value     uint64
	Available bool
}

type Limit struct {
	Value     uint64
	Available bool
	Unlimited bool
}

type Memory struct {
	RSS           Number
	CgroupCurrent Number
	CgroupLimit   Limit
}

func ReadMemory(pid int) (Memory, error) {
	return readMemoryAt("/proc", "/sys/fs/cgroup", pid)
}

func readMemoryAt(procRoot, cgroupRoot string, pid int) (Memory, error) {
	if pid <= 0 {
		return Memory{}, fmt.Errorf("PIDが不正です: %d", pid)
	}

	status, err := os.Open(filepath.Join(procRoot, strconv.Itoa(pid), "status"))
	if err != nil {
		return Memory{}, fmt.Errorf("プロセスのstatusを開く: %w", err)
	}
	rss, err := parseRSS(status)
	closeErr := status.Close()
	if err != nil {
		return Memory{}, err
	}
	if closeErr != nil {
		return Memory{}, fmt.Errorf("プロセスのstatusを閉じる: %w", closeErr)
	}

	memory := Memory{RSS: rss}
	cgroupPath, err := readCgroupPath(filepath.Join(
		procRoot,
		strconv.Itoa(pid),
		"cgroup",
	))
	if err != nil {
		return memory, nil
	}
	groupDir := filepath.Join(cgroupRoot, strings.TrimPrefix(cgroupPath, "/"))
	memory.CgroupCurrent = readNumber(filepath.Join(groupDir, "memory.current"))
	memory.CgroupLimit = readLimit(filepath.Join(groupDir, "memory.max"))
	return memory, nil
}

func parseRSS(status *os.File) (Number, error) {
	scanner := bufio.NewScanner(status)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || fields[0] != "VmRSS:" {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" {
			return Number{}, fmt.Errorf("VmRSSの形式が不正です: %q", scanner.Text())
		}
		kibibytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return Number{}, fmt.Errorf("VmRSSを読む: %w", err)
		}
		return Number{Value: kibibytes * 1024, Available: true}, nil
	}
	if err := scanner.Err(); err != nil {
		return Number{}, fmt.Errorf("プロセスのstatusを読む: %w", err)
	}
	return Number{}, nil
}

func readCgroupPath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "0::") {
			continue
		}
		group := strings.TrimPrefix(line, "0::")
		if group == "" || !filepath.IsAbs(group) {
			return "", errors.New("cgroup v2のパスが不正です")
		}
		return group, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("cgroup v2を使用していません")
}

func readNumber(path string) Number {
	data, err := os.ReadFile(path)
	if err != nil {
		return Number{}
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return Number{}
	}
	return Number{Value: value, Available: true}
}

func readLimit(path string) Limit {
	data, err := os.ReadFile(path)
	if err != nil {
		return Limit{}
	}
	value := strings.TrimSpace(string(data))
	if value == "max" {
		return Limit{Available: true, Unlimited: true}
	}
	limit, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return Limit{}
	}
	return Limit{Value: limit, Available: true}
}
