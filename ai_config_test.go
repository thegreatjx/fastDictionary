package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIConnOmitsKeyForNoKeyServices(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AIKey = "sk-test"
	for _, provider := range []string{"magpie", "ollama"} {
		cfg.AIProvider = provider
		if conn := cfg.AIConn("m"); conn.Key != "" {
			t.Fatalf("%s conn carried a key: %q", provider, conn.Key)
		}
	}
	cfg.AIProvider = "deepseek"
	if conn := cfg.AIConn("m"); conn.Key != "sk-test" {
		t.Fatalf("deepseek conn lost its key: %q", conn.Key)
	}
}

func TestNormalizeAIProviderConfig(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"old custom address", `{"ai_url":"https://api.deepseek.com/v1"}`, "custom"},
		{"old magpie address", `{"ai_url":"http://127.0.0.1:3425/v1/chat/completions"}`, "magpie"},
		{"old empty address", `{}`, "magpie"},
		{"explicit custom", `{"ai_provider":"custom","ai_url":"https://example.com/v1"}`, "custom"},
		{"unknown provider", `{"ai_provider":"something-new","ai_url":"https://example.com/v1"}`, "custom"},
		{"explicit magpie", `{"ai_provider":"magpie","ai_url":"https://example.com/v1"}`, "magpie"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			normalizeAIProviderConfig(&cfg, []byte(tc.raw))
			if cfg.AIProvider != tc.want {
				t.Fatalf("AIProvider = %q, want %q", cfg.AIProvider, tc.want)
			}
		})
	}
}

func TestApplyAIPresetKeepsOtherProviderKeySeparate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AIProvider = "custom"
	cfg.AIURL = "https://a.example/v1"
	cfg.AIModel = "model-a"
	cfg.AIKey = "sk-a"
	deepseek, _ := aiPresetByID("deepseek")
	custom, _ := aiPresetByID("custom")

	if !applyAIPreset(&cfg, deepseek) {
		t.Fatal("a keyed provider without a saved key should ask for one")
	}
	if cfg.AIProvider != "deepseek" || cfg.AIURL != deepseek.BaseURL || cfg.AIModel != deepseek.Model {
		t.Fatalf("preset was not applied: %+v", cfg)
	}
	if cfg.AIKey != "" || cfg.AIKeyEnc != "" {
		t.Fatalf("custom key leaked into deepseek: key=%q enc=%q", cfg.AIKey, cfg.AIKeyEnc)
	}
	applyAIPreset(&cfg, custom)
	if cfg.AIKey != "sk-a" {
		t.Fatalf("custom key lost on return: %q", cfg.AIKey)
	}
}

func TestApplyAIPresetNoKeyServiceDoesNotLeakOrForgetKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AIProvider = "custom"
	cfg.AIURL = "http://127.0.0.1:11434/v1"
	cfg.AIKey = "sk-a"
	ollama, _ := aiPresetByID("ollama")
	custom, _ := aiPresetByID("custom")

	if applyAIPreset(&cfg, ollama) {
		t.Fatal("a no-key service should not ask for a key")
	}
	if cfg.AIKey != "" || cfg.AIConn("").Key != "" {
		t.Fatalf("key leaked into no-key service: key=%q conn=%q", cfg.AIKey, cfg.AIConn("").Key)
	}
	applyAIPreset(&cfg, custom)
	if cfg.AIKey != "sk-a" {
		t.Fatalf("key lost after returning from no-key service: %q", cfg.AIKey)
	}
}

func TestApplyAIPresetCustomFromMagpieClearsDefaults(t *testing.T) {
	cfg := DefaultConfig()
	custom, _ := aiPresetByID("custom")
	applyAIPreset(&cfg, custom)
	if cfg.AIURL != "" || cfg.AIModel != "" {
		t.Fatalf("Magpie defaults leaked into custom: url=%q model=%q", cfg.AIURL, cfg.AIModel)
	}
}

func TestLegacyAIKeyEncMigratesToProviderMap(t *testing.T) {
	enc, err := protectSecret("sk-legacy")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	raw := map[string]any{
		"ai_enabled":  true,
		"ai_provider": "gemini",
		"ai_url":      "https://generativelanguage.googleapis.com/v1beta/openai/",
		"ai_model":    "gemini-3.5-flash-lite",
		"ai_key_enc":  enc,
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	swapConfigPath(t, path)

	cfg := newStore().Get()
	if cfg.AIKey != "sk-legacy" {
		t.Fatalf("legacy key = %q, want sk-legacy", cfg.AIKey)
	}
	if cfg.AIKeyEncByProvider["gemini"] != enc {
		t.Fatal("legacy ai_key_enc was not migrated into the provider map")
	}
}

func TestAIURLHostChangeForgetsCurrentProviderKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	swapConfigPath(t, path)

	s := newStore()
	s.Update(func(c *Config) {
		p, _ := aiPresetByID("gemini")
		applyAIPreset(c, p)
		c.AIKey = "sk-gemini"
	})
	s.Update(func(c *Config) {
		if endpointHost(c.AIURL) != "" {
			c.AIURL = "https://other.example/v1"
			c.AIKey = ""
			c.AIKeyEnc = ""
		}
	})

	if got := newStore().Get().AIKey; got != "" {
		t.Fatalf("key survived a host change: %q", got)
	}
}

func TestConfigSealsAIKeyAndLoadsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	swapConfigPath(t, path)

	s := newStore()
	s.Update(func(c *Config) {
		c.AIProvider = "custom"
		c.AIURL = "https://example.com/v1"
		c.AIModel = "model-a"
		c.AIKey = "sk-secret"
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sk-secret") {
		t.Fatalf("plaintext key found in config: %s", data)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["ai_key_enc"] == "" || raw["ai_key_enc"] == nil {
		t.Fatal("encrypted key was not written")
	}
	if _, ok := raw["ai_key_enc_by_provider"]; !ok {
		t.Fatal("per-provider encrypted key map was not written")
	}
	if _, ok := raw["ai_key"]; ok {
		t.Fatal("ai_key must not be written")
	}

	loaded := newStore()
	if got := loaded.Get().AIKey; got != "sk-secret" {
		t.Fatalf("loaded key = %q, want sk-secret", got)
	}
}
