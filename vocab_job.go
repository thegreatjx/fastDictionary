package main

import "sync"

var (
	vocabJobMu      sync.Mutex
	vocabJobRunning bool
	vocabJobLabel   string
)

// 词库任务单槽位，防止并发写同一个单例。
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
	vocabJobLabel = ""
	vocabJobMu.Unlock()
	return true
}

func endVocabJob() {
	vocabJobMu.Lock()
	vocabJobRunning = false
	vocabJobLabel = ""
	vocabJobMu.Unlock()
}

func setVocabJobLabel(label string) {
	vocabJobMu.Lock()
	vocabJobLabel = label
	vocabJobMu.Unlock()
}

func vocabJobStatus() (bool, string) {
	vocabJobMu.Lock()
	defer vocabJobMu.Unlock()
	return vocabJobRunning, vocabJobLabel
}
