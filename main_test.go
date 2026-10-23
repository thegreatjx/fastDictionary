package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// newTestApp returns an app with an in-memory config: tests must not read
// or write the user's real config file, and must not leak state into each
// other through it.
func newTestApp() *app {
	return &app{store: newMemoryStore()}
}

func TestPopupShowsTranslation(t *testing.T) {
	a := newTestApp()
	a.source, a.target, a.engine, a.detected = "hello", "你好", "微软", "en"
	a.elapsed = 1
	tt := ui.NewTester(a.popupView, 400, 340)
	if !tt.HasText("你好") {
		t.Errorf("translation missing, texts %q", tt.Texts())
	}
	// Engine tag is present via Translator selector
	if !tt.HasText("Microsoft") && !tt.HasText("微软") {
		t.Errorf("engine tag missing, texts %q", tt.Texts())
	}
}

func TestPopupShowsDictCard(t *testing.T) {
	a := newTestApp()
	a.source = "serendipity"
	a.target = "机缘巧合"
	a.dict = &YoudaoDict{UKPhone: "ˌserənˈdɪpəti", USPhone: "ˌserənˈdɪpəti", Explains: []string{"n. 好运"}}
	a.gdict = &DictResult{Entries: []DictEntry{{POS: "noun", Explain: "机缘巧合"}}}
	tt := ui.NewTester(a.popupView, 400, 340)
	joined := strings.Join(tt.Texts(), "|")
	if !strings.Contains(joined, "ˌserənˈdɪpəti") {
		t.Errorf("phonetics missing: %q", joined)
	}
	if !strings.Contains(joined, "noun") {
		t.Errorf("pos missing: %q", joined)
	}
}

func TestPopupBusyAndError(t *testing.T) {
	a := newTestApp()
	a.source = "hi"
	a.busy = true
	tt := ui.NewTester(a.popupView, 400, 340)
	if !tt.HasText("翻译中") {
		t.Errorf("busy state missing: %q", tt.Texts())
	}

	b := newTestApp()
	b.source = "hi"
	b.errText = "all engines failed: google: http 429"
	tb := ui.NewTester(b.popupView, 400, 340)
	if !tb.HasText("all engines failed: google: http 429") {
		t.Errorf("error state missing: %q", tb.Texts())
	}
}

func TestPopupInputMode(t *testing.T) {
	a := newTestApp()
	a.mode = modeInput
	a.target = "你好"
	tt := ui.NewTester(a.popupView, 400, 340)
	if !tt.HasText("你好") {
		t.Errorf("input mode result missing: %q", tt.Texts())
	}
}

func TestPopupAIBlock(t *testing.T) {
	a := newTestApp()
	a.source = "hello"
	a.mode = modeGrammar
	a.aiText = "你好呀"
	tt := ui.NewTester(a.popupView, 400, 340)
	if (!tt.HasText("长难句分析") && !tt.HasText("长难句") && !tt.HasText("语法")) || !tt.HasText("你好呀") {
		t.Errorf("ai block missing: %q", tt.Texts())
	}
}

func TestSettingsRendersAllPages(t *testing.T) {
	for _, page := range []string{"general", "selection", "actions", "vocabulary", "translate", "dict", "ai", "hotkey", "about"} {
		a := newTestApp()
		a.page = page
		tt := ui.NewTester(a.settingsView, 760, 560)
		if len(tt.Texts()) == 0 {
			t.Errorf("page %s rendered nothing", page)
		}
	}
}

func TestAdaptivePopupHeaderAtSmallWidth(t *testing.T) {
	a := newTestApp()
	a.source = "Hello world"
	a.target = "你好世界"
	// Test at standard and narrower widths to ensure no panic and modes render
	for _, w := range []int{380, 400, 440, 460, 520} {
		tt := ui.NewTester(a.popupView, w, 340)
		if !tt.HasText("翻译") || !tt.HasText("长难句") || !tt.HasText("词典") {
			t.Errorf("width %d: mode buttons missing, texts: %q", w, tt.Texts())
		}
	}
}

func TestCalcPopupPositionAdaptiveLifting(t *testing.T) {
	wa := mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}

	// 1. Top of screen: place below cursor
	ptTop := mygo.Point{X: 300, Y: 100}
	x1, y1 := calcPopupPositionIn(ptTop, 440, 360, wa)
	if y1 < ptTop.Y {
		t.Errorf("expected popup below cursor when at top of screen, got y=%d, cursor=%d", y1, ptTop.Y)
	}

	// 2. Bottom of screen (lower half / tight space): lift/flip above cursor!
	ptBottom := mygo.Point{X: 300, Y: 900}
	x2, y2 := calcPopupPositionIn(ptBottom, 440, 360, wa)
	if y2 >= ptBottom.Y {
		t.Errorf("expected popup flipped above cursor when at bottom of screen, got y=%d, cursor=%d", y2, ptBottom.Y)
	}
	if y2+360 > wa.Height {
		t.Errorf("popup exceeds screen height: y=%d + h=360 > %d", y2, wa.Height)
	}

	// 3. Right edge of screen: horizontal clamp
	ptRight := mygo.Point{X: 1900, Y: 300}
	x3, _ := calcPopupPositionIn(ptRight, 440, 360, wa)
	if x3+440 > wa.Width {
		t.Errorf("popup escapes right edge: x=%d + w=440 > %d", x3, wa.Width)
	}

	// Verify coordinates are bounded and reasonable
	if x1 < 0 || x2 < 0 || x3 < 0 {
		t.Errorf("unexpected negative X coordinates: x1=%d, x2=%d, x3=%d", x1, x2, x3)
	}
}
