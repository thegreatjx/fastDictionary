package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func waitForStaleState(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func sseDelta(content string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]string{"content": content}}},
	})
	return "data: " + string(b) + "\n\n"
}

func assertNewPassInFlight(t *testing.T, a *app) {
	t.Helper()
	a.mu.Lock()
	busy, errText, hasCancel := a.aiBusy, a.errText, a.aiCancel != nil
	a.mu.Unlock()
	if !busy || errText != "" || !hasCancel {
		t.Fatalf("stale AI pass corrupted the newer card: aiBusy=%v errText=%q aiCancelSet=%v",
			busy, errText, hasCancel)
	}
}

func TestRunActionDropsStalePass(t *testing.T) {
	const oldCard = `{"improved":"OLD-CARD"}`
	const newCard = `{"improved":"NEW-CARD"}`

	oldStreamed := make(chan struct{}, 1)
	oldCancelled := make(chan struct{}, 1)
	newStarted := make(chan struct{}, 1)
	releaseNew := make(chan struct{})
	testDone := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseNew) }) }

	var callsMu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsMu.Lock()
		calls++
		n := calls
		callsMu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		write := func(s string) {
			fmt.Fprint(w, s)
			if flusher != nil {
				flusher.Flush()
			}
		}

		switch n {
		case 1:
			write(sseDelta(oldCard))
			select {
			case oldStreamed <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
				select {
				case oldCancelled <- struct{}{}:
				default:
				}
			case <-testDone:
			}
		case 2:
			write(sseDelta(newCard))
			select {
			case newStarted <- struct{}{}:
			default:
			}
			select {
			case <-releaseNew:
			case <-testDone:
			}
			write("data: [DONE]\n\n")
		default:
			write("data: [DONE]\n\n")
		}
	}))
	defer srv.Close()
	defer close(testDone)
	defer release()

	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.AIEnabled = true
		c.AIURL = srv.URL
		c.ShowNotes = false
	})
	cfg := a.store.Get()
	act := cfg.GetAction("default-improve-writing")
	model := act.Model
	if model == "" {
		model = cfg.AIModel
	}
	oldKey := aiResultCacheKey("default-improve-writing", model, cfg.TargetLang, "the old sentence")

	a.runAction("default-improve-writing", "the old sentence")
	waitForStaleState(t, "the old pass to stream its chunk", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return strings.Contains(a.aiText, "OLD-CARD")
	})

	a.runAction("default-improve-writing", "the new sentence")
	assertNewPassInFlight(t, a)
	waitForStaleState(t, "the old pass to be cancelled", func() bool {
		select {
		case <-oldCancelled:
			return true
		default:
			return false
		}
	})
	waitForStaleState(t, "the new pass to reach the gateway", func() bool {
		select {
		case <-newStarted:
			return true
		default:
			return false
		}
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		assertNewPassInFlight(t, a)
		time.Sleep(5 * time.Millisecond)
	}

	release()
	waitForStaleState(t, "the new pass to finish", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return !a.aiBusy
	})

	a.mu.Lock()
	res := a.writingResult
	errText := a.errText
	aiText := a.aiText
	oldCached := a.cachedAIResult(oldKey)
	a.mu.Unlock()

	if res == nil || res.Improved != "NEW-CARD" {
		t.Fatalf("writingResult = %+v, want the new card", res)
	}
	if errText != "" {
		t.Errorf("errText = %q, want empty; a cancelled pass leaked its error", errText)
	}
	if strings.Contains(aiText, "OLD-CARD") {
		t.Errorf("aiText spliced two answers together: %q", aiText)
	}
	if oldCached != "" {
		t.Errorf("stale pass cached an answer under the old key: %q", oldCached)
	}
}

func TestTranslateDropsStaleAnswer(t *testing.T) {
	oldStarted := make(chan struct{}, 1)
	releaseOld := make(chan struct{})
	testDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Query().Get("q"), "OLD-SELECTION") {
			select {
			case oldStarted <- struct{}{}:
			default:
			}
			select {
			case <-releaseOld:
			case <-testDone:
				return
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, `[[["OLD-ANSWER"]],null,"en"]`)
			return
		}
		fmt.Fprint(w, `[[["NEW-ANSWER"]],null,"en"]`)
	}))
	defer srv.Close()
	defer close(testDone)

	oldURL := googleSingleURL
	googleSingleURL = srv.URL
	defer func() { googleSingleURL = oldURL }()

	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.EngineMode = "google"
		c.ShowNotes = false
		c.DictEnabled = false
		c.AutoPronounce = "off"
	})

	a.translate("OLD-SELECTION sentence")
	select {
	case <-oldStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("the first translation never reached the gateway")
	}

	a.translate("NEW-SELECTION sentence")
	close(releaseOld)

	waitForStaleState(t, "the new translation", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return !a.busy
	})

	a.mu.Lock()
	source, target, errText := a.source, a.target, a.errText
	a.mu.Unlock()
	if target != "NEW-ANSWER" {
		t.Fatalf("target = %q, want NEW-ANSWER; the stale answer won", target)
	}
	if source != "NEW-SELECTION sentence" {
		t.Errorf("source = %q, want the newer selection", source)
	}
	if errText != "" {
		t.Errorf("errText = %q, want empty", errText)
	}
}

type stubRoundTripper func(*http.Request) (*http.Response, error)

func (f stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func cannedHTTP(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func stubRaceHTTP(t *testing.T, fn stubRoundTripper) {
	t.Helper()
	old := httpClient
	httpClient = &http.Client{Transport: fn}
	t.Cleanup(func() { httpClient = old })
}

func freshBreakers(t *testing.T) {
	t.Helper()
	old := breakers.Load()
	breakers.Store(&engineBreaker{until: map[string]time.Time{}, fails: map[string]int{}})
	t.Cleanup(func() { breakers.Store(old) })
}

func assertBreakerClean(t *testing.T, name string) {
	t.Helper()
	b := currentBreaker()
	b.mu.Lock()
	fails := b.fails[name]
	_, open := b.until[name]
	b.mu.Unlock()
	if fails != 0 || open {
		t.Fatalf("a cancelled loser counted as a failure: fails=%d open=%v", fails, open)
	}
}

func TestRaceCancellationIsNotAnEngineFailure(t *testing.T) {
	t.Run("cancelled in flight", func(t *testing.T) {
		freshBreakers(t)
		loserCancelled := make(chan struct{}, 1)
		stubRaceHTTP(t, func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Host, "edge.microsoft.com") {
				select {
				case <-req.Context().Done():
					select {
					case loserCancelled <- struct{}{}:
					default:
					}
					return nil, req.Context().Err()
				case <-time.After(10 * time.Second):
					return nil, fmt.Errorf("stub loser was never cancelled")
				}
			}
			return cannedHTTP(200, `[[["GOOGLE-ANSWER"]],null,"en"]`), nil
		})

		r := TranslateRace(context.Background(), "the source sentence", "en", "xx", "race")
		if r.Err != nil || r.Text != "GOOGLE-ANSWER" {
			t.Fatalf("race = %+v, want GOOGLE-ANSWER", r)
		}
		select {
		case <-loserCancelled:
		case <-time.After(5 * time.Second):
			t.Fatal("the loser's request was never cancelled")
		}
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			assertBreakerClean(t, "microsoft")
			time.Sleep(5 * time.Millisecond)
		}
	})

	t.Run("context.Canceled error", func(t *testing.T) {
		freshBreakers(t)
		stubRaceHTTP(t, func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Host, "edge.microsoft.com") {
				return nil, context.Canceled
			}
			return cannedHTTP(200, `[[["GOOGLE-ANSWER"]],null,"en"]`), nil
		})

		r := TranslateRace(context.Background(), "the source sentence", "en", "xx", "race")
		if r.Err != nil || r.Text != "GOOGLE-ANSWER" {
			t.Fatalf("race = %+v, want GOOGLE-ANSWER", r)
		}
		assertBreakerClean(t, "microsoft")
	})
}

func TestRaceFailureIsRecordedOnce(t *testing.T) {
	freshBreakers(t)
	stubRaceHTTP(t, func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "edge.microsoft.com") {
			return cannedHTTP(429, `rate limited`), nil
		}
		return cannedHTTP(200, `[[["GOOGLE-ANSWER"]],null,"en"]`), nil
	})

	r := TranslateRace(context.Background(), "the source sentence", "en", "xx", "race")
	if r.Err != nil {
		t.Fatalf("race failed: %v", r.Err)
	}
	b := currentBreaker()
	b.mu.Lock()
	fails := b.fails["microsoft"]
	b.mu.Unlock()
	if fails != 1 {
		t.Fatalf("microsoft failures = %d, want 1 (one attempt, one record)", fails)
	}
	if b.open("microsoft") {
		t.Fatal("a single failure opened the breaker; breakerThreshold is 2")
	}
}
