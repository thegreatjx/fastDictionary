package main

import (
	"context"
	"testing"
)

func TestPinnedTencentReportsItsName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := TranslateRace(ctx, "hello", "en", "zh", "tencent")
	if r.Engine != "腾讯" {
		t.Errorf("pinned tencent reported engine %q, want 腾讯", r.Engine)
	}
}

func TestEngineKeyNamesTencent(t *testing.T) {
	for display, want := range map[string]string{
		"微软": "microsoft",
		"谷歌": "google",
		"腾讯": "tencent",
	} {
		if got := engineKey(display); got != want {
			t.Errorf("engineKey(%q) = %q, want %q", display, got, want)
		}
	}
}
