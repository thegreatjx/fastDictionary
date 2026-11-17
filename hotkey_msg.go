package main

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

func (a *app) setHotkeyMsgLocked(kind hotkeyMsgKind, text string) {
	a.hotkeyMsg = text
	if text == "" {
		kind = hotkeyMsgNone
	}
	a.hotkeyMsgKind = kind
}
