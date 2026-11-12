package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAIEndpointNormalization(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		magpie     bool
		wantChat   string
		wantModels string
		wantErr    bool
	}{
		{
			name: "base url", raw: "https://api.deepseek.com/v1",
			wantChat:   "https://api.deepseek.com/v1/chat/completions",
			wantModels: "https://api.deepseek.com/v1/models",
		},
		{
			name: "full endpoint", raw: "https://api.deepseek.com/v1/chat/completions",
			wantChat:   "https://api.deepseek.com/v1/chat/completions",
			wantModels: "https://api.deepseek.com/v1/models",
		},
		{
			name: "trailing slash", raw: "https://api.deepseek.com/v1/",
			wantChat:   "https://api.deepseek.com/v1/chat/completions",
			wantModels: "https://api.deepseek.com/v1/models",
		},
		{
			name: "host only", raw: "https://api.deepseek.com",
			wantChat:   "https://api.deepseek.com/v1/chat/completions",
			wantModels: "https://api.deepseek.com/v1/models",
		},
		{
			name: "provider path", raw: "https://open.bigmodel.cn/api/paas/v4",
			wantChat:   "https://open.bigmodel.cn/api/paas/v4/chat/completions",
			wantModels: "https://open.bigmodel.cn/api/paas/v4/models",
		},
		{
			name: "query kept on chat", raw: "https://example.com/v1/chat/completions?api-version=1",
			wantChat:   "https://example.com/v1/chat/completions?api-version=1",
			wantModels: "https://example.com/v1/models",
		},
		{
			name: "magpie empty", raw: "", magpie: true,
			wantChat:   "http://127.0.0.1:3425/v1/chat/completions",
			wantModels: "http://127.0.0.1:3425/v1/models",
		},
		{
			name: "custom empty", raw: "", wantErr: true,
		},
		{
			name: "userinfo rejected", raw: "https://user:pass@example.com/v1", wantErr: true,
		},
		{
			name: "fragment rejected", raw: "https://example.com/v1#frag", wantErr: true,
		},
		{
			name: "ftp rejected", raw: "ftp://example.com/v1", wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat, err := aiChatEndpoint(tc.raw, tc.magpie)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("aiChatEndpoint(%q) = %q, want error", tc.raw, chat)
				}
				return
			}
			if err != nil {
				t.Fatalf("aiChatEndpoint(%q): %v", tc.raw, err)
			}
			if chat != tc.wantChat {
				t.Errorf("chat = %q, want %q", chat, tc.wantChat)
			}
			models, err := aiModelsEndpoint(tc.raw, tc.magpie)
			if err != nil {
				t.Fatalf("aiModelsEndpoint(%q): %v", tc.raw, err)
			}
			if models != tc.wantModels {
				t.Errorf("models = %q, want %q", models, tc.wantModels)
			}
		})
	}
}

func TestStreamAISendsBearerAndAcceptsCompatibleSSE(t *testing.T) {
	var auth, path, accept string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path, accept = r.Header.Get("Authorization"), r.URL.Path, r.Header.Get("Accept")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data:{\"choices\":[{\"delta\":{\"content\":\"你\"}}],\"error\":null}\n\n")
		fmt.Fprint(w, "data:{\"choices\":[{\"delta\":{\"content\":\"好\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer srv.Close()

	out, err := StreamAI(context.Background(), aiConn{
		Provider: "custom", Endpoint: srv.URL + "/v1", Model: "m", Key: "sk-test",
	}, "system", "user", "", nil)
	if err != nil {
		t.Fatalf("StreamAI: %v", err)
	}
	if out != "你好" {
		t.Fatalf("output = %q, want 你好", out)
	}
	if auth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", auth)
	}
	if path != "/v1/chat/completions" {
		t.Errorf("path = %q", path)
	}
	if accept != "text/event-stream" {
		t.Errorf("Accept = %q", accept)
	}
	if body["model"] != "m" || body["stream"] != true {
		t.Errorf("request body = %#v", body)
	}
}

func TestStreamAIHTTPFailuresAreTyped(t *testing.T) {
	cases := []struct {
		status int
		want   aiFailureKind
	}{
		{http.StatusUnauthorized, aiFailureAuth},
		{http.StatusForbidden, aiFailurePermission},
		{http.StatusNotFound, aiFailureNotFound},
		{http.StatusTooManyRequests, aiFailureRateLimit},
		{http.StatusBadRequest, aiFailureBadRequest},
		{http.StatusInternalServerError, aiFailureServer},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"error":{"message":"provider detail"}}`)
			}))
			defer srv.Close()
			_, err := StreamAI(context.Background(), aiConn{
				Provider: "custom", Endpoint: srv.URL + "/v1", Model: "m",
			}, "s", "u", "", nil)
			var failure *aiFailure
			if !errors.As(err, &failure) {
				t.Fatalf("error = %v, want *aiFailure", err)
			}
			if failure.kind != tc.want {
				t.Fatalf("kind = %v, want %v (%s)", failure.kind, tc.want, failure.msg)
			}
		})
	}
}

func TestStreamAIRedactsKeyFromProviderError(t *testing.T) {
	const key = "sk-secret-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":{"message":"invalid key %s"}}`, key)
	}))
	defer srv.Close()

	_, err := StreamAI(context.Background(), aiConn{
		Provider: "custom", Endpoint: srv.URL + "/v1", Model: "m", Key: key,
	}, "s", "u", "", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), key) {
		t.Fatalf("error leaked the key: %v", err)
	}
	if !strings.Contains(err.Error(), "[已隐藏]") {
		t.Fatalf("error did not show the redaction marker: %v", err)
	}
}

func TestFetchAvailableModelsParsesBothShapes(t *testing.T) {
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		fmt.Fprint(w, `{"data":[{"id":"a"},{"id":"b"}],"models":[{"name":"models/c"}]}`)
	}))
	defer srv.Close()

	models, err := FetchAvailableModels(context.Background(), aiConn{
		Provider: "custom", Endpoint: srv.URL + "/v1", Model: "m", Key: "sk-test",
	})
	if err != nil {
		t.Fatalf("FetchAvailableModels: %v", err)
	}
	if strings.Join(models, ",") != "a,b,c" {
		t.Fatalf("models = %v", models)
	}
	if auth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", auth)
	}
	if path != "/v1/models" {
		t.Errorf("path = %q", path)
	}
}

func TestFetchAvailableModelsUnsupportedIsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchAvailableModels(context.Background(), aiConn{
		Provider: "custom", Endpoint: srv.URL + "/v1", Model: "m",
	})
	if !errors.Is(err, errAIModelsUnsupported) {
		t.Fatalf("error = %v, want errAIModelsUnsupported", err)
	}
}

func TestStreamAICustomEmptyEndpointDoesNotFallBackToMagpie(t *testing.T) {
	_, err := StreamAI(context.Background(), aiConn{
		Provider: "custom", Endpoint: "", Model: "m",
	}, "s", "u", "", nil)
	if err == nil {
		t.Fatal("expected an error for an empty custom address")
	}
	if !strings.Contains(err.Error(), "API 地址") {
		t.Fatalf("error = %v, want a missing-address message", err)
	}
}

func TestStreamAICustomNeverSendsReasoning(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	if _, err := StreamAI(context.Background(), aiConn{
		Provider: "custom", Endpoint: srv.URL + "/v1", Model: "m",
	}, "s", "u", "high", nil); err != nil {
		t.Fatalf("StreamAI: %v", err)
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatalf("custom request carried reasoning_effort: %#v", body)
	}
	if _, ok := body["reasoning"]; ok {
		t.Fatalf("custom request carried reasoning: %#v", body)
	}
}

func TestAIReasoningEffortOnlyForMagpie(t *testing.T) {
	magpie := DefaultConfig()
	magpie.AIEffort = EffortDeep
	if got := magpie.AIReasoningEffortFor(magpieDefaultModel); got != "high" {
		t.Fatalf("magpie deep effort = %q, want high", got)
	}
	custom := magpie
	custom.AIProvider = "deepseek"
	if got := custom.AIReasoningEffortFor(magpieDefaultModel); got != "" {
		t.Fatalf("custom effort = %q, want empty", got)
	}
}

func TestNormalizeAPIKey(t *testing.T) {
	cases := map[string]string{
		"  sk-test  ":       "sk-test",
		"Bearer sk-test":    "sk-test",
		"bearer   sk-test ": "sk-test",
		"sk-test\n":         "sk-test",
	}
	for in, want := range cases {
		if got := normalizeAPIKey(in); got != want {
			t.Errorf("normalizeAPIKey(%q) = %q, want %q", in, got, want)
		}
	}
}
