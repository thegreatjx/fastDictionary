package main

import (
	"sync"
	"testing"
)

// TestQuittingStateIsExplicitAndRaceSafe pins the fix for C5: isQuitting used
// to be a plain bool written under a.mu in one place, written bare in
// OnBeforeQuit, and read bare in the settings window's OnClose. Three
// synchronisation contracts for one flag is a data race waiting to happen.
// The flag is now an atomic.Bool with a single Store/Load contract.
//
// A real cross-callback race cannot be reproduced deterministically in a unit
// test (those callbacks run on mygo's window message loop), so this is a
// source-level contract test plus go test -race on the concurrent test below.
func TestQuittingStateIsExplicitAndRaceSafe(t *testing.T) {
	popup := readSourceFile(t, "popup.go")
	if !containsText(popup, "atomic.Bool") || !containsText(popup, `"sync/atomic"`) {
		t.Fatal("popup.go 未把 isQuitting 声明为 atomic.Bool / 未引入 sync/atomic")
	}
	if containsText(popup, "isQuitting   bool") {
		t.Fatal("isQuitting 仍是普通 bool")
	}

	main := readSourceFile(t, "main.go")
	if n := countOccurrences(main, "a.isQuitting = true"); n != 0 {
		t.Fatalf("main.go 仍有 %d 处裸写 isQuitting，退出流程存在数据竞争", n)
	}
	if n := countOccurrences(main, "a.isQuitting.Store(true)"); n != 2 {
		t.Fatalf("main.go 应有 2 处 Store(true)（quitApp 与 OnBeforeQuit），实际 %d 处", n)
	}

	settings := readSourceFile(t, "settings.go")
	if containsText(settings, "if a.isQuitting {") {
		t.Fatal("settings.go 仍裸读 isQuitting")
	}
	if n := countOccurrences(settings, "a.isQuitting.Load()"); n != 1 {
		t.Fatalf("settings.go 应有 1 处 Load()（OnClose），实际 %d 处", n)
	}
}

// TestQuittingAtomicConcurrentLoadStore exercises the atomic flag from many
// goroutines. Under go test -race this is what proves the Store/Load contract
// is race-free; a plain bool with mixed locked/bare access could not pass here.
func TestQuittingAtomicConcurrentLoadStore(t *testing.T) {
	a := newTestApp()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			a.isQuitting.Store(true)
		}()
		go func() {
			defer wg.Done()
			_ = a.isQuitting.Load()
		}()
	}
	wg.Wait()
	if !a.isQuitting.Load() {
		t.Fatal("退出标志在并发写入后应已置位")
	}
}
