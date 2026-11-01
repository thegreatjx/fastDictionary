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

// 腾讯交互翻译（Transmart）, the third keyless engine.
//
// Why this one: the user asked for a free Chinese engine and named 百度 and
// 有道. Both were tested and neither works without registering for a key —
// 百度's v2transapi answers `{"errno":1022}` to an unsigned request, and
// 有道's web form now redirects to error.html. Tencent's Transmart endpoint
// answers a plain JSON POST with no key, no token and no signature.
//
// Quality is the reason it earns a slot next to Microsoft and Google, not just
// availability. Measured 2026-10-26 on the sentence that started this:
//
//	"She cooked the books to hide the losses."
//	  腾讯: 她做假账以掩盖损失。      (idiom recognised)
//	  微软: 她伪造账目以掩盖损失。    (acceptable, flatter)
//	竞速原结果: 完成了烹饪            (wrong)
//
// The endpoint is undocumented, so every assumption about it is in one place
// and every one of them was checked against the live service:
//   - POST https://transmart.qq.com/api/imt, JSON body, no auth header.
//   - No Referer needed (tested with and without).
//   - A 1300-character single item is accepted and translated whole.
//   - Newlines inside one item survive round-trip.
//   - language codes are bare: "zh" and "zh-TW" for Chinese, "en", "ja";
//     the BCP-47 forms this app stores ("zh-Hans", "zh-CN") are rejected with
//     "Unrecognized target language". tencentLang does that mapping.
//   - The languages it cannot reach answer ret_code "Unsupported-Language"
//     rather than an HTTP error, so the code must read the body to notice.
//   - Its own source detection is unreliable: a Chinese sentence came back
//     labelled src_lang "ko". This app therefore sends the language it already
//     detected locally and only falls back to "auto" when it has nothing.

// tencentClient outlives the shared 8s client on purpose: Transmart is a
// cold-start service (measured 50ms warm, over 1s on the first call of a
// session) and the popup has a retry path, not a re-request path.
var tencentClient = &http.Client{Timeout: 10 * time.Second}

const tencentDefaultURL = "https://transmart.qq.com/api/imt"

// tencentAPIURL is a variable so tests can point the engine at a local server.
var tencentAPIURL = tencentDefaultURL

// tencentTargets is the set the service actually accepts as a target,
// measured one by one. A target that is not here is not offered to the race:
// the engine would answer "Unsupported-Language" on every request and its
// breaker would open for a language it was never able to do.
var tencentTargets = map[string]bool{
	"zh-Hans": true, "zh-Hant": true,
	"en": true, "ja": true, "ko": true,
	"fr": true, "de": true, "es": true, "pt": true, "it": true,
	"ru": true, "tr": true,
	"th": true, "vi": true, "ar": true,
}

// tencentSupportsTarget reports whether the engine can translate into tag.
// The race consults it before adding the engine as a contender, so an
// unsupported pair quietly falls back to Microsoft and Google.
func tencentSupportsTarget(tag string) bool {
	return tencentTargets[strings.TrimSpace(tag)]
}

// tencentLang maps this app's BCP-47 tags onto the bare codes Transmart
// expects. Anything unknown is passed through lower-cased: the service answers
// a clean "Unrecognized ... language" instead of translating into the wrong
// one, which is the failure mode that matters.
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

// tencentTranslate translates text into to. from may be empty, which asks the
// service to detect it; the rest of the app reads detected from SrcLang.
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
		// client_key is a self-identifying string the browser build sends; the
		// service accepts any value here, and it is not a credential.
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
	// The service reports its refusals in the body with HTTP 200, so a missing
	// translation is a failure even when the status says otherwise.
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

// tencentDetected returns the service's source language in the tags this app
// uses elsewhere, so the popup can show 检测.
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
