package main

import (
	"context"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type NoteItem struct {
	Word       string `json:"word"`
	Phonetic   string `json:"phonetic"`
	Pos        string `json:"pos"`
	Meaning    string `json:"meaning"`
	Difficulty string `json:"difficulty"`
	MemoryTips string `json:"memory_tips"`
	Checked    bool   `json:"checked"`
}

type popupMode int

const (
	modeTranslate popupMode = iota
	modeDictionary
	modeGrammar
	modeWriting
	modeInput
	modeSelectionPreview
)

const modeSelection = modeTranslate

func modeName(m popupMode) string {
	switch m {
	case modeTranslate:
		return "translate"
	case modeDictionary:
		return "dictionary"
	case modeGrammar:
		return "grammar"
	case modeWriting:
		return "writing"
	case modeInput:
		return "input"
	case modeSelectionPreview:
		return "selection-preview"
	}
	return "unknown"
}

type app struct {
	store *store

	mu        sync.Mutex
	mode      popupMode
	source    string
	target    string
	rawSource string
	engine    string
	detected  string
	errText   string
	echoNote  string
	notice    string
	busy      bool
	elapsed   time.Duration
	dict      *YoudaoDict
	gdict     *DictResult
	aiText    string
	aiBusy    bool
	aiCancel  context.CancelFunc
	// 代次用于丢弃旧 AI 请求的回包。
	aiGen           requestToken
	aiRefineGen     requestToken
	aiRefineCancel  context.CancelFunc
	inputText       string
	inputTargetMode popupMode
	trayInput       string
	sourceExpanded  bool
	snipExpanded    bool
	notesExpanded   bool
	ocrText         string
	ocrPath         string
	ocrCrop         winRect
	ocrBusy         bool
	ocrError        string
	ocrGeneration   requestToken
	ocrCancel       context.CancelFunc

	win                     *mygo.Window
	cfgWin                  *mygo.Window
	mainWinVisible          bool
	tray                    *mygo.Tray
	trayFlyout              *mygo.Window
	page                    string
	pinned                  bool
	showNotes               bool
	notes                   []NoteItem
	notesBusy               bool
	notesGen                requestToken
	notesCancel             context.CancelFunc
	onlyTrunk               bool
	grammarResult           *SentenceAnalysisResult
	grammarResultPrefill    bool
	cache                   *queryCache
	skipTransCache          bool
	skipNotesCache          bool
	skipAIResultCache       bool
	activeActionID          string
	dictResult              *DictionaryResult
	writingResult           *ImproveWritingResult
	actionSavedMsg          string
	saveErr                 string
	savedDictTerm           string
	copiedMsg               string
	feedbackText            string
	feedbackAt              time.Time
	feedbackTimer           *time.Timer
	editingActionID         string
	customizingAction       CustomActionConfig
	vocabSearchQuery        string
	actionSaveFeedback      string
	selHoverTip             string
	selHoverIdx             int
	moreFlyout              *mygo.Window
	moreFlyoutRect          ui.Rect
	moreFlyoutShown         bool
	moreBtnRect             ui.Rect
	showSlashes             bool
	showTree                bool
	vocabCategoryIdx        int
	vocabList               ui.ListState
	vocabExpandedID         string
	vocabReviewing          bool
	vocabReviewIDs          []string
	vocabReviewIdx          int
	vocabReviewRevealed     bool
	vocabReviewDone         int
	updateChecking          bool
	updateMsg               string
	updateURL               string
	updateNewer             bool
	notesSaved              bool
	grammarSaved            bool
	writingSaved            bool
	notebookCategoryIdx     int
	notebookSearchQuery     string
	notebookList            ui.ListState
	notebookExpandedID      string
	notebookOnlyTrunk       bool
	notebookShowSlashes     bool
	notebookShowTree        bool
	notebookConfirmDeleteID string
	confirmClearNotebook    bool
	lastDeletedNotebook     *NotebookEntry
	isQuitting              atomic.Bool
	pausedUntil             time.Time
	isScreenshotMode        bool
	translateGen            requestToken
	tabStates               map[popupMode]*tabViewState
	ocrCompare              bool
	ocrExpanded             bool
	screenshotPinned        bool
	snipBitmap              *ui.Bitmap
	popupX                  int
	popupY                  int
	popupW                  int
	popupH                  int
	// 卡片从底边向上生长时，resize 必须保持底边不动。
	popupAnchoredAbove bool
	popupAnchorBottom  int
	userResized        bool
	lastAutofitWinH    int
	autofit            autofitLoop

	sel              selState
	chipTip          chipTipState
	gestureGate      gestureGate
	captureSelection func() selection
	showChip         func(x, y int, text string)
	showPopup        func(m popupMode, text string)

	home homeState

	aiProbe                 aiProbeState
	aiModels                aiModelsState
	aiModelsBySource        map[string]aiModelsState
	aiShowKey               bool
	aiHelpOpen              bool
	aiRefining              bool
	aiRefineErr             string
	hotkeyMsg               string
	hotkeyMsgKind           hotkeyMsgKind
	pendingHotkeySelection  string
	pendingHotkeyInput      string
	pendingHotkeyScreenshot string
	hotkeyDraftsInit        bool
	newBlockedApp           string
	confirmClearVocab       bool
	vocabConfirmDeleteID    string
	lastDeletedVocab        *VocabularyEntry
	settingsScroll          ui.ScrollState
	navScroll               ui.ScrollState
	settingsPageSeen        string
}

type snapshot struct {
	mode                 popupMode
	source               string
	target               string
	engine               string
	detected             string
	errText              string
	echoNote             string
	notice               string
	busy                 bool
	elapsed              time.Duration
	dict                 *YoudaoDict
	gdict                *DictResult
	aiText               string
	aiBusy               bool
	inputText            string
	inputTargetMode      popupMode
	isWord               bool
	pinned               bool
	showNotes            bool
	notes                []NoteItem
	notesBusy            bool
	onlyTrunk            bool
	grammarResult        *SentenceAnalysisResult
	grammarResultPrefill bool
	aiRefining           bool
	aiRefineErr          string
	activeActionID       string
	dictResult           *DictionaryResult
	writingResult        *ImproveWritingResult
	actionSavedMsg       string
	saveErr              string
	savedDictTerm        string
	copiedMsg            string
	showSlashes          bool
	showTree             bool
	notesSaved           bool
	grammarSaved         bool
	writingSaved         bool
	isScreenshotMode     bool
	snipBitmap           *ui.Bitmap
	sourceExpanded       bool
	snipExpanded         bool
	notesExpanded        bool
	ocrBusy              bool
	ocrError             string
	ocrText              string
	ocrCompare           bool
	ocrExpanded          bool
}

const feedbackTTL = 2 * time.Second

func (a *app) noteFeedbackLocked() {
	text := ""
	if a.copiedMsg != "" || a.actionSavedMsg != "" {
		text = a.copiedMsg + "\x00" + a.actionSavedMsg
	}
	if text == a.feedbackText {
		return
	}
	if a.feedbackTimer != nil {
		a.feedbackTimer.Stop()
		a.feedbackTimer = nil
	}
	a.feedbackText = text
	a.feedbackAt = time.Now()
	if text == "" {
		return
	}
	popupWin, mainWin := a.win, a.cfgWin
	a.feedbackTimer = time.AfterFunc(feedbackTTL, func() {
		a.mu.Lock()
		a.feedbackTimer = nil
		a.mu.Unlock()
		for _, w := range []*mygo.Window{popupWin, mainWin} {
			if w != nil && !w.IsDestroyed() {
				w.Update(func() {})
			}
		}
	})
}

func (a *app) feedbackVisibleLocked() bool {
	return a.feedbackText != "" && time.Since(a.feedbackAt) < feedbackTTL
}

func (a *app) snap() snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.noteFeedbackLocked()
	copiedMsg, actionSavedMsg := a.copiedMsg, a.actionSavedMsg
	if !a.feedbackVisibleLocked() {
		copiedMsg, actionSavedMsg = "", ""
	}
	var notes []NoteItem
	if len(a.notes) > 0 {
		notes = append([]NoteItem(nil), a.notes...)
	}
	return snapshot{
		mode: a.mode, source: a.source, target: a.target, engine: a.engine,
		detected: a.detected, errText: a.errText, echoNote: a.echoNote, notice: a.notice, busy: a.busy, elapsed: a.elapsed,
		dict: a.dict, gdict: a.gdict, aiText: a.aiText, aiBusy: a.aiBusy,
		inputText: a.inputText, inputTargetMode: a.inputTargetMode, isWord: isWord(a.source), pinned: a.pinned, showNotes: a.showNotes, notes: notes, notesBusy: a.notesBusy, onlyTrunk: a.onlyTrunk, grammarResult: a.grammarResult, grammarResultPrefill: a.grammarResultPrefill, aiRefining: a.aiRefining, aiRefineErr: a.aiRefineErr,
		activeActionID: a.activeActionID, dictResult: a.dictResult, writingResult: a.writingResult, actionSavedMsg: actionSavedMsg, saveErr: a.saveErr, savedDictTerm: a.savedDictTerm, copiedMsg: copiedMsg, showSlashes: a.showSlashes, showTree: a.showTree, notesSaved: a.notesSaved, grammarSaved: a.grammarSaved, writingSaved: a.writingSaved,
		isScreenshotMode: a.isScreenshotMode, snipBitmap: a.snipBitmap, sourceExpanded: a.sourceExpanded,
		snipExpanded: a.snipExpanded, notesExpanded: a.notesExpanded, ocrBusy: a.ocrBusy, ocrError: a.ocrError, ocrText: a.ocrText,
		ocrCompare: a.ocrCompare, ocrExpanded: a.ocrExpanded,
	}
}

func (a *app) play(word string, us bool) {
	PlaySpeechLang(word, a.speechLang(), us)
}

func (a *app) speechLang() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.detected != "" {
		return a.detected
	}
	return detectLocal(a.source)
}

func (a *app) openPopup(m popupMode) {
	a.hideSelIcon()

	if m == modeSelection {
		app := foregroundAppName()
		sel := a.CaptureSelection()
		if sel.Text == "" {
			selfElevated := currentProcessElevated()
			blocked := !selfElevated && foregroundProcessElevated()
			log.Printf("fastDictionary: no selection from %q; opening input mode (elevated window=%v, self=%v)",
				app, blocked, selfElevated)
			notice := elevationNotice(app, blocked, selfElevated)
			if notice == "" {
				notice = "没取到选中的文字（" + app + "）。可以直接在下面输入，或在设置里把「取词方式」改成「模拟复制」。"
			}
			a.mu.Lock()
			a.notice = notice
			a.mu.Unlock()
			a.openPopupWithText(modeInput, "")
			return
		}
		log.Printf("fastDictionary: selection %s from %q", a.logText(sel.Text, 40), app)
		mode := modeSelection
		if preferred, ok := a.store.Get().InitialAIMode(); ok && isPopupModeEnabled(a.store.Get(), preferred) {
			mode = preferred
			log.Printf("fastDictionary: selection opens in %s (ai_mode)", modeName(mode))
		}
		a.openPopupWithText(mode, sel.Text)
		return
	}
	a.openPopupWithText(m, "")
}

func (a *app) rerunCurrentMode(text string) {
	switch a.mode {
	case modeDictionary:
		a.runAction("default-dictionary", text)
	case modeGrammar:
		a.runAction("default-sentence-analysis", text)
	case modeWriting:
		a.runAction("default-improve-writing", text)
	default:
		a.translate(text)
	}
}

func (a *app) ensureCacheLocked() *queryCache {
	if a.cache == nil {
		a.cache = newQueryCache("")
	}
	return a.cache
}

func (a *app) ensureCache() *queryCache {
	a.mu.Lock()
	qc := a.ensureCacheLocked()
	a.mu.Unlock()
	return qc
}

func (a *app) rerunCurrentModeFresh(text string) {
	a.mu.Lock()
	switch a.mode {
	case modeDictionary, modeGrammar, modeWriting:
		a.skipAIResultCache = true
	default:
		a.skipTransCache = true
		a.skipNotesCache = true
	}
	a.mu.Unlock()
	a.rerunCurrentMode(text)
}

func (a *app) switchMode(m popupMode) {
	cfg := a.store.Get()
	notice := ""
	if (m == modeGrammar || m == modeWriting) && !isPopupModeEnabled(cfg, m) {
		m = modeTranslate
	} else if problem := cfg.AISetupProblem(); (m == modeGrammar || m == modeWriting) && problem != "" {
		m = modeTranslate
		notice = "AI 精翻暂不可用（" + problem + "），已使用机器翻译。可在 设置 → AI 精翻 中设置。"
	}
	a.stopScreenshotOCR()
	a.mu.Lock()
	if a.mode == m {
		if notice != "" {
			a.notice = notice
		}
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
		}
		return
	}
	a.saveTabLocked()
	a.stopAIPassLocked()
	a.stopNotesLocked()
	text := a.source
	a.mode = m
	a.busy = false
	a.translateGen++
	a.notice = notice
	a.errText = ""
	a.echoNote = ""
	if strings.TrimSpace(text) == "" {
		a.clearTabLocked()
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
		}
		return
	}
	if st := a.tabStates[m]; st != nil && st.source == text && st.hasContent() {
		a.restoreTabLocked(st)
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
			a.fitHeight()
		}
		return
	}
	a.clearTabLocked()
	a.mu.Unlock()
	a.rerunCurrentMode(text)
}

func (a *app) resetPopupTransientLocked() {
	a.onlyTrunk = false
	a.showSlashes = false
	a.showTree = false
	a.activeActionID = ""
}

func (a *app) resetScreenshotPinLocked() {
	if a.screenshotPinned {
		a.pinned = false
		a.screenshotPinned = false
	}
}

func (a *app) openPopupAction(m popupMode, text string) {
	a.stopScreenshotOCR()
	cfg := a.store.Get()
	raw := text
	text = cleanSourceForLayout(raw, cfg.LayoutMode, false)
	win := a.win
	if win == nil {
		return
	}

	a.hideSelIcon()

	pt := mygo.Screen.CursorScreenPoint()
	w := popupWidth(cfg.PopupWidth)
	initH := 380
	if m == modeDictionary {
		initH = 460
	} else if m == modeGrammar || m == modeWriting {
		initH = 440
	}

	x, y, anchoredAbove := a.calcPopupPlacement(pt, int(w), initH)
	a.popupX = x
	a.popupY = y
	a.popupW = int(w)
	a.popupH = initH
	a.popupAnchoredAbove = anchoredAbove
	a.popupAnchorBottom = y + initH

	nb := GetNotebookStore()
	a.mu.Lock()
	a.userResized = false
	a.autofit.reset()
	a.stopAIPassLocked()
	a.stopNotesLocked()
	a.tabStates = nil
	a.mode = m
	a.inputTargetMode = m
	a.inputText = ""
	a.notice = ""
	a.source = text
	a.rawSource = raw
	a.sourceExpanded = false
	a.target = ""
	a.errText = ""
	a.echoNote = ""
	a.aiText = ""
	a.dict = nil
	a.gdict = nil
	a.busy = false
	a.translateGen++
	a.aiBusy = false
	a.dictResult = nil
	a.grammarResult = nil
	a.grammarResultPrefill = false
	a.writingResult = nil
	a.actionSavedMsg = ""
	a.saveErr = ""
	a.copiedMsg = ""
	a.feedbackText = ""
	a.notesSaved = false
	a.grammarSaved = text != "" && nb.Contains("grammar", grammarAnalysisText(text))
	a.writingSaved = text != "" && nb.Contains("writing", text)
	a.resetPopupTransientLocked()
	a.notes = nil
	a.notesBusy = false
	a.isScreenshotMode = false
	a.snipBitmap = nil
	a.snipExpanded = false
	a.notesExpanded = false
	a.ocrText, a.ocrPath, a.ocrError = "", "", ""
	a.ocrCompare, a.ocrExpanded = false, false
	a.resetScreenshotPinLocked()
	a.mu.Unlock()

	win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: int(w), Height: initH})
	win.SetAlwaysOnTop(cfg.AlwaysOnTop)
	win.Show()
	win.Focus()
	win.Update(func() {})

	if text != "" {
		a.rerunCurrentMode(text)
	}
}

func (a *app) openPopupWithText(m popupMode, text string) {
	a.stopScreenshotOCR()
	cfg := a.store.Get()
	raw := text
	text = cleanSourceForLayout(raw, cfg.LayoutMode, false)
	win := a.win
	if win == nil {
		return
	}

	pt := mygo.Screen.CursorScreenPoint()
	w := popupWidth(cfg.PopupWidth)
	initH := 340
	if m == modeInput {
		initH = 260
	}

	x, y, anchoredAbove := a.calcPopupPlacement(pt, int(w), initH)
	a.popupX = x
	a.popupY = y
	a.popupW = int(w)
	a.popupH = initH
	a.popupAnchoredAbove = anchoredAbove
	a.popupAnchorBottom = y + initH

	nb := GetNotebookStore()
	a.mu.Lock()
	a.userResized = false
	a.autofit.reset()
	a.stopAIPassLocked()
	a.stopNotesLocked()
	a.tabStates = nil
	a.mode = m
	if m != modeInput {
		a.inputTargetMode = m
	} else if a.inputTargetMode == 0 {
		a.inputTargetMode = modeTranslate
	}
	a.inputText = ""
	a.notice = ""
	a.source, a.target, a.errText, a.aiText = "", "", "", ""
	a.rawSource = raw
	a.sourceExpanded = false
	a.dict, a.gdict, a.busy, a.aiBusy = nil, nil, false, false
	a.translateGen++
	a.activeActionID = ""
	a.dictResult = nil
	a.writingResult = nil
	a.grammarResult = nil
	a.grammarResultPrefill = false
	a.actionSavedMsg = ""
	a.saveErr = ""
	a.copiedMsg = ""
	a.feedbackText = ""
	a.notes, a.notesBusy, a.notesSaved, a.notesExpanded = nil, false, false, false
	a.grammarSaved = text != "" && nb.Contains("grammar", grammarAnalysisText(text))
	a.writingSaved = text != "" && nb.Contains("writing", text)
	a.resetPopupTransientLocked()
	a.isScreenshotMode, a.snipExpanded = false, false
	a.snipBitmap = nil
	a.ocrText, a.ocrPath, a.ocrError = "", "", ""
	a.ocrCompare, a.ocrExpanded = false, false
	a.resetScreenshotPinLocked()
	a.mu.Unlock()

	win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: int(w), Height: initH})
	win.SetAlwaysOnTop(cfg.AlwaysOnTop)
	win.Show()
	win.Focus()
	win.Update(func() {})

	if text != "" {
		a.rerunCurrentMode(text)
	}
}
