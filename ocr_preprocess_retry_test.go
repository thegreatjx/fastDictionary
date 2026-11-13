package main

import (
	"image"
	"image/color"
	"testing"
)

func fillRGBA(width, height int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestAdaptivePadForOCRUsesEdgeColor(t *testing.T) {
	src := fillRGBA(100, 80, color.RGBA{R: 31, G: 31, B: 31, A: 255})
	padded := adaptivePadForOCR(src)
	wantPad := ocrMinEdgePad // 0.30 * min(100,80) = 24
	if got := padded.Bounds().Dx(); got != 100+2*wantPad {
		t.Fatalf("padded width = %d, want %d", got, 100+2*wantPad)
	}
	if got := padded.Bounds().Dy(); got != 80+2*wantPad {
		t.Fatalf("padded height = %d, want %d", got, 80+2*wantPad)
	}
	corner := color.RGBAModel.Convert(padded.At(0, 0)).(color.RGBA)
	if corner.R != 31 || corner.G != 31 || corner.B != 31 {
		t.Fatalf("padded corner = %+v, want replicated dark edge", corner)
	}
}

func TestColorDistanceForOCRSeparatesEqualLumaColors(t *testing.T) {
	bg := color.RGBA{R: 180, G: 100, B: 100, A: 255}
	fg := color.RGBA{R: 80, G: 150, B: 105, A: 255}
	src := fillRGBA(200, 60, bg)
	for y := 20; y < 40; y++ {
		for x := 50; x < 150; x++ {
			src.SetRGBA(x, y, fg)
		}
	}
	out := colorDistanceForOCR(src)
	paper := color.RGBAModel.Convert(out.At(5, 5)).(color.RGBA)
	ink := color.RGBAModel.Convert(out.At(100, 30)).(color.RGBA)
	if paper.R < 200 || paper.G < 200 || paper.B < 200 {
		t.Fatalf("background did not become white paper: %+v", paper)
	}
	if ink.R > 80 || ink.G > 80 || ink.B > 80 {
		t.Fatalf("equal-luma foreground did not become dark ink: %+v", ink)
	}
}

func TestBackgroundSubtractForOCRProducesInk(t *testing.T) {
	src := fillRGBA(160, 80, color.RGBA{R: 40, G: 80, B: 120, A: 255})
	block := color.RGBA{R: 240, G: 240, B: 240, A: 255}
	for y := 25; y < 55; y++ {
		for x := 40; x < 120; x++ {
			src.SetRGBA(x, y, block)
		}
	}
	out := backgroundSubtractForOCR(src)
	foundInk := false
	for y := 0; y < 80 && !foundInk; y++ {
		for x := 0; x < 160; x++ {
			c := color.RGBAModel.Convert(out.At(x, y)).(color.RGBA)
			if c.R < 100 {
				foundInk = true
				break
			}
		}
	}
	if !foundInk {
		t.Fatal("background subtraction produced no dark ink")
	}
}

func TestIsSuspiciousOCRText(t *testing.T) {
	cases := map[string]bool{
		"":                       true,
		"st and efficient\nF\na": true,
		"□ and efficient":        true,
		"Fast and efficient":     false,
		"January 6-29\nFridays & Saturdays at 8 pm": false,
	}
	for text, want := range cases {
		if got := isSuspiciousOCRText(text); got != want {
			t.Errorf("isSuspiciousOCRText(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestScoreOCRTextPrefersCompleteResult(t *testing.T) {
	complete := scoreOCRText("Fast and efficient")
	fragmented := scoreOCRText("st and efficient\nF\na")
	if complete <= fragmented {
		t.Fatalf("complete score %d must beat fragmented score %d", complete, fragmented)
	}
}

func TestNeedsColorRetry(t *testing.T) {
	lowContrast := fillRGBA(80, 40, color.RGBA{R: 200, G: 200, B: 200, A: 255})
	if !needsColorRetry(lowContrast) {
		t.Fatal("low dynamic range should allow a colour retry")
	}
	normal := fillRGBA(80, 40, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	black := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	for y := 10; y < 30; y++ {
		for x := 10; x < 70; x++ {
			normal.SetRGBA(x, y, black)
		}
	}
	if needsColorRetry(normal) {
		t.Fatal("clean white paper should stay on the fast path")
	}
}
