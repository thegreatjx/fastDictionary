package main

import (
	"testing"
)

func TestGrammarAnalysisTextJoinsSentencesIntoOneParagraph(t *testing.T) {
	text := "This is a slide.\nI am confident it works.\n\nThen we stop!"
	if got, want := grammarAnalysisText(text), "This is a slide. I am confident it works. Then we stop!"; got != want {
		t.Fatalf("analysis text = %q, want %q", got, want)
	}
	if got := grammarAnalysisText("   "); got != "" {
		t.Fatalf("blank text produced %q, want nothing", got)
	}
}

func TestGrammarCardAnalysesTheWholeSelection(t *testing.T) {
	src := readSourceFile(t, "popup_view.go")
	if !containsText(src, "grammarAnalysisText(s.source)") {
		t.Error("the grammar card is not handed the whole selection")
	}
	card := readSourceFile(t, "grammar.go")
	if containsText(card, `第 %d / %d 句`) {
		t.Error("the grammar card still has a sentence pager")
	}
}

func TestSplitGrammarSentencesSplitsAParagraph(t *testing.T) {
	sents := splitGrammarSentences("This is a slide. I am confident it works. Then we stop!")
	if len(sents) != 3 {
		t.Fatalf("got %d sentences, want 3: %q", len(sents), sents)
	}
	for i, want := range []string{"This is a slide.", "I am confident it works.", "Then we stop!"} {
		if sents[i] != want {
			t.Errorf("sentence %d = %q, want %q", i, sents[i], want)
		}
	}
}

func TestSplitGrammarSentencesKeepsAbbreviationsAndDecimals(t *testing.T) {
	for _, text := range []string{
		"Mr. Smith paid 3.5 dollars.",
		"Dr. Lee wrote the e.g. example.",
	} {
		if sents := splitGrammarSentences(text); len(sents) != 1 {
			t.Errorf("%q split into %d sentences: %q", text, len(sents), sents)
		}
	}
}

func TestSplitGrammarSentencesKeepsTrailingQuoteWithItsSentence(t *testing.T) {
	sents := splitGrammarSentences(`He said "stop." Then he left.`)
	if len(sents) != 2 {
		t.Fatalf("got %d sentences, want 2: %q", len(sents), sents)
	}
	if sents[0] != `He said "stop."` {
		t.Errorf("first sentence = %q, want the closing quote kept", sents[0])
	}
}

func TestSplitGrammarSentencesHandlesCJKPunctuationAndBlankLines(t *testing.T) {
	sents := splitGrammarSentences("第一句。\n\n第二句！第三句？")
	if len(sents) != 3 {
		t.Fatalf("got %d sentences, want 3: %q", len(sents), sents)
	}
	if sents[2] != "第三句？" {
		t.Errorf("last sentence = %q", sents[2])
	}
}

func TestSplitGrammarSentencesFallsBackToTheWholeText(t *testing.T) {
	if sents := splitGrammarSentences("a fragment with no punctuation"); len(sents) != 1 {
		t.Fatalf("got %q, want the text as one sentence", sents)
	}
	if sents := splitGrammarSentences("   "); len(sents) != 0 {
		t.Fatalf("blank text produced %q, want nothing", sents)
	}
}
