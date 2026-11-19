package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestGrammarV5ClauseLessNewsSentenceAndTreeView(t *testing.T) {
	const src = "Japan's dispute with China over Prime Minister Sanae Takaichi's remarks on Taiwan has spread from diplomatic protests to export restrictions by Beijing on critical minerals used in everything from cars and semiconductors to military equipment."
	const raw = `{
		"backbone_summary": "争端已蔓延到出口限制。",
		"segments": [
			{"text": "Japan's dispute", "role": "subject", "is_backbone": true},
			{"text": " with China", "role": "modifier", "phrase_id": "p1"},
			{"text": " over Prime Minister Sanae Takaichi's remarks", "role": "modifier", "break_before": true, "phrase_id": "p2"},
			{"text": " on Taiwan", "role": "modifier", "phrase_id": "p3"},
			{"text": " has spread", "role": "predicate", "break_before": true, "is_backbone": true},
			{"text": " from diplomatic protests", "role": "modifier", "break_before": true, "phrase_id": "p4"},
			{"text": " to export restrictions", "role": "modifier", "break_before": true, "phrase_id": "p5"},
			{"text": " by Beijing", "role": "modifier", "phrase_id": "p6"},
			{"text": " on critical minerals", "role": "modifier", "break_before": true, "phrase_id": "p7"},
			{"text": " used in everything", "role": "modifier", "break_before": true, "phrase_id": "p8"},
			{"text": " from cars and semiconductors to military equipment", "role": "modifier", "phrase_id": "p9"},
			{"text": ".", "role": "punct"}
		],
		"clauses": [],
		"phrases": [
			{"id": "p1", "type": "介词短语", "short_tag": "介短", "role": "后置定语", "target": "修饰 dispute"},
			{"id": "p2", "type": "介词短语", "short_tag": "介短", "role": "后置定语", "target": "修饰 dispute"},
			{"id": "p3", "parent_id": "p2", "type": "介词短语", "short_tag": "介短", "role": "后置定语", "target": "修饰 remarks"},
			{"id": "p4", "type": "介词短语", "short_tag": "介短", "role": "起点状语", "target": "修饰 has spread"},
			{"id": "p5", "type": "介词短语", "short_tag": "介短", "role": "终点状语", "target": "修饰 has spread"},
			{"id": "p6", "parent_id": "p5", "type": "介词短语", "short_tag": "介短", "role": "后置定语", "target": "修饰 restrictions"},
			{"id": "p7", "parent_id": "p5", "type": "介词短语", "short_tag": "介短", "role": "后置定语", "target": "修饰 restrictions"},
			{"id": "p8", "parent_id": "p7", "type": "分词短语", "short_tag": "分词", "role": "后置定语", "target": "修饰 minerals"},
			{"id": "p9", "parent_id": "p8", "type": "介词短语", "short_tag": "介短", "role": "范围定语", "target": "修饰 everything"}
		],
		"difficulties": [
			{"title": "多级短语嵌套", "span_text": "used in everything from cars and semiconductors to military equipment", "explanation": "过去分词短语后置修饰 minerals。"}
		],
		"translation": "日本与中国围绕高市早苗涉台言论的争端已蔓延至北京对关键矿产的出口限制。"
	}`

	parsed := ParseSentenceAnalysisDetailed(raw, src)
	if parsed.Outcome != parseOutcomeStructured || parsed.Result == nil {
		t.Fatalf("ParseSentenceAnalysisDetailed failed: outcome=%v cause=%v", parsed.Outcome, parsed.Cause)
	}
	res := parsed.Result
	if len(res.Phrases) != 9 {
		t.Fatalf("got %d phrases, want 9", len(res.Phrases))
	}
	if concatSegments(res.Segments) != src {
		t.Fatalf("segments do not match original source:\ngot:  %q\nwant: %q", concatSegments(res.Segments), src)
	}

	rows := grammarTreeRows(res)
	if len(rows) < 6 {
		t.Fatalf("grammarTreeRows produced %d rows, want >= 6", len(rows))
	}
	sawNestedDepth := false
	for _, r := range rows {
		if r.Depth >= 2 && strings.Contains(r.Target, "minerals") {
			sawNestedDepth = true
		}
	}
	if !sawNestedDepth {
		t.Fatalf("expected nested depth >= 2 targeting minerals in rows: %+v", rows)
	}

	a := newTestApp()
	a.mode = modeGrammar
	a.source = src
	a.grammarResult = res

	ttInline := ui.NewTester(a.popupView, 520, 760)
	ttInline.Frame()
	if !ttInline.HasText("存入句法笔记") || !ttInline.HasText("结构树") || !ttInline.HasText("1 句 · 9 处短语") {
		t.Fatalf("inline grammar card missing V5 controls/scope: %v", ttInline.Texts())
	}
	joined := strings.ReplaceAll(strings.Join(ttInline.Texts(), " "), "\u2060", "")
	if !strings.Contains(joined, "介短") || !strings.Contains(joined, "分词") {
		t.Fatalf("inline grammar card missing 括号法 phrase badges: %s", joined)
	}

	a.onlyTrunk = true
	ttTrunk := ui.NewTester(a.popupView, 520, 760)
	ttTrunk.Frame()
	if !strings.Contains(strings.Join(ttTrunk.Texts(), " "), "(介短 …)") {
		t.Fatalf("onlyTrunk did not fold phrases into (介短 …): %v", ttTrunk.Texts())
	}

	a.onlyTrunk = false
	a.showTree = true
	for _, dark := range []bool{false, true} {
		a.store.Update(func(c *Config) { c.ThemeMode = themeModeFor(dark) })
		ttTree := ui.NewTester(a.popupView, 520, 820)
		ttTree.Frame()
		treeText := strings.Join(ttTree.Texts(), " ")
		if !strings.Contains(treeText, "↳ 修饰 minerals") || !strings.Contains(treeText, "分词·后置定语") {
			t.Fatalf("dark=%v tree view missing hierarchy labels: %v", dark, ttTree.Texts())
		}
	}
}
