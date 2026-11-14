package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 长难句卡片一次只分析一句：整段发给模型时，模型会把不同句子的成分混进
// 同一张卡片，回答也慢得多。拆句是纯函数，边界情况可以直接测。

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
	// 缩写和版本号里的点不是句末：把它们切开，卡片会把 "Mr." 单独当成一句话。
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
	// 没有句末标点时仍然要有内容，否则卡片会空着。
	if sents := splitGrammarSentences("a fragment with no punctuation"); len(sents) != 1 {
		t.Fatalf("got %q, want the text as one sentence", sents)
	}
	if sents := splitGrammarSentences("   "); len(sents) != 0 {
		t.Fatalf("blank text produced %q, want nothing", sents)
	}
}

func TestGrammarActiveSentenceClampsTheIndex(t *testing.T) {
	text := "One. Two. Three."

	if sent, idx, count := grammarActiveSentence(text, 0); sent != "One." || idx != 0 || count != 3 {
		t.Errorf("first = %q idx=%d count=%d", sent, idx, count)
	}
	// 负数夹到第一句：上一句按钮在下标 0 上被禁用，但状态有可能被别处写坏。
	if sent, idx, _ := grammarActiveSentence(text, -5); sent != "One." || idx != 0 {
		t.Errorf("negative index = %q idx=%d, want the first sentence", sent, idx)
	}
	// 越界夹到最后一句，而不是返回空字符串。
	if sent, idx, _ := grammarActiveSentence(text, 99); sent != "Three." || idx != 2 {
		t.Errorf("out of range index = %q idx=%d, want the last sentence", sent, idx)
	}
}

func TestClauseBadgeLabelOnlyLabelsRealClauses(t *testing.T) {
	cases := []struct {
		name string
		item SyntaxAnnotation
		want string
	}{
		{
			// 模型给副词填了 sense=time：没有 form=clause 就不是从句。
			name: "an adverb with a time sense is not a clause",
			item: SyntaxAnnotation{Text: "Tonight", Type: "adverbial", Sense: "time"},
			want: "",
		},
		{
			name: "a real adverbial clause keeps its sense label",
			item: SyntaxAnnotation{Text: "because it rained", Type: "adverbial", Form: "clause", Sense: "cause"},
			want: "原因状从",
		},
		{
			name: "case and spaces in form do not matter",
			item: SyntaxAnnotation{Text: "that he left", Type: "object", Form: " Clause "},
			want: "宾语从句",
		},
		{
			name: "a two word non-finite phrase gets no badge",
			item: SyntaxAnnotation{Text: "to rest", Type: "object", Form: "infinitive"},
			want: "",
		},
		{
			name: "a long enough non-finite phrase keeps its badge",
			item: SyntaxAnnotation{Text: "Exhausted by the long journey", Type: "adverbial", Form: "past-participle"},
			want: "过去分词",
		},
	}
	for _, tc := range cases {
		if got := clauseBadgeLabel(tc.item); got != tc.want {
			t.Errorf("%s: clauseBadgeLabel(%+v) = %q, want %q", tc.name, tc.item, got, tc.want)
		}
	}
}

// TestGrammarCardOffersASentencePager pins the UI half of the split: a
// multi-sentence selection shows one sentence with a pager, so the reader can
// walk the paragraph without asking the model for the whole thing at once.
func TestGrammarCardOffersASentencePager(t *testing.T) {
	a := newTestApp()
	a.source = "One sentence here. Two sentence here. Three sentence here."
	res := &SentenceAnalysisResult{
		Annotations: []SyntaxAnnotation{{Text: "One sentence here", Type: "subject"}},
		Translation: "第一句。",
	}
	tt := ui.NewTester(func(c *ui.Context) {
		sent, _, _ := grammarActiveSentence(a.source, a.grammarSentIdx)
		a.renderGrammarVisualCard(c, grammarThemeForTest(t), sent, res, false, nil)
	}, 700, 400)
	tt.Frame()
	texts := strings.Join(tt.Texts(), "")
	if !strings.Contains(texts, "第 1 / 3 句") {
		t.Errorf("no sentence pager in the card, texts %q", texts)
	}
}

// TestGrammarCardShowsOneSentence pins the wiring: the card renders the active
// sentence, and a multi-sentence text offers a pager instead of analysing the
// whole paragraph at once.
func TestGrammarCardShowsOneSentence(t *testing.T) {
	src := readSourceFile(t, "popup_view.go")
	if !containsText(src, "grammarActiveSentence(s.source, s.grammarSentIdx)") {
		t.Error("the grammar card is handed the whole selection instead of the active sentence")
	}
	card := readSourceFile(t, "grammar.go")
	if !containsText(card, `第 %d / %d 句`) {
		t.Error("the grammar card has no sentence pager")
	}
}
