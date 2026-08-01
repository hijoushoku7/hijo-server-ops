package hsperfdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

var ErrNotFound = errors.New("hsperfdata not found")

const hotSpotTempDir = "/tmp"

type Reader struct {
	file *os.File
	data []byte
}

func Open(pid int) (*Reader, error) {
	return openAt(hotSpotTempDir, pid)
}

func openAt(tempDir string, pid int) (*Reader, error) {
	return openAtUID(tempDir, pid, uint32(os.Geteuid()))
}

func openAtUID(tempDir string, pid int, uid uint32) (*Reader, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("invalid PID: %d", pid)
	}
	matches, err := filepath.Glob(filepath.Join(
		tempDir,
		"hsperfdata_*",
		strconv.Itoa(pid),
	))
	if err != nil {
		return nil, fmt.Errorf("look for hsperfdata: %w", err)
	}
	var candidateErr error
	for _, path := range matches {
		if !ownedBy(filepath.Dir(path), uid) {
			continue
		}
		reader, err := openPath(path, uid)
		if err == nil {
			return reader, nil
		}
		if candidateErr == nil {
			candidateErr = err
		}
	}
	if candidateErr != nil {
		return nil, candidateErr
	}
	return nil, fmt.Errorf("%w: PID %d", ErrNotFound, pid)
}

func openPath(path string, uid uint32) (*Reader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open hsperfdata: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("read hsperfdata size: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < prologueSize || !fileInfoOwnedBy(info, uid) {
		_ = file.Close()
		return nil, errors.New("unexpected hsperfdata type, size or owner")
	}

	data, err := syscall.Mmap(
		int(file.Fd()),
		0,
		int(info.Size()),
		syscall.PROT_READ,
		syscall.MAP_SHARED,
	)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("mmap hsperfdata: %w", err)
	}
	return &Reader{file: file, data: data}, nil
}

func (r *Reader) Sample() (Snapshot, error) {
	if r == nil || r.data == nil {
		return Snapshot{}, errors.New("hsperfdata reader is closed")
	}
	return Parse(r.data)
}

func (r *Reader) Close() error {
	if r == nil {
		return nil
	}
	var unmapErr error
	if r.data != nil {
		unmapErr = syscall.Munmap(r.data)
		r.data = nil
	}
	var closeErr error
	if r.file != nil {
		closeErr = r.file.Close()
		r.file = nil
	}
	return errors.Join(unmapErr, closeErr)
}

func ownedBy(path string, uid uint32) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && fileInfoOwnedBy(info, uid)
}

func fileInfoOwnedBy(info os.FileInfo, uid uint32) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uid
}
