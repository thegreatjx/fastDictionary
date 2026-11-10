package main

import (
	"errors"
	"testing"
	"time"
)

func resetIndraForTest(t *testing.T) {
	t.Helper()
	indraMu.Lock()
	oldInst, oldErr, oldLoaded, oldFailedAt := indraInst, indraErr, indraLoaded, indraFailedAt
	oldLoader := indraLoadEngine
	indraInst, indraErr, indraLoaded, indraFailedAt = nil, nil, false, time.Time{}
	indraMu.Unlock()
	t.Cleanup(func() {
		indraMu.Lock()
		indraInst, indraErr, indraLoaded, indraFailedAt = oldInst, oldErr, oldLoaded, oldFailedAt
		indraLoadEngine = oldLoader
		indraMu.Unlock()
	})
}

// TestIndraRetriesAfterCooldown is the regression: the first failed load was
// cached for the whole process, so a transient problem (model file locked,
// memory pressure) turned OCR off until the user restarted.
func TestIndraRetriesAfterCooldown(t *testing.T) {
	resetIndraForTest(t)
	calls := 0
	indraLoadEngine = func() (*indraEngine, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("模型文件被占用")
		}
		return &indraEngine{}, nil
	}

	if _, err := indraGet(); err == nil {
		t.Fatal("第一次加载应失败")
	}
	if _, err := indraGet(); err == nil {
		t.Fatal("冷却期内应继续返回上次的错误")
	}
	if calls != 1 {
		t.Fatalf("冷却期内不应重复加载，loader 调用 %d 次", calls)
	}

	indraMu.Lock()
	indraFailedAt = time.Now().Add(-indraRetryCooldown - time.Second)
	indraMu.Unlock()

	if _, err := indraGet(); err != nil {
		t.Fatalf("冷却结束后应重试并成功，得到 %v", err)
	}
	if calls != 2 {
		t.Fatalf("冷却结束后应重新加载一次，loader 调用 %d 次", calls)
	}
}

func TestIndraSuccessIsNotReloaded(t *testing.T) {
	resetIndraForTest(t)
	calls := 0
	indraLoadEngine = func() (*indraEngine, error) {
		calls++
		return &indraEngine{}, nil
	}
	for i := 0; i < 3; i++ {
		if _, err := indraGet(); err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i+1, err)
		}
	}
	if calls != 1 {
		t.Fatalf("成功后不应重复加载，loader 调用 %d 次", calls)
	}
}
