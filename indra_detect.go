package main

import (
	"math"
)

const (
	detCoreXShrink   = 0.15
	detOutStride     = 2
	detInputMultiple = 32
	detLongSide      = 1024
	detThresh        = 0.5
	detMinArea       = 2
)

func detPageToInput(page grayImage) ([]float32, int, int, float64) {
	scale := float64(detLongSide) / float64(maxInt(page.H, page.W))
	nh := maxInt(1, int(math.Round(float64(page.H)*scale)))
	nw := maxInt(1, int(math.Round(float64(page.W)*scale)))
	small := resizeGray(page, nw, nh)

	ph := int(math.Ceil(float64(nh)/detInputMultiple)) * detInputMultiple
	pw := int(math.Ceil(float64(nw)/detInputMultiple)) * detInputMultiple

	ink := make([]float32, ph*pw)
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			v := float32(small.Pix[y*nw+x]) / 255
			ink[y*pw+x] = 1 - v
		}
	}
	return ink, ph, pw, scale
}

func detDecode(out []float32, h, w int, scale float64) []lineBox {
	if len(out) < 3*h*w {
		return nil
	}
	core := out[:h*w]
	upDist := out[h*w : 2*h*w]
	dnDist := out[2*h*w : 3*h*w]

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

type component struct {
	pixels     []int
	area       int
	minX, maxX int
	minY, maxY int
}

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
