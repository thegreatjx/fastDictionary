package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func swapAIProbe(t *testing.T, probe func(context.Context, aiConn, string, string, string, func(string)) (string, error)) {
	t.Helper()
	old := aiProbeFunc
	aiProbeFunc = probe
	t.Cleanup(func() { aiProbeFunc = old })
}

func waitAIProbeSettled(t *testing.T, a *app) aiProbeState {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		state := a.aiProbe
		a.mu.Unlock()
		if state.phase != aiProbeRunning {
			return state
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("AI probe did not finish in time")
	return aiProbeState{}
}

func TestAIProbeDropsStaleResult(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls int32
	swapAIProbe(t, func(ctx context.Context, conn aiConn, system, user, effort string, onDelta func(string)) (string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(firstStarted)
			<-releaseFirst
			return "old answer", nil
		}
		return "pong", nil
	})

	a := newTestApp()
	a.testAIConnection()
	<-firstStarted
	a.testAIConnection()
	state := waitAIProbeSettled(t, a)
	if state.phase != aiProbeSucceeded || !strings.Contains(state.msg, "pong") {
		t.Fatalf("second probe result was not kept: %+v", state)
	}
	close(releaseFirst)
	time.Sleep(80 * time.Millisecond)

	a.mu.Lock()
	got := a.aiProbe
	a.mu.Unlock()
	if strings.Contains(got.msg, "old answer") {
		t.Fatalf("stale probe overwrote the newer result: %+v", got)
	}
}

func TestAIProbeCancelsPreviousRun(t *testing.T) {
	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	var calls int32
	swapAIProbe(t, func(ctx context.Context, conn aiConn, system, user, effort string, onDelta func(string)) (string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(firstStarted)
			<-ctx.Done()
			close(firstCanceled)
			return "", ctx.Err()
		}
		return "pong", nil
	})

	a := newTestApp()
	a.testAIConnection()
	<-firstStarted
	a.testAIConnection()
	select {
	case <-firstCanceled:
	case <-time.After(time.Second):
		t.Fatal("new probe did not cancel the previous one")
	}
	if state := waitAIProbeSettled(t, a); state.phase != aiProbeSucceeded {
		t.Fatalf("second probe did not succeed: %+v", state)
	}
}

func TestAIProbeSourceChangeHidesOldResult(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	swapAIProbe(t, func(ctx context.Context, conn aiConn, system, user, effort string, onDelta func(string)) (string, error) {
		close(firstStarted)
		<-releaseFirst
		return "old answer", nil
	})

	a := newTestApp()
	a.store.Update(func(c *Config) { c.AIModel = "model-a" })
	a.testAIConnection()
	<-firstStarted
	a.store.Update(func(c *Config) { c.AIModel = "model-b" })
	close(releaseFirst)

	deadline := time.Now().Add(time.Second)
	for {
		a.mu.Lock()
		phase := a.aiProbe.phase
		a.mu.Unlock()
		if phase != aiProbeRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("probe did not finish")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if state := a.aiProbeStatus(a.store.Get()); state.phase != aiProbeIdle || state.msg != "" {
		t.Fatalf("result from model-a is visible for model-b: %+v", state)
	}
}

func TestAIProbeUsesRealStreamAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.AIProvider = "custom"
		c.AIURL = srv.URL + "/v1"
		c.AIModel = "m"
		c.AIKey = "sk-test"
	})
	a.testAIConnection()
	state := waitAIProbeSettled(t, a)
	if state.phase != aiProbeSucceeded || !strings.Contains(state.msg, "pong") {
		t.Fatalf("probe did not use the real client: %+v", state)
	}
}

func TestAIProbeRefusesEmptyModel(t *testing.T) {
	called := false
	swapAIProbe(t, func(ctx context.Context, conn aiConn, system, user, effort string, onDelta func(string)) (string, error) {
		called = true
		return "pong", nil
	})
	a := newTestApp()
	a.store.Update(func(c *Config) { c.AIModel = "" })
	a.testAIConnection()
	state := a.aiProbeStatus(a.store.Get())
	if called {
		t.Fatal("probe sent a request with an empty model")
	}
	if state.phase != aiProbeFailed || !strings.Contains(state.msg, "模型") {
		t.Fatalf("empty model did not produce a clear failure: %+v", state)
	}
}
