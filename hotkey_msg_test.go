package main

import "testing"

func TestHotkeyMsgKind(t *testing.T) {
	cases := []struct {
		kind  hotkeyMsgKind
		name  string
		isErr bool
	}{
		{hotkeyMsgNone, "none", false},
		{hotkeyMsgSuccess, "success", false},
		{hotkeyMsgError, "error", true},
		{hotkeyMsgKind(99), "unknown", false},
	}
	for _, c := range cases {
		if got := c.kind.String(); got != c.name {
			t.Errorf("%d: got %q want %q", c.kind, got, c.name)
		}
		if got := c.kind.isError(); got != c.isErr {
			t.Errorf("%s: isError got %v want %v", c.name, got, c.isErr)
		}
	}
}

func TestSetHotkeyMsgKeepsSeverity(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	a.setHotkeyMsgLocked(hotkeyMsgError, "注册全局热键失败：句柄无效")
	a.mu.Unlock()
	a.mu.Lock()
	if !a.hotkeyMsgKind.isError() {
		t.Fatal("任意注册错误都应显示为错误色，而不是靠文案里出现「冲突」「失败」才变色")
	}
	a.setHotkeyMsgLocked(hotkeyMsgSuccess, "快捷键已成功生效 ✓")
	a.mu.Unlock()
	a.mu.Lock()
	if a.hotkeyMsgKind.isError() {
		t.Fatal("成功提示不应标成错误")
	}
	a.setHotkeyMsgLocked(hotkeyMsgError, "")
	a.mu.Unlock()
	a.mu.Lock()
	if a.hotkeyMsgKind != hotkeyMsgNone {
		t.Fatal("清空文案时严重级别也应回到 none")
	}
	a.mu.Unlock()
}

// TestHotkeyMsgSeverityIsExplicit is the source contract: the colour must come
// from the recorded kind, never from searching the message text.
func TestHotkeyMsgSeverityIsExplicit(t *testing.T) {
	src := readSourceFile(t, "settings_pages.go")
	if containsText(src, `strings.Contains(a.hotkeyMsg, "冲突")`) {
		t.Fatal("热键提示仍在用文案子串推断严重级别")
	}
	if !containsText(src, "a.hotkeyMsgKind.isError()") {
		t.Fatal("热键提示应使用 hotkeyMsgKind 决定颜色")
	}
	set := readSourceFile(t, "settings.go")
	if containsText(set, "\t\ta.hotkeyMsg = ") {
		t.Fatal("settings.go 仍在直接赋值 hotkeyMsg，绕过严重级别记录")
	}
}
