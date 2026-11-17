package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVocabularyStoreLastSaveError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("准备阻塞文件失败: %v", err)
	}
	badPath := filepath.Join(blocker, "nested", "vocabulary.json")

	s := &VocabularyStore{path: badPath}
	if err := s.LastSaveError(); err != nil {
		t.Fatalf("新建的词库不应带历史保存错误: %v", err)
	}

	s.Add(VocabularyEntry{Term: "alpha", Phonetic: "/a/", Difficulty: "A1", Definition: "第一个"})
	if s.Count() != 1 {
		t.Fatalf("内存中的词条数 = %d，期望 1", s.Count())
	}
	if err := s.LastSaveError(); err == nil {
		t.Fatal("磁盘写入失败后 LastSaveError 仍为 nil：界面会误报保存成功")
	}

	if !s.Delete(s.entries[0].ID) {
		t.Fatal("删除刚加入的词条失败")
	}
	if err := s.LastSaveError(); err == nil {
		t.Fatal("删除写盘失败后 LastSaveError 仍为 nil")
	}
}

func TestVocabularyStoreLastSaveErrorClearsOnSuccess(t *testing.T) {
	dir := t.TempDir()
	s := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json")}
	s.Add(VocabularyEntry{Term: "beta", Phonetic: "/b/", Difficulty: "A2", Definition: "第二个"})
	if err := s.LastSaveError(); err != nil {
		t.Fatalf("正常写盘后 LastSaveError = %v，期望 nil", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vocabulary.json")); err != nil {
		t.Fatalf("词库文件没有落盘: %v", err)
	}
}

func TestQuickSaveSurfacesPersistenceFailure(t *testing.T) {
	actions := readSourceFile(t, "popup_actions.go")
	if got := countOccurrences(actions, "vocabStore.LastSaveError()"); got != 3 {
		t.Fatalf("卡片快速保存检查 LastSaveError 的次数 = %d，期望 3", got)
	}
	if got := countOccurrences(actions, `a.saveErr = "保存失败: "`); got != 3 {
		t.Fatalf("快速保存失败提示次数 = %d，期望 3", got)
	}

	settings := readSourceFile(t, "settings_vocab.go")
	if got := countOccurrences(settings, "vocabStore.LastSaveError()"); got != 2 {
		t.Fatalf("设置页撤销/删除检查 LastSaveError 的次数 = %d，期望 2", got)
	}
	if !containsText(settings, "恢复失败: ") || !containsText(settings, "删除未写入磁盘: ") {
		t.Fatal("设置页撤销或删除缺少失败提示")
	}
}
