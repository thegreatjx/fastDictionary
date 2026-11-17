package main

import "testing"

func TestCollapsedNotesAllowShrink(t *testing.T) {
	if heightHeldWhileBusy(snapshot{notesBusy: true, notesExpanded: false}) {
		t.Error("收起生词面板后，后台推荐仍在跑不该阻止窗口回落：用户会看到一整块空白")
	}

	if !heightHeldWhileBusy(snapshot{notesBusy: true, notesExpanded: true}) {
		t.Error("生词面板展开时推荐词仍在增加，窗口不该缩")
	}

	for _, s := range []snapshot{
		{aiBusy: true},
		{busy: true},
		{aiBusy: true, notesExpanded: false},
	} {
		if !heightHeldWhileBusy(s) {
			t.Errorf("%+v：进行中的可见内容仍应挡住缩小", s)
		}
	}
}
