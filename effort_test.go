package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveEffortSendsWhatIsFastForTheModelFamily(t *testing.T) {
	modelEfforts.Lock()
	modelEfforts.m = map[string][]string{
		"workbuddy-ai/deepseek-v4.1-flash": {"low", "high", "max"},
		"auto/gemini-3.8-flash":            {"low", "medium", "high"},
		"some/only-slow-model":             {"high", "max"},
		"some/unlisted-model":              {"minimal", "low", "medium", "high"},
	}
	modelEfforts.Unlock()

	cases := []struct{ model, want string }{
		{"workbuddy-ai/deepseek-v4.1-flash", ""},
		{"auto/gemini-3.8-flash", "low"},
		{"some/only-slow-model", "high"},
		{"some/unlisted-model", "minimal"},
	}
	for _, c := range cases {
		if got := ResolveEffort(EffortFast, c.model); got != c.want {
			t.Errorf("ResolveEffort(fast, %q) = %q, want %q", c.model, got, c.want)
		}
	}

	if got := ResolveEffort(EffortFast, "model/nobody-listed"); got != "" {
		t.Errorf("ResolveEffort(fast, unknown) = %q, want empty", got)
	}
}

func TestResolveEffortDeepAlwaysAsksForHigh(t *testing.T) {
	modelEfforts.Lock()
	modelEfforts.m = map[string][]string{"m": {"minimal", "low", "medium", "high"}}
	modelEfforts.Unlock()

	for _, model := range []string{"m", "workbuddy-ai/deepseek-v4.1-flash", "model/nobody-listed"} {
		if got := ResolveEffort(EffortDeep, model); got != "high" {
			t.Errorf("ResolveEffort(deep, %q) = %q, want high", model, got)
		}
	}
}

func TestFetchAvailableModelsRecordsReasoningLevels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "auto/gemini-3.8-flash", "reasoning_levels": []string{"low", "medium", "high"}},
				{"id": "plain/model"},
			},
		})
	}))
	defer srv.Close()

	ids, err := FetchAvailableModels(context.Background(), aiConn{
		Provider: "magpie",
		Endpoint: srv.URL,
		Model:    "test",
	})
	if err != nil {
		t.Fatalf("FetchAvailableModels: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d ids, want 2: %v", len(ids), ids)
	}
	if got := ResolveEffort(EffortFast, "auto/gemini-3.8-flash"); got != "low" {
		t.Errorf("gemini lowest = %q, want low", got)
	}
	if got := ResolveEffort(EffortFast, "plain/model"); got != "" {
		t.Errorf("model without levels = %q, want empty", got)
	}
}

func TestLegacyEffortLabelsAreMigrated(t *testing.T) {
	cases := map[string]string{
		"":     EffortFast,
		"none": EffortFast,
		"low":  EffortFast,
		"off":  EffortFast,
		"high": EffortDeep,
		"fast": EffortFast,
		"deep": EffortDeep,
	}
	for in, want := range cases {
		cfg := DefaultConfig()
		cfg.AIEffort = in
		normalizeAIEffort(&cfg)
		if cfg.AIEffort != want {
			t.Errorf("normalizeAIEffort(ai_effort=%q) = %q, want %q", in, cfg.AIEffort, want)
		}
	}
}
