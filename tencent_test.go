package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTencentLangMapsChineseTags(t *testing.T) {
	for in, want := range map[string]string{
		"zh-Hans": "zh",
		"zh-CN":   "zh",
		"zh":      "zh",
		"ZH-HANS": "zh",
		"zh-Hant": "zh-TW",
		"zh-TW":   "zh-TW",
		"zh-HK":   "zh-TW",
		"en":      "en",
		"JA":      "ja",
		"":        "",
	} {
		if got := tencentLang(in); got != want {
			t.Errorf("tencentLang(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTencentSupportsTargetOnlyListsMeasuredLanguages pins the whitelist to the
// set that was actually verified against the live service. Widening it without
// testing means the engine answers "Unsupported-Language" on every request and
// its circuit breaker opens for a language it could have served.
func TestTencentSupportsTargetOnlyListsMeasuredLanguages(t *testing.T) {
	for _, tag := range []string{"zh-Hans", "zh-Hant", "en", "ja", "ko", "fr", "de", "ru", "ar"} {
		if !tencentSupportsTarget(tag) {
			t.Errorf("tencentSupportsTarget(%q) = false, want true", tag)
		}
	}
	for _, tag := range []string{"id", "tl", "he", "hi", "nl", "pl", "uk", "th-TH", ""} {
		if tencentSupportsTarget(tag) {
			t.Errorf("tencentSupportsTarget(%q) = true, want false (service answered Unsupported-Language)", tag)
		}
	}
}

func TestTencentDetectedNormalisesChinese(t *testing.T) {
	for in, want := range map[string]string{
		"":      "",
		"zh":    "zh-Hans",
		"ZH":    "zh-Hans",
		"zh-tw": "zh-Hant",
		"en":    "en",
		"JA":    "ja",
	} {
		if got := tencentDetected(in); got != want {
			t.Errorf("tencentDetected(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTencentTranslateRejectsRefusalInBody covers the failure mode that makes
// this endpoint different: it answers HTTP 200 with ret_code "error", so a
// status-code-only check would hand an empty translation to the race and let
// it win.
func TestTencentTranslateRejectsRefusalInBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"header":{"ret_code":"error"},"message":"Unrecognized target language: zh-Hans"}`))
	}))
	defer srv.Close()

	old := tencentAPIURL
	tencentAPIURL = srv.URL
	defer func() { tencentAPIURL = old }()

	if _, err := tencentTranslate(context.Background(), "hello", "en", "zh-Hans"); err == nil {
		t.Fatal("expected an error for ret_code error with HTTP 200")
	} else if !strings.Contains(err.Error(), "Unrecognized target language") {
		t.Fatalf("error should carry the service message, got %v", err)
	}
}

func TestTencentTranslateSendsBareLanguageCodes(t *testing.T) {
	type payload struct {
		Header map[string]any `json:"header"`
		Source struct {
			Lang     string   `json:"lang"`
			TextList []string `json:"text_list"`
		} `json:"source"`
		Target struct {
			Lang string `json:"lang"`
		} `json:"target"`
	}
	var got payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"header":{"ret_code":"succ"},"auto_translation":["你好"],"src_lang":"en"}`))
	}))
	defer srv.Close()

	old := tencentAPIURL
	tencentAPIURL = srv.URL
	defer func() { tencentAPIURL = old }()

	res, err := tencentTranslate(context.Background(), "hello", "en", "zh-Hans")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if res.Text != "你好" {
		t.Errorf("text = %q, want 你好", res.Text)
	}
	if res.Detected != "en" {
		t.Errorf("detected = %q, want en", res.Detected)
	}
	if got.Target.Lang != "zh" {
		t.Errorf("target lang sent = %q, want the bare code zh", got.Target.Lang)
	}
	if got.Source.Lang != "en" {
		t.Errorf("source lang sent = %q, want en", got.Source.Lang)
	}
	if len(got.Source.TextList) != 1 || got.Source.TextList[0] != "hello" {
		t.Errorf("text_list = %v, want [hello]", got.Source.TextList)
	}
	if got.Header["fn"] != "auto_translation" {
		t.Errorf("header.fn = %v, want auto_translation", got.Header["fn"])
	}
}

// TestTencentTranslateAsksServiceToDetect keeps the documented fallback: with
// no local detection the engine is asked to detect, instead of being sent an
// empty language.
func TestTencentTranslateAsksServiceToDetect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Source struct {
				Lang string `json:"lang"`
			} `json:"source"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Source.Lang != "auto" {
			t.Errorf("source lang = %q, want auto when nothing was detected locally", body.Source.Lang)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"header":{"ret_code":"succ"},"auto_translation":["你好"],"src_lang":"en"}`))
	}))
	defer srv.Close()

	old := tencentAPIURL
	tencentAPIURL = srv.URL
	defer func() { tencentAPIURL = old }()

	if _, err := tencentTranslate(context.Background(), "hello", "", "zh-Hans"); err != nil {
		t.Fatalf("translate: %v", err)
	}
}
