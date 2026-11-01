package main

import (
	"image"
	"math"
)

// Image handling for the Indra recognizer.
//
// This mirrors read-frog-style preprocessing in the Indra package
// (indra_ocr/rec_preproc.py and det_post.py). The recognizers were trained on
// crops made by exactly these rules, so the margins and the ink-map convention
// have to match: a line fed with different padding reads worse even though
// nothing is "wrong" with it.

// Recognizer geometry, from Indra's rec_preproc.py.
const (
	// indraLineHeight is the height a line crop is resized to. rec_cjk.onnx
	// declares [B,1,32,W]; the Latin recognizer wants 96, see indraRecHeight.
	indraLineHeight = 32
	// indraLatinHeight is rec.onnx's input height. It is trained at a larger
	// scale than the CJK recognizer, so the same line is cropped taller.
	indraLatinHeight = 96
	// indraPadY and indraPadX are margins around the ink box, as a fraction of
	// the line's height. They give the recognizer context above and below.
	indraPadY = 0.2
	// indraWidthMultiple is the timestep stride: widths are padded up to a
	// multiple of it.
	indraWidthMultiple = 8
	// indraMaxLineWidth caps a single crop, from the original's max_width.
	indraMaxLineWidth = 4096
)

// indraPadX is the horizontal margin around the ink box, as a fraction of the
// line's height. It is a variable, not a constant, because it is the knob the
// doubling sweep turns; 0.3 is the original Python value.
var indraPadX = 0.3

// grayImage is a single-channel page, the form both models take.
type grayImage struct {
	W, H int
	Pix  []uint8 // row-major, 0 = black
}

// toGray converts an image to 8-bit grayscale.
//
// The recognizers read dark ink on light paper, and the detector works on an
// inverted map, so a single gray buffer serves both.
//
// The concrete types are handled directly rather than through image.Image.At:
// At is an interface call per pixel, and on a 640x940 screenshot that alone cost
// 166ms — twenty times the cost of the recognition it fed. Decoded screenshots
// are almost always one of these three, so the slow path is rare.
func toGray(src image.Image) grayImage {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	g := grayImage{W: w, H: h, Pix: make([]uint8, w*h)}

	// Rec.601 luma, the same weights cv2's BGR2GRAY uses.
	luma := func(r, gg, bb uint8) uint8 {
		return uint8((299*uint32(r) + 587*uint32(gg) + 114*uint32(bb)) / 1000)
	}

	switch m := src.(type) {
	case *image.RGBA:
		for y := 0; y < h; y++ {
			si := m.PixOffset(b.Min.X, b.Min.Y+y)
			row := g.Pix[y*w : (y+1)*w]
			for x := 0; x < w; x++ {
				p := m.Pix[si+x*4:]
				if p[3] == 0 {
					row[x] = 255
					continue
				}
				row[x] = luma(p[0], p[1], p[2])
			}
		}
		return g
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			si := m.PixOffset(b.Min.X, b.Min.Y+y)
			row := g.Pix[y*w : (y+1)*w]
			for x := 0; x < w; x++ {
				p := m.Pix[si+x*4:]
				if p[3] == 0 {
					row[x] = 255
					continue
				}
				// NRGBA is already non-premultiplied.
				row[x] = luma(p[0], p[1], p[2])
			}
		}
		return g
	case *image.Gray:
		for y := 0; y < h; y++ {
			si := m.PixOffset(b.Min.X, b.Min.Y+y)
			copy(g.Pix[y*w:(y+1)*w], m.Pix[si:si+w])
		}
		return g
	}

	// Anything else: ask the pixel. Slower, but correct.
	for y := 0; y < h; y++ {
		row := g.Pix[y*w : (y+1)*w]
		for x := 0; x < w; x++ {
			r, gg, bb, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 {
				row[x] = 255
				continue
			}
			row[x] = luma(uint8(r>>8), uint8(gg>>8), uint8(bb>>8))
		}
	}
	return g
}

// resizeGray resizes with the same choices as the original: area averaging when
// shrinking (which is what keeps small text readable instead of aliasing it
// away) and bilinear when growing.
func resizeGray(src grayImage, w, h int) grayImage {
	if w == src.W && h == src.H {
		return src
	}
	if w <= 0 || h <= 0 {
		return grayImage{W: 1, H: 1, Pix: []uint8{255}}
	}
	dst := grayImage{W: w, H: h, Pix: make([]uint8, w*h)}
	shrinking := w < src.W || h < src.H
	if shrinking {
		resizeArea(src, dst)
	} else {
		resizeBilinear(src, dst)
	}
	return dst
}

// resizeArea averages the source pixels that fall inside each destination pixel,
// the way cv2.INTER_AREA does. This is the mode that preserves thin strokes.
func resizeArea(src, dst grayImage) {
	sx := float64(src.W) / float64(dst.W)
	sy := float64(src.H) / float64(dst.H)
	for y := 0; y < dst.H; y++ {
		y0 := float64(y) * sy
		y1 := y0 + sy
		for x := 0; x < dst.W; x++ {
			x0 := float64(x) * sx
			x1 := x0 + sx
			var sum, cnt float64
			for yy := int(y0); yy < int(math.Ceil(y1)); yy++ {
				if yy < 0 || yy >= src.H {
					continue
				}
				wy := math.Min(y1, float64(yy+1)) - math.Max(y0, float64(yy))
				if wy <= 0 {
					continue
				}
				for xx := int(x0); xx < int(math.Ceil(x1)); xx++ {
					if xx < 0 || xx >= src.W {
						continue
					}
					wx := math.Min(x1, float64(xx+1)) - math.Max(x0, float64(xx))
					if wx <= 0 {
						continue
					}
					w := wx * wy
					sum += float64(src.Pix[yy*src.W+xx]) * w
					cnt += w
				}
			}
			if cnt > 0 {
				dst.Pix[y*dst.W+x] = uint8(math.Round(sum / cnt))
			}
		}
	}
}

// resizeBilinear is the plain bilinear interpolation cv2.INTER_LINEAR does.
func resizeBilinear(src, dst grayImage) {
	sx := float64(src.W) / float64(dst.W)
	sy := float64(src.H) / float64(dst.H)
	for y := 0; y < dst.H; y++ {
		fy := (float64(y)+0.5)*sy - 0.5
		y0 := int(math.Floor(fy))
		dy := fy - float64(y0)
		if y0 < 0 {
			y0, dy = 0, 0
		}
		y1 := y0 + 1
		if y1 >= src.H {
			y1 = src.H - 1
		}
		for x := 0; x < dst.W; x++ {
			fx := (float64(x)+0.5)*sx - 0.5
			x0 := int(math.Floor(fx))
			dx := fx - float64(x0)
			if x0 < 0 {
				x0, dx = 0, 0
			}
			x1 := x0 + 1
			if x1 >= src.W {
				x1 = src.W - 1
			}
			p00 := float64(src.Pix[y0*src.W+x0])
			p10 := float64(src.Pix[y0*src.W+x1])
			p01 := float64(src.Pix[y1*src.W+x0])
			p11 := float64(src.Pix[y1*src.W+x1])
			top := p00*(1-dx) + p10*dx
			bot := p01*(1-dx) + p11*dx
			dst.Pix[y*dst.W+x] = uint8(math.Round(top*(1-dy) + bot*dy))
		}
	}
}

// lineBox is a detected text line, in page pixels.
type lineBox struct {
	X0, Y0, X1, Y1 float64
	Score          float64
}

// cropLine cuts one line and returns it as the recognizer's ink map: a
// row-major [height][w] float32 where 1 is ink and 0 is paper.
//
// window is how many line-heights of page the crop covers, from meta.json's
// rec_window. The Latin recognizer reads a tall window — it wants the line plus
// the page above and below it, which is why rec.onnx declares an input height of
// 96 while rec_cjk.onnx declares 32: 3 x 32 = 96. Cropping a Latin line at 32
// without the surrounding context made it read far worse than the CJK model
// ("January 6-29" came back as "danuar\forall -20"), which is how this was found.
//
// The remaining steps match Indra's crop_line: pad the ink box by a fraction of
// its height, fill outside the page with paper, resize keeping the aspect ratio,
// then invert unless the line was light text on a dark ground.
func cropLine(page grayImage, box lineBox, height int, window int) ([]float32, int) {
	if window < 1 {
		window = 1
	}
	h := math.Max(1, box.Y1-box.Y0)
	// A window of n line-heights is centred on the line: the padding grows so
	// that the line keeps its scale inside the taller crop.
	padY := (float64(window)*(1+2*indraPadY) - 1) / 2
	height *= window

	X0 := box.X0 - indraPadX*h
	X1 := box.X1 + indraPadX*h
	Y0 := box.Y0 - padY*h
	Y1 := box.Y1 + padY*h

	ix0, iy0 := int(math.Floor(X0)), int(math.Floor(Y0))
	ix1, iy1 := int(math.Ceil(X1)), int(math.Ceil(Y1))
	cw, ch := maxInt(1, ix1-ix0), maxInt(1, iy1-iy0)

	dark := isDarkLine(page, box)
	fill := uint8(255)
	if dark {
		fill = 0
	}
	crop := grayImage{W: cw, H: ch, Pix: make([]uint8, cw*ch)}
	for i := range crop.Pix {
		crop.Pix[i] = fill
	}
	for y := 0; y < ch; y++ {
		sy := iy0 + y
		if sy < 0 || sy >= page.H {
			continue
		}
		for x := 0; x < cw; x++ {
			sx := ix0 + x
			if sx < 0 || sx >= page.W {
				continue
			}
			crop.Pix[y*cw+x] = page.Pix[sy*page.W+sx]
		}
	}

	outW := int(math.Round(float64(cw) * float64(height) / float64(ch)))
	if outW < indraWidthMultiple {
		outW = indraWidthMultiple
	}
	if outW > indraMaxLineWidth {
		outW = indraMaxLineWidth
	}
	resized := resizeGray(crop, outW, height)

	ink := make([]float32, outW*height)
	for i, v := range resized.Pix {
		f := float32(v) / 255
		if dark {
			ink[i] = f
		} else {
			ink[i] = 1 - f
		}
	}
	return ink, outW
}

// isDarkLine reports light text on a dark ground.
//
// It compares the line's median luminance against its dark and bright tails: if
// the bright tail is further from the median than the dark tail is, the ground
// is the dark part and the text is light. This is what lets a line of white
// text on a coloured banner be recognised at all, since the recognizers only
// read dark on light.
func isDarkLine(page grayImage, box lineBox) bool {
	x0, y0 := int(box.X0), int(box.Y0)
	x1, y1 := int(box.X1), int(box.Y1)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= page.W {
		x1 = page.W - 1
	}
	if y1 >= page.H {
		y1 = page.H - 1
	}
	if x1 < x0 || y1 < y0 {
		return false
	}
	var hist [256]int
	n := 0
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			hist[page.Pix[y*page.W+x]]++
			n++
		}
	}
	if n == 0 {
		return false
	}
	lo, med, hi := histPercentile(&hist, n, 0.02), histPercentile(&hist, n, 0.50), histPercentile(&hist, n, 0.98)
	return hi-med > 2*(med-lo)+8
}

// histPercentile reads a percentile out of a 256-bin histogram without sorting.
func histPercentile(hist *[256]int, n int, q float64) float64 {
	x := q * float64(n-1)
	i := int(math.Floor(x))
	g := x - float64(i)
	lo := histSearch(hist, i)
	hi := histSearch(hist, minInt(i+1, n-1))
	d := hi - lo
	if g >= 0.5 {
		return hi - d*(1-g)
	}
	return lo + d*g
}

// histSearch returns the value whose cumulative count first exceeds idx,
// matching numpy.searchsorted(..., side="right").
func histSearch(hist *[256]int, idx int) float64 {
	cum := 0
	for v := 0; v < 256; v++ {
		cum += hist[v]
		if cum > idx {
			return float64(v)
		}
	}
	return 255
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
