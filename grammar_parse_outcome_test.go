package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestParseSentenceAnalysisOutcomes pins where each parse path stops.
func TestParseSentenceAnalysisOutcomes(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		source    string
		want      ParseOutcome
		wantSeg   int
		wantTrans string
	}{
		{"empty input", "   ", "", parseOutcomeEmpty, -1, ""},
		{"structured", `{"segments":[{"text":"She","role":"subject","is_backbone":true},{"text":" cooked","role":"predicate","is_backbone":true}],"translation":"她做饭"}`, "She cooked", parseOutcomeStructured, 2, "她做饭"},
		{"missing spaces are repaired", `{"segments":[{"text":"She","role":"subject"},{"text":"cooked","role":"predicate"}]}`, "She cooked", parseOutcomeStructured, 2, ""},
		{"translation only", `{"translation":"只有译文"}`, "", parseOutcomeTranslationOnly, 0, "只有译文"},
		{"segments do not match the source", `{"segments":[{"text":"He","role":"subject"}],"translation":"他"}`, "She", parseOutcomeFallbackDirect, 0, "他"},
		{"raw annotation array", `[{"text":"She","type":"subject"}]`, "", parseOutcomeFailed, -1, ""},
		{"not json", `this is not json at all`, "", parseOutcomeFailed, -1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseSentenceAnalysisDetailed(tc.raw, tc.source)
			if got.Outcome != tc.want {
				t.Fatalf("ParseSentenceAnalysisDetailed(%q) outcome = %v, want %v", tc.raw, got.Outcome, tc.want)
			}
			if tc.wantSeg < 0 {
				if got.Result != nil {
					t.Fatalf("expected no result, got %+v", got.Result)
				}
				if tc.want == parseOutcomeFailed && got.Cause == nil {
					t.Errorf("failed parse should carry a cause")
				}
				return
			}
			if got.Result == nil {
				t.Fatalf("outcome %v returned no result", got.Outcome)
			}
			if len(got.Result.Segments) != tc.wantSeg {
				t.Errorf("segments = %d, want %d", len(got.Result.Segments), tc.wantSeg)
			}
			if got.Result.Translation != tc.wantTrans {
				t.Errorf("translation = %q, want %q", got.Result.Translation, tc.wantTrans)
			}
			if tc.want == parseOutcomeStructured && concatSegments(got.Result.Segments) != tc.source {
				t.Errorf("segments restore %q, want %q", concatSegments(got.Result.Segments), tc.source)
			}
			plain := ParseSentenceAnalysis(tc.raw, tc.source)
			if plain == nil || len(plain.Segments) != tc.wantSeg || plain.Translation != tc.wantTrans {
				t.Errorf("wrapper disagrees with detailed parse: %+v", plain)
			}
		})
	}
}

func TestParseOutcomeString(t *testing.T) {
	cases := map[ParseOutcome]string{
		parseOutcomeEmpty:           "empty",
		parseOutcomeStructured:      "structured",
		parseOutcomeTranslationOnly: "translation_only",
		parseOutcomeFallbackDirect:  "fallback_direct",
		parseOutcomeFallbackArray:   "fallback_array",
		parseOutcomeFailed:          "failed",
		ParseOutcome(99):            "unknown",
	}
	for outcome, want := range cases {
		if got := outcome.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", outcome, got, want)
		}
	}
}

// TestGrammarCardExplainsMissingSegments pins that a translation-only answer
// says why the sentence has no analysis.
func TestGrammarCardExplainsMissingSegments(t *testing.T) {
	a := newTestApp()
	res := &SentenceAnalysisResult{Translation: "只有译文"}
	tt := ui.NewTester(func(c *ui.Context) {
		a.renderGrammarVisualCard(c, grammarThemeForTest(t), "Only then did she realize the truth.", res, false, nil)
	}, 700, 600)
	tt.Frame()
	joined := strings.Join(tt.Texts(), "|")
	if !strings.Contains(joined, "没有按原文切分") {
		t.Fatalf("grammar card did not explain the missing segments: %v", tt.Texts())
	}
}
