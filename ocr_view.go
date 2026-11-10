package main

// ocrViewPhase 是截图识别区的显式状态。
//
// 现状由 renderScreenshotSource 用 ocrBusy、ocrError 与 source/ocrText
// 三个裸判断拼出来（docs/STATE_MACHINES.md §4.3 A4），属于隐式状态机。
// 阶段 D 建立这个只读视图：它从 snapshot 即时派生，不写入任何字段，
// 旧字段仍是唯一事实来源，因此行为不会改变。
type ocrViewPhase uint8

const (
	// ocrViewIdle：没有识别文本也没有错误，识别区保持空白。
	ocrViewIdle ocrViewPhase = iota
	// ocrViewRecognizing：正在识别，只显示 spinner 与取消按钮。
	ocrViewRecognizing
	// ocrViewError：识别失败或已取消，显示错误提示，并仍显示编辑区供改字重试。
	ocrViewError
	// ocrViewEditing：已有识别文本或原文，显示编辑区。
	ocrViewEditing
)

func (p ocrViewPhase) String() string {
	switch p {
	case ocrViewIdle:
		return "idle"
	case ocrViewRecognizing:
		return "recognizing"
	case ocrViewError:
		return "error"
	case ocrViewEditing:
		return "editing"
	default:
		return "unknown"
	}
}

// classifyOCRView 把识别区的隐式状态读成显式枚举。
//
// 判定顺序与 renderScreenshotSource 一致：先 busy，再错误，再是否有可编辑文本。
// 纯函数：只读取 snapshot，不写 app 字段，不持锁，也不触发副作用。
func classifyOCRView(s snapshot) ocrViewPhase {
	switch {
	case s.ocrBusy:
		return ocrViewRecognizing
	case s.ocrError != "":
		return ocrViewError
	case s.source != "" || s.ocrText != "":
		return ocrViewEditing
	default:
		return ocrViewIdle
	}
}

// ocrShowErrorBox 报告错误提示框是否显示。
//
// 只有错误态显示；识别中由 busy 分支的 return 先行退出，不会到达这里。
func ocrShowErrorBox(s snapshot) bool {
	return classifyOCRView(s) == ocrViewError
}

// ocrShowEditor 报告「识别原文」编辑区是否显示。
//
// 错误态也显示：识别失败时用户仍可直接改字再翻译，这是原有叠加渲染的语义。
func ocrShowEditor(s snapshot) bool {
	p := classifyOCRView(s)
	return p == ocrViewError || p == ocrViewEditing
}
