package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// tabViewState is the result a reading tab already owns. The three tabs used
// to share one card, so switching away and back either re-ran a slow model or
// showed the other tab's answer under the new heading. Each tab now carries
// its own result and the source text it belongs to; restoring is refused when
// the source has changed.
type tabViewState struct {
	source                string
	target                string
	engine                string
	detected              string
	errText               string
	elapsed               time.Duration
	dict                  *YoudaoDict
	gdict                 *DictResult
	aiText                string
	aiBusy                bool
	grammarResult         *SentenceAnalysisResult
	grammarResultPrefill  bool
	dictResult            *DictionaryResult
	writingResult         *ImproveWritingResult
	activeActionID        string
	notes                 []NoteItem
	notesBusy             bool
	notesSaved            bool
	grammarSaved          bool
	writingSaved          bool
	savedDictTerm         string
	onlyTrunk             bool
	showSlashes           bool
	filterRole            string
	grammarHoverKnowledge string
	sourceExpanded        bool
	notesExpanded         bool
	isScreenshotMode      bool
	ocrText               string
	ocrPath               string
	ocrError              string
	ocrCompare            bool
	ocrExpanded           bool
	snipExpanded          bool
	snipBitmap            *ui.Bitmap
}

func (s *tabViewState) hasContent() bool {
	// 正在加载的页签不是完成态。switchMode 用这个判定决定能否直接恢复缓存，
	// 若把进行中的页签算作有内容，半成品 aiText / notes 会被当成最终答案展示，
	// 且命中缓存后不会再补发请求。切回来应走 clearTabLocked + 重跑当前模式。
	return s != nil && !s.aiBusy && !s.notesBusy && (s.target != "" || s.errText != "" || s.dictResult != nil ||
		s.grammarResult != nil || s.writingResult != nil || s.dict != nil ||
		s.gdict != nil || s.aiText != "" || s.isScreenshotMode && s.source != "")
}

func (a *app) saveTabLocked() {
	if a.tabStates == nil {
		a.tabStates = make(map[popupMode]*tabViewState)
	}
	a.tabStates[a.mode] = &tabViewState{
		source:                a.source,
		target:                a.target,
		engine:                a.engine,
		detected:              a.detected,
		errText:               a.errText,
		elapsed:               a.elapsed,
		dict:                  a.dict,
		gdict:                 a.gdict,
		aiText:                a.aiText,
		aiBusy:                a.aiBusy,
		grammarResult:         a.grammarResult,
		grammarResultPrefill:  a.grammarResultPrefill,
		dictResult:            a.dictResult,
		writingResult:         a.writingResult,
		activeActionID:        a.activeActionID,
		notes:                 append([]NoteItem(nil), a.notes...),
		notesBusy:             a.notesBusy,
		notesSaved:            a.notesSaved,
		grammarSaved:          a.grammarSaved,
		writingSaved:          a.writingSaved,
		savedDictTerm:         a.savedDictTerm,
		onlyTrunk:             a.onlyTrunk,
		showSlashes:           a.showSlashes,
		filterRole:            a.filterRole,
		grammarHoverKnowledge: a.grammarHoverKnowledge,
		sourceExpanded:        a.sourceExpanded,
		notesExpanded:         a.notesExpanded,
		isScreenshotMode:      a.isScreenshotMode,
		ocrText:               a.ocrText,
		ocrPath:               a.ocrPath,
		ocrError:              a.ocrError,
		ocrCompare:            a.ocrCompare,
		ocrExpanded:           a.ocrExpanded,
		snipExpanded:          a.snipExpanded,
		snipBitmap:            a.snipBitmap,
	}
}

func (a *app) restoreTabLocked(st *tabViewState) {
	a.target = st.target
	a.engine = st.engine
	a.detected = st.detected
	a.errText = st.errText
	a.elapsed = st.elapsed
	a.dict = st.dict
	a.gdict = st.gdict
	a.aiText = st.aiText
	a.aiBusy = false
	a.aiCancel = nil
	a.grammarResult = st.grammarResult
	a.grammarResultPrefill = st.grammarResultPrefill
	a.dictResult = st.dictResult
	a.writingResult = st.writingResult
	a.activeActionID = st.activeActionID
	a.notes = append([]NoteItem(nil), st.notes...)
	a.notesBusy = false
	a.notesCancel = nil
	a.notesSaved = st.notesSaved
	a.grammarSaved = st.grammarSaved
	a.writingSaved = st.writingSaved
	a.savedDictTerm = st.savedDictTerm
	a.onlyTrunk = st.onlyTrunk
	a.showSlashes = st.showSlashes
	a.filterRole = st.filterRole
	a.grammarHoverKnowledge = st.grammarHoverKnowledge
	a.sourceExpanded = st.sourceExpanded
	a.notesExpanded = st.notesExpanded
	a.isScreenshotMode = st.isScreenshotMode
	a.ocrText = st.ocrText
	a.ocrPath = st.ocrPath
	a.ocrError = st.ocrError
	a.ocrCompare = st.ocrCompare
	a.ocrExpanded = st.ocrExpanded
	a.snipExpanded = st.snipExpanded
	a.snipBitmap = st.snipBitmap
}

// clearTabLocked removes the result fields of the current tab while keeping
// its source text. It is used before starting the tab's first request.
//
// 清零与恢复是同一份字段清单：把零值 tabViewState 交给 restoreTabLocked，
// 新增字段时只需在 save/restore 一处补齐，不必再同步第三份清单。
// restoreTabLocked 不写 source，正好保留当前查询文本；它把 aiCancel 与
// notesCancel 归零，和原先的清理一致。等价性由 x1_tab_field_sync_test.go 断言。
func (a *app) clearTabLocked() {
	a.restoreTabLocked(&tabViewState{})
}
