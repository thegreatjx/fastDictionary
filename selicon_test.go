package main

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func oneDisplay() ([]geomDisplay, geomDisplay) {
	d := geomDisplay{
		Bounds:      mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1080},
		WorkArea:    mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040},
		ScaleFactor: 1,
	}
	return []geomDisplay{d}, d
}

func TestPlaceChipSitsBelowTheSelection(t *testing.T) {
	ds, p := oneDisplay()
	x, y := placeChip(ds, p, 400, 300)
	if x != 400 {
		t.Errorf("x = %d, want the anchor's x (400)", x)
	}
	if y != 300+chipGap {
		t.Errorf("y = %d, want %d (below the anchor)", y, 300+chipGap)
	}
}

func TestPlaceChipFlipsAboveWhenNoRoomBelow(t *testing.T) {
	ds, p := oneDisplay()
	_, y := placeChip(ds, p, 400, 1030)
	if y+selIconHeight > p.WorkArea.Height {
		t.Errorf("chip y=%d (+%d) escapes the work area height %d", y, selIconHeight, p.WorkArea.Height)
	}
	if y >= 1030 {
		t.Errorf("chip should flip above the anchor: y=%d anchor=1030", y)
	}
}

func TestPlaceChipStaysInsideRightEdge(t *testing.T) {
	ds, p := oneDisplay()
	x, _ := placeChip(ds, p, 1919, 300)
	width := selectionChipWidth(len(DefaultCustomActions()))
	if x+width > p.WorkArea.X+p.WorkArea.Width {
		t.Errorf("chip x=%d (+%d) escapes width %d", x, width, p.WorkArea.Width)
	}
	if x < 0 {
		t.Errorf("chip x=%d is negative", x)
	}
}

func TestPlaceChipUsesTheHoldingDisplay(t *testing.T) {
	first := geomDisplay{
		Bounds:      mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1080},
		WorkArea:    mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040},
		ScaleFactor: 1,
	}
	second := geomDisplay{
		Bounds:      mygo.Rectangle{X: 1920, Y: 0, Width: 2560, Height: 1440},
		WorkArea:    mygo.Rectangle{X: 1920, Y: 0, Width: 2560, Height: 1400},
		ScaleFactor: 1.5,
	}
	ds := []geomDisplay{first, second}

	px := int32((1920 + 100) * 1.5)
	x, y := physicalToDIPIn(ds, first, px, 300)
	if x != 2020 || y != 200 {
		t.Errorf("physicalToDIPIn = %d,%d, want 2020,200", x, y)
	}

	cx, _ := placeChip(ds, first, int32((1920+2550)*1.5), 300)
	if cx+selectionChipWidth(len(DefaultCustomActions())) > second.WorkArea.X+second.WorkArea.Width {
		t.Errorf("chip x=%d escapes the second display's work area", cx)
	}
}

func TestPlaceChipFallsBackToPrimary(t *testing.T) {
	ds, p := oneDisplay()
	x, y := physicalToDIPIn(ds, p, -500, -500)
	if x != -500 || y != -500 {
		t.Errorf("off-screen conversion = %d,%d, want -500,-500 at scale 1", x, y)
	}
}

func TestSelectionIconCornersAreTransparent(t *testing.T) {
	a := newTestApp()
	pillWidth := selectionChipWidth(len(DefaultCustomActions()))
	tt := ui.NewTester(a.selIconView, chipWindowWidth(pillWidth), chipWindowHeight())
	img := tt.Image()
	if img == nil {
		t.Fatal("no image rendered")
	}
	if _, _, _, alpha := img.At(0, 0).RGBA(); alpha > 48<<8 {
		t.Errorf("top-left pixel is %d/255 opaque: the root is painting a background", alpha>>8)
	}
	cx, cy := img.Bounds().Dx()/2, img.Bounds().Dy()/2
	if _, _, _, alpha := img.At(cx, cy).RGBA(); alpha == 0 {
		t.Errorf("centre pixel is transparent: the chip did not draw")
	}
}

func TestSelectionIconRenders(t *testing.T) {
	a := newTestApp()
	pillWidth := selectionChipWidth(len(DefaultCustomActions()))
	tt := ui.NewTester(a.selIconView, chipWindowWidth(pillWidth), chipWindowHeight())
	img := tt.Image()
	if img == nil {
		t.Fatal("no image rendered for selection toolbar")
	}
}

func TestSelectionIconHidesAIChipWhenDisabled(t *testing.T) {
}

func TestSelectionDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SelectionTrigger != "icon" {
		t.Errorf("default selection_trigger = %q, want icon", cfg.SelectionTrigger)
	}
	if cfg.SelectionCapture != "auto" {
		t.Errorf("default selection_capture = %q, want auto", cfg.SelectionCapture)
	}
}

func TestSelectionCaptureModeIsHonoured(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionCapture = "copy" })
	if got := a.store.Get().SelectionCapture; got != "copy" {
		t.Fatalf("capture mode did not stick: %q", got)
	}
}

func TestSelectionSettingsPageRenders(t *testing.T) {
	a := newTestApp()
	a.page = "selection"
	tt := ui.NewTester(a.settingsView, 760, 560)
	if len(tt.Texts()) == 0 {
		t.Error("selection page rendered nothing")
	}
	if !tt.HasText("显示图标") {
		t.Errorf("trigger options missing: %q", tt.Texts())
	}
}
