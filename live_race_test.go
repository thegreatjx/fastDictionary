package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveRaceIncludesGoogle(t *testing.T) {
	if os.Getenv("FASTDICTIONARY_LIVE") == "" {
		t.Skip("set FASTDICTIONARY_LIVE=1 to hit the live engines")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

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
