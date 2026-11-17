package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportFileReportsSaveFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup blocker: %v", err)
	}
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
