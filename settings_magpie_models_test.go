package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// swapMagpieFetchForTest replaces the gateway model probe with a stub.
func swapMagpieFetchForTest(t *testing.T, fetch func(string) ([]string, error)) {
	t.Helper()
	old := fetchAvailableModelsFunc
	fetchAvailableModelsFunc = fetch
	t.Cleanup(func() { fetchAvailableModelsFunc = old })
}

// resetMagpieModelsForTest clears the cached model list and restores it when
// the test ends.
func resetMagpieModelsForTest(t *testing.T, a *app) {
	t.Helper()
	a.mu.Lock()
	oldModels, oldURL, oldAt, oldGen, oldLoading := a.magpieModels, a.magpieModelsURL, a.magpieModelsLoadedAt, a.magpieModelsGen, a.loadingModels
	a.magpieModels, a.magpieModelsURL, a.magpieModelsLoadedAt, a.magpieModelsGen, a.loadingModels = nil, "", time.Time{}, 0, false
	a.mu.Unlock()
	t.Cleanup(func() {
		a.mu.Lock()
		a.magpieModels, a.magpieModelsURL, a.magpieModelsLoadedAt, a.magpieModelsGen, a.loadingModels = oldModels, oldURL, oldAt, oldGen, oldLoading
		a.mu.Unlock()
	})
}

// waitMagpieIdle waits until the background probe has finished, so a test
// cannot restore the fetch stub while its goroutine is still running.
func waitMagpieIdle(t *testing.T, a *app) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		a.mu.Lock()
		busy := a.loadingModels
		a.mu.Unlock()
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("模型探测没有结束")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestMagpieModelFetchIsSingleFlight(t *testing.T) {
	a := newTestApp()
	resetMagpieModelsForTest(t, a)
	var calls int32
	swapMagpieFetchForTest(t, func(string) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		return []string{"model-a"}, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.loadMagpieModels()
		}()
	}
	wg.Wait()
	waitMagpieIdle(t, a)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("重复打开设置触发了 %d 次模型探测，期望 1 次", got)
	}
	deadline := time.Now().Add(time.Second)
	for {
		a.mu.Lock()
		models := len(a.magpieModels)
		a.mu.Unlock()
		if models == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("模型列表没有写回缓存: %d", models)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestMagpieModelFetchSkipsFreshCache(t *testing.T) {
	a := newTestApp()
	resetMagpieModelsForTest(t, a)
	var calls int32
	swapMagpieFetchForTest(t, func(string) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"fresh"}, nil
	})

	a.mu.Lock()
	a.magpieModels = []string{"cached"}
	a.magpieModelsURL = a.store.Get().AIURL
	a.magpieModelsLoadedAt = time.Now()
	a.mu.Unlock()

	a.loadMagpieModels()
	waitMagpieIdle(t, a)
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("五分钟内的有效缓存仍然触发了 %d 次探测", got)
	}
}

func TestMagpieModelFetchRefetchesForOtherGateway(t *testing.T) {
	a := newTestApp()
	resetMagpieModelsForTest(t, a)
	var calls int32
	swapMagpieFetchForTest(t, func(string) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"new"}, nil
	})

	a.mu.Lock()
	a.magpieModels = []string{"cached"}
	a.magpieModelsURL = "http://old-gateway/v1"
	a.magpieModelsLoadedAt = time.Now()
	a.mu.Unlock()

	a.loadMagpieModels()
	waitMagpieIdle(t, a)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("网关地址变化后没有重新探测: %d", got)
	}
}

func TestMagpieModelRefreshBypassesCache(t *testing.T) {
	a := newTestApp()
	resetMagpieModelsForTest(t, a)
	var calls int32
	swapMagpieFetchForTest(t, func(string) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"forced"}, nil
	})

	a.mu.Lock()
	a.magpieModels = []string{"cached"}
	a.magpieModelsURL = a.store.Get().AIURL
	a.magpieModelsLoadedAt = time.Now()
	a.mu.Unlock()

	a.refreshMagpieModels()
	waitMagpieIdle(t, a)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("手动刷新没有绕过缓存: %d", got)
	}
}
