package main

import (
	"context"
	"testing"
)

// TestPinnedTencentReportsItsName keeps the engine tag honest: the popup shows
// which engine answered, so a 腾讯 answer labelled 微软 would be a lie.
func TestPinnedTencentReportsItsName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // fail fast, do not depend on the network
	r := TranslateRace(ctx, "hello", "en", "zh", "tencent")
	if r.Engine != "腾讯" {
		t.Errorf("pinned tencent reported engine %q, want 腾讯", r.Engine)
	}
}

// TestEngineKeyNamesTencent keeps the circuit breaker per engine: a shared key
// would let a Tencent outage suspend Microsoft.
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
