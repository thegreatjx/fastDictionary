package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

const (
	magpieDefaultEndpoint = "http://127.0.0.1:3425/v1/chat/completions"
	magpieDefaultModel    = "workbuddy-ai/deepseek-v4.1-flash"
	geminiDefaultEndpoint = "https://generativelanguage.googleapis.com/v1beta/openai/"
	geminiDefaultModel    = "gemini-3.5-flash-lite"
	geminiKeyURL          = "https://aistudio.google.com/apikey"
)

var defaultGeminiModels = []string{
	geminiDefaultModel,
	"gemini-3.1-flash-lite",
	"gemini-3.8-flash",
	"gemini-3-flash-preview",
	"gemini-2.5-flash-lite",
	"gemini-2.5-flash",
	"gemini-2.5-pro",
}

type aiPreset struct {
	ID           string
	Name         string
	BaseURL      string
	Model        string
	ModelOptions []string
	KeyURL       string
	NeedsKey     bool
	Magpie       bool
}

var aiPresets = []aiPreset{
	{
		ID: "magpie", Name: "本机 Magpie", Magpie: true,
		BaseURL: magpieDefaultEndpoint,
		Model:   magpieDefaultModel,
	},
	{
		ID: "deepseek", Name: "DeepSeek", NeedsKey: true,
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-flash",
		KeyURL:  "https://platform.deepseek.com/api_keys",
	},
	{
		ID: "qwen", Name: "通义千问（百炼）", NeedsKey: true,
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Model:   "qwen-plus",
		KeyURL:  "https://bailian.console.aliyun.com/",
	},
	{
		ID: "zhipu", Name: "智谱 GLM", NeedsKey: true,
		BaseURL: "https://open.bigmodel.cn/api/paas/v4",
		Model:   "glm-4-flash",
		KeyURL:  "https://open.bigmodel.cn/usercenter/apikeys",
	},
	{
		ID: "siliconflow", Name: "硅基流动", NeedsKey: true,
		BaseURL: "https://api.siliconflow.cn/v1",
		KeyURL:  "https://cloud.siliconflow.cn/account/ak",
	},
	{
		ID: "openrouter", Name: "OpenRouter", NeedsKey: true,
		BaseURL: "https://openrouter.ai/api/v1",
		Model:   "deepseek/deepseek-chat",
		KeyURL:  "https://openrouter.ai/keys",
	},
	{
		ID: "openai", Name: "OpenAI", NeedsKey: true,
		BaseURL: "https://api.openai.com/v1",
		KeyURL:  "https://platform.openai.com/api-keys",
	},
	{
		ID: "gemini", Name: "Gemini（Google AI Studio）", NeedsKey: true,
		BaseURL:      geminiDefaultEndpoint,
		Model:        geminiDefaultModel,
		ModelOptions: defaultGeminiModels,
		KeyURL:       geminiKeyURL,
	},
	{
		ID: "ollama", Name: "Ollama（本机）",
		BaseURL: "http://127.0.0.1:11434/v1",
		Model:   "qwen2.5:7b",
	},
	{
		ID: "custom", Name: "自定义", NeedsKey: true,
	},
}

func aiPresetByID(id string) (aiPreset, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, p := range aiPresets {
		if p.ID == id {
			return p, true
		}
	}
	return aiPreset{}, false
}

func aiPresetByName(name string) (aiPreset, bool) {
	name = strings.TrimSpace(name)
	for _, p := range aiPresets {
		if p.Name == name {
			return p, true
		}
	}
	return aiPreset{}, false
}

func aiPresetNames() []string {
	names := make([]string, 0, len(aiPresets))
	for _, p := range aiPresets {
		names = append(names, p.Name)
	}
	return names
}

func normalizeAIProvider(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || id == "magpie" {
		return "magpie"
	}
	if _, ok := aiPresetByID(id); ok {
		return id
	}
	return "custom"
}

func aiPresetForConfig(cfg Config) aiPreset {
	id := normalizeAIProvider(cfg.AIProvider)
	if p, ok := aiPresetByID(id); ok {
		return p
	}
	p, _ := aiPresetByID("custom")
	return p
}

func applyAIPreset(c *Config, p aiPreset) bool {
	oldProvider := normalizeAIProvider(c.AIProvider)
	if oldProvider == p.ID {
		return false
	}
	c.rememberAISettingsFor(oldProvider)
	_ = c.sealAIKey()

	c.AIProvider = p.ID
	c.AIVision = false

	switch p.ID {
	case "custom":
		if isMagpieEndpoint(c.AIURL) {
			c.AIURL = ""
		}
		if c.AIModel == magpieDefaultModel {
			c.AIModel = ""
		}
	default:
		if p.BaseURL != "" {
			c.AIURL = p.BaseURL
		}
		if p.Model != "" {
			c.AIModel = p.Model
		} else if p.BaseURL != "" {
			c.AIModel = ""
		}
	}

	if settings, ok := c.restoreAISettingsFor(p.ID); ok {
		c.AIURL = settings.URL
		c.AIModel = settings.Model
	}
	c.rememberAISettingsFor(p.ID)
	if err := c.restoreAIKeyFor(p.ID); err != nil {
		c.AIKey = ""
		c.AIKeyEnc = ""
	}
	return p.NeedsKey && p.ID != "custom" && normalizeAPIKey(c.AIKey) == ""
}

func aiEndpointSource(cfg Config) string {
	endpoint := strings.TrimSpace(cfg.AIURL)
	if endpoint == "" && cfg.IsMagpie() {
		endpoint = magpieDefaultEndpoint
	}
	return normalizeAIProvider(cfg.AIProvider) + "\x00" + endpoint + "\x00" + aiKeyFingerprint(cfg.AIKey)
}

func aiProbeSource(cfg Config) string {
	return aiEndpointSource(cfg) + "\x00" + strings.TrimSpace(cfg.AIModel) + "\x00" + boolString(cfg.AIVisionAllowed())
}

func aiKeyFingerprint(key string) string {
	key = normalizeAPIKey(key)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

func normalizeAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) >= 7 && strings.EqualFold(key[:7], "Bearer ") {
		key = strings.TrimSpace(key[7:])
	}
	return key
}

func boolString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func endpointHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

func isMagpieEndpoint(raw string) bool {
	switch endpointHost(raw) {
	case "127.0.0.1:3425", "localhost:3425":
		return true
	}
	return false
}
