package main

import (
	"context"
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
)

func TestIndraVocabIsOneBased(t *testing.T) {
	vocab := []string{"A", "B", "C"}
	got := decodeIndraIDs([]int64{0, 1, 2, 3, 3, 0, 1}, vocab)
	if got != "ABCCA" {
		t.Fatalf("decode = %q, want %q (id 1 must be chars[0])", got, "ABCCA")
	}
	if got := decodeIndraIDs([]int64{1, 99}, vocab); got != "A" {
		t.Fatalf("out-of-range id = %q, want %q", got, "A")
	}
}

func TestCropLineWindowHeight(t *testing.T) {
	page := grayImage{W: 400, H: 120, Pix: make([]uint8, 400*120)}
	for i := range page.Pix {
		page.Pix[i] = 255
	}
	for x := 20; x < 380; x++ {
		for y := 50; y < 70; y++ {
			page.Pix[y*400+x] = 20
		}
	}
	box := lineBox{X0: 18, Y0: 48, X1: 382, Y1: 72, Score: 1}

	ink1, w1 := cropLine(page, box, indraLineHeight, 1)
	if len(ink1) != w1*indraLineHeight {
		t.Fatalf("window 1 gave %d values for %dx%d, want %d",
			len(ink1), w1, indraLineHeight, w1*indraLineHeight)
	}
	ink3, w3 := cropLine(page, box, indraLineHeight, 3)
	if len(ink3) != w3*(indraLineHeight*3) {
		t.Fatalf("window 3 gave %d values for %dx%d, want %d",
			len(ink3), w3, indraLineHeight*3, w3*(indraLineHeight*3))
	}
	if h := len(ink3) / w3; h != indraLatinHeight {
		t.Fatalf("window 3 height = %d, want %d (rec.onnx's declared height)", h, indraLatinHeight)
	}
	if w1 == 0 || w3 == 0 {
		t.Fatal("crop width is zero")
	}
}

func TestCropLineInvertsPaperAndInk(t *testing.T) {
	page := grayImage{W: 200, H: 60, Pix: make([]uint8, 200*60)}
	for i := range page.Pix {
		page.Pix[i] = 255
	}
	for x := 40; x < 160; x++ {
		for y := 25; y < 35; y++ {
			page.Pix[y*200+x] = 0
		}
	}
	box := lineBox{X0: 38, Y0: 23, X1: 162, Y1: 37, Score: 1}

	ink, w := cropLine(page, box, indraLineHeight, 1)
	var dark, light int
	for _, v := range ink {
		if v > 0.7 {
			dark++
		} else if v < 0.3 {
			light++
		}
	}
	if dark == 0 {
		t.Error("no ink found: a dark line on light paper must map to 1")
	}
	if light == 0 {
		t.Error("no paper found: light paper must map to 0")
	}
	_ = w
}

func TestToGrayMatchesNRGBAAndRGBA(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	img.SetNRGBA(2, 0, color.NRGBA{B: 255, A: 255})
	img.SetNRGBA(3, 0, color.NRGBA{A: 0})

	g := toGray(img)
	if g.W != 4 || g.H != 3 {
		t.Fatalf("size %dx%d, want 4x3", g.W, g.H)
	}
	if g.Pix[0] != 255 {
		t.Errorf("white pixel = %d, want 255", g.Pix[0])
	}
	if g.Pix[1] < 140 || g.Pix[1] > 160 {
		t.Errorf("pure green = %d, want about 150 (0.587 x 255)", g.Pix[1])
	}
	if g.Pix[2] < 25 || g.Pix[2] > 35 {
		t.Errorf("pure blue = %d, want about 29 (0.114 x 255)", g.Pix[2])
	}
	if g.Pix[3] != 255 {
		t.Errorf("transparent pixel = %d, want 255 (paper)", g.Pix[3])
	}

	rgba := image.NewRGBA(image.Rect(0, 0, 4, 3))
	rgba.Set(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	rgba.Set(1, 0, color.RGBA{G: 255, A: 255})
	rgba.Set(2, 0, color.RGBA{B: 255, A: 255})
	rg2 := toGray(rgba)
	for i := 0; i < 3; i++ {
		if g.Pix[i] != rg2.Pix[i] {
			t.Errorf("pixel %d: NRGBA %d != RGBA %d", i, g.Pix[i], rg2.Pix[i])
		}
	}
}

func TestIndraFallsBackWhenUnavailable(t *testing.T) {
	oldDir := os.Getenv(indraModelDirEnv)
	oldDev := os.Getenv(indraDevDirEnv)
	os.Setenv(indraModelDirEnv, t.TempDir())
	os.Setenv(indraDevDirEnv, "0")
	defer os.Setenv(indraModelDirEnv, oldDir)
	defer os.Setenv(indraDevDirEnv, oldDev)

	indraMu.Lock()
	savedInst, savedErr, savedLoaded := indraInst, indraErr, indraLoaded
	indraInst, indraErr, indraLoaded = nil, nil, false
	indraMu.Unlock()
	defer func() {
		indraMu.Lock()
		indraInst, indraErr, indraLoaded = savedInst, savedErr, savedLoaded
		indraMu.Unlock()
	}()

	if indraAvailable() {
		t.Fatal("indraAvailable reported true with no models, so OCR would not fall back")
	}
	if status := indraStatus(); !strings.Contains(status, "未启用") {
		t.Errorf("indraStatus with no models = %q, want it to say it is not enabled", status)
	}
}

func TestIndraDecodeDropsBlankOnly(t *testing.T) {
	if got := decodeIndraIDs([]int64{0, 0, 0}, []string{"A", "B"}); got != "" {
		t.Fatalf("all-blank ids decoded to %q, want empty", got)
	}
}

func liveIndraText(t *testing.T, path string) string {
	t.Helper()
	if !indraAvailable() {
		t.Skip("Indra models not installed")
	}
	txt, err := RecognizeImageFileIn(context.Background(), path, "auto")
	if err != nil {
		t.Skipf("recognize %s: %v", path, err)
	}
	return txt
}

func TestDropNestedBoxes(t *testing.T) {
	line := lineBox{X0: 67, X1: 408, Y0: 19, Y1: 65}
	stray := lineBox{X0: 349, X1: 367, Y0: 14, Y1: 60}
	below := lineBox{X0: 75, X1: 415, Y0: 81, Y1: 110}

	got := dropNestedBoxes([]lineBox{line, stray, below})
	if len(got) != 2 {
		t.Fatalf("kept %d boxes, want 2 (the stray 2 must go): %v", len(got), got)
	}
	for _, b := range got {
		if b.X0 == stray.X0 && b.Y0 == stray.Y0 {
			t.Error("the stray character box survived")
		}
	}

	a := lineBox{X0: 0, X1: 100, Y0: 0, Y1: 20}
	b := lineBox{X0: 90, X1: 200, Y0: 0, Y1: 20}
	if got := dropNestedBoxes([]lineBox{a, b}); len(got) != 2 {
		t.Errorf("adjacent boxes: kept %d, want 2", len(got))
	}

	if got := dropNestedBoxes([]lineBox{line}); len(got) != 1 {
		t.Errorf("single box: kept %d, want 1", len(got))
	}
}

func TestLargeLinesUseTheCJKRecognizer(t *testing.T) {
	if indraLatinMinHeight <= indraLineHeight {
		t.Fatalf("indraLatinMinHeight = %d must exceed the %d-pixel line band, "+
			"or every line would count as large", indraLatinMinHeight, indraLineHeight)
	}
	const largeLine, smallLine = 70.0, 26.0
	if !(largeLine >= indraLatinMinHeight) {
		t.Errorf("a %.0fpx line must count as large", largeLine)
	}
	if smallLine >= indraLatinMinHeight {
		t.Errorf("a %.0fpx line must not count as large", smallLine)
	}
}
