package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
)

const (
	logMaxBytes     int64 = 2 << 20
	logBackupSuffix       = ".1"
)

// 超过 maxBytes 轮转一次，避免长期运行撑满磁盘。
func openLogFile(path string, maxBytes int64) (*os.File, error) {
	if info, err := os.Stat(path); err == nil && info.Size() >= maxBytes {
		backup := path + logBackupSuffix
		_ = os.Remove(backup)
		_ = os.Rename(path, backup)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

func recoverLog(where string) {
	if r := recover(); r != nil {
		log.Printf("fastDictionary: panic in %s: %v\n%s", where, r, debug.Stack())
	}
}

func (a *app) logText(s string, n int) string {
	if a != nil && a.store != nil && a.store.Get().LogSelectionText {
		return fmt.Sprintf("%q", truncateFor(s, n))
	}
	return fmt.Sprintf("<%d 字符，未记录>", len([]rune(s)))
}
