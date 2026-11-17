package main

// 上一轮 resize 请求与锁定的内容高度，用来确认窗口是否真的变了。
type autofitLoop struct {
	locked      bool
	requested   bool
	requestH    int
	lockContent float32
}

func (l *autofitLoop) decide(delta float32, ok bool, curH int, contentH float32) (float32, bool) {
	if !ok {
		return 0, false
	}

	if l.locked {
		if diff := contentH - l.lockContent; diff > 8 || diff < -8 {
			l.locked, l.requested = false, false
		} else {
			return 0, false
		}
	}

	if l.requested && curH == l.requestH {
		l.locked = true
		l.lockContent = contentH
		return 0, false
	}

	l.requestH = curH
	l.requested = true
	return delta, true
}

func (l *autofitLoop) confirm(curH int) {
	if l.requested && curH != l.requestH {
		l.requested = false
	}
}

func (l *autofitLoop) reset() {
	l.locked, l.requested = false, false
}
