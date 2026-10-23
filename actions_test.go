package main

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestParseDictionaryResult(t *testing.T) {
	rawJSON := `{"term":"blossom","phonetic":"/ˈblɒs.əm/","partOfSpeech":"noun","definition":"花；花朵","context":"Cherry blossoms remind us to cherish each moment.","contextTerm":[{"text":"blossoms"}],"contextTranslation":"樱花提醒我们珍惜每一刻。","memoryTips":"词源：和 bloom 同源\n搭配：cherry blossoms","difficulty":"B2"}`
	res := ParseDictionaryResult(rawJSON)
	if res == nil {
		t.Fatal("failed to parse valid dictionary json")
	}
	if res.Term != "blossom" {
		t.Errorf("expected term blossom, got %s", res.Term)
	}
	if res.Phonetic != "/ˈblɒs.əm/" {
		t.Errorf("expected phonetic, got %s", res.Phonetic)
	}
	if res.Difficulty != "B2" {
		t.Errorf("expected difficulty B2, got %s", res.Difficulty)
	}
	if res.ContextTerm != "blossoms" {
		t.Errorf("expected contextTerm blossoms, got %s", res.ContextTerm)
	}
	if !strings.Contains(res.MemoryTips, "bloom") {
		t.Errorf("expected memory tips to contain bloom, got %s", res.MemoryTips)
	}

	// Test Chinese keys fallback
	zhJSON := `{"词条":"感動","音标":"kandou","词性":"noun","释义":"感动","句子":"映画を見て感動した。","句中词形":"感動","句子翻译":"看了电影很感动。","记忆技巧":"汉字：感+动","难度":"B1"}`
	zhRes := ParseDictionaryResult(zhJSON)
	if zhRes == nil || zhRes.Term != "感動" || zhRes.Difficulty != "B1" {
		t.Fatalf("failed to parse Chinese keys fallback: %+v", zhRes)
	}
}

func TestParseSentenceAnalysisStructured(t *testing.T) {
	rawJSON := `{"annotations":[{"text":"The report","type":"subject"},{"text":"surprised","type":"predicate","form":"clause"},{"text":"many readers","type":"object"}],"translation":"这份报告让许多读者感到惊讶。"}`
	res := ParseSentenceAnalysis(rawJSON)
	if res == nil {
		t.Fatal("failed to parse sentence analysis")
	}
	if len(res.Annotations) != 3 {
		t.Errorf("expected 3 annotations, got %d", len(res.Annotations))
	}
	if res.Translation != "这份报告让许多读者感到惊讶。" {
		t.Errorf("unexpected translation: %s", res.Translation)
	}

	// Test string-serialized annotations format ($1 / $2)
	serialized := `{"$1":"[{\"text\":\"Only then\",\"type\":\"adverbial\",\"obstacle\":\"fronting\"},{\"text\":\"did\",\"type\":\"predicate\",\"obstacle\":\"inversion\"}]","$2":"直到那时她才拒绝了这个提议。"}`
	sRes := ParseSentenceAnalysis(serialized)
	if sRes == nil || len(sRes.Annotations) != 2 || sRes.Translation == "" {
		t.Fatalf("failed to parse serialized annotations: %+v", sRes)
	}
}

func TestParseImproveWritingResult(t *testing.T) {
	rawJSON := `{"setting":"正式 · 邮件","annotations":[{"text":"I very like","fix":"I really like","type":"grammar","tag":"副词位置","note":"very 不能直接修饰动词"},{"text":"ur","fix":"your","type":"register","tag":"缩写","note":"商务信函不要用聊天缩写"}],"improved":"I really like your proposal.","summary":"用词清晰，注意避免口语缩写与副词修饰位置错误。"}`
	res := ParseImproveWritingResult(rawJSON)
	if res == nil {
		t.Fatal("failed to parse improve writing result")
	}
	if res.Setting != "正式 · 邮件" {
		t.Errorf("expected setting 正式 · 邮件, got %s", res.Setting)
	}
	if len(res.Annotations) != 2 {
		t.Errorf("expected 2 annotations, got %d", len(res.Annotations))
	}
	if res.Annotations[0].Fix != "I really like" {
		t.Errorf("expected fix I really like, got %s", res.Annotations[0].Fix)
	}
	if res.Improved != "I really like your proposal." {
		t.Errorf("expected improved text, got %s", res.Improved)
	}
}

func TestVocabularyStoreMemory(t *testing.T) {
	store := newMemoryVocabularyStore()
	if store.Count() != 0 {
		t.Fatalf("expected empty store, got %d", store.Count())
	}

	store.Add(VocabularyEntry{
		Term:       "serendipity",
		Phonetic:   "/ˌserənˈdɪpəti/",
		Definition: "机缘巧合",
		Context:    "Finding it was pure serendipity.",
		MemoryTips: "词源：源自波斯童话三位锡兰王子",
		Source:     "dict",
	})

	if store.Count() != 1 {
		t.Fatalf("expected 1 entry, got %d", store.Count())
	}

	results := store.Search("serendipity")
	if len(results) != 1 || results[0].Term != "serendipity" {
		t.Fatalf("search serendipity failed: %+v", results)
	}

	// Duplicate update test (same term and context)
	store.Add(VocabularyEntry{
		Term:       "serendipity",
		Phonetic:   "/ˌserənˈdɪpəti/",
		Definition: "机缘巧合，意外发现的珍宝",
		Context:    "Finding it was pure serendipity.",
		Source:     "dict",
	})
	if store.Count() != 1 {
		t.Fatalf("expected duplicate to update rather than add, got %d", store.Count())
	}
	if store.List()[0].Definition != "机缘巧合，意外发现的珍宝" {
		t.Errorf("expected updated definition, got %s", store.List()[0].Definition)
	}

	// Delete test
	id := store.List()[0].ID
	if !store.Delete(id) || store.Count() != 0 {
		t.Fatalf("delete failed, count = %d", store.Count())
	}
}

func TestCustomActionConfigMethods(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.CustomActions) != 3 {
		t.Fatalf("expected 3 built-in actions, got %d", len(cfg.CustomActions))
	}

	dict := cfg.GetAction("default-dictionary")
	if dict == nil || dict.Name != "词典" {
		t.Fatalf("failed to get dictionary action: %+v", dict)
	}

	enabled := cfg.GetEnabledActions()
	if len(enabled) != 3 {
		t.Fatalf("expected 3 enabled actions, got %d", len(enabled))
	}

	// Disable dictionary
	dict.Enabled = false
	if len(cfg.GetEnabledActions()) != 2 {
		t.Fatalf("expected 2 enabled actions after disabling dict, got %d", len(cfg.GetEnabledActions()))
	}

	// Reset action
	cfg.ResetAction("default-dictionary")
	if !cfg.GetAction("default-dictionary").Enabled {
		t.Fatal("expected dictionary to be re-enabled after reset")
	}
}

func TestPopupRendersDictionaryCard(t *testing.T) {
	a := newTestApp()
	a.source = "blossom"
	a.mode = modeDictionary
	a.activeActionID = "default-dictionary"
	a.dictResult = &DictionaryResult{
		Term:               "blossom",
		Phonetic:           "/ˈblɒs.əm/",
		PartOfSpeech:       "noun",
		Definition:         "花朵（尤指果树的花）",
		Context:            "Cherry blossoms remind us to cherish each moment.",
		ContextTerm:        "blossoms",
		ContextTranslation: "樱花提醒我们珍惜每一刻。",
		MemoryTips:         "词源：和 bloom 同源",
		Difficulty:         "B2",
	}

	tt := ui.NewTester(a.popupView, 480, 500)
	if !tt.HasText("blossom") || !tt.HasText("花朵（尤指果树的花）") || !tt.HasText("B2") {
		t.Errorf("dictionary card missing expected texts: %v", tt.Texts())
	}
	if !tt.HasText("保存到笔记本") {
		t.Errorf("save to notebook button missing: %v", tt.Texts())
	}
}

func TestPopupRendersImproveWritingCard(t *testing.T) {
	a := newTestApp()
	a.source = "I very like this"
	a.mode = modeWriting
	a.activeActionID = "default-improve-writing"
	a.writingResult = &ImproveWritingResult{
		Setting: "正式 · 邮件",
		Annotations: []WritingAnnotation{
			{Text: "I very like", Fix: "I really like", Type: "grammar", Tag: "副词位置", Note: "very不能直接修饰动词"},
		},
		Improved: "I really like this.",
		Summary:  "注意副词修饰动词的位置。",
	}

	tt := ui.NewTester(a.popupView, 480, 500)
	if !tt.HasText("正式 · 邮件") || !tt.HasText("I really like this.") || !tt.HasText("导师总评") {
		t.Errorf("writing card missing expected texts: %v", tt.Texts())
	}
}

func TestSettingsCustomActionsAndVocabularyPages(t *testing.T) {
	a := newTestApp()

	// 1. Actions page list view
	a.page = "actions"
	tt := ui.NewTester(a.settingsView, 760, 560)
	if !tt.HasText("AI 指令") || !tt.HasText("词典") || !tt.HasText("长难句分析") || !tt.HasText("写作改进") {
		t.Errorf("actions page missing expected text: %v", tt.Texts())
	}

	// 2. Actions page customization view
	a.editingActionID = "default-dictionary"
	a.customizingAction = *a.store.Get().GetAction("default-dictionary")
	ttEdit := ui.NewTester(a.settingsView, 760, 560)
	if !ttEdit.HasText("定制化指令：词典") || !ttEdit.HasText("系统提示词") {
		t.Errorf("customization form missing: %v", ttEdit.Texts())
	}

	// 3. Vocabulary page empty view
	a.editingActionID = ""
	a.page = "vocabulary"
	ttVocab := ui.NewTester(a.settingsView, 760, 560)
	if !ttVocab.HasText("生词本") {
		t.Errorf("vocabulary page missing: %v", ttVocab.Texts())
	}
}
