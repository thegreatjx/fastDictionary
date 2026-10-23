package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
)

// Config is everything the user can change. Zero value is not valid; use
// DefaultConfig and fill missing fields on load.

// CustomActionConfig models Read-Frog's customizable AI actions
type CustomActionConfig struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	Icon         string   `json:"icon"`
	ProviderID   string   `json:"provider_id"`
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt"`
	Prompt       string   `json:"prompt"`
	OutputSchema []string `json:"output_schema"`
}

type Config struct {
	// shortcuts
	HotkeySelection  string `json:"hotkey_selection"`
	HotkeyInput      string `json:"hotkey_input"`
	HotkeyScreenshot string `json:"hotkey_screenshot"`

	// translation
	EngineMode    string `json:"engine_mode"` // race | microsoft | google
	TargetLang    string `json:"target_lang"` // zh-Hans | en | ja ...
	SecondLang    string `json:"second_lang"` // when source == target
	ShowEngineTag bool   `json:"show_engine_tag"`

	// selection behaviour
	// SelectionTrigger decides what a text selection does by itself:
	// "icon" shows the small floating chip, "auto" translates at once,
	// "hotkey" waits for the shortcut.
	SelectionTrigger string `json:"selection_trigger"`
	// SelectionCapture prefers UI Automation over a simulated copy.
	SelectionCapture string `json:"selection_capture"` // auto | copy | uia
	// SelectionBlocklist names the executables where a mouse drag is the
	// application's own gesture, never a text selection. Screenshot tools
	// live here: their snip region is a drag, and reacting to it made
	// fastpot tear Snipaste's capture down.
	SelectionBlocklist []string `json:"selection_blocklist"`

	// dictionary and audio
	DictEnabled   bool   `json:"dict_enabled"`
	AutoPronounce string `json:"auto_pronounce"` // off | uk | us
	AudioVolume   int    `json:"audio_volume"`

	// AI refinement via the local magpie gateway
	AIEnabled bool   `json:"ai_enabled"`
	AIURL     string `json:"ai_url"`
	AIModel   string `json:"ai_model"`
	AIEffort  string `json:"ai_effort"` // low | high

	// window behaviour
	CloseOnBlur    bool    `json:"close_on_blur"`
	AlwaysOnTop    bool    `json:"always_on_top"`
	PopupWidth     float32 `json:"popup_width"`
	ThemeMode      string  `json:"theme_mode"` // system | light | dark
	HistoryEnabled bool    `json:"history_enabled"`
	// ShowNotes is the "猜你想存" card: whether the popup offers to save the
	// words an AI found in the selected sentence.
	ShowNotes     bool                 `json:"show_notes"`
	WindowOpacity float64              `json:"window_opacity"` // 0.5 to 1.0
	AIMode        string               `json:"ai_mode"`        // "translate" | "grammar" | "dict" | "polish"
	CustomActions []CustomActionConfig `json:"custom_actions"`

	// Onboarded records that the first run has happened, so the settings
	// window is only opened automatically once.
	Onboarded bool `json:"onboarded"`
}

// DefaultConfig returns the shipped defaults.
func DefaultConfig() Config {
	return Config{
		HotkeySelection:    "Alt+D",
		HotkeyInput:        "Alt+Q",
		HotkeyScreenshot:   "Alt+C",
		EngineMode:         "race",
		TargetLang:         "zh-Hans",
		SecondLang:         "en",
		ShowEngineTag:      true,
		SelectionTrigger:   "icon",
		SelectionCapture:   "auto",
		SelectionBlocklist: DefaultSelectionBlocklist(),
		DictEnabled:        true,
		AutoPronounce:      "off",
		AudioVolume:        80,
		AIEnabled:          true,
		AIURL:              "http://127.0.0.1:3425/v1/chat/completions",
		AIModel:            "antigravity/gemini-3.8-flash",
		AIEffort:           "none",
		CloseOnBlur:        true,
		AlwaysOnTop:        true,
		PopupWidth:         460,
		ThemeMode:          "system",
		HistoryEnabled:     true,
		ShowNotes:          true,
		WindowOpacity:      0.95,
		AIMode:             "grammar",
		CustomActions:      DefaultCustomActions(),
	}
}

// newMemoryStore returns a store with no file behind it: tests use it so
// they neither read nor write the user's real config.
func newMemoryStore() *store {
	return &store{cfg: DefaultConfig()}
}

// store holds the live config plus its file path, guarded for concurrent use.
type store struct {
	mu   sync.RWMutex
	cfg  Config
	path string
}

func newStore() *store {
	s := &store{cfg: DefaultConfig()}
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
		s.path = filepath.Join(dir, "config.json")
	} else {
		log.Printf("fastpot: no user data dir: %v", err)
	}
	if !s.load() && s.path != "" {
		// First run: materialise the defaults so the file is there to edit.
		if err := saveConfig(s.path, s.cfg); err != nil {
			log.Printf("fastpot: could not write default config: %v", err)
		}
	}
	log.Printf("fastpot: config path %s", s.path)
	return s
}

// Get returns a copy of the current config, safe to read from any goroutine.
func (s *store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Update mutates the config on a copy and persists it.
func (s *store) Update(fn func(*Config)) Config {
	s.mu.Lock()
	fn(&s.cfg)
	cfg := s.cfg
	path := s.path
	s.mu.Unlock()
	if path != "" {
		if err := saveConfig(path, cfg); err != nil {
			log.Printf("fastpot: config save failed: %v", err)
		}
	}
	return cfg
}

// load reads the config file, filling anything missing with the default.
// It reports whether a file was found.
func (s *store) load() bool {
	if s.path == "" {
		return false
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return false // first run
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("fastpot: config parse failed, using defaults: %v", err)
		return false
	}
	ensureDefaultActions(&cfg)
	if cfg.PopupWidth < 360 {
		cfg.PopupWidth = 360
	}
	if cfg.WindowOpacity <= 0 || cfg.WindowOpacity > 1 {
		cfg.WindowOpacity = 0.95
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return true
}

func saveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DefaultCustomActions() []CustomActionConfig {
	return []CustomActionConfig{
		{
			ID:           "default-dictionary",
			Name:         "词典",
			Enabled:      true,
			Icon:         "dict",
			ProviderID:   "magpie",
			Model:        "",
			SystemPrompt: learnerDictSystemPrompt,
			Prompt:       "## 输入\n选中文本：{{selection}}\n段落内容：{{paragraphs}}\n目标语言：{{targetLanguage}}",
			OutputSchema: []string{"term", "phonetic", "partOfSpeech", "definition", "context", "contextTerm", "contextTranslation", "memoryTips", "difficulty"},
		},
		{
			ID:           "default-sentence-analysis",
			Name:         "长难句分析",
			Enabled:      true,
			Icon:         "grammar",
			ProviderID:   "magpie",
			Model:        "",
			SystemPrompt: grammarSystemPrompt,
			Prompt:       "## Input\nSelection: {{selection}}\nParagraphs: {{paragraphs}}\nTarget language: {{targetLanguage}}",
			OutputSchema: []string{"annotations", "translation"},
		},
		{
			ID:           "default-improve-writing",
			Name:         "写作改进",
			Enabled:      true,
			Icon:         "polish",
			ProviderID:   "magpie",
			Model:        "",
			SystemPrompt: polishSystemPrompt,
			Prompt:       "## Input\nSelection: {{selection}}\nParagraphs: {{paragraphs}}\nTarget language: {{targetLanguage}}",
			OutputSchema: []string{"setting", "annotations", "improved", "summary"},
		},
	}
}

// GetAction finds an action by ID
func (c Config) GetAction(id string) *CustomActionConfig {
	for i := range c.CustomActions {
		if c.CustomActions[i].ID == id {
			return &c.CustomActions[i]
		}
	}
	return nil
}

// GetEnabledActions returns only active custom actions
func (c Config) GetEnabledActions() []CustomActionConfig {
	var list []CustomActionConfig
	for _, a := range c.CustomActions {
		if a.Enabled {
			list = append(list, a)
		}
	}
	return list
}

// ResetAction restores a built-in action to default values
func (c *Config) ResetAction(id string) bool {
	defaults := DefaultCustomActions()
	for _, def := range defaults {
		if def.ID == id {
			for i := range c.CustomActions {
				if c.CustomActions[i].ID == id {
					c.CustomActions[i] = def
					return true
				}
			}
			c.CustomActions = append(c.CustomActions, def)
			return true
		}
	}
	return false
}

func ensureDefaultActions(cfg *Config) {
	defaults := DefaultCustomActions()
	if len(cfg.CustomActions) == 0 {
		cfg.CustomActions = defaults
		return
	}
	existingMap := make(map[string]int)
	for i, a := range cfg.CustomActions {
		existingMap[a.ID] = i
	}
	for _, def := range defaults {
		if idx, exists := existingMap[def.ID]; !exists {
			cfg.CustomActions = append(cfg.CustomActions, def)
		} else {
			// Backfill missing critical prompt fields if empty
			if cfg.CustomActions[idx].SystemPrompt == "" {
				cfg.CustomActions[idx].SystemPrompt = def.SystemPrompt
			}
			if cfg.CustomActions[idx].Prompt == "" {
				cfg.CustomActions[idx].Prompt = def.Prompt
			}
			if len(cfg.CustomActions[idx].OutputSchema) == 0 {
				cfg.CustomActions[idx].OutputSchema = def.OutputSchema
			}
		}
	}
}

// activeStore is the config store the audio path reads. It is set once in
// main so playback can honour the volume setting without threading the app
// through every helper.
var activeStore *store

// GetConfigVolume returns the configured playback volume (0-100).
func GetConfigVolume() int {
	if activeStore == nil {
		return 80
	}
	return activeStore.Get().AudioVolume
}
