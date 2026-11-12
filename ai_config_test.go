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

func TestApplyAIPresetClearsKeyOnHostChange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AIProvider = "custom"
	cfg.AIURL = "https://a.example/v1"
	cfg.AIModel = "model-a"
	cfg.AIKey = "sk-a"
	deepseek, _ := aiPresetByID("deepseek")

	if !applyAIPreset(&cfg, deepseek) {
		t.Fatal("switching to another host should report the cleared key")
	}
	if cfg.AIProvider != "deepseek" || cfg.AIURL != deepseek.BaseURL || cfg.AIModel != deepseek.Model {
		t.Fatalf("preset was not applied: %+v", cfg)
	}
	if cfg.AIKey != "" || cfg.AIKeyEnc != "" {
		t.Fatalf("key survived a host change: key=%q enc=%q", cfg.AIKey, cfg.AIKeyEnc)
	}
}

func TestApplyAIPresetKeepsKeyOnSameHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AIProvider = "custom"
	cfg.AIURL = "https://a.example/v1"
	cfg.AIModel = "model-a"
	cfg.AIKey = "sk-a"
	other := aiPreset{ID: "other", Name: "Other", BaseURL: "https://a.example/v1", Model: "model-b", NeedsKey: true}

	if applyAIPreset(&cfg, other) {
		t.Fatal("same-host switch should not clear the key")
	}
	if cfg.AIKey != "sk-a" {
		t.Fatalf("key = %q, want sk-a", cfg.AIKey)
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

func TestApplyAIPresetNoKeyServiceClearsKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AIProvider = "custom"
	cfg.AIURL = "http://127.0.0.1:11434/v1"
	cfg.AIKey = "sk-a"
	ollama, _ := aiPresetByID("ollama")
	if !applyAIPreset(&cfg, ollama) {
		t.Fatal("switching to a no-key service should report the cleared key")
	}
	if cfg.AIKey != "" {
		t.Fatalf("key = %q, want empty", cfg.AIKey)
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
	if _, ok := raw["ai_key"]; ok {
		t.Fatal("ai_key must not be written")
	}

	loaded := newStore()
	if got := loaded.Get().AIKey; got != "sk-secret" {
		t.Fatalf("loaded key = %q, want sk-secret", got)
	}
}
