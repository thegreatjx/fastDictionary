package main

import "testing"

// cardGuardCases 固定「卡片就绪」的旧判定。渲染、窗口高度、快捷保存三处
// 原先各写一份 s.grammarResult / s.dictResult / s.writingResult != nil，
// 批次 2c 统一换成 cardReadyForMode(s)。这组表在切换前后必须逐字不变。
func cardGuardCases() []struct {
	name  string
	s     snapshot
	ready bool
} {
	grammar := &SentenceAnalysisResult{Translation: "译句"}
	dict := &DictionaryResult{Term: "seize", Definition: "抓住"}
	writing := &ImproveWritingResult{Improved: "改写句"}
	return []struct {
		name  string
		s     snapshot
		ready bool
	}{
		{"grammar / 无卡片", snapshot{mode: modeGrammar}, false},
		{"grammar / 自己的卡片", snapshot{mode: modeGrammar, grammarResult: grammar}, true},
		{"grammar / 只有别的 mode 的卡片", snapshot{mode: modeGrammar, dictResult: dict, writingResult: writing}, false},
		{"dictionary / 无卡片", snapshot{mode: modeDictionary}, false},
		{"dictionary / 自己的卡片", snapshot{mode: modeDictionary, dictResult: dict}, true},
		{"dictionary / 只有别的 mode 的卡片", snapshot{mode: modeDictionary, grammarResult: grammar, writingResult: writing}, false},
		{"writing / 无卡片", snapshot{mode: modeWriting}, false},
		{"writing / 自己的卡片", snapshot{mode: modeWriting, writingResult: writing}, true},
		{"writing / 只有别的 mode 的卡片", snapshot{mode: modeWriting, grammarResult: grammar, dictResult: dict}, false},
		{"translate / 三张卡片都在", snapshot{mode: modeTranslate, grammarResult: grammar, dictResult: dict, writingResult: writing}, false},
		{"input / 三张卡片都在", snapshot{mode: modeInput, grammarResult: grammar, dictResult: dict, writingResult: writing}, false},
	}
}

func TestCardReadyForModeMatchesLegacyChecks(t *testing.T) {
	for _, tc := range cardGuardCases() {
		t.Run(tc.name, func(t *testing.T) {
			if got := cardReadyForMode(tc.s); got != tc.ready {
				t.Fatalf("cardReadyForMode = %v, 旧判定 = %v", got, tc.ready)
			}
		})
	}
}

// TestCardReadyMatchesClassifyAIView 保证渲染与布局、快捷保存共用同一判定：
// 智能体以后只需改 cardReadyForMode 一处，不必追三个文件里的 != nil。
func TestCardReadyMatchesClassifyAIView(t *testing.T) {
	for _, tc := range cardGuardCases() {
		t.Run(tc.name, func(t *testing.T) {
			rendered := classifyAIView(tc.s) == aiPassStructured
			if rendered != cardReadyForMode(tc.s) {
				t.Fatalf("渲染判定 = %v, 卡片就绪判定 = %v，两者必须同源", rendered, cardReadyForMode(tc.s))
			}
		})
	}
}

// TestPopupContentHeightCardGuardBaseline 录下批次 2c 改前的窗口高度。
// 切换只改判断形式、不改判断结果；want 逐字不变才算零行为变化。
func TestPopupContentHeightCardGuardBaseline(t *testing.T) {
	cases := []struct {
		name string
		s    snapshot
		want float32
	}{
		{"dictionary/无卡片", snapshot{mode: modeDictionary}, 310},
		{"dictionary/短卡片", snapshot{mode: modeDictionary, dictResult: &DictionaryResult{Term: "seize", Definition: "抓住"}}, 262},
		{"dictionary/完整卡片", snapshot{mode: modeDictionary, dictResult: &DictionaryResult{Term: "seize", Definition: "抓住，攫取", Context: "seize the day", ContextTranslation: "把握当下", MemoryTips: "与 cease 区分"}}, 380},
		{"grammar/无卡片", snapshot{mode: modeGrammar}, 420},
		{"grammar/有译文卡片", snapshot{mode: modeGrammar, grammarResult: &SentenceAnalysisResult{Translation: "这是一个译句"}}, 436},
		{"writing/无卡片", snapshot{mode: modeWriting}, autofitMinHeight},
		{"writing/有批注卡片", snapshot{mode: modeWriting, writingResult: &ImproveWritingResult{Annotations: []WritingAnnotation{{Text: "alpha", Fix: "beta", Note: "说明"}}, Improved: "改写后的句子", Summary: "摘要"}}, 366},
		{"translate/三张外来卡片", snapshot{mode: modeTranslate, grammarResult: &SentenceAnalysisResult{}, dictResult: &DictionaryResult{}, writingResult: &ImproveWritingResult{}}, autofitMinHeight},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := popupContentHeight(tc.s, 440); got != tc.want {
				t.Fatalf("popupContentHeight = %v, 改前基线 = %v", got, tc.want)
			}
		})
	}
}
