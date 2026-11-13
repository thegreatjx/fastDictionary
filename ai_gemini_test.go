package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiPresetUsesOpenAICompatEndpointAndDefaultModel(t *testing.T) {
	p, ok := aiPresetByID("gemini")
	if !ok {
		t.Fatal("缺少 Gemini 预设")
	}
	if p.BaseURL != geminiDefaultEndpoint || p.Model != geminiDefaultModel || !p.NeedsKey {
		t.Fatalf("Gemini 预设不正确: %+v", p)
	}
	if p.KeyURL != geminiKeyURL {
		t.Fatalf("KeyURL = %q, want %q", p.KeyURL, geminiKeyURL)
	}
	chat, err := aiChatEndpoint(geminiDefaultEndpoint, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"; chat != want {
		t.Fatalf("chat endpoint = %q, want %q", chat, want)
	}
	models, err := aiModelsEndpoint(geminiDefaultEndpoint, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://generativelanguage.googleapis.com/v1beta/openai/models"; models != want {
		t.Fatalf("models endpoint = %q, want %q", models, want)
	}

	cfg := DefaultConfig()
	cfg.AIKey = "sk-other-service"
	applyAIPreset(&cfg, p)
	if cfg.AIProvider != "gemini" || cfg.AIURL != p.BaseURL || cfg.AIModel != p.Model {
		t.Fatalf("预设没有生效: %+v", cfg)
	}
	if cfg.AIKey != "" || cfg.AIKeyEnc != "" {
		t.Fatalf("切换主机后旧 Key 仍在: key=%q enc=%q", cfg.AIKey, cfg.AIKeyEnc)
	}
}

func TestGeminiCuratedModelsAreSelectable(t *testing.T) {
	cfg := DefaultConfig()
	p, _ := aiPresetByID("gemini")
	applyAIPreset(&cfg, p)

	models := aiModelsForState(cfg, aiModelsState{})
	if len(models) < 2 {
		t.Fatalf("Gemini 离线清单过短: %v", models)
	}
	if models[0] != geminiDefaultModel {
		t.Fatalf("默认模型 = %q, want %q", models[0], geminiDefaultModel)
	}
	wantSelect := len(aiPresetForConfig(cfg).ModelOptions) > 0
	if !wantSelect {
		t.Fatal("Gemini 预设应让模型字段显示下拉清单")
	}
	if !strings.Contains(strings.Join(models, " "), "gemini-2.5-flash-lite") {
		t.Fatalf("清单缺少备用免费模型: %v", models)
	}
}

func TestGeminiOpenAICompatImageRequest(t *testing.T) {
	var gotPath, gotAuth string
	var got struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("无法解析请求: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"已看图\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.AIProvider = "gemini"
	cfg.AIURL = srv.URL + "/v1beta/openai/"
	cfg.AIModel = geminiDefaultModel
	cfg.AIKey = "test-key"
	conn := cfg.AIConn(cfg.AIModel)
	const image = "data:image/jpeg;base64,AAAA"
	out, err := StreamAIWithImage(context.Background(), conn, "系统提示", "请翻译", image, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "已看图" {
		t.Fatalf("out = %q, want %q", out, "已看图")
	}
	if gotPath != "/v1beta/openai/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if got.Model != geminiDefaultModel || !got.Stream || len(got.Messages) != 2 {
		t.Fatalf("请求主体不正确: %+v", got)
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(got.Messages[1].Content, &parts); err != nil {
		t.Fatalf("user content 不是多模态数组: %v", err)
	}
	if len(parts) != 2 || parts[0].Type != "text" || parts[0].Text != "请翻译" ||
		parts[1].Type != "image_url" || parts[1].ImageURL.URL != image {
		t.Fatalf("图片内容不正确: %+v", parts)
	}
}

func TestGeminiModelsEndpointListsModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/openai/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"models/gemini-3.5-flash-lite"},{"id":"models/gemini-3.1-flash-lite"}]}`)
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.AIProvider = "gemini"
	cfg.AIURL = srv.URL + "/v1beta/openai/"
	cfg.AIKey = "test-key"
	models, err := FetchAvailableModels(context.Background(), cfg.AIConn(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "gemini-3.1-flash-lite" || models[1] != "gemini-3.5-flash-lite" {
		t.Fatalf("models = %v", models)
	}
}
