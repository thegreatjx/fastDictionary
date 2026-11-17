package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type tabViewState struct {
	source               string
	target               string
	engine               string
	detected             string
	errText              string
	elapsed              time.Duration
	dict                 *YoudaoDict
	gdict                *DictResult
	aiText               string
	aiBusy               bool
	busy                 bool
	ocrBusy              bool
	grammarResult        *SentenceAnalysisResult
	grammarResultPrefill bool
	dictResult           *DictionaryResult
	writingResult        *ImproveWritingResult
	activeActionID       string
	notes                []NoteItem
	notesBusy            bool
	notesSaved           bool
	grammarSaved         bool
	writingSaved         bool
	savedDictTerm        string
	onlyTrunk            bool
	showSlashes          bool
	sourceExpanded       bool
	notesExpanded        bool
	isScreenshotMode     bool
	ocrText              string
	ocrPath              string
	ocrError             string
	ocrCompare           bool
	ocrExpanded          bool
	snipExpanded         bool
	snipBitmap           *ui.Bitmap
}

func (s *tabViewState) hasContent() bool {
	return s != nil && !cardHasPendingWork(snapshot{aiBusy: s.aiBusy, notesBusy: s.notesBusy, busy: s.busy, ocrBusy: s.ocrBusy}) && (s.target != "" || s.errText != "" || s.dictResult != nil ||
		s.grammarResult != nil || s.writingResult != nil || s.dict != nil ||
		s.gdict != nil || s.aiText != "" || s.isScreenshotMode && s.source != "")
}

func (a *app) saveTabLocked() {
	if a.tabStates == nil {
		a.tabStates = make(map[popupMode]*tabViewState)
	}
	a.tabStates[a.mode] = &tabViewState{
		source:               a.source,
		target:               a.target,
		engine:               a.engine,
		detected:             a.detected,
		errText:              a.errText,
		elapsed:              a.elapsed,
		dict:                 a.dict,
		gdict:                a.gdict,
		aiText:               a.aiText,
		aiBusy:               a.aiBusy,
		busy:                 a.busy,
		ocrBusy:              a.ocrBusy,
		grammarResult:        a.grammarResult,
		grammarResultPrefill: a.grammarResultPrefill,
		dictResult:           a.dictResult,
		writingResult:        a.writingResult,
		activeActionID:       a.activeActionID,
		notes:                append([]NoteItem(nil), a.notes...),
		notesBusy:            a.notesBusy,
		notesSaved:           a.notesSaved,
		grammarSaved:         a.grammarSaved,
		writingSaved:         a.writingSaved,
		savedDictTerm:        a.savedDictTerm,
		onlyTrunk:            a.onlyTrunk,
		showSlashes:          a.showSlashes,
		sourceExpanded:       a.sourceExpanded,
		notesExpanded:        a.notesExpanded,
		isScreenshotMode:     a.isScreenshotMode,
		ocrText:              a.ocrText,
		ocrPath:              a.ocrPath,
		ocrError:             a.ocrError,
		ocrCompare:           a.ocrCompare,
		ocrExpanded:          a.ocrExpanded,
		snipExpanded:         a.snipExpanded,
		snipBitmap:           a.snipBitmap,
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
	a.busy = false
	a.ocrBusy = false
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

func (a *app) clearTabLocked() {
	a.restoreTabLocked(&tabViewState{})
}
