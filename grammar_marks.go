package main

import (
	"math"
	"math/rand"

	"github.com/egoist/mygo/ui"
)

// Hand-drawn grammar marks.
//
// This is a port of the underline generator in read-frog's layout engine
// (@read-frog/layout-engine@0.3.0, dist/presets.js: the functions `F`, `P`,
// `z`, `fe`, `B`, `pe`). Its marks are not CSS borders: each is a polyline
// jittered by a seeded RNG and smoothed into quadratic Bézier segments, which
// is what makes them look drawn by hand rather than computed. mygo has no SVG
// and no dashed/double border, so the same geometry is built here and stroked
// through Painter.StrokePath / StrokePolyline.
//
// The seed is derived from the word's position, exactly as the original does
// (R = i*7919 + j*104729 + 1), so a given word keeps the same wobble across
// repaints instead of shimmering as the window redraws.

// markKind is one of the six line styles the grammar card uses.
type markKind int

const (
	markUnderline markKind = iota
	markWavy
	markDouble
	markDotted
	markDashed
	markHighlight
)

// markBox is the drawing box for each kind, in the engine's own units. The
// aspect ratio differs per kind: the wavy and dotted marks are stretched
// horizontally (preserveAspectRatio slice), the rest are not.
type markBox struct {
	w, h float32
}

var markBoxes = map[markKind]markBox{
	markUnderline: {140, 10},
	markWavy:      {800, 10},
	markDouble:    {140, 14},
	markDotted:    {800, 10},
	markDashed:    {800, 10},
	markHighlight: {340, 32},
}

// jitter returns a small pseudo-random offset in [-scale, scale], seeded so the
// same (seed, call index) always gives the same value.
type jitter struct {
	r *rand.Rand
}

func newJitter(seed int64) *jitter {
	return &jitter{r: rand.New(rand.NewSource(seed))}
}

// amp returns a signed offset of magnitude up to scale, biased away from zero
// the way the original's A() does: (rand*2-1)*scale.
func (j *jitter) amp(scale float32) float32 {
	if scale == 0 {
		return 0
	}
	return (j.r.Float32()*2 - 1) * scale
}

// drawMark strokes a hand-drawn mark across the given box. It is meant to be
// called from Element.Draw, so the box is the element's own rectangle.
func drawMark(p *ui.Painter, r ui.Rect, kind markKind, c ui.Color, seed int64) {
	if r.W <= 1 || r.H <= 1 {
		return
	}
	j := newJitter(seed)
	box := markBoxes[kind]

	switch kind {
	case markUnderline:
		// o:5 is the engine's own default: the stroke sits in the middle of
		// the 10-unit box. Passing 0 put the curve above the box, where the
		// painter clipped it and the underline looked half drawn.
		strokeSmooth(p, r, box, underlinePoints(j, 5, 1.9, 0), c, 2.6)
	case markDouble:
		// Two strokes, 5 engine units apart: the engine's own geometry for a
		// double underline. The second is thinner and lighter, as in the
		// original, so the pair reads as two lines rather than one heavy one.
		strokeSmooth(p, r, box, underlinePoints(j, 7, 1.9, 1), c, 2.4)
		strokeSmooth(p, r, box, underlinePoints(j, 12, 1.9, 3), withAlpha(c, 0.9), 1.9)
	case markWavy:
		// The wave is one period in engine units, repeated across the word.
		// Its viewBox is 800 wide but a single cycle is 18 units, so it has to
		// be tiled rather than stretched: stretching a 800-wide box onto a
		// 40 DIP word flattens the wave into a straight line.
		strokeTiledWave(p, r, c, seed)
	case markDotted:
		strokeDashedSmooth(p, r, box, smoothPoints(j, 2, 10, 5, 0.9), c, 3.2, []float32{0, 5})
	case markDashed:
		strokeDashedSmooth(p, r, box, smoothPoints(j, 2, 10, 5, 0.9), c, 2.4, []float32{10, 5, 6, 5})
	case markHighlight:
		fillSmooth(p, r, box, highlightPoints(j), c)
	}
}

// markHeight is how tall a mark's drawing box is, in DIPs.
//
// These follow the engine's own boxes at the 0.42 DIP unit scale: an underline
// is 10 units (4.2), a double is 14 (5.9) because its two strokes are 5 units
// apart, and the highlight is 32 (13.4). Giving the double the same height as
// an underline is what made its two strokes overlap.
func markHeight(kind markKind) float32 {
	switch kind {
	case markWavy:
		return 7
	case markDouble:
		return 7
	case markDotted:
		return 5
	case markDashed:
		return 5
	case markHighlight:
		return 13
	default:
		return 5
	}
}

// strokeTiledWave repeats one period of the sine wave across the box.
//
// The wave is what marks a predicate, so it has to be visible as a wave at
// 14px text: a shallow ripple reads as a smudge or a printing fault. The
// amplitude and period are set from the mark height rather than the box width,
// so a one-letter verb and a five-word verb get the same wave.
func strokeTiledWave(p *ui.Painter, r ui.Rect, c ui.Color, seed int64) {
	j := newJitter(seed)
	midY := r.Y + r.H/2
	amp := (r.H - 1) / 2
	period := r.H * 2.2
	if period < 9 {
		period = 9
	}

	pts := make([][2]float32, 0, 96)
	// Sample densely enough that the sine reads as a curve rather than a
	// zigzag: eight points per period.
	step := period / 8
	for x := r.X; x <= r.X+r.W; x += step {
		t := (x - r.X) / period
		y := midY + amp*float32(math.Sin(2*math.Pi*float64(t)))
		pts = append(pts, [2]float32{x, y + j.amp(0.25)})
	}
	if len(pts) >= 2 {
		p.StrokePolyline(pts, 2.7, c)
	}
}

// underlinePoints builds the gentle arc a solid underline follows: a long
// shallow curve with a little noise, thicker in the middle.
func underlinePoints(j *jitter, offset, amp, tail float32) [][2]float32 {
	const lo, hi = 3.0, 137.0
	const steps = 4
	dir := float32(1)
	if j.r.Float32() < 0.5 {
		dir = -1
	}
	sway := dir * amp * (0.6 + 0.4*j.r.Float32())

	pts := make([][2]float32, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float32(i) / steps
		x := lo + (hi-lo)*t
		mid := i > 0 && i < steps
		if mid {
			x += j.amp((hi - lo) / steps * 0.2)
		}
		y := offset + sway*float32(math.Sin(math.Pi*float64(t)))
		if mid {
			y += j.amp(0.5)
		}
		pts = append(pts, [2]float32{x, y})
	}
	if len(pts) > 0 {
		pts[0][1] += float32(math.Abs(float64(j.amp(0.75))))
		pts[len(pts)-1][1] -= float32(math.Abs(float64(j.amp(0.5))))
	}
	_ = tail
	return pts
}

// wavyPoints builds one period of a sine wave, sampled so that it can be
// repeated by the caller if the word is wide.
func wavyPoints(j *jitter) [][2]float32 {
	const period = 18.0
	pts := make([][2]float32, 0, 7)
	for i := 0; i <= 6; i++ {
		t := float32(i) / 6
		x := float32(period) * t
		y := 5 + 4.4*float32(math.Sin(2*math.Pi*float64(t)))
		y += j.amp(0.6)
		pts = append(pts, [2]float32{x, y})
	}
	return pts
}

// smoothPoints is the generic jittered horizontal line used by the dotted and
// dashed marks.
func smoothPoints(j *jitter, lo, hi, amp, noise float32) [][2]float32 {
	const steps = 5
	pts := make([][2]float32, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float32(i) / steps
		x := lo + (hi-lo)*t
		y := amp * float32(math.Sin(math.Pi*float64(t)))
		if i > 0 && i < steps {
			y += j.amp(noise)
		}
		pts = append(pts, [2]float32{x, y})
	}
	return pts
}

// highlightPoints traces the marker-pen outline: a wobbly rounded rectangle
// with the two horizontal runs bowed outward.
func highlightPoints(j *jitter) [][2]float32 {
	const lo, hi = 2.0, 27.0
	const top, bot = 4.0, 28.0
	pts := make([][2]float32, 0, 12)
	steps := 6
	for i := 1; i <= steps; i++ {
		t := float32(i) / float32(steps)
		x := lo + (hi-lo)*t
		y := top + 2*float32(math.Sin(math.Pi*float64(t)))
		if i < steps {
			y += j.amp(1)
		}
		pts = append(pts, [2]float32{x, y})
	}
	for i := 1; i <= steps; i++ {
		t := float32(i) / float32(steps)
		x := hi - (hi-lo)*t
		y := bot - 2*float32(math.Sin(math.Pi*float64(t)))
		if i < steps {
			y += j.amp(1)
		}
		pts = append(pts, [2]float32{x, y})
	}
	return pts
}

// scalePoints maps the engine's own coordinates onto the element's box.
//
// Only X is stretched to the word's width. Y keeps a fixed scale taken from the
// engine's own units, because the mark's shape is vertical: a 14-unit double
// underline squeezed into a 7 DIP box collapses its two strokes onto each
// other and reads as one thick line. The engine gets away with stretching both
// axes because it hands the result to the browser as a background image with
// its own preserveAspectRatio; here the geometry is drawn directly, so the
// vertical proportions have to be preserved by hand.
func scalePoints(pts [][2]float32, box markBox, r ui.Rect) [][2]float32 {
	sx := r.W / box.w
	// One engine unit is 0.42 DIP, which puts a 10-unit underline at ~4.2 DIP
	// tall and a 14-unit double at ~5.9 — the proportions read-frog's card
	// shows at 14px text.
	sy := float32(0.42)
	out := make([][2]float32, len(pts))
	for i, p := range pts {
		out[i] = [2]float32{r.X + p[0]*sx, r.Y + p[1]*sy}
	}
	return out
}

// strokeSmooth strokes a polyline as quadratic Bézier segments, which is what
// turns the sampled points into a drawn-looking curve.
func strokeSmooth(p *ui.Painter, r ui.Rect, box markBox, pts [][2]float32, c ui.Color, width float32) {
	if len(pts) < 2 {
		return
	}
	pts = scalePoints(pts, box, r)
	path := &ui.Path{}
	path.MoveTo(pts[0][0], pts[0][1])
	for i := 1; i < len(pts)-1; i++ {
		next := pts[i+1]
		mid := [2]float32{(pts[i][0] + next[0]) / 2, (pts[i][1] + next[1]) / 2}
		path.QuadTo(pts[i][0], pts[i][1], mid[0], mid[1])
	}
	last := pts[len(pts)-1]
	path.LineTo(last[0], last[1])
	p.StrokePath(path, width, c)
}

// strokeDashedSmooth strokes a polyline with a dash pattern. mygo has no dashed
// path stroke, so the pattern is applied by walking the polyline and stroking
// each visible run separately.
func strokeDashedSmooth(p *ui.Painter, r ui.Rect, box markBox, pts [][2]float32, c ui.Color, width float32, pattern []float32) {
	if len(pts) < 2 {
		return
	}
	pts = scalePoints(pts, box, r)
	// Total length, so the pattern can be walked in absolute terms.
	total := float32(0)
	for i := 1; i < len(pts); i++ {
		total += dist(pts[i-1], pts[i])
	}
	if total == 0 {
		return
	}

	pos := float32(0)
	idx := 0
	on := true
	for pos < total {
		seg := pattern[idx%len(pattern)]
		idx++
		if seg <= 0 {
			on = !on
			continue
		}
		if on {
			strokeSubPolyline(p, pts, pos, pos+seg, c, width)
		}
		pos += seg
		on = !on
	}
}

// strokeSubPolyline strokes the part of pts between arc-lengths from and to.
func strokeSubPolyline(p *ui.Painter, pts [][2]float32, from, to float32, c ui.Color, width float32) {
	run := make([][2]float32, 0, 8)
	acc := float32(0)
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		d := dist(a, b)
		if d == 0 {
			continue
		}
		segStart, segEnd := acc, acc+d
		if segEnd >= from && segStart <= to {
			// Clip this segment to the window.
			t0 := clamp01((from - segStart) / d)
			t1 := clamp01((to - segStart) / d)
			run = append(run, lerp(a, b, t0))
			if t1 > t0 {
				run = append(run, lerp(a, b, t1))
			}
		}
		acc = segEnd
		if acc > to {
			break
		}
	}
	if len(run) >= 2 {
		p.StrokePolyline(run, width, c)
	}
}

// fillSmooth fills a closed polyline, for the marker-pen highlight.
func fillSmooth(p *ui.Painter, r ui.Rect, box markBox, pts [][2]float32, c ui.Color) {
	if len(pts) < 3 {
		return
	}
	pts = scalePoints(pts, box, r)
	path := &ui.Path{}
	path.MoveTo(pts[0][0], pts[0][1])
	for i := 1; i < len(pts); i++ {
		path.LineTo(pts[i][0], pts[i][1])
	}
	path.Close()
	p.FillPath(path, c)
}

func dist(a, b [2]float32) float32 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	return float32(math.Hypot(float64(dx), float64(dy)))
}

func lerp(a, b [2]float32, t float32) [2]float32 {
	return [2]float32{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t}
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// withAlpha returns c at the given opacity.
func withAlpha(c ui.Color, a float32) ui.Color {
	if a < 0 {
		a = 0
	}
	if a > 1 {
		a = 1
	}
	c.A = uint8(float32(c.A) * a)
	return c
}
