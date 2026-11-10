package main

import (
	"reflect"
	"strings"
	"testing"
)

// tabFieldFuncBody 从源码里切出一个函数的函数体文本，用于源码级契约断言。
func tabFieldFuncBody(t *testing.T, src, sig string) string {
	t.Helper()
	start := strings.Index(src, sig)
	if start < 0 {
		t.Fatalf("找不到函数 %s", sig)
	}
	rest := src[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("函数 %s 没有正常结束", sig)
	}
	return rest[:end]
}

// filledAppForTabSync 造一个字段填满非零值的 app；两个副本共用同一个 store，
// 这样 reflect.DeepEqual 比较两个 app 时不会因为 store 指针不同而误判。
func filledAppForTabSync(s *store) *app {
	return &app{
		store:                 s,
		source:                "hello world",
		target:                "translated",
		engine:                "engine",
		detected:              "en",
		errText:               "err",
		elapsed:               123,
		dict:                  &YoudaoDict{},
		gdict:                 &DictResult{},
		aiText:                "partial ai",
		aiBusy:                true,
		aiCancel:              func() {},
		grammarResult:         &SentenceAnalysisResult{},
		grammarResultPrefill:  true,
		dictResult:            &DictionaryResult{},
		writingResult:         &ImproveWritingResult{},
		activeActionID:        "action",
		notes:                 []NoteItem{{}},
		notesBusy:             true,
		notesCancel:           func() {},
		notesSaved:            true,
		grammarSaved:          true,
		writingSaved:          true,
		savedDictTerm:         "term",
		onlyTrunk:             true,
		showSlashes:           true,
		filterRole:            "role",
		grammarHoverKnowledge: "knowledge",
		sourceExpanded:        true,
		notesExpanded:         true,
		isScreenshotMode:      true,
		ocrText:               "ocr",
		ocrPath:               "path",
		ocrError:              "ocrerr",
		ocrCompare:            true,
		ocrExpanded:           true,
		snipExpanded:          true,
		snipBitmap:            nil,
	}
}

// TestClearTabMatchesZeroValueRestore 是委托改动的合法性证明：
// clearTabLocked() 与 restoreTabLocked(&tabViewState{}) 对同一个填满值的 app
// 产生逐字段完全相同的结果，才允许把前者收口成后者。
func TestClearTabMatchesZeroValueRestore(t *testing.T) {
	s := newMemoryStore()
	cleared := filledAppForTabSync(s)
	restored := filledAppForTabSync(s)

	cleared.clearTabLocked()
	restored.restoreTabLocked(&tabViewState{})

	if !reflect.DeepEqual(cleared, restored) {
		t.Fatal("clearTabLocked 与零值 restoreTabLocked 结果不一致，不能合并清单")
	}
	// 顺带钉住两条容易被合并掉的设计：source 保留、取消回调归零。
	if cleared.source != "hello world" {
		t.Fatalf("清理后 source 不应被改：%q", cleared.source)
	}
	if cleared.aiCancel != nil || cleared.notesCancel != nil {
		t.Fatal("清理后取消回调应归零")
	}
}

// TestTabFieldListsAreSynced 把「三份字段清单靠人肉同步」变成可执行契约：
// saveTabLocked 保存 tabViewState 每个字段、restoreTabLocked 写回除 source 外的
// 每个字段、snap() 填充 snapshot 每个字段，且 snapshot 与 tabViewState 同名字段
// 类型一致。以后加字段忘了改任一处，测试立刻红。
func TestTabFieldListsAreSynced(t *testing.T) {
	tabs := readSourceFile(t, "popup_tabs.go")
	pop := readSourceFile(t, "popup.go")

	save := tabFieldFuncBody(t, tabs, "func (a *app) saveTabLocked()")
	restore := tabFieldFuncBody(t, tabs, "func (a *app) restoreTabLocked(")
	snap := tabFieldFuncBody(t, pop, "func (a *app) snap() snapshot")

	st := reflect.TypeOf(tabViewState{})
	sn := reflect.TypeOf(snapshot{})

	for i := 0; i < st.NumField(); i++ {
		name := st.Field(i).Name
		if !strings.Contains(save, name+":") {
			t.Errorf("saveTabLocked 没有保存 tabViewState.%s", name)
		}
		if name != "source" && !strings.Contains(restore, "a."+name) {
			t.Errorf("restoreTabLocked 没有写回 tabViewState.%s", name)
		}
		if f, ok := sn.FieldByName(name); ok && f.Type != st.Field(i).Type {
			t.Errorf("字段 %s 在 snapshot 与 tabViewState 的类型不一致", name)
		}
	}
	for i := 0; i < sn.NumField(); i++ {
		name := sn.Field(i).Name
		if !strings.Contains(snap, name+":") {
			t.Errorf("snap() 没有填充 snapshot.%s", name)
		}
	}
}

// TestClearTabDelegatesToRestore 驱动清单合并：clearTabLocked 不再自己维护
// 一份逐字段清零清单，而是委托 restore 复用同一份清单。
func TestClearTabDelegatesToRestore(t *testing.T) {
	tabs := readSourceFile(t, "popup_tabs.go")
	body := tabFieldFuncBody(t, tabs, "func (a *app) clearTabLocked()")
	if !containsText(body, "restoreTabLocked") {
		t.Fatal("clearTabLocked 仍自己维护一份字段清零清单；应委托 restoreTabLocked 复用同一份清单")
	}
	if countOccurrences(body, "a.") > 2 {
		t.Fatalf("clearTabLocked 仍在逐字段清零（a. 出现 %d 次）", countOccurrences(body, "a."))
	}
}
