package main

// aiPassPhase 是一次 AI 结果在结果区的显式状态。
//
// 现状由 popup_view.go 的 renderAIResultState 用 aiBusy、aiText 和
// looksLikeStructuredPayload 三个判断拼出来，属于隐式状态机
// （docs/STATE_MACHINES.md §4.2 U1、§4.4 X2）。阶段 A 先建立这个只读视图：
// 它从 snapshot 即时派生，不写入任何字段，旧字段仍是唯一事实来源，
// 因此行为不会改变。阶段 B 才把状态收敛成可迁移的 aiPass 对象。
type aiPassPhase uint8

const (
	// aiPassIdle：没有 AI 结果，结果区不渲染任何内容。
	aiPassIdle aiPassPhase = iota
	// aiPassStreaming：结果还在生成，结果区显示 spinner 与一句说明。
	aiPassStreaming
	// aiPassProse：模型返回散文，结果区按 markdown 展示。
	aiPassProse
	// aiPassParseFailed：模型返回了结构化文本却没解析成卡片，结果区展示提示。
	aiPassParseFailed
	// aiPassStructured：结构化卡片已就绪，由 popupView 的 mode 分支直接渲染，
	// 不经过 renderAIResultState。classifyAIResult 不产生这个值；卡片就绪
	// 由更高一层的 classifyAIView 判定并产生（批次 2b 启用）。
	aiPassStructured
)

func (p aiPassPhase) String() string {
	switch p {
	case aiPassIdle:
		return "idle"
	case aiPassStreaming:
		return "streaming"
	case aiPassProse:
		return "prose"
	case aiPassParseFailed:
		return "parse_failed"
	case aiPassStructured:
		return "structured"
	default:
		return "unknown"
	}
}

// classifyAIResult 把结果区的隐式状态读成显式枚举。
//
// 判定顺序必须与 popup_view.go 的 renderAIResultState 一致：先 busy，
// 再空文本，再是否像结构化 payload。它只分类「没有结构化卡片」的结果区；
// popupView 在 grammarResult / dictResult / writingResult 非 nil 时直接渲染
// 卡片，不会走到这里。
//
// 纯函数：只读取 snapshot，不写 app 字段，不持有锁，也不触发副作用。
//
// 分层：classifyAIView 先判断卡片是否就绪，未就绪时才回落到本函数，
// 因此 popupView 与 renderAIResultState 共用同一套状态名字。
func classifyAIResult(s snapshot) aiPassPhase {
	switch {
	case s.aiBusy:
		return aiPassStreaming
	case s.aiText == "":
		return aiPassIdle
	case !looksLikeStructuredPayload(s.aiText):
		return aiPassProse
	default:
		return aiPassParseFailed
	}
}

// cardReadyForMode 报告当前 mode 的结构化卡片是否就绪。
//
// 每个 mode 只认自己的结果字段：语法模式不会因为词典卡片存在就改画词典卡。
// popupView 原先在三个 case 里各写一个 != nil 判断，这里收成一个带名字的
// 查询，判断结果逐字不变。
//
// 批次 2c 起，popupContentHeight 与 handleQuickSave 也用这个查询。调用点
// 常位于 switch s.mode 的 case 内，那里它等价于对应的 != nil 判断；集中
// 映射的价值是让渲染、窗口高度、快捷保存共用同一份判定，改一处即可。
func cardReadyForMode(s snapshot) bool {
	switch s.mode {
	case modeGrammar:
		return s.grammarResult != nil
	case modeDictionary:
		return s.dictResult != nil
	case modeWriting:
		return s.writingResult != nil
	default:
		return false
	}
}

// classifyAIView 把 popupView 三个 AI mode 分支的隐式三态读成显式枚举：
// 卡片 / 进行中的 AI 结果 / 空态。
//
// 判定顺序与原先的 if / else if / else 完全一致：先看卡片，再看结果区，
// 最后才是空态。它只读取 snapshot，不写字段、不持锁、不触发副作用。
func classifyAIView(s snapshot) aiPassPhase {
	if cardReadyForMode(s) {
		return aiPassStructured
	}
	return classifyAIResult(s)
}
