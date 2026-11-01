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
	if !tt.HasText("正在翻译") {
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

// The build stamp is how the user tells a fresh binary from the one the
// shortcut used to launch. If it stops reaching 关于, the only way to answer
// "did my build take effect" is to compare file timestamps by hand again.
func TestAboutPageShowsBuildStamp(t *testing.T) {
	old := buildStamp
	buildStamp = "2026-10-25T07:23"
	t.Cleanup(func() { buildStamp = old })

	a := newTestApp()
	a.page = "about"
	tt := ui.NewTester(a.settingsView, 760, 560)
	if !tt.HasText("构建") || !tt.HasText("2026-10-25T07:23") {
		t.Errorf("about page missing build stamp: %q", tt.Texts())
	}
}

func TestAdaptivePopupHeaderKeepsReadingToolsReachable(t *testing.T) {
	for _, w := range []int{360, 380, 400, 440, 460, 520} {
		a := newTestApp()
		a.source, a.target, a.popupW = "Hello world", "你好世界", w
		tt := ui.NewTester(a.popupView, w, 340)
		for _, want := range []string{"翻译", "复制译文", "复制原文", "朗读原文", "翻译引擎", "重新翻译"} {
			if !tt.HasText(want) {
				t.Errorf("width %d: missing %q, texts: %q", w, want, tt.Texts())
			}
		}
		modeLabel := "阅读工具"
		if w < 440 {
			modeLabel = "切换阅读工具"
		} else {
			for _, want := range popupModeLabels {
				if !tt.HasText(want) {
					t.Errorf("width %d: direct mode %q missing", w, want)
				}
			}
		}
		for _, label := range []string{modeLabel, "简体中文", "固定窗口（失焦不关闭）", "翻译设置"} {
			r, ok := tt.Find(label)
			if !ok {
				t.Errorf("width %d: %q not found", w, label)
				continue
			}
			if r.X < 0 || r.X+r.W > float32(w) {
				t.Errorf("width %d: %q clips at %+v", w, label, r)
			}
		}
		if tt.HasText("本地窗口 · 在线翻译") {
			t.Errorf("width %d: implementation detail is shown in place of engine controls", w)
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

func TestRenderTranslatePopupScreenshot(t *testing.T) {
	a := newTestApp()
	a.source = "The quick brown fox jumps over the lazy dog. A journey of a thousand miles begins with a single step."
	a.target = "敏捷的棕色狐狸跳过了懒狗。千里之行，始于足下。"
	a.engine = "Microsoft Translator"
	a.popupW = 500
	tt := ui.NewTester(a.popupView, 500, 260)
	img := tt.Image()
	if img == nil {
		t.Fatal("expected rendered popup image")
	}
}
