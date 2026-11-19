package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNotebookStoreCRUDAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notebook.json")
	s := &NotebookStore{path: path, entries: []NotebookEntry{}}

	now := time.Now().Round(time.Second)
	ok := s.Add(NotebookEntry{
		Kind:        "grammar",
		Source:      "Japan's dispute with China has spread.",
		Translation: "日本与中国的争端已经蔓延。",
		Grammar: &SentenceAnalysisResult{
			BackboneSummary: "争端已经蔓延。",
			Translation:     "日本与中国的争端已经蔓延。",
			Phrases: []PhraseInfo{
				{ID: "p1", Type: "介词短语", ShortTag: "介短", Role: "后置定语", Target: "修饰 dispute"},
			},
		},
		CreatedAt: now,
	})
	if !ok {
		t.Fatal("Add grammar failed")
	}
	if !s.Contains("grammar", " Japan's dispute with China has spread. ") {
		t.Fatal("Contains did not match trimmed grammar source")
	}
	if s.Contains("writing", "Japan's dispute with China has spread.") {
		t.Fatal("Contains leaked grammar entry across kinds")
	}

	s.Add(NotebookEntry{
		Kind:        "writing",
		Source:      "pls check asap",
		Translation: "Could you please check?",
		Writing: &ImproveWritingResult{
			Setting:  "正式邮件",
			Improved: "Could you please check?",
			Summary:  "避免聊天缩写。",
		},
	})

	all, gr, wr := s.KindCounts()
	if all != 2 || gr != 1 || wr != 1 {
		t.Fatalf("KindCounts = %d/%d/%d, want 2/1/1", all, gr, wr)
	}

	filtered := s.Filter("grammar", "dispute")
	if len(filtered) != 1 || filtered[0].Grammar == nil || len(filtered[0].Grammar.Phrases) != 1 {
		t.Fatalf("Filter grammar mismatch: %+v", filtered)
	}

	reloaded := &NotebookStore{path: path}
	reloaded.load()
	if reloaded.Count() != 2 {
		t.Fatalf("reloaded count = %d, want 2", reloaded.Count())
	}

	id := reloaded.Filter("writing", "")[0].ID
	if !reloaded.Delete(id) || reloaded.Count() != 1 {
		t.Fatalf("Delete failed, count = %d", reloaded.Count())
	}
	if err := reloaded.Clear(); err != nil || reloaded.Count() != 0 {
		t.Fatalf("Clear failed: err=%v count=%d", err, reloaded.Count())
	}
}

func TestMigrateNotebookFromVocabMovesGrammarAndWritingOnly(t *testing.T) {
	dir := t.TempDir()
	vocab := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json")}
	vocab.Add(VocabularyEntry{ID: "w1", Term: "dispute", Definition: "争端", Source: "dict"})
	vocab.Add(VocabularyEntry{ID: "n1", Term: "mineral", Definition: "矿物", Source: "notes"})
	vocab.Add(VocabularyEntry{ID: "t1", Term: "Hello world", Context: "Hello world", ContextTranslation: "你好世界", Source: "translate"})
	vocab.Add(VocabularyEntry{ID: "g1", Term: "Only then...", Context: "Only then did she realize.", ContextTranslation: "直到那时她才意识到。", Source: "grammar"})
	vocab.Add(VocabularyEntry{ID: "wr1", Term: "pls check", Definition: "Could you please check?", Context: "pls check", ContextTranslation: "注意正式语气。", Source: "writing"})

	nb := &NotebookStore{path: filepath.Join(dir, "notebook.json"), entries: []NotebookEntry{}}
	moved := migrateNotebookFromVocab(nb, vocab)
	if moved != 2 {
		t.Fatalf("moved = %d, want 2", moved)
	}
	if vocab.Count() != 3 {
		t.Fatalf("vocab remaining = %d, want 3 (dict/notes/translate)", vocab.Count())
	}
	if !nb.Contains("grammar", "Only then did she realize.") || !nb.Contains("writing", "pls check") {
		t.Fatalf("notebook missing migrated entries: %+v", nb.entries)
	}

	if again := migrateNotebookFromVocab(nb, vocab); again != 0 {
		t.Fatalf("second migration moved %d entries, want 0", again)
	}
}
