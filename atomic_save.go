package main

import (
	"os"
	"path/filepath"
	"time"
)

var renameFile = os.Rename

type atomicWriteOptions struct {
	// Windows 索引/杀软会短暂占用刚关闭的文件，rename 需要重试。
	retries int
	backup  bool
}

func atomicWriteFile(path string, data []byte, opts atomicWriteOptions) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	closed := false
	cleanup := func() {
		if !closed {
			_ = f.Close()
		}
		_ = os.Remove(tmp)
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		closed = true
		_ = os.Remove(tmp)
		return err
	}
	closed = true

	if opts.backup {
		if prev, readErr := os.ReadFile(path); readErr == nil && len(prev) > 0 {
			_ = os.WriteFile(path+".bak", prev, 0o644)
		}
	}

	retries := opts.retries
	if retries < 1 {
		retries = 1
	}
	var renameErr error
	for attempt := 0; attempt < retries; attempt++ {
		if renameErr = renameFile(tmp, path); renameErr == nil {
			return nil
		}
		if attempt < retries-1 {
			time.Sleep(time.Duration(10+attempt*8) * time.Millisecond)
		}
	}
	_ = os.Remove(tmp)
	return renameErr
}
