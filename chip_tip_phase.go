package main

import "time"

// chipTipPhase 命名 hover 标签在一次指针事件里可处的阶段。它只是对
// chipTipState 的读时派生视图，不驱动迁移；旧字段仍是唯一事实来源。
// 完整迁移表跨鼠标钩子 goroutine、定时器与主线程三个执行上下文，
// 不在本阶段改动，见 docs/STATE_MACHINES_FIX_PLAYBOOK.md §5 批次 6d。
type chipTipPhase uint8

const (
	tipHidden  chipTipPhase = iota // 指针不在按钮上，标签已收起
	tipWaiting                     // 悬停未到 chipTipDelay，继续等
	tipWarm                        // 刚收起过标签，扫过相邻按钮可立即接续
	tipReady                       // 悬停已达到 chipTipDelay，可以显示
	tipShown                       // 标签正在显示
)

func (p chipTipPhase) String() string {
	switch p {
	case tipHidden:
		return "hidden"
	case tipWaiting:
		return "waiting"
	case tipWarm:
		return "warm"
	case tipReady:
		return "ready"
	case tipShown:
		return "shown"
	default:
		return "unknown"
	}
}

// chipTipIsWarm 报告 hiddenAt 是否落在 chipTipWarm 的接续窗口内。
// 与 onChipPointer 原内联表达式逐字等价。
func chipTipIsWarm(hiddenAt, now time.Time) bool {
	return !hiddenAt.IsZero() && now.Sub(hiddenAt) <= chipTipWarm
}

// chipTipIsDue 报告悬停是否已达到显示延时且标签尚未显示。
// 与 onChipPointer 原内联表达式逐字等价。
func chipTipIsDue(shown bool, since, now time.Time) bool {
	return !shown && now.Sub(since) >= chipTipDelay
}

// classifyChipTipPhase 把 hover 状态读成具名阶段，供测试与诊断使用。
// 它是纯读时派生视图：迁移仍由 onChipPointer 的两个谓词分别驱动，
// 因为 warm 决定是否重置 since、due 决定是否显示，二者不能合成一次分支。
func classifyChipTipPhase(inside, shown bool, hiddenAt, since, now time.Time) chipTipPhase {
	switch {
	case !inside:
		return tipHidden
	case shown:
		return tipShown
	case chipTipIsWarm(hiddenAt, now):
		return tipWarm
	case chipTipIsDue(shown, since, now):
		return tipReady
	default:
		return tipWaiting
	}
}
