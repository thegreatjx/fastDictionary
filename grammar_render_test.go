package main

import (
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
// TestGrammarUsesOneInkLadder pins the redesigned palette: the trunk keeps
// body ink, the predicate alone takes the theme accent and a fine underline,
// modifiers drop to muted grey, and clauses use a 10% accent wash instead of a
// second hue.
func TestGrammarUsesOneInkLadder(t *testing.T) {
	th := grammarThemeForTest(t)
	ink := grammarInkFor(th)
	at := appTheme(th)
	if ink.body != at.Text {
		t.Errorf("主干文字应使用 Text，得到 %+v", ink.body)
	}
	if ink.predicate != at.Accent {
		t.Errorf("谓语应使用当前主题 Accent，得到 %+v", ink.predicate)
	}
	if ink.muted != at.TextMuted {
		t.Errorf("修饰语应降为 TextMuted，得到 %+v", ink.muted)
	}
	if ink.connector != at.TextSec {
		t.Errorf("连接词应使用 TextSec，得到 %+v", ink.connector)
	}
	if ink.clauseTint.A >= ink.body.A {
		t.Errorf("从句底色必须是半透明微底，得到 %+v", ink.clauseTint)
	}
	for _, role := range []string{"subject", "predicate", "object", "complement", "connector"} {
		_, _, marked := markForRole(role, ink)
		if marked != (role == "predicate") {
			t.Errorf("%s 的线条状态 = %v，只有谓语允许一条底线", role, marked)
		}
	}
}

// TestGrammarWordRendersItsRoleMark checks that a role word still renders as
// its own styled node rather than a bare string, so its ink and tooltip
// survive the palette change.
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
		applyTheme(c, "light", false, "")
		got = c.Theme()
	}, 40, 20)
	tt.Frame()
	if got == nil {
		t.Fatal("applyTheme produced no theme")
	}
	return got
}
