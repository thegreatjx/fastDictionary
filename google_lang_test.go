package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGoogleTranslateSendsAutoForUndetectedText is the regression test for the
// bug the user reported as "谷歌翻译又是400". detectLocal returns "" for plain
// English, googleTranslate used to send that empty value as sl, and Google
// answers an empty sl with HTTP 400 — so Google failed on every English
// sentence and the race silently became Microsoft-only.
func TestGoogleTranslateSendsAutoForUndetectedText(t *testing.T) {
	var gotSl string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSl = r.URL.Query().Get("sl")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[[["你好",null,null,null,null]],null,"en",null]`))
	}))
	defer srv.Close()

	old := googleSingleURL
	googleSingleURL = srv.URL + "/translate_a/single"
	defer func() { googleSingleURL = old }()

	out, detected, err := GoogleTranslate(context.Background(), "hello", "", "zh-Hans")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if out != "你好" {
		t.Errorf("text = %q, want 你好", out)
	}
	if detected != "en" {
		t.Errorf("detected = %q, want en", detected)
	}
	if gotSl != "auto" {
		t.Errorf("sl sent = %q, want auto; an empty sl makes Google answer 400", gotSl)
	}
}

// TestGoogleTranslateKeepsDetectedSource keeps the other half: when the local
// detector does know the language, that knowledge is sent as-is.
func TestGoogleTranslateKeepsDetectedSource(t *testing.T) {
	var gotSl string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSl = r.URL.Query().Get("sl")
		w.Write([]byte(`[[["Hello",null,null,null,null]],null,"zh-Hans",null]`))
	}))
	defer srv.Close()

	old := googleSingleURL
	googleSingleURL = srv.URL + "/translate_a/single"
	defer func() { googleSingleURL = old }()

	if _, _, err := GoogleTranslate(context.Background(), "你好", "zh-Hans", "en"); err != nil {
		t.Fatalf("translate: %v", err)
	}
	if gotSl != "zh-Hans" {
		t.Errorf("sl sent = %q, want zh-Hans", gotSl)
	}
}

// TestGoogleDictSendsAutoForUndetectedWord covers the second call site that had
// the same empty-parameter bug. It only asserts on the request: the response
// parser has a separate pre-existing problem with dt=bd replies (it reads the
// group's entries as plain strings, while Google sends
// ["抢占", ["seize"], null, 0.11] tuples), so a successful parse is not what
// this test is about.
func TestGoogleDictSendsAutoForUndetectedWord(t *testing.T) {
	var gotSl string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSl = r.URL.Query().Get("sl")
		w.Write([]byte(`[[[null,null,null,"sēz"]],[["verb",["抢占","夺取"],[["抢占",["seize"],null,0.1]]]]]`))
	}))
	defer srv.Close()

	old := googleSingleURL
	googleSingleURL = srv.URL + "/translate_a/single"
	defer func() { googleSingleURL = old }()

	GoogleDict(context.Background(), "seize", "", "zh-Hans")
	if gotSl != "auto" {
		t.Errorf("sl sent = %q, want auto; Google answers 400 when it is empty", gotSl)
	}
}
