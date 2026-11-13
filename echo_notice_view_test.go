package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestEchoedTranslationRendersAsNotice pins the card the reader complained
// about: 微软 answered "fastDictionary" with the word itself, and the card
// showed a red failure plus a 重试 button that reproduced the same text. The
// echo is now an explanation, not an error.
func TestEchoedTranslationRendersAsNotice(t *testing.T) {
	a := newTestApp()
	a.mode = modeTranslate
	a.source = "fastDictionary"
	a.engine = "微软"
	a.echoNote = "微软没有给出译文，返回的内容与原文相同：多半是专有名词或简称，也可能这组语言它不翻译。切到其它引擎可以拿到意译。"

	tt := ui.NewTester(a.popupView, 500, 620)
	joined := strings.Join(tt.Texts(), "|")
	if !strings.Contains(joined, "没有给出译文") {
		t.Fatalf("the echo notice is missing: %q", joined)
	}
	if !strings.Contains(joined, "微软") {
		t.Fatalf("the notice does not name the engine: %q", joined)
	}
	if strings.Contains(joined, "重试") {
		t.Fatalf("an echo must not offer a retry that returns the same text: %q", joined)
	}
}

// TestTransportFailureStillRendersAsError keeps the distinction the fix
// introduced: a request that actually failed keeps the red box and its retry,
// because retrying can succeed there.
func TestTransportFailureStillRendersAsError(t *testing.T) {
	a := newTestApp()
	a.mode = modeTranslate
	a.source = "hello"
	a.errText = "all engines failed: microsoft: http 429"

	tt := ui.NewTester(a.popupView, 500, 620)
	joined := strings.Join(tt.Texts(), "|")
	if !strings.Contains(joined, "http 429") {
		t.Fatalf("the failure reason is missing: %q", joined)
	}
	if !strings.Contains(joined, "重试") {
		t.Fatalf("a real failure must keep its retry: %q", joined)
	}
	if strings.Contains(joined, "没有给出译文") {
		t.Fatalf("a transport failure was explained as an echo: %q", joined)
	}
}
