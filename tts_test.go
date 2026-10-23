package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestEdgeTTSSignature pins the signature format: appid::base64::date::id,
// with every component lowercased before signing.
func TestEdgeTTSSignature(t *testing.T) {
	now := time.Date(2026, 10, 18, 6, 30, 0, 0, time.UTC)
	sig, err := edgeTTSSignature(now, "abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	parts := splitSignature(sig)
	if len(parts) != 4 {
		t.Fatalf("signature has %d parts, want 4: %q", len(parts), sig)
	}
	if parts[0] != edgeTTSAppID {
		t.Errorf("appid = %q, want %q", parts[0], edgeTTSAppID)
	}
	if parts[2] != "sun, 18 oct 2026 06:30:00 GMT" {
		t.Errorf("date = %q", parts[2])
	}
	if parts[3] != "abcdef0123456789" {
		t.Errorf("request id = %q", parts[3])
	}
	// The signature is deterministic for the same inputs.
	again, _ := edgeTTSSignature(now, "abcdef0123456789")
	if again != sig {
		t.Errorf("signature is not deterministic")
	}
}

func splitSignature(s string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		if i+1 < len(s) && s[i] == ':' && s[i+1] == ':' {
			out = append(out, cur)
			cur = ""
			i++
			continue
		}
		cur += string(s[i])
	}
	out = append(out, cur)
	return out
}

// TestEdgeTTSVoiceMapping keeps the voice choice honest.
func TestEdgeTTSVoiceMapping(t *testing.T) {
	cases := []struct {
		lang string
		us   bool
		want string
	}{
		{"en", true, "en-US-AriaNeural"},
		{"en", false, "en-GB-SoniaNeural"},
		{"zh-CN", false, "zh-CN-XiaoxiaoNeural"},
		{"ja", false, "ja-JP-NanamiNeural"},
	}
	for _, c := range cases {
		if got := edgeTTSVoice(c.lang, c.us, ""); got != c.want {
			t.Errorf("edgeTTSVoice(%q,%v) = %q, want %q", c.lang, c.us, got, c.want)
		}
	}
	if got := edgeTTSVoice("en", true, "en-US-GuyNeural"); got != "en-US-GuyNeural" {
		t.Errorf("override ignored: %q", got)
	}
}

// TestEdgeTTSSSML checks the escaping and locale derivation.
func TestEdgeTTSSSML(t *testing.T) {
	got := edgeTTSSSML("a & b < c", "en-US-AriaNeural")
	if !contains(got, "xml:lang=\"en-US\"") {
		t.Errorf("locale missing: %s", got)
	}
	if !contains(got, "a &amp; b &lt; c") {
		t.Errorf("text not escaped: %s", got)
	}
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}

// TestEdgeTTSLive hits the real service. It only runs with FASTPOT_LIVE=1
// so the offline suite stays offline.
func TestEdgeTTSLive(t *testing.T) {
	if os.Getenv("FASTPOT_LIVE") != "1" {
		t.Skip("set FASTPOT_LIVE=1 to run the live TTS probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	path, err := FetchEdgeTTS(ctx, "Serendipity means finding something good without looking for it.", "en-US-AriaNeural")
	if err != nil {
		t.Fatalf("FetchEdgeTTS: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Size() < 5000 {
		t.Fatalf("audio too small: %d bytes", fi.Size())
	}
	t.Logf("synthesised %d bytes in %v -> %s", fi.Size(), time.Since(start), path)
}
