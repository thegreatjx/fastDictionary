package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func swapConfigPath(t *testing.T, path string) {
	t.Helper()
	old := resolveConfigPath
	resolveConfigPath = func() string { return path }
	t.Cleanup(func() { resolveConfigPath = old })
}

func TestCorruptConfigIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	corrupt := []byte(`{"ai_url": "http://user.example/v1/chat", "ai_model":`)
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	swapConfigPath(t, path)

	newStore()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("损坏的 config.json 被默认配置覆盖，用户设置无法找回：\n got: %s\nwant: %s", got, corrupt)
	}
}

func TestFirstRunStillWritesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	swapConfigPath(t, path)

	newStore()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("首次运行应写出默认配置：%v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("写出的默认配置不是合法 JSON：%v", err)
	}
	want := DefaultConfig()
	applyFirstRunAIDefaults(&want)
	if cfg.AIURL != want.AIURL || cfg.AIModel != want.AIModel {
		t.Fatalf("首次运行写出的不是默认值：AIURL=%q AIModel=%q", cfg.AIURL, cfg.AIModel)
	}
}
