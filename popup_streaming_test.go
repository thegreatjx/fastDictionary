package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestStreamingDoesNotDumpJson is the regression test for the user's "生成过程会
// 显示很多json" report. While the model streams, the three AI modes must show an
// indicator, never the raw, still-arriving payload.
func TestStreamingDoesNotDumpJson(t *testing.T) {
	a := newTestApp()
	for _, mode := range []popupMode{modeDictionary, modeGrammar, modeWriting} {
		a.mode = mode
		a.aiBusy = true
		a.aiText = `{"term":"seize","phonetic":"/siː` // the leak looked like this
		tt := ui.NewTester(a.popupView, 500, 620)
		for _, leaked := range []string{"term", "phonetic", "{", "seize"} {
			if tt.HasText(leaked) {
				t.Errorf("mode %v rendered the raw stream, found %q in %q", mode, leaked, tt.Texts())
			}
		}
	}
}

// TestFailedParseExplainsItself covers what happens after the stream ends with
// something unparseable: showing the braces is what the user complained about,
// so the body says what happened instead.
func TestFailedParseExplainsItself(t *testing.T) {
	a := newTestApp()
	a.mode = modeDictionary
	a.aiBusy = false
	a.aiText = `{"term": "seize", "phonetic": "/siː`
	tt := ui.NewTester(a.popupView, 500, 620)
	if tt.HasText("phonetic") {
		t.Fatalf("unparseable payload was printed: %q", tt.Texts())
	}
	if !tt.HasText("这次没能解析成结构化卡片") {
		t.Fatalf("no explanation for the failed parse: %q", tt.Texts())
	}
}

// TestProseAnswerIsStillShown keeps the other half of the rule. A model that
// answers in sentences has produced something readable, and withholding it
// would throw away the only result there is.
func TestProseAnswerIsStillShown(t *testing.T) {
	a := newTestApp()
	a.mode = modeDictionary
	a.aiBusy = false
	a.aiText = "seize 表示突然用力抓住，常指夺取权力或没收财产。"
	tt := ui.NewTester(a.popupView, 500, 620)
	if !tt.HasText("seize 表示突然用力抓住，常指夺取权力或没收财产。") {
		t.Fatalf("prose answer was withheld: %q", tt.Texts())
	}
	if tt.HasText("这次没能解析成结构化卡片") {
		t.Fatal("prose answer was reported as a failed parse")
	}
}

// TestStreamingShowsSingleIndicator guards against the busy state being drawn
// twice. It used to be: a banner at the top naming the pass plus a second
// spinner and an indeterminate progress bar in the body. One spinner in the
// body, next to one short message, is the whole indicator now.
func TestStreamingShowsSingleIndicator(t *testing.T) {
	a := newTestApp()
	a.mode = modeDictionary
	a.aiBusy = true
	tt := ui.NewTester(a.popupView, 500, 620)
	if !tt.HasText("正在生成词条与例句…") {
		t.Fatalf("the busy indicator lost its message: %q", tt.Texts())
	}
	seen := 0
	for _, s := range tt.Texts() {
		if strings.Contains(s, "正在生成词条与例句") {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("the busy message is drawn %d times: %q", seen, tt.Texts())
	}
}

// TestSavedDictionaryButtonStopsOfferingAgain is the second half of "保存到笔记本
// 根本点不了": once the entry is in the notebook the card must say so rather
// than inviting a click that would add it twice.
func TestSavedDictionaryButtonStopsOfferingAgain(t *testing.T) {
	a := newTestApp()
	a.mode = modeDictionary
	a.dictResult = &DictionaryResult{Term: "seize", Definition: "夺取"}
	a.savedDictTerm = "seize"
	tt := ui.NewTester(a.popupView, 500, 700)
	if !tt.HasText("已保存") {
		t.Fatalf("the button still offers to save an entry that is already there: %q", tt.Texts())
	}
}

// TestDictionarySaveBannerAppearsBelowTheCard is the last piece of the same
// report: the click worked but said nothing in this mode, so it read as dead.
func TestDictionarySaveBannerAppearsBelowTheCard(t *testing.T) {
	a := newTestApp()
	a.mode = modeDictionary
	a.dictResult = &DictionaryResult{Term: "seize", Definition: "夺取"}
	a.actionSavedMsg = "已保存「seize」到生词本 ✓"
	tt := ui.NewTester(a.popupView, 500, 700)
	if !tt.HasText("已保存「seize」到生词本 ✓") {
		t.Fatalf("no confirmation in the dictionary mode: %q", tt.Texts())
	}
}

// TestDictionarySaveButtonIsReachable is the geometry half. The button used to
// land past the bottom of a fixed 460px card, so the click never reached it.
// This test clicks it through the real layout instead of merely finding it.
func TestDictionarySaveButtonIsReachable(t *testing.T) {
	a := newTestApp()
	a.mode = modeDictionary
	a.source = "seize"
	a.activeActionID = "default-dictionary"
	a.dictResult = &DictionaryResult{
		Term:         "seize",
		Phonetic:     "/siːz/",
		PartOfSpeech: "verb",
		Definition:   "夺取；没收（财产、公司等）",
		Context:      "I'm so tired of people who don't build talking about seizing companies and wealth from those who do.",
		MemoryTips:   "搭配 seize power（夺权）",
		Difficulty:   "B2",
	}

	a.savedDictTerm = ""
	tt := ui.NewTester(a.popupView, 500, 900)
	tt.Click("保存到生词本")
	// Add() merges entries by term, so the notebook count cannot prove the
	// click landed; the recorded term can.
	if a.savedDictTerm != "seize" {
		t.Fatalf("the save button did not fire: savedDictTerm = %q, texts %q", a.savedDictTerm, tt.Texts())
	}
	if a.actionSavedMsg == "" {
		t.Fatal("the save produced no confirmation message")
	}
}
