package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsAIPageRendersBothServiceModes(t *testing.T) {
	a := newTestApp()
	a.page = "ai"

	tt := ui.NewTester(a.settingsView, 880, 640)
	for _, want := range []string{"AI 精翻", "模型服务", "本机 Magpie", "网关地址", "推理强度", "测试连接"} {
		if !tt.HasText(want) {
			t.Errorf("Magpie mode is missing %q: %v", want, tt.Texts())
		}
	}
	if tt.HasText("去获取 Key") {
		t.Errorf("Magpie mode should not show a key link: %v", tt.Texts())
	}

	a.store.Update(func(c *Config) {
		c.AIProvider = "deepseek"
		c.AIURL = "https://api.deepseek.com/v1"
		c.AIModel = "deepseek-chat"
	})
	ttDirect := ui.NewTester(a.settingsView, 880, 640)
	for _, want := range []string{"API 地址", "API Key", "去获取 Key", "图片输入"} {
		if !ttDirect.HasText(want) {
			t.Errorf("direct mode is missing %q: %v", want, ttDirect.Texts())
		}
	}
	if ttDirect.HasText("网关地址") {
		t.Errorf("direct mode still shows the Magpie address field: %v", ttDirect.Texts())
	}
}

func TestSettingsAIHelpPanelUsesSelectedService(t *testing.T) {
	a := newTestApp()
	a.page = "ai"
	a.aiHelpOpen = true

	tt := ui.NewTester(a.settingsView, 880, 640)
	if !tt.HasText("本机 Magpie 需要先启动") {
		t.Errorf("Magpie help is missing: %v", tt.Texts())
	}

	a.store.Update(func(c *Config) {
		c.AIProvider = "custom"
		c.AIURL = "https://example.com/v1"
		c.AIModel = "my-model"
	})
	ttCustom := ui.NewTester(a.settingsView, 880, 640)
	if !ttCustom.HasText("以任意 OpenAI 兼容服务为例：") {
		t.Errorf("custom help is missing: %v", ttCustom.Texts())
	}

	a.store.Update(func(c *Config) {
		c.AIProvider = "ollama"
		c.AIURL = "http://127.0.0.1:11434/v1"
		c.AIModel = "qwen2.5:7b"
	})
	ttOllama := ui.NewTester(a.settingsView, 880, 640)
	if !ttOllama.HasText("Ollama 不需要 API Key。") {
		t.Errorf("Ollama help is missing: %v", ttOllama.Texts())
	}
}
