package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStreamingDoesNotDumpJson(t *testing.T) {
	a := newTestApp()
	for _, mode := range []popupMode{modeDictionary, modeGrammar, modeWriting} {
		a.mode = mode
		a.aiBusy = true
		a.aiText = `{"term":"seize","phonetic":"/siː`
		tt := ui.NewTester(a.popupView, 500, 620)
		for _, leaked := range []string{"term", "phonetic", "{", "seize"} {
			if tt.HasText(leaked) {
				t.Errorf("mode %v rendered the raw stream, found %q in %q", mode, leaked, tt.Texts())
			}
		}
	}
}

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

func TestDictionarySaveSuccessNeedsNoBanner(t *testing.T) {
	a := newTestApp()
	a.mode = modeDictionary
	a.dictResult = &DictionaryResult{Term: "seize", Definition: "夺取"}
	a.actionSavedMsg = "已保存「seize」到生词本 ✓"
	tt := ui.NewTester(a.popupView, 500, 700)
	if tt.HasText("已保存「seize」到生词本 ✓") {
		t.Fatalf("the redundant save banner is still rendered: %q", tt.Texts())
	}
}

func TestDictionarySaveFailureAppearsBelowTheCard(t *testing.T) {
	a := newTestApp()
	a.mode = modeDictionary
	a.dictResult = &DictionaryResult{Term: "seize", Definition: "夺取"}
	a.saveErr = "保存失败: disk full"
	tt := ui.NewTester(a.popupView, 500, 700)
	if !tt.HasText("保存失败: disk full") {
		t.Fatalf("no failure reason in the dictionary mode: %q", tt.Texts())
	}
}

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
	if a.savedDictTerm != "seize" {
		t.Fatalf("the save button did not fire: savedDictTerm = %q, texts %q", a.savedDictTerm, tt.Texts())
	}
	if a.actionSavedMsg != "" || a.saveErr != "" {
		t.Fatalf("a successful save left a banner: msg=%q err=%q", a.actionSavedMsg, a.saveErr)
	}
}
