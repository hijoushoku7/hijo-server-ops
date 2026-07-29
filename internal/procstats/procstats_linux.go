package procstats

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	// HostTotal は /proc/meminfo の MemTotal。cgroup 制限がない環境で
	// RSS の割合を出すための分母に使う。
	HostTotal Number
}

type Duration struct {
	Value     time.Duration
	Available bool
}

type cgroupMembership struct {
	version int
	path    string
}

type cgroupMount struct {
	version    int
	root       string
	mountPoint string
}

func ReadMemory(pid int) (Memory, error) {
	return readMemoryAt("/proc", "/proc/self/mountinfo", pid)
}

func ReadCPUTime(pid int) (Duration, error) {
	return readCPUTimeAt("/proc", pid)
}

func readCPUTimeAt(procRoot string, pid int) (Duration, error) {
	if pid <= 0 {
		return Duration{}, fmt.Errorf("PIDが不正です: %d", pid)
	}
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return Duration{}, fmt.Errorf("プロセスのstatを読む: %w", err)
	}
	return parseCPUTime(data)
}

func parseCPUTime(data []byte) (Duration, error) {
	line := strings.TrimSpace(string(data))
	close := strings.LastIndexByte(line, ')')
	if close < 0 {
		return Duration{}, errors.New("プロセスのstat形式が不正です")
	}
	fields := strings.Fields(line[close+1:])
	if len(fields) < 13 {
		return Duration{}, errors.New("プロセスのstatフィールドが不足しています")
	}
	user, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return Duration{}, fmt.Errorf("プロセスのuser CPU時間を読む: %w", err)
	}
	system, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return Duration{}, fmt.Errorf("プロセスのsystem CPU時間を読む: %w", err)
	}
	if user > ^uint64(0)-system {
		return Duration{}, errors.New("プロセスのCPU時間が大きすぎます")
	}
	// Linuxのamd64/arm64では/procのCPU時間にUSER_HZ=100を使う。
	const clockTicksPerSecond = uint64(100)
	ticks := user + system
	seconds := ticks / clockTicksPerSecond
	if seconds > uint64((1<<63-1)/int64(time.Second)) {
		return Duration{}, errors.New("プロセスのCPU時間が大きすぎます")
	}
	value := time.Duration(seconds)*time.Second +
		time.Duration(ticks%clockTicksPerSecond)*time.Second/
			time.Duration(clockTicksPerSecond)
	return Duration{Value: value, Available: true}, nil
}

func readMemoryAt(procRoot, mountInfoPath string, pid int) (Memory, error) {
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

	memory := Memory{
		RSS:       rss,
		HostTotal: readMemTotal(filepath.Join(procRoot, "meminfo")),
	}
	memberships, err := readCgroups(filepath.Join(
		procRoot,
		strconv.Itoa(pid),
		"cgroup",
	))
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
			groupDir := resolveCgroupDir(mount, membership.path)
			switch membership.version {
			case 2:
				memory.CgroupCurrent = readNumber(filepath.Join(groupDir, "memory.current"))
				memory.CgroupLimit = readLimit(filepath.Join(groupDir, "memory.max"), false)
			case 1:
				memory.CgroupCurrent = readNumber(filepath.Join(
					groupDir,
					"memory.usage_in_bytes",
				))
				memory.CgroupLimit = readLimit(filepath.Join(
					groupDir,
					"memory.limit_in_bytes",
				), true)
			}
			if memory.CgroupCurrent.Available || memory.CgroupLimit.Available {
				return memory, nil
			}
		}
	}
	return memory, nil
}

// readMemTotal は /proc/meminfo の MemTotal を返す。読めなければ n/a。
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

func readCgroups(path string) ([]cgroupMembership, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var memberships []cgroupMembership
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), ":", 3)
		if len(fields) != 3 || fields[2] == "" || !filepath.IsAbs(fields[2]) {
			continue
		}
		switch {
		case fields[0] == "0" && fields[1] == "":
			memberships = append(memberships, cgroupMembership{version: 2, path: fields[2]})
		case contains(fields[1], "memory"):
			memberships = append(memberships, cgroupMembership{version: 1, path: fields[2]})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, errors.New("memory cgroupに所属していません")
	}
	return memberships, nil
}

func readCgroupMounts(path string) ([]cgroupMount, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var mounts []cgroupMount
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		separator := index(fields, "-")
		if separator < 6 || separator+3 >= len(fields) {
			continue
		}

		version := 0
		switch fields[separator+1] {
		case "cgroup2":
			version = 2
		case "cgroup":
			if contains(fields[separator+3], "memory") {
				version = 1
			}
		}
		if version == 0 {
			continue
		}
		mounts = append(mounts, cgroupMount{
			version:    version,
			root:       unescapeMountField(fields[3]),
			mountPoint: unescapeMountField(fields[4]),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(mounts) == 0 {
		return nil, errors.New("memory cgroupのマウントがありません")
	}
	return mounts, nil
}

func resolveCgroupDir(mount cgroupMount, group string) string {
	root := filepath.Clean(mount.root)
	group = filepath.Clean(group)
	relative := strings.TrimPrefix(group, string(filepath.Separator))
	if root != string(filepath.Separator) {
		switch {
		case group == root:
			relative = ""
		case strings.HasPrefix(group, root+string(filepath.Separator)):
			relative = strings.TrimPrefix(group, root+string(filepath.Separator))
		}
	}
	return filepath.Join(mount.mountPoint, relative)
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

func readLimit(path string, version1 bool) Limit {
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
	const cgroupV1Unlimited = uint64(1<<63 - 4096)
	if version1 && limit >= cgroupV1Unlimited {
		return Limit{Available: true, Unlimited: true}
	}
	return Limit{Value: limit, Available: true}
}

func contains(list, item string) bool {
	for _, value := range strings.Split(list, ",") {
		if value == item {
			return true
		}
	}
	return false
}

func index(values []string, target string) int {
	for position, value := range values {
		if value == target {
			return position
		}
	}
	return -1
}

func unescapeMountField(value string) string {
	replacer := strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)
	return replacer.Replace(value)
}
