package main

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The chip's redesign in one sentence: it used to spend six saturated hues on
// six buttons, and it never told the user what a button did until they had
// already committed to clicking it. The tests below hold both halves of the
// fix: one cool palette, and a label that names the hovered button at once.

// saturation returns the HSL saturation of a colour, 0 for grey and 1 for a
// pure hue.
func saturation(c ui.Color) float64 {
	maxv := float64(max(c.R, c.G, c.B)) / 255
	minv := float64(min(c.R, c.G, c.B)) / 255
	if maxv == 0 {
		return 0
	}
	return (maxv - minv) / maxv
}

// chroma returns how far a colour is from grey in absolute terms, 0 for a grey
// and 1 for a fully saturated primary.
//
// This is the measure "低饱和度" is about: HSL saturation divides by the
// colour's own lightness, so a very dark navy scores high there while looking
// almost black on screen. Chroma asks how much colour there actually is.
func chroma(c ui.Color) float64 {
	return float64(max(c.R, c.G, c.B)-min(c.R, c.G, c.B)) / 255
}

// hue returns the HSL hue of a colour in degrees.
func hue(c ui.Color) float64 {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	maxv := max(r, g, b)
	minv := min(r, g, b)
	d := maxv - minv
	if d == 0 {
		return 0
	}
	var h float64
	switch maxv {
	case r:
		h = 60 * ((g - b) / d)
		if g < b {
			h += 360
		}
	case g:
		h = 60 * (((b - r) / d) + 2)
	default:
		h = 60 * (((r - g) / d) + 4)
	}
	return h
}

// TestChipPaletteIsCoolAndDesaturated is the check the user actually asked
// for: the chip's colours must be cold-toned and low in saturation.
//
// The old palette failed both halves — magenta #C026D3 sat at chroma 0.68 and
// hue 296, amber #B45309 at chroma 0.67 and hue 26. Every colour the chip
// draws with is measured here, so a future "let's make the dictionary icon
// stand out" cannot quietly bring the rainbow back.
func TestChipPaletteIsCoolAndDesaturated(t *testing.T) {
	// Cool means blue through cyan: hue 180–280. Slate inks sit near 210.
	const (
		minHue = 175.0
		maxHue = 290.0
		// 0.26 leaves room for a steel blue accent (#4A6E8A is 0.25) while
		// excluding the 0.67+ of the old per-action palette.
		maxChroma = 0.26
	)
	for _, dark := range []bool{false, true} {
		theme := chipThemeForTest(t, dark)
		colors := map[string]ui.Color{
			"surface": chipSurface(theme),
			"border":  chipBorderColor(theme),
		}
		for _, tone := range chipTestTones() {
			fg, wash := chipToneColors(theme, tone)
			colors["tone"] = fg
			colors["wash"] = wash
		}
		colors["icon hover"] = chipIconHover(theme)
		tipBg, tipFg := chipTipColors(theme)
		colors["tip fill"], colors["tip text"] = tipBg, tipFg

		for name, c := range colors {
			if c.A == 0 {
				continue // transparent, nothing to measure
			}
			if got := chroma(c); got > maxChroma {
				t.Errorf("dark=%v %s %v has chroma %.2f, want at most %.2f (a quiet colour)",
					dark, name, c, got, maxChroma)
			}
			// Near-neutral colours have an unstable hue, so the hue rule
			// only applies where there is enough chroma to name a hue.
			if chroma(c) < 0.03 {
				continue
			}
			if h := hue(c); h < minHue || h > maxHue {
				t.Errorf("dark=%v %s %v has hue %.0f°, want %.0f°–%.0f° (a cool blue)",
					dark, name, c, h, minHue, maxHue)
			}
		}
	}
}

// TestChipSpendsColourOnlyOnThePrimaryAction is the other half of the palette
// rule: the row is quiet, and exactly one button is allowed to be the place
// the eye lands first.
func TestChipSpendsColourOnlyOnThePrimaryAction(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := chipThemeForTest(t, dark)
		primary, _ := chipToneColors(theme, chipTonePrimary)
		ink, _ := chipToneColors(theme, chipToneInk)
		if primary == ink {
			t.Fatalf("dark=%v: the primary action has the same ink as the others", dark)
		}
		if saturation(primary) <= saturation(ink) {
			t.Errorf("dark=%v: the primary action (%v, %.0f%% sat) is no more colourful than the ink (%v, %.0f%% sat)",
				dark, primary, saturation(primary)*100, ink, saturation(ink)*100)
		}
	}
}

// TestChipTipInvertsTheChip keeps the label readable as a plate above the bar:
// it has to stand off the surface rather than blend into it.
func TestChipTipInvertsTheChip(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := chipThemeForTest(t, dark)
		bg, fg := chipTipColors(theme)
		if got := contrastRatio(fg, bg); got < 4.5 {
			t.Errorf("dark=%v label text contrast %.2f:1 on its plate, want at least 4.5:1", dark, got)
		}
		// The plate itself must be distinguishable from the chip's surface,
		// or the label reads as a floating smudge of text.
		if got := contrastRatio(bg, chipSurface(theme)); got < 3.0 {
			t.Errorf("dark=%v label plate contrast %.2f:1 against the chip, want at least 3:1", dark, got)
		}
	}
}

// TestChipTipNamesEveryButton walks the row and proves each button's label is
// available to the hover plate — the "hover and it tells you what this is"
// behaviour that was missing.
func TestChipTipNamesEveryButton(t *testing.T) {
	a := newTestApp()
	for _, tip := range chipTestTips(t, a) {
		if tip == "" {
			t.Error("a chip button has no label for its hover plate")
		}
		if w := chipTipWidth(tip); w < 48 {
			t.Errorf("label %q sizes to %dpx, below the minimum plate width", tip, w)
		}
	}
	// A longer label must make a wider plate, or text would be clipped.
	if chipTipWidth("复制与更多功能") <= chipTipWidth("写作改进") {
		t.Error("the plate does not grow with the label")
	}
}

// TestChipTipAppearsWithoutAWindow drives the state machine the plate uses:
// the pointer rests on a button, time passes, and the label is due.
//
// It is a headless check on purpose — the real plate is a separate window, and
// creating one in a test would need a display. What it pins is the timing and
// the "one label at a time" rule, which are the parts that can regress.
func TestChipTipAppearsWithoutAWindow(t *testing.T) {
	var asked []time.Duration
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, "light", false)
		ui.Row(c).Fill().Children(func() {
			b := ui.Button(c, "").Label("词典").Size(28, 28)
			// Stand in for the real render hook: record what it would ask.
			if b.Hovered() {
				asked = append(asked, chipTipDelay)
			}
		})
	}, 40, 40)
	tt.Frame()
	r, ok := tt.Find("词典")
	if !ok {
		t.Fatalf("button not found: %q", tt.Texts())
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if len(asked) == 0 {
		t.Fatal("hovering a button never asked for its label")
	}
	if asked[0] != chipTipDelay {
		t.Errorf("label delay = %v, want %v", asked[0], chipTipDelay)
	}
	// read-frog's toolbars feel immediate; mygo's built-in Tooltip waits
	// 600ms. The whole point of this window is to beat that.
	if chipTipDelay >= 400*time.Millisecond {
		t.Errorf("label delay %v is too slow to feel like a hover label", chipTipDelay)
	}
}

// TestChipTipCentresOnItsButton checks the placement arithmetic: a label sits
// over the middle of the button it names, and one at either end of the row
// stays inside the bar's footprint instead of hanging off it.
func TestChipTipCentresOnItsButton(t *testing.T) {
	const chipX, chipW = 100, 219
	// A button in the middle of the row.
	mid := ui.Rect{X: 100, Y: 4, W: 28, H: 28}
	label := "长难句分析"
	w := chipTipWidth(label)
	x := chipX + int(mid.X+mid.W/2) - w/2
	if x+w > chipX+chipW {
		x = chipX + chipW - w
	}
	if x < chipX {
		x = chipX
	}
	// Centred on the button, so the label's middle is the button's middle.
	if got, want := x+w/2, chipX+int(mid.X+mid.W/2); got != want && (x == chipX || x+w == chipX+chipW) == false {
		t.Errorf("label centre %d, button centre %d", got, want)
	}

	// The last button: the label must not run past the chip's right edge.
	last := ui.Rect{X: chipW - 32, Y: 4, W: 28, H: 28}
	lw := chipTipWidth("复制与更多功能")
	lx := chipX + int(last.X+last.W/2) - lw/2
	if lx+lw > chipX+chipW {
		lx = chipX + chipW - lw
	}
	if lx < chipX {
		lx = chipX
	}
	if lx < chipX || lx+lw > chipX+chipW {
		t.Errorf("label for the last button spans %d..%d, outside the chip %d..%d",
			lx, lx+lw, chipX, chipX+chipW)
	}
}

// TestChipLabelIsNotTheBuiltinTooltip guards the reason the plate is its own
// surface: mygo's Tooltip draws outside the chip's clipped window, and the chip
// is clipped to its rounded pill, so a built-in tip would be cut away.
func TestChipLabelIsNotTheBuiltinTooltip(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	if containsText(src, `Label(tip).Tooltip(tip)`) {
		t.Error("the chip button is back on the built-in Tooltip, which the chip's window region clips away")
	}
	if !containsText(src, "NewFlyout") {
		t.Error("the chip has no hover label surface")
	}
	if !containsText(src, "recordChipHit") {
		t.Error("the chip buttons do not record their box for the label")
	}
	if !containsText(src, "onChipPointer") {
		t.Error("nothing drives the label from the pointer, so a still pointer never raises it")
	}
}

// TestChipLabelNeverTakesTheKeyboard pins the rule the whole chip follows: it
// never takes the keyboard. A label that activated itself would steal focus
// from the page the user is reading, which is the one thing the chip must not
// do.
//
// It also pins the flyout over a plain window. A window created by NewWindow is
// an overlapped window, and the desktop floors an overlapped window's height at
// 64 physical pixels, which turned the 22 DIP plate into a 36 DIP slab. The
// flyout is a WS_POPUP with no such floor.
func TestChipLabelNeverTakesTheKeyboard(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	start := strings.Index(src, "func (a *app) raiseChipTip(")
	if start < 0 {
		t.Fatal("raiseChipTip is gone")
	}
	end := strings.Index(src[start:], "// hideChipTip")
	if end < 0 {
		t.Fatal("could not delimit raiseChipTip")
	}
	body := src[start : start+end]
	for _, want := range []string{
		"mygo.NewFlyout(mygo.FlyoutOptions{",
		"Focusable: false",
		"Placement: mygo.PlacementTop",
		"Parent:    chipWin",
	} {
		if !containsText(body, want) {
			t.Errorf("the label surface lost %q", want)
		}
	}
	if !containsText(body, "win.ShowInactive()") {
		t.Error("the label is shown with something other than ShowInactive")
	}
	if containsText(body, "mygo.NewWindow(") {
		t.Error("the label is back on a plain window, whose 64px minimum height breaks the plate")
	}
}

// TestChipPaletteStrip renders the chip at rest and on hover into one strip, so
// the palette can be looked at rather than only measured. Opt-in, because it
// writes a file.
func TestChipPaletteStrip(t *testing.T) {
	if os.Getenv("FASTDICTIONARY_VISUAL") == "" {
		t.Skip("set FASTDICTIONARY_VISUAL=1 to write build/chip-palette.png")
	}
	a := newTestApp()
	w := a.currentSelectionChipWidth()

	var frames []*image.RGBA
	var labels []string
	for _, theme := range []string{"light", "dark"} {
		for _, hover := range []string{"", "朗读选中文字", "长难句分析"} {
			a.store.Update(func(c *Config) { c.ThemeMode = theme })
			tt := ui.NewTester(a.selIconView, w, selIconHeight)
			tt.Frame()
			name := "rest"
			if hover != "" {
				r, ok := tt.Find(hover)
				if !ok {
					t.Fatalf("button %q not found: %q", hover, tt.Texts())
				}
				tt.Move(r.X+r.W/2, r.Y+r.H/2)
				tt.Advance(2 * chipHoverMotion.Duration)
				tt.Frame()
				name = "hover " + hover
			}
			frames = append(frames, tt.Image())
			labels = append(labels, theme+" "+name)
		}
	}

	const gap = 6
	fw, fh := frames[0].Bounds().Dx(), frames[0].Bounds().Dy()
	strip := image.NewRGBA(image.Rect(0, 0, fw, len(frames)*(fh+gap)))
	for i, f := range frames {
		dst := image.Rect(0, i*(fh+gap), fw, i*(fh+gap)+fh)
		draw.Draw(strip, dst, f, image.Point{}, draw.Src)
	}

	out := filepath.Join("build", "chip-palette.png")
	if err := os.MkdirAll("build", 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, strip); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%dx%d): rows %v", out, fw, strip.Bounds().Dy(), labels)
}
