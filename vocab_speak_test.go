package main

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// swapVocabStore installs a scratch store for the duration of a test. The page
// reads the singleton, so a test that wants a known list has to replace it.
//
// The real store is built first, which spends the sync.Once. Assigning the
// singleton before that ever ran let the constructor overwrite the scratch
// store on first use — the test then rendered whatever was in the reader's own
// notebook instead of the entries it declared.
func swapVocabStore(t *testing.T, entries ...VocabularyEntry) *VocabularyStore {
	t.Helper()
	prev := GetVocabularyStore()
	s := newMemoryVocabularyStore()
	s.entries = entries
	globalVocabStore = s
	t.Cleanup(func() { globalVocabStore = prev })
	return s
}

// TestVocabularyRowOffersSpeakButton: the notebook stores what the reader
// looked up in order to review it, and reviewing a word starts with hearing it.
// The popup has always had a speaker; the notebook did not, so a saved word
// could only be listened to by selecting it again on the page it came from.
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
	// renderIconButton sets the tooltip as the label, which is what makes the
	// button findable and what a screen reader announces.
	if !tt.HasText("朗读") {
		t.Fatalf("vocabulary row has no 朗读 button: %v", tt.Texts())
	}
}

// TestVocabularySpeakButtonSpeaksTheTerm guards against wiring the speaker to
// the wrong field: the definition is Chinese for an English word, and reading
// it with the English voice is exactly the robotic sound that was reported.
// Checked at the source because the click itself starts a network request.
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
