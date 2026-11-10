package main

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func swapGatewayProbe(t *testing.T, probe func(context.Context, string, string, string, string, string, func(string)) (string, error)) {
	t.Helper()
	old := streamMagpieProbe
	streamMagpieProbe = probe
	t.Cleanup(func() { streamMagpieProbe = old })
}

func waitGatewayTestSettled(t *testing.T, a *app) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		msg := a.aiTest
		a.mu.Unlock()
		if msg != "" && msg != "测试中…" {
			return msg
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("网关测试没有在预期时间内结束")
	return ""
}

func TestGatewayProbeDropsStaleResult(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls int32
	swapGatewayProbe(t, func(ctx context.Context, endpoint, model, system, user, effort string, onDelta func(string)) (string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(firstStarted)
			<-releaseFirst
			return "old answer", nil
		}
		return "pong", nil
	})

	a := newTestApp()
	a.testGateway()
	<-firstStarted
	a.testGateway()
	if msg := waitGatewayTestSettled(t, a); !strings.Contains(msg, "pong") {
		t.Fatalf("第二次探测的结果被覆盖：%q", msg)
	}
	close(releaseFirst)
	time.Sleep(80 * time.Millisecond)
	a.mu.Lock()
	got := a.aiTest
	a.mu.Unlock()
	if strings.Contains(got, "old answer") {
		t.Fatalf("旧探测结果覆盖了新结果：%q", got)
	}
}

func TestGatewayProbeCancelsPreviousRun(t *testing.T) {
	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	var calls int32
	swapGatewayProbe(t, func(ctx context.Context, endpoint, model, system, user, effort string, onDelta func(string)) (string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(firstStarted)
			<-ctx.Done()
			close(firstCanceled)
			return "", ctx.Err()
		}
		return "pong", nil
	})

	a := newTestApp()
	a.testGateway()
	<-firstStarted
	a.testGateway()
	select {
	case <-firstCanceled:
	case <-time.After(time.Second):
		t.Fatal("新一次探测没有取消上一次探测")
	}
	waitGatewayTestSettled(t, a)
}

func TestGatewayProbeIsGenerationScoped(t *testing.T) {
	src := readSourceFile(t, "settings.go")
	for _, want := range []string{"aiTestGen++", "if gen != a.aiTestGen", "aiTestCancel()"} {
		if !containsText(src, want) {
			t.Fatalf("settings.go 缺少代次保护：%q", want)
		}
	}
}
