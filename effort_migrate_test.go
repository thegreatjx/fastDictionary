package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRetiredEffortLabelIsRewrittenOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	old := map[string]any{"ai_effort": "none", "ai_model": "workbuddy-ai/deepseek-v4.1-flash"}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	s := &store{cfg: DefaultConfig(), path: path}
	if !s.load() {
		t.Fatal("load reported no file")
	}
	if got := s.Get().AIEffort; got != EffortFast {
		t.Fatalf("in-memory AIEffort = %q, want %q", got, EffortFast)
	}
	if !s.effortMigrated {
		t.Fatal("load did not mark the effort migration, so nothing will be written back")
	}

	if err := saveConfig(path, s.Get()); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	var onDisk struct {
		AIEffort string `json:"ai_effort"`
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.AIEffort != EffortFast {
		t.Fatalf("config on disk still says ai_effort=%q, want %q", onDisk.AIEffort, EffortFast)
	}
}

func TestCurrentEffortLabelIsNotRewritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data, _ := json.Marshal(map[string]any{"ai_effort": EffortFast})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	s := &store{cfg: DefaultConfig(), path: path}
	if !s.load() {
		t.Fatal("load reported no file")
	}
	if s.effortMigrated {
		t.Fatal("a file already spelling the current label was marked for rewriting")
	}
}
