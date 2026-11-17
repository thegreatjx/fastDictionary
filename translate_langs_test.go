package main

import (
	"strings"
	"testing"
)

func TestTranslateLangsFallsBackToTheSecondLanguage(t *testing.T) {
	base := Config{TargetLang: "zh-Hans", SecondLang: "en"}

	cases := []struct {
		name     string
		cfg      Config
		text     string
		wantFrom string
		wantTo   string
	}{
		{
			name:     "中文输入、目标中文时译到第二语言",
			cfg:      base,
			text:     "处女作",
			wantFrom: "zh-Hans",
			wantTo:   "en",
		},
		{
			name:     "中英混排、英文术语居多时仍译到第二语言",
			cfg:      base,
			text:     "Claude 这个 Building Always on Agents，实际是 Personal Agent 的最基本的设计模式。",
			wantFrom: "zh-Hans",
			wantTo:   "en",
		},
		{
			name:     "英文句子、目标中文时照常译成中文",
			cfg:      base,
			text:     "debut work",
			wantFrom: "",
			wantTo:   "zh-Hans",
		},
		{
			name:     "第二语言为空时给出可用方向",
			cfg:      Config{TargetLang: "zh-Hans"},
			text:     "处女作",
			wantFrom: "zh-Hans",
			wantTo:   "en",
		},
		{
			name:     "第二语言等于原文时也给出可用方向",
			cfg:      Config{TargetLang: "zh-Hans", SecondLang: "zh-Hant"},
			text:     "处女作",
			wantFrom: "zh-Hans",
			wantTo:   "en",
		},
		{
			name:     "目标语言为空时按简体中文处理",
			cfg:      Config{SecondLang: "en"},
			text:     "debut work",
			wantFrom: "",
			wantTo:   "zh-Hans",
		},
		{
			name:     "检测不出原文语言时保留目标语言，由引擎自己判定",
			cfg:      base,
			text:     "où est la gare",
			wantFrom: "",
			wantTo:   "zh-Hans",
		},
	}

	for _, tc := range cases {
		from, to := translateLangs(tc.cfg, tc.text)
		if from != tc.wantFrom || to != tc.wantTo {
			t.Errorf("%s：translateLangs(%q) = (%q, %q)，期望 (%q, %q)",
				tc.name, tc.text, from, to, tc.wantFrom, tc.wantTo)
		}
	}
}

func TestRefineEchoedInput(t *testing.T) {
	cases := []struct {
		source string
		out    string
		want   bool
	}{
		{"处女作", "处女作", true},
		{"处女作", "  处女作 \n", true},
		{"debut work", "Debut  Work", true},
		{"处女作", "debut work", false},
		{"debut work", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		if got := refineEchoedInput(tc.source, tc.out); got != tc.want {
			t.Errorf("refineEchoedInput(%q, %q) = %v，期望 %v", tc.source, tc.out, got, tc.want)
		}
	}
}

func TestRefineUsesTheResolvedTargetLanguage(t *testing.T) {
	for _, f := range []string{"home.go", "popup_actions.go"} {
		src := readSourceFile(t, f)
		if !containsText(src, "translateLangs(cfg, source)") {
			t.Errorf("%s 的精翻没有用回退后的语言对", f)
		}
		if !containsText(src, "langPromptName(refineTo)") {
			t.Errorf("%s 的精翻提示词没有用回退后的目标语言", f)
		}
		if !containsText(src, "refineEchoedInput(") {
			t.Errorf("%s 的精翻没有拦住「模型原样返回原文」", f)
		}
		if strings.Contains(src, "langPromptName(cfg.TargetLang)") {
			t.Errorf("%s 仍在用 cfg.TargetLang 直接写精翻提示词", f)
		}
	}
}
