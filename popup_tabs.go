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
	return s != nil && (s.target != "" || s.errText != "" || s.dictResult != nil ||
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
func (a *app) clearTabLocked() {
	a.target = ""
	a.engine = ""
	a.detected = ""
	a.errText = ""
	a.elapsed = 0
	a.dict = nil
	a.gdict = nil
	a.aiText = ""
	a.aiBusy = false
	a.aiCancel = nil
	a.grammarResult = nil
	a.grammarResultPrefill = false
	a.dictResult = nil
	a.writingResult = nil
	a.activeActionID = ""
	a.notes = nil
	a.notesBusy = false
	a.notesCancel = nil
	a.notesSaved = false
	a.grammarSaved = false
	a.onlyTrunk = false
	a.showSlashes = false
	a.filterRole = ""
	a.grammarHoverKnowledge = ""
	a.sourceExpanded = false
	a.notesExpanded = false
	a.isScreenshotMode = false
	a.ocrText = ""
	a.ocrPath = ""
	a.ocrError = ""
	a.ocrCompare = false
	a.ocrExpanded = false
	a.snipExpanded = false
	a.snipBitmap = nil
}
