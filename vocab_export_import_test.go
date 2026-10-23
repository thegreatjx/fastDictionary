package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportImportVocabulary(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fastpot_vocab_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &VocabularyStore{
		path: filepath.Join(tmpDir, "test_vocab.json"),
		entries: []VocabularyEntry{
			{
				ID:           "test_1",
				Term:         "blossom",
				Phonetic:     "/ˈblɒs.əm/",
				PartOfSpeech: "noun",
				Definition:   "花；花朵",
				Difficulty:   "B2",
				Source:       "dict",
			},
			{
				ID:           "test_2",
				Term:         "ambitious",
				Phonetic:     "/æmˈbɪʃ.əs/",
				PartOfSpeech: "adj.",
				Definition:   "有雄心的",
				Difficulty:   "B2",
				Source:       "notes",
			},
		},
	}

	// 1. Test ExportJSON
	jsonPath := filepath.Join(tmpDir, "export.json")
	if err := store.ExportJSON(jsonPath); err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}
	jsonBytes, err := os.ReadFile(jsonPath)
	if err != nil || !strings.Contains(string(jsonBytes), "ambitious") {
		t.Fatalf("JSON export content invalid")
	}

	// 2. Test ExportCSV
	csvPath := filepath.Join(tmpDir, "export.csv")
	if err := store.ExportCSV(csvPath); err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}
	csvBytes, err := os.ReadFile(csvPath)
	if err != nil || !strings.Contains(string(csvBytes), "ambitious") {
		t.Fatalf("CSV export content invalid")
	}
	// Verify UTF-8 BOM
	if len(csvBytes) < 3 || csvBytes[0] != 0xEF || csvBytes[1] != 0xBB || csvBytes[2] != 0xBF {
		t.Fatalf("CSV missing UTF-8 BOM")
	}

	// 3. Test Import into new store
	newStore := &VocabularyStore{
		path: filepath.Join(tmpDir, "imported_vocab.json"),
	}
	count, err := newStore.ImportFile(jsonPath)
	if err != nil {
		t.Fatalf("ImportFile JSON failed: %v", err)
	}
	if count != 2 || newStore.Count() != 2 {
		t.Fatalf("expected 2 imported items, got count=%d store=%d", count, newStore.Count())
	}

	// 4. Test Import from CSV
	csvStore := &VocabularyStore{
		path: filepath.Join(tmpDir, "csv_imported_vocab.json"),
	}
	countCSV, err := csvStore.ImportFile(csvPath)
	if err != nil {
		t.Fatalf("ImportFile CSV failed: %v", err)
	}
	if countCSV != 2 || csvStore.Count() != 2 {
		t.Fatalf("expected 2 items from CSV, got count=%d store=%d", countCSV, csvStore.Count())
	}

	// 5. Test Import from TXT
	txtPath := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(txtPath, []byte("legendary\t传奇的\nephemeral - 短暂的\ncherish: 珍惜"), 0o644)
	txtStore := &VocabularyStore{
		path: filepath.Join(tmpDir, "txt_imported_vocab.json"),
	}
	countTXT, err := txtStore.ImportFile(txtPath)
	if err != nil {
		t.Fatalf("ImportFile TXT failed: %v", err)
	}
	if countTXT != 3 {
		t.Fatalf("expected 3 items from TXT, got %d", countTXT)
	}
	// Verify auto-enrichment occurred on imported TXT words
	legEntry := txtStore.Search("legendary")
	if len(legEntry) == 0 || legEntry[0].Phonetic == "" || legEntry[0].Difficulty == "" {
		t.Fatalf("expected legendary to be enriched with phonetic and difficulty")
	}
}
