package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestLangChoicesAreComplete pins the two lists the pickers are built from: the
// full option list is exactly the primary five plus the region groups, every
// code has a label, and no label is used twice (langFromLabel reverses the map
// by scanning labels, so a duplicate would silently pick the wrong language).
func TestLangChoicesAreComplete(t *testing.T) {
	var want []string
	want = append(want, langPrimary...)
	for _, group := range langMoreGroups {
		want = append(want, group...)
	}
	if strings.Join(langOptions, ",") != strings.Join(want, ",") {
		t.Fatalf("langOptions 与 langPrimary+langMoreGroups 不一致：\n got %v\nwant %v", langOptions, want)
	}

	seen := map[string]bool{}
	labels := map[string]string{}
	for _, code := range langOptions {
		if seen[code] {
			t.Errorf("语言 %q 重复出现", code)
		}
		seen[code] = true
		label := langLabels[code]
		if label == "" {
			t.Errorf("语言 %q 没有中文名", code)
			continue
		}
		if other, dup := labels[label]; dup {
			t.Errorf("中文名 %q 同时属于 %q 和 %q", label, other, code)
		}
		labels[label] = code
	}
}

// TestLangMenuHidesTheRestBehindOneSubmenu pins the fix for the 34-row menu:
// the top level offers the primary five plus a single 更多语言 submenu, and no
// picker builds its own flat list any more.
func TestLangMenuHidesTheRestBehindOneSubmenu(t *testing.T) {
	settings := readSourceFile(t, "settings.go")
	build := settings[strings.Index(settings, "func buildLangMenu("):]
	build = build[:strings.Index(build, "\n}\n")]
	if !containsText(build, "for _, tag := range langPrimary") {
		t.Error("一级菜单没有只摆常用语言")
	}
	if !containsText(build, `m.Submenu("更多语言`) {
		t.Error("其余语言没有收进「更多语言」二级菜单")
	}
	if countOccurrences(build, "m.Item(") != 1 {
		t.Errorf("一级菜单里出现了 %d 处逐项添加，应该只有常用语言那一处循环", countOccurrences(build, "m.Item("))
	}

	for _, f := range []string{"popup_translation.go", "home.go"} {
		src := readSourceFile(t, f)
		if !containsText(src, "buildLangMenu(m,") {
			t.Errorf("%s 没有走统一的语言菜单", f)
		}
		if containsText(src, "for _, label := range langList()") {
			t.Errorf("%s 仍在自己铺开完整语言列表", f)
		}
	}
}

// TestCurrentLanguageInSubmenuIsNamedOnTheParent covers the other half of the
// report: with the list collapsed, the parent item has to say which of the
// hidden languages is active, or the user cannot tell what they are using.
func TestCurrentLanguageInSubmenuIsNamedOnTheParent(t *testing.T) {
	settings := readSourceFile(t, "settings.go")
	if !containsText(settings, `"更多语言（"+langLabels[current]+"）"`) {
		t.Error("当前语言在二级菜单里时，父项没有写出它的名字")
	}
	if !inLangMore("de") {
		t.Error("德语应属于二级菜单")
	}
	if inLangMore("ja") {
		t.Error("日语应在一级菜单")
	}
}

// TestTargetLanguageMenuIsShort is the direct answer to the report: opening the
// picker shows the five common languages and one 更多语言 entrance, and a
// language inside the submenu is still reachable.
func TestTargetLanguageMenuIsShort(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, "light", false)
		a.pageHome(c, c.Theme(), a.store.Get())
	}, 920, 660)
	tt.Frame()

	if err := tt.Click("→ 简体中文"); err != nil {
		t.Fatalf("点不开语言选择器：%v", err)
	}
	labels := tt.Menu()
	var items []string
	for _, l := range labels {
		if l != "-" {
			items = append(items, l)
		}
	}
	if len(items) != len(langPrimary)+1 {
		t.Fatalf("一级菜单有 %d 项，期望常用 %d 项 + 一个「更多语言」：%q", len(items), len(langPrimary), labels)
	}
	for _, want := range []string{"简体中文", "英语", "日语", "韩语", "更多语言"} {
		if !containsText(strings.Join(items, "\x00"), want) {
			t.Errorf("一级菜单缺少 %q：%q", want, items)
		}
	}
	if containsText(strings.Join(items, "\x00"), "德语") {
		t.Errorf("二级菜单的语言仍摆在一级：%q", items)
	}

	if err := tt.ChooseMenuItem("更多语言", "德语"); err != nil {
		t.Fatalf("选不到二级菜单里的德语：%v", err)
	}
	if got := a.store.Get().TargetLang; got != "de" {
		t.Fatalf("选了德语，TargetLang = %q", got)
	}
}
