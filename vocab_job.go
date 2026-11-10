package main

import "sync"

// 导入 / 导出 CSV / 导出 Anki / 智能补全这四个动作都会读写同一个词库单例，
// 并写同一条反馈消息。单一作业槽位把并发执行收口成互斥会话：连点时第二次
// 不再启动，避免并发写盘、多个文件对话框和 toast 互相覆盖。
var (
	vocabJobMu      sync.Mutex
	vocabJobRunning bool
)

// tryBeginVocabJob 尝试占用作业槽位。已有任务在跑时给出可见提示并返回 false。
// 两把锁从不同时持有，避免与 go func 内部的 a.mu 形成锁顺序反转。
func (a *app) tryBeginVocabJob() bool {
	vocabJobMu.Lock()
	if vocabJobRunning {
		vocabJobMu.Unlock()
		a.mu.Lock()
		a.actionSavedMsg = "有生词任务正在进行，请稍候…"
		a.mu.Unlock()
		if a.cfgWin != nil {
			a.cfgWin.Update(func() {})
		}
		return false
	}
	vocabJobRunning = true
	vocabJobMu.Unlock()
	return true
}

// endVocabJob 释放作业槽位；每个成功开始的动作都用 defer 调用一次。
func endVocabJob() {
	vocabJobMu.Lock()
	vocabJobRunning = false
	vocabJobMu.Unlock()
}
