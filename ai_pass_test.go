package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestClassifyAIResult(t *testing.T) {
	cases := []struct {
		name  string
		snap  snapshot
		phase aiPassPhase
		text  string
	}{
		{name: "empty result", snap: snapshot{mode: modeDictionary}, phase: aiPassIdle, text: "idle"},
		{name: "streaming with partial JSON", snap: snapshot{mode: modeDictionary, aiBusy: true, aiText: `{"term":"seize"`}, phase: aiPassStreaming, text: "streaming"},
		{name: "busy wins over empty text", snap: snapshot{mode: modeGrammar, aiBusy: true}, phase: aiPassStreaming, text: "streaming"},
		{name: "prose answer", snap: snapshot{mode: modeDictionary, aiText: "seize 表示突然用力抓住。"}, phase: aiPassProse, text: "prose"},
		{name: "unparsed JSON", snap: snapshot{mode: modeDictionary, aiText: `{"term": "seize", "phonetic": "/siː`}, phase: aiPassParseFailed, text: "parse_failed"},
		{name: "fenced block", snap: snapshot{mode: modeWriting, aiText: "```json\n{}\n```"}, phase: aiPassParseFailed, text: "parse_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyAIResult(tc.snap)
			if got != tc.phase {
				t.Fatalf("phase = %s, want %s", got, tc.phase)
			}
			if got.String() != tc.text {
				t.Fatalf("String() = %q, want %q", got.String(), tc.text)
			}
		})
	}
}

func TestAIResultStateRenderingBaseline(t *testing.T) {
	cases := []struct {
		phase  aiPassPhase
		snap   snapshot
		want   []string
		absent []string
	}{
		{
			phase:  aiPassIdle,
			snap:   snapshot{mode: modeDictionary},
			absent: []string{"正在生成词条与例句…", "这次没能解析成结构化卡片"},
		},
		{
			phase:  aiPassStreaming,
			snap:   snapshot{mode: modeDictionary, aiBusy: true, aiText: `{"term":"seize"`},
			want:   []string{"正在生成词条与例句…"},
			absent: []string{"term", "seize", "这次没能解析成结构化卡片"},
		},
		{
			phase:  aiPassProse,
			snap:   snapshot{mode: modeDictionary, aiText: "seize 表示突然用力抓住。"},
			want:   []string{"seize 表示突然用力抓住。"},
			absent: []string{"这次没能解析成结构化卡片"},
		},
		{
			phase:  aiPassParseFailed,
			snap:   snapshot{mode: modeDictionary, aiText: `{"term": "seize", "phonetic": "/siː`},
			want:   []string{"这次没能解析成结构化卡片", "重新翻译"},
			absent: []string{"phonetic"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.phase.String(), func(t *testing.T) {
			if got := classifyAIResult(tc.snap); got != tc.phase {
				t.Fatalf("phase = %s, want %s", got, tc.phase)
			}
			a := newTestApp()
			a.mode = tc.snap.mode
			a.aiBusy = tc.snap.aiBusy
			a.aiText = tc.snap.aiText
			tt := ui.NewTester(a.popupView, 500, 620)
			joined := strings.Join(tt.Texts(), "|")
			for _, s := range tc.want {
				if !strings.Contains(joined, s) {
					t.Errorf("missing %q in %q", s, joined)
				}
			}
			for _, s := range tc.absent {
				if strings.Contains(joined, s) {
					t.Errorf("unexpected %q in %q", s, joined)
				}
			}
		})
	}
}

func TestAICardDispatchEquivalence(t *testing.T) {
	cases := []struct {
		name   string
		aiBusy bool
		aiText string
		want   bool
	}{
		{name: "idle", aiBusy: false, aiText: "", want: false},
		{name: "prose text", aiBusy: false, aiText: "seize 表示突然用力抓住。", want: true},
		{name: "busy without text", aiBusy: true, aiText: "", want: true},
		{name: "busy with partial JSON", aiBusy: true, aiText: `{"term":"seize"`, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := snapshot{mode: modeDictionary, aiBusy: tc.aiBusy, aiText: tc.aiText}
			legacy := s.aiBusy || s.aiText != ""
			explicit := classifyAIResult(s) != aiPassIdle
			if legacy != tc.want || explicit != tc.want {
				t.Fatalf("legacy=%v explicit=%v want=%v", legacy, explicit, tc.want)
			}
		})
	}
}

func TestAIModeDispatchRenderingBaseline(t *testing.T) {
	cases := []struct {
		name   string
		mode   popupMode
		aiBusy bool
		want   string
		absent string
	}{
		{name: "grammar streaming", mode: modeGrammar, aiBusy: true, want: aiBusyMessage(modeGrammar), absent: "键盘输入长句"},
		{name: "grammar idle", mode: modeGrammar, want: "键盘输入长句", absent: aiBusyMessage(modeGrammar)},
		{name: "dictionary streaming", mode: modeDictionary, aiBusy: true, want: aiBusyMessage(modeDictionary), absent: "键盘输入查词"},
		{name: "dictionary idle", mode: modeDictionary, want: "键盘输入查词", absent: aiBusyMessage(modeDictionary)},
		{name: "writing streaming", mode: modeWriting, aiBusy: true, want: aiBusyMessage(modeWriting), absent: "键盘输入句子"},
		{name: "writing idle", mode: modeWriting, want: "键盘输入句子", absent: aiBusyMessage(modeWriting)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp()
			a.mode = tc.mode
			a.aiBusy = tc.aiBusy
			tt := ui.NewTester(a.popupView, 500, 620)
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
