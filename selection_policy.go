// The policy of the automatic selection trigger: "does this gesture deserve
// a chip?".
//
// It lives here, as pure functions and named constants, because it used to be
// scattered across the hook callback and the delivery path, where no test
// could reach it and every threshold was a magic number. Two failure modes
// came out of that:
//
//   - the trigger fired for gestures that are not selections at all (a
//     scrollbar drag, a window edge, a stray 1-character read), so the chip
//     "appeared by itself";
//   - the same spot could produce the same chip twice, because nothing
//     remembered the gesture that had just been accepted.
//
// The thresholds are deliberately conservative: a missed chip costs the user
// one more drag, a chip nobody asked for interrupts whatever they were doing.
package main

import (
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	// minSelectionDrag is how far the pointer must travel between press and
	// release for the gesture to count as a drag at all. It used to be 10px,
	// which is inside the range of a scrollbar thumb, a window title bar and
	// a window edge, so dragging any of those raised the chip. 14 keeps that
	// protection and still catches a short word: at 125–150% scaling a three
	// letter word is only 12–16px wide, and 20 dropped those drags.
	minSelectionDrag = 14

	// minSelectionDragDuration rejects a drag that is over almost as soon as
	// it starts. Flicking a scrollbar and grabbing-then-releasing a window
	// edge produce exactly the pointer travel of a selection, but they are
	// done in a fraction of the time. 45ms is the floor now: double-clicking
	// a word and dragging a word are both fast, and 80ms dropped the quick
	// ones without keeping anything the 45ms floor loses.
	minSelectionDragDuration = 45 * time.Millisecond

	// shortSelectionRunes is the length at or below which a read is treated
	// as an accident: a stray click that left the caret on one character, or
	// a provider that answered with the character under the pointer rather
	// than with a selection.
	shortSelectionRunes = 1

	// shortSelectionNeedsWordRunes is the length at or below which the text
	// must be a plausible word: letters, digits or CJK, with no spaces or
	// punctuation. A 2-3 character read that is not a word is the tail of a
	// longer selection the provider got wrong, and translating "omp" or "aud"
	// helps nobody.
	shortSelectionNeedsWordRunes = 3

	// gestureCooldown is the minimum time between two accepted gestures,
	// whatever their position. It absorbs the double report a single drag can
	// produce when the application also selects on its own.
	gestureCooldown = 300 * time.Millisecond

	// sameSpotCooldown and sameSpotRadius drop a repeat of the gesture that
	// just fired. Selecting the same few pixels twice within a second is a
	// re-drag or a stutter, not a new question.
	sameSpotCooldown = time.Second
	sameSpotRadius   = 8

	// typingGrace is how long after a keystroke a *drag* stops counting as a
	// selection.
	//
	// This is the rule the user asked for by name: "the chip jumps out while
	// I am typing in an input box". Inside a text field a drag is the user
	// placing the caret, or sweeping a few characters to rewrite them; it is
	// not a request to translate. Nothing in the geometry can tell those two
	// apart — a caret drag and a selection drag are the same shape — but the
	// keyboard can: a drag that begins while the user is still typing is
	// always the former.
	//
	// The window is measured from the last keystroke to the moment the button
	// went down, not to the moment it came up: a drag that starts while the
	// user is typing and lasts half a second is still a caret drag.
	//
	// A double click is deliberately exempt. Selecting a word with a double
	// click is how a dictionary is normally used, and typing a word and then
	// double-clicking it is the most common sequence there is; a guard that
	// swallowed it would remove the feature in order to fix it.
	typingGrace = 350 * time.Millisecond
)

// selectionGesture is what the hooks hand to the trigger. It is a struct
// rather than an argument list because the pieces are gathered at different
// moments inside the hook callback: the position and the owner at the end of
// the drag, the keystroke clocks at its beginning.
type selectionGesture struct {
	// X, Y is where the gesture ended, in physical screen pixels.
	X, Y int32
	// Owner is the executable name of the application the gesture happened
	// in, which is what the screenshot-tool guard needs.
	Owner string
	// Kind says how the gesture was made; see gestureKind.
	Kind gestureKind
	// PressedAt is when the gesture began: the button going down, or the
	// last key of a keyboard selection coming up.
	PressedAt time.Time
	// LastKey is when a key was last pressed before PressedAt. The zero
	// value means "no keystroke seen since start".
	LastKey time.Time
}

// typingSuppresses reports whether a gesture happened while the user was
// still typing, and so is a caret drag rather than a selection.
//
// It judges the moment the gesture began (PressedAt), not the moment it was
// reported: the report is delayed by settleAfterMouseUp, and a caret drag
// that starts while the user is typing is still a caret drag half a second
// later.
func typingSuppresses(g selectionGesture) bool {
	if g.Kind != gestureDrag {
		// A double click and Shift+arrow both mean "this is the word I want"
		// without any ambiguity, so they survive the guard. Only the drag is
		// the shape a caret placement shares with a selection.
		return false
	}
	if g.LastKey.IsZero() {
		// The user has not typed since the program started; there is nothing
		// to compare against.
		return false
	}
	if g.PressedAt.IsZero() {
		return false
	}
	since := g.PressedAt.Sub(g.LastKey)
	if since < 0 {
		// The two clocks cannot be compared. Guessing would silence the
		// trigger, which is worse than one stray chip.
		return false
	}
	return since < typingGrace
}

// gestureKind says how a gesture was made. The two differ in what the user
// most likely meant, which is why the typing guard only applies to one.
type gestureKind int

const (
	// gestureDrag is a left-button drag: the shape of a text selection, and
	// also the shape of a caret placement inside a text field.
	gestureDrag gestureKind = iota
	// gestureDoubleClick is a double click, which applications answer by
	// selecting the word under the pointer. It is exempt from the typing
	// guard: typing a word and double-clicking it is the normal way to look
	// it up, and that is exactly what a dictionary is for.
	gestureDoubleClick
	// gestureKeyboard is Shift plus an arrow key. It is a deliberate
	// selection by definition — there is no other thing it could be — so the
	// typing guard does not apply: the user types, then selects, then looks
	// the word up.
	gestureKeyboard
)

// String makes a gestureKind readable in a log line.
func (k gestureKind) String() string {
	switch k {
	case gestureDoubleClick:
		return "double-click"
	case gestureKeyboard:
		return "keyboard"
	default:
		return "drag"
	}
}

// dragIsSelection reports whether a completed drag is a text selection.
//
// It answers false for the two shapes that are not: a drag too short to be
// anything but a click or a scrollbar nudge, and a drag that ended as fast as
// it began.
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

// selectionTextProblem explains why a freshly read selection is not worth
// showing, or returns "" when it is.
//
// The text is judged before it is cleaned, because cleaning collapses the
// whitespace that tells a real phrase from a provider's fragment.
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

// isWordLike reports whether every rune is a letter, a digit or a CJK
// character: the shape of a word someone would look up.
func isWordLike(runes []rune) bool {
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || isCJK(r) {
			continue
		}
		return false
	}
	return true
}

// gestureGate remembers the last accepted gesture so the cooldown and the
// same-spot rule have something to compare against.
//
// It is called from the gesture goroutine, one at a time in practice, but the
// mutex keeps that from being an assumption.
type gestureGate struct {
	mu    sync.Mutex
	fired bool
	at    time.Time
	x, y  int32
}

// allow reports whether a gesture at (x, y) happening at now may raise the
// chip, and records it when it may. The recorded gesture is the one that was
// accepted, not the one that was refused, so a refusal never extends the
// cooldown.
//
// canAllow and record split the same rule in two. A caller that only learns
// whether the gesture is worth acting on, and records it once the text really
// was captured, uses the pair: recording before the capture burned the
// same-spot cooldown on a gesture that never produced a card.
func (g *gestureGate) allow(now time.Time, x, y int32) (bool, string) {
	ok, why := g.canAllow(now, x, y)
	if !ok {
		return false, why
	}
	g.record(now, x, y)
	return true, ""
}

// canAllow is allow without the bookkeeping: the same cooldown and same-spot
// rules, judged against the last recorded gesture, leaving the gate untouched.
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

// record marks a gesture as taken, which starts the cooldown.
func (g *gestureGate) record(now time.Time, x, y int32) {
	g.mu.Lock()
	g.fired = true
	g.at = now
	g.x, g.y = x, y
	g.mu.Unlock()
}

// reset forgets the last gesture, so a pause or a mode change does not leave
// a cooldown behind.
func (g *gestureGate) reset() {
	g.mu.Lock()
	g.fired = false
	g.mu.Unlock()
}
