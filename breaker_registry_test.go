package main

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBreakerRegistryIsAtomic(t *testing.T) {
	src := readSourceFile(t, "services.go")
	if !containsText(src, "var breakers atomic.Pointer[engineBreaker]") {
		t.Fatal("breakers 仍是可被无同步替换的普通指针；测试与在飞请求会并发读写它")
	}
	if containsText(src, "breakers = &engineBreaker") {
		t.Fatal("breakers 仍有直接赋值；应通过 atomic.Pointer.Store 替换")
	}
	if !containsText(src, "func currentBreaker() *engineBreaker") {
		t.Fatal("缺少统一的 currentBreaker() 读取点")
	}
}

func TestBreakerSwapIsRaceSafe(t *testing.T) {
	old := breakers.Load()
	t.Cleanup(func() { breakers.Store(old) })

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					currentBreaker().record("microsoft", errors.New("boom"))
				}
			}
		}()
	}
	for i := 0; i < 50; i++ {
		breakers.Store(&engineBreaker{
			until:   map[string]time.Time{},
			fails:   map[string]int{},
			lastErr: map[string]string{},
		})
		time.Sleep(time.Millisecond)
	}
	close(stop)
	wg.Wait()
}
