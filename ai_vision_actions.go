package main

import (
	"context"
	"log"
	"time"
)

// visionRefine 是截图模式「AI 精翻」按钮的实现。它直接把压缩截图发给模型，
// 同时把本地 OCR 文本作为提示；模型以图片为准，流式刷新原文和译文。
func (a *app) visionRefine() {
	cfg := a.store.Get()
	if problem := cfg.AISetupProblem(); problem != "" {
		a.setVisionRefineError(problem)
		return
	}
	a.mu.Lock()
	if a.aiRefining {
		a.mu.Unlock()
		return
	}
	if !a.isScreenshotMode || a.ocrPath == "" {
		a.mu.Unlock()
		a.refineWithAI()
		return
	}
	path := a.ocrPath
	sourceHint := a.source
	targetLang := cfg.TargetLang
	a.aiRefineGen++
	if a.aiRefineCancel != nil {
		a.aiRefineCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	a.aiRefineCancel = cancel
	gen := a.aiRefineGen
	ocrGen := a.ocrGeneration
	a.translateGen++
	a.aiRefining = true
	a.aiRefineErr = ""
	a.ocrError = ""
	a.busy = false
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}

	started := time.Now()
	go func() {
		defer recoverLog("ai_vision_actions.go:visionRefine")
		defer cancel()
		dataURL, err := compressedImageFileDataURL(path)
		if err != nil {
			a.finishVisionRefineError(ctx, gen, ocrGen, err.Error())
			return
		}
		parser := &visionStreamParser{}
		out, err := StreamAIWithImage(ctx, cfg.AIConn(cfg.AIModel),
			visionSystemPrompt(targetLang), visionUserPrompt(sourceHint), dataURL,
			cfg.AIReasoningEffortFor(cfg.AIModel), func(delta string) {
				source, target := parser.Push(delta)
				if source == "" && target == "" {
					return
				}
				a.mu.Lock()
				if gen != a.aiRefineGen || ocrGen != a.ocrGeneration || !a.isScreenshotMode {
					a.mu.Unlock()
					return
				}
				if source != "" {
					a.ocrText = CleanOcrText(source)
					a.source = a.ocrText
				}
				if target != "" {
					a.target = flattenTranslation(target)
				}
				win := a.win
				a.mu.Unlock()
				if win != nil {
					win.Update(func() {})
				}
			})
		if err != nil {
			a.finishVisionRefineError(ctx, gen, ocrGen, friendlyAIError(err))
			return
		}
		source, target := parseVisionOutput(out, sourceHint)
		if target == "" {
			a.finishVisionRefineError(ctx, gen, ocrGen, "模型没有返回译文")
			return
		}
		a.mu.Lock()
		if gen != a.aiRefineGen || ocrGen != a.ocrGeneration || !a.isScreenshotMode {
			a.mu.Unlock()
			return
		}
		a.aiRefining = false
		a.aiRefineCancel = nil
		a.aiRefineErr = ""
		a.ocrError = ""
		a.ocrText = CleanOcrText(source)
		a.source = a.ocrText
		a.target = target
		a.engine = "AI 精翻"
		a.elapsed = time.Since(started)
		win := a.win
		a.mu.Unlock()
		if win != nil {
			win.Update(func() {})
		}
		a.fitHeight()
	}()
}

func (a *app) setVisionRefineError(message string) {
	a.mu.Lock()
	a.aiRefineErr = message
	a.ocrError = "AI 精翻失败：" + message
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}
}

func (a *app) finishVisionRefineError(ctx context.Context, gen, ocrGen requestToken, message string) {
	if ctx.Err() != nil {
		message = ""
	}
	a.mu.Lock()
	if gen != a.aiRefineGen || ocrGen != a.ocrGeneration || !a.isScreenshotMode {
		a.mu.Unlock()
		return
	}
	a.aiRefining = false
	a.aiRefineCancel = nil
	if message != "" {
		a.aiRefineErr = message
		a.ocrError = "AI 精翻失败：" + message
	} else {
		a.aiRefineErr = ""
		a.ocrError = ""
	}
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}
	a.fitHeight()
}

// recognizeScreenshotVision 是「AI 直接看图」模式和「本地优先」自动复核共用的入口。
// fallbackText 非空时，AI 请求失败仍保留本地 OCR 结果并继续机器翻译。
func (a *app) recognizeScreenshotVision(ctx context.Context, cancel context.CancelFunc, generation requestToken, path, fallbackText string) {
	defer recoverLog("ai_vision_actions.go:recognizeScreenshotVision")
	defer cancel()
	cfg := a.store.Get()
	if problem := cfg.AISetupProblem(); problem != "" {
		a.finishScreenshotVisionError(generation, fallbackText, problem)
		return
	}
	dataURL, err := compressedImageFileDataURL(path)
	if err != nil {
		a.finishScreenshotVisionError(generation, fallbackText, err.Error())
		return
	}
	parser := &visionStreamParser{}
	started := time.Now()
	out, err := StreamAIWithImage(ctx, cfg.AIConn(cfg.AIModel),
		visionSystemPrompt(cfg.TargetLang), visionUserPrompt(fallbackText), dataURL,
		cfg.AIReasoningEffortFor(cfg.AIModel), func(delta string) {
			source, target := parser.Push(delta)
			if source == "" && target == "" {
				return
			}
			a.mu.Lock()
			if generation != a.ocrGeneration || !a.isScreenshotMode {
				a.mu.Unlock()
				return
			}
			if source != "" {
				a.ocrText = CleanOcrText(source)
				a.source = a.ocrText
			}
			if target != "" {
				a.target = flattenTranslation(target)
			}
			win := a.win
			a.mu.Unlock()
			if win != nil {
				win.Update(func() {})
			}
		})
	if err != nil {
		message := friendlyAIError(err)
		if ctx.Err() != nil {
			message = "AI 直接看图已取消或超时"
		}
		a.finishScreenshotVisionError(generation, fallbackText, message)
		return
	}
	source, target := parseVisionOutput(out, fallbackText)
	if target == "" {
		a.finishScreenshotVisionError(generation, fallbackText, "模型没有返回译文")
		return
	}
	a.mu.Lock()
	if generation != a.ocrGeneration || !a.isScreenshotMode {
		a.mu.Unlock()
		return
	}
	a.ocrBusy = false
	a.ocrCancel = nil
	a.ocrText = CleanOcrText(source)
	a.source = a.ocrText
	a.target = target
	a.ocrError = ""
	a.engine = "AI 精翻"
	a.elapsed = time.Since(started)
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}
	a.fitHeight()
}

func (a *app) finishScreenshotVisionError(generation requestToken, fallbackText, message string) {
	a.mu.Lock()
	if generation != a.ocrGeneration || !a.isScreenshotMode {
		a.mu.Unlock()
		return
	}
	a.ocrBusy = false
	a.ocrCancel = nil
	if fallbackText != "" {
		a.ocrText = fallbackText
		a.source = fallbackText
		a.ocrError = "AI 直接看图失败：" + message + "；已保留本地识别结果"
		win := a.win
		a.mu.Unlock()
		if win != nil {
			win.Update(func() {})
		}
		a.translateKeepingLines(fallbackText)
		a.fitHeight()
		return
	}
	a.ocrText = ""
	a.source = ""
	a.target = ""
	a.ocrError = "AI 直接看图失败：" + message
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() {})
	}
	a.fitHeight()
}

func (a *app) logVisionFallback(err error) {
	log.Printf("fastDictionary: AI vision failed, using local OCR: %v", err)
}
