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

	zhJSON := `{"词条":"感動","音标":"kandou","词性":"noun","释义":"感动","句子":"映画を見て感動した。","句中词形":"感動","句子翻译":"看了电影很感动。","记忆技巧":"汉字：感+动","难度":"B1"}`
	zhRes := ParseDictionaryResult(zhJSON)
	if zhRes == nil || zhRes.Term != "感動" || zhRes.Difficulty != "B1" {
		t.Fatalf("failed to parse Chinese keys fallback: %+v", zhRes)
	}
}

func TestParseSentenceAnalysisStructured(t *testing.T) {
	rawJSON := `{"segments":[{"text":"The report","role":"subject","is_backbone":true},{"text":" surprised","role":"predicate","is_backbone":true},{"text":" many readers","role":"object","is_backbone":true}],"translation":"这份报告让许多读者感到惊讶。"}`
	const sentence = "The report surprised many readers"
	res := ParseSentenceAnalysis(rawJSON, sentence)
	if res == nil {
		t.Fatal("failed to parse sentence analysis")
	}
	if len(res.Segments) != 3 {
		t.Errorf("expected 3 segments, got %d", len(res.Segments))
	}
	if concatSegments(res.Segments) != sentence {
		t.Errorf("segments do not restore the sentence: %q", concatSegments(res.Segments))
	}
	if res.Translation != "这份报告让许多读者感到惊讶。" {
		t.Errorf("unexpected translation: %s", res.Translation)
	}

	noSource := `{"segments":[{"text":"Only then","role":"modifier"},{"text":" did she realize","role":"predicate"}],"translation":"直到那时她才意识到。"}`
	sRes := ParseSentenceAnalysis(noSource)
	if sRes == nil || len(sRes.Segments) != 2 || sRes.Translation == "" {
		t.Fatalf("failed to parse segments without a source: %+v", sRes)
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

	id := store.List()[0].ID
	if !store.Delete(id) || store.Count() != 0 {
		t.Fatalf("delete failed, count = %d", store.Count())
	}
}

func TestCustomActionConfigMethods(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.CustomActions) != 2 {
		t.Fatalf("expected 2 built-in actions, got %d", len(cfg.CustomActions))
	}

	if retired := cfg.GetAction("default-dictionary"); retired != nil {
		t.Fatalf("default-dictionary should be gone, got %+v", retired)
	}

	grammar := cfg.GetAction("default-sentence-analysis")
	if grammar == nil || grammar.Name != "长难句分析" {
		t.Fatalf("failed to get sentence-analysis action: %+v", grammar)
	}

	enabled := cfg.GetEnabledActions()
	if len(enabled) != 2 {
		t.Fatalf("expected 2 enabled actions, got %d", len(enabled))
	}

	grammar.Enabled = false
	if len(cfg.GetEnabledActions()) != 1 {
		t.Fatalf("expected 1 enabled action after disabling grammar, got %d", len(cfg.GetEnabledActions()))
	}

	cfg.ResetAction("default-sentence-analysis")
	if !cfg.GetAction("default-sentence-analysis").Enabled {
		t.Fatal("expected sentence analysis to be re-enabled after reset")
	}
}

func TestLegacyFilesDropTheDictionaryAction(t *testing.T) {
	var cfg Config
	cfg.CustomActions = []CustomActionConfig{
		{ID: "default-dictionary", Name: "词典", Enabled: true},
		{ID: "default-sentence-analysis", Name: "长难句分析", Enabled: true},
		{ID: "my-own", Name: "我的指令", Enabled: true},
	}
	if !ensureDefaultActions(&cfg) {
		t.Fatal("cleanup did not report the dictionary action it removed")
	}
	if cfg.GetAction("default-dictionary") != nil {
		t.Fatal("legacy default-dictionary survived the migration")
	}
	if cfg.GetAction("default-sentence-analysis") == nil || cfg.GetAction("my-own") == nil {
		t.Fatalf("migration removed the wrong actions: %+v", cfg.CustomActions)
	}
	var clean Config
	clean.CustomActions = []CustomActionConfig{{ID: "default-sentence-analysis", Name: "长难句分析", Enabled: true}}
	if ensureDefaultActions(&clean) {
		t.Fatal("cleanup reported a change for a config that had none")
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

	a.page = "actions"
	tt := ui.NewTester(a.settingsView, 760, 560)
	if !tt.HasText("AI 指令") || !tt.HasText("长难句分析") || !tt.HasText("写作改进") {
		t.Errorf("actions page missing expected text: %v", tt.Texts())
	}
	if tt.HasText("面向语言学习者的精细词典") {
		t.Errorf("actions page still describes the retired 词典 action: %v", tt.Texts())
	}

	a.editingActionID = "default-sentence-analysis"
	a.customizingAction = *a.store.Get().GetAction("default-sentence-analysis")
	ttEdit := ui.NewTester(a.settingsView, 760, 560)
	if !ttEdit.HasText("定制化指令：长难句分析") || !ttEdit.HasText("系统提示词") {
		t.Errorf("customization form missing: %v", ttEdit.Texts())
	}

	a.editingActionID = ""
	a.page = "vocabulary"
	ttVocab := ui.NewTester(a.settingsView, 760, 560)
	if !ttVocab.HasText("生词本") {
		t.Errorf("vocabulary page missing: %v", ttVocab.Texts())
	}
}
