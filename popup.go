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
	modeTranslate        popupMode = iota // 划词翻译 (Fast Translation + 猜你想存)
	modeDictionary                        // 词典精读 (Learner's Dictionary Layout)
	modeGrammar                           // 长难句分析 (Sentence Analysis Layout)
	modeWriting                           // 写作改进 (Improve Writing Layout)
	modeInput                             // 输入模式
	modeSelectionPreview                  // Demo preview
)

const modeSelection = modeTranslate

// modeName is the human label used in the log, so a user reading fastDictionary.log
// can tell which card a gesture opened without decoding the enum.
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

// app is the whole client state.
type app struct {
	store *store

	mu       sync.Mutex
	mode     popupMode
	source   string
	target   string
	engine   string
	detected string
	errText  string
	// echoNote explains a translation attempt whose engine echoed the source:
	// an answer, not a failure. It names the engine that kept the text and, when
	// another engine supplied the reading, the one that did.
	echoNote string
	notice   string
	busy     bool
	elapsed  time.Duration
	dict     *YoudaoDict
	gdict    *DictResult
	aiText   string
	aiBusy   bool
	aiCancel context.CancelFunc
	// aiGen counts AI passes and anything that supersedes them. A cancelled
	// pass keeps running until its HTTP request returns, so without this its
	// teardown would write over the newer request's card.
	aiGen           requestToken
	aiRefineGen     requestToken // 精翻专用代次，与卡片流分开
	aiRefineCancel  context.CancelFunc
	inputText       string
	inputTargetMode popupMode
	trayInput       string // the tray quick-search box, separate from the popup's input
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

	win        *mygo.Window
	cfgWin     *mygo.Window
	tray       *mygo.Tray
	trayFlyout *mygo.Window
	page       string
	pinned     bool
	showNotes  bool
	notes      []NoteItem
	notesBusy  bool
	// notesGen counts note extractions and every popup/translation that
	// supersedes them. The request is slow (up to 20s), so its teardown must
	// not write into a newer card after the user has moved on.
	notesGen      requestToken
	notesCancel   context.CancelFunc
	onlyTrunk     bool
	grammarResult *SentenceAnalysisResult
	// grammarResultPrefill reports that grammarResult.Translation came from the
	// machine engines rather than the model, so the card can say so while the
	// annotations are still being generated.
	grammarResultPrefill bool
	// aiResultCache keeps the last few AI action answers, keyed by action,
	// model and source text. The model is non-deterministic: re-selecting the
	// same sentence used to draw different annotations every time, and
	// switching tabs re-ran a pass that takes 8-13s. The footer's 重新翻译
	// button bypasses it, because there "try again" is the whole point.
	aiResultCache map[string]string
	aiResultOrder []string
	// skipAIResultCache is set by the refresh button: it promises a new
	// answer, so the cache must not answer instead.
	skipAIResultCache bool
	activeActionID    string
	dictResult        *DictionaryResult
	writingResult     *ImproveWritingResult
	actionSavedMsg    string
	// savedDictTerm is the term that went into the notebook on the last save,
	// so the card's own button can answer 已保存 instead of offering a second
	// click that would add the same entry twice.
	savedDictTerm         string
	copiedMsg             string
	feedbackText          string
	feedbackAt            time.Time
	feedbackTimer         *time.Timer
	editingActionID       string
	customizingAction     CustomActionConfig
	vocabSearchQuery      string
	actionSaveFeedback    string
	selHoverTip           string
	selHoverIdx           int
	moreFlyout            *mygo.Window
	showSlashes           bool
	filterRole            string
	grammarHoverKnowledge string
	vocabCategoryIdx      int
	vocabList             ui.ListState
	// vocabExpandedID is the 生词本 entry currently opened as a full word card.
	// The card the 词典 tab used to render lives here now, so this decides
	// which row is showing it; "" means every row is in its summary form.
	vocabExpandedID string
	// Spaced-repetition session: a snapshot of due IDs keeps the queue stable
	// while grading changes due dates mid-session.
	vocabReviewing      bool
	vocabReviewIDs      []string
	vocabReviewIdx      int
	vocabReviewRevealed bool
	vocabReviewDone     int
	// update* holds the result of the user-triggered 检查更新. Guarded by mu:
	// the fetch runs in a goroutine while the About page paints.
	updateChecking bool
	updateMsg      string
	updateURL      string
	updateNewer    bool
	// notesSaved records that the 猜你想存 checklist was saved; grammarSaved
	// records that the sentence card was. They were one flag, so saving words
	// from 猜你想存 made the sentence card claim "already in the notebook" for
	// a sentence that was never saved, and the other way round.
	notesSaved   bool
	grammarSaved bool
	writingSaved bool
	// 退出标志由 quitApp、OnBeforeQuit 与设置窗口 OnClose 三处访问，用原子类型统一读写契约。
	isQuitting atomic.Bool
	// pausedUntil stops the selection trigger until the moment it holds;
	// the zero value means "not paused".
	pausedUntil      time.Time
	isScreenshotMode bool
	// translateGen counts translations so a slow answer that has been
	// superseded can be recognised and dropped instead of overwriting the
	// newer result.
	translateGen requestToken
	// tabStates keeps the answer each reading tab already owns. Switching
	// tabs must not rerun a slow model or show the previous tab's text under
	// the new heading.
	tabStates map[popupMode]*tabViewState
	// OCR paragraph view: compare shows each OCR paragraph beside its
	// translation; expanded lifts the height cap for long screenshots.
	ocrCompare  bool
	ocrExpanded bool
	// screenshotPinned remembers that the *screenshot* translation asked
	// for a pinned window, so the next normal popup can un-pin it. Setting
	// pinned alone made every popup after one screenshot permanently
	// pinned: OnBlur stopped closing it and the pin button stayed lit.
	screenshotPinned bool
	snipBitmap       *ui.Bitmap
	popupX           int
	popupY           int
	popupW           int
	popupH           int
	// measuredViewportH/measuredContentH and autofitTarget record what the
	// last frame laid out and the height the autofit asked for. The window
	// resize itself cannot be observed from a test, so the measurement is
	// kept here and asserted directly. lastAutofitWinH/ViewportH/ContentH
	// remember the last measurement that was acted on, so a frame that still
	// lays out at the old size does not apply the same delta again.
	measuredViewportH    float32
	measuredContentH     float32
	autofitTarget        float32
	lastAutofitWinH      int
	lastAutofitViewportH float32
	lastAutofitContentH  float32
	// autofitSteps counts the corrections applied since the content last
	// changed and autofitDir is the direction of the last one. The body is a
	// Scroll that always fills the window, so the content height it is
	// measured against depends on the window height the autofit just set:
	// shrinking produces a frame that reports taller content and asks to grow
	// again. Two passes that disagree never settle — that was the window
	// bouncing between two heights on every repaint — so the reversal is
	// dropped and autofitLocked keeps it from trying again until the content
	// actually changes.
	autofitSteps        int
	autofitDir          float32
	autofitLocked       bool
	autofitLockContentH float32 // content height when the reversal locked the loop

	// the floating selection chip
	sel selState
	// chipTip is the label that names the button under the pointer. It has
	// its own lock, so the render pass never waits on app.mu for it.
	chipTip chipTipState
	// gestureGate drops a gesture that repeats the one just accepted. It is
	// what keeps a scrollbar drag or a stutter from raising the chip twice.
	gestureGate gestureGate
	// captureSelection reads the current selection; nil uses the real
	// UI Automation and clipboard readers. Tests set it.
	captureSelection func() selection
	// showChip and showPopup place the two outcomes of a selection. They
	// are fields so the trigger policy can be verified without windows.
	showChip  func(x, y int, text string)
	showPopup func(m popupMode, text string)

	// settings window scratch state
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
	// newBlockedApp is the text being typed into the selection blocklist
	// editor on the 划词 settings page.
	newBlockedApp string
	// confirmClearVocab turns the 清空 button into a confirm/cancel pair.
	confirmClearVocab    bool
	vocabConfirmDeleteID string
	lastDeletedVocab     *VocabularyEntry
	// settingsScroll keeps the settings page's offset across re-renders, so
	// toggling a switch near the bottom does not jump back to the top. It
	// also carries MaxY, which is how "can this page scroll at all" is
	// asserted in the layout test.
	settingsScroll ui.ScrollState
	// settingsPageSeen is the page the scroll offset belongs to; switching
	// pages starts the new one at the top.
	settingsPageSeen string
}

type snapshot struct {
	mode                  popupMode
	source                string
	target                string
	engine                string
	detected              string
	errText               string
	echoNote              string
	notice                string
	busy                  bool
	elapsed               time.Duration
	dict                  *YoudaoDict
	gdict                 *DictResult
	aiText                string
	aiBusy                bool
	inputText             string
	inputTargetMode       popupMode
	isWord                bool
	pinned                bool
	showNotes             bool
	notes                 []NoteItem
	notesBusy             bool
	onlyTrunk             bool
	grammarResult         *SentenceAnalysisResult
	grammarResultPrefill  bool
	aiRefining            bool
	aiRefineErr           string
	activeActionID        string
	dictResult            *DictionaryResult
	writingResult         *ImproveWritingResult
	actionSavedMsg        string
	savedDictTerm         string
	copiedMsg             string
	showSlashes           bool
	filterRole            string
	grammarHoverKnowledge string
	notesSaved            bool
	grammarSaved          bool
	writingSaved          bool
	isScreenshotMode      bool
	snipBitmap            *ui.Bitmap
	sourceExpanded        bool
	snipExpanded          bool
	notesExpanded         bool
	ocrBusy               bool
	ocrError              string
	ocrText               string
	ocrCompare            bool
	ocrExpanded           bool
}

// feedbackTTL is how long a copy or save confirmation stays on screen. Without
// an expiry the banner stayed until the next popup was opened, which looked
// like a stuck notification rather than a confirmation of one action.
const feedbackTTL = 3 * time.Second

// noteFeedbackLocked records when the current banner text first appeared. The
// popup renders from a snapshot, so the clock has to live next to the text.
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
	win := a.win
	a.feedbackTimer = time.AfterFunc(feedbackTTL, func() {
		a.mu.Lock()
		a.feedbackTimer = nil
		a.mu.Unlock()
		if win != nil && !win.IsDestroyed() {
			win.Update(func() {})
		}
	})
}

// feedbackVisibleLocked reports whether the banner recorded at feedbackAt is
// still within its display window.
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
	return snapshot{
		mode: a.mode, source: a.source, target: a.target, engine: a.engine,
		detected: a.detected, errText: a.errText, echoNote: a.echoNote, notice: a.notice, busy: a.busy, elapsed: a.elapsed,
		dict: a.dict, gdict: a.gdict, aiText: a.aiText, aiBusy: a.aiBusy,
		inputText: a.inputText, inputTargetMode: a.inputTargetMode, isWord: isWord(a.source), pinned: a.pinned, showNotes: a.showNotes, notes: a.notes, notesBusy: a.notesBusy, onlyTrunk: a.onlyTrunk, grammarResult: a.grammarResult, grammarResultPrefill: a.grammarResultPrefill, aiRefining: a.aiRefining, aiRefineErr: a.aiRefineErr,
		activeActionID: a.activeActionID, dictResult: a.dictResult, writingResult: a.writingResult, actionSavedMsg: actionSavedMsg, savedDictTerm: a.savedDictTerm, copiedMsg: copiedMsg, showSlashes: a.showSlashes, filterRole: a.filterRole, grammarHoverKnowledge: a.grammarHoverKnowledge, notesSaved: a.notesSaved, grammarSaved: a.grammarSaved, writingSaved: a.writingSaved,
		isScreenshotMode: a.isScreenshotMode, snipBitmap: a.snipBitmap, sourceExpanded: a.sourceExpanded,
		snipExpanded: a.snipExpanded, notesExpanded: a.notesExpanded, ocrBusy: a.ocrBusy, ocrError: a.ocrError, ocrText: a.ocrText,
		ocrCompare: a.ocrCompare, ocrExpanded: a.ocrExpanded,
	}
}

// play speaks a word or sentence in the language the popup knows it is in.
//
// The tag comes from `detected` — the engine's own answer — before the local
// guess, because detectLocal cannot separate French from Dutch and returns ""
// for both. An empty tag used to reach the voice picker and fall through to an
// English voice, which is what made foreign text sound robotic.
func (a *app) play(word string, us bool) {
	PlaySpeechLang(word, a.speechLang(), us)
}

// speechLang returns the tag to speak the popup's text in.
func (a *app) speechLang() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.detected != "" {
		return a.detected
	}
	return detectLocal(a.source)
}

// openPopup shows the window near the cursor and starts the right mode.
//
// In selection mode the text is read *before* the window appears: showing
// or focusing a window moves the keyboard focus away from the application
// the user selected in, and both UI Automation's TextPattern and a
// simulated Ctrl+C answer about the focused application. Capturing first is
// what makes the shortcut translate what the user actually selected.
func (a *app) openPopup(m popupMode) {
	a.hideSelIcon()

	if m == modeSelection {
		app := foregroundAppName()
		sel := a.CaptureSelection()
		if sel.Text == "" {
			// Nothing was selected, or the application would not give it up.
			// Say so: an input box with no explanation reads as "the
			// shortcut opened the wrong thing". An elevated foreground
			// window is the one case where no setting can help, so it gets
			// its own explanation instead of the generic one.
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
		// Honour the "默认模式" preference: with AI 精翻 on, the user picked
		// which card a fresh selection should land in. Without this the
		// setting was inert — it rendered, it persisted, and it changed
		// nothing about what actually opened.
		mode := modeSelection
		if preferred, ok := a.store.Get().InitialAIMode(); ok {
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

// rerunCurrentModeFresh is the footer's 重新翻译: it promises a new answer, so
// it must not be served the cached one the way reopening the same text is.
func (a *app) rerunCurrentModeFresh(text string) {
	// Translate mode has no AI cache: setting the flag there would leak into
	// the next AI pass and silently bypass its cache.
	switch a.mode {
	case modeDictionary, modeGrammar, modeWriting:
		a.mu.Lock()
		a.skipAIResultCache = true
		a.mu.Unlock()
	}
	a.rerunCurrentMode(text)
}

func (a *app) switchMode(m popupMode) {
	cfg := a.store.Get()
	notice := ""
	if problem := cfg.AISetupProblem(); (m == modeGrammar || m == modeWriting) && problem != "" {
		// The AI tabs cannot run without a usable service. Falling back to the
		// machine translation is better than opening a blank card whose only
		// explanation is an error.
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
	// Switching tabs is also switching cards. Save the old tab before its
	// async work is cancelled, then restore the tab's own answer if it has
	// one. An old AI pass or note request must not paint over the new tab.
	a.saveTabLocked()
	a.stopAIPassLocked()
	a.stopNotesLocked()
	text := a.source
	a.mode = m
	a.busy = false
	// 切页签会丢弃当前翻译的展示，但旧 goroutine 仍在飞。不自增代次，
	// 它回来时 gen == a.translateGen 仍通过，会把旧译文写进新模式视图。
	a.translateGen++
	a.notice = notice
	a.errText = ""
	a.echoNote = ""
	if strings.TrimSpace(text) == "" {
		// An empty source means there is no new request to replace the old
		// card. Leaving the old answer on screen under a different tab makes
		// it look as if the newly selected feature produced that answer.
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

// resetPopupTransientLocked 清理「只对当前这张卡片有效」的开关和动作 ID。
// 两个弹窗入口必须用同一份清单：onlyTrunk 曾经只在一条入口里复位，另一条
// 打开的新卡片会继续折叠，缺了从句标记和下划线，看起来像标注坏了。
// 调用方持 a.mu。
func (a *app) resetPopupTransientLocked() {
	a.onlyTrunk = false
	a.showSlashes = false
	a.filterRole = ""
	a.grammarHoverKnowledge = ""
	a.activeActionID = ""
}

// resetScreenshotPinLocked 把「截图翻译临时置顶」的复位规则收在一处：
// 普通弹窗是新卡片，不能继承上一张截图卡片为自身窗口申请的置顶。
// 调用方持 a.mu；用户手动置顶（screenshotPinned 为 false）不动。
func (a *app) resetScreenshotPinLocked() {
	if a.screenshotPinned {
		a.pinned = false
		a.screenshotPinned = false
	}
}

func (a *app) openPopupAction(m popupMode, text string) {
	a.stopScreenshotOCR()
	text = CleanSelectionText(text)
	cfg := a.store.Get()
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

	x, y := a.calcPopupPosition(pt, int(w), initH)
	a.popupX = x
	a.popupY = y
	a.popupW = int(w)
	a.popupH = initH

	a.mu.Lock()
	// Opening a new card supersedes every async result that belonged to the
	// previous one, including the note extraction.
	a.stopAIPassLocked()
	a.stopNotesLocked()
	// A new card has no tab history: results from the previous source must
	// never be restored under the new text.
	a.tabStates = nil
	a.mode = m
	a.inputTargetMode = m
	a.inputText = ""
	a.notice = ""
	a.source = text
	a.sourceExpanded = false
	a.target = ""
	a.errText = ""
	a.echoNote = ""
	a.aiText = ""
	a.dict = nil
	a.gdict = nil
	a.busy = false
	// 新卡片同样要让在飞的机器翻译失效，否则旧译文会写进这张新卡片。
	a.translateGen++
	a.aiBusy = false
	a.dictResult = nil
	a.grammarResult = nil
	a.grammarResultPrefill = false
	a.writingResult = nil
	a.actionSavedMsg = ""
	a.copiedMsg = ""
	a.feedbackText = ""
	a.notesSaved = false
	a.grammarSaved = false
	a.writingSaved = false
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

	win.SetSize(int(w), initH)
	win.SetPosition(x, y)
	win.SetAlwaysOnTop(cfg.AlwaysOnTop)
	win.Show()
	win.Focus()
	win.Update(func() {})

	if text != "" {
		a.rerunCurrentMode(text)
	}
}

// openPopupWithText shows the popup near the cursor and, for the modes that
// have one, translates text that was already captured.
func (a *app) openPopupWithText(m popupMode, text string) {
	a.stopScreenshotOCR()
	text = CleanSelectionText(text)
	cfg := a.store.Get()
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

	x, y := a.calcPopupPosition(pt, int(w), initH)
	a.popupX = x
	a.popupY = y
	a.popupW = int(w)
	a.popupH = initH

	a.mu.Lock()
	// A normal popup is a new card too: the previous AI pass and note request
	// must not write into it after the user has selected another text.
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
	a.sourceExpanded = false
	a.dict, a.gdict, a.busy, a.aiBusy = nil, nil, false, false
	// 新卡片同样要让在飞的机器翻译失效，否则旧译文会写进这张新卡片。
	a.translateGen++
	a.activeActionID = ""
	a.dictResult = nil
	a.writingResult = nil
	a.grammarResult = nil
	a.grammarResultPrefill = false
	a.actionSavedMsg = ""
	a.copiedMsg = ""
	a.feedbackText = ""
	a.notes, a.notesBusy, a.notesSaved, a.notesExpanded = nil, false, false, false
	a.grammarSaved = false
	a.writingSaved = false
	a.resetPopupTransientLocked()
	a.isScreenshotMode, a.snipExpanded = false, false
	a.snipBitmap = nil
	a.ocrText, a.ocrPath, a.ocrError = "", "", ""
	a.ocrCompare, a.ocrExpanded = false, false
	// 普通弹窗不继承截图卡片的置顶；规则见 resetScreenshotPinLocked。
	a.resetScreenshotPinLocked()
	a.mu.Unlock()

	win.SetSize(int(w), initH)
	win.SetPosition(x, y)
	win.SetAlwaysOnTop(cfg.AlwaysOnTop)
	win.Show()
	win.Focus()
	win.Update(func() {})

	if text != "" {
		// Dispatch on the mode we just set. It used to call translate()
		// unconditionally, which is why opening straight into 词典精读 or
		// 长难句分析 showed machine translation instead of the AI card:
		// the mode was set, then ignored by the very next line.
		a.rerunCurrentMode(text)
	}
}

// ---------------------------------------------------------------------------
// The popup view — design: "ink and signal". The translation is the hero,
// marked by a single ink-teal rule; everything else stays quiet.
// ---------------------------------------------------------------------------
