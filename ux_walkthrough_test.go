package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestWritingCardSaveStateFeedback(t *testing.T) {
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

	// Before save: should show "保存到笔记本"
	ttBefore := ui.NewTester(a.popupView, 480, 500)
	if !ttBefore.HasText("保存到笔记本") || ttBefore.HasText("✓ 已存入笔记本") {
		t.Errorf("expected unsaved state to show '保存到笔记本', got: %v", ttBefore.Texts())
	}

	// Trigger save
	a.saveWritingToVocab(a.writingResult)

	if !a.writingSaved {
		t.Fatal("saveWritingToVocab failed to set writingSaved")
	}

	// After save: should show "✓ 已存入笔记本"
	ttAfter := ui.NewTester(a.popupView, 480, 500)
	if !ttAfter.HasText("✓ 已存入笔记本") {
		t.Errorf("expected saved state to show '✓ 已存入笔记本', got: %v", ttAfter.Texts())
	}

	// Tab switch persistence: should restore writingSaved
	a.switchMode(modeTranslate)
	if a.mode != modeTranslate {
		t.Fatalf("expected modeTranslate, got %v", a.mode)
	}
	a.switchMode(modeWriting)
	if !a.writingSaved {
		t.Fatal("writingSaved state was lost after tab switch")
	}
}

func TestGrammarObstaclesDarkThemeAdaptive(t *testing.T) {
	a := newTestApp()
	a.mode = modeGrammar
	res := &SentenceAnalysisResult{
		Annotations: []SyntaxAnnotation{
			{Text: "Only then", Type: "adverbial", Obstacle: "fronting"},
			{Text: "did she realize", Type: "predicate", Obstacle: "inversion", Restore: "she realized"},
		},
		Translation: "直到那时她才意识到。",
	}

	// Test Dark Mode rendering
	ttDark := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, "dark", true)
		a.renderGrammarVisualCard(c, c.Theme(), "Only then did she realize", res, false, nil)
	}, 500, 400)
	ttDark.Frame()

	texts := ttDark.Texts()
	joined := strings.Join(texts, " ")
	for _, expected := range []string{"句式难点", "语序", "还原语序：", "she realized"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("grammar card in dark mode missing text %q: %v", expected, texts)
		}
	}
}

func TestGrammarCardSaveStateFeedback(t *testing.T) {
	a := newTestApp()
	a.source = "Only then did she realize what happened"
	a.mode = modeGrammar
	a.grammarResult = &SentenceAnalysisResult{
		Annotations: []SyntaxAnnotation{
			{Text: "Only then", Type: "adverbial"},
			{Text: "did she realize", Type: "predicate"},
		},
		Translation: "直到那时她才明白发生了什么。",
	}

	// Before save: should show "保存到长难句本"
	ttBefore := ui.NewTester(a.popupView, 480, 500)
	if !ttBefore.HasText("保存到长难句本") || ttBefore.HasText("✓ 已存入长难句本") {
		t.Errorf("expected unsaved grammar state to show '保存到长难句本', got: %v", ttBefore.Texts())
	}

	// Save
	a.saveGrammarToVocab(a.grammarResult)
	if !a.grammarSaved {
		t.Fatal("saveGrammarToVocab failed to set grammarSaved")
	}

	// After save: should show static badge "✓ 已存入长难句本"
	ttAfter := ui.NewTester(a.popupView, 480, 500)
	if !ttAfter.HasText("✓ 已存入长难句本") {
		t.Errorf("expected saved grammar state to show '✓ 已存入长难句本', got: %v", ttAfter.Texts())
	}

	// Tab switch persistence
	a.switchMode(modeTranslate)
	a.switchMode(modeGrammar)
	if !a.grammarSaved {
		t.Fatal("grammarSaved was lost after tab switch")
	}
}

func TestDictionaryTabPersistenceSavedTerm(t *testing.T) {
	a := newTestApp()
	a.mode = modeDictionary
	a.source = "serendipity"
	a.savedDictTerm = "serendipity"
	a.dictResult = &DictionaryResult{
		Term:       "serendipity",
		Definition: "意外发现珍奇事物的本领",
	}

	// Tab switch to translate and back
	a.switchMode(modeTranslate)
	if a.savedDictTerm != "" && a.mode == modeTranslate {
		// in translate mode
	}
	a.switchMode(modeDictionary)
	if a.savedDictTerm != "serendipity" {
		t.Fatalf("expected savedDictTerm to be restored as 'serendipity', got %q", a.savedDictTerm)
	}
}

func TestWordSaveButtonFeedback(t *testing.T) {
	a := newTestApp()
	a.mode = modeTranslate
	a.source = "apple"
	a.target = "苹果"
	a.dict = &YoudaoDict{
		UKPhone:  "ˈæpl",
		USPhone:  "ˈæpl",
		Explains: []string{"n. 苹果"},
	}

	ttBefore := ui.NewTester(a.popupView, 480, 500)
	if !ttBefore.HasText("保存生词") || ttBefore.HasText("已保存 ✓") {
		t.Errorf("expected unsaved word button to say '保存生词', got: %v", ttBefore.Texts())
	}

	// Mark as saved
	a.savedDictTerm = "apple"
	ttAfter := ui.NewTester(a.popupView, 480, 500)
	if !ttAfter.HasText("已保存 ✓") {
		t.Errorf("expected saved word to show '已保存 ✓', got: %v", ttAfter.Texts())
	}
}

func TestNonWordVocabCardDoesNotShowExpandWordCard(t *testing.T) {
	a := newTestApp()
	store := GetVocabularyStore()
	store.Clear()
	store.Add(VocabularyEntry{
		Term:       "Only then did she realize",
		Definition: "",
		Context:    "Only then did she realize her true potential.",
		Source:     "grammar",
	})

	tt := ui.NewTester(func(c *ui.Context) {
		a.pageVocabulary(c, c.Theme(), a.store.Get())
	}, 600, 500)
	tt.Frame()

	// Should not have "词卡" button for grammar sentence
	texts := tt.Texts()
	for _, txt := range texts {
		if txt == "词卡" {
			t.Errorf("expected grammar sentence in vocabulary page NOT to have '词卡' button: %v", texts)
		}
	}
}

func TestOcrParagraphRefineErrorHasRetryButtons(t *testing.T) {
	a := newTestApp()
	a.mode = modeTranslate
	a.isScreenshotMode = true
	a.ocrCompare = true
	a.source = "First line\n\nSecond line"
	a.target = "第一行\n\n第二行"
	a.aiRefineErr = "连接网关失败"

	tt := ui.NewTester(a.popupView, 480, 500)
	if !tt.HasText("重试") || !tt.HasText("打开 AI 设置") {
		t.Errorf("paragraph view missing retry or settings button on refine failure: %v", tt.Texts())
	}
}

func TestTrayQuitButtonDarkTheme(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, "dark", true)
		a.trayFlyoutView(c)
	}, 320, 240)
	tt.Frame()
	if !tt.HasText("退出应用") {
		t.Errorf("tray flyout missing 退出应用 button: %v", tt.Texts())
	}
}

func TestAnkiExportCardFormatAndStyling(t *testing.T) {
	dir := t.TempDir()
	store := &VocabularyStore{
		path: filepath.Join(dir, "test_vocab.json"),
		entries: []VocabularyEntry{
			{
				ID:                 "w1",
				Term:               "serendipity",
				Phonetic:           "/ˌser.ənˈdɪp.ə.ti/",
				PartOfSpeech:       "noun",
				Definition:         "意外发现珍奇事物的本领",
				Difficulty:         "C1",
				Context:            "Finding this book was sheer serendipity.",
				ContextTerm:        "serendipity",
				ContextTranslation: "找到这本书纯属机缘巧合。",
				MemoryTips:         "源自波斯童话《斯里兰卡三王子》",
				Source:             "dict",
			},
			{
				ID:                 "g1",
				Term:               "Only then did she realize the truth.",
				Definition:         "倒装句分析",
				Difficulty:         "B2",
				ContextTranslation: "直到那时她才明白真相。",
				Source:             "grammar",
			},
			{
				ID:         "wrt1",
				Term:       "I very like it",
				Definition: "very 不能直接修饰动词",
				Context:    "I really like it.",
				Source:     "writing",
			},
		},
	}

	ankiPath := filepath.Join(dir, "export_anki.txt")
	if err := store.ExportAnki(ankiPath); err != nil {
		t.Fatalf("ExportAnki failed: %v", err)
	}

	content, err := os.ReadFile(ankiPath)
	if err != nil {
		t.Fatalf("read anki export: %v", err)
	}
	text := string(content)

	if !strings.HasPrefix(text, "#separator:tab\n#html:true\n#tags column:4\n") {
		t.Fatalf("Anki export missing standard Anki headers, got: %s", text[:min(len(text), 100)])
	}

	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 total lines in TSV, got %d", len(lines))
	}

	// Row 1: word entry
	rowWord := strings.Split(lines[3], "\t")
	if len(rowWord) != 4 {
		t.Fatalf("expected 4 columns in word card row, got %d", len(rowWord))
	}
	if !strings.Contains(rowWord[0], "serendipity") || !strings.Contains(rowWord[0], "/ˌser.ənˈdɪp.ə.ti/") {
		t.Errorf("word card front missing term or phonetic: %s", rowWord[0])
	}
	if !strings.Contains(rowWord[1], "fd-badge-c1") || !strings.Contains(rowWord[1], "fd-hl") {
		t.Errorf("word card back missing C1 badge or keyword highlight: %s", rowWord[1])
	}
	if rowWord[2] != "serendipity" {
		t.Errorf("expected term column 'serendipity', got %q", rowWord[2])
	}
	if !strings.Contains(rowWord[3], "fastDictionary") || !strings.Contains(rowWord[3], "CEFR-C1") {
		t.Errorf("expected tags to include fastDictionary and CEFR-C1, got: %q", rowWord[3])
	}

	// Row 2: grammar entry
	rowGrammar := strings.Split(lines[4], "\t")
	if !strings.Contains(rowGrammar[0], "长难句拆解") {
		t.Errorf("grammar card front missing '长难句拆解': %s", rowGrammar[0])
	}
	if !strings.Contains(rowGrammar[3], "长难句") {
		t.Errorf("grammar card tags missing '长难句': %s", rowGrammar[3])
	}

	// Row 3: writing entry
	rowWriting := strings.Split(lines[5], "\t")
	if !strings.Contains(rowWriting[0], "写作批改") {
		t.Errorf("writing card front missing '写作批改': %s", rowWriting[0])
	}
	if !strings.Contains(rowWriting[1], "I really like it.") {
		t.Errorf("writing card back missing improved text: %s", rowWriting[1])
	}
}

func TestExportHTMLAnkiAndDarkMode(t *testing.T) {
	dir := t.TempDir()
	store := &VocabularyStore{
		path: filepath.Join(dir, "vocab.json"),
		entries: []VocabularyEntry{
			{
				ID:         "1",
				Term:       "lucid",
				Phonetic:   "/ˈluː.sɪd/",
				Definition: "清晰的",
				Difficulty: "B2",
				Context:    "His explanation was perfectly lucid.",
				Source:     "dict",
			},
		},
	}
	path, err := store.ExportHTML()
	if err != nil {
		t.Fatalf("ExportHTML failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read html: %v", err)
	}
	page := string(data)

	if !strings.Contains(page, "📇 导出 Anki") || !strings.Contains(page, "exportAnki()") {
		t.Errorf("ExportHTML missing Anki export button or script")
	}

	if !strings.Contains(page, "@media (prefers-color-scheme:dark)") {
		t.Errorf("ExportHTML missing dark mode media query")
	}

	if !strings.Contains(page, "card-diff b2") {
		t.Errorf("ExportHTML missing card-diff b2 class: %s", page)
	}

	if strings.Count(page, "</script>") != 1 {
		t.Fatalf("expected exactly 1 </script> tag, got %d", strings.Count(page, "</script>"))
	}
}

func TestVocabSettingsEmptyStateSearchAndCategories(t *testing.T) {
	a := newTestApp()
	store := GetVocabularyStore()
	store.Clear()

	ttEmpty := ui.NewTester(func(c *ui.Context) {
		a.pageVocabulary(c, c.Theme(), a.store.Get())
	}, 600, 500)
	ttEmpty.Frame()
	if !ttEmpty.HasText("笔记本还是空的") {
		t.Errorf("expected initial empty state to say '笔记本还是空的': %v", ttEmpty.Texts())
	}

	store.Add(VocabularyEntry{
		Term:       "resilience",
		Definition: "韧性",
		Source:     "dict",
	})

	a.vocabSearchQuery = "nonexistentkeyword"
	ttSearch := ui.NewTester(func(c *ui.Context) {
		a.pageVocabulary(c, c.Theme(), a.store.Get())
	}, 600, 500)
	ttSearch.Frame()
	if !ttSearch.HasText("未找到与「nonexistentkeyword」相关的笔记") || !ttSearch.HasText("清除搜索关键词") {
		t.Errorf("expected search empty state with clear button: %v", ttSearch.Texts())
	}

	ttSearch.Click("清除搜索关键词")
	ttSearch.Frame()
	if a.vocabSearchQuery != "" {
		t.Errorf("expected vocabSearchQuery to be cleared, got %q", a.vocabSearchQuery)
	}
}

func TestPopupEmptyStatesGuidance(t *testing.T) {
	a := newTestApp()
	a.source = ""
	a.target = ""

	a.mode = modeTranslate
	ttTrans := ui.NewTester(a.popupView, 480, 500)
	if !ttTrans.HasText("划词翻译就绪") || !ttTrans.HasText("键盘输入翻译") {
		t.Errorf("expected translate mode empty state to guide input: %v", ttTrans.Texts())
	}

	a.mode = modeGrammar
	a.grammarResult = nil
	ttGrammar := ui.NewTester(a.popupView, 480, 500)
	if !ttGrammar.HasText("长难句拆解") || !ttGrammar.HasText("键盘输入长句") {
		t.Errorf("expected grammar mode empty state to guide input: %v", ttGrammar.Texts())
	}

	a.mode = modeWriting
	a.writingResult = nil
	ttWriting := ui.NewTester(a.popupView, 480, 500)
	if !ttWriting.HasText("AI 写作批改与润色") || !ttWriting.HasText("键盘输入句子") {
		t.Errorf("expected writing mode empty state to guide input: %v", ttWriting.Texts())
	}
}

func TestMultiParagraphNormalTranslationHasCompareButton(t *testing.T) {
	a := newTestApp()
	a.mode = modeTranslate
	a.isScreenshotMode = false
	a.source = "First paragraph here.\n\nSecond paragraph here."
	a.target = "第一段在这里。\n\n第二段在这里。"

	tt := ui.NewTester(a.popupView, 480, 500)
	if !tt.HasText("段落对照") {
		t.Errorf("expected normal multi-paragraph translation to offer '段落对照' button: %v", tt.Texts())
	}

	tt.Click("段落对照")
	tt.Frame()
	if !a.ocrCompare {
		t.Errorf("expected clicking '段落对照' to enable ocrCompare")
	}

	ttCompare := ui.NewTester(a.popupView, 480, 500)
	if !ttCompare.HasText("§ 1") || !ttCompare.HasText("§ 2") {
		t.Errorf("expected paragraph compare view to have '§ 1' and '§ 2': %v", ttCompare.Texts())
	}
}

func TestPopupKeyboardShortcuts(t *testing.T) {
	a := newTestApp()
	a.source = "apple"
	a.target = "苹果"
	a.mode = modeTranslate

	tt := ui.NewTester(a.popupView, 480, 500)
	tt.Frame()

	// 1. Switch to Grammar mode via Ctrl+2
	tt.Key(ui.Ctrl, ui.Key2)
	tt.Frame()
	if a.mode != modeGrammar {
		t.Errorf("expected Ctrl+2 to switch to modeGrammar, got %v", a.mode)
	}

	// 2. Switch to Writing mode via Ctrl+3
	tt.Key(ui.Ctrl, ui.Key3)
	tt.Frame()
	if a.mode != modeWriting {
		t.Errorf("expected Ctrl+3 to switch to modeWriting, got %v", a.mode)
	}

	// 3. Switch back to Translate mode via Ctrl+1
	tt.Key(ui.Ctrl, ui.Key1)
	tt.Frame()
	if a.mode != modeTranslate {
		t.Errorf("expected Ctrl+1 to switch to modeTranslate, got %v", a.mode)
	}

	// 4. Test Ctrl+S quick save for word
	if a.savedDictTerm != "" {
		t.Fatal("savedDictTerm should be empty initially")
	}
	tt.Key(ui.Ctrl, ui.KeyS)
	tt.Frame()
	if a.savedDictTerm != "apple" {
		t.Errorf("expected Ctrl+S to save term 'apple', got %q", a.savedDictTerm)
	}
}

func TestHighlightTermInContextEntitySafety(t *testing.T) {
	// 1. amp should not match inside &amp;
	res := highlightTermInContext("Tom & Jerry", "amp")
	if strings.Contains(res, "<b class=\"fd-hl\">amp</b>") {
		t.Errorf("corrupted entity &amp; found in: %s", res)
	}
	if res != "Tom &amp; Jerry" {
		t.Errorf("expected 'Tom &amp; Jerry', got %q", res)
	}

	// 2. & should be highlighted as <b class="fd-hl">&amp;</b>
	resAmp := highlightTermInContext("Tom & Jerry", "&")
	if !strings.Contains(resAmp, "<b class=\"fd-hl\">&amp;</b>") {
		t.Errorf("expected '&amp;' to be highlighted: %s", resAmp)
	}

	// 3. lt should not match inside &lt;
	resLt := highlightTermInContext("a < b", "lt")
	if strings.Contains(resLt, "<b class=\"fd-hl\">lt</b>") {
		t.Errorf("corrupted entity &lt; found in: %s", resLt)
	}

	// 4. Quotation marks should remain valid HTML entities
	resQuote := highlightTermInContext("He said \"hello\"", "hello")
	if !strings.Contains(resQuote, "<b class=\"fd-hl\">hello</b>") || !strings.Contains(resQuote, "&#34;") {
		t.Errorf("quotes or term improperly escaped: %s", resQuote)
	}

	// 5. Script tag escaping
	resScript := highlightTermInContext("<script>alert(1)</script>", "<script>")
	if strings.Contains(resScript, "<script>") {
		t.Errorf("unescaped script tag found: %s", resScript)
	}
	if !strings.Contains(resScript, "<b class=\"fd-hl\">&lt;script&gt;</b>") {
		t.Errorf("expected escaped script tag highlighted: %s", resScript)
	}
}

func TestAnkiClozeEntitySafety(t *testing.T) {
	e := VocabularyEntry{
		Term:    "amp",
		Context: "Fish & chips",
		Source:  "dict",
	}
	front, _, _, _ := formatAnkiCard(e)
	if strings.Contains(front, "&<span") {
		t.Errorf("Anki cloze corrupted HTML entity in front card: %s", front)
	}

	e2 := VocabularyEntry{
		Term:        "Jerry",
		ContextTerm: "Jerry",
		Context:     "Tom & Jerry were best friends.",
		Source:      "dict",
	}
	front2, _, _, _ := formatAnkiCard(e2)
	if !strings.Contains(front2, "Tom &amp; <span") {
		t.Errorf("expected Tom &amp; before cloze, got: %s", front2)
	}
}

func TestExportHTMLFullSearchAndStyling(t *testing.T) {
	dir := t.TempDir()
	store := &VocabularyStore{
		path: filepath.Join(dir, "vocab.json"),
		entries: []VocabularyEntry{
			{
				ID:                 "1",
				Term:               "serendipity",
				Definition:         "意外发现珍奇事物的本领",
				Context:            "Finding this book was pure serendipity.",
				ContextTranslation: "发现这本书纯属偶然的惊喜。",
				MemoryTips:         "源自波斯童话《塞伦迪普三王子》",
				Source:             "dict",
			},
		},
	}
	path, err := store.ExportHTML()
	if err != nil {
		t.Fatalf("ExportHTML failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read html: %v", err)
	}
	page := string(data)

	// .fd-hl CSS class must be defined
	if !strings.Contains(page, ".fd-hl, .hl") {
		t.Errorf("ExportHTML missing .fd-hl class definition: %s", page)
	}

	// #noResult empty state must be present
	if !strings.Contains(page, "id=\"noResult\"") {
		t.Errorf("ExportHTML missing #noResult element")
	}

	// data-text must include example sentence and memory tips for search
	if !strings.Contains(page, "pure serendipity") || !strings.Contains(page, "塞伦迪普三王子") {
		t.Errorf("ExportHTML card data-text missing context or tips: %s", page)
	}
}

func TestInputTargetModeFlow(t *testing.T) {
	a := newTestApp()
	a.source = ""
	a.target = ""
	a.mode = modeGrammar

	tt := ui.NewTester(a.popupView, 480, 500)
	tt.Frame()

	// 1. Click "键盘输入长句" in Grammar mode
	if !tt.HasText("键盘输入长句") {
		t.Fatalf("missing '键盘输入长句' button")
	}
	tt.Click("键盘输入长句")
	tt.Frame()

	if a.mode != modeInput {
		t.Errorf("expected modeInput, got %v", a.mode)
	}
	if a.inputTargetMode != modeGrammar {
		t.Errorf("expected inputTargetMode modeGrammar, got %v", a.inputTargetMode)
	}

	// Check active tab indicator stays on Grammar (index 1)
	if idx := popupModeIndex(a.mode, a.inputTargetMode); idx != 1 {
		t.Errorf("expected tab index 1 (Grammar), got %d", idx)
	}

	// 2. Submit text in TextArea
	a.inputText = "Although it was raining, they went out for a walk."
	// Trigger Enter on TextArea
	ttSubmit := ui.NewTester(a.popupView, 480, 500)
	ttSubmit.Frame()
	ttSubmit.Key(0, ui.KeyEnter)
	ttSubmit.Frame()

	if a.mode != modeGrammar {
		t.Errorf("expected Enter to switch to modeGrammar, got %v", a.mode)
	}
	if a.source != "Although it was raining, they went out for a walk." {
		t.Errorf("expected source to be updated with entered text, got %q", a.source)
	}
}

func TestQuickSaveDictionaryAndSentence(t *testing.T) {
	store := GetVocabularyStore()
	store.Clear()

	a := newTestApp()

	// 1. Test quick save in dictionary mode
	a.mode = modeDictionary
	a.dictResult = &DictionaryResult{
		Term:       "ephemeral",
		Phonetic:   "/ɪˈfem.ər.əl/",
		Definition: "短暂的",
	}
	a.handleQuickSave(a.snap())
	if a.savedDictTerm != "ephemeral" {
		t.Errorf("expected savedDictTerm to be 'ephemeral', got %q", a.savedDictTerm)
	}
	if store.Count() == 0 {
		t.Errorf("expected dictionary entry to be saved to store")
	}

	// 2. Test quick save in translation mode for a full sentence
	a.mode = modeTranslate
	a.source = "Knowledge is power."
	a.target = "知识就是力量。"
	a.handleQuickSave(a.snap())
	// Let background goroutine execute
	time.Sleep(50 * time.Millisecond)

	foundSentence := false
	for _, e := range store.List() {
		if strings.Contains(e.Term, "Knowledge is power") {
			foundSentence = true
			break
		}
	}
	if !foundSentence {
		t.Errorf("expected sentence translation to be saved to vocab store")
	}
}

func TestHotkeyConflictDetection(t *testing.T) {
	// 1. Conflict between Selection and Input
	msg1 := checkHotkeyConflict("Alt+D", "alt+d", "Alt+C")
	if !strings.Contains(msg1, "划词快捷键与输入快捷键相同") {
		t.Errorf("expected conflict between sel and inp, got: %q", msg1)
	}

	// 2. Conflict between Selection and Screenshot
	msg2 := checkHotkeyConflict("Ctrl+Alt+S", "Ctrl+Alt+I", "ctrl+alt+s")
	if !strings.Contains(msg2, "划词快捷键与截图快捷键相同") {
		t.Errorf("expected conflict between sel and ocr, got: %q", msg2)
	}

	// 3. Conflict between Input and Screenshot
	msg3 := checkHotkeyConflict("Alt+D", "Alt+Q", "alt + q")
	if !strings.Contains(msg3, "输入快捷键与截图快捷键相同") {
		t.Errorf("expected conflict between inp and ocr, got: %q", msg3)
	}

	// 3b. Modifier permutation conflict (Ctrl+Alt+S vs Alt+Ctrl+S)
	msgPerm := checkHotkeyConflict("Ctrl+Alt+S", "Alt+Ctrl+S", "Alt+C")
	if !strings.Contains(msgPerm, "划词快捷键与输入快捷键相同") {
		t.Errorf("expected permutation conflict between Ctrl+Alt+S and Alt+Ctrl+S, got: %q", msgPerm)
	}

	// 4. Distinct keys have no conflict
	msg4 := checkHotkeyConflict("Alt+D", "Alt+Q", "Alt+C")
	if msg4 != "" {
		t.Errorf("expected no conflict, got: %q", msg4)
	}

	// 5. Test applyHotkeys blocks conflict and preserves previous configuration
	a := newTestApp()
	a.initHotkeyDrafts()
	a.pendingHotkeySelection = "Alt+D"
	a.pendingHotkeyInput = "Alt+D"
	a.pendingHotkeyScreenshot = "Alt+C"
	a.applyHotkeys()

	if !strings.Contains(a.hotkeyMsg, "快捷键冲突") {
		t.Errorf("expected a.hotkeyMsg to contain conflict error, got: %q", a.hotkeyMsg)
	}
	// Config should not have applied the duplicate hotkey
	cfg := a.store.Get()
	if cfg.HotkeyInput == "Alt+D" {
		t.Errorf("conflicting hotkey should not be persisted to config")
	}

	// 6. Test UI banner rendering in pageHotkey
	tt := ui.NewTester(func(c *ui.Context) {
		a.pageHotkey(c, c.Theme(), a.store.Get())
	}, 600, 600)
	tt.Frame()
	if !tt.HasText("冲突提示") || !tt.HasText("快捷键冲突：划词快捷键与输入快捷键相同 (Alt+D)，请分别指定不同按键") {
		t.Errorf("expected pageHotkey to render conflict warning banner: %v", tt.Texts())
	}
}

func TestHotkeyResetToDefault(t *testing.T) {
	orig := registerHotkeysFunc
	registerHotkeysFunc = func(sel, inp, ocr string, a *app) string { return "" }
	defer func() { registerHotkeysFunc = orig }()

	a := newTestApp()
	// Set custom hotkeys first
	a.pendingHotkeySelection = "Ctrl+Shift+1"
	a.pendingHotkeyScreenshot = "Ctrl+Shift+2"
	a.pendingHotkeyInput = "Ctrl+Shift+3"
	a.applyHotkeys()

	// Reset to default
	a.resetHotkeysToDefault()
	cfg := a.store.Get()
	def := DefaultConfig()

	if cfg.HotkeySelection != def.HotkeySelection ||
		cfg.HotkeyScreenshot != def.HotkeyScreenshot ||
		cfg.HotkeyInput != def.HotkeyInput {
		t.Errorf("expected hotkeys to reset to defaults (%+v), got (%+v)", def, cfg)
	}
	if a.pendingHotkeySelection != def.HotkeySelection {
		t.Errorf("expected pendingHotkeySelection to be %q, got %q", def.HotkeySelection, a.pendingHotkeySelection)
	}
}

func TestHotkeyDraftBackspaceFix(t *testing.T) {
	a := newTestApp()
	a.initHotkeyDrafts()

	// User selects all and presses backspace to clear the field
	a.pendingHotkeySelection = ""

	// Next frame renders pageHotkey
	tt := ui.NewTester(func(c *ui.Context) {
		a.pageHotkey(c, c.Theme(), a.store.Get())
	}, 600, 600)
	tt.Frame()

	// Crucial fix: empty pending string must NOT be overwritten by initHotkeyDrafts!
	if a.pendingHotkeySelection != "" {
		t.Errorf("pendingHotkeySelection was aggressively overwritten by initHotkeyDrafts, got %q", a.pendingHotkeySelection)
	}
}

func TestVocabSingleItemDeleteAccidentalPrevention(t *testing.T) {
	store := GetVocabularyStore()
	store.Clear()
	store.Add(VocabularyEntry{
		ID:         "test-entry-1",
		Term:       "quintessential",
		Definition: "典型的，典范的",
		Source:     "dict",
	})

	a := newTestApp()

	// 1. First render: should have "删除", but NOT "确认删除"
	tt := ui.NewTester(func(c *ui.Context) {
		a.pageVocabulary(c, c.Theme(), a.store.Get())
	}, 600, 500)
	tt.Frame()

	if !tt.HasText("删除") {
		t.Fatalf("expected pageVocabulary to display '删除' button")
	}
	if tt.HasText("确认删除") {
		t.Fatalf("initial state should NOT show '确认删除'")
	}

	// 2. Click "删除" -> transitions into confirmation mode, DOES NOT delete entry
	tt.Click("删除")
	tt.Frame()

	if store.Count() != 1 {
		t.Fatalf("first click on '删除' must NOT immediately delete entry from store")
	}
	if a.vocabConfirmDeleteID != "test-entry-1" {
		t.Errorf("expected vocabConfirmDeleteID to be 'test-entry-1', got %q", a.vocabConfirmDeleteID)
	}

	// Re-render tester with confirmation state
	ttConfirm := ui.NewTester(func(c *ui.Context) {
		a.pageVocabulary(c, c.Theme(), a.store.Get())
	}, 600, 500)
	ttConfirm.Frame()

	if !ttConfirm.HasText("确认删除") || !ttConfirm.HasText("取消") {
		t.Fatalf("expected confirmation state to show '确认删除' and '取消'")
	}

	// 3. Click "取消" -> dismisses confirmation, entry is preserved
	ttConfirm.Click("取消")
	ttConfirm.Frame()

	if a.vocabConfirmDeleteID != "" {
		t.Errorf("expected vocabConfirmDeleteID to be cleared after clicking '取消'")
	}
	if store.Count() != 1 {
		t.Fatalf("entry should remain in store after cancelling")
	}

	// 4. Click "删除" again -> click "确认删除" -> entry is removed!
	ttConfirm.Click("删除")
	ttConfirm.Frame()
	ttConfirm.Click("确认删除")
	ttConfirm.Frame()

	if store.Count() != 0 {
		t.Errorf("expected entry to be deleted after clicking '确认删除', store count = %d", store.Count())
	}
	if !strings.Contains(a.actionSavedMsg, "已删除「quintessential」") {
		t.Errorf("expected feedback toast '已删除「quintessential」', got: %q", a.actionSavedMsg)
	}
}

func TestSelectionChipScreenEdgeAvoidance(t *testing.T) {
	wa := mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}
	width := 200

	// 1. Extreme left edge: cursor at X=0
	xLeft, _ := clampChip(0, 500, wa, width)
	if xLeft < chipGap {
		t.Errorf("xLeft=%d, must be at least chipGap (%d) from left edge", xLeft, chipGap)
	}

	// 2. Extreme right edge: cursor at X=1919
	xRight, _ := clampChip(1919, 500, wa, width)
	if xRight+width+chipGap > wa.X+wa.Width {
		t.Errorf("xRight=%d with width %d escapes right boundary (wa.Width=%d)", xRight, width, wa.Width)
	}

	// 3. Extreme top edge: cursor at Y=0
	_, yTop := clampChip(500, 0, wa, width)
	if yTop < chipGap {
		t.Errorf("yTop=%d, must be at least chipGap (%d) from top edge", yTop, chipGap)
	}

	// 4. Extreme bottom edge (e.g. mouse up on taskbar at Y=1060)
	// Must flip above AND bottom edge must not exceed WorkArea.Height - chipGap
	_, yBottom := clampChip(500, 1060, wa, width)
	if yBottom+selIconHeight+chipGap > wa.Y+wa.Height {
		t.Errorf("yBottom=%d (+%d) escapes work area height %d with chipGap %d", yBottom, selIconHeight, wa.Height, chipGap)
	}
	if yBottom < wa.Y+chipGap {
		t.Errorf("yBottom=%d is above top margin", yBottom)
	}
}

func TestOcrParagraphMismatchAdaptivePairing(t *testing.T) {
	a := newTestApp()
	a.source = "First paragraph here.\n\nSecond paragraph here.\n\nThird paragraph here."
	a.target = "第一段在这里。\n\n第二段在这里。" // 3 source vs 2 target
	a.ocrCompare = true

	tt := ui.NewTester(func(c *ui.Context) {
		a.renderOcrParagraphPairs(c, c.Theme(), a.snap())
	}, 500, 600)
	tt.Frame()

	// 1. Must show adaptive pairing explanation
	if !tt.HasText("识别到 3 段原文、2 段译文（部分段落已自适应对齐对照）") {
		t.Errorf("missing adaptive pairing explanation: %v", tt.Texts())
	}

	// 2. Must show § 1, § 2, and § 3!
	for _, sec := range []string{"§ 1", "§ 2", "§ 3"} {
		if !tt.HasText(sec) {
			t.Errorf("expected paragraph card for %s, got texts: %v", sec, tt.Texts())
		}
	}

	// 3. Third source paragraph must NOT be discarded
	joined := strings.Join(tt.Texts(), " ")
	if !strings.Contains(joined, "Third paragraph here.") {
		t.Errorf("third source paragraph was lost: %s", joined)
	}

	// 4. Test empty boundary case
	a.source = ""
	a.target = ""
	ttEmpty := ui.NewTester(func(c *ui.Context) {
		a.renderOcrParagraphPairs(c, c.Theme(), a.snap())
	}, 500, 300)
	ttEmpty.Frame()
	if !ttEmpty.HasText("暂无段落内容") {
		t.Errorf("expected empty state '暂无段落内容', got: %v", ttEmpty.Texts())
	}
}

func TestBlockedAppInputSubmitOnEnter(t *testing.T) {
	a := newTestApp()
	a.newBlockedApp = "my_screen_tool.exe"

	tt := ui.NewTester(func(c *ui.Context) {
		a.pageSelection(c, c.Theme(), a.store.Get())
	}, 600, 600)
	tt.Frame()

	tt.Click("添加")
	tt.Frame()

	cfg := a.store.Get()
	found := false
	for _, app := range cfg.SelectionBlocklist {
		if app == "my_screen_tool.exe" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'my_screen_tool.exe' to be added to selection blocklist, got: %v", cfg.SelectionBlocklist)
	}
	if a.newBlockedApp != "" {
		t.Errorf("expected newBlockedApp to be cleared after adding, got %q", a.newBlockedApp)
	}
}

func TestBlockedAppNameSanitization(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{`"C:\Program Files\Snipaste\Snipaste.exe"`, "snipaste.exe"},
		{`'D:/Tools/ShareX/sharex.exe'`, "sharex.exe"},
		{"notepad", "notepad.exe"},
		{"NOTEPAD.EXE", "notepad.exe"},
		{"  greenshot  ", "greenshot.exe"},
		{"", ""},
	}
	for _, tc := range cases {
		got := cleanBlockedAppName(tc.input)
		if got != tc.expected {
			t.Errorf("cleanBlockedAppName(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}

	// Verify path-pasted blocked app successfully triggers appIsBlocked
	a := newTestApp()
	a.newBlockedApp = `"C:\Tools\SpecialSnip.EXE"`
	tt := ui.NewTester(func(c *ui.Context) {
		a.pageSelection(c, c.Theme(), a.store.Get())
	}, 600, 600)
	tt.Frame()
	tt.Click("添加")
	tt.Frame()

	cfg := a.store.Get()
	if !appIsBlocked("specialsnip.exe", cfg.SelectionBlocklist) {
		t.Fatalf("expected appIsBlocked to match specialsnip.exe after sanitization, got list: %v", cfg.SelectionBlocklist)
	}
}

func TestVocabSingleItemDeleteUndo(t *testing.T) {
	store := GetVocabularyStore()
	store.Clear()
	store.Add(VocabularyEntry{
		ID:         "entry-undo-test",
		Term:       "ephemeral",
		Definition: "短暂的，转瞬即逝的",
		Difficulty: "C1",
		Source:     "dict",
	})

	a := newTestApp()

	// 1. Render vocabulary page and click "删除"
	tt := ui.NewTester(func(c *ui.Context) {
		a.pageVocabulary(c, c.Theme(), a.store.Get())
	}, 600, 500)
	tt.Frame()

	tt.Click("删除")
	tt.Frame()
	tt.Click("确认删除")
	tt.Frame()

	if store.Count() != 0 {
		t.Fatalf("expected store to be empty after confirmation, count=%d", store.Count())
	}
	if a.lastDeletedVocab == nil || a.lastDeletedVocab.Term != "ephemeral" {
		t.Fatalf("expected lastDeletedVocab to hold 'ephemeral', got: %v", a.lastDeletedVocab)
	}

	// 2. The action bar must now display the Undo button
	ttUndo := ui.NewTester(func(c *ui.Context) {
		a.pageVocabulary(c, c.Theme(), a.store.Get())
	}, 600, 500)
	ttUndo.Frame()

	if !ttUndo.HasText("↩ 撤销删除「ephemeral」") {
		t.Fatalf("expected undo button '↩ 撤销删除「ephemeral」', got texts: %v", ttUndo.Texts())
	}

	// 3. Click Undo button -> restores entry to store!
	ttUndo.Click("↩ 撤销删除「ephemeral」")
	ttUndo.Frame()

	if store.Count() != 1 {
		t.Fatalf("expected store to restore entry after undo, count=%d", store.Count())
	}
	if a.lastDeletedVocab != nil {
		t.Fatalf("lastDeletedVocab should be cleared after undo")
	}
	if !strings.Contains(a.actionSavedMsg, "已恢复「ephemeral」") {
		t.Errorf("expected feedback toast '已恢复「ephemeral」', got: %q", a.actionSavedMsg)
	}
}

func TestMultiLineTextAreaShiftEnterDoesNotSubmit(t *testing.T) {
	a := newTestApp()
	a.source = ""
	a.target = ""
	a.mode = modeInput
	a.inputTargetMode = modeTranslate
	a.inputText = "Hello World"

	tt := ui.NewTester(a.popupView, 480, 500)
	tt.Frame()

	// 1. Shift+Enter should NOT submit translation
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Frame()

	if a.mode != modeInput {
		t.Fatalf("Shift+Enter must not submit translation in popup TextArea, mode changed to: %v", a.mode)
	}

	// 2. Plain Enter should submit translation
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	if a.mode != modeTranslate {
		t.Fatalf("Plain Enter must submit translation, got mode: %v", a.mode)
	}
	if a.source != "Hello World" {
		t.Fatalf("expected source to be 'Hello World', got: %q", a.source)
	}
}

func TestOcrParagraphCopyAllHeaderButton(t *testing.T) {
	a := newTestApp()
	a.source = "First line of text\n\nSecond line of text"
	a.target = "第一行文本\n\n第二行文本"
	a.ocrCompare = true

	tt := ui.NewTester(func(c *ui.Context) {
		a.renderOcrParagraphPairs(c, c.Theme(), a.snap())
	}, 500, 600)
	tt.Frame()

	if !tt.HasText("复制全文") {
		t.Fatalf("expected '复制全文' button in paragraph compare header: %v", tt.Texts())
	}
}
