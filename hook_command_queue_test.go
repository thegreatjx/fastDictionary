package main

import (
	"strings"
	"testing"
)

func hookFuncBody(t *testing.T, src, signature string) string {
	t.Helper()
	start := strings.Index(src, signature)
	if start < 0 {
		t.Fatalf("找不到函数 %s", signature)
	}
	rel := strings.Index(src[start:], "\n}\n")
	if rel < 0 {
		t.Fatalf("函数 %s 没有结束", signature)
	}
	return src[start : start+rel]
}

// TestHookCommandQueueIsBuffered is the regression: the old single-slot
// hookCommandFn let a second post overwrite the first before the hook thread
// drained it, so a fast mode switch (or quitting right after one) could drop
// installHooks/removeHooks for good.
func TestHookCommandQueueIsBuffered(t *testing.T) {
	src := readSourceFile(t, "hook.go")
	if !containsText(src, "make(chan func(),") {
		t.Fatal("钩子命令仍是单槽：第二次投递会覆盖还没执行的第一条")
	}
	if containsText(src, "hookCommandFn") {
		t.Fatal("hookCommandFn 单槽仍然存在，命令会被覆盖丢失")
	}
	post := hookFuncBody(t, src, "func postHookCommand(")
	if !containsText(post, "hookCommands <- fn") {
		t.Fatal("postHookCommand 没有把命令放进队列")
	}
	run := hookFuncBody(t, src, "func runHookCommand(")
	if !containsText(run, "case fn := <-hookCommands:") {
		t.Fatal("runHookCommand 没有消费队列")
	}
}

func TestHookCommandQueueDeliversEveryCommand(t *testing.T) {
	drainHookCommands()
	var order []string
	for _, name := range []string{"first", "second", "third"} {
		label := name
		hookCommands <- func() { order = append(order, label) }
	}
	runHookCommand()
	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Fatalf("执行顺序 = %v，want [first second third]：每条命令都必须执行一次", order)
	}
}

func TestHookCommandQueueSkipsNilCommands(t *testing.T) {
	drainHookCommands()
	ran := false
	hookCommands <- nil
	hookCommands <- func() { ran = true }
	runHookCommand()
	if !ran {
		t.Fatal("nil 命令之后的排队的命令也必须执行")
	}
}

func drainHookCommands() {
	for {
		select {
		case <-hookCommands:
		default:
			return
		}
	}
}
