package main

import (
	"image"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func chipTestTips(t *testing.T, a *app) []string {
	t.Helper()
	cfg := a.store.Get()
	tips := []string{
		"划词翻译（" + cfg.HotkeySelection + "）",
		"朗读选中文字",
		"截图翻译（" + cfg.HotkeyScreenshot + "）",
	}
	for _, act := range cfg.GetEnabledActions() {
		tips = append(tips, act.Name)
	}
	return append(tips, "复制与更多功能")
}

func chipTestTones() []chipTone {
	return []chipTone{
		chipTonePrimary, chipToneInk, chipToneInk,
		chipToneInk, chipToneInk, chipToneInk,
		chipToneInk,
	}
}

func chipThemeForTest(t *testing.T, dark bool) *ui.Theme {
	t.Helper()
	var got *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, map[bool]string{true: "dark", false: "light"}[dark], dark, "")
		got = c.Theme()
	}, 40, 20)
	tt.Frame()
	if got == nil {
		t.Fatal("applyTheme produced no theme")
	}
	if got.Dark != dark {
		t.Fatalf("applyTheme(dark=%v) gave a theme with Dark=%v", dark, got.Dark)
	}
	return got
}

func relativeLuminance(c ui.Color) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

func contrastRatio(a, b ui.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func rectDigest(img *image.RGBA, r ui.Rect) uint64 {
	var sum uint64
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			c := img.RGBAAt(x, y)
			sum += uint64(c.R)*3 + uint64(c.G)*5 + uint64(c.B)*7 + uint64(c.A)
		}
	}
	return sum
}

func TestChipTonesContrastTheSurface(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := chipThemeForTest(t, dark)
		for _, tone := range chipTestTones() {
			fg, _ := chipToneColors(theme, tone)
			if got := contrastRatio(fg, chipSurface(theme)); got < 3.0 {
				t.Errorf("dark=%v tone %d icon contrast %.2f:1 on the chip surface, want at least 3:1", dark, tone, got)
			}
		}
	}
}

func TestChipHoverUsesOneQuietWash(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := chipThemeForTest(t, dark)
		_, inkWash := chipToneColors(theme, chipToneInk)
		_, accentWash := chipToneColors(theme, chipTonePrimary)
		if inkWash == accentWash {
			t.Errorf("dark=%v: the primary action's wash is the same as every other button's, so it is no longer findable", dark)
		}
		for _, tone := range chipTestTones() {
			if tone != chipToneInk {
				continue
			}
			if _, wash := chipToneColors(theme, tone); wash != inkWash {
				t.Errorf("dark=%v: an ink button has a wash of its own (%v vs %v)", dark, wash, inkWash)
			}
		}
	}
}

func TestChipHoverChangesTheRenderedPixels(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.selIconView, chipWindowWidth(a.currentSelectionChipWidth()), chipWindowHeight())
	tt.Frame()

	r, ok := tt.Find("朗读选中文字")
	if !ok {
		t.Fatalf("speech button not found: %q", tt.Texts())
	}
	rest := rectDigest(tt.Image(), r)

	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(2 * chipHoverMotion.Duration)
	tt.Frame()
	hover := rectDigest(tt.Image(), r)

	if rest == hover {
		t.Errorf("button pixels are identical at rest and on hover: the hover wash never rendered")
	}
}

func TestChipPressedShrinksTheIcon(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	if !containsText(src, "iconEl.Scale(0.88, 0.88)") {
		t.Error("the pressed state no longer scales the icon")
	}
	if containsText(src, ".Width(chipIconSize+") || containsText(src, ".Height(chipIconSize+") {
		t.Error("press feedback resizes the button box, which would shift its neighbours")
	}
}

func TestChipIconsStayOneSize(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	if got := countOccurrences(src, ".Size(chipIconSize, chipIconSize)"); got != 1 {
		t.Errorf("icon size is set in %d places, want the single shared chipIconSize", got)
	}
}
