package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// swapEdgeTTSAuthForTest installs a known cached token and generation counter,
// restoring the previous values when the test ends.
func swapEdgeTTSAuthForTest(t *testing.T, auth edgeTTSAuth, gen uint64) {
	t.Helper()
	edgeTTSMu.Lock()
	oldAuth, oldGen := edgeTTSAuth_, edgeTTSAuthGen
	edgeTTSAuth_, edgeTTSAuthGen = auth, gen
	edgeTTSMu.Unlock()
	t.Cleanup(func() {
		edgeTTSMu.Lock()
		edgeTTSAuth_, edgeTTSAuthGen = oldAuth, oldGen
		edgeTTSMu.Unlock()
	})
}

// swapEdgeTTSFetchForTest replaces the token handshake with a stub so the
// cache rules can be tested without network access.
func swapEdgeTTSFetchForTest(t *testing.T, fetch func(context.Context) (edgeTTSAuth, error)) {
	t.Helper()
	old := edgeTTSFetchAuthFunc
	edgeTTSFetchAuthFunc = fetch
	t.Cleanup(func() { edgeTTSFetchAuthFunc = old })
}

func cachedEdgeTTSAuthForTest() edgeTTSAuth {
	edgeTTSMu.Lock()
	defer edgeTTSMu.Unlock()
	return edgeTTSAuth_
}

func freshEdgeTTSAuthForTest() edgeTTSAuth {
	return edgeTTSAuth{token: "fresh-token", region: "eastasia", expiresAt: time.Now().Add(time.Hour)}
}

func TestEdgeTTSAuthInvalidationIsGenerationScoped(t *testing.T) {
	swapEdgeTTSAuthForTest(t, freshEdgeTTSAuthForTest(), 7)

	invalidateEdgeTTSAuth(6)
	if got := cachedEdgeTTSAuthForTest(); got.token == "" {
		t.Fatal("旧代次的 401 清掉了刚换来的新 token")
	}

	invalidateEdgeTTSAuth(7)
	if got := cachedEdgeTTSAuthForTest(); got.token != "" {
		t.Fatalf("当前代次的 401 没有清掉 token: %q", got.token)
	}
}

func TestEdgeTTSRefreshIsSerialized(t *testing.T) {
	swapEdgeTTSAuthForTest(t, edgeTTSAuth{}, 0)
	var calls int32
	swapEdgeTTSFetchForTest(t, func(context.Context) (edgeTTSAuth, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		return freshEdgeTTSAuthForTest(), nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := edgeTTSGetAuth(context.Background()); err != nil {
				t.Errorf("edgeTTSGetAuth: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("并发刷新调用了 %d 次握手，期望 1 次", got)
	}
}

func TestEdgeTTSGetAuthReusesValidToken(t *testing.T) {
	auth := freshEdgeTTSAuthForTest()
	swapEdgeTTSAuthForTest(t, auth, 3)
	var calls int32
	swapEdgeTTSFetchForTest(t, func(context.Context) (edgeTTSAuth, error) {
		atomic.AddInt32(&calls, 1)
		return edgeTTSAuth{}, nil
	})

	got, gen, err := edgeTTSGetAuth(context.Background())
	if err != nil {
		t.Fatalf("edgeTTSGetAuth: %v", err)
	}
	if got.token != auth.token || gen != 3 {
		t.Fatalf("缓存命中返回了 token=%q gen=%d，期望 token=%q gen=3", got.token, gen, auth.token)
	}
	if calls != 0 {
		t.Fatalf("有效缓存仍然触发了 %d 次握手", calls)
	}
}
