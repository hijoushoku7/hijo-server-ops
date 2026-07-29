package procstats

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReadMemoryCgroupV2FromNonstandardMount(t *testing.T) {
	procRoot := t.TempDir()
	cgroupRoot := filepath.Join(t.TempDir(), "custom cgroup")
	writeProcessFile(t, procRoot, 123, "status", "Name:\tjava\nVmRSS:\t12345 kB\n")
	writeProcessFile(t, procRoot, 123, "cgroup", "0::/server.slice/minecraft\n")

	groupDir := filepath.Join(cgroupRoot, "server.slice", "minecraft")
	writeCgroupFile(t, groupDir, "memory.current", "23456789\n")
	writeCgroupFile(t, groupDir, "memory.max", "1073741824\n")
	mountInfo := writeMountInfo(t, procRoot, fmt.Sprintf(
		"1 1 0:1 / %s rw - cgroup2 cgroup rw\n",
		escapeMountField(cgroupRoot),
	))

	memory, err := readMemoryAt(procRoot, mountInfo, 123)
	if err != nil {
		t.Fatal(err)
	}
	assertNumber(t, memory.RSS, 12345*1024)
	assertNumber(t, memory.CgroupCurrent, 23456789)
	if !memory.CgroupLimit.Available ||
		memory.CgroupLimit.Unlimited ||
		memory.CgroupLimit.Value != 1073741824 {
		t.Fatalf("CgroupLimit = %#v", memory.CgroupLimit)
	}
}

func TestReadMemoryCurrentProcess(t *testing.T) {
	memory, err := ReadMemory(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if !memory.RSS.Available || memory.RSS.Value == 0 {
		t.Fatalf("RSS = %#v", memory.RSS)
	}
}

func TestReadCPUTimeCurrentProcess(t *testing.T) {
	cpuTime, err := ReadCPUTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if !cpuTime.Available || cpuTime.Value < 0 {
		t.Fatalf("CPU time = %#v", cpuTime)
	}
}

func TestParseCPUTimeAllowsParenthesesInCommand(t *testing.T) {
	fields := make([]string, 13)
	for index := range fields {
		fields[index] = "0"
	}
	fields[0] = "S"
	fields[11] = "125"
	fields[12] = "75"

	cpuTime, err := parseCPUTime([]byte(
		"123 (server ) name) " + strings.Join(fields, " ") + "\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	if !cpuTime.Available || cpuTime.Value != 2*time.Second {
		t.Fatalf("CPU time = %#v", cpuTime)
	}
}

func TestReadMemoryCgroupV1(t *testing.T) {
	procRoot := t.TempDir()
	cgroupRoot := filepath.Join(t.TempDir(), "memory")
	writeProcessFile(t, procRoot, 123, "status", "VmRSS:\t10 kB\n")
	writeProcessFile(t, procRoot, 123, "cgroup", "5:cpu:/server\n6:memory:/server\n")

	groupDir := filepath.Join(cgroupRoot, "server")
	writeCgroupFile(t, groupDir, "memory.usage_in_bytes", "200\n")
	writeCgroupFile(t, groupDir, "memory.limit_in_bytes", "500\n")
	mountInfo := writeMountInfo(t, procRoot, fmt.Sprintf(
		"2 1 0:2 / %s rw - cgroup cgroup rw,memory\n",
		escapeMountField(cgroupRoot),
	))

	memory, err := readMemoryAt(procRoot, mountInfo, 123)
	if err != nil {
		t.Fatal(err)
	}
	assertNumber(t, memory.CgroupCurrent, 200)
	if !memory.CgroupLimit.Available ||
		memory.CgroupLimit.Unlimited ||
		memory.CgroupLimit.Value != 500 {
		t.Fatalf("CgroupLimit = %#v", memory.CgroupLimit)
	}
}

func TestReadMemoryReportsUnlimitedCgroup(t *testing.T) {
	procRoot := t.TempDir()
	cgroupRoot := t.TempDir()
	writeProcessFile(t, procRoot, 123, "status", "VmRSS:\t1 kB\n")
	writeProcessFile(t, procRoot, 123, "cgroup", "0::/\n")
	writeCgroupFile(t, cgroupRoot, "memory.current", "100\n")
	writeCgroupFile(t, cgroupRoot, "memory.max", "max\n")
	mountInfo := writeMountInfo(t, procRoot, fmt.Sprintf(
		"1 1 0:1 / %s rw - cgroup2 cgroup rw\n",
		escapeMountField(cgroupRoot),
	))

	memory, err := readMemoryAt(procRoot, mountInfo, 123)
	if err != nil {
		t.Fatal(err)
	}
	if !memory.CgroupLimit.Available || !memory.CgroupLimit.Unlimited {
		t.Fatalf("CgroupLimit = %#v", memory.CgroupLimit)
	}
}

func TestReadMemoryKeepsCgroupUnavailable(t *testing.T) {
	procRoot := t.TempDir()
	writeProcessFile(t, procRoot, 123, "status", "VmRSS:\t10 kB\n")
	writeProcessFile(t, procRoot, 123, "cgroup", "")
	mountInfo := writeMountInfo(t, procRoot, "")

	memory, err := readMemoryAt(procRoot, mountInfo, 123)
	if err != nil {
		t.Fatal(err)
	}
	assertNumber(t, memory.RSS, 10*1024)
	if memory.CgroupCurrent.Available || memory.CgroupLimit.Available {
		t.Fatalf("Memory = %#v", memory)
	}
}

func TestReadMemoryKeepsMissingRSSUnavailable(t *testing.T) {
	procRoot := t.TempDir()
	writeProcessFile(t, procRoot, 123, "status", "Name:\tjava\n")
	writeProcessFile(t, procRoot, 123, "cgroup", "")
	mountInfo := writeMountInfo(t, procRoot, "")

	memory, err := readMemoryAt(procRoot, mountInfo, 123)
	if err != nil {
		t.Fatal(err)
	}
	if memory.RSS.Available {
		t.Fatalf("RSS = %#v", memory.RSS)
	}
}

func TestReadMemoryRejectsMalformedRSS(t *testing.T) {
	procRoot := t.TempDir()
	writeProcessFile(t, procRoot, 123, "status", "VmRSS:\tunknown kB\n")

	if _, err := readMemoryAt(procRoot, "", 123); err == nil {
		t.Fatal("readMemoryAt succeeded")
	}
}

func TestReadMemTotalConvertsKibibytes(t *testing.T) {
	procRoot := t.TempDir()
	writeProcessFile(t, procRoot, 123, "status", "VmRSS:\t10 kB\n")
	writeProcessFile(t, procRoot, 123, "cgroup", "")
	mountInfo := writeMountInfo(t, procRoot, "")
	meminfo := filepath.Join(procRoot, "meminfo")
	content := "MemTotal:       16384000 kB\nMemFree:         1000 kB\n"
	if err := os.WriteFile(meminfo, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	memory, err := readMemoryAt(procRoot, mountInfo, 123)
	if err != nil {
		t.Fatal(err)
	}
	assertNumber(t, memory.HostTotal, 16384000*1024)
}

func TestReadMemTotalKeepsUnreadableValuesUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing file"},
		{name: "no MemTotal", content: "MemFree: 1000 kB\n"},
		{name: "wrong unit", content: "MemTotal: 16384000 MB\n"},
		{name: "missing unit", content: "MemTotal: 16384000\n"},
		{name: "not a number", content: "MemTotal: unknown kB\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "meminfo")
			if test.content != "" {
				if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if got := readMemTotal(path); got.Available {
				t.Fatalf("HostTotal = %#v", got)
			}
		})
	}
}

func TestResolveCgroupMountRoot(t *testing.T) {
	mount := cgroupMount{
		root:       "/parent",
		mountPoint: "/sys/fs/cgroup",
	}
	got := resolveCgroupDir(mount, "/parent/server")
	if got != "/sys/fs/cgroup/server" {
		t.Fatalf("path = %q", got)
	}
}

func writeProcessFile(t *testing.T, root string, pid int, name, content string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeCgroupFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeMountInfo(t *testing.T, root, content string) string {
	t.Helper()
	path := filepath.Join(root, "mountinfo")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func escapeMountField(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\134`,
		" ", `\040`,
		"\t", `\011`,
		"\n", `\012`,
	)
	return replacer.Replace(value)
}

func assertNumber(t *testing.T, got Number, want uint64) {
	t.Helper()
	if !got.Available || got.Value != want {
		t.Fatalf("Number = %#v, want %d", got, want)
	}
}
