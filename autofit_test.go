package main

import "testing"

// TestAutofitLocksWhenWindowDoesNotMove 替换了旧的「方向反转即震荡」规则。
//
// 旧测试断言「收缩后要求增长的修正必须丢弃」，把用户折叠生词面板这种正常
// 操作也当成了抖动：窗口停在展开时的高度，下方留一整块空白。真正的收敛条件
// 是窗口有没有跟着动 —— 到上限或被最小高度夹住时窗口不动，那才该停。
func TestAutofitLocksWhenWindowDoesNotMove(t *testing.T) {
	var l autofitLoop

	if d, ok := l.decide(-43.9, true, 580, 700); !ok || d != -43.9 {
		t.Fatalf("首次修正应被采纳：delta=%v ok=%v", d, ok)
	}
	// 窗口仍是 580，说明上一次修正没有生效：锁定，别再每帧重试。
	if _, ok := l.decide(-43.9, true, 580, 700); ok {
		t.Fatal("窗口没动时应锁定")
	}
	// 内容自己变了（折叠面板 / 流式增长）：解锁，方向不论。
	if d, ok := l.decide(20.8, true, 580, 400); !ok || d != 20.8 {
		t.Fatalf("内容变化后应解锁并采纳新方向：delta=%v ok=%v", d, ok)
	}
}

// TestAutofitResumesAfterReset：fitHeight 换了一份内容时调用 reset。
func TestAutofitResumesAfterReset(t *testing.T) {
	var l autofitLoop
	l.decide(-40, true, 500, 600)
	if _, ok := l.decide(-40, true, 500, 600); ok {
		t.Fatal("应锁定")
	}

	l.reset()

	if d, ok := l.decide(40, true, 500, 400); !ok || d != 40 {
		t.Fatalf("reset 后应采纳任何方向的修正：delta=%v ok=%v", d, ok)
	}
}

// TestAutofitIgnoresFramesInsideTheDeadZone：差值在容差内的帧不该动窗口。
func TestAutofitIgnoresFramesInsideTheDeadZone(t *testing.T) {
	var l autofitLoop
	l.decide(-43, true, 500, 600)

	if _, ok := l.decide(0, false, 500, 600); ok {
		t.Fatal("死区内的帧不该应用修正")
	}
}
