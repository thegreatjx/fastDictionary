package main

import (
	"os"
	"path/filepath"
	"time"
)

// renameFile is the last step of every atomic save. It is a variable so the
// retry loop can be exercised without relying on a real sharing violation.
var renameFile = os.Rename

type atomicWriteOptions struct {
	// retries counts the rename attempts. Windows denies a rename while an
	// indexer or antivirus scanner still holds the just-closed file, so the
	// default is a short retry instead of losing the user's data.
	retries int
	// backup keeps the previous good copy next to the file before replacing it.
	backup bool
}

// atomicWriteFile writes data to a unique temp file in the target directory,
// flushes it, then renames it into place. A unique temp name per call means two
// concurrent saves can never write into the same file and rename a half-written
// mixture; the retry loop absorbs transient sharing violations on Windows.
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
			// A failed backup must not block the save: the new file is still
			// better than dropping the user's change.
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
