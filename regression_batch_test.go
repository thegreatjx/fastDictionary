package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestUIACaptureHasTimeout guards the frozen-app case: the UIA round trip
// must be bounded, because a misbehaving provider used to block the whole
// capture path for seconds.
func TestUIACaptureHasTimeout(t *testing.T) {
	src, err := os.ReadFile("capture.go")
	if err != nil {
		t.Fatalf("read capture.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "uiaTimeout") {
		t.Fatal("capture.go has no uiaTimeout: a stuck UIA provider freezes the capture")
	}
	if !strings.Contains(body, "time.After(uiaTimeout)") {
		t.Fatal("captureUIA does not bound the COM call with time.After")
	}
}

// TestRenderViewsArePanicSafe: every window view must be wrapped in
// safeView, so one bad snapshot renders an error box instead of killing
// the tray-resident process.
func TestRenderViewsArePanicSafe(t *testing.T) {
	for name, path := range map[string]string{
		"popup":    "main.go",
		"chip":     "selicon.go",
		"more":     "selicon.go",
		"settings": "settings.go",
		"tray":     "tray.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(src), "safeView(\""+name+"\"") {
			t.Errorf("%s view (%s) is not wrapped in safeView: a render panic takes the whole process down", name, path)
		}
	}
}

// TestTranslateNilWindowGuard: translate() used to call win.Update without
// a nil check; a translation landing after the window was torn down
// crashed instead of being dropped.
func TestTranslateNilWindowGuard(t *testing.T) {
	src, err := os.ReadFile("popup_actions.go")
	if err != nil {
		t.Fatalf("read popup_actions.go: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "func (a *app) translate(text string)")
	if start < 0 {
		t.Fatal("translate not found")
	}
	end := strings.Index(body[start:], "\n}\n")
	if end < 0 {
		t.Fatal("translate body not delimited")
	}
	fn := body[start : start+end]
	// The guarded call is indented inside an "if win != nil" block; a bare
	// call would appear at exactly one tab of depth relative to the func.
	if strings.Contains(fn, "\n\twin.Update(func() {})\n") {
		t.Fatal("translate still calls win.Update unguarded")
	}
}

// TestVocabularyListIsVirtualized: the settings page used to render every
// entry as a full card tree each frame; the ui.List keeps a thousand
// entries smooth, and the tab counters are one pass, not four.
func TestVocabularyListIsVirtualized(t *testing.T) {
	src, err := os.ReadFile("settings_vocab.go")
	if err != nil {
		t.Fatalf("read settings_vocab.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "ui.List(c, &a.vocabList, len(entries)") {
		t.Fatal("pageVocabulary does not render entries through ui.List")
	}
	if strings.Contains(body, "for _, item := range entries") {
		t.Fatal("pageVocabulary still builds every entry per frame")
	}
	if strings.Contains(body, "vocabStore.CountByCategory(\"words\")") {
		t.Fatal("pageVocabulary still scans the store four times per frame; use CategoryCounts")
	}
}

// TestCategoryCountsSinglePass verifies the one-pass counter directly.
func TestCategoryCountsSinglePass(t *testing.T) {
	s := &VocabularyStore{path: filepath.Join(t.TempDir(), "vocabulary.json"), entries: []VocabularyEntry{
		{ID: "1", Term: "alpha", Source: "dict"},                      // words
		{ID: "2", Term: "a longer sentence entry", Source: "grammar"}, // sentences
		{ID: "3", Term: "polished text", Source: "writing"},           // writing
		{ID: "4", Term: "beta", Source: "notes"},                      // words
	}}
	all, words, sentences, writing := s.CategoryCounts()
	if all != 4 || words != 2 || sentences != 1 || writing != 1 {
		t.Fatalf("CategoryCounts = all:%d words:%d sentences:%d writing:%d, want 4/2/1/1", all, words, sentences, writing)
	}
}

// TestHTMLExportEscapesScriptTag: an entry containing </script> must not
// break the exported notebook page.
func TestHTMLExportEscapesScriptTag(t *testing.T) {
	dir := t.TempDir()
	s := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json"), entries: []VocabularyEntry{
		{ID: "x", Term: "</script><b>evil</b>", Definition: "d"},
	}}
	s.saveOrLog()
	path, err := s.ExportHTML()
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported html: %v", err)
	}
	page := string(data)
	if strings.Count(page, "</script>") != 1 {
		// exactly one: the page's own closing tag
		t.Fatalf("exported page has %d </script> occurrences, want 1 (unescaped entry leaked into the script block)", strings.Count(page, "</script>"))
	}
	if !strings.Contains(page, "&lt;/script&gt;") {
		t.Fatal("the term's markup is not HTML-escaped in the card body")
	}
}

// TestExportHTMLRefusesMemoryStore: an in-memory store must not write the
// notebook into whatever the current working directory happens to be.
func TestExportHTMLRefusesMemoryStore(t *testing.T) {
	s := &VocabularyStore{path: "", entries: []VocabularyEntry{{ID: "x", Term: "alpha"}}}
	if _, err := s.ExportHTML(); err == nil {
		t.Fatal("ExportHTML on a store with no backing file must fail, not write to the working directory")
	}
}

// TestImportEntriesReportsRealAdditions: the toast must say how many were
// new, not how many were attempted (duplicates merge silently).
func TestImportEntriesReportsRealAdditions(t *testing.T) {
	dir := t.TempDir()
	s := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json"), entries: []VocabularyEntry{}}
	first := s.ImportEntries([]VocabularyEntry{
		{Term: "alpha", Definition: "d", Difficulty: "A1", Phonetic: "/a/"},
		{Term: "beta", Definition: "d", Difficulty: "A1", Phonetic: "/b/"},
	})
	if first != 2 {
		t.Fatalf("first import added %d, want 2", first)
	}
	second := s.ImportEntries([]VocabularyEntry{
		{Term: "alpha", Definition: "d", Difficulty: "A1", Phonetic: "/a/"}, // duplicate: merges
		{Term: "gamma", Definition: "d", Difficulty: "A1", Phonetic: "/g/"},
	})
	if second != 1 {
		t.Fatalf("second import added %d, want 1 (alpha is a duplicate)", second)
	}
}

// TestOcrTextKeepsLines: OCR output keeps its line structure; folding the
// breaks away turned a paragraph into one unwieldy line.
func TestOcrTextKeepsLines(t *testing.T) {
	got := CleanOcrText("  first line  \r\nsecond\tline\r\n\r\nthird  ")
	want := "first line\nsecond line\n\nthird"
	if got != want {
		t.Fatalf("CleanOcrText = %q, want %q", got, want)
	}
	if CleanOcrText("a\nb") == "a b" {
		t.Fatal("CleanOcrText folded the newline into a space")
	}
}

// TestQuitCleansUpAudioAndHooks: quitApp used to leave a playing sound and
// the low-level hooks behind.
func TestQuitCleansUpAudioAndHooks(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "func (a *app) quitApp()")
	if start < 0 {
		t.Fatal("quitApp not found")
	}
	end := strings.Index(body[start:], "\n}\n")
	if end < 0 {
		t.Fatal("quitApp body not delimited")
	}
	fn := body[start : start+end]
	if !strings.Contains(fn, "stopAudio()") {
		t.Fatal("quitApp does not stop audio")
	}
	if !strings.Contains(fn, "removeHooks") {
		t.Fatal("quitApp does not remove the low-level hooks")
	}
}

// TestAudioCacheUsesPartFile: a crash mid-download used to leave a
// truncated mp3 that the size check then treated as a valid cache hit.
func TestAudioCacheUsesPartFile(t *testing.T) {
	src, err := os.ReadFile("audio.go")
	if err != nil {
		t.Fatalf("read audio.go: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "func FetchYoudaoAudio")
	if start < 0 {
		t.Fatal("FetchYoudaoAudio not found")
	}
	end := strings.Index(body[start:], "\n}\n")
	fn := body[start : start+end]
	if !strings.Contains(fn, ".part") {
		t.Fatal("FetchYoudaoAudio writes straight to the cache path; use .part + rename")
	}
}

// TestGooglePinnedModeIsNotGlobal: the pinned-engine timeout used to be a
// global atomic, so a pinned translation raced a concurrent race-mode one.
func TestGooglePinnedModeIsNotGlobal(t *testing.T) {
	src, err := os.ReadFile("services.go")
	if err != nil {
		t.Fatalf("read services.go: %v", err)
	}
	if strings.Contains(string(src), "singleEngineMode") {
		t.Fatal("singleEngineMode global still exists; pinned-ness must be a parameter")
	}
}

// TestDoubleClickUsesSystemThreshold: the hook's double-click window must
// come from GetDoubleClickTime, not a hardcoded 500ms.
func TestDoubleClickUsesSystemThreshold(t *testing.T) {
	src, err := os.ReadFile("hook.go")
	if err != nil {
		t.Fatalf("read hook.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "doubleClickThreshold()") {
		t.Fatal("the mouse hook does not read the system double-click threshold")
	}
	if strings.Contains(body, "doubleClickMS    = 500") {
		t.Fatal("the hardcoded 500ms double-click constant is still in use")
	}
}

// TestSettingsShowsRealVocabPath guards the display of where the notebook
// actually lives, rather than a path that can drift from reality.
func TestSettingsShowsRealVocabPath(t *testing.T) {
	src := readSourceFile(t, "settings_vocab.go")
	if containsText(src, `%AppData%\\Dictionary\\vocabulary.json`) {
		t.Fatal("the vocabulary page still prints a hardcoded path")
	}
	if !containsText(src, "vocabStore.Path()") {
		t.Fatal("the vocabulary page does not ask the store where it saves")
	}
	// And the store has to answer with something real.
	s := newMemoryVocabularyStore()
	if s.Path() != "" {
		t.Fatalf("the in-memory store reports a path (%q); it must not pretend", s.Path())
	}
	if p := defaultVocabPath(); !strings.HasSuffix(p, "vocabulary.json") {
		t.Fatalf("defaultVocabPath() = %q, not a vocabulary.json file", p)
	}
}

// TestVocabularyStoreReportsItsPath pins the accessor the settings page uses.
func TestVocabularyStoreReportsItsPath(t *testing.T) {
	dir := t.TempDir()
	s := &VocabularyStore{path: filepath.Join(dir, "vocabulary.json"), entries: []VocabularyEntry{}}
	if got := s.Path(); got != s.path {
		t.Fatalf("Path() = %q, want %q", got, s.path)
	}
	var nilStore *VocabularyStore
	if nilStore.Path() != "" {
		t.Fatal("a nil store must report an empty path rather than panic")
	}
}

// TestUIATimeoutValueIsSane pins the chosen bound.
func TestUIATimeoutValueIsSane(t *testing.T) {
	if uiaTimeout < 100*time.Millisecond || uiaTimeout > time.Second {
		t.Fatalf("uiaTimeout = %v, outside the sane 100ms..1s window", uiaTimeout)
	}
}

// TestAIModeSelectsTheOpeningCard covers the setting that used to be inert:
// "默认模式" was persisted and rendered but never read, so choosing 长难句分析
// changed nothing about what a fresh selection opened.
func TestAIModeSelectsTheOpeningCard(t *testing.T) {
	cases := []struct {
		aiMode  string
		aiOn    bool
		chosen  bool
		want    popupMode
		applies bool
	}{
		{"grammar", true, true, modeGrammar, true},
		{"translate", true, true, modeWriting, true},
		{"polish", true, true, modeWriting, true},
		{"", true, true, modeTranslate, false},
		{"nonsense", true, true, modeTranslate, false},
		// 词典精读 is retired with its tab; a file that still spells "dict"
		// must fall back to translating rather than open a view that no longer
		// exists. normalizeInitialAIMode erases the value on load.
		{"dict", true, true, modeTranslate, false},
		// With AI off the actions cannot run, so the preference must not
		// apply: opening an empty AI card is worse than translating.
		{"grammar", false, true, modeTranslate, false},
		// A stored ai_mode without an explicit choice is just the shipped
		// default ("grammar"), not a request for 长难句分析 on every
		// selection. Honouring it would reroute every existing install.
		{"grammar", true, false, modeTranslate, false},
	}
	for _, tc := range cases {
		cfg := DefaultConfig()
		cfg.AIMode = tc.aiMode
		cfg.AIEnabled = tc.aiOn
		cfg.AIModeIsChoice = tc.chosen
		got, applies := cfg.InitialAIMode()
		if got != tc.want || applies != tc.applies {
			t.Errorf("ai_mode=%q aiEnabled=%v chosen=%v → (%v, %v), want (%v, %v)",
				tc.aiMode, tc.aiOn, tc.chosen, modeName(got), applies, modeName(tc.want), tc.applies)
		}
	}
}

// TestDefaultConfigDoesNotClaimAModeChoice pins that the shipped default does
// not masquerade as a user preference: DefaultConfig sets AIMode to "grammar"
// for the settings page, and InitialAIMode must still decline to apply it.
func TestDefaultConfigDoesNotClaimAModeChoice(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AIModeIsChoice {
		t.Fatal("DefaultConfig marks the built-in ai_mode as a user choice")
	}
	if _, applies := cfg.InitialAIMode(); applies {
		t.Fatalf("a fresh config applies ai_mode %q instead of translating", cfg.AIMode)
	}
}

// TestSettingsRecordsTheModeChoice keeps the settings page honest: it has to
// flip AIModeIsChoice when a mode is picked, and offer a way back to plain
// translation, otherwise the preference can never be cleared again.
func TestSettingsRecordsTheModeChoice(t *testing.T) {
	src := readSourceFile(t, "settings_pages.go")
	if !containsText(src, "c.AIModeIsChoice = true") {
		t.Fatal("picking a 默认模式 does not mark it as a choice")
	}
	if !containsText(src, "c.AIModeIsChoice = false") {
		t.Fatal("there is no way to return to plain translation")
	}
	if !containsText(src, "跟随翻译") {
		t.Fatal("the settings page has no \"no preference\" option")
	}
}

// TestAIModeIsWiredIntoThePopup makes sure the preference is actually consulted
// where the selection popup opens, not merely computed and dropped.
func TestAIModeIsWiredIntoThePopup(t *testing.T) {
	src := readSourceFile(t, "popup.go")
	if !containsText(src, "InitialAIMode()") {
		t.Fatal("openPopup never asks the config which card to open")
	}
	// The mode must be dispatched through rerunCurrentMode: calling
	// translate() unconditionally is exactly the bug this replaced.
	if countOccurrences(src, "a.rerunCurrentMode(text)") < 2 {
		t.Fatal("openPopupWithText does not dispatch on the mode it just set")
	}
}

// TestCopyGivesFeedback covers the silent clipboard: both copy buttons wrote
// the clipboard and showed nothing, which reads as a dead button.
//
// The popup is split by responsibility: translation controls live in
// popup_translation.go, shared feedback in popup_view.go, and the clipboard
// action in popup_actions.go.
func TestCopyGivesFeedback(t *testing.T) {
	translation := readSourceFile(t, "popup_translation.go")
	view := readSourceFile(t, "popup_view.go")
	actions := readSourceFile(t, "popup_actions.go")
	src := translation + view + actions
	if countOccurrences(src, "mygo.Clipboard.WriteText(s.source)") != 0 {
		t.Fatal("复制原文 still writes the clipboard without any feedback")
	}
	if countOccurrences(src, "mygo.Clipboard.WriteText(s.target)") != 0 {
		t.Fatal("复制译文 still writes the clipboard without any feedback")
	}
	if !containsText(src, "a.copyToClipboard(s.source)") || !containsText(src, "a.copyToClipboard(s.target)") {
		t.Fatal("the copy buttons do not go through copyToClipboard")
	}
	if !containsText(src, "a.copiedMsg = ") {
		t.Fatal("copyToClipboard sets no message")
	}
	if !containsText(src, "if s.copiedMsg != \"\"") {
		t.Fatal("the popup never renders copiedMsg")
	}
}

// TestCopyToClipboardHandlesEmptyText pins the guard that keeps the button from
// reporting "copied 0 characters" on an empty result.
func TestCopyToClipboardHandlesEmptyText(t *testing.T) {
	a := newTestApp()
	a.copyToClipboard("   ")
	if a.copiedMsg != "" {
		t.Fatalf("copying blank text set a message: %q", a.copiedMsg)
	}
}

// TestNoOrphanedRefineComment keeps the leftover doc comment of a deleted
// function from coming back: it documented nothing and confused readers.
func TestNoOrphanedRefineComment(t *testing.T) {
	src := readSourceFile(t, "popup.go")
	if containsText(src, "// refine streams an AI re-translation") {
		t.Fatal("the orphaned refine comment is back with no function under it")
	}
	if countOccurrences(src, "\nfunc refine(") != 0 {
		t.Fatal("refine exists again; restore its documentation too")
	}
}
