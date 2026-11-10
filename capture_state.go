package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

// captureStrategy is the parsed form of the 取词方式 preference. The raw field
// is a string, and the two capture entry points used to interpret it
// differently: the hotkey path honoured "copy" while the gesture path always
// tried UI Automation first. One parse, one meaning.
type captureStrategy uint8

const (
	// captureAuto asks UI Automation first and falls back to a simulated copy.
	captureAuto captureStrategy = iota
	// captureCopyOnly never touches UI Automation; some apps expose a tree that
	// looks valid but returns the wrong text.
	captureCopyOnly
	// captureUIAOnly never synthesises Ctrl+C, even when UI Automation is empty.
	captureUIAOnly
)

func (s captureStrategy) String() string {
	switch s {
	case captureAuto:
		return "auto"
	case captureCopyOnly:
		return "copy"
	case captureUIAOnly:
		return "uia"
	default:
		return "unknown"
	}
}

func parseCaptureStrategy(raw string) captureStrategy {
	switch raw {
	case "copy":
		return captureCopyOnly
	case "uia":
		return captureUIAOnly
	default:
		return captureAuto
	}
}

// capturePlan splits the strategy into the two decisions the callers make.
func (s captureStrategy) capturePlan() (tryUIA, allowCopy bool) {
	switch s {
	case captureCopyOnly:
		return false, true
	case captureUIAOnly:
		return true, false
	default:
		return true, true
	}
}

// clipboardTransaction owns the "clipboard is temporarily borrowed" state.
// The restore is deferred by the caller, so an early return, a panic or a
// killed goroutine can no longer leave the user's clipboard empty.
type clipboardTransaction struct {
	snapshot  transfer.Data
	previous  string
	restored  bool
	restoreFn func(transfer.Data, string)
}

func beginClipboardTransaction() *clipboardTransaction {
	return &clipboardTransaction{
		snapshot:  snapshotClipboard(),
		previous:  mygo.Clipboard.ReadText(),
		restoreFn: restoreClipboard,
	}
}

func (tx *clipboardTransaction) restore() {
	if tx == nil || tx.restored {
		return
	}
	tx.restored = true
	if tx.restoreFn != nil {
		tx.restoreFn(tx.snapshot, tx.previous)
		return
	}
	restoreClipboard(tx.snapshot, tx.previous)
}
