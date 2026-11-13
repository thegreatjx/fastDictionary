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

// AI 看图把截图交给用户自选模型。模型是否收图片由服务端回答；客户端不按模型名
// 或旧版「图片输入」开关预先禁用，避免把用户自选模型锁死。
const (
	visionSourceMarker = "[SOURCE]"
	visionTargetMarker = "[TARGET]"
	visionMaxSide      = 1600
	visionJPEGQuality  = 85
)

// compressedImageFileDataURL 缩小大图、把透明底铺白，再编码为 JPEG。截图常是
// PNG，直接 base64 原图会把 2K/4K 图片和透明像素一起发给模型，既慢又贵。
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

// Push 返回当前已经能确定的原文和译文。流式片段可能把标记切开，缓冲到完整标记
// 出现后再显示；没有 [TARGET] 时不把半截标记当成译文。
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

// stripPartialVisionTargetMarker 删掉流式片段末尾尚未完整的 [TARGET] 前缀，
// 避免把 "[TAR" 临时显示成原文。
func stripPartialVisionTargetMarker(s string) string {
	upper := strings.ToUpper(s)
	for n := len(visionTargetMarker) - 1; n > 0; n-- {
		if strings.HasSuffix(upper, visionTargetMarker[:n]) {
			return s[:len(s)-n]
		}
	}
	return s
}

// parseVisionOutput 统一处理带分隔符和纯译文两种返回。纯译文流保留用户原有的
// 流式体验，原文继续用本地 OCR 或用户已编辑的文本。
func parseVisionOutput(out, fallbackSource string) (source, target string) {
	var parser visionStreamParser
	source, target = parser.Push(out)
	if target == "" {
		target = trimVisionSection(out)
		source = fallbackSource
	}
	if source == "" {
		source = fallbackSource
	}
	return CleanOcrText(source), flattenTranslation(target)
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
