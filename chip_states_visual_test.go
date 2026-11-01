package main

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestChipStateStrip renders the chip at rest, on hover and pressed into one
// strip, so the micro-interaction can be looked at rather than only measured.
// It is opt-in (FASTDICTIONARY_VISUAL=1) because it writes a file; the pixel checks in
// selicon_tone_test.go are what run in the normal suite.
func TestChipStateStrip(t *testing.T) {
	if os.Getenv("FASTDICTIONARY_VISUAL") == "" {
		t.Skip("set FASTDICTIONARY_VISUAL=1 to write build/chip-states.png")
	}
	a := newTestApp()
	w := a.currentSelectionChipWidth()
	const target = "长难句分析"

	type state struct {
		name string
		act  func(tt *ui.Tester, r ui.Rect)
	}
	states := []state{
		{"rest", func(tt *ui.Tester, r ui.Rect) {}},
		{"hover", func(tt *ui.Tester, r ui.Rect) {
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
			tt.Advance(2 * chipHoverMotion.Duration)
		}},
		{"pressed", func(tt *ui.Tester, r ui.Rect) {
			tt.Press(r.X+r.W/2, r.Y+r.H/2)
			tt.Advance(2 * chipHoverMotion.Duration)
		}},
	}

	frames := make([]*image.RGBA, 0, len(states)*2)
	labels := make([]string, 0, len(states)*2)
	for _, theme := range []string{"light", "dark"} {
		for _, s := range states {
			a.store.Update(func(c *Config) { c.ThemeMode = theme })
			tt := ui.NewTester(a.selIconView, w, selIconHeight)
			tt.Frame()
			r, ok := tt.Find(target)
			if !ok {
				t.Fatalf("button %q not found: %q", target, tt.Texts())
			}
			s.act(tt, r)
			tt.Frame()
			frames = append(frames, tt.Image())
			labels = append(labels, theme+" "+s.name)
		}
	}

	const gap = 6
	fw, fh := frames[0].Bounds().Dx(), frames[0].Bounds().Dy()
	strip := image.NewRGBA(image.Rect(0, 0, fw, len(frames)*fh+(len(frames)-1)*gap))
	for i, f := range frames {
		dst := image.Rect(0, i*(fh+gap), fw, i*(fh+gap)+fh)
		draw.Draw(strip, dst, f, image.Point{}, draw.Src)
	}

	out := filepath.Join("build", "chip-states.png")
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
