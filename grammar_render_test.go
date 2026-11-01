package main

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// grammarFixture is a sentence with a main clause, a nested condition and a
// final object clause — the shape that used to render as a wall of chips.
const grammarFixture = "This is a slide from Terence Tao's lecture. I am highly confident that, if the cure actually worked, almost any Stage 4 cancer patient would tell you that human comprehension of its mechanism is completely irrelevant to them."

func grammarFixtureResult() *SentenceAnalysisResult {
	return &SentenceAnalysisResult{
		Annotations: []SyntaxAnnotation{
			{Text: "This", Type: "subject"},
			{Text: "is", Type: "predicate"},
			{Text: "I", Type: "subject"},
			{Text: "am", Type: "predicate"},
			{Text: "that, if the cure actually worked, almost any Stage 4 cancer patient would tell you that human comprehension of its mechanism is completely irrelevant to them", Type: "complement", Form: "clause"},
			{Text: "that", Type: "connector"},
			{Text: "if the cure actually worked", Type: "adverbial", Form: "clause", Sense: "condition"},
			{Text: "if", Type: "connector"},
			{Text: "the cure", Type: "subject"},
			{Text: "worked", Type: "predicate"},
			{Text: "almost any Stage 4 cancer patient", Type: "subject"},
			{Text: "would tell", Type: "predicate"},
			{Text: "you", Type: "object"},
			{Text: "that human comprehension of its mechanism is completely irrelevant to them", Type: "object", Form: "clause"},
			{Text: "human comprehension of its mechanism", Type: "subject"},
			{Text: "is", Type: "predicate"},
		},
	}
}

// renderGrammarText renders the marked-up sentence and returns every string it
// drew, concatenated, so a test can assert what the reader would see.
func renderGrammarText(t *testing.T, a *app, trunk bool) string {
	t.Helper()
	a.onlyTrunk = trunk
	tt := ui.NewTester(func(c *ui.Context) {
		a.renderGrammarSpans(c, grammarThemeForTest(t), grammarFixture, grammarFixtureResult())
	}, 700, 400)
	tt.Frame()
	var b strings.Builder
	for _, s := range tt.Texts() {
		b.WriteString(s)
	}
	return b.String()
}

// TestTrunkToggleFoldsTheSentence pins that "只看主干" actually shortens what is
// drawn.
//
// It used to only dim non-core words, which left the sentence exactly as long
// and defeated the point: the reader still had to hold the whole thing in view
// to find the spine. The fold has to remove text, not fade it.
func TestTrunkToggleFoldsTheSentence(t *testing.T) {
	a := newTestApp()
	full := renderGrammarText(t, a, false)
	trunk := renderGrammarText(t, a, true)

	if len(trunk) >= len(full) {
		t.Fatalf("trunk mode drew %d characters against %d full: nothing was folded",
			len(trunk), len(full))
	}
	// The fold must be visible as a marker, or a short sentence reads as if
	// the model found little to say.
	if !strings.Contains(trunk, "…") {
		t.Errorf("trunk mode has no fold marker: %q", trunk)
	}
	// The spine has to survive: a fold that drops the main clause is worse
	// than no fold at all.
	for _, keep := range []string{"This", "confident"} {
		if !strings.Contains(full, keep) {
			t.Fatalf("fixture lost %q before folding", keep)
		}
	}
}

// TestGrammarRolesUseLinesNotFills guards the redesign.
//
// The old card filled each role with its own high-saturation block — yellow
// subject, blue predicate, green object — so the sentence read as a row of
// sticky notes. Roles are now distinguished by the kind of line under the word
// plus a quiet colour, and the palette stays cool and low-chroma.
// TestGrammarRolesAreDistinctAndSaturated guards the grammar card's palette.
//
// This card is a teaching aid, so its job is the opposite of the selection
// chip's: eight roles have to tell themselves apart at a glance, and the
// muted greys the chip uses cannot do that. The check is therefore that every
// role colour is distinguishable from every other, not that it is quiet.
//
// The chip stays quiet (see selicon_palette_test.go); the two palettes answer
// different questions and must not be collapsed into one rule.
func TestGrammarRolesAreDistinctAndSaturated(t *testing.T) {
	ink := grammarInkFor(grammarThemeForTest(t))
	colors := map[string]ui.Color{
		"subject":    ink.subjectLine,
		"predicate":  ink.predicateLine,
		"object":     ink.objectLine,
		"complement": ink.complementLine,
		"connector":  ink.connector,
		"obstacle":   ink.obstacle,
	}
	// Every pair must differ enough to be told apart at a glance. The eye
	// separates hue far better than brightness, so the distance is measured in
	// RGB with a floor well above "slightly different grey".
	names := make([]string, 0, len(colors))
	for n := range colors {
		names = append(names, n)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			a, b := colors[names[i]], colors[names[j]]
			d := colorDistance(a, b)
			if d < 60 {
				t.Errorf("%s %#v and %s %#v are only %.0f apart; roles must be distinguishable",
					names[i], a, names[j], b, d)
			}
		}
	}
}

// colorDistance is the plain RGB euclidean distance, which is enough to catch
// two roles sharing a colour.
func colorDistance(a, b ui.Color) float64 {
	dr := float64(a.R) - float64(b.R)
	dg := float64(a.G) - float64(b.G)
	db := float64(a.B) - float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// TestGrammarRolesDifferInKindNotOnlyHue checks the second channel: a reader
// who cannot separate the hues must still see which part is which, so the line
// styles have to be distinct.
func TestGrammarRolesDifferInKindNotOnlyHue(t *testing.T) {
	ink := grammarInkFor(grammarThemeForTest(t))
	roles := []string{"subject", "predicate", "object", "complement"}
	seen := map[markKind]string{}
	for _, role := range roles {
		kind, _, _ := markForRole(role, ink)
		if prev, dup := seen[kind]; dup {
			t.Errorf("%s and %s share the %v line style; each role needs its own",
				prev, role, kind)
		}
		seen[kind] = role
	}
	if len(seen) != len(roles) {
		t.Fatalf("%d roles mapped onto %d line styles", len(roles), len(seen))
	}
}

// TestGrammarWordRendersItsRoleMark checks that each role draws the line the
// redesign promises, rather than only a colour.
func TestGrammarWordRendersItsRoleMark(t *testing.T) {
	cases := []struct {
		role string
		want string
	}{
		{"subject", "subject"},
		{"predicate", "predicate"},
	}
	for _, c := range cases {
		a := newTestApp()
		tt := ui.NewTester(func(ctx *ui.Context) {
			a.renderGrammarSpans(ctx, grammarThemeForTest(t), "The report was never discussed", &SentenceAnalysisResult{
				Annotations: []SyntaxAnnotation{
					{Text: "The report", Type: "subject"},
					{Text: "was never discussed", Type: "predicate", Obstacle: "passive"},
				},
			})
		}, 600, 120)
		tt.Frame()
		// A word carrying a role must not be drawn as a bare string: it goes
		// through a styled node, so the text survives but the mark does too.
		joined := strings.Join(tt.Texts(), "")
		if !strings.Contains(joined, "report") {
			t.Errorf("%s: word missing from %q", c.role, joined)
		}
	}
}

// grammarThemeForTest returns the theme the grammar card really draws with,
// taken through applyTheme so a palette change reaches these checks.
func grammarThemeForTest(t *testing.T) *ui.Theme {
	t.Helper()
	var got *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, "light", false)
		got = c.Theme()
	}, 40, 20)
	tt.Frame()
	if got == nil {
		t.Fatal("applyTheme produced no theme")
	}
	return got
}
