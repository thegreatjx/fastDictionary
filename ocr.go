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

func unwrapOCRText(raw string) string {
	const start, end = "EXTRACTED_TEXT_START", "EXTRACTED_TEXT_END"
	_, body, found := strings.Cut(raw, start)
	if !found {
		return raw
	}
	body, _, _ = strings.Cut(body, end)
	return strings.Trim(body, "\r\n")
}

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

type ocrPreprocessResult struct {
	Path     string
	Cleanup  func()
	Degraded bool
	Reason   string
}

func prepareOCRImage(path string) (string, func(), error) {
	res, err := prepareOCRImageDetailed(path)
	if err != nil {
		return "", func() {}, err
	}
	return res.Path, res.Cleanup, nil
}

func prepareOCRImageDetailed(path string) (ocrPreprocessResult, error) {
	if _, err := os.Stat(path); err != nil {
		return ocrPreprocessResult{Cleanup: func() {}}, fmt.Errorf("读取截图失败：%w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return ocrPreprocessResult{Cleanup: func() {}}, fmt.Errorf("读取截图失败：%w", err)
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return ocrPreprocessResult{Cleanup: func() {}}, fmt.Errorf("解码截图失败：%w", err)
	}

	b := src.Bounds()
	if b.Dx()*snipOCRScale > maxOCRSide || b.Dy()*snipOCRScale > maxOCRSide {
		return ocrPreprocessResult{Path: path, Cleanup: func() {}, Degraded: true, Reason: "图片较大，已按原尺寸识别"}, nil
	}

	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*snipOCRScale, b.Dy()*snipOCRScale))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)

	out, err := os.CreateTemp(filepath.Dir(path), "ocr-*.png")
	if err != nil {
		return ocrPreprocessResult{Path: path, Cleanup: func() {}, Degraded: true, Reason: "放大图无法写入临时目录"}, nil
	}
	name := out.Name()
	if err := png.Encode(out, dst); err != nil {
		out.Close()
		os.Remove(name)
		return ocrPreprocessResult{Path: path, Cleanup: func() {}, Degraded: true, Reason: "放大图编码失败"}, nil
	}
	if err := out.Close(); err != nil {
		os.Remove(name)
		return ocrPreprocessResult{Path: path, Cleanup: func() {}, Degraded: true, Reason: "放大图写入失败"}, nil
	}
	return ocrPreprocessResult{Path: name, Cleanup: func() { os.Remove(name) }}, nil
}

func ocrEmptyTextMessage(degraded bool) string {
	if degraded {
		return "没有识别到文字：图片较大，已按原尺寸识别；可以缩小截图范围后重试"
	}
	return "没有识别到文字，请重新框选清晰文本"
}

func RecognizeImageFile(ctx context.Context, path string) (string, error) {
	return RecognizeImageFileIn(ctx, path, "auto")
}

var recognizeImageFileIn = RecognizeImageFileIn

func RecognizeImageFileIn(ctx context.Context, path, language string) (string, error) {
	var best string
	if indraAvailable() {
		img, err := decodeOCRImage(path)
		if err != nil {
			log.Printf("fastDictionary: decode screenshot failed (%v), falling back to Windows OCR", err)
		} else if text, err := recognizeIndraWithRetries(ctx, img); err == nil {
			best = text
			if best != "" && !isSuspiciousOCRText(best) {
				return best, nil
			}
			log.Printf("fastDictionary: indra result suspicious, checking Windows OCR")
		} else {
			log.Printf("fastDictionary: indra failed (%v), falling back to Windows OCR", err)
		}
	}
	winText, winErr := windowsRecognizeFileIn(ctx, path, language)
	if winErr == nil && (best == "" || scoreOCRText(winText) > scoreOCRText(best)) {
		return winText, nil
	}
	if best != "" {
		return best, nil
	}
	if winErr != nil {
		return "", winErr
	}
	return "", fmt.Errorf("没有识别到文字")
}

func decodeOCRImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("读取截图失败：%w", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("解码截图失败：%w", err)
	}
	return img, nil
}

func indraRecognizeFile(ctx context.Context, path string) (string, error) {
	img, err := decodeOCRImage(path)
	if err != nil {
		return "", err
	}
	return indraRecognize(ctx, img)
}

func indraRecognizePrepared(ctx context.Context, img image.Image) (string, error) {
	text, err := indraRecognize(ctx, adaptivePadForOCR(img))
	if err != nil {
		return "", err
	}
	return CleanOcrText(fixDoubledLettersAll(text)), nil
}

func recognizeIndraWithRetries(ctx context.Context, img image.Image) (string, error) {
	best, err := indraRecognizePrepared(ctx, img)
	if err != nil {
		return "", err
	}
	if !isSuspiciousOCRText(best) || !needsColorRetry(img) {
		return best, nil
	}
	for _, variant := range []image.Image{colorDistanceForOCR(img), backgroundSubtractForOCR(img)} {
		candidate, err := indraRecognizePrepared(ctx, variant)
		if err != nil {
			continue
		}
		if scoreOCRText(candidate) > scoreOCRText(best) {
			best = candidate
		}
		if !isSuspiciousOCRText(best) {
			break
		}
	}
	return best, nil
}

func windowsRecognizeFileIn(ctx context.Context, path, language string) (string, error) {
	scriptPath, err := ensureOCRScript()
	if err != nil {
		return "", fmt.Errorf("准备本地 OCR 失败：%w", err)
	}

	prep, err := prepareOCRImageDetailed(path)
	if err != nil {
		return "", err
	}
	defer prep.Cleanup()
	ocrPath := prep.Path

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
	text := CleanOcrText(unwrapOCRText(string(out)))
	if text == "" {
		return "", fmt.Errorf("%s", ocrEmptyTextMessage(prep.Degraded))
	}
	return text, nil
}

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
	a.stopAIPassLocked()
	a.stopNotesLocked()
	a.mode = modeTranslate
	a.source, a.target, a.engine, a.detected, a.errText, a.notice = "", "", "", "", "", ""
	a.dict, a.gdict, a.busy, a.aiBusy = nil, nil, false, false
	a.aiText, a.activeActionID, a.actionSavedMsg, a.saveErr, a.copiedMsg = "", "", "", "", ""
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
	cfg := a.store.Get()
	mode := normalizeOCRMode(cfg.OCRMode)
	a.stopScreenshotOCR()
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.ocrGeneration++
	generation := a.ocrGeneration
	a.ocrCancel = cancel
	a.ocrBusy, a.ocrError = true, ""
	a.busy = false
	a.translateGen++
	a.source, a.target = "", ""
	a.rawSource = ""
	a.ocrText = ""
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}
	if mode == ocrModeVision {
		go a.recognizeScreenshotVision(ctx, cancel, generation, imgPath, "")
		return
	}
	go func() {
		defer recoverLog("ocr.go:recognizeScreenshot")
		text, err := recognizeImageFileIn(ctx, imgPath, cfg.OCRLang)
		if mode == ocrModeLocalFirst && (err != nil || isSuspiciousOCRText(text)) {
			a.recognizeScreenshotVision(ctx, cancel, generation, imgPath, CleanOcrText(text))
			return
		}
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
			log.Printf("fastDictionary: screenshot OCR: %s", a.logText(text, 80))
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
	if classifyOCRView(s) == ocrViewRecognizing {
		at := appTheme(t)
		ui.Column(c).FillWidth().Gap(6).Padding(12, 2).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(15, 15).TextColor(at.Accent)
				ui.Text(c, "正在识别文字…").FontSize(13).TextColor(at.TextSec).Grow(1)
				if ui.Button(c, "取消").Height(26).FontSize(11).Clicked() {
					a.stopScreenshotOCR()
					a.mu.Lock()
					a.ocrError = "已取消识别。可以重试或重新截图。"
					a.mu.Unlock()
				}
			})
			IndeterminateBar(c, t, 3, "")
		})
		return
	}
	if ocrShowErrorBox(s) {
		ui.Box(c).FillWidth().Padding(8, 10).Radius(RadiusSM).Background(dangerSoft(t)).Border(1, dangerLine(t)).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, s.ocrError).FontSize(12).TextColor(t.Danger).Wrap()
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "重新识别").Height(28).FontSize(12).Clicked() {
						a.retryScreenshotOCR()
					}
					if ui.Button(c, "重新截图").Height(28).FontSize(12).Clicked() {
						a.TriggerScreenshotOCR()
					}
					if s.aiRefining {
						ui.Text(c, "AI 精翻处理中…").FontSize(11).TextColor(t.TextMuted)
					} else if ui.Button(c, "AI 精翻").Height(28).FontSize(12).Tooltip("把截图发给 AI 核对原文并翻译，避免 OCR 错字").Clicked() {
						a.visionRefine()
					}
				})
			})
		})
	}
	if ocrShowEditor(s) {
		ui.Text(c, "识别原文 · 可直接修正（回车翻译 · Shift+回车换行）").FontSize(11).TextColor(t.TextMuted)
		ui.TextArea(c, &a.ocrText).Height(72).FillWidth().
			Placeholder("识别结果会出现在这里，也可直接输入文字 · 回车快速翻译 · Shift+回车换行").
			OnTextKey(func(k ui.TextKey) bool {
				if k.Key == ui.KeyEnter && k.Mods&ui.Shift == 0 {
					a.translateEditedScreenshot()
					return true
				}
				return false
			})
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
			if s.aiRefining {
				ui.Text(c, "AI 精翻处理中…").FontSize(11).TextColor(t.TextMuted)
			} else if ui.Button(c, "AI 精翻").Height(28).FontSize(11).Tooltip("把截图发给 AI 核对原文并翻译，避免 OCR 错字").Clicked() {
				a.visionRefine()
			}
		})
	}
}
