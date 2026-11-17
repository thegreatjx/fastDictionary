package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type recordedGesture struct {
	owner string
	text  string
	why   string
}

func loadRecordedGestures(t *testing.T, path string) []recordedGesture {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []recordedGesture
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			t.Fatalf("malformed row %q in %s", line, path)
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			t.Fatalf("bad character count in row %q: %v", line, err)
		}
		head := strings.ReplaceAll(parts[2], `\n`, "\n")
		head = strings.ReplaceAll(head, `\r`, "\r")
		runes := []rune(head)
		if n < len(runes) {
			runes = runes[:n]
		}
		out = append(out, recordedGesture{
			owner: strings.TrimSpace(parts[0]),
			text:  string(runes),
		})
	}
	return out
}

func TestDragIsSelection(t *testing.T) {
	cases := []struct {
		name  string
		start winPoint
		end   winPoint
		held  time.Duration
		want  bool
		why   string
	}{
		{
			name:  "a deliberate drag is a selection",
			start: winPoint{100, 100}, end: winPoint{200, 140},
			held: 300 * time.Millisecond,
			want: true,
		},
		{
			name:  "travel exactly at the floor is enough",
			start: winPoint{0, 0}, end: winPoint{minSelectionDrag, 0},
			held: 300 * time.Millisecond,
			want: true,
		},
		{
			name:  "diagonal travel is measured as distance",
			start: winPoint{0, 0}, end: winPoint{12, 16},
			held: 300 * time.Millisecond,
			want: true,
		},
		{
			name:  "one pixel short of the floor is a click",
			start: winPoint{0, 0}, end: winPoint{minSelectionDrag - 1, 0},
			held: 300 * time.Millisecond,
			want: false, why: "drag too short",
		},
		{
			name:  "a flick with selection-like travel is refused",
			start: winPoint{0, 0}, end: winPoint{60, 0},
			held: minSelectionDragDuration - time.Millisecond,
			want: false, why: "drag too fast",
		},
		{
			name:  "holding exactly at the duration floor passes",
			start: winPoint{0, 0}, end: winPoint{60, 0},
			held: minSelectionDragDuration,
			want: true,
		},
		{
			name:  "an unknown duration only judges travel",
			start: winPoint{0, 0}, end: winPoint{60, 0},
			held: 0,
			want: true,
		},
		{
			name:  "dragging up and to the left is a selection",
			start: winPoint{500, 500}, end: winPoint{440, 460},
			held: 200 * time.Millisecond,
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, why := dragIsSelection(tc.start, tc.end, tc.held)
			if got != tc.want {
				t.Fatalf("dragIsSelection(%v, %v, %v) = %v (%q), want %v",
					tc.start, tc.end, tc.held, got, why, tc.want)
			}
			if why != tc.why {
				t.Errorf("reason = %q, want %q", why, tc.why)
			}
		})
	}
}

func TestSelectionTextProblem(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"", "empty"},
		{"   ", "empty"},
		{"\r\n\t ", "empty"},

		{"p", "one character"},
		{"中", "one character"},
		{"7", "one character"},

		{"AI", ""},
		{"42", ""},
		{"omp", ""},
		{"aud", ""},
		{"the", ""},
		{"你好", ""},
		{"1+1", "fragment"},
		{"a b", "fragment"},
		{"，，", "fragment"},
		{"—", "one character"},

		{"hello", ""},
		{"serendipity", ""},
		{"898.8−845.4=", ""},
		{"达利奥", ""},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			if got := selectionTextProblem(tc.text); got != tc.want {
				t.Errorf("selectionTextProblem(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestIsWordLike(t *testing.T) {
	cases := map[string]bool{
		"abc":    true,
		"ABC":    true,
		"42":     true,
		"中文":     true,
		"ひらがな":   true,
		"カタカナ":   true,
		"한글":     true,
		"a1":     true,
		"a-b":    false,
		"a b":    false,
		"3.14":   false,
		"“引号”":   false,
		"a+b":    false,
		"emoji😀": false,
		"":       true,
		"a_b":    false,
		"#tag":   false,
		"（括号）":   false,
	}
	for text, want := range cases {
		t.Run(text, func(t *testing.T) {
			if got := isWordLike([]rune(text)); got != want {
				t.Errorf("isWordLike(%q) = %v, want %v", text, got, want)
			}
		})
	}
}

func TestGestureGateCooldown(t *testing.T) {
	base := time.Date(2026, 10, 23, 13, 0, 0, 0, time.UTC)
	at := func(d time.Duration) time.Time { return base.Add(d) }

	t.Run("first gesture is always allowed", func(t *testing.T) {
		var g gestureGate
		if ok, why := g.allow(at(0), 100, 100); !ok {
			t.Fatalf("first gesture refused: %q", why)
		}
	})

	t.Run("a repeat within the cooldown is refused", func(t *testing.T) {
		var g gestureGate
		g.allow(at(0), 100, 100)
		if ok, why := g.allow(at(gestureCooldown/2), 900, 900); ok {
			t.Fatalf("repeat within the cooldown was allowed")
		} else if why != "cooldown" {
			t.Errorf("reason = %q, want %q", why, "cooldown")
		}
	})

	t.Run("the same spot is refused after the cooldown", func(t *testing.T) {
		var g gestureGate
		g.allow(at(0), 100, 100)
		if ok, why := g.allow(at(gestureCooldown+time.Millisecond), 104, 103); ok {
			t.Fatalf("same spot was allowed")
		} else if why != "same spot" {
			t.Errorf("reason = %q, want %q", why, "same spot")
		}
	})

	t.Run("moving past the radius is a new gesture", func(t *testing.T) {
		var g gestureGate
		g.allow(at(0), 100, 100)
		if ok, why := g.allow(at(gestureCooldown+time.Millisecond),
			100+sameSpotRadius+1, 100+sameSpotRadius+1); !ok {
			t.Fatalf("a gesture outside the radius was refused: %q", why)
		}
	})

	t.Run("the radius is inclusive", func(t *testing.T) {
		var g gestureGate
		g.allow(at(0), 100, 100)
		if ok, _ := g.allow(at(gestureCooldown+time.Millisecond), 100+sameSpotRadius, 100); ok {
			t.Fatalf("a gesture exactly on the radius was allowed")
		}
	})

	t.Run("the same spot is allowed once the window passes", func(t *testing.T) {
		var g gestureGate
		g.allow(at(0), 100, 100)
		if ok, why := g.allow(at(sameSpotCooldown), 100, 100); !ok {
			t.Fatalf("same spot after the window was refused: %q", why)
		}
	})

	t.Run("a refusal does not extend the cooldown", func(t *testing.T) {
		var g gestureGate
		g.allow(at(0), 100, 100)
		g.allow(at(100*time.Millisecond), 900, 900)
		if ok, why := g.allow(at(gestureCooldown+50*time.Millisecond), 900, 900); !ok {
			t.Fatalf("cooldown was extended by a refusal: %q", why)
		}
	})

	t.Run("reset forgets the last gesture", func(t *testing.T) {
		var g gestureGate
		g.allow(at(0), 100, 100)
		g.reset()
		if ok, why := g.allow(at(0), 100, 100); !ok {
			t.Fatalf("gesture after reset was refused: %q", why)
		}
	})
}

func TestGestureGateResetOnResume(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) { c.SelectionTrigger = "icon" })
	reads := 0
	a.captureSelection = func() selection { reads++; return selection{Text: "serendipity"} }
	a.showChip = func(x, y int, text string) {}

	a.onSelectionGesture(dragGesture(100, 200, "chrome.exe"))
	if reads != 1 {
		t.Fatalf("first gesture read %d times, want 1", reads)
	}
	a.onSelectionGesture(dragGesture(100, 200, "chrome.exe"))
	if reads != 1 {
		t.Fatalf("repeat at the same spot was read again: reads=%d, want 1", reads)
	}

	a.pauseSelection(time.Minute)
	a.resumeSelection()

	a.onSelectionGesture(dragGesture(100, 200, "chrome.exe"))
	if reads != 2 {
		t.Errorf("gesture after resume was dropped by the old cooldown: reads=%d, want 2", reads)
	}
}

func TestRecordedGesturesReplay(t *testing.T) {
	rows := loadRecordedGestures(t, "testdata/gestures_2026-10-23.tsv")
	if len(rows) < 100 {
		t.Fatalf("only %d recorded gestures; the fixture looks truncated", len(rows))
	}

	var rejected []recordedGesture
	for _, g := range rows {
		if why := selectionTextProblem(g.text); why != "" {
			g.why = why
			rejected = append(rejected, g)
		}
	}

	if len(rejected) != 1 {
		for _, g := range rejected {
			t.Logf("rejected: owner=%q chars=%d text=%q reason=%q",
				g.owner, len([]rune(g.text)), g.text, g.why)
		}
		t.Fatalf("rejected %d of %d recorded gestures, want 1", len(rejected), len(rows))
	}
	got := rejected[0]
	if got.text != "p" {
		t.Errorf("rejected %q, want the single character %q", got.text, "p")
	}
	if got.why != "one character" {
		t.Errorf("reason for %q = %q, want %q", got.text, got.why, "one character")
	}

	for _, want := range []string{"aud", "omp"} {
		found := false
		for _, g := range rows {
			if g.text != want {
				continue
			}
			found = true
			if why := selectionTextProblem(g.text); why != "" {
				t.Errorf("%q was rejected as %q; it is a word the user selected", want, why)
			}
		}
		if !found {
			t.Errorf("%q is missing from the recording; the fixture changed", want)
		}
	}
}

func TestTypingSuppresses(t *testing.T) {
	base := time.Date(2026, 10, 23, 22, 10, 0, 0, time.Local)
	typed := base.Add(10 * time.Second)

	drag := func(pressedAfter time.Duration) selectionGesture {
		return selectionGesture{
			Kind:      gestureDrag,
			LastKey:   typed,
			PressedAt: typed.Add(pressedAfter),
		}
	}

	cases := []struct {
		name string
		g    selectionGesture
		want bool
		why  string
	}{
		{
			name: "a drag inside the typing window is a caret move",
			g:    drag(50 * time.Millisecond),
			want: true,
		},
		{
			name: "a drag just inside the window is still typing",
			g:    drag(typingGrace - time.Millisecond),
			want: true,
		},
		{
			name: "a drag at the edge of the window is not typing",
			g:    drag(typingGrace),
			want: false,
		},
		{
			name: "a drag long after the last keystroke is a selection",
			g:    drag(5 * time.Second),
			want: false,
		},
		{
			name: "no keystroke seen since start cannot suppress anything",
			g:    selectionGesture{Kind: gestureDrag, PressedAt: typed},
			want: false,
		},
		{
			name: "an unset press time cannot suppress anything",
			g:    selectionGesture{Kind: gestureDrag, LastKey: typed},
			want: false,
		},
		{
			name: "clocks that disagree cannot suppress anything",
			g: selectionGesture{
				Kind:      gestureDrag,
				LastKey:   typed,
				PressedAt: typed.Add(-time.Second),
			},
			want: false,
		},
		{
			name: "a double click is exempt: it is how a word is looked up",
			g: selectionGesture{
				Kind:      gestureDoubleClick,
				LastKey:   typed,
				PressedAt: typed.Add(10 * time.Millisecond),
			},
			want: false,
		},
		{
			name: "a keyboard selection is exempt: it is deliberate by definition",
			g: selectionGesture{
				Kind:      gestureKeyboard,
				LastKey:   typed,
				PressedAt: typed.Add(10 * time.Millisecond),
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := typingSuppresses(tc.g); got != tc.want {
				t.Errorf("typingSuppresses(%s, last key %v before press) = %v, want %v",
					tc.g.Kind, tc.g.PressedAt.Sub(tc.g.LastKey), got, tc.want)
			}
		})
	}
}

func TestTypingGraceIsShortEnough(t *testing.T) {
	if typingGrace < 200*time.Millisecond {
		t.Errorf("typingGrace = %v; too short to cover the reach for the mouse after typing", typingGrace)
	}
	if typingGrace > 600*time.Millisecond {
		t.Errorf("typingGrace = %v; long enough to swallow a deliberate selection", typingGrace)
	}
}

func TestGestureKindString(t *testing.T) {
	cases := map[gestureKind]string{
		gestureDrag:        "drag",
		gestureDoubleClick: "double-click",
		gestureTripleClick: "triple-click",
		gestureKeyboard:    "keyboard",
	}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Errorf("gestureKind(%d).String() = %q, want %q", int(kind), got, want)
		}
	}
}
