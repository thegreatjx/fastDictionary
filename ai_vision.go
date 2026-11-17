package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const (
	visionSourceMarker = "[SOURCE]"
	visionTargetMarker = "[TARGET]"
	visionMaxSide      = 1600
	visionJPEGQuality  = 85
)

func compressedImageFileDataURL(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("截图路径为空")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("读取截图失败: %w", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("解码截图失败: %w", err)
	}
	flat := flattenRGBA(img)
	w, h := flat.Bounds().Dx(), flat.Bounds().Dy()
	if long := maxInt(w, h); long > visionMaxSide {
		scale := float64(visionMaxSide) / float64(long)
		nw := maxInt(1, int(float64(w)*scale))
		nh := maxInt(1, int(float64(h)*scale))
		resized := image.NewRGBA(image.Rect(0, 0, nw, nh))
		xdraw.CatmullRom.Scale(resized, resized.Bounds(), flat, flat.Bounds(), xdraw.Src, nil)
		flat = resized
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: visionJPEGQuality}); err != nil {
		return "", fmt.Errorf("压缩截图失败: %w", err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func visionSystemPrompt(targetLang string) string {
	return "You are a screenshot OCR and translation engine. The screenshot is the source of truth. " +
		"Correct OCR mistakes, keep the natural reading order and paragraph breaks, then translate into " + langPromptName(targetLang) + ". " +
		"Output exactly this format and nothing else, with no JSON and no Markdown fence:\n" +
		visionSourceMarker + "\n整理好阅读顺序的原文\n" +
		visionTargetMarker + "\n流畅译文\n" +
		"Keep terminology accurate and keep the original tone. If the OCR hint is wrong, ignore it."
}

func visionUserPrompt(ocrText string) string {
	if strings.TrimSpace(ocrText) == "" {
		return "请识别截图中的文字，整理阅读顺序，再按系统要求的分隔符格式输出原文和译文。"
	}
	return "OCR 文本（可能有识别错误，仅供提示）：\n" + ocrText +
		"\n\n请以截图为准，纠正 OCR 错误，并按 [SOURCE]/[TARGET] 分隔符格式输出。"
}

type visionStreamParser struct {
	buf strings.Builder
}

func (p *visionStreamParser) Push(delta string) (source, target string) {
	p.buf.WriteString(delta)
	text := p.buf.String()
	upper := strings.ToUpper(text)
	si := strings.Index(upper, visionSourceMarker)
	ti := strings.Index(upper, visionTargetMarker)
	if ti >= 0 {
		target = trimVisionSection(text[ti+len(visionTargetMarker):])
		if si >= 0 && si < ti {
			source = trimVisionSection(text[si+len(visionSourceMarker) : ti])
		}
		return source, target
	}
	if si >= 0 {
		source = trimVisionSection(text[si+len(visionSourceMarker):])
		source = trimVisionSection(stripPartialVisionTargetMarker(source))
	}
	return source, ""
}

func stripPartialVisionTargetMarker(s string) string {
	upper := strings.ToUpper(s)
	for n := len(visionTargetMarker) - 1; n > 0; n-- {
		if strings.HasSuffix(upper, visionTargetMarker[:n]) {
			return s[:len(s)-n]
		}
	}
	return s
}

func parseVisionOutput(out, fallbackSource string) (source, target string) {
	source, target = rawVisionSections(out, fallbackSource)
	return CleanOcrText(source), flattenTranslation(target)
}

func parseVisionOutputForLayout(out, fallbackSource, mode string) (source, target string) {
	source, target = rawVisionSections(out, fallbackSource)
	return cleanSourceForLayout(source, mode, true), formatTranslationForLayout(target, mode, true)
}

func rawVisionSections(out, fallbackSource string) (source, target string) {
	var parser visionStreamParser
	source, target = parser.Push(out)
	if target == "" {
		target = trimVisionSection(out)
		source = fallbackSource
	}
	if source == "" {
		source = fallbackSource
	}
	return source, target
}

func trimVisionSection(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		} else {
			s = strings.TrimPrefix(s, "```")
		}
	}
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
