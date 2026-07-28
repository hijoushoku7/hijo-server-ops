package hsperfdata

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReaderMapsPerfDataFile(t *testing.T) {
	tempDir := t.TempDir()
	perfDir := filepath.Join(tempDir, "hsperfdata_test")
	if err := os.Mkdir(perfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(perfDir, "123")
	data := perfData(t, binary.LittleEndian, longEntry("java.threads.live", 9))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	reader, err := openAt(tempDir, 123)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	snapshot, err := reader.Sample()
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := snapshot.Long("java.threads.live"); !ok || value != 9 {
		t.Fatalf("threads = %d, %v", value, ok)
	}
}

func TestHotSpotTempDirDoesNotFollowTMPDIR(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	if hotSpotTempDir != "/tmp" {
		t.Fatalf("hotSpotTempDir = %q", hotSpotTempDir)
	}
}

func TestReaderReportsMissingFile(t *testing.T) {
	_, err := openAt(t.TempDir(), 123)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestReaderPreservesCandidateError(t *testing.T) {
	tempDir := t.TempDir()
	perfDir := filepath.Join(tempDir, "hsperfdata_test")
	if err := os.Mkdir(perfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(perfDir, "123"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := openAt(tempDir, 123)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "種類、サイズ、所有者") {
		t.Fatalf("err = %v", err)
	}
}

func TestReaderIgnoresFilesOwnedByAnotherUID(t *testing.T) {
	tempDir := t.TempDir()
	perfDir := filepath.Join(tempDir, "hsperfdata_other")
	if err := os.Mkdir(perfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(perfDir, "123"),
		perfData(t, binary.LittleEndian, longEntry("counter", 1)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := openAtUID(tempDir, 123, uint32(os.Geteuid()+1))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestClosedReaderCannotSample(t *testing.T) {
	tempDir := t.TempDir()
	perfDir := filepath.Join(tempDir, "hsperfdata_test")
	if err := os.Mkdir(perfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(perfDir, "123")
	if err := os.WriteFile(
		path,
		perfData(t, binary.LittleEndian, longEntry("counter", 1)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	reader, err := openAt(tempDir, 123)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Sample(); err == nil {
		t.Fatal("Sample succeeded after Close")
	}
}
