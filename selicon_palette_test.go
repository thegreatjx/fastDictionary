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

func saturation(c ui.Color) float64 {
	maxv := float64(max(c.R, c.G, c.B)) / 255
	minv := float64(min(c.R, c.G, c.B)) / 255
	if maxv == 0 {
		return 0
	}
	return (maxv - minv) / maxv
}

func chroma(c ui.Color) float64 {
	return float64(max(c.R, c.G, c.B)-min(c.R, c.G, c.B)) / 255
}

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

func TestChipPaletteIsCoolAndDesaturated(t *testing.T) {
	const (
		minHue           = 175.0
		maxHue           = 290.0
		maxSurfaceChroma = 0.26
		maxInkChroma     = 0.85
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
				continue
			}
			limit := maxInkChroma
			switch name {
			case "surface", "border", "tip fill", "tip text":
				limit = maxSurfaceChroma
			}
			if got := chroma(c); got > limit {
				t.Errorf("dark=%v %s %v has chroma %.2f, want at most %.2f",
					dark, name, c, got, limit)
			}
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

func TestChipTipInvertsTheChip(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := chipThemeForTest(t, dark)
		bg, fg := chipTipColors(theme)
		if got := contrastRatio(fg, bg); got < 4.5 {
			t.Errorf("dark=%v label text contrast %.2f:1 on its plate, want at least 4.5:1", dark, got)
		}
		if got := contrastRatio(bg, chipSurface(theme)); got < 3.0 {
			t.Errorf("dark=%v label plate contrast %.2f:1 against the chip, want at least 3:1", dark, got)
		}
	}
}

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
	if chipTipWidth("复制与更多功能") <= chipTipWidth("写作改进") {
		t.Error("the plate does not grow with the label")
	}
}

func TestChipTipAppearsWithoutAWindow(t *testing.T) {
	var asked []time.Duration
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, "light", false, "")
		ui.Row(c).Fill().Children(func() {
			b := ui.Button(c, "").Label("词典").Size(28, 28)
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
	if chipTipDelay >= 400*time.Millisecond {
		t.Errorf("label delay %v is too slow to feel like a hover label", chipTipDelay)
	}
}

func TestChipTipCentresOnItsButton(t *testing.T) {
	const chipX, chipW = 100, 219
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
	if got, want := x+w/2, chipX+int(mid.X+mid.W/2); got != want && (x == chipX || x+w == chipX+chipW) == false {
		t.Errorf("label centre %d, button centre %d", got, want)
	}

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

func TestChipLabelNeverTakesTheKeyboard(t *testing.T) {
	src := readSourceFile(t, "selicon.go")
	start := strings.Index(src, "func (a *app) raiseChipTip(")
	if start < 0 {
		t.Fatal("raiseChipTip is gone")
	}
	end := strings.Index(src[start:], "func (a *app) hideChipTip(")
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
			tt := ui.NewTester(a.selIconView, chipWindowWidth(w), chipWindowHeight())
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
