package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveRaceIncludesGoogle is the end-to-end check for the user's "谷歌翻译
// 又是400" report. It is skipped in a normal run: it needs the network, and the
// race picks whichever engine is fastest, so it cannot assert on the winner.
// Run it with FASTDICTIONARY_LIVE=1 to see which engines answered.
func TestLiveRaceIncludesGoogle(t *testing.T) {
	if os.Getenv("FASTDICTIONARY_LIVE") == "" {
		t.Skip("set FASTDICTIONARY_LIVE=1 to hit the live engines")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// detectLocal returns "" for this sentence, which is exactly the case that
	// used to make Google answer 400.
	const sentence = "She cooked the books to hide the losses."
	if from := detectLocal(sentence); from != "" {
		t.Logf("note: local detection now returns %q, so this no longer reproduces the empty-sl path", from)
	}

	start := time.Now()
	r := TranslateRace(ctx, sentence, "", "zh-Hans", "race")
	if r.Err != nil {
		t.Fatalf("race failed: %v", r.Err)
	}
	t.Logf("race in %v via %s (%s): %s", time.Since(start), r.Engine, r.Detected, r.Text)

	if r.Engine == "微软" {
		t.Log("Microsoft won on speed; that is a result, not a failure")
	}

	// Google pinned must also work on its own, which is the direct assertion:
	// before the sl=auto fix this returned "google: http 400".
	gctx, gcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer gcancel()
	out, detected, err := GoogleTranslate(gctx, sentence, "", "zh-Hans")
	if err != nil {
		t.Fatalf("pinned google still fails: %v", err)
	}
	t.Logf("google (%s): %s", detected, out)

	tctx, tcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer tcancel()
	tres, terr := tencentTranslate(tctx, sentence, "en", "zh-Hans")
	if terr != nil {
		t.Logf("tencent unavailable right now: %v", terr)
	} else {
		t.Logf("tencent (%s): %s", tres.Detected, tres.Text)
	}
}
