package main

// 截图识别的三种模式。模型是否支持图片由服务端决定；这里只决定发不发截图。
const (
	ocrModeLocal      = "local"
	ocrModeVision     = "vision"
	ocrModeLocalFirst = "local_first"
)

func normalizeOCRMode(mode string) string {
	switch mode {
	case ocrModeVision, ocrModeLocalFirst:
		return mode
	default:
		return ocrModeLocal
	}
}

func ocrModeValues() []string {
	return []string{ocrModeLocal, ocrModeVision, ocrModeLocalFirst}
}

func ocrModeLabels() []string {
	return []string{"本地", "AI 直接看图", "本地优先"}
}
