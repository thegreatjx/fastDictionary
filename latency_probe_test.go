package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDetectLocal(t *testing.T) {
	cases := map[string]string{
		"hello world": "en",
		"你好世界":        "zh-CN",
		"こんにちは":       "ja",
		"안녕하세요":       "ko",
	}
	for text, want := range cases {
		if got := detectLocal(text); got != want {
			t.Errorf("detectLocal(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestIsWord(t *testing.T) {
	if !isWord("serendipity") || !isWord("hello-world") || isWord("hello world") || isWord("你好") {
		t.Error("isWord misclassified")
	}
}

func TestSameLang(t *testing.T) {
	if !sameLang("zh-CN", "zh-Hans") || sameLang("en", "zh-Hans") {
		t.Error("sameLang wrong")
	}
}

// TestLatencyProbe measures the hot path against the live endpoints.
func TestLatencyProbe(t *testing.T) {
	texts := []string{"hello world", "你好，世界", "serendipity"}
	for _, text := range texts {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		from := detectLocal(text)
		to := "zh-Hans"
		if strings.HasPrefix(from, "zh") {
			to = "en"
		}
		r := TranslateRace(ctx, text, from, to, "race")
		total := time.Since(start)
		if r.Err != nil {
			t.Logf("TEXT=%q RACE FAIL=%v total=%v", text, r.Err, total)
		} else {
			t.Logf("TEXT=%q -> %q engine=%s TOTAL=%v", text, r.Text, r.Engine, total)
		}
		cancel()
	}
}
