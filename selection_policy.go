package main

import (
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	minSelectionDrag = 14

	minSelectionDragDuration = 45 * time.Millisecond

	shortSelectionRunes = 1

	shortSelectionNeedsWordRunes = 3

	gestureCooldown = 300 * time.Millisecond

	sameSpotCooldown = time.Second
	sameSpotRadius   = 8

	typingGrace = 350 * time.Millisecond
)

type selectionGesture struct {
	X, Y      int32
	Owner     string
	Kind      gestureKind
	PressedAt time.Time
	LastKey   time.Time
}

func typingSuppresses(g selectionGesture) bool {
	if g.Kind != gestureDrag {
		return false
	}
	if g.LastKey.IsZero() {
		return false
	}
	if g.PressedAt.IsZero() {
		return false
	}
	since := g.PressedAt.Sub(g.LastKey)
	if since < 0 {
		return false
	}
	return since < typingGrace
}

type gestureKind int

const (
	gestureDrag gestureKind = iota
	gestureDoubleClick
	gestureKeyboard
	gestureTripleClick
)

func (k gestureKind) String() string {
	switch k {
	case gestureDoubleClick:
		return "double-click"
	case gestureTripleClick:
		return "triple-click"
	case gestureKeyboard:
		return "keyboard"
	default:
		return "drag"
	}
}

func dragIsSelection(start, end winPoint, held time.Duration) (bool, string) {
	dx := int(end.X - start.X)
	dy := int(end.Y - start.Y)
	if dx*dx+dy*dy < minSelectionDrag*minSelectionDrag {
		return false, "drag too short"
	}
	if held > 0 && held < minSelectionDragDuration {
		return false, "drag too fast"
	}
	return true, ""
}

func selectionTextProblem(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "empty"
	}
	runes := []rune(trimmed)
	switch {
	case len(runes) <= shortSelectionRunes:
		return "one character"
	case len(runes) <= shortSelectionNeedsWordRunes && !isWordLike(runes):
		return "fragment"
	}
	return ""
}

func isWordLike(runes []rune) bool {
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || isCJK(r) {
			continue
		}
		return false
	}
	return true
}

type gestureGate struct {
	mu    sync.Mutex
	fired bool
	at    time.Time
	x, y  int32
}

func (g *gestureGate) allow(now time.Time, x, y int32) (bool, string) {
	ok, why := g.canAllow(now, x, y)
	if !ok {
		return false, why
	}
	g.record(now, x, y)
	return true, ""
}

func (g *gestureGate) canAllow(now time.Time, x, y int32) (bool, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fired {
		if since := now.Sub(g.at); since >= 0 && since < gestureCooldown {
			return false, "cooldown"
		}
		dx := int(x - g.x)
		dy := int(y - g.y)
		if dx*dx+dy*dy <= sameSpotRadius*sameSpotRadius && now.Sub(g.at) < sameSpotCooldown {
			return false, "same spot"
		}
	}
	return true, ""
}

func (g *gestureGate) record(now time.Time, x, y int32) {
	g.mu.Lock()
	g.fired = true
	g.at = now
	g.x, g.y = x, y
	g.mu.Unlock()
}

func (g *gestureGate) reset() {
	g.mu.Lock()
	g.fired = false
	g.mu.Unlock()
}
