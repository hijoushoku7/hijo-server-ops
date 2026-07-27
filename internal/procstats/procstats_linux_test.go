package procstats

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestReadMemory(t *testing.T) {
	procRoot := t.TempDir()
	cgroupRoot := t.TempDir()
	writeProcessFile(t, procRoot, 123, "status", "Name:\tjava\nVmRSS:\t12345 kB\n")
	writeProcessFile(t, procRoot, 123, "cgroup", "0::/server.slice/minecraft\n")

	groupDir := filepath.Join(cgroupRoot, "server.slice", "minecraft")
	if err := os.MkdirAll(groupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(groupDir, "memory.current"),
		[]byte("23456789\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(groupDir, "memory.max"),
		[]byte("1073741824\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	memory, err := readMemoryAt(procRoot, cgroupRoot, 123)
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

func TestReadMemoryReportsUnlimitedCgroup(t *testing.T) {
	procRoot := t.TempDir()
	cgroupRoot := t.TempDir()
	writeProcessFile(t, procRoot, 123, "status", "VmRSS:\t1 kB\n")
	writeProcessFile(t, procRoot, 123, "cgroup", "0::/\n")
	if err := os.WriteFile(filepath.Join(cgroupRoot, "memory.current"), []byte("100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cgroupRoot, "memory.max"), []byte("max\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	memory, err := readMemoryAt(procRoot, cgroupRoot, 123)
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
	writeProcessFile(t, procRoot, 123, "cgroup", "5:memory:/server\n")

	memory, err := readMemoryAt(procRoot, t.TempDir(), 123)
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

	memory, err := readMemoryAt(procRoot, t.TempDir(), 123)
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

	if _, err := readMemoryAt(procRoot, t.TempDir(), 123); err == nil {
		t.Fatal("readMemoryAt succeeded")
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

func assertNumber(t *testing.T, got Number, want uint64) {
	t.Helper()
	if !got.Available || got.Value != want {
		t.Fatalf("Number = %#v, want %d", got, want)
	}
}
