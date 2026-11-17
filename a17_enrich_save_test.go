package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnrichButtonReportsSaveFailure(t *testing.T) {
	vocab := readSourceFile(t, "vocabulary.go")
	if !containsText(vocab, "func (s *VocabularyStore) EnrichAll() (int, error)") {
		t.Fatal("补全仍返回 int：写盘失败被吞掉，设置页会显示假的「已智能补全 N 条 ✓」")
	}
	ui := readSourceFile(t, "settings_vocab.go")
	start := strings.Index(ui, `btnEnrich := ui.Button(c, "补全")`)
	if start < 0 {
		t.Fatal("找不到补全按钮段")
	}
	end := strings.Index(ui[start:], `btnWeb := ui.Button(c, "网页版")`)
	if end < 0 {
		t.Fatal("找不到补全按钮段结尾")
	}
	seg := ui[start : start+end]
	if !containsText(seg, "if err != nil") || !containsText(seg, "补全失败") {
		t.Fatal("补全按钮没有按 err 分支，保存失败仍显示成功 ✓")
	}
}

func TestEnrichAllReportsSaveFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup blocker: %v", err)
	}
	s := &VocabularyStore{
		path:    filepath.Join(blocker, "nested", "vocabulary.json"),
		entries: []VocabularyEntry{{ID: "x", Term: "生词"}},
	}
	n, err := s.EnrichAll()
	if err == nil {
		t.Fatalf("补全在磁盘写入失败时仍报成功（n=%d），用户重启后会丢补全数据", n)
	}
}

func TestEnrichAllSuccessKeepsNoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.json")
	s := &VocabularyStore{
		path:    path,
		entries: []VocabularyEntry{{ID: "x", Term: "生词"}},
	}
	n, err := s.EnrichAll()
	if err != nil || n != 1 {
		t.Fatalf("正常补全 = (%d, %v), want (1, nil)", n, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("补全结果没有落盘: %v", err)
	}
}
