package main

import (
	"os"
	"path/filepath"
	"testing"
)

const corruptVocabJSON = `[{"id":"keep-1","term":"alpha"`

func writeCorruptVocabulary(t *testing.T, path string) []byte {
	t.Helper()
	data := []byte(corruptVocabJSON)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("写损坏词库失败: %v", err)
	}
	return data
}

func TestCorruptVocabularyKeepsRecoverableCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.json")
	original := writeCorruptVocabulary(t, path)

	s := &VocabularyStore{path: path, entries: []VocabularyEntry{}}
	s.load()

	if got := s.Count(); got != 0 {
		t.Fatalf("损坏词库加载后 count = %d, want 0（保持既有「空库启动」行为）", got)
	}
	recovery := path + ".corrupt"
	kept, err := os.ReadFile(recovery)
	if err != nil {
		t.Fatalf("损坏的词库没有保留可恢复副本 %s: %v", recovery, err)
	}
	if string(kept) != string(original) {
		t.Fatal("恢复副本与损坏原文不一致")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取原文件失败: %v", err)
	}
	if string(current) != string(original) {
		t.Fatal("load() 改动了损坏原文件")
	}
}

func TestRecoveryCopySurvivesLaterSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.json")
	original := writeCorruptVocabulary(t, path)

	s := &VocabularyStore{path: path, entries: []VocabularyEntry{}}
	s.load()

	for _, term := range []string{"alpha", "beta"} {
		s.Add(VocabularyEntry{Term: term, Phonetic: "/p/", Difficulty: "A1"})
	}

	kept, err := os.ReadFile(path + ".corrupt")
	if err != nil {
		t.Fatalf("加词写盘后恢复副本丢失: %v", err)
	}
	if string(kept) != string(original) {
		t.Fatal("加词写盘后恢复副本被覆盖，损坏原文不可恢复")
	}
}

func TestIntactVocabularyLoadsWithoutRecoveryCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.json")
	intact := `[{"id":"keep-1","term":"alpha","phonetic":"/a/","definition":"d","difficulty":"A1"}]`
	if err := os.WriteFile(path, []byte(intact), 0o644); err != nil {
		t.Fatalf("写合法词库失败: %v", err)
	}
	s := &VocabularyStore{path: path, entries: []VocabularyEntry{}}
	s.load()
	if got := s.Count(); got != 1 {
		t.Fatalf("合法词库 count = %d, want 1", got)
	}
	if _, err := os.Stat(path + ".corrupt"); !os.IsNotExist(err) {
		t.Fatal("合法词库不应产生恢复副本")
	}
}

func TestMissingVocabularyNeedsNoRecoveryCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.json")
	s := &VocabularyStore{path: path, entries: []VocabularyEntry{}}
	s.load()
	if _, err := os.Stat(path + ".corrupt"); !os.IsNotExist(err) {
		t.Fatal("文件不存在时不应产生恢复副本")
	}
}
