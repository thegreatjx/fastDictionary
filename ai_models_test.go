package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func swapAIFetch(t *testing.T, fetch func(context.Context, aiConn) ([]string, error)) {
	t.Helper()
	old := fetchAvailableModelsFunc
	fetchAvailableModelsFunc = fetch
	t.Cleanup(func() { fetchAvailableModelsFunc = old })
}

func waitAIModelsSettled(t *testing.T, a *app) aiModelsState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		state := a.aiModels
		a.mu.Unlock()
		if state.phase != aiModelsLoading {
			return state
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("model fetch did not finish in time")
	return aiModelsState{}
}

func TestAIModelFetchIsSingleFlight(t *testing.T) {
	a := newTestApp()
	var calls int32
	swapAIFetch(t, func(context.Context, aiConn) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		return []string{"model-a"}, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.fetchAIModels(false)
		}()
	}
	wg.Wait()
	state := waitAIModelsSettled(t, a)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("opening settings triggered %d fetches, want 1", got)
	}
	if state.phase != aiModelsReady || len(state.models) != 1 || state.models[0] != "model-a" {
		t.Fatalf("model list was not stored: %+v", state)
	}
}

func TestAIModelFetchSkipsFreshCache(t *testing.T) {
	a := newTestApp()
	var calls int32
	swapAIFetch(t, func(context.Context, aiConn) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"fresh"}, nil
	})
	cfg := a.store.Get()
	a.mu.Lock()
	a.aiModels = aiModelsState{
		phase:  aiModelsReady,
		source: aiEndpointSource(cfg),
		models: []string{"cached"},
		at:     time.Now(),
	}
	a.mu.Unlock()

	a.fetchAIModels(false)
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("fresh cache still triggered %d fetches", got)
	}
}

func TestAIModelFetchRefetchesForOtherEndpoint(t *testing.T) {
	a := newTestApp()
	var calls int32
	swapAIFetch(t, func(context.Context, aiConn) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"new"}, nil
	})
	a.mu.Lock()
	a.aiModels = aiModelsState{
		phase:  aiModelsReady,
		source: "custom\x00http://old-gateway/v1\x00",
		models: []string{"cached"},
		at:     time.Now(),
	}
	a.mu.Unlock()

	a.fetchAIModels(false)
	state := waitAIModelsSettled(t, a)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("endpoint change did not refetch: %d", got)
	}
	if state.phase != aiModelsReady || len(state.models) != 1 || state.models[0] != "new" {
		t.Fatalf("new endpoint's models were not stored: %+v", state)
	}
}

func TestAIModelRefreshBypassesCache(t *testing.T) {
	a := newTestApp()
	var calls int32
	swapAIFetch(t, func(context.Context, aiConn) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"forced"}, nil
	})
	cfg := a.store.Get()
	a.mu.Lock()
	a.aiModels = aiModelsState{
		phase:  aiModelsReady,
		source: aiEndpointSource(cfg),
		models: []string{"cached"},
		at:     time.Now(),
	}
	a.mu.Unlock()

	a.refreshAIModels()
	waitAIModelsSettled(t, a)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("manual refresh did not bypass the cache: %d", got)
	}
}

func TestAIModelStaleResultIsDropped(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls int32
	swapAIFetch(t, func(_ context.Context, conn aiConn) ([]string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(firstStarted)
			<-releaseFirst
			return []string{"old"}, nil
		}
		return []string{"new"}, nil
	})

	a := newTestApp()
	a.fetchAIModels(false)
	<-firstStarted
	a.store.Update(func(c *Config) { c.AIURL = "https://example.com/v1"; c.AIProvider = "custom" })
	a.fetchAIModels(false)
	close(releaseFirst)

	state := waitAIModelsSettled(t, a)
	deadline := time.Now().Add(time.Second)
	for {
		a.mu.Lock()
		state = a.aiModels
		a.mu.Unlock()
		if state.phase != aiModelsLoading {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("model fetch did not settle")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if state.phase != aiModelsReady || len(state.models) != 1 || state.models[0] != "new" {
		t.Fatalf("stale fetch overwrote the newer source: %+v", state)
	}
}

func TestAIModelUnsupportedIsExplicit(t *testing.T) {
	a := newTestApp()
	swapAIFetch(t, func(context.Context, aiConn) ([]string, error) {
		return nil, errAIModelsUnsupported
	})
	a.fetchAIModels(true)
	state := waitAIModelsSettled(t, a)
	if state.phase != aiModelsUnsupported {
		t.Fatalf("unsupported /models should be its own state: %+v", state)
	}
}

func TestGetAIModelsDoesNotLeakMagpieFallbackToCustom(t *testing.T) {
	a := newTestApp()
	a.store.Update(func(c *Config) {
		c.AIProvider = "custom"
		c.AIURL = "https://example.com/v1"
		c.AIModel = "my-model"
	})
	models := a.getAIModels(a.store.Get())
	if len(models) != 1 || models[0] != "my-model" {
		t.Fatalf("custom service got a foreign model list: %v", models)
	}
}

func TestGetAIModelsKeepsMagpieFallback(t *testing.T) {
	a := newTestApp()
	models := a.getAIModels(a.store.Get())
	if len(models) != len(defaultMagpieModels) {
		t.Fatalf("Magpie fallback list = %v", models)
	}
}
