package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRepairSplitLatinRestoresMicrosoftLetterSplit(t *testing.T) {
	const source = "I can't believe now I am counting on Huawei and Deepseek for the very survival of the OSS community. What a loss..."
	const mangled = "我简直不敢相信现在我竟然指望华为和Deepsee克来拯救OSS社区的生存。真是损失......"
	const want = "我简直不敢相信现在我竟然指望华为和Deepseek来拯救OSS社区的生存。真是损失......"

	if got := repairSplitLatin(source, mangled); got != want {
		t.Fatalf("repaired = %q\nwant      %q", got, want)
	}
}

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

func TestRepairSplitLatinNeedsAFourLetterRun(t *testing.T) {
	if got := repairSplitLatin("Two cats.", "两只cat猫。"); got != "两只cat猫。" {
		t.Fatalf("short run was repaired: %q", got)
	}
}

func TestRepairSplitLatinOnlySwapsOneLetter(t *testing.T) {
	if got := repairSplitLatin("Kubernetes runs it.", "Kube克在运行它。"); got != "Kube克在运行它。" {
		t.Fatalf("a five-letter loss was repaired: %q", got)
	}
	if got := repairSplitLatin("Two kittens.", "两只kitte猫。"); got != "两只kitte猫。" {
		t.Fatalf("a two-letter loss was repaired: %q", got)
	}
}

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
