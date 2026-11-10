package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func swapRenameFile(t *testing.T, fn func(oldpath, newpath string) error) {
	t.Helper()
	old := renameFile
	renameFile = fn
	t.Cleanup(func() { renameFile = old })
}

// transientRenameFailure fails the first n renames, then delegates. It models
// the Windows sharing violation that used to make a wordbook save fail outright.
func transientRenameFailure(t *testing.T, n int) (*int, func(string, string) error) {
	t.Helper()
	var mu sync.Mutex
	attempts := 0
	return &attempts, func(oldpath, newpath string) error {
		mu.Lock()
		attempts++
		count := attempts
		mu.Unlock()
		if count <= n {
			return os.ErrPermission
		}
		return os.Rename(oldpath, newpath)
	}
}

func TestVocabularySaveRetriesTransientRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.json")
	attempts, fake := transientRenameFailure(t, 2)
	swapRenameFile(t, fake)

	s := &VocabularyStore{path: path, entries: []VocabularyEntry{{Term: "alpha", Phonetic: "/a/", Difficulty: "A1"}}}
	if err := s.save(); err != nil {
		t.Fatalf("词库保存遇到瞬时占用应重试成功，却直接失败: %v", err)
	}
	if *attempts < 3 {
		t.Fatalf("应至少重试到第三次才成功，实际 rename 次数 %d", *attempts)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("重试成功后文件不存在: %v", err)
	}
}

func TestConfigSaveRetriesTransientRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	attempts, fake := transientRenameFailure(t, 2)
	swapRenameFile(t, fake)

	if err := saveConfig(path, DefaultConfig()); err != nil {
		t.Fatalf("配置保存遇到瞬时占用应重试成功，却直接失败: %v", err)
	}
	if *attempts < 3 {
		t.Fatalf("应至少重试到第三次才成功，实际 rename 次数 %d", *attempts)
	}
}

func TestAtomicWriteBackupOnlyWhenRequested(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.json")
	if err := os.WriteFile(path, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := atomicWriteFile(path, []byte("first"), atomicWriteOptions{retries: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatal("未要求备份时不应产生 .bak")
	}

	if err := os.WriteFile(path, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteFile(path, []byte("second"), atomicWriteOptions{retries: 1, backup: true}); err != nil {
		t.Fatal(err)
	}
	prev, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("要求备份时应保留上一份内容: %v", err)
	}
	if string(prev) != "previous" {
		t.Fatalf(".bak 内容应为 previous，实际 %q", string(prev))
	}
}

func TestAtomicWriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := atomicWriteFile(path, []byte("{}"), atomicWriteOptions{retries: 2}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Fatalf("保存后残留临时文件: %s", e.Name())
		}
	}
}
