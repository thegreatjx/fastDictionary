package main

import (
	"math"
	"math/rand"

	"github.com/egoist/mygo/ui"
)

type markKind int

const (
	markNone markKind = iota
	markUnderline
	markWavy
	markDouble
	markDotted
	markDashed
	markHighlight
)

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

type jitter struct {
	r *rand.Rand
}

func newJitter(seed int64) *jitter {
	return &jitter{r: rand.New(rand.NewSource(seed))}
}

func (j *jitter) next() float32 {
	return j.r.Float32()
}

func (j *jitter) amp(scale float32) float32 {
	if scale == 0 {
		return 0
	}
	return (j.r.Float32()*2 - 1) * scale
}

func markSeed(salt, span int) int64 {
	return int64(salt*7919 + span*104729 + 1)
}

func markHeight(kind markKind) float32 {
	switch kind {
	case markWavy, markDouble:
		return 6.5
	case markDotted, markDashed, markUnderline:
		return 5.0
	case markHighlight:
		return 14.0
	default:
		return 0
	}
}

func drawMark(p *ui.Painter, r ui.Rect, kind markKind, c ui.Color, seed int64) {
	if kind == markNone || r.W <= 1 || r.H <= 1 || c.A == 0 {
		return
	}
	box := markBoxes[kind]

	switch kind {
	case markUnderline:
		j := newJitter(markSeed(11, int(seed)))
		strokeSmooth(p, r, box, underlinePoints(j, 5, 1.8, 0), c, 2.1)
	case markDouble:
		j1 := newJitter(markSeed(37, int(seed)))
		j2 := newJitter(markSeed(38, int(seed)))
		strokeSmooth(p, r, box, underlinePoints(j1, 5.5, 1.1, 1), c, 2.0)
		strokeSmooth(p, r, box, underlinePoints(j2, 11.5, 1.1, 3.5), withAlpha(c, 0.78), 1.45)
	case markWavy:
		strokeTiledWave(p, r, c, markSeed(23, int(seed)))
	case markDotted:
		j := newJitter(markSeed(41, int(seed)))
		strokeDottedSmooth(p, r, box, smoothPoints(j, 2, box.w, 5, 0.8), c, 1.3)
	case markDashed:
		j := newJitter(markSeed(53, int(seed)))
		strokeDashedSmooth(p, r, box, smoothPoints(j, 2, box.w, 5, 0.85), c, 1.8, []float32{6.5, 3.5, 4.2, 3.5})
	case markHighlight:
		j := newJitter(markSeed(67, int(seed)))
		fillSmooth(p, r, box, highlightPoints(j), c)
	}
}

func underlinePoints(j *jitter, offset, bow, inset float32) [][2]float32 {
	lo := 3 + inset
	hi := 137 - inset
	steps := 4
	dir := float32(1)
	if j.next() < 0.5 {
		dir = -1
	}
	arch := dir * bow * (0.6 + 0.4*j.next())
	pts := make([][2]float32, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		x := lo + (hi-lo)*t
		if i > 0 && i < steps {
			x += j.amp((hi - lo) / float32(steps) * 0.18)
		}
		y := offset + arch*float32(math.Sin(math.Pi*float64(t))) + j.amp(0.45)
		pts = append(pts, [2]float32{x, y})
	}
	pts[0][1] += float32(math.Abs(float64(j.amp(0.65))))
	pts[steps][1] -= float32(math.Abs(float64(j.amp(0.45))))
	return pts
}

func smoothPoints(j *jitter, lo, hi, offset, noise float32) [][2]float32 {
	steps := 5
	pts := make([][2]float32, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		x := lo + (hi-lo)*t
		y := offset
		if i > 0 && i < steps {
			y += j.amp(noise)
		}
		pts = append(pts, [2]float32{x, y})
	}
	return pts
}

func strokeTiledWave(p *ui.Painter, r ui.Rect, c ui.Color, seed int64) {
	j := newJitter(seed)
	midY := r.Y + r.H*0.48
	amp := min(max((r.H-1)*0.36, 1.65), 2.35)
	halfPeriod := float32(4.6)

	var base [6][2]float32
	for k := 0; k < 6; k++ {
		sign := float32(-1)
		if k%2 == 1 {
			sign = 1
		}
		base[k] = [2]float32{
			float32(k)*halfPeriod + j.amp(0.45),
			sign * (amp + j.amp(0.28)),
		}
	}
	cycleW := 6 * halfPeriod
	pointAt := func(idx int) [2]float32 {
		m := ((idx % 6) + 6) % 6
		cycles := float32(math.Floor(float64(idx) / 6.0))
		pt := base[m]
		x := r.X + pt[0] + cycles*cycleW
		y := midY + pt[1]
		return [2]float32{x, y}
	}

	maxSteps := int(math.Ceil(float64(r.W/halfPeriod))) + 2
	if maxSteps < 3 {
		maxSteps = 3
	}
	clampX := func(pt [2]float32) [2]float32 {
		if pt[0] < r.X {
			pt[0] = r.X
		}
		if pt[0] > r.X+r.W {
			pt[0] = r.X + r.W
		}
		return pt
	}

	path := &ui.Path{}
	pPrev := clampX(pointAt(0))
	p0 := clampX(pointAt(1))
	startMid := [2]float32{(pPrev[0] + p0[0]) / 2, (pPrev[1] + p0[1]) / 2}
	path.MoveTo(r.X, startMid[1])
	for idx := 0; idx < maxSteps; idx++ {
		ctrl := clampX(pointAt(idx))
		next := clampX(pointAt(idx + 1))
		mid := [2]float32{(ctrl[0] + next[0]) / 2, (ctrl[1] + next[1]) / 2}
		path.QuadTo(ctrl[0], ctrl[1], mid[0], mid[1])
		if mid[0] >= r.X+r.W-0.5 {
			break
		}
	}
	p.StrokePath(path, 1.85, c)
}

func highlightPoints(j *jitter) [][2]float32 {
	const lo, hi = 2.0, 338.0
	const top, bot = 4.0, 28.0
	pts := make([][2]float32, 0, 14)
	steps := 6
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		x := lo + (hi-lo)*t
		y := top - 1.8*float32(math.Sin(math.Pi*float64(t)))
		if i > 0 && i < steps {
			y += j.amp(0.9)
		}
		pts = append(pts, [2]float32{x, y})
	}
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		x := hi - (hi-lo)*t
		y := bot + 1.6*float32(math.Sin(math.Pi*float64(t)))
		if i > 0 && i < steps {
			y += j.amp(0.9)
		}
		pts = append(pts, [2]float32{x, y})
	}
	return pts
}

func scalePoints(pts [][2]float32, box markBox, r ui.Rect) [][2]float32 {
	sx := r.W / box.w
	sy := float32(0.42)
	if box.h >= 30 {
		sy = r.H / box.h
	}
	out := make([][2]float32, len(pts))
	for i, pt := range pts {
		out[i] = [2]float32{r.X + pt[0]*sx, r.Y + pt[1]*sy}
	}
	return out
}

func strokeSmooth(p *ui.Painter, r ui.Rect, box markBox, pts [][2]float32, c ui.Color, width float32) {
	if len(pts) < 2 {
		return
	}
	pts = scalePoints(pts, box, r)
	strokeQuadChain(p, pts, false, width, c)
}

func strokeQuadChain(p *ui.Painter, pts [][2]float32, closed bool, width float32, c ui.Color) {
	if len(pts) < 2 {
		return
	}
	path := &ui.Path{}
	path.MoveTo(pts[0][0], pts[0][1])
	lastIdx := len(pts) - 1
	for i := 1; i < lastIdx; i++ {
		next := pts[i+1]
		mid := [2]float32{(pts[i][0] + next[0]) / 2, (pts[i][1] + next[1]) / 2}
		path.QuadTo(pts[i][0], pts[i][1], mid[0], mid[1])
	}
	if closed {
		path.QuadTo(pts[lastIdx][0], pts[lastIdx][1], pts[0][0], pts[0][1])
		path.Close()
	} else {
		path.LineTo(pts[lastIdx][0], pts[lastIdx][1])
	}
	p.StrokePath(path, width, c)
}

func strokeDottedSmooth(p *ui.Painter, r ui.Rect, box markBox, pts [][2]float32, c ui.Color, dotRadius float32) {
	if len(pts) < 2 {
		return
	}
	pts = scalePoints(pts, box, r)
	total := float32(0)
	for i := 1; i < len(pts); i++ {
		total += dist(pts[i-1], pts[i])
	}
	if total <= 0 {
		return
	}
	step := float32(4.6)
	d := dotRadius * 2
	for pos := dotRadius; pos <= total; pos += step {
		pt := pointAtArcLength(pts, pos)
		p.Fill(ui.Rect{X: pt[0] - dotRadius, Y: pt[1] - dotRadius, W: d, H: d}, c, dotRadius)
	}
}

func pointAtArcLength(pts [][2]float32, target float32) [2]float32 {
	acc := float32(0)
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		d := dist(a, b)
		if d <= 0 {
			continue
		}
		if acc+d >= target {
			return lerp(a, b, clamp01((target-acc)/d))
		}
		acc += d
	}
	return pts[len(pts)-1]
}

func strokeDashedSmooth(p *ui.Painter, r ui.Rect, box markBox, pts [][2]float32, c ui.Color, width float32, pattern []float32) {
	if len(pts) < 2 {
		return
	}
	pts = scalePoints(pts, box, r)
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

func strokeSubPolyline(p *ui.Painter, pts [][2]float32, from, to float32, c ui.Color, width float32) {
	run := make([][2]float32, 0, 6)
	acc := float32(0)
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		d := dist(a, b)
		if d == 0 {
			continue
		}
		segStart, segEnd := acc, acc+d
		if segEnd >= from && segStart <= to {
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

func fillSmooth(p *ui.Painter, r ui.Rect, box markBox, pts [][2]float32, c ui.Color) {
	if len(pts) < 4 {
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
	path.Close()
	p.FillPath(path, c)
}

// drawConnectorOval ports Read-Frog's _e() double-loop hand-drawn oval around
// clause connectors (which, that, because, while, if, etc.).
func drawConnectorOval(p *ui.Painter, r ui.Rect, c ui.Color, seed int64) {
	if r.W <= 2 || r.H <= 2 || c.A == 0 {
		return
	}
	j := newJitter(markSeed(79, int(seed)))
	buildLoop := func(rx, ry, startDeg, sweepDeg float32, steps int) [][2]float32 {
		pts := make([][2]float32, 0, steps+1)
		for s := 0; s <= steps; s++ {
			cur := sweepDeg * float32(s) / float32(steps)
			rad := float64((startDeg + cur) * math.Pi / 180.0)
			spiral := float32(1.0)
			if sweepDeg > 360 && cur > 360 {
				spiral = 1.0 - 0.06*(cur-360)/max(1, sweepDeg-360)
			}
			d := (1.0 + j.amp(0.035)) * spiral
			vx := 50 + rx*d*float32(math.Cos(rad))
			vy := 21.5 + ry*d*float32(math.Sin(rad)) + j.amp(0.55)
			pts = append(pts, [2]float32{
				r.X + (vx/100.0)*r.W,
				r.Y + (vy/40.0)*r.H,
			})
		}
		return pts
	}
	outer := buildLoop(47, 18.5, 150, 382, 10)
	inner := buildLoop(44, 16.8, 135, 335, 8)
	strokeQuadChain(p, outer, false, 1.5, c)
	strokeQuadChain(p, inner, false, 1.0, withAlpha(c, 0.52))
}

// drawCurvedBracket ports Read-Frog's ge('left'/'right') hand-drawn bowed bracket.
func drawCurvedBracket(p *ui.Painter, r ui.Rect, isLeft bool, c ui.Color, seed int64) {
	if r.W <= 1 || r.H <= 2 || c.A == 0 {
		return
	}
	salt := 71
	if !isLeft {
		salt = 73
	}
	j := newJitter(markSeed(salt, int(seed)))
	const steps = 6
	pts := make([][2]float32, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		bow := 5.2*float32(math.Pow(math.Sin(math.Pi*float64(t)), 0.85)) + j.amp(0.32)
		vx := 2.4 + bow
		if isLeft {
			vx = 9.6 - bow
		}
		vy := 2.0 + 36.0*t
		if i > 0 && i < steps {
			vy += j.amp(0.7)
		}
		pts = append(pts, [2]float32{
			r.X + (vx/12.0)*r.W,
			r.Y + (vy/40.0)*r.H,
		})
	}
	strokeQuadChain(p, pts, false, 1.95, c)
}

// drawHandStar ports Read-Frog's ye() hand-drawn 5-point obstacle star.
func drawHandStar(p *ui.Painter, r ui.Rect, c ui.Color, seed int64) {
	if r.W <= 2 || r.H <= 2 || c.A == 0 {
		return
	}
	j := newJitter(markSeed(83, int(seed)))
	var verts [6][2]float32
	for n := 0; n <= 5; n++ {
		rad := float64((-90.0 + 144.0*float32(n) + j.amp(3.8)) * math.Pi / 180.0)
		radius := 8.3 + j.amp(0.55)
		vx := 10.0 + radius*float32(math.Cos(rad))
		vy := 10.6 + radius*float32(math.Sin(rad))
		verts[n] = [2]float32{
			r.X + (vx/20.0)*r.W,
			r.Y + (vy/20.0)*r.H,
		}
	}
	path := &ui.Path{}
	path.MoveTo(verts[0][0], verts[0][1])
	prev := verts[0]
	for n := 1; n <= 5; n++ {
		cur := verts[n]
		mx, my := (prev[0]+cur[0])/2, (prev[1]+cur[1])/2
		dx, dy := cur[0]-prev[0], cur[1]-prev[1]
		length := float32(math.Hypot(float64(dx), float64(dy)))
		if length < 1 {
			length = 1
		}
		bend := j.amp(0.55)
		path.QuadTo(mx-dy/length*bend, my+dx/length*bend, cur[0], cur[1])
		prev = cur
	}
	p.StrokePath(path, 1.45, c)
}

// drawHandSlash ports Read-Frog's xe(isDouble) hand-drawn single/double sense-group slash.
func drawHandSlash(p *ui.Painter, r ui.Rect, isDouble bool, c ui.Color, seed int64) {
	if r.W <= 1 || r.H <= 2 || c.A == 0 {
		return
	}
	flag := 0
	if isDouble {
		flag = 1
	}
	j := newJitter(markSeed(89, int(seed)+flag))
	viewW := float32(8.0)
	xs := []float32{4.0}
	if isDouble {
		viewW = 14.0
		xs = []float32{4.2, 9.8}
	}
	for _, xCenter := range xs {
		x0 := r.X + ((xCenter+2.5+j.amp(0.28))/viewW)*r.W
		y0 := r.Y + (1.6/20.0)*r.H
		x1 := r.X + ((xCenter-2.1+j.amp(0.28))/viewW)*r.W
		y1 := r.Y + (18.4/20.0)*r.H
		mx := (x0+x1)/2 - 0.35 + j.amp(0.32)
		my := (y0 + y1) / 2
		path := &ui.Path{}
		path.MoveTo(x0, y0)
		path.QuadTo(mx, my, x1, y1)
		p.StrokePath(path, 1.55, c)
	}
}

// drawModifierArrow ports Read-Frog's [data-tip-arrows] cubic Bézier curved
// arrow pointing from a clause/phrase badge to its modified head noun.
func drawModifierArrow(p *ui.Painter, fromBadge, toWord ui.Rect, minTopY float32, c ui.Color, width float32) {
	if fromBadge.W <= 0 || toWord.W <= 0 || c.A == 0 {
		return
	}
	x0 := fromBadge.X + min(10, fromBadge.W*0.5)
	y0 := fromBadge.Y + 1
	x3 := toWord.X + toWord.W*0.5
	y3 := toWord.Y + 1

	dx := float32(math.Abs(float64(x3 - x0)))
	dy := float32(math.Abs(float64(y3 - y0)))
	if dx < 4 && dy < 4 {
		return
	}
	arch := min(22, max(10, dx*0.11))
	peakY := max(minTopY, min(y0, y3)-arch)

	c1x, c1y := x0+1.5, peakY
	c2x, c2y := x3-2.0, peakY-1.0

	curve := &ui.Path{}
	curve.MoveTo(x0, y0)
	curve.CubeTo(c1x, c1y, c2x, c2y, x3, y3)
	p.StrokePath(curve, width, c)

	// Compute tangent direction near t=1 using cubic Bézier sampling, matching Read-Frog's xe(d).
	nx, ny := c2x, c2y
	for t := float32(0.96); t > 0.1; t -= 0.03 {
		bx, by := cubicBezierPoint(x0, y0, c1x, c1y, c2x, c2y, x3, y3, t)
		if float32(math.Hypot(float64(bx-x3), float64(by-y3))) >= 6.5 {
			nx, ny = bx, by
			break
		}
	}
	distEnd := float32(math.Hypot(float64(nx-x3), float64(ny-y3)))
	ux, uy := float32(0), float32(-1)
	if distEnd > 0.01 {
		ux = (nx - x3) / distEnd
		uy = (ny - y3) / distEnd
	}
	wing := func(angle, length float32) (float32, float32) {
		ca := float32(math.Cos(float64(angle)))
		sa := float32(math.Sin(float64(angle)))
		return x3 + length*(ux*ca-uy*sa), y3 + length*(ux*sa+uy*ca)
	}
	w1x, w1y := wing(0.50, 6.4)
	w2x, w2y := wing(-0.47, 6.8)
	head := &ui.Path{}
	head.MoveTo(w1x, w1y)
	head.LineTo(x3, y3)
	head.LineTo(w2x, w2y)
	p.StrokePath(head, width, c)
}

func cubicBezierPoint(x0, y0, x1, y1, x2, y2, x3, y3, t float32) (float32, float32) {
	u := 1 - t
	b0 := u * u * u
	b1 := 3 * u * u * t
	b2 := 3 * u * t * t
	b3 := t * t * t
	return b0*x0 + b1*x1 + b2*x2 + b3*x3, b0*y0 + b1*y1 + b2*y2 + b3*y3
}

func dist(a, b [2]float32) float32 {
	return float32(math.Hypot(float64(b[0]-a[0]), float64(b[1]-a[1])))
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
