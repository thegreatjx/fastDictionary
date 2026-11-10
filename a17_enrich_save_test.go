package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnrichButtonReportsSaveFailure 是 A17 残余「补全按钮假成功」的源码契约护栏。
// 补全逻辑在 go func() 里、需要真实按钮与窗口，headless 无法触发，故用源码断言
// 钉住「错误必须沿 EnrichAll → 设置页上送」这条数据流：改前失败，改后通过。
func TestEnrichButtonReportsSaveFailure(t *testing.T) {
	vocab := readSourceFile(t, "vocabulary.go")
	if !containsText(vocab, "func (s *VocabularyStore) EnrichAll() (int, error)") {
		t.Fatal("补全仍返回 int：写盘失败被吞掉，设置页会显示假的「已智能补全 N 条 ✓」")
	}
	ui := readSourceFile(t, "settings_vocab.go")
	start := strings.Index(ui, "// [✨ 补全]")
	if start < 0 {
		t.Fatal("找不到补全按钮段")
	}
	end := strings.Index(ui[start:], "// [🌐 网页版]")
	if end < 0 {
		t.Fatal("找不到补全按钮段结尾")
	}
	seg := ui[start : start+end]
	if !containsText(seg, "if err != nil") || !containsText(seg, "补全失败") {
		t.Fatal("补全按钮没有按 err 分支，保存失败仍显示成功 ✓")
	}
}

// TestEnrichAllReportsSaveFailure 是行为级护栏：磁盘写不进去时，补全必须把
// error 交出去，否则设置页会显示「已智能补全 N 条 ✓」，重启后数据全丢。
// 父目录是普通文件 → os.MkdirAll 失败；词条用非单词 Term → EnrichVocabularyEntry
// 立即返回、不触发网络补全，保证测试确定性。
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

// TestEnrichAllSuccessKeepsNoError 是对照：正常路径必须继续返回 nil error，
// 且确实把结果落盘，不能为了修假成功而破坏「补全」这个功能本身。
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
