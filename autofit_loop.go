package main

const autofitContentThreshold float32 = 24

type autofitLoop struct {
	locked      bool
	requested   bool
	requestH    int
	lockContent float32
	lastContent float32
	steps       int
	lastSign    int
}

func (l *autofitLoop) decide(delta float32, ok bool, curH int, contentH float32) (float32, bool) {
	if !ok {
		return 0, false
	}

	if l.locked {
		diff := contentH - l.lockContent
		if diff > autofitContentThreshold || diff < -autofitContentThreshold {
			l.locked, l.requested = false, false
			l.steps, l.lastSign = 0, 0
		} else {
			return 0, false
		}
	}

	if l.requested && curH == l.requestH {
		l.locked = true
		l.lockContent = contentH
		l.lastContent = contentH
		l.requested = false
		l.steps, l.lastSign = 0, 0
		return 0, false
	}

	contentDiff := contentH - l.lastContent
	if contentDiff > autofitContentThreshold || contentDiff < -autofitContentThreshold {
		l.steps, l.lastSign = 0, 0
	}

	sign := 1
	if delta < 0 {
		sign = -1
	}
	if (l.lastSign != 0 && sign != l.lastSign) || l.steps >= 4 {
		l.locked = true
		l.lockContent = contentH
		l.lastContent = contentH
		l.requested = false
		l.steps, l.lastSign = 0, 0
		return 0, false
	}

	l.lastContent = contentH
	l.lastSign = sign
	l.steps++
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
	l.steps, l.lastSign = 0, 0
	l.lockContent, l.lastContent = 0, 0
}
