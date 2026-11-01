package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestTrunkSummaryShowsOnlyTheMainClause is the regression test for the user's
// "长难句的句子划分和各种波浪线红线少了很多" report. The trunk line joined
// every subject and predicate in the sentence, so a compound sentence read as
// "主语: she / she / She / her arms / You" — six subjects for one trunk.
func TestTrunkSummaryShowsOnlyTheMainClause(t *testing.T) {
	a := newTestApp()
	a.mode = modeGrammar
	a.source = "She hands you a bottle, lays down on her back, her arms are up."
	a.grammarResult = &SentenceAnalysisResult{
		Annotations: []SyntaxAnnotation{
			{Text: "She", Type: "subject"},
			{Text: "hands", Type: "predicate"},
			{Text: "a bottle", Type: "object"},
			{Text: "lays", Type: "predicate"},
			{Text: "her arms", Type: "subject"},
			{Text: "are", Type: "predicate"},
		},
		Translation: "她递给你一瓶，躺下，双臂举起。",
	}

	tt := ui.NewTester(a.popupView, 520, 900)
	texts := tt.Texts()
	joined := ""
	for _, s := range texts {
		joined += s + "\n"
	}

	// The first predicate belongs to the first subject; the second subject and
	// its verb must not be strung onto the same line.
	if !containsText(joined, "主语: She") {
		t.Fatalf("trunk line lost its subject: %q", texts)
	}
	if !containsText(joined, "谓语: hands") {
		t.Fatalf("trunk line lost its predicate: %q", texts)
	}
	if containsText(joined, "hands / lays") || containsText(joined, "She / her arms") {
		t.Fatalf("trunk line still joins every clause: %q", texts)
	}
	// The omitted parts are still accounted for.
	if !containsText(joined, "另有") {
		t.Fatalf("trunk line does not say how much was left out: %q", texts)
	}
}

// TestTrunkSummaryPairsTheVerbWithItsOwnSubject covers the other half of the
// same report: the summary paired "the first subject in the list" with "the
// first predicate in the list", so a model that listed annotations out of
// sentence order produced "主语: This ➔ 谓语: is ➔ 宾语: you" for a sentence
// whose object was "a slide" and whose second subject was "I".
func TestTrunkSummaryPairsTheVerbWithItsOwnSubject(t *testing.T) {
	const sentence = "This is a slide from Terence Tao. I am confident that the cure worked."
	a := newTestApp()
	a.mode = modeGrammar
	a.source = sentence
	a.grammarResult = &SentenceAnalysisResult{
		Annotations: []SyntaxAnnotation{
			// Listed in the order the model noticed them, not sentence order.
			{Text: "you", Type: "object"},
			{Text: "I", Type: "subject"},
			{Text: "This", Type: "subject"},
			{Text: "a slide", Type: "object"},
			{Text: "am", Type: "predicate"},
			{Text: "is", Type: "predicate"},
		},
		Translation: "这是一张幻灯片。我相信治疗有效。",
	}

	tt := ui.NewTester(a.popupView, 520, 900)
	joined := ""
	for _, s := range tt.Texts() {
		joined += s + "\n"
	}

	if !containsText(joined, "主语: This") {
		t.Fatalf("trunk line paired the wrong subject: %q", joined)
	}
	if !containsText(joined, "谓语: is") {
		t.Fatalf("trunk line paired the wrong predicate: %q", joined)
	}
	if !containsText(joined, "宾语: a slide") {
		t.Fatalf("trunk line paired the wrong object: %q", joined)
	}
}
