package main

import (
	"path/filepath"
	"testing"
)

func TestApplyAIPresetPreservesKeyAcrossProviderRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	gemini, _ := aiPresetByID("gemini")
	magpie, _ := aiPresetByID("magpie")

	applyAIPreset(&cfg, gemini)
	cfg.AIKey = "sk-gemini"
	cfg.AIKeyEnc = "stored-gemini-key-blob"

	applyAIPreset(&cfg, magpie)
	if conn := cfg.AIConn(""); conn.Key != "" {
		t.Fatalf("Magpie must not carry the Gemini key: %q", conn.Key)
	}

	applyAIPreset(&cfg, gemini)
	if cfg.AIKey != "sk-gemini" {
		t.Fatalf("Gemini key lost after Gemini -> Magpie -> Gemini: %q", cfg.AIKey)
	}
	if conn := cfg.AIConn(""); conn.Key != "sk-gemini" {
		t.Fatalf("Gemini connection key = %q, want %q", conn.Key, "sk-gemini")
	}
}

func TestProviderKeySurvivesConfigReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	swapConfigPath(t, path)

	s := newStore()
	s.Update(func(c *Config) {
		p, _ := aiPresetByID("gemini")
		applyAIPreset(c, p)
		c.AIKey = "sk-gemini"
	})
	s.Update(func(c *Config) {
		p, _ := aiPresetByID("magpie")
		applyAIPreset(c, p)
	})

	reloaded := newStore()
	reloaded.Update(func(c *Config) {
		p, _ := aiPresetByID("gemini")
		applyAIPreset(c, p)
	})
	if got := reloaded.Get().AIKey; got != "sk-gemini" {
		t.Fatalf("Gemini key lost after config reload: %q", got)
	}
}
