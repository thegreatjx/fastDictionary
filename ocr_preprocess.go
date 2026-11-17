package main

import (
	"image"
	"image/color"
	"image/draw"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"
)

const (
	ocrMinEdgePad     = 24
	ocrMaxEdgePad     = 64
	ocrPadRatio       = 0.30
	ocrLowContrast    = 95
	ocrDarkBackground = 100
	ocrColorSpread    = 25
)

func flattenRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), &image.Uniform{C: color.RGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Over)
	return out
}

func adaptivePadForOCR(src image.Image) image.Image {
	flat := flattenRGBA(src)
	w, h := flat.Bounds().Dx(), flat.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return flat
	}
	pad := int(float64(minInt(w, h)) * ocrPadRatio)
	if pad < ocrMinEdgePad {
		pad = ocrMinEdgePad
	}
	if pad > ocrMaxEdgePad {
		pad = ocrMaxEdgePad
	}
	out := image.NewRGBA(image.Rect(0, 0, w+2*pad, h+2*pad))
	draw.Draw(out, image.Rect(pad, pad, pad+w, pad+h), flat, flat.Bounds().Min, draw.Src)

	for x := 0; x < w; x++ {
		top := flat.RGBAAt(x, 0)
		bottom := flat.RGBAAt(x, h-1)
		for y := 0; y < pad; y++ {
			out.SetRGBA(x+pad, y, top)
			out.SetRGBA(x+pad, pad+h+y, bottom)
		}
	}
	for y := 0; y < h+2*pad; y++ {
		sy := y - pad
		if sy < 0 {
			sy = 0
		}
		if sy >= h {
			sy = h - 1
		}
		left := flat.RGBAAt(0, sy)
		right := flat.RGBAAt(w-1, sy)
		for x := 0; x < pad; x++ {
			out.SetRGBA(x, y, left)
			out.SetRGBA(pad+w+x, y, right)
		}
	}
	return out
}

func colorDistanceForOCR(src image.Image) image.Image {
	flat := flattenRGBA(src)
	bg := borderMedianRGB(flat)
	out := image.NewRGBA(flat.Bounds())
	for y := 0; y < flat.Bounds().Dy(); y++ {
		for x := 0; x < flat.Bounds().Dx(); x++ {
			c := flat.RGBAAt(x, y)
			dist := maxInt(absInt(int(c.R)-bg[0]), maxInt(absInt(int(c.G)-bg[1]), absInt(int(c.B)-bg[2])))
			ink := clampByte((dist - 4) * 5)
			v := 255 - ink
			out.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return out
}

func backgroundSubtractForOCR(src image.Image) image.Image {
	flat := flattenRGBA(src)
	w, h := flat.Bounds().Dx(), flat.Bounds().Dy()
	small := image.NewRGBA(image.Rect(0, 0, maxInt(1, w/8), maxInt(1, h/8)))
	xdraw.CatmullRom.Scale(small, small.Bounds(), flat, flat.Bounds(), xdraw.Src, nil)
	smooth := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(smooth, smooth.Bounds(), small, small.Bounds(), xdraw.Src, nil)

	out := image.NewRGBA(flat.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := flat.RGBAAt(x, y)
			b := smooth.RGBAAt(x, y)
			dist := maxInt(absInt(int(c.R)-int(b.R)), maxInt(absInt(int(c.G)-int(b.G)), absInt(int(c.B)-int(b.B))))
			ink := clampByte((dist - 5) * 5)
			v := 255 - ink
			out.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return out
}

type ocrImageStats struct {
	dynamicRange float64
	background   float64
	colorSpread  float64
}

func inspectOCRImage(src image.Image) ocrImageStats {
	flat := flattenRGBA(src)
	var hist [256]int
	n := 0
	spreadSum := 0
	for y := 0; y < flat.Bounds().Dy(); y++ {
		for x := 0; x < flat.Bounds().Dx(); x++ {
			c := flat.RGBAAt(x, y)
			lum := (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000
			hist[lum]++
			spreadSum += maxInt(int(c.R), maxInt(int(c.G), int(c.B))) - minInt(int(c.R), minInt(int(c.G), int(c.B)))
			n++
		}
	}
	if n == 0 {
		return ocrImageStats{}
	}
	bg := borderMedianRGB(flat)
	background := (299*bg[0] + 587*bg[1] + 114*bg[2]) / 1000
	return ocrImageStats{
		dynamicRange: histPercentile(&hist, n, 0.98) - histPercentile(&hist, n, 0.02),
		background:   float64(background),
		colorSpread:  float64(spreadSum) / float64(n),
	}
}

func needsColorRetry(src image.Image) bool {
	s := inspectOCRImage(src)
	return s.dynamicRange < ocrLowContrast || s.background < ocrDarkBackground || s.colorSpread > ocrColorSpread
}

func isSuspiciousOCRText(text string) bool {
	clean := CleanOcrText(text)
	if clean == "" || strings.ContainsAny(clean, "\uFFFD□") {
		return true
	}
	lines := strings.Split(clean, "\n")
	isolated := 0
	short := 0
	for _, line := range lines {
		length := utf8.RuneCountInString(strings.TrimSpace(line))
		if length == 1 {
			isolated++
		}
		if length <= 2 {
			short++
		}
	}
	if len(lines) > 1 && isolated > 0 {
		return true
	}
	if len(lines) >= 4 && short*2 >= len(lines) {
		return true
	}
	fields := strings.Fields(clean)
	if len(fields) == 0 {
		return false
	}
	core, _ := splitTrailingPunct(fields[0])
	if len(core) < 3 || !isASCIIWord(core) {
		return false
	}
	_, ok := spellWords()[strings.ToLower(core)]
	return !ok
}

func scoreOCRText(text string) int {
	clean := CleanOcrText(text)
	score := utf8.RuneCountInString(clean)
	score -= 60 * (strings.Count(clean, "\uFFFD") + strings.Count(clean, "□"))
	lines := strings.Split(clean, "\n")
	if len(lines) > 1 {
		for _, line := range lines {
			if utf8.RuneCountInString(strings.TrimSpace(line)) == 1 {
				score -= 25
			}
		}
	}
	for _, word := range strings.Fields(clean) {
		core, _ := splitTrailingPunct(word)
		if len(core) < 3 || !isASCIIWord(core) {
			continue
		}
		if _, ok := spellWords()[strings.ToLower(core)]; ok {
			score += 8
		}
	}
	return score
}

func borderMedianRGB(src *image.RGBA) [3]int {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	channels := [3][]int{}
	add := func(c color.RGBA) {
		channels[0] = append(channels[0], int(c.R))
		channels[1] = append(channels[1], int(c.G))
		channels[2] = append(channels[2], int(c.B))
	}
	for x := 0; x < w; x++ {
		add(src.RGBAAt(x, 0))
		if h > 1 {
			add(src.RGBAAt(x, h-1))
		}
	}
	for y := 0; y < h; y++ {
		add(src.RGBAAt(0, y))
		if w > 1 {
			add(src.RGBAAt(w-1, y))
		}
	}
	var out [3]int
	for i, values := range channels {
		sort.Ints(values)
		out[i] = values[len(values)/2]
	}
	return out
}

func isASCIIWord(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII || !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}

func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
