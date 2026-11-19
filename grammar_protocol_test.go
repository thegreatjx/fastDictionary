package main

import (
	"strings"
	"testing"
)

func TestParseGrammarToleratesReorderedSegments(t *testing.T) {
	const source = "She carefully opened a sealed letter before the meeting began."
	const raw = `{
	  "segments": [
	    {"text":"She","role":"subject","is_backbone":true,"clause_id":"c0"},
	    {"text":" before the meeting","role":"connector","clause_id":"c1"},
	    {"text":" began","role":"predicate"},
	    {"text":" carefully opened","role":"predicate","is_backbone":true},
	    {"text":" a sealed letter","role":"object","is_backbone":true}
	  ],
	  "clauses": [
	    {"id":"c0","type":"主句","short_tag":"主从"},
	    {"id":"c1","type":"时间状语从句","short_tag":"状从","target":"修饰 opened"}
	  ],
	  "translation":"她在开会前小心地拆开了一封密封的信。"
	}`

	got := ParseSentenceAnalysisDetailed(raw, source)
	if got.Outcome != parseOutcomeStructured {
		t.Fatalf("outcome = %v, want structured; cause=%v", got.Outcome, got.Cause)
	}
	if got.Result == nil {
		t.Fatal("structured parse returned no result")
	}
	if concatSegments(got.Result.Segments) != source {
		t.Fatalf("segments restore %q, want %q", concatSegments(got.Result.Segments), source)
	}
	if len(got.Result.Clauses) != 1 || got.Result.Clauses[0].ID != "c1" {
		t.Fatalf("fabricated main clause was not dropped: %+v", got.Result.Clauses)
	}
	var clauseIDs []string
	for _, seg := range got.Result.Segments {
		if seg.ClauseID == "c0" {
			t.Errorf("segment %q still points at the dropped main clause", seg.Text)
		}
		clauseIDs = append(clauseIDs, seg.ClauseID)
	}
	if len(clauseIDs) < 2 || clauseIDs[len(clauseIDs)-2] != "c1" || clauseIDs[len(clauseIDs)-1] != "c1" {
		t.Errorf("open clause did not continue to the sentence end: %v", clauseIDs)
	}
	if got.Result.Segments[0].Text != "She" && got.Result.Segments[0].Text != "She " {
		t.Errorf("first segment = %q, want the source's first phrase", got.Result.Segments[0].Text)
	}
	if role := got.Result.Segments[1].Role; role != RolePredicate {
		t.Errorf("reordered predicate role = %q, want predicate", role)
	}
}

func TestParseGrammarRejectsRewrittenSource(t *testing.T) {
	const source = "The quick brown fox jumps over the lazy dog."
	const raw = `{"segments":[{"text":"A fast dark wolf leaps above a sleepy canine.","role":"subject"}],"translation":"一只敏捷的黑狼越过一只困倦的犬。"}`

	got := ParseSentenceAnalysisDetailed(raw, source)
	if got.Outcome != parseOutcomeFallbackDirect {
		t.Fatalf("outcome = %v, want fallback_direct", got.Outcome)
	}
	if got.Result == nil || len(got.Result.Segments) != 0 {
		t.Fatalf("fallback kept model segments: %+v", got.Result)
	}
	if got.Result.Translation == "" {
		t.Error("fallback dropped the translation")
	}
}

func setGrammarAction(cfg *Config, prompt string, schema []string) *CustomActionConfig {
	cfg.CustomActions = DefaultCustomActions()
	for i := range cfg.CustomActions {
		if cfg.CustomActions[i].ID != "default-sentence-analysis" {
			continue
		}
		cfg.CustomActions[i].SystemPrompt = prompt
		cfg.CustomActions[i].OutputSchema = schema
		return &cfg.CustomActions[i]
	}
	return nil
}

func grammarAction(cfg *Config) *CustomActionConfig {
	for i := range cfg.CustomActions {
		if cfg.CustomActions[i].ID == "default-sentence-analysis" {
			return &cfg.CustomActions[i]
		}
	}
	return nil
}

func TestEnsureDefaultActionsMigratesLegacyGrammarPrompts(t *testing.T) {
	legacyAnnotations := `请输出严格 JSON：{"annotations":[{"text":"x","type":"subject"}],"translation":"译文"}`
	cases := []struct {
		name   string
		prompt string
		schema []string
	}{
		{"V2 default", grammarSystemPromptV2, []string{"annotations", "translation"}},
		{"V3 default", grammarSystemPromptV3, []string{"segments", "clauses", "translation"}},
		{"V4 default", grammarSystemPromptV4, []string{"segments", "clauses", "difficulties", "translation"}},
		{"legacy annotations", legacyAnnotations, []string{"annotations", "translation"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			action := setGrammarAction(&cfg, tc.prompt, tc.schema)
			if action == nil {
				t.Fatal("default-sentence-analysis missing")
			}
			if !ensureDefaultActions(&cfg) {
				t.Fatal("legacy grammar prompt was not migrated")
			}
			action = grammarAction(&cfg)
			if action.SystemPrompt != grammarSystemPromptV5 {
				t.Fatalf("prompt was not upgraded to V5 (len=%d)", len(action.SystemPrompt))
			}
			if !strings.Contains(action.SystemPrompt, `"segments"`) || !strings.Contains(action.SystemPrompt, `"phrases"`) || strings.Contains(action.SystemPrompt, `"annotations"`) {
				t.Fatalf("migrated prompt does not describe the V5 protocol")
			}
			if !sameStrings(action.OutputSchema, []string{"segments", "clauses", "phrases", "difficulties", "translation"}) {
				t.Fatalf("output schema = %v, want the V5 fields", action.OutputSchema)
			}
			if ensureDefaultActions(&cfg) {
				t.Fatal("migration is not idempotent; the config would be rewritten on every start")
			}
		})
	}
}

func TestEnsureDefaultActionsPreservesEditedGrammarPrompt(t *testing.T) {
	const edited = `我的自定义分段指令：只输出 {"segments":[],"translation":""}`
	cfg := DefaultConfig()
	setGrammarAction(&cfg, edited, []string{"segments", "translation"})
	if ensureDefaultActions(&cfg) {
		t.Fatal("an edited segments prompt must not be rewritten")
	}
	if got := grammarAction(&cfg).SystemPrompt; got != edited {
		t.Fatalf("edited prompt changed to %q", got)
	}
}
