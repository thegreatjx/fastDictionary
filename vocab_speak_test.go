package main

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func swapVocabStore(t *testing.T, entries ...VocabularyEntry) *VocabularyStore {
	t.Helper()
	prev := GetVocabularyStore()
	s := newMemoryVocabularyStore()
	s.entries = entries
	globalVocabStore = s
	t.Cleanup(func() { globalVocabStore = prev })
	return s
}

func TestVocabularyRowOffersSpeakButton(t *testing.T) {
	swapVocabStore(t, VocabularyEntry{
		ID: "1", Term: "seize", Phonetic: "/siːz/", PartOfSpeech: "verb",
		Definition: "夺取；没收", Source: "dict",
	})

	a := newTestApp()
	cfg := a.store.Get()
	tt := ui.NewTester(func(c *ui.Context) {
		a.pageVocabulary(c, grammarThemeForTest(t), cfg)
	}, 900, 800)

	if !tt.HasText("seize") {
		t.Fatalf("the entry did not render at all: %v", tt.Texts())
	}
	if !tt.HasText("朗读") {
		t.Fatalf("vocabulary row has no 朗读 button: %v", tt.Texts())
	}
}

func TestVocabularySpeakButtonSpeaksTheTerm(t *testing.T) {
	src, err := os.ReadFile("settings_vocab.go")
	if err != nil {
		t.Fatalf("read settings_vocab.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "PlaySpeechLang(entry.Term") {
		t.Fatal("the vocabulary speaker does not speak entry.Term")
	}
	for _, wrong := range []string{"PlaySpeechLang(entry.Definition", "PlaySpeechLang(entry.Context"} {
		if strings.Contains(body, wrong) {
			t.Fatalf("the vocabulary speaker speaks the wrong field: %s", wrong)
		}
	}
}
