package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var tencentClient = &http.Client{Timeout: 10 * time.Second}

const tencentDefaultURL = "https://transmart.qq.com/api/imt"

var tencentAPIURL = tencentDefaultURL

var tencentTargets = map[string]bool{
	"zh-Hans": true, "zh-Hant": true,
	"en": true, "ja": true, "ko": true,
	"fr": true, "de": true, "es": true, "pt": true, "it": true,
	"ru": true, "tr": true,
	"th": true, "vi": true, "ar": true,
}

func tencentSupportsTarget(tag string) bool {
	return tencentTargets[strings.TrimSpace(tag)]
}

func tencentLang(tag string) string {
	t := strings.ToLower(strings.TrimSpace(tag))
	switch t {
	case "zh-hans", "zh-cn", "zh":
		return "zh"
	case "zh-hant", "zh-tw", "zh-hk":
		return "zh-TW"
	}
	return t
}

type tencentResponse struct {
	Header struct {
		RetCode string `json:"ret_code"`
	} `json:"header"`
	AutoTranslation []string `json:"auto_translation"`
	Message         string   `json:"message"`
	SrcLang         string   `json:"src_lang"`
}

func tencentTranslate(ctx context.Context, text, from, to string) (msResult, error) {
	if strings.TrimSpace(text) == "" {
		return msResult{}, fmt.Errorf("腾讯: 缺少原文")
	}
	target := tencentLang(to)
	if target == "" {
		return msResult{}, fmt.Errorf("腾讯: 缺少目标语言")
	}
	source := tencentLang(from)
	if source == "" {
		source = "auto"
	}

	body := map[string]any{
		"header": map[string]any{
			"fn":         "auto_translation",
			"client_key": "browser-chrome-126.0.0-Windows_10",
		},
		"type":           "plain",
		"model_category": "normal",
		"source":         map[string]any{"lang": source, "text_list": []string{text}},
		"target":         map[string]any{"lang": target},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return msResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tencentAPIURL, bytes.NewReader(payload))
	if err != nil {
		return msResult{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")

	resp, err := tencentClient.Do(req)
	if err != nil {
		return msResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return msResult{}, err
	}
	if resp.StatusCode != 200 {
		return msResult{}, fmt.Errorf("腾讯: http %d", resp.StatusCode)
	}

	var parsed tencentResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return msResult{}, fmt.Errorf("腾讯: bad json: %w", err)
	}
	if parsed.Header.RetCode != "succ" {
		msg := parsed.Message
		if msg == "" {
			msg = parsed.Header.RetCode
		}
		return msResult{}, fmt.Errorf("腾讯: %s", msg)
	}
	out := strings.TrimSpace(strings.Join(parsed.AutoTranslation, "\n"))
	if out == "" {
		return msResult{}, fmt.Errorf("腾讯: empty result")
	}
	return msResult{Text: out, Detected: tencentDetected(parsed.SrcLang)}, nil
}

func tencentDetected(tag string) string {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "":
		return ""
	case "zh":
		return "zh-Hans"
	case "zh-tw":
		return "zh-Hant"
	}
	return strings.ToLower(strings.TrimSpace(tag))
}
