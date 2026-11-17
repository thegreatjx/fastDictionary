package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestClassifyAIViewEquivalence(t *testing.T) {
	grammarCard := &SentenceAnalysisResult{Translation: "她递给你一瓶。"}
	dictCard := &DictionaryResult{Term: "seize", Definition: "夺取"}
	writingCard := &ImproveWritingResult{Improved: "I really like this."}

	cases := []struct {
		name string
		snap snapshot
		want aiPassPhase
	}{
		{name: "grammar card", snap: snapshot{mode: modeGrammar, grammarResult: grammarCard}, want: aiPassStructured},
		{name: "grammar card wins over busy", snap: snapshot{mode: modeGrammar, grammarResult: grammarCard, aiBusy: true, aiText: `{"term":`}, want: aiPassStructured},
		{name: "grammar ignores dictionary card", snap: snapshot{mode: modeGrammar, dictResult: dictCard}, want: aiPassIdle},
		{name: "dictionary card", snap: snapshot{mode: modeDictionary, dictResult: dictCard}, want: aiPassStructured},
		{name: "dictionary card wins over prose", snap: snapshot{mode: modeDictionary, dictResult: dictCard, aiText: "seize 表示抓住。"}, want: aiPassStructured},
		{name: "dictionary ignores grammar card", snap: snapshot{mode: modeDictionary, grammarResult: grammarCard}, want: aiPassIdle},
		{name: "writing card", snap: snapshot{mode: modeWriting, writingResult: writingCard}, want: aiPassStructured},
		{name: "writing ignores dictionary card", snap: snapshot{mode: modeWriting, dictResult: dictCard}, want: aiPassIdle},
		{name: "grammar streaming", snap: snapshot{mode: modeGrammar, aiBusy: true}, want: aiPassStreaming},
		{name: "dictionary prose", snap: snapshot{mode: modeDictionary, aiText: "seize 表示抓住。"}, want: aiPassProse},
		{name: "writing parse failed", snap: snapshot{mode: modeWriting, aiText: `{"term":`}, want: aiPassParseFailed},
		{name: "grammar empty", snap: snapshot{mode: modeGrammar}, want: aiPassIdle},
		{name: "non-AI mode falls back to result phase", snap: snapshot{mode: modeTranslate, aiBusy: true}, want: aiPassStreaming},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyAIView(tc.snap); got != tc.want {
				t.Fatalf("classifyAIView = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClassifyAIViewFallsBackToResult(t *testing.T) {
	snaps := []snapshot{
		{mode: modeGrammar},
		{mode: modeDictionary, aiBusy: true},
		{mode: modeWriting, aiText: "seize 表示抓住。"},
		{mode: modeGrammar, aiText: `{"term":`},
		{mode: modeTranslate, aiBusy: true},
	}
	for i, s := range snaps {
		if got, want := classifyAIView(s), classifyAIResult(s); got != want {
			t.Errorf("case %d: classifyAIView = %s, classifyAIResult = %s", i, got, want)
		}
	}
}

func TestAIViewRenderingBaseline(t *testing.T) {
	cases := []struct {
		name   string
		mode   popupMode
		setup  func(a *app)
		want   string
		absent string
	}{
		{
			name: "grammar card", mode: modeGrammar,
			setup: func(a *app) { a.grammarResult = &SentenceAnalysisResult{Translation: "她递给你一瓶。"} },
			want:  "她递给你一瓶。", absent: "键盘输入长句",
		},
		{
			name: "grammar pass", mode: modeGrammar,
			setup: func(a *app) { a.aiBusy = true },
			want:  aiBusyMessage(modeGrammar), absent: "键盘输入长句",
		},
		{
			name: "grammar empty", mode: modeGrammar,
			want: "键盘输入长句", absent: aiBusyMessage(modeGrammar),
		},
		{
			name: "dictionary card", mode: modeDictionary,
			setup: func(a *app) { a.dictResult = &DictionaryResult{Term: "seize", Definition: "夺取"} },
			want:  "seize", absent: "键盘输入查词",
		},
		{
			name: "dictionary pass", mode: modeDictionary,
			setup: func(a *app) { a.aiBusy = true },
			want:  aiBusyMessage(modeDictionary), absent: "键盘输入查词",
		},
		{
			name: "dictionary empty", mode: modeDictionary,
			want: "键盘输入查词", absent: aiBusyMessage(modeDictionary),
		},
		{
			name: "writing card", mode: modeWriting,
			setup: func(a *app) {
				a.writingResult = &ImproveWritingResult{
					Setting: "正式 · 邮件",
					Annotations: []WritingAnnotation{
						{Text: "I very like", Fix: "I really like", Type: "grammar", Tag: "副词位置", Note: "very不能直接修饰动词"},
					},
					Improved: "I really like this.",
					Summary:  "注意副词修饰动词的位置。",
				}
			},
			want: "I really like this.", absent: "键盘输入句子",
		},
		{
			name: "writing pass", mode: modeWriting,
			setup: func(a *app) { a.aiBusy = true },
			want:  aiBusyMessage(modeWriting), absent: "键盘输入句子",
		},
		{
			name: "writing empty", mode: modeWriting,
			want: "键盘输入句子", absent: aiBusyMessage(modeWriting),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp()
			a.mode = tc.mode
			if tc.setup != nil {
				tc.setup(a)
			}
			tt := ui.NewTester(a.popupView, 500, 700)
			joined := strings.Join(tt.Texts(), "|")
			if !strings.Contains(joined, tc.want) {
				t.Errorf("missing %q in %q", tc.want, joined)
			}
			if strings.Contains(joined, tc.absent) {
				t.Errorf("unexpected %q in %q", tc.absent, joined)
			}
		})
	}
}
