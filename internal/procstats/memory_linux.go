package procstats

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ReadMemory(pid int) (Memory, error) {
	return readMemoryAt("/proc", "/proc/self/mountinfo", pid)
}

func readMemoryAt(procRoot, mountInfoPath string, pid int) (Memory, error) {
	if pid <= 0 {
		return Memory{}, fmt.Errorf("invalid PID: %d", pid)
	}

	status, err := os.Open(filepath.Join(procRoot, strconv.Itoa(pid), "status"))
	if err != nil {
		return Memory{}, fmt.Errorf("open process status: %w", err)
	}
	rss, readErr := parseRSS(status)
	closeErr := status.Close()
	if readErr != nil {
		return Memory{}, readErr
	}
	if closeErr != nil {
		return Memory{}, fmt.Errorf("close process status: %w", closeErr)
	}

	memory := Memory{
		RSS:       rss,
		HostTotal: readMemTotal(filepath.Join(procRoot, "meminfo")),
	}
	memberships, err := readCgroups(filepath.Join(procRoot, strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return memory, nil
	}
	mounts, err := readCgroupMounts(mountInfoPath)
	if err != nil {
		return memory, nil
	}

	for _, membership := range memberships {
		for _, mount := range mounts {
			if membership.version != mount.version {
				continue
			}
			current, limit := readCgroupMemory(mount, membership)
			memory.CgroupCurrent = current
			memory.CgroupLimit = limit
			if current.Available || limit.Available {
				return memory, nil
			}
		}
	}
	return memory, nil
}

func readMemTotal(path string) Number {
	file, err := os.Open(path)
	if err != nil {
		return Number{}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || fields[0] != "MemTotal:" || fields[2] != "kB" {
			continue
		}
		kibibytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return Number{}
		}
		return Number{Value: kibibytes * 1024, Available: true}
	}
	return Number{}
}

func parseRSS(status *os.File) (Number, error) {
	scanner := bufio.NewScanner(status)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || fields[0] != "VmRSS:" {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" {
			return Number{}, fmt.Errorf("malformed VmRSS: %q", scanner.Text())
		}
		kibibytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return Number{}, fmt.Errorf("read VmRSS: %w", err)
		}
		return Number{Value: kibibytes * 1024, Available: true}, nil
	}
	if err := scanner.Err(); err != nil {
		return Number{}, fmt.Errorf("read process status: %w", err)
	}
	return Number{}, nil
}
