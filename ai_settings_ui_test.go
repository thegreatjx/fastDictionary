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

func TestAIConnectionResultShowsFullModelAndTime(t *testing.T) {
	a := newTestApp()
	a.page = "ai"
	cfg := a.store.Get()
	msg := "正常 · 用时 1.234s · minimax-code/MiniMax-M2.7-highspeed · 返回 pong"
	a.aiProbe = aiProbeState{phase: aiProbeSucceeded, source: aiProbeSource(cfg), msg: msg}

	tt := ui.NewTester(a.settingsView, 880, 1400)
	for _, want := range []string{"用时 1.234s", "minimax-code/MiniMax-M2.7-highspeed", "返回 pong"} {
		if !tt.HasText(want) {
			t.Errorf("连通性结果缺少 %q：%v", want, tt.Texts())
		}
	}
	r, ok := tt.Find(msg)
	if !ok {
		t.Fatalf("找不到完整结果文本：%v", tt.Texts())
	}
	if r.W <= 220 {
		t.Errorf("结果仍被限制在 220px 内：%+v", r)
	}
}

func TestSettingsAIPageShowsGeminiModelGuidance(t *testing.T) {
	a := newTestApp()
	a.page = "ai"
	a.aiHelpOpen = true
	a.store.Update(func(c *Config) {
		p, _ := aiPresetByID("gemini")
		applyAIPreset(c, p)
	})

	tt := ui.NewTester(a.settingsView, 880, 640)
	for _, want := range []string{
		"Gemini（Google AI Studio）",
		"从清单选择模型 ID",
		"以 Google AI Studio 的 Gemini 为例",
		"该模型支持图片输入和文字输出",
	} {
		if !tt.HasText(want) {
			t.Errorf("Gemini 设置页缺少 %q: %v", want, tt.Texts())
		}
	}
}
