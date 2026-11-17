package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

const grammarFixture = "We are doubling the context window for Argon and Carbon because users keep hitting the limit."

func grammarFixtureResult() *SentenceAnalysisResult {
	return &SentenceAnalysisResult{
		BackboneSummary: "我们正在扩大上下文窗口。",
		Segments: []Segment{
			{Text: "We", Role: RoleSubject, IsBackbone: true},
			{Text: " are doubling", Role: RolePredicate, IsBackbone: true},
			{Text: " the context window", Role: RoleObject, IsBackbone: true},
			{Text: " for Argon and Carbon", Role: RoleModifier, Note: "介词短语作定语，修饰 window"},
			{Text: " because", Role: RoleConnector, BreakBefore: true, ClauseID: "c1", Note: "引导原因状语从句"},
			{Text: " users", Role: RoleSubject, ClauseID: "c1"},
			{Text: " keep hitting", Role: RolePredicate, ClauseID: "c1"},
			{Text: " the limit", Role: RoleObject, ClauseID: "c1"},
			{Text: ".", Role: RolePunct},
		},
		Clauses: []ClauseInfo{
			{ID: "c1", Type: "原因状语从句", ShortTag: "状从", Target: "修饰 are doubling"},
		},
		Difficulties: []DifficultyPoint{
			{Title: "后置修饰", SpanText: "for Argon and Carbon", Explanation: "介词短语跟在宾语后作定语，不是句子的第二个宾语。"},
		},
		Translation: "我们正在为 Argon 和 Carbon 扩大上下文窗口，因为用户不断触到上限。",
	}
}

func renderGrammarText(t *testing.T, trunk bool) string {
	t.Helper()
	th := grammarThemeForTest(t)
	tt := ui.NewTester(func(c *ui.Context) {
		renderGrammarVisualCardEx(c, th, grammarFixture, grammarFixtureResult(), false, grammarCardState{
			OnlyTrunk: trunk,
		})
	}, 700, 500)
	tt.Frame()
	return strings.Join(tt.Texts(), "")
}

func TestTrunkToggleFoldsTheSentence(t *testing.T) {
	full := renderGrammarText(t, false)
	trunk := renderGrammarText(t, true)

	if len(trunk) >= len(full) {
		t.Fatalf("trunk mode drew %d characters against %d full: nothing was folded",
			len(trunk), len(full))
	}
	if !strings.Contains(trunk, "…") {
		t.Errorf("trunk mode has no fold marker: %q", trunk)
	}
	for _, keep := range []string{"We", "doubling", "context"} {
		if !strings.Contains(full, keep) {
			t.Fatalf("fixture lost %q before folding", keep)
		}
	}
	if !strings.Contains(trunk, "doubling") {
		t.Errorf("trunk mode dropped the main predicate: %q", trunk)
	}
}

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
	if colors := grammarClausePalette(th, "宾从"); colors.wash.A >= colors.badgeBg.A || colors.badgeBg.A >= 255 {
		t.Errorf("从句底色与徽章底色必须分层：wash=%+v badge=%+v", colors.wash, colors.badgeBg)
	}
}

func TestGrammarCardRendersSegmentText(t *testing.T) {
	th := grammarThemeForTest(t)
	const sentence = "The report was never discussed"
	tt := ui.NewTester(func(c *ui.Context) {
		renderGrammarVisualCardEx(c, th, sentence, &SentenceAnalysisResult{
			Segments: []Segment{
				{Text: "The report", Role: RoleSubject, IsBackbone: true},
				{Text: " was never discussed", Role: RolePredicate, IsBackbone: true},
			},
		}, false, grammarCardState{})
	}, 600, 300)
	tt.Frame()
	joined := strings.Join(tt.Texts(), "")
	for _, want := range []string{"The report", "was never discussed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q missing from %q", want, joined)
		}
	}
}

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
