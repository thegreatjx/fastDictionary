package main

import (
	"context"
	"errors"
	"testing"
)

func TestShouldExtractNotes(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"hello", false},                  // a single dictionary word is handled by the word card
		{"hello world", false},            // two words is a phrase, not a sentence
		{"this is a sentence", true},      // four English words
		{"今天天气真的非常好", true},               // eight CJK characters
		{"今天很好", false},                   // too short
		{"hello-world", false},            // still one token
		{"a quick brown fox jumps", true}, // five English words
	}
	for _, tc := range cases {
		if got := shouldExtractNotes(tc.text); got != tc.want {
			t.Errorf("shouldExtractNotes(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestSplitOcrParagraphsKeepsBlankLineBlocks(t *testing.T) {
	got := splitOcrParagraphs("First line\nsecond line\n\nSecond paragraph\n\nThird")
	want := []string{"First line", "second line", "Second paragraph", "Third"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("paragraph %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFriendlyAIError(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "网络超时，请稍后重试。"},
		{&aiFailure{kind: aiFailureRateLimit}, "请求过于频繁或额度用完，请稍后再试。"},
		{&aiFailure{kind: aiFailureBadRequest}, "服务拒绝了请求：当前模型可能不支持图片输入，或请求格式不兼容；请检查模型 ID 和服务商文档。"},
		{&aiFailure{kind: aiFailureAuth}, "API Key 无效或已过期，请在 设置 → AI 精翻 里重新填写。"},
		{&aiFailure{kind: aiFailureNetwork, msg: "x"}, "无法连接 AI 服务，请检查地址、网络或代理。"},
		{&aiFailure{kind: aiFailureMagpieDown}, "连不上本机 Magpie，请在设置里换一个服务商，或先启动 Magpie。"},
		// 不再按字符串猜：没分类的错误原样显示，不冒充成别的原因。
		{errors.New("HTTP 429: too many requests"), "HTTP 429: too many requests"},
	}
	for _, tc := range cases {
		if got := friendlyAIError(tc.err); got != tc.want {
			t.Errorf("friendlyAIError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestTabStateSaveRestore(t *testing.T) {
	a := &app{
		mode:   modeTranslate,
		source: "source text",
		target: "translated text",
		engine: "谷歌",
		notes:  []NoteItem{{Word: "example"}},
	}
	a.saveTabLocked()
	a.target = "other tab"
	a.engine = "微软"
	a.notes = nil
	a.restoreTabLocked(a.tabStates[modeTranslate])
	if a.target != "translated text" || a.engine != "谷歌" {
		t.Fatalf("restored target=%q engine=%q", a.target, a.engine)
	}
	if len(a.notes) != 1 || a.notes[0].Word != "example" {
		t.Fatalf("restored notes = %#v", a.notes)
	}
	if !a.tabStates[modeTranslate].hasContent() {
		t.Fatal("saved tab should report content")
	}
}
