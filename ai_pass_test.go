package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestClassifyAIResult 是 aiPassPhase 的规格：输入是结果区的真实组合，
// 期望值是每种组合在界面上应有的名字。它锁定「什么输入算什么状态」，
// 不照抄 renderAIResultState 的实现。
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

// TestAIResultStateRenderingBaseline 把每种 phase 对应的界面输出录下来。
// 阶段 A 第 2 步把 renderAIResultState 改成按 aiPassPhase 分支后，这张表
// 必须逐字不变；它是「零行为变化」的判据，不是实现说明。
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

// TestAICardDispatchEquivalence 锁定批次 2 要替换的布尔表达式。
// popupView 的三个 AI mode 分支原本用 s.aiBusy || s.aiText != "" 决定走
// 结果区还是空态；切到 classifyAIResult(s) != aiPassIdle 后，所有
// (aiBusy, aiText) 组合必须给出相同判定。这是「零行为变化」的直接证据。
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

// TestAIModeDispatchRenderingBaseline 录下三个 AI mode 在两种分发结果下
// 各自画出的区块。空态按钮文案是各 mode 独有的，不会与顶部标签混淆。
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
