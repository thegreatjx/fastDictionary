package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVisionStreamParser(t *testing.T) {
	parser := &visionStreamParser{}
	if source, target := parser.Push("[SOUR"); source != "" || target != "" {
		t.Fatalf("partial marker leaked source=%q target=%q", source, target)
	}
	if source, target := parser.Push("CE]\n原文\n[TAR"); source != "原文" || target != "" {
		t.Fatalf("source stream = %q, target = %q", source, target)
	}
	source, target := parser.Push("GET]\n译文")
	if source != "原文" || target != "译文" {
		t.Fatalf("final stream = %q / %q, want 原文 / 译文", source, target)
	}
}

func TestParseVisionOutput(t *testing.T) {
	source, target := parseVisionOutput("```\n[SOURCE]\nFast and efficient\n[TARGET]\n快速且高效\n```", "bad OCR")
	if source != "Fast and efficient" || target != "快速且高效" {
		t.Fatalf("delimited output = %q / %q", source, target)
	}
	source, target = parseVisionOutput("快速且高效", "local OCR")
	if source != "local OCR" || target != "快速且高效" {
		t.Fatalf("plain translation output = %q / %q", source, target)
	}
}

func TestCompressedImageFileDataURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transparent.png")
	big := image.NewNRGBA(image.Rect(0, 0, 2400, 100))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, big); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	dataURL, err := compressedImageFileDataURL(path)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "data:image/jpeg;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		t.Fatalf("data URL is not JPEG: %q", dataURL[:40])
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, prefix))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bounds().Dx(); got > visionMaxSide {
		t.Fatalf("compressed width = %d, want <= %d", got, visionMaxSide)
	}
	matte := color.RGBAModel.Convert(decoded.At(0, 0)).(color.RGBA)
	if matte.R < 240 || matte.G < 240 || matte.B < 240 {
		t.Fatalf("transparent pixel was not flattened to white: %+v", matte)
	}
}

func TestStreamAIWithImageDoesNotGateOnVisionFlag(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	got, err := StreamAIWithImage(context.Background(), aiConn{
		Provider: "custom", Endpoint: srv.URL, Model: "m", Vision: false,
	}, "system", "user", "data:image/jpeg;base64,AAAA", "", nil)
	if err != nil {
		t.Fatalf("StreamAIWithImage with Vision=false: %v", err)
	}
	if got != "ok" {
		t.Fatalf("output = %q, want ok", got)
	}
	messages, _ := captured["messages"].([]any)
	if len(messages) < 2 {
		t.Fatalf("missing multimodal messages: %#v", captured["messages"])
	}
	user, _ := messages[1].(map[string]any)
	parts, _ := user["content"].([]any)
	if len(parts) != 2 {
		t.Fatalf("user content = %#v, want text + image", user["content"])
	}
}
