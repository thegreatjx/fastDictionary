package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestRepairSplitLatinRestoresMicrosoftLetterSplit is the regression test for
// the sentence a reader reported: 微软 answered "Deepsee克" for "Deepseek" and
// won the race, so the popup showed a Chinese character inside a brand name.
// Both strings are verbatim: the source is what the reader selected, mangled is
// the body edge.microsoft.com/translate/translatetext returned on 2026-11-14.
func TestRepairSplitLatinRestoresMicrosoftLetterSplit(t *testing.T) {
	const source = "I can't believe now I am counting on Huawei and Deepseek for the very survival of the OSS community. What a loss..."
	const mangled = "我简直不敢相信现在我竟然指望华为和Deepsee克来拯救OSS社区的生存。真是损失......"
	const want = "我简直不敢相信现在我竟然指望华为和Deepseek来拯救OSS社区的生存。真是损失......"

	if got := repairSplitLatin(source, mangled); got != want {
		t.Fatalf("repaired = %q\nwant      %q", got, want)
	}
}

// TestRepairSplitLatinKeepsGoodTranslations guards the other side: 腾讯 answers
// this exact sentence correctly, and a repair must not touch it.
func TestRepairSplitLatinKeepsGoodTranslations(t *testing.T) {
	const source = "I can't believe now I am counting on Huawei and Deepseek for the very survival of the OSS community."
	cases := []struct{ name, translated string }{
		{"腾讯's correct output", "我不敢相信现在我指望华为和Deepseek来维持OSS社区的生存。"},
		{"the word appears whole later", "华为和Deepsee克都靠Deepseek活着。"},
		{"no Han character after the run", "华为和Deepsee"},
		{"run too short to be a split", "猫cat猫"},
		{"plain Chinese", "我不敢相信。"},
		{"empty", ""},
	}
	for _, c := range cases {
		if got := repairSplitLatin(source, c.translated); got != c.translated {
			t.Errorf("%s: rewritten to %q, want it untouched", c.name, got)
		}
	}
}

// TestRepairSplitLatinNeedsAFourLetterRun pins the size guard that keeps the
// repair away from ordinary words: "cats" behind "cat猫" is a plausible real
// translation, and dropping the 猫 to restore "cats" would be a worse trade.
func TestRepairSplitLatinNeedsAFourLetterRun(t *testing.T) {
	if got := repairSplitLatin("Two cats.", "两只cat猫。"); got != "两只cat猫。" {
		t.Fatalf("short run was repaired: %q", got)
	}
}

// TestRepairSplitLatinOnlySwapsOneLetter: the engine swallowed one letter into
// one Han character. A two-letter tail behind one Han character is a different
// defect, and "repairing" it would delete a character the engine did translate.
func TestRepairSplitLatinOnlySwapsOneLetter(t *testing.T) {
	if got := repairSplitLatin("Kubernetes runs it.", "Kube克在运行它。"); got != "Kube克在运行它。" {
		t.Fatalf("a five-letter loss was repaired: %q", got)
	}
	if got := repairSplitLatin("Two kittens.", "两只kitte猫。"); got != "两只kitte猫。" {
		t.Fatalf("a two-letter loss was repaired: %q", got)
	}
}

// TestLiveMicrosoftSplitIsRepaired is the end-to-end check behind the report:
// the real engine, then the real repair. Skipped by default because it needs
// the network, and because the engine may fix its tokenizer at any time — when
// that happens this test should say so in its log, not fail the build.
func TestLiveMicrosoftSplitIsRepaired(t *testing.T) {
	if os.Getenv("FASTDICTIONARY_LIVE") == "" {
		t.Skip("set FASTDICTIONARY_LIVE=1 to hit the live engine")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	const source = "I can't believe now I am counting on Huawei and Deepseek for the very survival of the OSS community."
	r, err := MicrosoftTranslate(ctx, source, "en", "zh-Hans")
	if err != nil {
		t.Skipf("microsoft unreachable: %v", err)
	}
	fixed := repairSplitLatin(source, r.Text)
	t.Logf("engine       : %s", r.Text)
	t.Logf("after repair : %s", fixed)
	if !strings.Contains(fixed, "Deepseek") {
		t.Errorf("the brand name is still mangled: %q", fixed)
	}
}
