package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestApplyAIPresetPreservesModelAcrossProviderRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	custom, _ := aiPresetByID("custom")
	magpie, _ := aiPresetByID("magpie")
	gemini, _ := aiPresetByID("gemini")

	applyAIPreset(&cfg, custom)
	cfg.AIURL = "https://my-gateway.example/v1"
	cfg.AIModel = "minimax-code/MiniMax-M3.1-Flash"

	applyAIPreset(&cfg, magpie)
	applyAIPreset(&cfg, gemini)
	applyAIPreset(&cfg, custom)

	if cfg.AIURL != "https://my-gateway.example/v1" {
		t.Fatalf("custom URL lost on return: %q", cfg.AIURL)
	}
	if cfg.AIModel != "minimax-code/MiniMax-M3.1-Flash" {
		t.Fatalf("custom model lost on return: %q", cfg.AIModel)
	}
}

func TestProviderSettingsSurviveConfigReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	swapConfigPath(t, path)
	const want = "minimax-code/MiniMax-M3.1-Flash"

	s := newStore()
	s.Update(func(c *Config) {
		p, _ := aiPresetByID("magpie")
		applyAIPreset(c, p)
		c.AIModel = want
	})
	s.Update(func(c *Config) {
		p, _ := aiPresetByID("gemini")
		applyAIPreset(c, p)
		c.AIKey = "test-key"
	})

	reloaded := newStore()
	reloaded.Update(func(c *Config) {
		p, _ := aiPresetByID("magpie")
		applyAIPreset(c, p)
	})
	if got := reloaded.Get().AIModel; got != want {
		t.Fatalf("Magpie model lost after config reload: %q", got)
	}
}

func TestMagpieModelSurvivesProviderSwitch(t *testing.T) {
	cfg := DefaultConfig()
	magpie, _ := aiPresetByID("magpie")
	gemini, _ := aiPresetByID("gemini")
	const want = "minimax-code/MiniMax-M3.1-Flash"

	applyAIPreset(&cfg, magpie)
	cfg.AIModel = want
	applyAIPreset(&cfg, gemini)
	applyAIPreset(&cfg, magpie)

	if cfg.AIModel != want {
		t.Fatalf("Magpie model reset to %q, want %q", cfg.AIModel, want)
	}
}

func TestMagpieModelListSurvivesProviderSwitch(t *testing.T) {
	a := newTestApp()
	full := make([]string, 26)
	for i := range full {
		full[i] = fmt.Sprintf("model-%02d", i)
	}
	var calls int32
	swapAIFetch(t, func(context.Context, aiConn) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return append([]string(nil), full...), nil
	})

	a.store.Update(func(c *Config) {
		p, _ := aiPresetByID("magpie")
		applyAIPreset(c, p)
	})
	a.loadAIModels()
	waitAIModelsSettled(t, a)

	a.store.Update(func(c *Config) {
		p, _ := aiPresetByID("gemini")
		applyAIPreset(c, p)
		c.AIKey = "test-key"
	})
	a.resetAIStatus()
	a.store.Update(func(c *Config) {
		p, _ := aiPresetByID("magpie")
		applyAIPreset(c, p)
	})
	a.resetAIStatus()

	models := a.getAIModels(a.store.Get())
	if len(models) != len(full) {
		t.Fatalf("Magpie full model list lost after provider switch: got %d, want %d: %v", len(models), len(full), models)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("switching back should reuse the cached list, got %d fetches", got)
	}
}

func TestConfiguredDirectProviderLoadsFullModelListOnSettingsOpen(t *testing.T) {
	a := newTestApp()
	full := make([]string, 64)
	for i := range full {
		full[i] = fmt.Sprintf("gemini-model-%02d", i)
	}
	var calls int32
	swapAIFetch(t, func(context.Context, aiConn) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return append([]string(nil), full...), nil
	})

	a.store.Update(func(c *Config) {
		p, _ := aiPresetByID("gemini")
		applyAIPreset(c, p)
		c.AIKey = "test-key"
	})
	a.loadAIModels()
	waitAIModelsSettled(t, a)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("settings open triggered %d fetches, want 1", got)
	}
	models := a.getAIModels(a.store.Get())
	if len(models) != len(full) {
		t.Fatalf("direct provider full model list = %d, want %d: %v", len(models), len(full), models)
	}
}
