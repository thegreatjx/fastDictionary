package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestResolveEffortSendsWhatIsFastForTheModelFamily covers the reason 快速 must
// consult a measured table instead of each model's lowest declared level. The
// gateway drops a level the model does not declare, but a level it *does*
// declare still turns reasoning on: measured on the same sentence, deepseek
// answers in 1.2s with no reasoning field and 38s with "low", while gemini is
// the other way round (10.5s with no field, 2.9s with "low").
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
		// Measured families win over their declared levels.
		{"workbuddy-ai/deepseek-v4.1-flash", ""},
		{"auto/gemini-3.8-flash", "low"},
		// Unknown families fall back to the lowest declared level.
		{"some/only-slow-model", "high"},
		{"some/unlisted-model", "minimal"},
	}
	for _, c := range cases {
		if got := ResolveEffort(EffortFast, c.model); got != c.want {
			t.Errorf("ResolveEffort(fast, %q) = %q, want %q", c.model, got, c.want)
		}
	}

	// With no levels reported at all an unknown model must still send nothing
	// rather than guess: a level the gateway drops leaves the default in place.
	if got := ResolveEffort(EffortFast, "model/nobody-listed"); got != "" {
		t.Errorf("ResolveEffort(fast, unknown) = %q, want empty", got)
	}
}

// TestResolveEffortDeepAlwaysAsksForHigh pins the other direction: 深度思考
// means high everywhere, and must not depend on what the gateway reported.
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

// TestFetchAvailableModelsRecordsReasoningLevels is the wiring test: the levels
// ResolveEffort needs only exist in the /v1/models response, so the fetch must
// keep them and not just the ids.
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

	ids, err := FetchAvailableModels(srv.URL)
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

// TestLegacyEffortLabelsAreMigrated covers configs written before the option
// was renamed. Without the migration an old file decodes into a value the
// segmented control cannot show, so the page renders with nothing selected.
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
