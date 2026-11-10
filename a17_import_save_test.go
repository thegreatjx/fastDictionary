package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestImportFileReportsSaveFailure 录下 A17：导入把磁盘写入失败吞掉，
// 设置页因此显示「成功导入 N 条生词 ✓」，重启后生词全丢。
func TestImportFileReportsSaveFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup blocker: %v", err)
	}
	// 父目录是普通文件：MkdirAll 与 OpenFile 都会失败。
	s := &VocabularyStore{path: filepath.Join(blocker, "nested", "vocabulary.json")}
	src := filepath.Join(dir, "words.json")
	if err := os.WriteFile(src, []byte(`[{"term":"alpha","definition":"d","phonetic":"/a/","difficulty":"A1"}]`), 0o644); err != nil {
		t.Fatalf("setup import file: %v", err)
	}
	n, err := s.ImportFile(src)
	if err == nil {
		t.Fatalf("导入在磁盘写入失败时仍报成功（n=%d），用户重启后会丢生词", n)
	}
}

// TestImportFileSuccessKeepsNoError 是对照：正常路径必须继续返回 nil error。
func TestImportFileSuccessKeepsNoError(t *testing.T) {
	dir := t.TempDir()
	s := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json")}
	src := filepath.Join(dir, "words.json")
	if err := os.WriteFile(src, []byte(`[{"term":"alpha","definition":"d","phonetic":"/a/","difficulty":"A1"}]`), 0o644); err != nil {
		t.Fatalf("setup import file: %v", err)
	}
	n, err := s.ImportFile(src)
	if err != nil || n != 1 {
		t.Fatalf("正常导入 = (%d, %v), want (1, nil)", n, err)
	}
}
