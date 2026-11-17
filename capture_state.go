package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

type captureStrategy uint8

const (
	captureAuto captureStrategy = iota
	captureCopyOnly
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
