package main

import "testing"

// TestCollapsedNotesAllowShrink 是「收起猜你想存后留下一整块空白」的回归测试。
//
// fitHeight 在内容变矮时本来会缩小窗口，但 busy / aiBusy / notesBusy 会挡住
// 它，避免流式回答还在长的时候抖。生词推荐是例外：面板收起后它不可见，
// 后台还在翻译推荐词时把展开时的尺寸留在窗口上，就是用户看到的那块空白。
// 注释写的意图是「用户折叠面板就按新的内容高度回落」，这里把它钉住。
func TestCollapsedNotesAllowShrink(t *testing.T) {
	if heightHeldWhileBusy(snapshot{notesBusy: true, notesExpanded: false}) {
		t.Error("收起生词面板后，后台推荐仍在跑不该阻止窗口回落：用户会看到一整块空白")
	}

	// 面板展开时相反：推荐词正在追加，高度还在变，先不缩。
	if !heightHeldWhileBusy(snapshot{notesBusy: true, notesExpanded: true}) {
		t.Error("生词面板展开时推荐词仍在增加，窗口不该缩")
	}

	// 流式回答与机器翻译照旧挡住缩小，和折叠无关。
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
