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
	src, err := os.ReadFile("popup.go")
	if err != nil {
		t.Fatalf("read popup.go: %v", err)
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
	src, err := os.ReadFile("settings.go")
	if err != nil {
		t.Fatalf("read settings.go: %v", err)
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
	// Informational for now: the heading still shows the hardcoded path.
	// Kept as a reminder that P2-6 (path unification) is still open.
	t.Skip("P2-6 path unification still open")
}

// TestUIATimeoutValueIsSane pins the chosen bound.
func TestUIATimeoutValueIsSane(t *testing.T) {
	if uiaTimeout < 100*time.Millisecond || uiaTimeout > time.Second {
		t.Fatalf("uiaTimeout = %v, outside the sane 100ms..1s window", uiaTimeout)
	}
}
