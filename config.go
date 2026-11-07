package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	// OCRLang is the language the screenshot OCR reads with, as a BCP-47 tag:
	// "auto" takes whatever OCR language Windows puts first for the user,
	// "zh-Hans-CN" and "en-US" force one. It matters because the engine reads
	// an English screenshot with the Chinese model otherwise, which is where
	// the wrong glyphs came from.
	OCRLang string `json:"ocr_lang"`

	// translation
	// EngineMode picks how the text is translated: "race" asks 微软, 谷歌 and
	// 腾讯 at once and keeps the best answer inside a short grace window; the
	// other values pin one engine. All three are keyless, so a fresh install
	// translates without any setup — that was the deciding argument against
	// keeping 百度 and DeepL: they cost the reader a signup before the first
	// word came back, and both were behind a quota that runs out.
	EngineMode    string `json:"engine_mode"` // race | microsoft | google | tencent
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
	// fastDictionary tear Snipaste's capture down.
	SelectionBlocklist []string `json:"selection_blocklist"`

	// dictionary and audio
	DictEnabled   bool   `json:"dict_enabled"`
	AutoPronounce string `json:"auto_pronounce"` // off | uk | us
	AudioVolume   int    `json:"audio_volume"`

	// AI refinement via the local magpie gateway
	AIEnabled bool   `json:"ai_enabled"`
	AIURL     string `json:"ai_url"`
	AIModel   string `json:"ai_model"`
	AIEffort  string `json:"ai_effort"` // fast | deep

	// window behaviour
	CloseOnBlur    bool    `json:"close_on_blur"`
	AlwaysOnTop    bool    `json:"always_on_top"`
	PopupWidth     float32 `json:"popup_width"`
	ThemeMode      string  `json:"theme_mode"` // system | light | dark
	HistoryEnabled bool    `json:"history_enabled"`
	// ShowNotes is the "猜你想存" card: whether the popup offers to save the
	// words an AI found in the selected sentence.
	ShowNotes bool `json:"show_notes"`
	// DismissedNoteWords remembers recommendations the user rejected with
	// "不再推荐此词". It is a preference, not per-card state, and survives
	// restarts like the rest of the config.
	DismissedNoteWords []string             `json:"dismissed_note_words"`
	WindowOpacity      float64              `json:"window_opacity"` // 0.5 to 1.0
	AIMode             string               `json:"ai_mode"`        // "translate" | "grammar" | "dict" | "polish"
	CustomActions      []CustomActionConfig `json:"custom_actions"`
	// NotesMigrated marks a config file written after 猜你想存 was turned back
	// on by default. Its absence is how a file that predates the change is
	// recognised; without it, flipping the default would do nothing for
	// anyone already running the app, because their file spells out
	// show_notes: false and that value wins over the default.
	NotesMigrated int `json:"notes_migrated"`

	// AIModeIsChoice records that the user actually picked a 默认模式, as
	// opposed to inheriting the shipped default. Without it, turning the
	// preference on would silently reroute every existing installation:
	// their stored ai_mode is "grammar" simply because that is the default,
	// not because anyone asked for 长难句分析 to open on every selection.
	AIModeIsChoice bool `json:"ai_mode_is_choice"`

	// Onboarded records that the first run has happened, so the settings
	// window is only opened automatically once.
	Onboarded bool `json:"onboarded"`
}

// NoteWordDismissed reports whether the user asked the recommendation card to
// stop suggesting this word. Matching is case-insensitive because the model
// may return the same word with a different capitalisation.
func (c Config) NoteWordDismissed(word string) bool {
	word = strings.TrimSpace(word)
	if word == "" {
		return false
	}
	for _, item := range c.DismissedNoteWords {
		if strings.EqualFold(strings.TrimSpace(item), word) {
			return true
		}
	}
	return false
}

// InitialAIMode reports which mode a selection popup should open in, given the
// "默认模式" preference on the AI settings page, and whether that preference
// applies at all.
//
// AIMode was stored, rendered in the settings page, and never read: switching
// it to 词典精读 changed nothing because the popup always started from the
// translate engine and only reached the AI cards after an explicit tab click.
// The two values the settings page writes are "grammar" (长难句分析) and
// "translate" (润色翻译 — the writing/polish action, not the
// machine-translation engine). A third value "dict" (词典精读) was retired
// with the 词典 tab; normalizeInitialAIMode clears it on load.
//
// Two conditions gate the preference, both of them there to avoid changing
// behaviour for people who never asked for it:
//   - AI 精翻 must be on, or the actions cannot run and the card would be empty.
//   - The user must have actually picked a mode (AIModeIsChoice). Existing
//     configs all carry ai_mode "grammar" because that is the shipped default;
//     honouring it would reroute every selection to 长难句分析 on upgrade.
func (c Config) InitialAIMode() (popupMode, bool) {
	if !c.AIEnabled || !c.AIModeIsChoice {
		return modeTranslate, false
	}
	switch c.AIMode {
	case "grammar":
		return modeGrammar, true
	case "translate", "polish":
		return modeWriting, true
	}
	return modeTranslate, false
}

// normalizeInitialAIMode clears a 默认模式 whose target view no longer exists.
// 词典精读 is not a popup tab any more — a word's card lives in 生词本 — so a
// file that still says "dict" would claim a preference the popup cannot honour.
// Falls back to 跟随翻译, which is the honest description of what will happen.
func normalizeInitialAIMode(cfg *Config) {
	if cfg.AIMode == "dict" {
		cfg.AIMode = ""
		cfg.AIModeIsChoice = false
	}
}

// DefaultConfig returns the shipped defaults.
func DefaultConfig() Config {
	return Config{
		HotkeySelection:    "Alt+D",
		HotkeyInput:        "Alt+Q",
		HotkeyScreenshot:   "Alt+C",
		OCRLang:            "auto",
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
		AIModel:            "workbuddy-ai/deepseek-v4.1-flash",
		AIEffort:           EffortFast,
		CloseOnBlur:        true,
		AlwaysOnTop:        true,
		PopupWidth:         500,
		ThemeMode:          "system",
		HistoryEnabled:     true,
		ShowNotes:          true,
		NotesMigrated:      1,
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
	mu     sync.RWMutex
	saveMu sync.Mutex
	cfg    Config
	path   string
	// lastSaveErr records the most recent persistence failure so settings can
	// tell the user their change is only in memory. Guarded by mu.
	lastSaveErr error
	// notesMigrated records that load() applied the 猜你想存 default to a file
	// that predated the marker, so the caller can persist the decision. Without
	// the write-back the file keeps saying show_notes: false, and the migration
	// would silently re-run and undo the switch the first time the user turns
	// the card off.
	notesMigrated bool
	// effortMigrated records that load() folded a retired 推理强度 label into
	// one of the two that exist now. The settings page reads the in-memory
	// value, so the fold works without this; persisting it keeps the file from
	// saying "none" forever, which is a value no control can show.
	effortMigrated bool
	// retiredViewMigrated records that load() cleaned up after the retired
	// 词典 view: it either cleared a 默认模式 that pointed at it, or removed
	// the default-dictionary action that only that view could render. Same
	// reasoning as effortMigrated — the in-memory config is already correct,
	// but without the write-back the file keeps the dead references and every
	// start re-runs the cleanup.
	retiredViewMigrated bool
}

func newStore() *store {
	s := &store{cfg: DefaultConfig()}
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
		s.path = filepath.Join(dir, "config.json")
	} else {
		log.Printf("fastDictionary: no user data dir: %v", err)
	}
	if !s.load() && s.path != "" {
		// First run: materialise the defaults so the file is there to edit.
		if err := saveConfig(s.path, s.cfg); err != nil {
			log.Printf("fastDictionary: could not write default config: %v", err)
		}
	} else if s.notesMigrated && s.path != "" {
		// Persist the one-time migration so the marker stops re-running on a
		// file that still spells out show_notes: false.
		if err := saveConfig(s.path, s.Get()); err != nil {
			log.Printf("fastDictionary: could not persist notes migration: %v", err)
		}
	} else if s.effortMigrated && s.path != "" {
		if err := saveConfig(s.path, s.Get()); err != nil {
			log.Printf("fastDictionary: could not persist 推理强度 migration: %v", err)
		}
	} else if s.retiredViewMigrated && s.path != "" {
		if err := saveConfig(s.path, s.Get()); err != nil {
			log.Printf("fastDictionary: could not persist 词典 view cleanup: %v", err)
		}
	}
	log.Printf("fastDictionary: config path %s", s.path)
	return s
}

// cloneConfig returns a copy whose slices are independent of cfg. A plain
// struct copy is not enough: CustomActions and SelectionBlocklist share their
// backing arrays, so a caller that edits a returned config could mutate the
// live store without holding its lock.
func cloneConfig(cfg Config) Config {
	clone := cfg
	if cfg.SelectionBlocklist != nil {
		clone.SelectionBlocklist = append([]string(nil), cfg.SelectionBlocklist...)
	}
	if cfg.DismissedNoteWords != nil {
		clone.DismissedNoteWords = append([]string(nil), cfg.DismissedNoteWords...)
	}
	if cfg.CustomActions != nil {
		clone.CustomActions = make([]CustomActionConfig, len(cfg.CustomActions))
		for i, action := range cfg.CustomActions {
			clone.CustomActions[i] = action
			if action.OutputSchema != nil {
				clone.CustomActions[i].OutputSchema = append([]string(nil), action.OutputSchema...)
			}
		}
	}
	return clone
}

// Get returns a deep copy of the current config, safe to read and mutate from
// any goroutine without touching the store's live slices.
func (s *store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfig(s.cfg)
}

// LastSaveError reports the most recent failure to persist the config. The
// caller can use it to tell the user that a setting will be lost on restart.
func (s *store) LastSaveError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSaveErr
}

// RetrySave writes the current in-memory config again after a failed update.
// The settings banner uses it instead of asking the user to change a value
// twice just to trigger another save.
func (s *store) RetrySave() error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.RLock()
	cfg := cloneConfig(s.cfg)
	path := s.path
	s.mu.RUnlock()
	var err error
	if path != "" {
		err = saveConfig(path, cfg)
	}
	s.mu.Lock()
	s.lastSaveErr = err
	s.mu.Unlock()
	return err
}

// Path returns the config file path, or "" for the in-memory test store.
func (s *store) Path() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}

// Update mutates the config and persists it. saveMu serialises the whole
// read-modify-write-and-save sequence; two concurrent settings changes used to
// race in saveConfig's fixed temp path and one update could overwrite the
// other's JSON on disk.
func (s *store) Update(fn func(*Config)) Config {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.Lock()
	fn(&s.cfg)
	cfg := cloneConfig(s.cfg)
	path := s.path
	s.lastSaveErr = nil
	s.mu.Unlock()

	var saveErr error
	if path != "" {
		saveErr = saveConfig(path, cfg)
		if saveErr != nil {
			log.Printf("fastDictionary: config save failed: %v", saveErr)
		}
	}

	s.mu.Lock()
	s.lastSaveErr = saveErr
	s.mu.Unlock()
	return cfg
}

// normalizeAIEffort folds the retired 推理强度 labels into the two levels that
// exist now.
//
// The old option 免思考 was not a level at all: it meant "send no reasoning
// field", which is only "no reasoning" for models whose default is no
// reasoning. Measured on the same sentence, deepseek 1.54s / 0 reasoning
// tokens but gemini 8.93s / 743 tokens. Without this fold, a file written
// before the rename decodes into a value the segmented control cannot show, so
// the settings page renders with nothing selected.
func normalizeAIEffort(cfg *Config) {
	switch cfg.AIEffort {
	case EffortFast, EffortDeep:
		return
	case "high":
		cfg.AIEffort = EffortDeep
	default: // "", "none", "low", "off", anything unknown
		cfg.AIEffort = EffortFast
	}
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
		log.Printf("fastDictionary: config parse failed, using defaults: %v", err)
		return false
	}
	if dropped := ensureDefaultActions(&cfg); dropped {
		s.mu.Lock()
		s.retiredViewMigrated = true
		s.mu.Unlock()
	}
	normalizeInitialAIMode(&cfg)
	// A file written while 词典精读 still existed is rewritten once, so it
	// stops claiming a default view the popup cannot open.
	var rawMode struct {
		AIMode *string `json:"ai_mode"`
	}
	if json.Unmarshal(data, &rawMode) == nil && rawMode.AIMode != nil && *rawMode.AIMode != cfg.AIMode {
		s.mu.Lock()
		s.retiredViewMigrated = true
		s.mu.Unlock()
	}
	// 猜你想存 was on by default, then briefly off, and the user reported it
	// as a lost feature rather than as a preference they had changed. A file
	// without the marker never had a chance to choose, so it gets the new
	// default once; after that the value in the file is the user's answer and
	// is left alone.
	//
	// Presence in the JSON decides, not the decoded number: unmarshalling into
	// DefaultConfig() leaves every absent key at its default, and the default
	// carries the marker, so checking cfg.NotesMigrated == 0 never fired for
	// the files this migration exists for. Verified against the real
	// %APPDATA%/fastDictionary/config.json, which has no such key.
	var marker struct {
		NotesMigrated *int `json:"notes_migrated"`
	}
	if err := json.Unmarshal(data, &marker); err != nil || marker.NotesMigrated == nil || *marker.NotesMigrated == 0 {
		cfg.ShowNotes = true
		cfg.NotesMigrated = 1
		s.mu.Lock()
		s.notesMigrated = true
		s.mu.Unlock()
	}
	normalizeAIEffort(&cfg)
	// A file that spelled a retired label is rewritten once, so it stops
	// carrying a value no control can show.
	var rawEffort struct {
		AIEffort *string `json:"ai_effort"`
	}
	if json.Unmarshal(data, &rawEffort) == nil && rawEffort.AIEffort != nil && *rawEffort.AIEffort != cfg.AIEffort {
		s.mu.Lock()
		s.effortMigrated = true
		s.mu.Unlock()
	}
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
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// A unique temp name per save prevents two concurrent settings changes
	// from writing into the same file and renaming a half-written mixture.
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	closed := false
	cleanup := func() {
		if !closed {
			_ = f.Close()
		}
		_ = os.Remove(tmp)
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		closed = true
		_ = os.Remove(tmp)
		return err
	}
	closed = true

	// Windows sometimes denies the rename while the just-closed file is still
	// held by an indexer or antivirus scanner. Retry briefly instead of losing
	// the user's settings for a transient sharing violation.
	var renameErr error
	for attempt := 0; attempt < 6; attempt++ {
		if renameErr = os.Rename(tmp, path); renameErr == nil {
			return nil
		}
		if attempt < 5 {
			time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
		}
	}
	_ = os.Remove(tmp)
	return renameErr
}

func DefaultCustomActions() []CustomActionConfig {
	return []CustomActionConfig{
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

func ensureDefaultActions(cfg *Config) bool {
	defaults := DefaultCustomActions()
	if len(cfg.CustomActions) == 0 {
		cfg.CustomActions = defaults
		return false
	}
	// 词典 was retired along with its popup tab; a word's card belongs in
	// 生词本 now. The action was persisted into every existing config file, so
	// dropping it from DefaultCustomActions() alone would leave the old entry
	// sitting in the list, showing up in the 划词 chip and in 指令 settings
	// with nothing able to render it. Reports whether it had to remove one,
	// so load() can rewrite the file instead of filtering the same stale entry
	// on every start.
	dropped := false
	kept := cfg.CustomActions[:0]
	for _, a := range cfg.CustomActions {
		if a.ID == "default-dictionary" {
			dropped = true
			continue
		}
		kept = append(kept, a)
	}
	cfg.CustomActions = kept
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
	return dropped
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
