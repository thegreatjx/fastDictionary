package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

// TestWrappedLines pins the wrap estimator: every popup height below is built
// on it, so an off-by-one here is a clipped save button there.
func TestWrappedLines(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		width       float32
		fontSize    float32
		wantAtLeast int
		wantAtMost  int
	}{
		{"empty", "", 400, 14, 0, 0},
		{"one short ascii line", "hello", 400, 14, 1, 1},
		{"explicit newlines", "a\nb\nc", 400, 14, 3, 3},
		{"blank line still counts", "a\n\nb", 400, 14, 3, 3},
		// 200 ASCII chars at 14px ≈ 7.7px each = 1540px over a ~354px
		// usable width: five lines, and certainly more than one.
		{"long ascii wraps", strings.Repeat("abcdefghij", 20), 400, 14, 4, 6},
		{"cjk is wider", "中文字符测试", 200, 14, 1, 2},
		// The 80px usable floor keeps a degenerate width from turning a
		// short string into dozens of lines.
		{"tiny width uses the floor", "hello world hello world", 10, 14, 2, 4},
	}
	for _, tc := range cases {
		got := wrappedLines(tc.text, tc.width, tc.fontSize)
		if got < tc.wantAtLeast || got > tc.wantAtMost {
			t.Errorf("%s: wrappedLines(%q, %v, %v) = %d, want %d..%d",
				tc.name, truncateFor(tc.text, 20), tc.width, tc.fontSize,
				got, tc.wantAtLeast, tc.wantAtMost)
		}
	}
}

// TestWrappedLinesGrowsWithText guards the monotonicity the height maths
// depends on: more text must never mean fewer lines.
func TestWrappedLinesGrowsWithText(t *testing.T) {
	prev := 0
	for n := 0; n <= 200; n += 10 {
		got := wrappedLines(strings.Repeat("word ", n), 400, 14)
		if got < prev {
			t.Fatalf("wrappedLines went backwards at n=%d: %d < %d", n, got, prev)
		}
		prev = got
	}
}

// TestPopupContentHeightFloor pins the 196px minimum: a window shorter than
// that clips the source and the target together.
func TestPopupContentHeightFloor(t *testing.T) {
	if h := popupContentHeight(snapshot{mode: modeTranslate}, 440); h < 196 {
		t.Fatalf("empty translate popup = %vpx, below the 196 floor", h)
	}
	if h := popupContentHeight(snapshot{mode: modeInput}, 440); h < 196 {
		t.Fatalf("empty input popup = %vpx, below the 196 floor", h)
	}
}

// TestPopupContentHeightGrowsWithContent checks that each mode reacts to the
// data it renders — the bug class here is a fixed height that hides the
// buttons the user needs.
func TestPopupContentHeightGrowsWithContent(t *testing.T) {
	short := popupContentHeight(snapshot{
		mode:   modeTranslate,
		source: "hello",
		target: "你好",
	}, 440)
	long := popupContentHeight(snapshot{
		mode:   modeTranslate,
		source: strings.Repeat("the quick brown fox ", 8),
		target: strings.Repeat("敏捷的棕色狐狸跳过了懒狗。", 8),
	}, 440)
	if long <= short {
		t.Fatalf("long translation (%vpx) is not taller than short (%vpx)", long, short)
	}

	dict := popupContentHeight(snapshot{mode: modeDictionary}, 440)
	plain := popupContentHeight(snapshot{mode: modeTranslate}, 440)
	if dict <= plain {
		t.Fatalf("dictionary popup (%vpx) is not taller than an empty translate popup (%vpx)", dict, plain)
	}
}

// TestPopupContentHeightWritingCountsAnnotations is the regression that
// motivated the estimate: a fixed 40px per annotation clipped the save row
// as soon as an annotation wrapped.
func TestPopupContentHeightWritingCountsAnnotations(t *testing.T) {
	one := popupContentHeight(snapshot{
		mode: modeWriting,
		writingResult: &ImproveWritingResult{
			Annotations: []WritingAnnotation{{Text: "alpha", Fix: "beta"}},
		},
	}, 440)
	many := popupContentHeight(snapshot{
		mode: modeWriting,
		writingResult: &ImproveWritingResult{
			Annotations: []WritingAnnotation{
				{Text: strings.Repeat("alpha ", 20), Fix: strings.Repeat("beta ", 20)},
				{Text: strings.Repeat("gamma ", 20), Fix: strings.Repeat("delta ", 20)},
				{Text: strings.Repeat("epsilon ", 20), Fix: strings.Repeat("zeta ", 20)},
			},
			Improved: strings.Repeat("improved sentence ", 10),
			Summary:  strings.Repeat("summary text ", 10),
		},
	}, 440)
	if many <= one {
		t.Fatalf("three wrapped annotations (%vpx) not taller than one short one (%vpx)", many, one)
	}
}

// TestClampPopupHeight covers the work-area cap.
func TestClampPopupHeight(t *testing.T) {
	if got := clampPopupHeight(900, 0, 1080); got != 900 {
		t.Fatalf("clampPopupHeight(900, 0, 1080) = %v, want 900 (already fits)", got)
	}
	if got := clampPopupHeight(2000, 0, 1080); got != 1080 {
		t.Fatalf("clampPopupHeight(2000, 0, 1080) = %v, want 1080 (capped)", got)
	}
	// Margins wider than the work area must not yield a negative height.
	if got := clampPopupHeight(500, 600, 500); got > 500 {
		t.Fatalf("clampPopupHeight(500, 600, 500) = %v, want <= 500", got)
	}
}

// TestLiftPopupY covers the branches of the 主动抬高 logic.
func TestLiftPopupY(t *testing.T) {
	const waTop, waBottom = 14, 1066
	cases := []struct {
		name    string
		curY    int
		targetH int
		wantY   int
		wantH   int
	}{
		// Fits below the bottom edge: untouched.
		{"fits", 100, 300, 100, 300},
		// Bottom overflow: lifted so the last pixel lands on waBottom.
		{"lifts up", 900, 300, 766, 300},
		// Already above the top edge but short enough to fit: pinned down.
		{"pinned to top", -50, 300, 14, 300},
		// Taller than the work area: pinned to the top and shrunk to fit.
		{"shrunk to work area", 20, 2000, 14, 1052},
	}
	for _, tc := range cases {
		gotY, gotH := liftPopupY(tc.curY, tc.targetH, waTop, waBottom)
		if gotY != tc.wantY || gotH != tc.wantH {
			t.Errorf("%s: liftPopupY(%d, %d) = (%d, %d), want (%d, %d)",
				tc.name, tc.curY, tc.targetH, gotY, gotH, tc.wantY, tc.wantH)
		}
	}
}

// TestLiftPopupYAlwaysStaysInBounds is the invariant: whatever the inputs,
// the popup must end up fully visible.
func TestLiftPopupYAlwaysStaysInBounds(t *testing.T) {
	const waTop, waBottom = 14, 1066
	for _, curY := range []int{-500, 0, 100, 500, 1000, 5000} {
		for _, targetH := range []int{0, 260, 800, 2000} {
			y, h := liftPopupY(curY, targetH, waTop, waBottom)
			if y < waTop {
				t.Fatalf("liftPopupY(%d, %d) put y at %d, above waTop %d", curY, targetH, y, waTop)
			}
			if y+h > waBottom {
				t.Fatalf("liftPopupY(%d, %d) = (%d, %d) overflows waBottom %d", curY, targetH, y, h, waBottom)
			}
		}
	}
}

// TestCalcPopupPositionInStaysInsideWorkArea is the same invariant for the
// horizontal placement: the popup must never hang off the screen, which is
// how it became unreachable on a secondary monitor.
func TestCalcPopupPositionInStaysInsideWorkArea(t *testing.T) {
	wa := mygoRect(0, 0, 1920, 1080)
	const w, h = 440, 300
	for _, pt := range []mygo.Point{
		{X: 0, Y: 0}, {X: 960, Y: 540}, {X: 1919, Y: 1079},
		{X: -100, Y: -100}, {X: 5000, Y: 5000},
	} {
		x, y := calcPopupPositionIn(pt, w, h, wa)
		if x < 14 || x+w > 1920-14 {
			t.Fatalf("cursor %v: x=%d puts a %dpx popup outside the work area", pt, x, w)
		}
		if y < 14 || y+h > 1080-14 {
			t.Fatalf("cursor %v: y=%d puts a %dpx popup outside the work area", pt, y, h)
		}
	}
}

// TestCalcPopupPositionFlipsAboveTheCursor pins the deliberate behaviour: in
// the lower half of the screen the popup opens upwards so it is not cut off.
func TestCalcPopupPositionFlipsAboveTheCursor(t *testing.T) {
	wa := mygoRect(0, 0, 1920, 1080)
	_, highY := calcPopupPositionIn(mygo.Point{X: 800, Y: 100}, 440, 300, wa)
	_, lowY := calcPopupPositionIn(mygo.Point{X: 800, Y: 900}, 440, 300, wa)
	if highY <= 100 {
		t.Fatalf("cursor in the upper half: y=%d, want below the cursor", highY)
	}
	if lowY >= 900 {
		t.Fatalf("cursor in the lower half: y=%d, want above the cursor", lowY)
	}
}

// TestPopupContentHeightNotesExpansion ensures that extracting notes does not
// cause vertical layout jumps when collapsed, and properly accounts for the notes card
// only when explicitly expanded.
func TestPopupContentHeightNotesExpansion(t *testing.T) {
	baseSnap := snapshot{
		mode:   modeTranslate,
		source: "This is a relatively long sentence used to verify translation layout and notes extraction behavior.",
		target: "这是一个用于验证翻译布局和生词提取行为的较长句子。",
	}
	hBase := popupContentHeight(baseSnap, 440)

	busySnap := baseSnap
	busySnap.notesBusy = true
	hBusy := popupContentHeight(busySnap, 440)
	if hBusy != hBase {
		t.Fatalf("busy notes caused height jump when unexpanded: base=%v, busy=%v", hBase, hBusy)
	}

	readyCollapsedSnap := baseSnap
	readyCollapsedSnap.notes = []NoteItem{
		{Word: "relatively", Pos: "adv.", Meaning: "相对地"},
		{Word: "behavior", Pos: "n.", Meaning: "行为"},
	}
	readyCollapsedSnap.notesExpanded = false
	hReadyCollapsed := popupContentHeight(readyCollapsedSnap, 440)
	if hReadyCollapsed != hBase {
		t.Fatalf("ready notes caused height jump when collapsed: base=%v, collapsed=%v", hBase, hReadyCollapsed)
	}

	expandedSnap := readyCollapsedSnap
	expandedSnap.notesExpanded = true
	hExpanded := popupContentHeight(expandedSnap, 440)
	if hExpanded <= hBase {
		t.Fatalf("expanded notes did not grow height: base=%v, expanded=%v", hBase, hExpanded)
	}

	legalSnap := snapshot{
		mode:   modeTranslate,
		source: "Legal disclaimer: A registration statement relating to these securities has been filed with the SEC but has not yet become effective. These securities may not be sold nor may offers to buy be accepted prior to the time the registration statement becomes effective. This communication shall not constitute an offer to sell or the solicitation of an offer to buy these securities, nor shall there be any sale of these securities in any State or jurisdiction in which such offer, solicitation or sale would be unlawful prior to registration or qualification under the securities laws of any such State or jurisdiction.",
		target: "法律免责声明：与这些证券相关的注册声明已提交给SEC，但尚未生效。在注册声明生效之前，这些证券不得出售，也不得接受购买要约。本帖不构成出售或招揽购买这些证券的要约，也在任何州或司法管辖区，在注册或根据该州或司法管辖区的证券法认证前，此类要约、招集或销售不合法，也不会有任何销售。",
		notes: []NoteItem{
			{Word: "disclaimer", Pos: "n.", Meaning: "免责声明"},
			{Word: "solicitation", Pos: "n.", Meaning: "招揽"},
		},
		notesExpanded: true,
	}
	hLegal := popupContentHeight(legalSnap, 440)
	if hLegal > 600 {
		t.Fatalf("hLegal (expanded) = %vpx, bloated with excessive whitespace (>600px)", hLegal)
	}

	legalCollapsed := legalSnap
	legalCollapsed.notesExpanded = false
	hLegalCollapsed := popupContentHeight(legalCollapsed, 440)
	if hLegal <= hLegalCollapsed {
		t.Fatalf("hLegal (expanded) %vpx not taller than collapsed %vpx", hLegal, hLegalCollapsed)
	}
	if hLegalCollapsed < 320 {
		t.Fatalf("hLegal (collapsed) = %vpx, too cramped (<320px)", hLegalCollapsed)
	}
}
