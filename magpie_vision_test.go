package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamAIWithImageSendsMultimodalPayload(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"译文\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	got, err := StreamAIWithImage(context.Background(), aiConn{
		Provider: "magpie", Endpoint: srv.URL, Model: "m", Vision: true,
	}, "system", "OCR 原文",
		"data:image/png;base64,AAAA", "", nil)
	if err != nil {
		t.Fatalf("StreamAIWithImage: %v", err)
	}
	if got != "译文" {
		t.Fatalf("output = %q, want 译文", got)
	}

	messages, ok := captured["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want 2 entries", captured["messages"])
	}
	user, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("user message = %#v", messages[1])
	}
	parts, ok := user["content"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("content = %#v, want text + image parts", user["content"])
	}
	textPart, _ := parts[0].(map[string]any)
	if textPart["type"] != "text" || textPart["text"] != "OCR 原文" {
		t.Fatalf("text part = %#v", textPart)
	}
	imagePart, _ := parts[1].(map[string]any)
	imageURL, _ := imagePart["image_url"].(map[string]any)
	if imagePart["type"] != "image_url" || imageURL["url"] != "data:image/png;base64,AAAA" {
		t.Fatalf("image part = %#v", imagePart)
	}
}

func TestImageFileDataURLReadsScreenshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snip.png")
	// The PNG signature is enough for http.DetectContentType and keeps this
	// test independent of the screenshot encoder.
	if err := os.WriteFile(path, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := imageFileDataURL(path)
	if err != nil {
		t.Fatalf("imageFileDataURL: %v", err)
	}
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("data URL = %q", got)
	}
}

func TestScreenshotRefinePromptMentionsScreenshotCorrection(t *testing.T) {
	sys := screenshotRefineSystemPrompt("zh-Hans")
	if !strings.Contains(sys, "correct OCR errors") || !strings.Contains(sys, "Simplified Chinese") {
		t.Fatalf("system prompt = %q", sys)
	}
	user := screenshotRefineUserPrompt("helo wrold")
	if !strings.Contains(user, "helo wrold") || !strings.Contains(user, "先纠正 OCR 错误") {
		t.Fatalf("user prompt = %q", user)
	}
}
