package main

import (
	"image"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// chipTestTips returns every chip button's label, in render order.
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

// chipTestTones lists the tone of each tip above, in the same order: the first
// button is the primary action, every other one shares the slate ink.
func chipTestTones() []chipTone {
	return []chipTone{
		chipTonePrimary, chipToneInk, chipToneInk,
		chipToneInk, chipToneInk, chipToneInk,
		chipToneInk,
	}
}

// chipThemeForTest returns the theme the chip really draws with, taken through
// applyTheme rather than rebuilt by hand, so a change to the palette reaches
// these checks instead of leaving them asserting stale hex values.
func chipThemeForTest(t *testing.T, dark bool) *ui.Theme {
	t.Helper()
	var got *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, map[bool]string{true: "dark", false: "light"}[dark], dark)
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

// relativeLuminance is the WCAG 2.x definition, used to check icon contrast.
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

// contrastRatio is the WCAG contrast of two opaque colours.
func contrastRatio(a, b ui.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// rectDigest sums the pixels inside r, so two renders of the same button can be
// compared without depending on which single pixel the icon happens to cover.
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

// TestChipTonesAreDistinct guarded the old per-action palette: six hues so the
// row could be read by colour. The redesign spends colour once, on the primary
// action, so the check that matters is now the opposite one — see
// TestChipSpendsColourOnlyOnThePrimaryAction.
//
// TestChipTonesContrastTheSurface keeps every icon legible on the chip in both
// themes. 3:1 is the non-text contrast floor: below it an icon that carries
// meaning stops being reliably visible. With the hues gone, this is the check
// that stops the redesign from going too pale to read.
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

// TestChipHoverUsesOneQuietWash replaces the old per-tone wash check. A single
// slate wash for every button is what makes the row read as one toolbar; the
// primary action is the only one allowed a different tint.
func TestChipHoverUsesOneQuietWash(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := chipThemeForTest(t, dark)
		_, inkWash := chipToneColors(theme, chipToneInk)
		_, accentWash := chipToneColors(theme, chipTonePrimary)
		if inkWash == accentWash {
			t.Errorf("dark=%v: the primary action's wash is the same as every other button's, so it is no longer findable", dark)
		}
		// Every ink button must share one wash.
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

// TestChipHoverChangesTheRenderedPixels is the end-to-end check: the same
// button draws differently with the pointer on it. A hover state that only
// exists in the source but never reaches the screen is the failure this
// catches. The clock advances past chipHoverMotion, because the wash is
// animated rather than switched on.
func TestChipHoverChangesTheRenderedPixels(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.selIconView, a.currentSelectionChipWidth(), selIconHeight)
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

// TestChipPressedShrinksTheIcon keeps the press feedback from shifting the
// layout: the icon scales around its own centre, so no neighbour moves and the
// row cannot jitter as the pointer crosses it.
func TestChipPressedShrinksTheIcon(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	if !containsText(src, "iconEl.Scale(0.88, 0.88)") {
		t.Error("the pressed state no longer scales the icon")
	}
	if containsText(src, ".Width(chipIconSize+") || containsText(src, ".Height(chipIconSize+") {
		t.Error("press feedback resizes the button box, which would shift its neighbours")
	}
}

// TestChipIconsStayOneSize keeps the visual rhythm: every icon is drawn at the
// same size, so the colours differentiate the actions and the size does not.
func TestChipIconsStayOneSize(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	if got := countOccurrences(src, ".Size(chipIconSize, chipIconSize)"); got != 1 {
		t.Errorf("icon size is set in %d places, want the single shared chipIconSize", got)
	}
}
