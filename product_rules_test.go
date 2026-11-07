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
		{errors.New("HTTP 429: too many requests"), "请求过于频繁（429），请稍后再试。"},
		{errors.New("401 unauthorized: api key missing"), "API Key 无效或未配置，请检查 AI 网关设置。"},
		{errors.New("dial tcp 127.0.0.1:1: connection refused"), "无法连接 AI 网关，请检查网关地址是否已启动。"},
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
