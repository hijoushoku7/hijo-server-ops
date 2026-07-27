package hsperfdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

var ErrNotFound = errors.New("hsperfdataが見つかりません")

type Reader struct {
	file *os.File
	data []byte
}

func Open(pid int) (*Reader, error) {
	return openAt(os.TempDir(), pid)
}

func openAt(tempDir string, pid int) (*Reader, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("PIDが不正です: %d", pid)
	}
	matches, err := filepath.Glob(filepath.Join(
		tempDir,
		"hsperfdata_*",
		strconv.Itoa(pid),
	))
	if err != nil {
		return nil, fmt.Errorf("hsperfdataを探す: %w", err)
	}
	for _, path := range matches {
		reader, err := openPath(path)
		if err == nil {
			return reader, nil
		}
	}
	return nil, fmt.Errorf("%w: PID %d", ErrNotFound, pid)
}

func openPath(path string) (*Reader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("hsperfdataを開く: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("hsperfdataのサイズを読む: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < prologueSize {
		_ = file.Close()
		return nil, errors.New("hsperfdataが通常ファイルではないか、短すぎます")
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
		return nil, fmt.Errorf("hsperfdataをmmapする: %w", err)
	}
	return &Reader{file: file, data: data}, nil
}

func (r *Reader) Sample() (Snapshot, error) {
	if r == nil || r.data == nil {
		return Snapshot{}, errors.New("hsperfdataリーダーは閉じられています")
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
