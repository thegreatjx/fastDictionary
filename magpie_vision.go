package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// imageFileDataURL reads a screenshot and turns it into the data URL accepted
// by the OpenAI-compatible vision message format. The screenshot is kept on
// disk for OCR retry and preview, so this does not add another image cache.
func imageFileDataURL(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("截图路径为空")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取截图失败: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("截图文件为空")
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return "", fmt.Errorf("截图格式无法识别: %s", mime)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func screenshotRefineSystemPrompt(targetLang string) string {
	return "You are a translator. The user sends a screenshot of text and its OCR text. " +
		"The OCR text may contain recognition mistakes. Treat the screenshot as the source of truth: " +
		"correct OCR errors first, then translate into " + langPromptName(targetLang) +
		". Keep terminology accurate, keep the original's tone and structure, and output only the translation with no explanation."
}

func screenshotRefineUserPrompt(ocrText string) string {
	if strings.TrimSpace(ocrText) == "" {
		return "请识别截图中的文字并翻译。"
	}
	return "OCR 文本（可能有识别错误）：\n" + ocrText + "\n\n请以截图为准，先纠正 OCR 错误，再翻译。"
}
