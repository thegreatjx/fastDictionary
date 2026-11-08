package main

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
)

// unwrapOCRText removes the PowerShell transport markers. They delimit stdout
// for reliable extraction; they are not part of the selected image's text.
func unwrapOCRText(raw string) string {
	const start, end = "EXTRACTED_TEXT_START", "EXTRACTED_TEXT_END"
	_, body, found := strings.Cut(raw, start)
	if !found {
		return raw
	}
	body, _, _ = strings.Cut(body, end)
	return strings.Trim(body, "\r\n")
}

// ensureOCRScript writes the WinRT OCR runner to the app cache directory if not present
func ensureOCRScript() (string, error) {
	cacheDir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return "", err
	}
	ocrDir := filepath.Join(cacheDir, "ocr")
	if err := os.MkdirAll(ocrDir, 0755); err != nil {
		return "", err
	}
	scriptPath := filepath.Join(ocrDir, "ocr.ps1")
	// Always sync with the embedded resource (or bundled file) so the OCR script
	// can be updated without requiring the user to clear the cache.
	// For development, read from resources/ocr.ps1.
	data, err := os.ReadFile("resources/ocr.ps1")
	if err != nil {
		resDir, err2 := mygo.App.Path(mygo.PathResources)
		if err2 != nil {
			return "", err
		}
		data, err = os.ReadFile(filepath.Join(resDir, "resources", "ocr.ps1"))
		if err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(scriptPath, data, 0644); err != nil {
		return "", err
	}
	return scriptPath, nil
}

// prepareOCRImage returns a path to a version of the image scaled for the OCR
// engine, plus a cleanup to call when recognition is done.
//
// Windows.Media.Ocr reads small type badly, and screen type is small: a 12px
// CJK glyph is about 12 device pixels tall, which the engine misreads most of
// the time. Measured on a 900x200 crop of real Windows text: 1x turned
// “微软翻译可以识别图片中的文字内容” into “胡译可以识凵片中的文字内冒” — six
// wrong glyphs in fourteen; at 2x the same crop read correctly apart from one
// glyph. English went from “The quick brown ×” to the exact sentence.
//
// Bicubic, not nearest: nearest is visibly worse. Grayscale, contrast and
// thresholding were measured too and none helped — grayscale produced output
// identical to scaling alone, and thresholding made 12px Chinese worse. 2x is
// the sweet spot; 3x and 4x score worse again and cost more.
//
// The scaled copy is written beside the original rather than replacing it: the
// preview shown to the user, and the crop bounds recorded for the next snip,
// both have to stay at native size.
func prepareOCRImage(path string) (string, func(), error) {
	noop := func() {}
	f, err := os.Open(path)
	if err != nil {
		return "", noop, fmt.Errorf("读取截图失败：%w", err)
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return "", noop, fmt.Errorf("解码截图失败：%w", err)
	}

	b := src.Bounds()
	if b.Dx()*snipOCRScale > maxOCRSide || b.Dy()*snipOCRScale > maxOCRSide {
		// Too large to scale: the engine throws instead of returning text, and
		// a 1x read of a huge image still beats an error.
		return path, noop, nil
	}

	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*snipOCRScale, b.Dy()*snipOCRScale))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)

	out, err := os.CreateTemp(filepath.Dir(path), "ocr-*.png")
	if err != nil {
		// A read-only cache dir must not cost the user the translation: fall
		// back to the original image and let the engine do its best.
		return path, noop, nil
	}
	name := out.Name()
	if err := png.Encode(out, dst); err != nil {
		out.Close()
		os.Remove(name)
		return path, noop, nil
	}
	if err := out.Close(); err != nil {
		os.Remove(name)
		return path, noop, nil
	}
	return name, func() { os.Remove(name) }, nil
}

// RecognizeImageFile extracts text with Windows OCR and honors cancellation.
func RecognizeImageFile(ctx context.Context, path string) (string, error) {
	return RecognizeImageFileIn(ctx, path, "auto")
}

// RecognizeImageFileIn is RecognizeImageFile with an explicit OCR language.
//
// The tag is passed to the engine, so an English screenshot can be read by the
// English recognizer on a Chinese Windows install; "auto" keeps the engine's
// own choice from the user's Windows language list. A tag with no recognizer
// installed falls back to that list rather than failing, so a stale setting
// RecognizeImageFileIn reads text from a screenshot.
//
// Indra runs first, because it reads stylised and small type that
// Windows.Media.Ocr turns into noise: on the public "Plaid" OCR test image it
// returns "Fridays & Saturdays at 8 pm" where Windows returns "Fridays U
// Saturdays at 8 pm". Windows OCR stays as the fallback — if the models or the
// runtime are missing, or Indra finds nothing, the screenshot still translates.
func RecognizeImageFileIn(ctx context.Context, path, language string) (string, error) {
	if indraAvailable() {
		if text, err := indraRecognizeFile(ctx, path); err == nil {
			// Repair doubled letters before anything reads the text. The
			// recognizer sometimes writes "Sttocks", which the translator turns
			// into a confident sentence about the wrong subject; see ocr_spell.go.
			text = fixDoubledLettersAll(text)
			if cleaned := CleanOcrText(text); cleaned != "" {
				return cleaned, nil
			}
			log.Printf("fastDictionary: indra found no text, falling back to Windows OCR")
		} else {
			log.Printf("fastDictionary: indra failed (%v), falling back to Windows OCR", err)
		}
	}
	return windowsRecognizeFileIn(ctx, path, language)
}

// indraRecognizeFile decodes a file and hands it to the Indra engine.
//
// No upscaling: the recognizer resizes its own crops to the trained height, and
// the detector works at a fixed long side, so pre-scaling only costs time and
// loses detail.
func indraRecognizeFile(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("读取截图失败：%w", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("解码截图失败：%w", err)
	}
	return indraRecognize(ctx, img)
}

// windowsRecognizeFileIn is the Windows.Media.Ocr path through PowerShell.
func windowsRecognizeFileIn(ctx context.Context, path, language string) (string, error) {
	scriptPath, err := ensureOCRScript()
	if err != nil {
		return "", fmt.Errorf("准备本地 OCR 失败：%w", err)
	}

	// Scaling happens here rather than at capture time so that it covers the
	// retry path too: a failed recognition retries the file already on disk,
	// and a capture-time-only scale would leave that retry reading the
	// original small image and failing the same way.
	ocrPath, cleanup, err := prepareOCRImage(path)
	if err != nil {
		return "", err
	}
	defer cleanup()

	if language == "" {
		language = "auto"
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath, "-ImagePath", ocrPath, "-Language", language)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("本地识别已取消或超时，可以重试或重新截图")
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			message := strings.TrimSpace(string(ee.Stderr))
			if line, _, found := strings.Cut(message, "\n"); found {
				message = strings.TrimSpace(line)
			}
			if message == "" {
				message = err.Error()
			}
			return "", fmt.Errorf("本地识别失败：%s", message)
		}
		return "", fmt.Errorf("无法启动本地识别：%w", err)
	}
	// OCR keeps the image's own line breaks; CleanSelectionText would
	// collapse them and turn a multi-line snip into an unreadable blob.
	text := CleanOcrText(unwrapOCRText(string(out)))
	if text == "" {
		return "", fmt.Errorf("没有识别到文字，请重新框选清晰文本")
	}
	return text, nil
}

// TriggerScreenshotOCR opens the snipper. The translation window appears as
// soon as a crop is available, not after the potentially slow OCR finishes.
func (a *app) TriggerScreenshotOCR() {
	a.hideSelIcon()
	StartNativeSnip(func(rect winRect, imgPath string) {
		a.beginScreenshotOCR(rect, imgPath)
	}, func(err error) {
		log.Printf("fastDictionary: screenshot failed: %v", err)
	})
}

func (a *app) beginScreenshotOCR(rect winRect, imgPath string) {
	data, err := os.ReadFile(imgPath)
	var bmp *ui.Bitmap
	if err == nil {
		bmp, err = ui.DecodeBitmap(data)
	}
	if err != nil {
		log.Printf("fastDictionary: screenshot preview: %v", err)
	}
	a.stopScreenshotOCR()
	a.openPinnedScreenshot(rect, imgPath, bmp)
	a.recognizeScreenshot(imgPath)
}

// openPinnedScreenshot places the window beside the original snip and puts
// it into a structured OCR state. Errors are never stored as source text.
func (a *app) openPinnedScreenshot(rect winRect, imgPath string, bmp *ui.Bitmap) {
	win := a.win
	if win == nil {
		return
	}
	cfg := a.store.Get()
	w := int(popupWidth(cfg.PopupWidth))
	cx, cy := int(rect.Left+(rect.Right-rect.Left)/2), int(rect.Top+(rect.Bottom-rect.Top)/2)
	x, y := a.calcPopupPosition(mygo.Point{X: cx, Y: cy}, w, 520)
	workArea := mygo.Screen.DisplayNearestPoint(mygo.Point{X: cx, Y: cy}).WorkArea
	if int(rect.Right)+8+w <= workArea.X+workArea.Width {
		x = int(rect.Right) + 8
		y = int(rect.Top)
	}
	a.mu.Lock()
	// A screenshot preview is a new card; do not let the previous popup's AI
	// pass or note extraction write into it while OCR is running.
	a.stopAIPassLocked()
	a.stopNotesLocked()
	a.mode = modeTranslate
	a.source, a.target, a.engine, a.detected, a.errText, a.notice = "", "", "", "", "", ""
	a.dict, a.gdict, a.busy, a.aiBusy = nil, nil, false, false
	a.aiText, a.activeActionID, a.actionSavedMsg, a.copiedMsg = "", "", "", ""
	a.dictResult, a.grammarResult, a.writingResult = nil, nil, nil
	a.notes, a.notesBusy, a.notesSaved = nil, false, false
	a.grammarSaved = false
	a.tabStates = nil
	a.inputText, a.ocrText, a.ocrPath, a.ocrCrop = "", "", imgPath, rect
	a.ocrBusy, a.ocrError = true, ""
	a.ocrCompare, a.ocrExpanded = false, false
	a.snipExpanded, a.sourceExpanded, a.notesExpanded = false, false, false
	a.isScreenshotMode, a.screenshotPinned, a.pinned = true, true, true
	a.snipBitmap = bmp
	a.popupX, a.popupY, a.popupW = x, y, w
	a.mu.Unlock()

	win.SetSize(w, 420)
	win.SetPosition(x, y)
	win.SetAlwaysOnTop(true)
	win.Show()
	win.Focus()
	win.Update(func() {})
}

// stopScreenshotOCR cancels PowerShell as well as making its late result
// stale. Merely hiding a busy message would leave an invisible OCR running.
func (a *app) stopScreenshotOCR() {
	a.mu.Lock()
	cancel := a.ocrCancel
	a.ocrCancel = nil
	a.ocrGeneration++
	a.ocrBusy = false
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *app) recognizeScreenshot(imgPath string) {
	a.stopScreenshotOCR()
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.ocrGeneration++
	generation := a.ocrGeneration
	a.ocrCancel = cancel
	a.ocrBusy, a.ocrError = true, ""
	a.busy = false
	a.source, a.target = "", ""
	a.ocrText = ""
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}
	go func() {
		text, err := RecognizeImageFileIn(ctx, imgPath, a.store.Get().OCRLang)
		a.mu.Lock()
		if generation != a.ocrGeneration || !a.isScreenshotMode {
			a.mu.Unlock()
			return
		}
		a.ocrBusy = false
		a.ocrCancel = nil
		if err != nil {
			a.ocrError = err.Error()
			a.ocrText = ""
			a.mu.Unlock()
			log.Printf("fastDictionary: OCR failed: %v", err)
		} else {
			a.ocrText = text
			a.source = text
			a.ocrError = ""
			a.mu.Unlock()
			log.Printf("fastDictionary: screenshot OCR: %q", truncateFor(text, 80))
			a.translateKeepingLines(text)
		}
		if win != nil {
			win.Update(func() {})
		}
		a.fitHeight()
	}()
}

func (a *app) retryScreenshotOCR() {
	a.mu.Lock()
	path := a.ocrPath
	busy := a.ocrBusy
	a.mu.Unlock()
	if path != "" && !busy {
		a.recognizeScreenshot(path)
	}
}

func (a *app) translateEditedScreenshot() {
	a.mu.Lock()
	if a.ocrBusy {
		a.mu.Unlock()
		return
	}
	text := CleanOcrText(a.ocrText)
	a.ocrText = text
	a.ocrError = ""
	a.mu.Unlock()
	if text == "" {
		a.mu.Lock()
		a.ocrError = "请输入识别后的文字，或重新框选。"
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {})
		}
		return
	}
	a.translateKeepingLines(text)
}

// renderScreenshotSource keeps the image optional and makes the OCR result
// editable. Correcting one wrong character must not require another snip.
func (a *app) renderScreenshotSource(c *ui.Context, t *ui.Theme, s snapshot) {
	if s.snipBitmap != nil {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, iconScreenshot).Size(12, 12).TextColor(t.TextMuted)
			ui.Text(c, "截图原文").FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c).Grow(1)
			if s.source != "" {
				if renderIconButton(c, iconCopy, "复制原文", 26, 13, false).Clicked() {
					a.copyToClipboard(s.source)
				}
				if renderIconButton(c, iconSpeak, "朗读原文", 26, 13, false).Clicked() {
					PlaySpeechLang(s.source, a.speechLang(), false)
				}
			}
			label := "查看截图"
			if s.snipExpanded {
				label = "收起截图"
			}
			if ui.Button(c, label).Height(24).Padding(1, 7).FontSize(11).Clicked() {
				a.mu.Lock()
				a.snipExpanded = !a.snipExpanded
				a.mu.Unlock()
				a.fitHeight()
			}
		})
		if s.snipExpanded {
			ui.Image(c, s.snipBitmap).FillWidth().MaxHeight(112).Fit(ui.Contain).Radius(4)
		}
	}
	if s.ocrBusy {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(12, 2).Children(func() {
			ui.Spinner(c).Size(15, 15)
			ui.Text(c, "正在本地识别文字…").FontSize(13).TextColor(t.TextMuted).Grow(1)
			if ui.Button(c, "取消").Height(26).FontSize(11).Clicked() {
				a.stopScreenshotOCR()
				a.mu.Lock()
				a.ocrError = "已取消识别。可以重试或重新截图。"
				a.mu.Unlock()
			}
		})
		return
	}
	if s.ocrError != "" {
		ui.Box(c).FillWidth().Padding(8, 10).Radius(6).Background(noticeBg(t)).Border(1, noticeBorder(t)).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, s.ocrError).FontSize(12).TextColor(noticeText(t)).Wrap()
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "重新识别").Height(28).FontSize(12).Clicked() {
						a.retryScreenshotOCR()
					}
					if ui.Button(c, "重新截图").Height(28).FontSize(12).Clicked() {
						a.TriggerScreenshotOCR()
					}
				})
			})
		})
	}
	if s.source != "" || s.ocrText != "" || s.ocrError != "" {
		ui.Text(c, "识别原文 · 可直接修正").FontSize(11).TextColor(t.TextMuted)
		ui.TextArea(c, &a.ocrText).Height(72).FillWidth().Placeholder("识别结果会出现在这里，也可直接输入文字")
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if ui.PrimaryButton(c, "翻译修正后的原文").Height(28).FontSize(12).Clicked() {
				a.translateEditedScreenshot()
			}
			if ui.Button(c, "重试").Height(28).FontSize(11).Tooltip("重新识别当前截图").Clicked() {
				a.retryScreenshotOCR()
			}
			if ui.Button(c, "重新截图").Height(28).FontSize(11).Clicked() {
				a.TriggerScreenshotOCR()
			}
		})
	}
}
