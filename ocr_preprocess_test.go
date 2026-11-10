package main

import (
	"image"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrepareOCRImageReportsOversizeDegradation 钉住「超大图降级」这条路径
// 会被显式记录下来：调用方需要知道交出去的是原图，才能在识别不到文字时
// 给用户一句可行动的提示，而不是只说「没有识别到文字」。
func TestPrepareOCRImageReportsOversizeDegradation(t *testing.T) {
	dir := t.TempDir()
	huge := image.NewRGBA(image.Rect(0, 0, maxOCRSide, 10))
	in := filepath.Join(dir, "snip.png")
	writeTestPNG(t, in, huge)

	res, err := prepareOCRImageDetailed(in)
	if err != nil {
		t.Fatalf("prepareOCRImageDetailed: %v", err)
	}
	if !res.Degraded {
		t.Fatal("超大图未能生成放大图，但没有标记为降级")
	}
	if res.Path != in {
		t.Fatalf("降级时应交原图，实际是 %s", res.Path)
	}
	if !strings.Contains(res.Reason, "原尺寸") {
		t.Fatalf("降级原因 = %q，缺少可解释的说明", res.Reason)
	}
}

// TestPrepareOCRImageReportsScaledSuccess 对照：正常放大不应被标记为降级。
func TestPrepareOCRImageReportsScaledSuccess(t *testing.T) {
	dir := t.TempDir()
	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	in := filepath.Join(dir, "snip.png")
	writeTestPNG(t, in, src)

	res, err := prepareOCRImageDetailed(in)
	if err != nil {
		t.Fatalf("prepareOCRImageDetailed: %v", err)
	}
	if res.Degraded {
		t.Fatal("成功生成放大图却被标记为降级")
	}
	if res.Path == in {
		t.Fatal("没有生成放大图")
	}
	defer res.Cleanup()
}

// TestOCREmptyTextMessageExplainsOversize 检查空结果提示按降级与否区分：
// 超大图要告诉用户可以缩小截图范围。
func TestOCREmptyTextMessageExplainsOversize(t *testing.T) {
	degraded := ocrEmptyTextMessage(true)
	if !strings.Contains(degraded, "图片较大") || !strings.Contains(degraded, "缩小截图范围") {
		t.Fatalf("降级提示 = %q，缺少可行动的说明", degraded)
	}
	plain := ocrEmptyTextMessage(false)
	if plain != "没有识别到文字，请重新框选清晰文本" {
		t.Fatalf("普通提示 = %q，不应改变原有文案", plain)
	}
}
