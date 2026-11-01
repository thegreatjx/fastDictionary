package main

import (
	"math"
)

// Detection post-processing for Indra's line detector.
//
// det.onnx outputs three channels at half the input resolution: a core-band
// logit, and the distance from each core pixel up to the top of its line and
// down to the bottom. Lines are found by thresholding the core, splitting it
// into connected components, and averaging those two distances per component.
// This follows indra_ocr/det_post.py; the constants are the original's.

const (
	// detCoreXShrink is how far past a component's horizontal extent the box
	// reaches, as a fraction of the line height. The core band is drawn
	// shorter than the line it belongs to, so the box has to grow back.
	detCoreXShrink = 0.15
	// detOutStride is output pixels per input pixel.
	detOutStride = 2
	// detInputMultiple is the input side's alignment requirement.
	detInputMultiple = 32
	// detLongSide is the page's long side after the detector's resize.
	detLongSide = 1024
	// detThresh is the core probability above which a pixel counts as inside.
	detThresh = 0.5
	// detMinArea drops specks: a component smaller than this is noise.
	detMinArea = 2
)

// detPageToInput resizes a page so its long side is detLongSide and pads to a
// multiple of detInputMultiple with paper. It returns the ink map (1 = ink) and
// the scale from page pixels to input pixels.
func detPageToInput(page grayImage) ([]float32, int, int, float64) {
	scale := float64(detLongSide) / float64(maxInt(page.H, page.W))
	nh := maxInt(1, int(math.Round(float64(page.H)*scale)))
	nw := maxInt(1, int(math.Round(float64(page.W)*scale)))
	small := resizeGray(page, nw, nh)

	ph := int(math.Ceil(float64(nh)/detInputMultiple)) * detInputMultiple
	pw := int(math.Ceil(float64(nw)/detInputMultiple)) * detInputMultiple

	// The ink map is inverted: paper is 0 so that padding blends with it.
	ink := make([]float32, ph*pw)
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			v := float32(small.Pix[y*nw+x]) / 255
			ink[y*pw+x] = 1 - v
		}
	}
	return ink, ph, pw, scale
}

// detDecode turns the detector's three output planes into line boxes in page
// pixels.
//
// out is [3][h][w]: core logit, up-distance, down-distance. The caller has
// already divided h and w by detOutStride relative to the input size.
func detDecode(out []float32, h, w int, scale float64) []lineBox {
	if len(out) < 3*h*w {
		return nil
	}
	core := out[:h*w]
	upDist := out[h*w : 2*h*w]
	dnDist := out[2*h*w : 3*h*w]

	// Threshold the core into a mask.
	mask := make([]bool, h*w)
	for i := 0; i < h*w; i++ {
		mask[i] = sigmoid(core[i]) > detThresh
	}

	comps := connectedComponents4(mask, h, w)
	boxes := make([]lineBox, 0, len(comps))
	for _, c := range comps {
		if c.area < detMinArea {
			continue
		}
		// Average the two distance fields over the component's pixels: each
		// core pixel knows how far the line extends above and below it, so the
		// mean over the component is the line's own extent.
		var sumTop, sumBot, sumProb float64
		for _, idx := range c.pixels {
			y := float64(idx/w) + 0.5
			sumTop += y*detOutStride - float64(upDist[idx])
			sumBot += y*detOutStride + float64(dnDist[idx])
			sumProb += float64(sigmoid(core[idx]))
		}
		n := float64(len(c.pixels))
		y0 := sumTop / n
		y1 := sumBot / n
		if y1 <= y0 {
			y1 = y0 + 1
		}

		lh := math.Max(1, y1-y0)
		ex := math.Min(detCoreXShrink*lh, 0.5*lh)
		x0 := float64(c.minX*detOutStride) - ex
		x1 := float64((c.maxX+1)*detOutStride) + ex

		// Back to page pixels.
		boxes = append(boxes, lineBox{
			X0: x0 / scale, Y0: y0 / scale,
			X1: x1 / scale, Y1: y1 / scale,
			Score: sumProb / n,
		})
	}
	return boxes
}

func sigmoid(x float32) float32 {
	return 1 / (1 + float32(math.Exp(-float64(x))))
}

// component is one connected region of the core mask.
type component struct {
	pixels     []int
	area       int
	minX, maxX int
	minY, maxY int
}

// connectedComponents4 labels the mask with 4-connectivity and returns each
// region's pixels and bounds.
//
// mygo has nothing like cv2.connectedComponentsWithStats, so this is a
// breadth-first flood fill with an explicit stack. 4-connectivity (not 8) is
// what the original uses: with 8, two lines one pixel apart diagonally would
// merge into one box.
func connectedComponents4(mask []bool, h, w int) []component {
	seen := make([]bool, h*w)
	stack := make([]int, 0, 256)
	var comps []component

	for start := 0; start < h*w; start++ {
		if !mask[start] || seen[start] {
			continue
		}
		c := component{minX: w, minY: h, maxX: -1, maxY: -1}
		stack = stack[:0]
		stack = append(stack, start)
		seen[start] = true

		for len(stack) > 0 {
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			c.pixels = append(c.pixels, idx)
			x, y := idx%w, idx/w
			if x < c.minX {
				c.minX = x
			}
			if x > c.maxX {
				c.maxX = x
			}
			if y < c.minY {
				c.minY = y
			}
			if y > c.maxY {
				c.maxY = y
			}
			// Four neighbours, in the order cv2 visits them.
			if x > 0 {
				if n := idx - 1; mask[n] && !seen[n] {
					seen[n] = true
					stack = append(stack, n)
				}
			}
			if x+1 < w {
				if n := idx + 1; mask[n] && !seen[n] {
					seen[n] = true
					stack = append(stack, n)
				}
			}
			if y > 0 {
				if n := idx - w; mask[n] && !seen[n] {
					seen[n] = true
					stack = append(stack, n)
				}
			}
			if y+1 < h {
				if n := idx + w; mask[n] && !seen[n] {
					seen[n] = true
					stack = append(stack, n)
				}
			}
		}
		c.area = len(c.pixels)
		comps = append(comps, c)
	}
	return comps
}

// sortBoxesByY orders lines top to bottom, then left to right, which is the
// order a person reads a block of text in.
func sortBoxesByY(boxes []lineBox) {
	for i := 1; i < len(boxes); i++ {
		for j := i; j > 0; j-- {
			a, b := boxes[j-1], boxes[j]
			if a.Y0 < b.Y0-1 || (math.Abs(a.Y0-b.Y0) <= 1 && a.X0 <= b.X0) {
				break
			}
			boxes[j-1], boxes[j] = boxes[j], boxes[j-1]
		}
	}
}
