package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestParseSentenceAnalysisOutcomes 钉住每一级解析的去向。
//
// 修复前 ParseSentenceAnalysis 只返回 nil / 非 nil，空输入、不是 JSON、
// 只有译文没有标注三种情况在调用方看来完全一样。
func TestParseSentenceAnalysisOutcomes(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		want      ParseOutcome
		wantAnn   int
		wantTrans string
	}{
		{"empty input", "   ", parseOutcomeEmpty, -1, ""},
		{"structured", `{"annotations":[{"text":"She","type":"subject"}],"translation":"她"}`, parseOutcomeStructured, 1, "她"},
		{"annotations as string", `{"annotations":"[{\"text\":\"She\",\"type\":\"subject\"}]"}`, parseOutcomeStructured, 1, ""},
		{"translation only", `{"translation":"只有译文"}`, parseOutcomeTranslationOnly, 0, "只有译文"},
		{"raw annotation array", `[{"text":"She","type":"subject"}]`, parseOutcomeFallbackArray, 1, ""},
		{"not json", `this is not json at all`, parseOutcomeFailed, -1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseSentenceAnalysisDetailed(tc.raw)
			if got.Outcome != tc.want {
				t.Fatalf("ParseSentenceAnalysisDetailed(%q) outcome = %v, want %v", tc.raw, got.Outcome, tc.want)
			}
			if tc.wantAnn < 0 {
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
			if len(got.Result.Annotations) != tc.wantAnn {
				t.Errorf("annotations = %d, want %d", len(got.Result.Annotations), tc.wantAnn)
			}
			if got.Result.Translation != tc.wantTrans {
				t.Errorf("translation = %q, want %q", got.Result.Translation, tc.wantTrans)
			}
			plain := ParseSentenceAnalysis(tc.raw)
			if plain == nil || len(plain.Annotations) != tc.wantAnn || plain.Translation != tc.wantTrans {
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

// TestGrammarCardExplainsMissingAnnotations 钉住「只有译文」时卡片要说明原因。
func TestGrammarCardExplainsMissingAnnotations(t *testing.T) {
	a := newTestApp()
	res := &SentenceAnalysisResult{Translation: "只有译文"}
	tt := ui.NewTester(func(c *ui.Context) {
		a.renderGrammarVisualCard(c, grammarThemeForTest(t), "Only then did she realize the truth.", res, false, nil)
	}, 700, 600)
	tt.Frame()
	joined := strings.Join(tt.Texts(), "|")
	if !strings.Contains(joined, "没有语法标注") {
		t.Fatalf("grammar card did not explain the missing annotations: %v", tt.Texts())
	}
}
