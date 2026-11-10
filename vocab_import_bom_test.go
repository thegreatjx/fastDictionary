package main

import (
	"os"
	"path/filepath"
	"testing"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func writeVocabImportFile(t *testing.T, name string, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("写入 %s 失败: %v", name, err)
	}
	return path
}

func newImportStore(t *testing.T) *VocabularyStore {
	t.Helper()
	return &VocabularyStore{path: filepath.Join(t.TempDir(), "vocabulary.json")}
}

// TestImportJSONWithUTF8BOM is the regression: Notepad and many exporters put a
// UTF-8 BOM in front of the file, and json.Unmarshal rejects it, so a perfectly
// good export failed with "无法识别的 JSON 数据格式".
func TestImportJSONWithUTF8BOM(t *testing.T) {
	body := append(append([]byte{}, utf8BOM...),
		[]byte(`[{"term":"alpha","definition":"第一个","phonetic":"/a/","difficulty":"A1"}]`)...)
	path := writeVocabImportFile(t, "words.json", body)
	s := newImportStore(t)

	n, err := s.ImportFile(path)
	if err != nil {
		t.Fatalf("带 BOM 的 JSON 应能导入，得到: %v", err)
	}
	if n != 1 || s.Count() != 1 {
		t.Fatalf("导入条数 = %d / 库存 %d，want 1", n, s.Count())
	}
	if !vocabHasTerm(s.entries, "alpha") {
		t.Fatalf("词条 = %+v，BOM 不应混进单词", s.entries)
	}
}

func TestImportTXTWithUTF8BOM(t *testing.T) {
	body := append(append([]byte{}, utf8BOM...), []byte("alpha\t第一个\nbeta\t第二个\n")...)
	path := writeVocabImportFile(t, "words.txt", body)
	s := newImportStore(t)

	if _, err := s.ImportFile(path); err != nil {
		t.Fatalf("带 BOM 的 TXT 应能导入: %v", err)
	}
	if !vocabHasTerm(s.entries, "alpha") || !vocabHasTerm(s.entries, "beta") {
		t.Fatalf("词条 = %+v，BOM 不应混进单词", s.entries)
	}
}

func TestImportCSVWithUTF8BOMStillWorks(t *testing.T) {
	body := append(append([]byte{}, utf8BOM...), []byte("term,definition\nalpha,第一个\n")...)
	path := writeVocabImportFile(t, "words.csv", body)
	s := newImportStore(t)

	if _, err := s.ImportFile(path); err != nil {
		t.Fatalf("带 BOM 的 CSV 应能导入: %v", err)
	}
	if !vocabHasTerm(s.entries, "alpha") {
		t.Fatalf("词条 = %+v", s.entries)
	}
}

func vocabHasTerm(entries []VocabularyEntry, term string) bool {
	for _, e := range entries {
		if e.Term == term {
			return true
		}
	}
	return false
}
