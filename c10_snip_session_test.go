package main

import (
	"strings"
	"testing"
)

// TestStartNativeSnipGuardsSingleton pins the session-boundary contract for the
// native snipping overlay. The overlay owns an OS thread and a Win32 message
// pump, so a headless test cannot drive its windows; the boundary guard itself
// is ordinary Go and can be asserted structurally.
func TestStartNativeSnipGuardsSingleton(t *testing.T) {
	src := readSourceFile(t, "snip.go")

	if !containsText(src, "func beginSnipSessionLocked(") {
		t.Fatal("缺少 beginSnipSessionLocked：连按截图仍会覆盖全局 session")
	}
	if !containsText(src, "func endSnipSessionLocked(") {
		t.Fatal("缺少 endSnipSessionLocked：早退路径仍会残留全局 session")
	}

	start := strings.Index(src, "func StartNativeSnip(")
	if start < 0 {
		t.Fatal("StartNativeSnip not found")
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		t.Fatal("StartNativeSnip body not delimited")
	}
	body := src[start : start+end]

	if !containsText(body, "beginSnipSessionLocked(") {
		t.Fatal("StartNativeSnip 没有通过守卫声明会话")
	}
	if countOccurrences(body, "currentSnip = session") != 0 {
		t.Fatal("StartNativeSnip 仍然直接给全局 session 赋值")
	}
	if !containsText(body, "defer") || !containsText(body, "endSnipSessionLocked(") {
		t.Fatal("StartNativeSnip 没有用 defer 覆盖所有早退路径")
	}

	// snipWndProc must still read the session pointer under snipMu.
	wnd := strings.Index(src, "func snipWndProc(")
	if wnd < 0 {
		t.Fatal("snipWndProc not found")
	}
	wndBody := src[wnd : wnd+600]
	lockAt := strings.Index(wndBody, "snipMu.Lock()")
	ptrAt := strings.Index(wndBody, "s := currentSnip")
	if lockAt < 0 || ptrAt < 0 || lockAt > ptrAt {
		t.Fatal("snipWndProc 不再在 snipMu 下读取 currentSnip")
	}
}

// TestBeginSnipSessionLocked exercises the boundary as an ordinary Go function,
// with no Win32 message pump. It walks one session's lifetime and isolates the
// package-level currentSnip so other tests are unaffected.
func TestBeginSnipSessionLocked(t *testing.T) {
	snipMu.Lock()
	saved := currentSnip
	currentSnip = nil
	snipMu.Unlock()
	t.Cleanup(func() {
		snipMu.Lock()
		currentSnip = saved
		snipMu.Unlock()
	})

	s1 := &snipSession{}
	s2 := &snipSession{}

	snipMu.Lock()
	okFirst := beginSnipSessionLocked(s1)
	gotFirst := currentSnip
	snipMu.Unlock()
	if !okFirst || gotFirst != s1 {
		t.Fatalf("首个会话应成功占位：ok=%v got=%p want=%p", okFirst, gotFirst, s1)
	}

	snipMu.Lock()
	okSecond := beginSnipSessionLocked(s2)
	gotSecond := currentSnip
	snipMu.Unlock()
	if okSecond || gotSecond != s1 {
		t.Fatalf("连按应被拒绝且不覆盖旧会话：ok=%v got=%p want=false/%p", okSecond, gotSecond, s1)
	}

	snipMu.Lock()
	endSnipSessionLocked(s2) // 错配 session 不应误清
	gotMismatch := currentSnip
	snipMu.Unlock()
	if gotMismatch != s1 {
		t.Fatalf("错配清理不应清掉在跑的会话：got=%p want=%p", gotMismatch, s1)
	}

	snipMu.Lock()
	endSnipSessionLocked(s1)
	gotEnd := currentSnip
	snipMu.Unlock()
	if gotEnd != nil {
		t.Fatalf("持有者结束后应清空：got=%p want=nil", gotEnd)
	}

	snipMu.Lock()
	okAgain := beginSnipSessionLocked(s1)
	gotAgain := currentSnip
	endSnipSessionLocked(s1) // 重复清理应保持幂等
	endSnipSessionLocked(s1)
	gotIdempotent := currentSnip
	snipMu.Unlock()
	if !okAgain || gotAgain != s1 {
		t.Fatalf("结束后应能重新占位：ok=%v got=%p want=true/%p", okAgain, gotAgain, s1)
	}
	if gotIdempotent != nil {
		t.Fatalf("重复 end 应保持已清空：got=%p want=nil", gotIdempotent)
	}
}
