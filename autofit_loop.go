package main

// autofitLoop 决定是否应用一次高度修正。
//
// 旧实现按 delta 的正负判断震荡：方向一反就锁死。这条规则把正常操作也
// 挡掉了 —— 用户折叠生词面板时内容突然变矮，delta 从正变负，窗口于是停在
// 展开时的高度，下方留一整块空白。
//
// 现在的收敛条件是「上次请求有没有真的改变窗口」：
//   - 窗口跟着变了，说明链路通着，允许下一次修正，方向不论；
//   - 窗口没变（被最大化上限或最小高度夹住），重试只会刷屏，锁定；
//   - 内容高度自己变了（用户折叠、流式回答增长），解锁重来。
//
// 这就是 Web 那边 ResizeObserver 的行为：内容变了就量一次，量完设置，
// 设置后没变化就停，从不因为「这次是变大还是变小」而放弃。
type autofitLoop struct {
	locked      bool
	requested   bool    // 上一次调用发出了修正，窗口高度还没被确认
	requestH    int     // 发出修正时的窗口高度
	lockContent float32 // 锁住时的内容高度，用来发现内容自己变了
}

// decide 返回要应用的 delta，以及是否应用。contentH 是本次测量的内容高度。
func (l *autofitLoop) decide(delta float32, ok bool, curH int, contentH float32) (float32, bool) {
	if !ok {
		return 0, false
	}

	// 内容自己变了，之前的锁定失效：新的一轮可以从任何方向开始。
	if l.locked {
		if diff := contentH - l.lockContent; diff > 8 || diff < -8 {
			l.locked, l.requested = false, false
		} else {
			return 0, false
		}
	}

	// 上一次修正发出后窗口高度没有变化：被夹住了，再试也是同一个结果。
	if l.requested && curH == l.requestH {
		l.locked = true
		l.lockContent = contentH
		return 0, false
	}

	l.requestH = curH
	l.requested = true
	return delta, true
}

// confirm 把「窗口已经跟着变了」记下来，让下一轮允许新的修正。
func (l *autofitLoop) confirm(curH int) {
	if l.requested && curH != l.requestH {
		l.requested = false
	}
}

// reset 在内容确实换了一份时调用（换页、打开新卡片、落地新结果）。
func (l *autofitLoop) reset() {
	l.locked, l.requested = false, false
}
