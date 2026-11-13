package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPopupWidthMatchesSettings(t *testing.T) {
	for input, want := range map[float32]float32{0: 360, 359: 360, 360: 360, 400: 400, 460: 460, 680: 680, 900: 680} {
		if got := popupWidth(input); got != want {
			t.Errorf("popupWidth(%v) = %v, want %v", input, got, want)
		}
	}
}

func TestNotesDisabledSkipsExtraction(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.ShowNotes, c.AIEnabled = false, true })
	a.extractNotes("This sentence is deliberately longer than twenty characters.")
	if a.notesBusy || len(a.notes) != 0 {
		t.Fatal("disabled recommendations started an AI request")
	}
}

func TestNotesEnabledByDefault(t *testing.T) {
	if !DefaultConfig().ShowNotes {
		t.Fatal("猜你想存 should be on for a fresh install")
	}
}

// TestNotesMigrationTurnsOldConfigOn covers the users who had already run a
// build that defaulted show_notes to false: their file says false, so changing
// the default alone would never reach them.
func TestNotesMigrationTurnsOldConfigOn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// A file from before the marker existed, with the card switched off.
	old := DefaultConfig()
	old.ShowNotes = false
	old.NotesMigrated = 0
	if err := saveConfig(path, old); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	s := &store{path: path}
	if !s.load() {
		t.Fatal("load reported no config file")
	}
	if !s.Get().ShowNotes {
		t.Fatal("old config was not migrated to the new default")
	}
}

// TestNotesMigrationRunsOnce keeps a deliberate choice: someone who turns the
// card off after the migration must not find it back on after a restart.
func TestNotesMigrationRunsOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	cfg.NotesMigrated = 1
	cfg.ShowNotes = false
	if err := saveConfig(path, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	s := &store{path: path}
	s.load()
	s.Update(func(c *Config) { c.ShowNotes = false })

	reloaded := &store{path: path}
	reloaded.load()
	if reloaded.Get().ShowNotes {
		t.Fatal("migration re-enabled the card after the user turned it off")
	}
}

func TestScreenshotEditingAndRecognitionState(t *testing.T) {
	a := newTestApp()
	a.mode, a.isScreenshotMode = modeTranslate, true
	a.ocrBusy, a.pinned = true, true
	tt := ui.NewTester(a.popupView, 460, 420)
	for _, want := range []string{"正在识别文字", "取消"} {
		if !tt.HasText(want) {
			t.Errorf("OCR progress misses %q: %q", want, tt.Texts())
		}
	}

	a.ocrBusy = false
	a.ocrText = "A correctable OCR result."
	a.ocrGeneration++
	if popupContentHeight(a.snap(), 460) < 430 {
		t.Fatal("editing screenshot result should allocate a usable editor")
	}
	tt = ui.NewTester(a.popupView, 460, 460)
	for _, want := range []string{"识别原文 · 可直接修正", "翻译修正后的原文", "重试", "AI 精翻"} {
		if !tt.HasText(want) {
			t.Errorf("OCR editing misses %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("AI 看图") {
		t.Errorf("OCR editing still shows a separate AI 看图 action: %q", tt.Texts())
	}
	a.ocrError = "本地识别不可用，请检查 Windows 设置。"
	tt = ui.NewTester(a.popupView, 460, 460)
	if !tt.HasText("本地识别不可用") {
		t.Fatal("OCR errors should remain visible instead of replacing the editable source")
	}
}

func TestStopScreenshotOCRInvalidatesLateResult(t *testing.T) {
	a := newTestApp()
	ctx, cancel := context.WithCancel(context.Background())
	a.ocrBusy, a.ocrCancel = true, cancel
	a.stopScreenshotOCR()
	if ctx.Err() != context.Canceled || a.ocrBusy || a.ocrCancel != nil {
		t.Fatal("closing OCR did not cancel the native process")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ocrGeneration != 1 {
		t.Errorf("generation = %d, want 1", a.ocrGeneration)
	}
}

func TestSelectionChipPrimaryActions(t *testing.T) {
	a := newTestApp()
	width := a.currentSelectionChipWidth()
	tt := ui.NewTester(a.selIconView, width, selIconHeight)
	texts := strings.Join(tt.Texts(), "|")
	for _, want := range []string{"划词翻译", "朗读选中文字", "截图翻译", "长难句分析", "写作改进", "复制与更多功能"} {
		if !strings.Contains(texts, want) {
			t.Errorf("selection chip misses %q: %q", want, texts)
		}
		r, ok := tt.Find(want)
		if !ok {
			continue
		}
		if r.X < 0 || r.X+r.W > float32(width) {
			t.Errorf("selection chip clips %q: bounds %+v, width %d", want, r, width)
		}
	}
	// 词典 is not a popup tab any more, so the chip must not offer it either.
	if strings.Contains(texts, "词典") {
		t.Errorf("selection chip still offers the retired 词典 action: %q", texts)
	}
	if got, want := selectionChipWidth(2), 189; got != want {
		t.Fatalf("default chip width = %d, want %d", got, want)
	}
}

func TestSelectionChipWidthFollowsEnabledActions(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) {
		for i := range c.CustomActions {
			c.CustomActions[i].Enabled = c.CustomActions[i].ID == "default-sentence-analysis"
		}
	})
	width := a.currentSelectionChipWidth()
	tt := ui.NewTester(a.selIconView, width, selIconHeight)
	if !tt.HasText("长难句分析") || tt.HasText("写作改进") || tt.HasText("词典") {
		t.Fatalf("chip did not follow the enabled actions: %q", tt.Texts())
	}
	r, ok := tt.Find("复制与更多功能")
	if !ok || r.X+r.W > float32(width) {
		t.Fatalf("adaptive chip clips its last action: %+v, width %d", r, width)
	}
}

func TestPopupSystemThemeFollowsNativeTheme(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.ThemeMode = "system" })
	// applyTheme must receive the native system state, not merely compare the
	// string "system" with "dark". Both callers now pass c.Theme().Dark.
	for _, dark := range []bool{false, true} {
		tt := ui.NewTester(func(c *ui.Context) {
			base := ui.LightTheme()
			if dark {
				base = ui.DarkTheme()
			}
			c.SetTheme(base)
			applyTheme(c, a.store.Get().ThemeMode, c.Theme().Dark)
			if c.Theme().Dark != dark {
				t.Errorf("system theme dark=%v became %v", dark, c.Theme().Dark)
			}
		}, 360, 260)
		_ = tt
	}
}

func TestPrimarySpeechTargetAvoidsRecitingChinese(t *testing.T) {
	// Case 1: Looking up English text translated to Chinese
	// Must pronounce English source, NOT Chinese target.
	text, lang, tip := primarySpeechTarget("Hello world", "你好世界")
	if text != "Hello world" || tip != "朗读原文" {
		t.Errorf("English source lookup: got (%q, %q), want ('Hello world', '朗读原文')", text, tip)
	}
	if !strings.HasPrefix(lang, "en") {
		t.Errorf("English source must carry an English tag, got %q: "+
			"the voice picker uses it and an empty one reads as English anyway", lang)
	}

	// Case 2: Looking up Chinese text translated to English
	// Must pronounce English target.
	text, lang, tip = primarySpeechTarget("你好", "Hello")
	if text != "Hello" || tip != "朗读英文译文" {
		t.Errorf("Chinese to English translation: got (%q, %q), want ('Hello', '朗读英文译文')", text, tip)
	}
	if !strings.HasPrefix(lang, "en") {
		t.Errorf("English target must carry an English tag, got %q", lang)
	}

	// Case 3: The tag must follow the text, not the source. A French
	// sentence translated to Chinese is read in French.
	text, lang, tip = primarySpeechTarget("Il fait beau aujourd'hui", "今天天气很好")
	if text != "Il fait beau aujourd'hui" || tip != "朗读原文" {
		t.Errorf("French source: got (%q, %q)", text, tip)
	}
	if lang != "" && !strings.HasPrefix(lang, "fr") {
		t.Errorf("French source tagged %q", lang)
	}
}

// TestNotesMigrationIsPersisted covers the write-back: the marker has to reach
// the file, or the next launch sees NotesMigrated == 0 again and re-applies the
// default over a choice the user has since made.
func TestNotesMigrationIsPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	old := DefaultConfig()
	old.ShowNotes = false
	old.NotesMigrated = 0
	if err := saveConfig(path, old); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	s := &store{path: path}
	if !s.load() {
		t.Fatal("load reported no config file")
	}
	if !s.notesMigrated {
		t.Fatal("load did not report the migration, so the caller cannot persist it")
	}
	if err := saveConfig(path, s.Get()); err != nil {
		t.Fatalf("persist: %v", err)
	}

	reopened := &store{path: path}
	reopened.load()
	if reopened.Get().NotesMigrated != 1 {
		t.Fatalf("notes_migrated did not reach the file: %d", reopened.Get().NotesMigrated)
	}
	if reopened.notesMigrated {
		t.Fatal("the migration ran a second time on an already-migrated file")
	}
}

// TestNotesMigrationHandlesFileWithoutTheKey is the case that actually ships:
// a config written by an older build has no notes_migrated key at all. Decoding
// into DefaultConfig() leaves absent keys at their default, and the default
// carries the marker, so the presence of the key in the JSON is what has to be
// tested — not the decoded number.
func TestNotesMigrationHandlesFileWithoutTheKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{"show_notes":false,"engine_mode":"microsoft"}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	s := &store{path: path}
	if !s.load() {
		t.Fatal("load reported no config file")
	}
	if !s.Get().ShowNotes {
		t.Fatal("a file without notes_migrated kept show_notes off")
	}
	if !s.notesMigrated {
		t.Fatal("load did not report the migration for a file without the key")
	}
}
