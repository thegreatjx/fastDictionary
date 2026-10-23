package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
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

// app is the whole client state.
type app struct {
	store *store

	mu             sync.Mutex
	mode           popupMode
	source         string
	target         string
	engine         string
	detected       string
	errText        string
	notice         string
	busy           bool
	elapsed        time.Duration
	dict           *YoudaoDict
	gdict          *DictResult
	aiText         string
	aiBusy         bool
	aiCancel       context.CancelFunc
	inputText      string
	trayInput      string // the tray quick-search box, separate from the popup's input
	sourceExpanded bool

	win                   *mygo.Window
	cfgWin                *mygo.Window
	tray                  *mygo.Tray
	trayFlyout            *mygo.Window
	page                  string
	pinned                bool
	showNotes             bool
	notes                 []NoteItem
	notesBusy             bool
	onlyTrunk             bool
	grammarResult         *SentenceAnalysisResult
	activeActionID        string
	dictResult            *DictionaryResult
	writingResult         *ImproveWritingResult
	actionSavedMsg        string
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
	notesSaved            bool
	isQuitting            bool
	// pausedUntil stops the selection trigger until the moment it holds;
	// the zero value means "not paused".
	pausedUntil      time.Time
	isScreenshotMode bool
	// translateGen counts translations so a slow answer that has been
	// superseded can be recognised and dropped instead of overwriting the
	// newer result.
	translateGen uint64
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

	// the floating selection chip
	sel selState
	// captureSelection reads the current selection; nil uses the real
	// UI Automation and clipboard readers. Tests set it.
	captureSelection func() selection
	// showChip and showPopup place the two outcomes of a selection. They
	// are fields so the trigger policy can be verified without windows.
	showChip  func(x, y int, text string)
	showPopup func(m popupMode, text string)

	// settings window scratch state
	aiTest                  string
	hotkeyMsg               string
	pendingHotkeySelection  string
	pendingHotkeyInput      string
	pendingHotkeyScreenshot string
	magpieModels            []string
	loadingModels           bool
	// newBlockedApp is the text being typed into the selection blocklist
	// editor on the 划词 settings page.
	newBlockedApp string
	// confirmClearVocab turns the 清空 button into a confirm/cancel pair.
	confirmClearVocab bool
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
	notice                string
	busy                  bool
	elapsed               time.Duration
	dict                  *YoudaoDict
	gdict                 *DictResult
	aiText                string
	aiBusy                bool
	inputText             string
	isWord                bool
	pinned                bool
	showNotes             bool
	notes                 []NoteItem
	notesBusy             bool
	onlyTrunk             bool
	grammarResult         *SentenceAnalysisResult
	activeActionID        string
	dictResult            *DictionaryResult
	writingResult         *ImproveWritingResult
	actionSavedMsg        string
	showSlashes           bool
	filterRole            string
	grammarHoverKnowledge string
	notesSaved            bool
	isScreenshotMode      bool
	snipBitmap            *ui.Bitmap
	sourceExpanded        bool
}

func (a *app) snap() snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return snapshot{
		mode: a.mode, source: a.source, target: a.target, engine: a.engine,
		detected: a.detected, errText: a.errText, notice: a.notice, busy: a.busy, elapsed: a.elapsed,
		dict: a.dict, gdict: a.gdict, aiText: a.aiText, aiBusy: a.aiBusy,
		inputText: a.inputText, isWord: isWord(a.source), pinned: a.pinned, showNotes: a.showNotes, notes: a.notes, notesBusy: a.notesBusy, onlyTrunk: a.onlyTrunk, grammarResult: a.grammarResult,
		activeActionID: a.activeActionID, dictResult: a.dictResult, writingResult: a.writingResult, actionSavedMsg: a.actionSavedMsg, showSlashes: a.showSlashes, filterRole: a.filterRole, grammarHoverKnowledge: a.grammarHoverKnowledge, notesSaved: a.notesSaved,
		isScreenshotMode: a.isScreenshotMode, snipBitmap: a.snipBitmap, sourceExpanded: a.sourceExpanded,
	}
}

// translate runs the fast path: engine race, then dictionary data for words.
func (a *app) translate(text string) {
	text = CleanSelectionText(text)
	cfg := a.store.Get()
	win := a.win

	a.mu.Lock()
	if a.aiCancel != nil {
		a.aiCancel()
		a.aiCancel = nil
	}
	a.source, a.target, a.errText = text, "", ""
	a.sourceExpanded = false
	a.engine, a.detected = "", ""
	a.dict, a.gdict, a.aiText, a.aiBusy = nil, nil, "", false
	a.busy = true
	// A new translation invalidates the one in flight. Without this, pressing
	// translate again while the first request was still running let the older
	// answer land last and replace the newer text on screen.
	a.translateGen++
	gen := a.translateGen
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}

	go func() {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		from := detectLocal(text)
		to := cfg.TargetLang
		if from == cfg.TargetLang || sameLang(from, to) {
			to = cfg.SecondLang // reading your own language: translate out
		}

		r := TranslateRace(ctx, text, from, to, cfg.EngineMode)

		word := isWord(text)
		var yd *YoudaoDict
		var gd *DictResult
		if r.Err == nil && word && cfg.DictEnabled {
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); yd, _ = LookupYoudao(ctx, text) }()
			go func() { defer wg.Done(); gd, _ = GoogleDict(ctx, text, from, to) }()
			wg.Wait()
		}

		elapsed := time.Since(start)
		a.mu.Lock()
		if gen != a.translateGen {
			// A newer translation started while this one was in flight: its
			// answer is the one the user is waiting for, so drop this one
			// instead of flashing a stale result.
			a.mu.Unlock()
			log.Printf("fastpot: dropped stale translation of %q", truncateFor(text, 40))
			return
		}
		a.busy = false
		a.elapsed = elapsed
		if r.Err != nil {
			a.errText = r.Err.Error()
		} else {
			a.target = r.Text
			a.engine, a.detected = r.Engine, r.Detected
			a.dict, a.gdict = yd, gd
		}
		a.mu.Unlock()
		log.Printf("fastpot: %q -> %q engine=%s dict=%v in %v (err=%v)",
			truncateFor(text, 40), truncateFor(r.Text, 40), r.Engine, yd != nil, elapsed, r.Err)
		if win != nil {
			win.Update(func() {})
		}
		a.fitHeight()

		// Auto pronunciation for single words, when configured.
		if r.Err == nil && word && cfg.AutoPronounce != "off" {
			a.play(text, cfg.AutoPronounce == "us")
		}
		// If it's a sentence, trigger Read-Frog "猜你想存" extraction in background
		if !word && len(text) > 20 {
			a.extractNotes(text)
		}
	}()
}

// refine streams an AI re-translation from the magpie gateway.

// extractNotes runs the Read-Frog style "猜你想存" note extraction
func (a *app) extractNotes(text string) {
	cfg := a.store.Get()
	if !cfg.AIEnabled || strings.TrimSpace(text) == "" {
		return
	}

	a.mu.Lock()
	a.notesBusy = true
	a.notes = nil
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		systemPrompt := "你是一个面向语言学习者的专业词典与背单词助手。\n" +
			"请从用户选中的英文文本或句子中，分析并挑选出 1 到 2 个最值得学习者记忆的核心生词。\n" +
			"必须为每个生词输出完整的词典数据（包含标准IPA音标、词性、精准释义、CEFR难度等级如A2/B1/B2/C1、简练词源记忆法）。\n" +
			"严格以紧凑的 JSON 数组格式输出，不要有任何 Markdown 标记或多余文字。\n" +
			"示例格式：[{\"word\":\"ambitious\",\"phonetic\":\"/æmˈbɪʃ.əs/\",\"pos\":\"adj.\",\"meaning\":\"有雄心的，抱负不凡的；宏大的\",\"difficulty\":\"B2\",\"memory_tips\":\"词源：ambi-（周围、环顾）+ -it-（走）+ -ious（形容词）→ 四处奔走谋求发展 → 雄心勃勃的\"}]"

		effort := cfg.AIEffort
		if effort == "" {
			effort = "none"
		}
		raw, err := StreamMagpie(ctx, cfg.AIURL, cfg.AIModel, systemPrompt, text, effort, nil)
		if err == nil && raw != "" {
			cleaned := strings.TrimSpace(raw)
			if strings.HasPrefix(cleaned, "```json") {
				cleaned = strings.TrimPrefix(cleaned, "```json")
			}
			if strings.HasPrefix(cleaned, "```") {
				cleaned = strings.TrimPrefix(cleaned, "```")
			}
			cleaned = strings.TrimSuffix(cleaned, "```")
			cleaned = strings.TrimSpace(cleaned)

			var items []NoteItem
			if err := json.Unmarshal([]byte(cleaned), &items); err == nil && len(items) > 0 {
				for i := range items {
					items[i].Checked = true
				}
				a.mu.Lock()
				a.notes = items
				a.mu.Unlock()
			}
		}

		a.mu.Lock()
		a.notesBusy = false
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
			a.fitHeight()
		}
	}()
}

func (a *app) saveDictToVocab(res *DictionaryResult) {
	if res == nil {
		return
	}
	term := res.Term
	if term == "" {
		term = a.source
	}
	entry := VocabularyEntry{
		Term:               term,
		Phonetic:           res.Phonetic,
		PartOfSpeech:       res.PartOfSpeech,
		Definition:         res.Definition,
		Context:            res.Context,
		ContextTerm:        res.ContextTerm,
		ContextTranslation: res.ContextTranslation,
		MemoryTips:         res.MemoryTips,
		Difficulty:         res.Difficulty,
		Source:             "dict",
		CreatedAt:          time.Now(),
	}
	GetVocabularyStore().Add(entry)
	a.mu.Lock()
	a.actionSavedMsg = "已保存「" + term + "」到生词本 ✓"
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) saveGrammarToVocab(res *SentenceAnalysisResult) {
	if res == nil {
		return
	}
	term := truncateFor(a.source, 24)
	entry := VocabularyEntry{
		Term:               term,
		Context:            a.source,
		ContextTranslation: res.Translation,
		Source:             "grammar",
		CreatedAt:          time.Now(),
	}
	GetVocabularyStore().Add(entry)
	a.mu.Lock()
	a.actionSavedMsg = "长难句已保存到生词本 ✓"
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) saveWritingToVocab(res *ImproveWritingResult) {
	if res == nil {
		return
	}
	term := truncateFor(a.source, 24)
	entry := VocabularyEntry{
		Term:               term,
		Definition:         res.Improved,
		Context:            a.source,
		ContextTranslation: res.Summary,
		Source:             "writing",
		CreatedAt:          time.Now(),
	}
	GetVocabularyStore().Add(entry)
	a.mu.Lock()
	a.actionSavedMsg = "写作改进已保存到生词本 ✓"
	a.mu.Unlock()
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) runAction(actionID string, text string) {
	cfg := a.store.Get()
	if !cfg.AIEnabled || strings.TrimSpace(text) == "" {
		return
	}
	act := cfg.GetAction(actionID)
	if act == nil {
		return
	}

	win := a.win
	a.mu.Lock()
	if a.aiCancel != nil {
		a.aiCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	a.aiCancel = cancel
	a.activeActionID = actionID
	a.aiText = ""
	a.aiBusy = true
	a.actionSavedMsg = ""
	// clear previous results for this action
	switch actionID {
	case "default-dictionary":
		a.dictResult = nil
	case "default-sentence-analysis":
		a.grammarResult = nil
	case "default-improve-writing":
		a.writingResult = nil
	}
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}

	go func() {
		targetLangName := langLabels[cfg.TargetLang]
		if targetLangName == "" {
			targetLangName = "简体中文"
		}
		userPrompt := FormatActionPrompt(act.Prompt, text, text, targetLangName)
		systemPrompt := act.SystemPrompt

		model := act.Model
		if model == "" {
			model = cfg.AIModel
		}

		// Streaming used to repaint the entire popup on every chunk, which
		// at a few hundred chunks a second spent more time laying out than
		// generating. Repaint at most every 40 ms; the final Update below
		// still shows the last of the text.
		var lastPaint time.Time
		_, err := StreamMagpie(ctx, cfg.AIURL, model, systemPrompt, userPrompt, cfg.AIEffort, func(chunk string) {
			a.mu.Lock()
			a.aiText += chunk
			due := time.Since(lastPaint) >= 40*time.Millisecond
			if due {
				lastPaint = time.Now()
			}
			a.mu.Unlock()
			if due && win != nil {
				win.Update(func() {})
			}
		})

		a.mu.Lock()
		a.aiBusy = false
		if err != nil {
			// Keep whatever arrived, but say it is incomplete: a truncated
			// answer used to render as an empty card that looked finished.
			a.errText = err.Error()
		}
		if a.aiText != "" {
			switch actionID {
			case "default-dictionary":
				a.dictResult = ParseDictionaryResult(a.aiText)
			case "default-sentence-analysis":
				a.grammarResult = ParseSentenceAnalysis(a.aiText)
			case "default-improve-writing":
				a.writingResult = ParseImproveWritingResult(a.aiText)
			}
		}
		a.aiCancel = nil
		a.mu.Unlock()
		if win != nil {
			win.Update(func() {})
			a.fitHeight()
		}
	}()
}

// calcPopupPosition calculates the ideal position for the popup window,
// proactively lifting it upwards (翻转置于选区上方或向上抬高) when the cursor
// is in the lower half of the screen or near the bottom, matching Read-Frog's behaviour.
func (a *app) calcPopupPosition(pt mygo.Point, targetW, targetH int) (int, int) {
	d := mygo.Screen.DisplayNearestPoint(pt)
	return calcPopupPositionIn(pt, targetW, targetH, d.WorkArea)
}

// calcPopupPositionIn calculates the ideal position for the popup window inside a given work area.
// It is a pure function of its inputs so its behaviour is verifiable without a screen.
func calcPopupPositionIn(pt mygo.Point, targetW, targetH int, wa mygo.Rectangle) (int, int) {
	waX := wa.X
	waY := wa.Y
	waW := wa.Width
	waH := wa.Height
	waRight := waX + waW
	waBottom := waY + waH

	const margin = 14

	// Horizontal: offset slightly to the left of the cursor, clamped to work area
	x := pt.X - 20
	if x+targetW > waRight-margin {
		x = waRight - targetW - margin
	}
	if x < waX+margin {
		x = waX + margin
	}

	// Vertical (主动抬高):
	// Check if cursor is in the lower half of the screen or if downward space is tight
	spaceBelow := waBottom - (pt.Y + 14)
	isLowerHalf := pt.Y > waY+int(float32(waH)*0.50)

	var y int
	if isLowerHalf || spaceBelow < targetH+margin {
		// Flip ABOVE the cursor/selection!
		y = pt.Y - targetH - 14
	} else {
		// Place BELOW the cursor/selection
		y = pt.Y + 14
	}

	// Clamp vertically within screen boundaries
	if y+targetH > waBottom-margin {
		y = waBottom - targetH - margin
	}
	if y < waY+margin {
		y = waY + margin
	}

	return x, y
}

// fitHeight resizes the popup according to its current active mode,
// and proactively lifts the window upwards if expanding downward would exceed the screen.
func (a *app) fitHeight() {
	win := a.win
	if win == nil {
		return
	}
	cfg := a.store.Get()
	s := a.snap()

	// The config clamps PopupWidth at load; honour the user's choice down
	// to that floor instead of forcing the old 440 minimum back on them.
	w := cfg.PopupWidth
	if w < 360 {
		w = 360
	}

	h := float32(160)
	switch s.mode {
	case modeDictionary:
		h = 460
	case modeGrammar:
		h = 420
		if s.source != "" {
			h += float32(wrappedLines(s.source, w-48, 14)) * 16
		}
		if s.grammarResult != nil && s.grammarResult.Translation != "" {
			h += float32(wrappedLines(s.grammarResult.Translation, w-48, 14)) * 16
		}
	case modeWriting:
		// Estimate from the text that is actually rendered: a fixed 40px
		// per annotation was too small for any annotation that wraps, which
		// left the save button below the visible area.
		h = 130
		if s.writingResult != nil {
			wr := s.writingResult
			if wr.Setting != "" {
				h += 30
			}
			for _, ann := range wr.Annotations {
				lines := wrappedLines(ann.Text+" "+ann.Fix, w-120, 13)
				if lines < 1 {
					lines = 1
				}
				h += 18 + float32(lines)*20
				if ann.Note != "" {
					n := wrappedLines(ann.Note, w-72, 12)
					if n < 1 {
						n = 1
					}
					h += float32(n) * 18
				}
				h += 8 // card padding and gap
			}
			if wr.Improved != "" {
				h += 44 + float32(wrappedLines(wr.Improved, w-72, 14))*22
			}
			if wr.Summary != "" {
				h += 40 + float32(wrappedLines(wr.Summary, w-72, 12))*18
			}
			h += 48 // save button row
		}
	case modeInput:
		h += 60
		if s.target != "" {
			h += 120
		}
	default:
		// Translation mode
		if s.source != "" {
			h += 34 + float32(wrappedLines(s.source, w-48, 13))*20
		}
		if s.target != "" {
			h += 38 + float32(wrappedLines(s.target, w-48, 15))*24
		}
		if s.dict != nil || s.gdict != nil {
			h += 60
		}
		if len(s.notes) > 0 {
			h += float32(len(s.notes))*42 + 60
		}
	}

	if h < 260 {
		h = 260
	}

	// Query monitor work area for adaptive lifting
	pt := mygo.Point{X: a.popupX, Y: a.popupY}
	if a.popupX == 0 && a.popupY == 0 {
		pt = mygo.Screen.CursorScreenPoint()
	}
	d := mygo.Screen.DisplayNearestPoint(pt)
	const margin = 14
	waTop := d.WorkArea.Y + margin
	waBottom := d.WorkArea.Y + d.WorkArea.Height - margin
	maxH := float32(waBottom - waTop)
	if h > maxH {
		h = maxH
	}

	targetH := int(h)
	curY := a.popupY

	// 主动抬高：如果当前 y + targetH 超出屏幕工作区底部，则将窗口向上抬高！
	if curY+targetH > waBottom {
		curY = waBottom - targetH
	}
	if curY < waTop {
		curY = waTop
		if targetH > waBottom-waTop {
			targetH = waBottom - waTop
		}
	}

	a.popupY = curY
	a.popupH = targetH
	a.popupW = int(w)

	win.SetSize(int(w), targetH)
	win.SetPosition(a.popupX, curY)
}

// wrappedLines estimates how many lines text needs: CJK glyphs are about a
// font size wide, latin about 0.55 of it.
func wrappedLines(text string, width, fontSize float32) int {
	if text == "" {
		return 0
	}
	usable := width - 46
	if usable < 80 {
		usable = 80
	}
	lines := 0
	for _, para := range strings.Split(text, "\n") {
		w := float32(0)
		n := 1
		for _, r := range para {
			adv := fontSize * 0.55
			if r > 0x2E80 {
				adv = fontSize
			}
			w += adv
			if w > usable {
				n++
				w = adv
			}
		}
		lines += n
	}
	return lines
}

// play speaks a word or sentence.
func (a *app) play(word string, us bool) {
	PlaySpeech(word, us)
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
			// shortcut opened the wrong thing".
			log.Printf("fastpot: no selection from %q; opening input mode", app)
			a.mu.Lock()
			a.notice = "没取到选中的文字（" + app + "）。可以直接在下面输入，或在设置里把「取词方式」改成「模拟复制」。"
			a.mu.Unlock()
			a.openPopupWithText(modeInput, "")
			return
		}
		log.Printf("fastpot: selection %q from %q", truncateFor(sel.Text, 40), app)
		a.openPopupWithText(modeSelection, sel.Text)
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

func (a *app) switchMode(m popupMode) {
	a.mu.Lock()
	a.mode = m
	text := a.source
	a.busy = false
	a.aiBusy = false
	a.notice = ""
	a.errText = ""
	a.mu.Unlock()
	if strings.TrimSpace(text) != "" {
		a.rerunCurrentMode(text)
	} else if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) openPopupAction(m popupMode, text string) {
	text = CleanSelectionText(text)
	cfg := a.store.Get()
	win := a.win
	if win == nil {
		return
	}

	a.hideSelIcon()

	pt := mygo.Screen.CursorScreenPoint()
	w := cfg.PopupWidth
	if w < 440 {
		w = 440
	}
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
	a.mode = m
	a.inputText = ""
	a.notice = ""
	a.source = text
	a.sourceExpanded = false
	a.target = ""
	a.errText = ""
	a.aiText = ""
	a.dict = nil
	a.gdict = nil
	a.busy = false
	a.aiBusy = false
	a.dictResult = nil
	a.grammarResult = nil
	a.writingResult = nil
	a.actionSavedMsg = ""
	a.notesSaved = false
	a.showSlashes = false
	a.filterRole = ""
	a.grammarHoverKnowledge = ""
	a.notes = nil
	a.notesBusy = false
	a.isScreenshotMode = false
	a.snipBitmap = nil
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
	text = CleanSelectionText(text)
	cfg := a.store.Get()
	win := a.win
	if win == nil {
		return
	}

	pt := mygo.Screen.CursorScreenPoint()
	w := cfg.PopupWidth
	if w < 440 {
		w = 440
	}
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
	a.mode = m
	a.inputText = ""
	a.notice = ""
	a.source, a.target, a.errText, a.aiText = "", "", "", ""
	a.sourceExpanded = false
	a.dict, a.gdict, a.busy, a.aiBusy = nil, nil, false, false
	a.activeActionID = ""
	a.dictResult = nil
	a.writingResult = nil
	a.grammarResult = nil
	a.actionSavedMsg = ""
	// A normal popup starts unpinned, even right after a screenshot
	// translation (which pins itself on purpose). Without this the pin
	// survived into every later popup and OnBlur never closed them again.
	if a.screenshotPinned {
		a.pinned = false
		a.screenshotPinned = false
	}
	a.mu.Unlock()

	win.SetSize(int(w), initH)
	win.SetPosition(x, y)
	win.SetAlwaysOnTop(cfg.AlwaysOnTop)
	win.Show()
	win.Focus()
	win.Update(func() {})

	if text != "" {
		a.translate(text)
	}
}

// ---------------------------------------------------------------------------
// The popup view — design: "ink and signal". The translation is the hero,
// marked by a single ink-teal rule; everything else stays quiet.
// ---------------------------------------------------------------------------

// renderIconButton renders a square icon button where the icon is guaranteed
// to be strictly horizontally and vertically centered with crisp feedback.
func renderIconButton(c *ui.Context, icon *ui.SVG, tip string, size, iconSize float32, active bool) ui.Element {
	t := c.Theme()
	b := ui.Box(c).Size(size, size).Radius(5).Center().Tooltip(tip)
	bg := ui.Color{}
	fg := t.TextMuted
	if active {
		bg = t.SurfaceHover
		fg = t.Accent
	}
	if b.Hovered() {
		bg = t.SurfaceHover
		fg = t.Text
	}
	if b.Pressed() {
		bg = t.SurfacePressed
	}
	b.Background(bg)
	b.Children(func() {
		ui.Icon(c, icon).Size(iconSize, iconSize).TextColor(fg)
	})
	return b
}

func (a *app) popupView(c *ui.Context) {
	cfg := a.store.Get()
	dark := cfg.ThemeMode == "dark"
	applyTheme(c, cfg.ThemeMode, dark)
	t := c.Theme()
	s := a.snap()

	if c.Shortcut(0, ui.KeyEscape) && a.win != nil {
		stopAudio()
		a.win.Hide()
	}

	ui.Column(c).FillWidth().FillHeight().Padding(10, 14, 10, 14).Gap(10).Children(func() {
		// ===================================================================
		// 1. Top Header: [Mode Tabs] ... [Lang ▾] [📌] [✕] (Self-adaptive, SpaceBetween)
		// ===================================================================
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).DragWindow().Children(func() {
			// Left: Mode segmented pill switcher: 翻译 / 长难句 / 词典
			modeIdx := 0
			switch s.mode {
			case modeGrammar:
				modeIdx = 1
			case modeDictionary:
				modeIdx = 2
			case modeWriting:
				modeIdx = 3
			}
			// 写作 is reachable from the chip and from a custom action; it
			// needs a tab of its own or the segmented control highlights
			// 翻译 while the window shows something else.
			ui.Segmented(c, &modeIdx, "翻译", "长难句", "词典", "写作").Height(26).FontSize(12).OnChange(func() {
				targetModes := []popupMode{modeTranslate, modeGrammar, modeDictionary, modeWriting}
				if modeIdx >= 0 && modeIdx < len(targetModes) {
					a.switchMode(targetModes[modeIdx])
				}
			})

			// Right Actions: [Target Lang ▾] [📌] [✕] (Always right-aligned, perfectly centered!)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if s.mode == modeTranslate || s.mode == modeInput {
					targetLabel := langLabels[cfg.TargetLang]
					if targetLabel == "" {
						targetLabel = "简体中文"
					}
					ui.Select(c, &targetLabel, langList()).Height(26).FontSize(12).OnChange(func() {
						code := langFromLabel(targetLabel)
						a.store.Update(func(c *Config) { c.TargetLang = code })
						if s.source != "" {
							a.rerunCurrentMode(s.source)
						}
					})
				}

				// [📌] Pin Button - perfectly centered!
				pinIcon := iconPin
				pinTip := "固定窗口 (失焦不关闭)"
				if s.pinned {
					pinIcon = iconPinned
					pinTip = "取消固定窗口"
				}
				btnPin := renderIconButton(c, pinIcon, pinTip, 30, 15, s.pinned)
				if btnPin.Clicked() {
					a.mu.Lock()
					a.pinned = !a.pinned
					isPinned := a.pinned
					a.mu.Unlock()
					if a.win != nil {
						a.win.SetAlwaysOnTop(isPinned || cfg.AlwaysOnTop)
						a.win.Update(func() {})
					}
				}

				// [✕] Close Button - perfectly centered!
				btnClose := renderIconButton(c, iconClose, "关闭 (Esc)", 30, 14, false)
				if btnClose.Clicked() && a.win != nil {
					stopAudio()
					a.win.Hide()
				}
			})
		})

		ui.Divider(c).FillWidth()

		// ===================================================================
		// 2. Main Content Scroll View (Independent Dedicated Views)
		// ===================================================================
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(10).Children(func() {
				// A notice explains why the shortcut opened something other
				// than what the user expected (no selection, for instance).
				// It used to be stored and never rendered, so the user saw an
				// empty input box with no explanation at all.
				if s.notice != "" {
					ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
						Background(noticeBg(t)).Border(1, noticeBorder(t)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(6).Children(func() {
							ui.Icon(c, iconInfo).Size(13, 13).TextColor(noticeText(t))
							ui.Text(c, s.notice).FontSize(12).TextColor(noticeText(t)).Wrap().Grow(1)
						})
					})
				}

				// Show general error if present
				if s.errText != "" {
					ui.Text(c, s.errText).FontSize(13).TextColor(t.Danger).Wrap()
				}

				// AI is busy banner
				if s.aiBusy {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(16, 6).Children(func() {
						ui.Spinner(c).Size(14, 14)
						busyMsg := "AI 正在分析中..."
						switch s.mode {
						case modeDictionary:
							busyMsg = "AI 词典导师正在生成词条与例句..."
						case modeGrammar:
							busyMsg = "AI 语法导师正在深度拆解长难句..."
						case modeWriting:
							busyMsg = "AI 写作导师正在评估并批改句子..."
						}
						ui.Text(c, busyMsg).FontSize(13).TextColor(t.TextMuted)
					})
				}

				// RENDER CLEAN HERO CONTENT BY MODE:
				switch s.mode {
				case modeGrammar:
					// Dedicated Sentence Analysis Mode!
					if s.grammarResult != nil {
						a.renderGrammarVisualCard(c, t, s.source, s.grammarResult, func() { a.saveGrammarToVocab(s.grammarResult) })
					} else if s.aiText != "" {
						renderAiMarkdown(c, s.aiText, s.aiBusy)
					}

				case modeDictionary:
					// Dedicated Learner's Dictionary Mode!
					if s.dictResult != nil {
						renderDictionaryCard(c, t, s.source, s.dictResult, func() { a.saveDictToVocab(s.dictResult) })
					} else if s.aiText != "" {
						renderAiMarkdown(c, s.aiText, s.aiBusy)
					}

				case modeWriting:
					// Dedicated Improve Writing Mode!
					if s.writingResult != nil {
						renderImproveWritingCard(c, t, s.source, s.writingResult, func() { a.saveWritingToVocab(s.writingResult) })
					} else if s.aiText != "" {
						renderAiMarkdown(c, s.aiText, s.aiBusy)
					}

				default:
					// TRANSLATION MODE (modeTranslate / modeInput / modeSelectionPreview)
					if s.mode == modeInput {
						ui.Box(c).FillWidth().Padding(10, 12).Radius(8).
							Background(t.Surface).Border(1, t.Border).Children(func() {
							ui.TextInput(c, &a.inputText).
								Placeholder("输入要翻译的内容 · 回车翻译 · Shift+回车换行 · Esc 退出").
								AutoFocus().
								OnTextKey(func(k ui.TextKey) bool {
									if k.Key == ui.KeyEnter {
										if txt := CleanSelectionText(k.Text()); txt != "" {
											a.translate(txt)
										}
										return true
									}
									if k.Key == ui.KeyEscape {
										if a.win != nil {
											stopAudio()
											a.win.Hide()
										}
										return true
									}
									return false
								}).
								OnSubmit(func() {
									if txt := CleanSelectionText(a.inputText); txt != "" {
										a.translate(txt)
									}
								})
						})
					}

					// Pinned Screenshot Preview (if in screenshot translation mode)
					if s.isScreenshotMode && s.snipBitmap != nil {
						ui.Box(c).FillWidth().Padding(6).Radius(8).Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
							ui.Column(c).FillWidth().Gap(4).Children(func() {
								ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
									ui.Icon(c, iconScreenshot).Size(12, 12).TextColor(t.Accent)
									ui.Text(c, "截屏原文 (已原地钉在屏幕 · 旁边实时对照)").FontSize(11).FontWeight(600).TextColor(t.Accent)
								})
								ui.Image(c, s.snipBitmap).FillWidth().MaxHeight(140).Fit(ui.Contain).Radius(4)
							})
						})
					}

					// Source Text (Collapsible & Tidy formatting, inspired by Read-Frog)
					if s.source != "" {
						ui.Column(c).FillWidth().Gap(6).Children(func() {
							runes := []rune(s.source)
							isLong := len(runes) > 110 || strings.Contains(s.source, "\n")
							displayText := s.source
							if isLong && !s.sourceExpanded {
								displayText = TruncateEllipsis(s.source, 110)
							}

							ui.Row(c).FillWidth().AlignItems(ui.Start).Justify(ui.SpaceBetween).Gap(8).Children(func() {
								// Source text content
								ui.Text(c, displayText).FontSize(14).TextColor(t.TextMuted).Wrap().Grow(1)

								// Action buttons: toggle expand/collapse, copy source, speak source
								ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
									if isLong {
										chevronIcon := iconChevronDown
										toggleTip := "展开完整原文"
										if s.sourceExpanded {
											chevronIcon = iconChevronUp
											toggleTip = "收起原文"
										}
										btnToggle := renderIconButton(c, chevronIcon, toggleTip, 26, 13, false)
										if btnToggle.Clicked() {
											a.mu.Lock()
											a.sourceExpanded = !a.sourceExpanded
											a.mu.Unlock()
											if a.win != nil {
												a.win.Update(func() {})
											}
										}
									}

									btnCopySrc := renderIconButton(c, iconCopy, "复制原文", 26, 13, false)
									if btnCopySrc.Clicked() {
										mygo.Clipboard.WriteText(s.source)
									}

									btnSpeakSrc := renderIconButton(c, iconSpeak, "朗读原文", 26, 13, false)
									if btnSpeakSrc.Clicked() {
										PlaySpeech(s.source, false)
									}
								})
							})
						})
					}

					ui.Divider(c).FillWidth()

					// Target Translation Area
					if s.busy {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(10, 0).Children(func() {
							ui.Spinner(c).Size(14, 14)
							ui.Text(c, "翻译中...").FontSize(13).TextColor(t.TextMuted)
						})
					}

					if s.target != "" {
						ui.Column(c).FillWidth().Gap(8).Children(func() {
							ui.Text(c, s.target).FontSize(15).FontWeight(600).TextColor(t.Text).Wrap()

							// The "显示引擎与语种" switch, honoured at last: a small
							// caption of which engine won the race and what it took.
							// It was a stored-but-never-rendered setting before.
							if cfg.ShowEngineTag && s.engine != "" {
								ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
									tag := s.engine
									if s.detected != "" && !sameLang(s.detected, cfg.TargetLang) {
										tag += " · 检测 " + s.detected
									}
									if s.elapsed > 0 {
										tag += fmt.Sprintf(" · %dms", s.elapsed.Milliseconds())
									}
									ui.Text(c, tag).FontSize(10).FontWeight(600).TextColor(t.TextMuted)
								})
							}

							// Action icons: [📋 复制] [🔊 朗读] (Centered)
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								btnCopy := renderIconButton(c, iconCopy, "复制译文", 28, 14, false)
								if btnCopy.Clicked() {
									mygo.Clipboard.WriteText(s.target)
								}

								btnSpeak := renderIconButton(c, iconSpeak, "朗读译文", 28, 14, false)
								if btnSpeak.Clicked() {
									PlaySpeech(s.target, false)
								}
							})

							// Dictionary breakdown if word
							if s.dict != nil || s.gdict != nil {
								dictBlock(c, t, s, a)
							}
						})
					}

					// 猜你想存 ONLY in Translation mode!
					if len(s.notes) > 0 || s.notesBusy {
						if a.store.Get().ShowNotes {
							renderNotesBlock(c, t, s, a)
						}
					}
				}
			})
		})

		ui.Divider(c).FillWidth()

		// ===================================================================
		// 3. Bottom Bar
		// ===================================================================
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			if s.mode == modeTranslate || s.mode == modeInput || s.mode == modeSelectionPreview {
				// Provider Selector [ ⊞ Microsoft Translator ▾ ]
				provLabel := "Microsoft Translator"
				switch cfg.EngineMode {
				case "google":
					provLabel = "Google Translate"
				case "race":
					provLabel = "Microsoft Translator (竞速)"
				}
				provOptions := []string{
					"Microsoft Translator",
					"Google Translate",
					"双路竞速模式",
				}
				ui.Select(c, &provLabel, provOptions).Height(26).FontSize(11).OnChange(func() {
					switch provLabel {
					case "Google Translate":
						a.store.Update(func(c *Config) { c.EngineMode = "google" })
					case "Microsoft Translator":
						a.store.Update(func(c *Config) { c.EngineMode = "microsoft" })
					default:
						a.store.Update(func(c *Config) { c.EngineMode = "race" })
					}
					if s.source != "" && !s.busy {
						a.translate(s.source)
					}
				})
			} else {
				// AI Mode Model Indicator
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconSparkles).Size(13, 13).TextColor(t.Accent)
					modelTag := cfg.AIModel
					if act := cfg.GetAction(a.activeActionID); act != nil && act.Model != "" {
						modelTag = act.Model
					}
					ui.Text(c, modelTag).FontSize(11).TextColor(t.TextMuted)
				})
			}

			ui.Spacer(c).Grow(1)

			btnFull := ui.Button(c, "").Tooltip("全屏/窗口切换").Height(24).Padding(2, 6).Radius(4)
			btnFull.Children(func() {
				ui.Icon(c, iconExpand).Size(13, 13).TextColor(t.TextMuted)
			})
			if btnFull.Clicked() && a.win != nil {
				a.win.ToggleFullScreen()
			}

			btnRefresh := ui.Button(c, "").Tooltip("重新运行").Height(24).Padding(2, 6).Radius(4)
			btnRefresh.Children(func() {
				ui.Icon(c, iconRefresh).Size(13, 13).TextColor(t.TextMuted)
			})
			if btnRefresh.Clicked() && s.source != "" {
				a.rerunCurrentMode(s.source)
			}
		})
	})
}

func renderNotesBlock(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	ui.Box(c).FillWidth().Padding(12, 14).Radius(8).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.06)).Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconBookmark).Size(15, 15).TextColor(t.TextMuted)
					ui.Text(c, "猜你想存").FontSize(13).FontWeight(600).TextColor(t.Text)
				})
				ui.Spacer(c).Grow(1)
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "显示猜你想存").FontSize(11).TextColor(t.TextMuted)
					// Bound to the live config: the local variable this used
					// to read was recreated on every frame, so the switch
					// always rendered as on and clicking it did nothing.
					enabled := a.store.Get().ShowNotes
					ui.Switch(c, &enabled).Height(18).OnChange(func() {
						a.store.Update(func(c *Config) { c.ShowNotes = enabled })
						if a.win != nil {
							a.win.Update(func() {})
						}
					})
				})
			})

			ui.Text(c, "选择要存入笔记本的词，将来随时复习").FontSize(11).TextColor(t.TextMuted)

			if s.notesBusy && len(s.notes) == 0 {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(8, 0).Children(func() {
					ui.Spinner(c).Size(12, 12)
					ui.Text(c, "AI 正在提取生词...").FontSize(12).TextColor(t.TextMuted)
				})
			}

			for idx := range s.notes {
				item := &s.notes[idx]
				ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
					Background(t.SurfaceHover).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(8).Children(func() {
						ui.Checkbox(c, &item.Checked, "")
						ui.Column(c).FillWidth().Gap(2).Children(func() {
							ui.Text(c, item.Word).FontSize(13).FontWeight(600).TextColor(t.Text)
							desc := item.Pos + " " + item.Meaning
							ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).Wrap()
						})
					})
				})
			}

			if len(s.notes) > 0 {
				ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
					if s.notesSaved {
						ui.Box(c).Height(24).Padding(2, 12).Radius(5).Background(ui.Hex("#10B981")).Children(func() {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								ui.Text(c, "✓ 已存入生词本").FontSize(11).FontWeight(600).TextColor(ui.Hex("#FFFFFF"))
							})
						})
					} else {
						btnSave := ui.Button(c, "").Height(24).Padding(2, 14).Radius(5).Background(ui.Hex("#EAB308")).TextColor(ui.Hex("#FFFFFF"))
						btnSave.Children(func() {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								ui.Icon(c, iconBookmark).Size(12, 12).TextColor(ui.Hex("#FFFFFF"))
								ui.Text(c, "保存所选生词").FontSize(11).FontWeight(600)
							})
						})
						if btnSave.Clicked() {
							// Collect under the lock, do the slow part
							// outside it. EnrichVocabularyEntry makes a
							// network call with a 3 s timeout and Add
							// rewrites the whole notebook file, so the old
							// code held a.mu - the render lock - for up to
							// three seconds per word and the window went
							// "not responding".
							a.mu.Lock()
							a.notesSaved = true
							a.actionSavedMsg = "已将所选生词存入生词库 ✓"
							pending := make([]VocabularyEntry, 0, len(s.notes))
							for _, n := range s.notes {
								if n.Checked {
									pending = append(pending, VocabularyEntry{
										Term:               n.Word,
										Phonetic:           n.Phonetic,
										PartOfSpeech:       n.Pos,
										Definition:         n.Meaning,
										Difficulty:         n.Difficulty,
										MemoryTips:         n.MemoryTips,
										Context:            s.source,
										ContextTerm:        n.Word,
										ContextTranslation: s.target,
										Source:             "notes",
										CreatedAt:          time.Now(),
									})
								}
							}
							a.mu.Unlock()
							if a.win != nil {
								a.win.Update(func() {})
							}
							go func() {
								for i := range pending {
									EnrichVocabularyEntry(&pending[i])
								}
								GetVocabularyStore().AddBatch(pending)
							}()
						}
					}
				})
			}
		})
	})
}

// dictBlock renders pronunciation controls and meanings. The IPA is quiet
// text beside a small speaker chip, so the phonetics read as part of the
// word instead of as a button label.
func dictBlock(c *ui.Context, t *ui.Theme, s snapshot, a *app) {
	ui.Divider(c).FillWidth()
	if s.dict != nil && (s.dict.UKPhone != "" || s.dict.USPhone != "") {
		ui.Row(c).FillWidth().Gap(20).AlignItems(ui.Center).Children(func() {
			if s.dict.UKPhone != "" {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					btnUK := ui.Button(c, "🔊 英").Height(22).Padding(1, 6).Radius(4).Tooltip("英式发音")
					if btnUK.Clicked() && a != nil {
						a.play(s.source, false)
					}
					ui.Text(c, "/"+s.dict.UKPhone+"/").FontSize(13).TextColor(t.TextMuted)
				})
			}
			if s.dict.USPhone != "" {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					btnUS := ui.Button(c, "🔊 美").Height(22).Padding(1, 6).Radius(4).Tooltip("美式发音")
					if btnUS.Clicked() && a != nil {
						a.play(s.source, true)
					}
					ui.Text(c, "/"+s.dict.USPhone+"/").FontSize(13).TextColor(t.TextMuted)
				})
			}

			ui.Spacer(c).Grow(1)

			if a != nil {
				saveDictLabel := "⭐ 存入生词库"
				saveDictBg := ui.Hex("#EAB308")
				if a.actionSavedMsg != "" {
					saveDictLabel = "✓ 已存入生词库"
					saveDictBg = ui.Hex("#10B981")
				}
				btnSaveDict := ui.Button(c, saveDictLabel).Height(22).Padding(1, 8).Radius(4).
					Background(saveDictBg).TextColor(ui.Hex("#FFFFFF")).Tooltip("将该单词保存到生词库")
				if btnSaveDict.Clicked() {
					phone := ""
					if s.dict != nil {
						if s.dict.UKPhone != "" {
							phone = "/" + s.dict.UKPhone + "/"
						} else if s.dict.USPhone != "" {
							phone = "/" + s.dict.USPhone + "/"
						}
					}
					expl := s.target
					pos := ""
					if s.gdict != nil && len(s.gdict.Entries) > 0 {
						pos = s.gdict.Entries[0].POS
					} else if s.dict != nil && len(s.dict.Explains) > 0 {
						expl = strings.Join(s.dict.Explains, "；")
						parts := strings.SplitN(s.dict.Explains[0], " ", 2)
						if len(parts) == 2 && strings.HasSuffix(parts[0], ".") {
							pos = parts[0]
						}
					}
					entry := VocabularyEntry{
						Term:         s.source,
						Phonetic:     phone,
						PartOfSpeech: pos,
						Definition:   expl,
						Source:       "dict",
						CreatedAt:    time.Now(),
					}
					EnrichVocabularyEntry(&entry)
					GetVocabularyStore().Add(entry)
					a.mu.Lock()
					a.actionSavedMsg = "已保存「" + s.source + "」到生词库 ✓"
					a.mu.Unlock()
					if a.win != nil {
						a.win.Update(func() {})
					}
				}
			}
		})
	} else if s.gdict != nil && s.gdict.Pron != "" {
		ui.Text(c, "/"+s.gdict.Pron+"/").FontSize(13).TextColor(t.Accent)
	}
	if s.gdict != nil {
		for _, e := range s.gdict.Entries {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
				ui.Text(c, e.POS).FontSize(12).TextColor(t.Accent).Width(56).FontWeight(600)
				ui.Text(c, e.Explain).FontSize(13).TextColor(t.Text).Grow(1).Wrap()
			})
		}
	} else if s.dict != nil {
		for _, ex := range s.dict.Explains {
			ui.Text(c, ex).FontSize(13).TextColor(t.Text).Wrap()
		}
	}
}

func (a *app) renderEmptyActionStart(c *ui.Context, t *ui.Theme, s snapshot, actionID, tip string) {
	ui.Row(c).FillWidth().Padding(6, 4).AlignItems(ui.Center).Children(func() {
		ui.Text(c, tip).FontSize(12).TextColor(t.TextMuted)
		ui.Spacer(c).Grow(1)
		btnStart := ui.Button(c, "").Height(24).Padding(2, 10).Radius(5).Background(ui.Hex("#2563EB")).TextColor(ui.Hex("#FFFFFF"))
		btnStart.Children(func() {
			ui.Text(c, "立即开始").FontSize(11).FontWeight(600)
		})
		if btnStart.Clicked() {
			txt := s.source
			if txt == "" {
				txt = s.target
			}
			if txt != "" {
				a.runAction(actionID, txt)
			}
		}
	})
}

// sameLang reports whether a detected tag and a target tag are the same
// language in practice (zh-CN vs zh-Hans and friends).
func sameLang(a, b string) bool {
	if a == b {
		return true
	}
	pa, pb := langPrefix(a), langPrefix(b)
	return pa == pb
}

func langPrefix(tag string) string {
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		return strings.ToLower(tag[:i])
	}
	return strings.ToLower(tag)
}

// renderAiMarkdown displays AI output using MyGo's markdown component:
// streaming with fade-in when busy, and complete rich view when finished.
func renderAiMarkdown(c *ui.Context, text string, busy bool) {
	if busy {
		markdown.Stream(c, text, nil)
	} else {
		markdown.View(c, text, nil)
	}
}
