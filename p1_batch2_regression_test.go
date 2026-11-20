package main

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatchHookHotPathsSurvivePanicAndCoalesce(t *testing.T) {
	prevDown, prevWheel, prevKey := OnMouseDown, OnMouseWheel, OnKeyDown
	defer func() {
		OnMouseDown = prevDown
		OnMouseWheel = prevWheel
		OnKeyDown = prevKey
	}()

	downDone := make(chan [2]int32, 4)
	OnMouseDown = func(x, y int32) {
		if x == 1 {
			panic("mouse down boom")
		}
		downDone <- [2]int32{x, y}
	}
	dispatchMouseDown(1, 2)
	dispatchMouseDown(3, 4)
	select {
	case got := <-downDone:
		if got != [2]int32{3, 4} {
			t.Fatalf("unexpected mouseDown coords: %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mouseDown callback after panic never ran")
	}

	wheelDone := make(chan [2]int32, 16)
	OnMouseWheel = func(x, y int32) {
		if x == 9 {
			panic("wheel boom")
		}
		wheelDone <- [2]int32{x, y}
	}
	dispatchMouseWheel(9, 9)
	time.Sleep(30 * time.Millisecond)
	dispatchMouseWheel(21, 22)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case got := <-wheelDone:
			if got == [2]int32{21, 22} {
				goto wheelOK
			}
		case <-deadline:
			t.Fatal("mouseWheel worker stopped after panic")
		}
	}
wheelOK:

	keyDone := make(chan uint32, 16)
	OnKeyDown = func(vk uint32) {
		if vk == 13 {
			panic("key boom")
		}
		keyDone <- vk
	}
	dispatchKeyDown(13)
	time.Sleep(30 * time.Millisecond)
	dispatchKeyDown(65)
	deadline = time.After(2 * time.Second)
	for {
		select {
		case got := <-keyDone:
			if got == 65 {
				return
			}
		case <-deadline:
			t.Fatal("keyDown worker stopped after panic")
		}
	}
}

func TestQueryCacheDebouncedPersistAndFlush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "query_cache.json")
	qc := newQueryCache(path)
	qc.persistDelay = 80 * time.Millisecond

	var reads atomic.Int32
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("k-%d", i)
		qc.putTranslation(key, cachedTranslation{Target: fmt.Sprintf("v-%d", i), Engine: "微软"})
		if _, ok := qc.getTranslation(key); ok {
			reads.Add(1)
		}
	}
	if reads.Load() != 20 {
		t.Fatalf("expected 20 immediate reads, got %d", reads.Load())
	}
	qc.flush()

	reloaded := newQueryCache(path)
	if tr, ok := reloaded.getTranslation("k-19"); !ok || tr.Target != "v-19" {
		t.Fatalf("expected k-19 persisted after flush, got ok=%v tr=%+v", ok, tr)
	}
}
