package main

import (
	"reflect"
	"strings"
	"testing"
)

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

func filledAppForTabSync(s *store) *app {
	return &app{
		store:                s,
		source:               "hello world",
		target:               "translated",
		engine:               "engine",
		detected:             "en",
		errText:              "err",
		elapsed:              123,
		dict:                 &YoudaoDict{},
		gdict:                &DictResult{},
		aiText:               "partial ai",
		aiBusy:               true,
		aiCancel:             func() {},
		grammarResult:        &SentenceAnalysisResult{},
		grammarResultPrefill: true,
		dictResult:           &DictionaryResult{},
		writingResult:        &ImproveWritingResult{},
		activeActionID:       "action",
		notes:                []NoteItem{{}},
		notesBusy:            true,
		notesCancel:          func() {},
		notesSaved:           true,
		grammarSaved:         true,
		writingSaved:         true,
		savedDictTerm:        "term",
		onlyTrunk:            true,
		showSlashes:          true,
		sourceExpanded:       true,
		notesExpanded:        true,
		isScreenshotMode:     true,
		ocrText:              "ocr",
		ocrPath:              "path",
		ocrError:             "ocrerr",
		ocrCompare:           true,
		ocrExpanded:          true,
		snipExpanded:         true,
		snipBitmap:           nil,
	}
}

func TestClearTabMatchesZeroValueRestore(t *testing.T) {
	s := newMemoryStore()
	cleared := filledAppForTabSync(s)
	restored := filledAppForTabSync(s)

	cleared.clearTabLocked()
	restored.restoreTabLocked(&tabViewState{})

	if !reflect.DeepEqual(cleared, restored) {
		t.Fatal("clearTabLocked 与零值 restoreTabLocked 结果不一致，不能合并清单")
	}
	if cleared.source != "hello world" {
		t.Fatalf("清理后 source 不应被改：%q", cleared.source)
	}
	if cleared.aiCancel != nil || cleared.notesCancel != nil {
		t.Fatal("清理后取消回调应归零")
	}
}

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
