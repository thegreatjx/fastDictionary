package main

// hotkeyMsgKind records why the 快捷键 message is on screen. Reading the
// severity back out of the message text broke as soon as a Windows
// registration error did not contain the literal words "冲突" or "失败".
type hotkeyMsgKind uint8

const (
	hotkeyMsgNone hotkeyMsgKind = iota
	hotkeyMsgSuccess
	hotkeyMsgError
)

func (k hotkeyMsgKind) String() string {
	switch k {
	case hotkeyMsgNone:
		return "none"
	case hotkeyMsgSuccess:
		return "success"
	case hotkeyMsgError:
		return "error"
	default:
		return "unknown"
	}
}

func (k hotkeyMsgKind) isError() bool {
	return k == hotkeyMsgError
}

// setHotkeyMsgLocked records the message together with its severity. The
// caller holds a.mu, the same way the message was already written.
func (a *app) setHotkeyMsgLocked(kind hotkeyMsgKind, text string) {
	a.hotkeyMsg = text
	if text == "" {
		kind = hotkeyMsgNone
	}
	a.hotkeyMsgKind = kind
}
