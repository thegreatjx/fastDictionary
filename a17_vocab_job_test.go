package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestVocabJobsUseSingleGuard(t *testing.T) {
	vocab := readSourceFile(t, "settings_vocab.go")
	if got := countOccurrences(vocab, "a.tryBeginVocabJob()"); got != 4 {
		t.Fatalf("四个生词动作都应由同一个作业守卫收口，当前只有 %d 处调用 a.tryBeginVocabJob()", got)
	}
	if got := countOccurrences(vocab, "defer endVocabJob()"); got != 4 {
		t.Fatalf("四个生词动作都应在退出时释放作业槽位，当前只有 %d 处 defer endVocabJob()", got)
	}
	guard := readSourceFile(t, "vocab_job.go")
	if !containsText(guard, "func (a *app) tryBeginVocabJob() bool") {
		t.Fatal("缺少显式的作业守卫方法 tryBeginVocabJob")
	}
	if !containsText(guard, "func endVocabJob()") {
		t.Fatal("缺少显式的作业释放方法 endVocabJob")
	}
}

func resetVocabJobForTest(t *testing.T) {
	t.Helper()
	vocabJobMu.Lock()
	prev := vocabJobRunning
	vocabJobRunning = false
	vocabJobMu.Unlock()
	t.Cleanup(func() {
		vocabJobMu.Lock()
		vocabJobRunning = prev
		vocabJobMu.Unlock()
	})
}

func TestVocabJobGuardTransitions(t *testing.T) {
	resetVocabJobForTest(t)
	a := newTestApp()

	if !a.tryBeginVocabJob() {
		t.Fatal("空闲时第一次 begin 应成功")
	}
	if a.tryBeginVocabJob() {
		t.Fatal("已有任务在跑时第二次 begin 应被拒绝")
	}
	endVocabJob()
	if !a.tryBeginVocabJob() {
		t.Fatal("结束后应能再次 begin")
	}
	endVocabJob()
	endVocabJob()
	if !a.tryBeginVocabJob() {
		t.Fatal("重复释放后仍应能 begin")
	}
	endVocabJob()
}

func TestVocabJobGuardConcurrentBegin(t *testing.T) {
	resetVocabJobForTest(t)
	a := newTestApp()

	const n = 24
	var wg sync.WaitGroup
	var won int32
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if a.tryBeginVocabJob() {
				atomic.AddInt32(&won, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := atomic.LoadInt32(&won); got != 1 {
		t.Fatalf("并发 begin 应恰好一个成功，实际 %d 个", got)
	}
	endVocabJob()
}
