package main

import "testing"

func TestAutofitLoopConverges(t *testing.T) {
	t.Run("折叠后缩小能被应用，不因方向反转被挡", func(t *testing.T) {
		var l autofitLoop

		if d, ok := l.decide(141.5, true, 580, 614.6); !ok || d != 141.5 {
			t.Fatalf("首次应应用增长，得到 (%v, %v)", d, ok)
		}
		if _, ok := l.decide(141.5, true, 580, 614.6); ok {
			t.Error("窗口没跟着变时应锁定，否则每帧重复同一个请求")
		}

		d, ok := l.decide(-261.6, true, 580, 211.5)
		if !ok || d != -261.6 {
			t.Errorf("折叠面板后应允许缩小，得到 (%v, %v)：方向反转不是震荡", d, ok)
		}
	})

	t.Run("窗口跟着变了就允许下一轮，方向不论", func(t *testing.T) {
		var l autofitLoop
		if _, ok := l.decide(100, true, 400, 500); !ok {
			t.Fatal("首次应应用")
		}
		l.confirm(450)
		d, ok := l.decide(-50, true, 450, 400)
		if !ok || d != -50 {
			t.Errorf("窗口已响应，下一轮应允许缩小，得到 (%v, %v)", d, ok)
		}
	})

	t.Run("内容没变且窗口没变时保持锁定", func(t *testing.T) {
		var l autofitLoop
		l.decide(200, true, 580, 700)
		if _, ok := l.decide(200, true, 580, 700); ok {
			t.Fatal("应锁定")
		}
		for i := 0; i < 5; i++ {
			if _, ok := l.decide(200, true, 580, 700); ok {
				t.Fatal("内容与窗口都没变，不应再重试")
			}
		}
	})

	t.Run("测量无效时不应用", func(t *testing.T) {
		var l autofitLoop
		if _, ok := l.decide(50, false, 580, 700); ok {
			t.Error("ok=false 表示差值在容差内，不该应用")
		}
	})
}
