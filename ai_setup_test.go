package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAISetupProblem(t *testing.T) {
	deepseek := func(mut func(*Config)) Config {
		c := DefaultConfig()
		p, _ := aiPresetByID("deepseek")
		applyAIPreset(&c, p)
		c.AIKey = "sk-test"
		if mut != nil {
			mut(&c)
		}
		return c
	}
	cases := []struct {
		name string
		cfg  Config
		want string // substring; "" means ready
	}{
		{"magpie default", DefaultConfig(), ""},
		{"disabled", func() Config { c := DefaultConfig(); c.AIEnabled = false; return c }(), "未启用"},
		{"deepseek ready", deepseek(nil), ""},
		{"deepseek no key", deepseek(func(c *Config) { c.AIKey = "" }), "API Key"},
		{"no model", deepseek(func(c *Config) { c.AIModel = " " }), "模型"},
		{"custom empty", func() Config { c := DefaultConfig(); c.AIProvider = "custom"; c.AIURL = ""; return c }(), "模型服务"},
		// 自定义常用来接 LM Studio 等本机无认证服务，设置页也写着 Key「可留空」。
		{"custom keyless local", func() Config {
			c := DefaultConfig()
			c.AIProvider, c.AIURL, c.AIModel, c.AIKey = "custom", "http://127.0.0.1:1234/v1", "local-model", ""
			return c
		}(), ""},
		{"ollama no key needed", func() Config {
			c := DefaultConfig()
			p, _ := aiPresetByID("ollama")
			applyAIPreset(&c, p)
			return c
		}(), ""},
	}
	for _, tc := range cases {
		got := tc.cfg.AISetupProblem()
		if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A fresh install must not assume Magpie: most people do not run it, and the
// notes extraction would then fire a doomed request on every selection.
func TestFirstRunAIIsUnconfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	swapConfigPath(t, path)

	s := newStore()

	cfg := s.Get()
	if cfg.IsMagpie() || cfg.AISetupProblem() == "" {
		t.Fatalf("first run should start unconfigured, got provider=%q url=%q problem=%q", cfg.AIProvider, cfg.AIURL, cfg.AISetupProblem())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk["ai_provider"] != "custom" {
		t.Fatalf("first-run file should record the unconfigured provider, got %v", onDisk["ai_provider"])
	}
}

// An existing Magpie user must keep working after upgrade.
func TestExistingMagpieConfigStaysMagpie(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"ai_enabled":true,"ai_url":"http://127.0.0.1:3425/v1/chat/completions","ai_model":"m"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	swapConfigPath(t, path)

	cfg := newStore().Get()
	if !cfg.IsMagpie() || cfg.AISetupProblem() != "" {
		t.Fatalf("existing Magpie config changed: provider=%q problem=%q", cfg.AIProvider, cfg.AISetupProblem())
	}
}

func unconfiguredApp() *app {
	a := newTestApp()
	a.store.Update(func(c *Config) {
		p, _ := aiPresetByID("deepseek")
		applyAIPreset(c, p)
		c.AIKey = ""
	})
	return a
}

func TestRefineWithAIExplainsMissingSetup(t *testing.T) {
	a := unconfiguredApp()
	a.source = "hello world"
	a.refineWithAI()
	if a.aiRefining || !strings.Contains(a.aiRefineErr, "API Key") {
		t.Fatalf("refine should stop and name the missing key: refining=%v err=%q", a.aiRefining, a.aiRefineErr)
	}
}

func TestNotesSkipWhenAINotReady(t *testing.T) {
	a := unconfiguredApp()
	a.store.Update(func(c *Config) { c.ShowNotes = true })
	a.extractNotesForGen(a.notesGen, "The quick brown fox jumps over the lazy dog again")
	if a.notesBusy {
		t.Fatal("notes extraction started a request without an API key")
	}
}

func TestSwitchModeFallsBackWhenAINotReady(t *testing.T) {
	a := unconfiguredApp()
	a.switchMode(modeGrammar)
	if a.mode == modeGrammar || !strings.Contains(a.notice, "API Key") {
		t.Fatalf("AI tab opened without setup: mode=%v notice=%q", a.mode, a.notice)
	}
}

func TestSettingsAIPageNamesMissingSetup(t *testing.T) {
	a := unconfiguredApp()
	a.page = "ai"
	tt := ui.NewTester(a.settingsView, 880, 640)
	if !tt.HasText("还差一步：还没填 API Key") {
		t.Fatalf("settings page does not say what is missing: %v", tt.Texts())
	}
}
