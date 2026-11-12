package main

import (
	"context"
	"errors"
	"os"
	"strings"
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
	out, err := StreamAI(ctx, cfg.AIConn(cfg.AIModel), refineSystemPrompt,
		"The quick brown fox jumps over the lazy dog", cfg.AIReasoningEffortFor(cfg.AIModel), func(chunk string) {
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

// TestLiveDirectProvider runs one real model list and one real refine
// through the direct (non-Magpie) client path. Skipped unless
// FASTDICT_LIVE_AI_PROVIDER is set:
//
//	FASTDICT_LIVE_AI_PROVIDER  preset ID, e.g. deepseek or custom
//	FASTDICT_LIVE_AI_KEY_FILE  file holding the API key (never on the command line)
//	FASTDICT_LIVE_AI_URL       optional address override (needed for custom)
//	FASTDICT_LIVE_AI_MODEL     optional model override
//
// Without a key file a placeholder key is sent, which only works against a
// service that ignores keys, such as the local Magpie gateway reached through
// 自定义. The wrong-key check runs only with a real key.
func TestLiveDirectProvider(t *testing.T) {
	providerID := os.Getenv("FASTDICT_LIVE_AI_PROVIDER")
	if providerID == "" {
		t.Skip("set FASTDICT_LIVE_AI_PROVIDER to run")
	}
	preset, ok := aiPresetByID(providerID)
	if !ok || preset.Magpie {
		t.Fatalf("%q is not a direct preset", providerID)
	}
	cfg := DefaultConfig()
	applyAIPreset(&cfg, preset)
	if u := os.Getenv("FASTDICT_LIVE_AI_URL"); u != "" {
		cfg.AIURL = u
	}
	if m := os.Getenv("FASTDICT_LIVE_AI_MODEL"); m != "" {
		cfg.AIModel = m
	}
	realKey := false
	if keyFile := os.Getenv("FASTDICT_LIVE_AI_KEY_FILE"); keyFile != "" {
		key, err := os.ReadFile(keyFile)
		if err != nil {
			t.Fatalf("read key file: %v", err)
		}
		cfg.AIKey = strings.TrimSpace(string(key))
		realKey = true
	} else if preset.NeedsKey {
		cfg.AIKey = "sk-placeholder-for-keyless-gateway"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	models, err := FetchAvailableModels(ctx, cfg.AIConn(""))
	switch {
	case errors.Is(err, errAIModelsUnsupported):
		t.Logf("%s: /models not supported", preset.Name)
	case err != nil:
		t.Fatalf("%s models: %v", preset.Name, err)
	default:
		t.Logf("%s: %d models, first %v", preset.Name, len(models), models[:min(5, len(models))])
	}
	if cfg.AIModel == "" {
		if len(models) == 0 {
			t.Fatal("preset has no default model and the list is empty")
		}
		cfg.AIModel = models[0]
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()
	start := time.Now()
	deltas := 0
	out, err := StreamAI(ctx2, cfg.AIConn(cfg.AIModel), refineSystemPrompt,
		"The quick brown fox jumps over the lazy dog", cfg.AIReasoningEffortFor(cfg.AIModel),
		func(string) { deltas++ })
	if err != nil {
		t.Fatalf("%s refine with %s: %v", preset.Name, cfg.AIModel, err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("empty answer")
	}
	t.Logf("%s %s in %v (%d deltas): %s", preset.Name, cfg.AIModel, time.Since(start), deltas, truncateFor(out, 60))

	if !realKey {
		return
	}
	// A wrong key must come back as the typed auth failure, not a raw 401.
	bad := cfg
	bad.AIKey = "sk-invalid-fastdict-probe"
	_, err = StreamAI(ctx2, bad.AIConn(cfg.AIModel), refineSystemPrompt, "hi", "", nil)
	var failure *aiFailure
	if !errors.As(err, &failure) || failure.kind != aiFailureAuth && failure.kind != aiFailurePermission {
		t.Errorf("bad key should map to auth failure, got %v", err)
	}
}

// TestLiveProvidersRejectBadKey sends a made-up key to every keyed preset and
// checks the real answer is classified as an auth failure with a usable
// message. No real key needed. Skipped unless FASTDICT_LIVE_AI_BADKEY=1.
func TestLiveProvidersRejectBadKey(t *testing.T) {
	if os.Getenv("FASTDICT_LIVE_AI_BADKEY") != "1" {
		t.Skip("set FASTDICT_LIVE_AI_BADKEY=1 to run")
	}
	for _, p := range aiPresets {
		if !p.NeedsKey || p.BaseURL == "" {
			continue
		}
		t.Run(p.ID, func(t *testing.T) {
			cfg := DefaultConfig()
			applyAIPreset(&cfg, p)
			cfg.AIKey = "sk-fastdict-invalid-probe-0000000000"
			model := cfg.AIModel
			if model == "" {
				model = "probe-model"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, chatErr := StreamAI(ctx, cfg.AIConn(model), "s", "hi", "", nil)
			_, listErr := FetchAvailableModels(ctx, cfg.AIConn(""))
			for name, err := range map[string]error{"chat": chatErr, "models": listErr} {
				var failure *aiFailure
				switch {
				case err == nil && name == "models":
					// OpenRouter 的 /models 公开，不校验 Key：列表拉得到不代表 Key 对。
					t.Logf("models: public list, key not checked")
				case errors.Is(err, errAIModelsUnsupported):
					t.Logf("%s: /models unsupported", name)
				case !errors.As(err, &failure):
					t.Errorf("%s: untyped error %v", name, err)
				case failure.kind == aiFailureNetwork || failure.kind == aiFailureTimeout:
					t.Logf("%s: unreachable from here (%s)", name, failure.msg)
				case failure.kind == aiFailureAuth || failure.kind == aiFailurePermission:
					t.Logf("%s: ok %s", name, failure.msg)
				default:
					t.Errorf("%s: bad key classified as kind %d: %s", name, failure.kind, failure.msg)
				}
			}
		})
	}
}
