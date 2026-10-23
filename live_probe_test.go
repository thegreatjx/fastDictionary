package main

import (
	"context"
	"testing"
	"time"
)

func TestLiveYoudaoDict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	d, err := LookupYoudao(ctx, "serendipity")
	if err != nil {
		t.Skipf("youdao unreachable: %v", err)
	}
	t.Logf("dict in %v: uk=/%s/ us=/%s/ explains=%v", time.Since(start), d.UKPhone, d.USPhone, d.Explains)
	if d.UKPhone == "" && d.USPhone == "" {
		t.Error("no phonetics")
	}
}

func TestLiveYoudaoAudio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := time.Now()
	path, err := FetchYoudaoAudio(ctx, "serendipity", true)
	if err != nil {
		t.Skipf("audio unreachable: %v", err)
	}
	t.Logf("audio %s fetched in %v", path, time.Since(start))
}

func TestLiveMagpieStream(t *testing.T) {
	cfg := DefaultConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	deltas := 0
	firstAt := 0
	out, err := StreamMagpie(ctx, cfg.AIURL, cfg.AIModel, refineSystemPrompt,
		"The quick brown fox jumps over the lazy dog", cfg.AIEffort, func(chunk string) {
			deltas++
			if deltas == 1 {
				firstAt = int(time.Since(start).Milliseconds())
			}
		})
	if err != nil {
		t.Skipf("magpie gateway not running: %v", err)
	}
	t.Logf("magpie in %v (first chunk %dms, %d deltas): %s", time.Since(start), firstAt, deltas, truncateFor(out, 60))
}
