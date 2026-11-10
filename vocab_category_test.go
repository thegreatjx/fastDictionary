package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestClassifyVocabEntry pins the single classification rule: the source wins,
// and only entries without a known source fall back to the word count.
func TestClassifyVocabEntry(t *testing.T) {
	cases := []struct {
		name   string
		term   string
		source string
		want   vocabCategory
	}{
		{"writing long stays writing", "this is a polished sentence", "writing", vocabCategoryWriting},
		{"writing short stays writing", "polished text", "writing", vocabCategoryWriting},
		{"grammar short is a sentence", "look up", "grammar", vocabCategorySentences},
		{"grammar long is a sentence", "a much longer grammar example here", "grammar", vocabCategorySentences},
		{"dict long is a word", "a very long dictionary term", "dict", vocabCategoryWords},
		{"notes long is a word", "a very long notes term", "notes", vocabCategoryWords},
		{"import long is a sentence", "imported sentence with many words", "import", vocabCategorySentences},
		{"import short is a word", "imported", "import", vocabCategoryWords},
		{"unknown source long is a sentence", "one two three", "", vocabCategorySentences},
		{"unknown source short is a word", "one two", "", vocabCategoryWords},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyVocabEntry(VocabularyEntry{Term: tc.term, Source: tc.source})
			if got != tc.want {
				t.Fatalf("classifyVocabEntry(%q, source=%q) = %s, want %s", tc.term, tc.source, got, tc.want)
			}
		})
	}
}

// TestVocabCategorySurfacesAgree is the regression for the three divergent
// copies of the rule: counters, filters, row badges and the exported notebook
// page must all put a writing entry with a long term in 写作批改.
func TestVocabCategorySurfacesAgree(t *testing.T) {
	entries := []VocabularyEntry{
		{ID: "1", Term: "alpha", Source: "dict"},
		{ID: "2", Term: "a longer sentence entry", Source: "grammar"},
		{ID: "3", Term: "this is a polished sentence", Source: "writing"},
		{ID: "4", Term: "this is a polished sentence too", Source: "writing"},
		{ID: "5", Term: "beta", Source: "notes"},
		{ID: "6", Term: "imported sentence with many words", Source: "import"},
	}
	dir := t.TempDir()
	s := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json"), entries: entries}

	all, words, sentences, writing := s.CategoryCounts()
	if all != len(entries) {
		t.Fatalf("CategoryCounts all = %d, want %d", all, len(entries))
	}

	// Every entry must appear in exactly one tab, and the per-tab sizes must
	// equal the counters.
	wantCounts := map[string]int{"words": 0, "sentences": 0, "writing": 0}
	for _, e := range entries {
		hits := 0
		for _, key := range []string{"words", "sentences", "writing"} {
			found := false
			for _, got := range s.Filter(key, "") {
				if got.ID == e.ID {
					found = true
					break
				}
			}
			if found {
				hits++
				wantCounts[key]++
			}
		}
		if hits != 1 {
			t.Fatalf("entry %s (%q, source=%q) appears in %d tabs, want exactly 1", e.ID, e.Term, e.Source, hits)
		}
	}
	if words != wantCounts["words"] || sentences != wantCounts["sentences"] || writing != wantCounts["writing"] {
		t.Fatalf("counters = words:%d sentences:%d writing:%d, filters = %v", words, sentences, writing, wantCounts)
	}

	// The exported page must carry the same category on each card.
	path, err := s.ExportHTML()
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported page: %v", err)
	}
	page := string(data)
	var cats []string
	rest := page
	const marker = `<div class="card" data-cat="`
	for {
		i := strings.Index(rest, marker)
		if i < 0 {
			break
		}
		rest = rest[i+len(marker):]
		j := strings.Index(rest, `"`)
		if j < 0 {
			break
		}
		cats = append(cats, rest[:j])
	}
	if len(cats) != len(entries) {
		t.Fatalf("exported page has %d cards, want %d", len(cats), len(entries))
	}
	for i, e := range entries {
		if want := classifyVocabEntry(e).String(); cats[i] != want {
			t.Fatalf("card %d (%q, source=%q) has data-cat %q, want %q", i, e.Term, e.Source, cats[i], want)
		}
	}
	if !strings.Contains(page, "let allCats = ") {
		t.Fatal("exported page does not pass the canonical categories to its scripts")
	}
}

// TestVocabBadgesUseCanonicalClassifier keeps the row badge and the export
// from growing their own copy of the rule again.
func TestVocabBadgesUseCanonicalClassifier(t *testing.T) {
	settingsSrc := readSourceFile(t, "settings_vocab.go")
	if !containsText(settingsSrc, "switch classifyVocabEntry(entry)") {
		t.Fatal("生词本行标签没有使用 classifyVocabEntry")
	}
	if containsText(settingsSrc, `entry.Source == "grammar" || len(strings.Fields(entry.Term)) > 2`) {
		t.Fatal("生词本行标签仍自己判断 grammar 与词数")
	}
	exportSrc := readSourceFile(t, "vocabulary_export.go")
	if containsText(exportSrc, `if e.Source == "grammar" || len(strings.Fields(e.Term)) > 2`) {
		t.Fatal("ExportHTML 仍自己判断 grammar 与词数")
	}
	if !containsText(exportSrc, "classifyVocabEntry(e)") {
		t.Fatal("ExportHTML 没有使用 classifyVocabEntry")
	}
}

func TestVocabCategoryKeys(t *testing.T) {
	for key, want := range map[string]vocabCategory{"words": vocabCategoryWords, "sentences": vocabCategorySentences, "writing": vocabCategoryWriting} {
		got, ok := vocabCategoryFromKey(key)
		if !ok || got != want {
			t.Fatalf("vocabCategoryFromKey(%q) = %v, %v; want %v, true", key, got, ok, want)
		}
		if got.String() != key {
			t.Fatalf("%v.String() = %q, want %q", got, got.String(), key)
		}
	}
	if _, ok := vocabCategoryFromKey("all"); ok {
		t.Fatal("all must not be a concrete category")
	}
	if _, ok := vocabCategoryFromKey("bogus"); ok {
		t.Fatal("bogus must not be a concrete category")
	}
}
