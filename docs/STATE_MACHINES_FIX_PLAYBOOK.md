# 不动功能修复隐式状态机：操作手册

配套 [STATE_MACHINES.md](STATE_MACHINES.md)。那篇回答「有哪些、在哪里、风险多大」，这篇只回答一件事：**具体怎么改**。

> 进度：**阶段 A（批次 0–3）已完成**，**阶段 B 批次 4a（前置护栏）、4b（U1 精翻独立代次）、4c（U6 页签加载态）、4d（U2 旧翻译写回）、4e（A17 假成功提示）、4f（A4 新截图让在飞翻译失效）已完成，阶段 B 收尾；阶段 C 批次 5a（A8 配置损坏不覆盖用户文件）、5b（A18 词库损坏保留可恢复副本）、5c（C5 isQuitting 显式化）、5d（U8 两入口置顶复位收口）、5e（C1 钩子卸载复位双击候选）、5f（C10 截图框选会话边界收口）、5g（A17 补全按钮假成功）已完成，阶段 C 到此收口**。批次 3 是全库读取点清点，结果见 §10：四类 UI 状态判断全部收进 `ai_pass.go` 的三个纯函数，剩余旧字段读取逐条归类且有保留理由，写入点一个没动。批次 4a 把阶段 B 要动的三个入口（见 §10.5）用 characterization test 钉住现状；4b 先写复现测试，再给精翻拆出独立代次，修掉「点 AI 精翻掐断卡片流」；4c 让「正在加载」的页签不再被当作完成态缓存，切回来会重跑当前模式；4d 让切页签 / 开新卡片时在飞的机器翻译代次失效，旧译文不再写进新模式视图；4e 把磁盘写入失败作为 `error` 沿导入链路上送，设置页不再在保存失败时显示「成功导入 N 条 ✓」；4f 实地核查后发现审计写的「取消后旧 OCR 结果写回」已被 `ocrGeneration` + `ctx` 挡住，确凿缺陷是新截图清空原文却不失效 `translateGen`，于是在 `recognizeScreenshot` 锁内补一次自增，并加测试接缝让取消路径可单测。**阶段 B 到此收尾**。**阶段 C 批次 5a（A8 配置损坏不覆盖用户文件）已完成**：实地核查发现审计写的「解析失败后磁盘文件未被覆盖」不成立，实际是 `newStore` 把解析失败当首次运行、用 `DefaultConfig()` 覆盖用户损坏的 config.json，设置永久丢失；改用 `os.Stat` 区分「文件不存在」（写默认）与「文件存在但损坏 / 不可读」（保留文件、仅用内存默认并打可执行指引），首次运行不受影响。**阶段 C 批次 5b（A18 词库损坏保留可恢复副本）已完成**：实地核查确认 `save()` 的 `.bak` 是滚动单副本，第一次加词就被覆盖，损坏原文第二次写盘后永久丢失且 UI 无提示；`load()` 解析失败时另存一份 `path+".corrupt"` 原始副本并打可执行指引，不改「空库启动」与「加词写盘」行为。**阶段 C 批次 5d（U8 两入口置顶复位收口）已完成**：openPopupWithText 有 screenshotPinned 兜底、openPopupAction 没有，从 chip / 托盘打开新卡片会继承截图翻译的置顶且失焦不关；把复位规则收进纯方法 `resetScreenshotPinLocked()`，两入口都调用，一处行为逐字不变、另一处补上缺失的复位。**阶段 C 批次 5e（C1 钩子卸载复位双击候选）已完成**：审计写的「重装不清 lastClickTime/lastClickPos」经核查落在 `removeHooks()` 这个普通 Go 函数里（非 Win32 回调），已在 `hookMu` 锁内用纯方法 `resetClickTrackingLocked()` 收口会话边界复位；双击判定一字不动、零行为变化，`hook.go` 是 diff=0 的干净文件。源码契约测试 `TestRemoveHooksResetsClickTracking` 先红后绿、`TestResetClickTrackingLocked` 表驱动覆盖残留 / 幂等。**阶段 C 批次 5f（C10 截图框选会话边界收口）已完成**：审计把 C10 和 U10 / C1 一起归为「需真实 Win32 消息泵，无法单测」，但逐行读 [snip.go](<../snip.go>) 后发现会话边界判定落在普通 Go 逻辑里（`StartNativeSnip` 的 goroutine 内、消息循环之前），不需要驱动窗口。新增 `beginSnipSessionLocked` / `endSnipSessionLocked` 两个纯方法，让 `StartNativeSnip` 在 `snipMu` 锁内原子完成「检查 + 占位」：连按截图不再弹第二个互相覆盖的 overlay，`hwnd==0` 等早退也不再残留全局单例；消息循环正常结束的 `:504-507` 保留不动，defer 兜底为 no-op。`snip.go` 开工时 diff=0。源码契约测试 `TestStartNativeSnipGuardsSingleton` 先红后绿、`TestBeginSnipSessionLocked` 走一遍会话生命周期。用户已明确「做到成熟工具软件的程度就可以，不要无止境一直做」，故阶段 C 收口：X1（三份字段清单合并）、A17 残余的四个 `go func()` 并发守卫、A4 的 `ocrBusy`/`ocrError` 结构性显式化、U10 的 `ChipTipPhase` 枚举化，都是无用户可见缺陷的结构性重构，刻意不做，理由见 §10.5。X1 再次推迟、批次 2a / 3 / 4b / 4c / 4e / 4f / 5a / 5b / 5c / 5d / 5e / 5f / 5g 范围修订与 4d 实地核查的理由见 §5；§6 的前置条件已用「差分法」绕开。用户随后又明确「那就做吧开工」，重新授权 A17 并发守卫 → X1 → A4 → U10 四项结构性收尾，故新开 **阶段 D**：批次 6a（A17 四个 go func 并发守卫）已完成 —— 原先四个长任务各起裸 goroutine、连点会并发写同一词库单例与同一条 toast 并弹出多个文件对话框；现用 vocab_job.go 的单一作业槽位 tryBeginVocabJob / endVocabJob 收口，四个 go func 各加守卫 + defer，其余逻辑一字不动，忙时给「有生词任务正在进行，请稍候…」可见提示。阶段 D 起工作区已在 d2fe230 提交后清空，不再需要「证明没碰用户改动」的差分，git diff 全部是本批改动。**阶段 D 批次 6b（X1 三套字段清单合并·第一刀）已完成**：实地读 `popup_tabs.go` 发现 `clearTabLocked()` 与零值 `restoreTabLocked` 是同一份字段清单复制两遍，用 `reflect.DeepEqual` 证明等价后让 clear 委托 restore，三份手工清单减到两份，并用反射契约 `TestTabFieldListsAreSynced` 钉住 save / restore / snap 三处字段同步；零行为变化，完整合一因 `snapshot` 含渲染期派生字段、语义不同而留后续。详见 §5 施工卡与 §7 验证记录。**阶段 D 批次 6c（A4 OCR 视图显式化）已完成**：`renderScreenshotSource` 的三块是叠加渲染而非互斥三态，只把三个裸 `if` 收成 `classifyOCRView` / `ocrShowErrorBox` / `ocrShowEditor` 三个具名谓词，控制流一字未动，零行为变化。**阶段 D 批次 6d（U10 Hover Tip 时间窗口谓词化）已完成**：把 `onChipPointer` 里的 warm / due 两个内联时间表达式收成 `chipTipIsWarm` / `chipTipIsDue` 两个具名谓词并加 `chipTipPhase` 阶段枚举，零行为变化。阶段 D 四项（6a–6d）到此全部完成。

## 0. 先分清两件事

用户说的「不动功能」，准确含义是**不破坏现有功能**。据此，工作必须分成两个阶段：

| 阶段 | 目标 | 行为变化 | 风险 |
| --- | --- | --- | --- |
| 阶段 A（批次 0–3） | 把散落的状态判断收进显式枚举（读时派生），并清点确认无遗漏 | **零**。同一输入序列下，UI 输出、持久化结果、网络/剪贴板/窗口副作用完全一致 | 极低，逐批可回滚 |
| 阶段 B（批次 4a–4f，已完成） | 4a 先用护栏钉住现状；4b 已修 U1、4c 已修 U6、4d 已修 U2、4e 已修 A17、4f 已修 A4；每条都按实地核查后的确凿缺陷收敛范围 | 4a 无；4b–4f **有**，但属于修 bug，不是破坏功能 | 中。每条先写复现测试再改 |
| 阶段 C（批次 5a–5g，已收口） | 5a 已修 A8、5b 已修 A18、5c 已修 C5、5d 已修 U8、5e 已修 C1、5f 已修 C10、5g 已修 A17 补全假成功；当时按用户「做到成熟即可」收口、剩余判为结构性重构刻意不做（见 §10.5），后由阶段 D 重新授权 | **有**，但属于修 bug / 显式化，不是破坏功能 | 中。每条先写复现测试再改 |
| 阶段 D（批次 6a–，用户重新授权） | 用户先要求「做到成熟即可」使阶段 C 收口，随后又明确「那就做吧开工」，重新授权 A17 并发守卫 → X1 → A4 → U10 四项；6a 已修 A17 连点并发、6b 把 clear 委托 restore 并补反射同步契约、6c 把 A4 的 OCR 三态收成具名谓词、6d 把 U10 的两个时间窗口收成具名谓词并加阶段枚举 | 6a 有（修 bug）；6b、6c、6d 零行为变化（显式化）；后续以显式化为主 | 中。每条先写复现测试再改；工作区已干净，git diff 全属本批 |

**把两者混在一次提交里，是「不动功能」失败的主要原因。** 阶段 A 只改**判断形式**（把散落的 `if` 收成显式枚举分支），不改**判断结果**：同一输入必须落到同一分支、画同一界面。这一点由改前/改后 `git diff` 差分 + 渲染基线表逐字不变共同证明（批次 1b 已按此执行）。阶段 B 必须单独提交、单独验证。

### 行为等价的判定标准

对同一输入序列，下列四项完全一致即视为等价：

1. 渲染出的文本与控件状态（含 Spinner、错误条、展开/折叠）；
2. 写入磁盘的配置与词库内容；
3. 触发的副作用次数与顺序（HTTP 请求、剪贴板写入、`win.Show/Hide/Update`、`SetAlwaysOnTop`）；
4. 异步回调到达时被采纳还是丢弃（代次判定结果）。

判定手段：**characterization test（录现状） + 全套检查 + 手工走查**，三者缺一不可。Windows 原生 UI 无法自动点击，视图切换必须手工看。

---

## 1. 唯一可行的机制：读时派生 + 逐点切读（strangler）

分批重写状态机一定会碰到正在运行的异步 goroutine；引入第三方状态机库要新增依赖，且 mygo 的消息循环需要定制适配。可行的是**三步渐进**，每一步的写入方都保持不变，直到新状态被证明与旧字段完全相等。

> **修订记录（批次 1a）**：初版手册写的是「写时镜像」（在每个写点同步写 `app.aiPass`）。实地核查后发现 `aiText`、`aiBusy`、`aiCancel`、`activeActionID`、`aiGen` 共有 **32 个写点**，散在 `main.go`、`ocr.go`、`popup.go`、`popup_actions.go`、`popup_tabs.go` 五个文件，多处还在不同 goroutine 里。逐个挂镜像既容易漏，又恰好复制了审计里 X1「三份状态手工同步」的病根。因此第 1 步改为**读时派生**：新增一个不写任何字段的纯函数，从 `snapshot` 即时算出显式状态。旧字段仍是唯一事实来源，天然不会漏，也无需在这 32 处加代码。

| 步 | 动作 | 谁是真值 | 行为变化 |
| --- | --- | --- | --- |
| 1 派生 | 新增显式状态类型与分类纯函数；**不写任何字段**，所有读写仍走旧字段 | 旧字段 | 零 |
| 2 切读 | 一次只改**一个**读取点，改完立即用渲染基线测试验证 | 逐点过渡 | 零（由基线测试保证） |
| 3 清点 | 全库清点剩余读取点并归类（可切 / 不可切 / 阶段 B 才动），确认无遗漏后阶段 A 收尾 | 旧字段仍是真值 | 零 |

三道硬约束：

- 第 1 步的分类函数**只读 `snapshot`、不写字段、不持锁、不触发副作用**。所有分支继续读旧字段。
- 第 1 步**不顺手修 bug**。已知缺陷（如 `stopAIPassLocked` 不清 `aiText`）必须原样保留，留到批次 4。
- 每批只含一个状态机或一个读取点，批间互不依赖；回滚用精确反向编辑，不用 `git checkout --`（见 §6）。

---

## 2. 第 1 步的代码形态（以 U1 / `aiGen` 为例，基于真实代码）

### 改造前

[popup_actions.go:52-64](<../popup_actions.go#L52-L64>) 的 `stopAIPassLocked` 原文：

```go
func (a *app) stopAIPassLocked() {
	a.aiGen++
	if a.aiCancel != nil {
		a.aiCancel()
		a.aiCancel = nil
	}
	a.aiBusy = false
	a.aiRefining = false
	a.aiRefineErr = ""
}
```

[popup.go:72-78](<../popup.go#L72-L78>) 里对应的旧字段：`aiText`、`aiBusy`、`aiCancel`、`aiGen`。

### 批次 1a 实际落地（新文件 `ai_pass.go`，不碰任何现有文件）

[ai_pass.go](<../ai_pass.go>) 只定义枚举和一个纯函数：

```go
type aiPassPhase uint8

const (
	aiPassIdle aiPassPhase = iota
	aiPassStreaming
	aiPassProse
	aiPassParseFailed
	aiPassStructured // 预留给 popupView 的 mode 分支
)

func (p aiPassPhase) String() string { /* idle / streaming / prose / parse_failed / structured */ }

// 判定顺序必须与 renderAIResultState 一致：先 busy，再空文本，再是否像结构化 payload。
// 只分类「没有结构化卡片」的结果区；纯函数，不写字段、不持锁。
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
```

`aiPassStructured` 不参与 `classifyAIResult`：`popupView` 的三个 mode 分支在
`grammarResult` / `dictResult` / `writingResult` 非 nil 时直接渲染卡片，不经过
`renderAIResultState`，优先级也不同（结构化卡片优先于 `aiBusy`）。

**批次 2b 已启用它**：[ai_pass.go](<../ai_pass.go>) 新增 `cardReadyForMode(s)`
（每个 mode 只认自己的结果字段）和 `classifyAIView(s)`（卡片就绪返回
`aiPassStructured`，否则回落到 `classifyAIResult`）。`popupView` 三个 mode 分支
改成 `switch classifyAIView(s)`：`aiPassStructured` 画卡片、`aiPassIdle` 画空态、
其余三态仍交给 `renderAIResultState`。至此同一套枚举覆盖了卡片、结果区和空态，
`aiPassStructured` 不再是预留值。

### 护栏（新文件 `ai_pass_test.go`）

- `TestClassifyAIResult`：6 组真实输入 → 期望 phase 与 `String()`，锁定枚举规格。
- `TestAIResultStateRenderingBaseline`：把 idle / streaming / prose / parse_failed
  四种状态经 `ui.NewTester(a.popupView, 500, 620)` 渲染出的文本特征逐项录下。
  批次 1b 把 `renderAIResultState` 改成按 phase 分支后，这张表必须逐字不变——
  它就是「零行为变化」的判据。

**如何证明这一步零行为变化**：`git status --short` 显示只新增两个文件，18 个
` M` 文件清单与动手前逐字相同；分类函数不写字段、不参与任何现有调用路径，
生产代码一行未改。

---

## 3. 第 2 步：一次切一个读取点

前提：第 1 步的 `classifyAIResult` 是 `renderAIResultState` 四个分支的读时等价物，且 `TestAIResultStateRenderingBaseline` 已把这四种状态的界面输出逐字录下。于是把整函数收成显式枚举分支，不改变判断结果。**批次 1b 已按此落地**（一次切一个函数，即一个读取点）：

```diff
 func renderAIResultState(c *ui.Context, t *ui.Theme, s snapshot) {
-\tif s.aiBusy {
-\t\t// 画 Spinner
-\t\treturn
-\t}
-\tif s.aiText == "" {
-\t\treturn
-\t}
-\tif !looksLikeStructuredPayload(s.aiText) {
-\t\trenderAiMarkdown(c, s.aiText, false)
-\t\treturn
-\t}
-\t// 画解析失败卡片
+\tswitch classifyAIResult(s) {
+\tcase aiPassIdle, aiPassStructured:
+\t\treturn
+\tcase aiPassStreaming:
+\t\t// 画 Spinner
+\t\treturn
+\tcase aiPassProse:
+\t\trenderAiMarkdown(c, s.aiText, false)
+\t\treturn
+\tcase aiPassParseFailed:
+\t\t// 画解析失败卡片
+\t}
 }
```

`aiPassStructured` 与 `aiPassIdle` 合并 return：`classifyAIResult` 不产生该值，结构化卡片由 `popupView` 的 mode 分支优先渲染，本函数只在无卡片时被调用（见 §2）。

规则：

- 一次提交只切一个读取点，切完立刻跑全套检查并手工走查该视图。
- 切换顺序从**纯渲染函数**开始（`popupView`、`renderAIResultState`、`renderTranslationContent`），最后才动 goroutine 回调里的代次校验。
- 每切一个点，用渲染基线表复核该视图的文本特征逐字不变。旧字段仍是唯一写入方，不存在两个真值并存的窗口。

第 3 步清理时，才允许把 `engine == "AI 精翻"`、`SelectionTrigger == "auto"` / `"hotkey"` 这类字符串哨兵收进 `const`，把 `vocabExpandedID == ""`、`pausedUntil.IsZero()` 这类 nil/零值哨兵收进显式 `bool` 字段。**提前做就是改行为。**

---

## 4. 护栏：characterization test 模板

批次 0 先建护栏，只新增一个测试文件，不碰任何现有文件、不改运行时代码。测试不断言「应该怎样」，只录「现在是怎样」——包括现状里的缺陷，因为阶段 A 必须连缺陷一起镜像。

项目已有 `newTestApp()`（见 [popup_streaming_test.go:14](<../popup_streaming_test.go#L14>)）与 `ui.NewTester`，可直接复用：

```go
// state_mirror_test.go（新增文件）
package main

import "testing"

// 录现状：stopAIPassLocked 清 busy/refining，但不碰 aiText。
// 阶段 A 的任何一批改造都必须让这张表继续通过。
func TestStopAIPassLockedLegacyBehavior(t *testing.T) {
	a := newTestApp()
	a.mu.Lock()
	a.aiGen = 7
	a.aiBusy = true
	a.aiText = "partial"
	a.aiRefining = true
	a.aiRefineErr = "old error"
	a.stopAIPassLocked()
	got := struct {
		gen              uint64
		busy, refining   bool
		err, text        string
	}{a.aiGen, a.aiBusy, a.aiRefining, a.aiRefineErr, a.aiText}
	a.mu.Unlock()

	want := struct {
		gen              uint64
		busy, refining   bool
		err, text        string
	}{8, false, false, "", "partial"}
	if got != want {
		t.Fatalf("stopAIPassLocked 改变了既有行为: got %+v want %+v", got, want)
	}
}
```

批次 1a 追加的两条护栏（见 [ai_pass_test.go](<../ai_pass_test.go>)）：

```go
// 规格：什么输入算什么状态，不照抄渲染实现。
func TestClassifyAIResult(t *testing.T) { /* 6 组表驱动 */ }

// 基线：四种 phase 经 ui.NewTester 渲染出的文本特征，切读后必须逐字不变。
func TestAIResultStateRenderingBaseline(t *testing.T) { /* idle/streaming/prose/parse_failed */ }
```

`go test -race ./...` 必须每批都跑：多个状态在鼠标钩子 goroutine、`time.AfterFunc`、`RunOnMain` 三个上下文里写，无竞争是「不动功能」的一部分。

---

## 5. 分批计划（每批独立可回滚）

| 批次 | 内容 | 改动文件 | 行为变化 | 回滚方式 |
| --- | --- | --- | --- | --- |
| 0（已完成） | 新增 characterization test | 新增 `state_mirror_test.go` | 无 | 删文件 |
| 1a（已完成） | `ai_pass.go` 枚举 + `classifyAIResult` 纯函数 + `ai_pass_test.go` 规格与渲染基线 | 新增 2 文件，现有文件零改动 | 无 | 删两个新文件 |
| 1b（已完成） | `renderAIResultState` 改按 `aiPassPhase` 分支，渲染基线表逐字不变 | 改 [popup_view.go](<../popup_view.go>) 一个函数 | 无（差分 + 四条验证通过） | 精确反向 edit（见 §6） |
| 2a（已完成） | `popupView` 三个 AI mode 的布尔组合条件切到 `classifyAIResult(s) != aiPassIdle`（同一模式三处，共用一条等价性证明） | 改 [popup_view.go](<../popup_view.go>) 三行 + 追加 2 条护栏测试 | 无（差分 + 四条验证通过） | 精确反向 edit（见 §6） |
| 2b（已完成） | `popupView` 三 mode 的整段分发收进 `switch classifyAIView(s)`，启用 `aiPassStructured`；`cardReadyForMode` 保证各 mode 只认自己的卡片 | 改 [ai_pass.go](<../ai_pass.go>)、[popup_view.go](<../popup_view.go>) 三段 + 新增 `ai_view_test.go` | 无（差分 + 四条验证通过） | 精确反向 edit（见 §6） |
| 2c（已完成） | `popupContentHeight` 与 `handleQuickSave` 共 6 处 `result != nil` 判断切到 `cardReadyForMode(s)`，渲染、高度、保存同源 | 改 [popup_layout.go](<../popup_layout.go>)、[popup_actions.go](<../popup_actions.go>) 各三行 + 新增 `card_ready_test.go` | 无（差分 + 四条验证通过） | 精确反向 edit（见 §6） |
| 3（已完成） | 全库清点剩余读取点并归类，产出 §10 清点表与阶段 A 收尾验收；结论是没有漏切的点，也无需再改源文件 | 仅文档（清点表 + 手册自洽修订） | 无（四条验证全绿） | 删新增小节 |
| 4a（已完成） | 补齐阶段 B 入口的 3 条 characterization test：`stopAIPassLocked`（批次 0 已有）、`restoreTabLocked`、`refineWithAI` | 只改 [state_mirror_test.go](<../state_mirror_test.go>) | 无（只新增测试） | 精确反向 edit 删两段 |
| 4b（已完成） | U1：给精翻拆出独立 `aiRefineGen` / `aiRefineCancel`，点「AI 精翻」不再掐断卡片流。复现测试 `TestRefineWithAIDoesNotCancelCardStream` 修复前 FAIL、修复后 PASS；`stopAIPassLocked` 不清 `aiText` 与 U6 同源，并入 4c | 局部 | **有（修 bug）** | 单提交 revert |
| 4c（已完成） | U6：`hasContent()` 不看 busy，切走时正在加载的页签被当作完成态恢复，半成品 `aiText` 当最终结果展示且不再补发请求。复现测试 `TestSwitchBackDoesNotRestoreLoadingTab` 修复前 FAIL、修复后 PASS；`restoreTabLocked` 不自增 gen 经核查在 `switchMode` 路径上已被 `stopAIPassLocked` / `stopNotesLocked` 覆盖 | 改 [popup_tabs.go](<../popup_tabs.go>) 一个函数 + 新增 `u6_tab_loading_test.go` | **有（修 bug）** | 精确反向 edit + 删新测试 |
| 4d（已完成） | U2：`switchMode` 直改 `busy=false` 不失效 `translateGen`，旧翻译 goroutine 回来会把旧译文 / 引擎 / 词典写进新模式视图。复现测试 `TestSwitchModeInvalidatesTranslationGen` 修复前 FAIL、修复后 PASS | 改 [popup.go](<../popup.go>) 三处（switchMode + 两个 openPopup Reset 段）+ 新增 `u2_switch_mode_test.go` | **有（修 bug）** | 精确反向 edit + 删新测试 |
| 4e（已完成） | A17：`saveOrLog()` 只记日志、不返回 error，`AddBatch` → `ImportEntries` → `ImportFile` 一路返回 `(int, nil)`，磁盘写入失败仍显示「成功导入 N 条 ✓」。复现测试 `TestImportFileReportsSaveFailure` 修复前 FAIL、修复后 PASS；设置页已有「导入失败」分支，只需把 error 显式上送 | 改 [vocabulary.go](<../vocabulary.go>) 一个文件 + 新增 `a17_import_save_test.go` | **有（修 bug）** | 精确反向 edit + 删新测试 |
| 4f（已完成） | A4：审计写的「取消后旧 OCR 结果写回」经实地核查已被 `ocrGeneration` + `ctx` 挡住；确凿缺陷是新截图清空 `source` / `target` 却不失效 `translateGen`，上一轮 OCR 译文的在飞翻译会把旧译文写进新卡片。复现测试 `TestScreenshotOCRInvalidatesInFlightTranslation` 先红后绿，另写 `TestCanceledScreenshotOCRDropsLateResult` 把「取消不写回」钉成规格；生产改动在 `recognizeScreenshot` 锁内补一次 `a.translateGen++`，并加 `recognizeImageFileIn` 变量接缝避免单测启动 PowerShell | 改 [ocr.go](<../ocr.go>) 一个文件 + 新增 `a4_ocr_session_test.go` | **有（修 bug）** | 精确反向 edit + 删新测试 |
| 5a（已完成） | A8：审计写的「解析失败后磁盘文件未被覆盖」不成立，实际 `newStore` 把损坏文件当首次运行、用 `DefaultConfig()` 覆盖用户 config.json，AI URL / 主题 / 自定义动作永久丢失。复现测试 `TestCorruptConfigIsNotOverwritten` 先红后绿，`TestFirstRunStillWritesDefaults` 作对照证明首次运行不受影响 | 改 [config.go](<../config.go>)（开工时 diff=0 的干净文件）+ 新增 `a8_config_corrupt_test.go` | **有（修 bug）** | 精确反向 edit + 删新测试 |
| 5b（已完成） | A18：词库损坏时 `load()` 只 log 后 return、`s.entries` 保持空；`save()` 的 `.bak` 是滚动单副本，第一次加词就被覆盖，第二次写盘后损坏原文永久丢失且 UI 无提示。复现测试 `TestCorruptVocabularyKeepsRecoverableCopy` 先红后绿，`TestRecoveryCopySurvivesLaterSaves` 钉住「两次写盘后副本仍逐字等于原文」，另两条对照证明合法 / 缺失文件不受影响 | 改 [vocabulary.go](<../vocabulary.go>) 的 `load()` 一处（该区间开工时 diff=0 命中，无需接缝）+ 新增 `a18_vocab_corrupt_test.go` | **有（修 bug）** | 精确反向 edit + 删新测试 |
| 5c（已完成） | C5：`isQuitting bool` 被 quitApp（加锁写）、OnBeforeQuit（无锁写）、设置窗口 OnClose（无锁读）三处按三套同步语义访问，构成明确数据竞争（Top10 #7）。改为 `atomic.Bool` + 统一 `Store(true)`/`Load()`、删掉只保护该 bool 的锁块；Store/Load 与原读写严格等价（零行为变化），只统一语义 + 减锁。`TestQuittingStateIsExplicitAndRaceSafe` 源码契约测试先红后绿，`TestQuittingAtomicConcurrentLoadStore` 64 组并发在 `-race` 下回归 | 改 [popup.go](<../popup.go>)、[main.go](<../main.go>)、[settings.go](<../settings.go>) 共 5 处 + 新增 `c5_quitting_state_test.go` | **有（显式化，零行为变化）** | 精确反向 edit + 删新测试 |
| 5d（已完成） | U8：截图翻译置顶后，openPopupWithText 有 `screenshotPinned` 兜底、openPopupAction 没有，两条入口不对等 —— 从 chip / 托盘打开新卡片时 `pinned` 仍为 true，OnBlur 不关闭、窗口永久置顶。新增纯方法 `resetScreenshotPinLocked()` 收口复位规则，两入口都调用；openPopupWithText 行为逐字不变、openPopupAction 补上缺失复位。`TestBothPopupEntriesResetScreenshotPin` 源码契约先红后绿，`TestResetScreenshotPinLocked` 表驱动覆盖 4 组（含用户手动置顶保留、重复调用幂等） | 改 [popup.go](<../popup.go>) 3 处（新增方法 + 两入口）+ 新增 `u8_screenshot_pin_test.go` | **有（修 bug）** | 精确反向 edit + 删新测试 |
| 5e（已完成） | C1：审计把 C1 / C10 / U10 一起判为「需真实 Win32 消息泵 / 钩子重装，无法单测」，但实地读 [hook.go](<../hook.go>) 发现「重装不清 lastClickTime/lastClickPos」的会话边界落在普通 Go 函数 `removeHooks()` 里（非回调），可在 `hookMu` 锁内用纯方法 `resetClickTrackingLocked()` 收口复位；`lowLevelMouseProc` 的 `clickCount++` / `clickCount == 2` 判定一字不动，零行为变化，`hook.go` 开工时 diff=0。`TestRemoveHooksResetsClickTracking` 源码契约先红后绿，`TestResetClickTrackingLocked` 表驱动覆盖残留 / 幂等，既有 `hook_lifecycle_test.go` / `regression_batch_test.go` 未受影响 | 改 [hook.go](<../hook.go>) 2 处（新增方法 + `removeHooks` 调用）+ 新增 `c1_click_tracking_test.go` | **有（显式化，零行为变化）** | 精确反向 edit + 删新测试 |
| 5f（已完成） | C10：审计把 C10 / U10 / C1 一起判为「需真实 Win32 消息泵，无法单测」，但逐行读 [snip.go](<../snip.go>) 发现会话边界落在 `StartNativeSnip` 的普通 Go 逻辑里（消息循环之前），可 headless 收口：`currentSnip` 全局单例被 `:464` 无条件覆盖、`hwnd==0` 等早退不清指针，连按截图两个消息循环各置 nil 并乱序触发回调。新增 `beginSnipSessionLocked`（锁内「检查 + 占位」原子完成，已有会话返回 false）+ `endSnipSessionLocked`（仅当 `currentSnip == s` 才清空，防误清新会话）；`:463-465` 裸赋值替换为守卫调用、false 则 return，成功后 defer 覆盖所有早退路径，`:504-507` 保留不动。`TestStartNativeSnipGuardsSingleton` 源码契约先红后绿，`TestBeginSnipSessionLocked` 走会话生命周期（占位 / 拒绝连按 / 错配不误清 / 结束可重占 / 重复清理幂等），既有 `snip_test.go` 等无需改断言 | 改 [snip.go](<../snip.go>) 3 处（新增 2 方法 + `:463-465` 替换 + defer）+ 新增 `c10_snip_session_test.go` | **有（修 bug）** | 精确反向 edit + 删新测试 |
| 5g（已完成） | A17 残余·补全按钮假成功：`settings_vocab.go` 补全 `go func()` 里 `n := vocabStore.EnrichAll()` 无条件显示「已智能补全 N 条 ✓」，`EnrichAll()` 内部 `saveOrLog()` 丢 error，磁盘写失败时用户看到假成功、重启后数据全丢（与 4e 同源）。`EnrichAll()` 改返回 `(int, error)`，补全按钮按 err 显示「补全失败: ...」；`a17_enrich_save_test.go` 源码契约先红后绿 + 两条行为测试（blocker 普通文件作父目录注入写盘失败、非 ASCII 词条跳过网络）。四个 `go func()` 的并发守卫属大重构，刻意不做 | `vocabulary.go`、`settings_vocab.go` | **有（修 bug）** | 精确反向 edit + 删新测试 |
| 6a（已完成） | A17 四个 go func 并发守卫（阶段 D 首条）：导入 / 导出 CSV / 导出 Anki / 补全原本各起裸 goroutine，连点会并发写同一 GetVocabularyStore() 单例、a.actionSavedMsg toast 互相覆盖、弹出多个 mygo.Dialog。新增 vocab_job.go 的单一作业槽位 tryBeginVocabJob / endVocabJob 收口，四个 go func 各加守卫 + defer，其余逻辑一字不动；忙时给「有生词任务正在进行，请稍候…」可见提示。TestVocabJobsUseSingleGuard 源码契约先红后绿 + TestVocabJobGuardTransitions 表驱动 + TestVocabJobGuardConcurrentBegin 并发恰好一个成功 | 新增 vocab_job.go、a17_vocab_job_test.go，改 [settings_vocab.go](<../settings_vocab.go>) 四处 | 有（修 bug）；单点行为不变 | 精确反向 edit + 删两个新文件 |
| 6b（已完成） | X1 三套字段清单合并·第一刀：`clearTabLocked()` 与 `restoreTabLocked(&tabViewState{})` 的字段集合逐字相同（同一份清单复制两遍），让 clear 委托 restore，三份手工清单减到两份；`snap()` 因含派生 `isWord` 保留但纳入反射同步契约。零行为变化（DeepEqual 证明）。`x1_tab_field_sync_test.go` 三条：characterization / 反射契约改前即绿，`TestClearTabDelegatesToRestore` 委托契约先红后绿。完整合一（`PopupState` struct）因语义不同留后续 | 改 [popup_tabs.go](<../popup_tabs.go>) 1 处（clearTabLocked 委托）+ 新增 `x1_tab_field_sync_test.go` | **零行为变化（显式化）** | 精确反向 edit 还原 36 行清单 + 删新测试 |
| 6c（已完成） | A4 OCR 视图显式化：`renderScreenshotSource` 的 `if s.ocrBusy` / `if s.ocrError != ""` / `if s.source`、`s.ocrText`、`s.ocrError` 任一非空 这三块是**叠加渲染**（只有 busy 分支 `return`，error 态下错误框与编辑区同时显示），收成互斥 switch 会破坏现状，故改为 `ocrViewPhase` 枚举 + `classifyOCRView` / `ocrShowErrorBox` / `ocrShowEditor` 三个纯谓词，控制流与渲染结构一字未动。`ocr_view_test.go` 四条：源码契约先红后绿、7 组分类表、枚举字符串、16 组组合与旧决策逐项对比 | 新增 [ocr_view.go](<../ocr_view.go>) + 改 [ocr.go](<../ocr.go>) 3 行 + 新增 [ocr_view_test.go](<../ocr_view_test.go>) | **零行为变化（显式化）** | 精确反向 edit 还原 3 行判断 + 删新文件 |
| 6d（已完成） | U10 Hover Tip 时间窗口谓词化：`onChipPointer` 的 `warm := !t.hiddenAt.IsZero() && now.Sub(t.hiddenAt) <= chipTipWarm` 与 `due := !t.shown && now.Sub(t.since) >= chipTipDelay` 收成 `chipTipIsWarm()` / `chipTipIsDue()` 两个具名谓词，并加 `chipTipPhase` 阶段枚举（`tipHidden` / `tipWaiting` / `tipWarm` / `tipReady` / `tipShown`）。U10 经复核已有 `mu` + `gen` / `shown` / `timer` / `hits` / `geom` 显式字段与 `hideChipTip()` 的 `gen++` + `win.Hide()` 兜底，无用户可见缺陷；本批零行为变化。`chip_tip_phase_test.go` 四条：源码契约先红后绿、13 组时间边界、6 组阶段优先级、枚举字符串 | 新增 [chip_tip_phase.go](<../chip_tip_phase.go>) + 改 [selicon.go](<../selicon.go>) 2 行 + 新增 [chip_tip_phase_test.go](<../chip_tip_phase_test.go>) | **零行为变化（显式化）** | 精确反向 edit 还原 2 行表达式 + 删新文件 |

批次 4b–4f 与 5a、5b、5c、5d、5e、5f、5g 的每一条都按「先写能复现问题的测试 → 修到通过」做，测试名直接对应 §4 的条目 ID。4a 只加护栏，不改行为；4b、4c、4d、4e、4f、5a、5b、5c、5d、5e、5f、5g 已按此执行（复现测试先红后绿，见 §7）。阶段 B 到此收尾，阶段 C 已按同一节奏完成 5a、5b、5c、5d、5e、5f、5g；用户要求做到成熟工具软件即可、不要无止境继续，阶段 C 到此收口。阶段 D 的 6a 修 A17 连点并发、6b 做 X1 清单合并第一刀、6c 把 A4 的 OCR 三态收成具名谓词、6d 把 U10 的两个时间窗口收成具名谓词并加阶段枚举，均按同一节奏验证。

**批次 4a 的实地修订：先钉现状再动刀。** §10.5 要求进入阶段 B 前把三个入口的现状钉住，否则修 U1 / U6 时无法区分「刻意修 bug」和「顺手改坏了别的行为」。三条护栏集中放在 [state_mirror_test.go](<../state_mirror_test.go>)：`TestStopAIPassLockedLegacyBehavior`（批次 0 已有，录 `aiGen` 7→8、清 busy / refining / err、不清 `aiText`）、`TestRestoreTabLockedLegacyBehavior`（录 `saveTabLocked` 存下 busy 但 `restoreTabLocked` 强制置 false、gen 不变）、`TestRefineWithAILegacyBehavior`（录 `refineWithAI` 调 `stopAIPassLocked` 后 `aiGen` 7→8 并复用该代次，卡片流因此以旧 gen 失配）。`refineWithAI` 会启动真实 goroutine，测试把 `AIURL` 指向无监听端口 `127.0.0.1:1` 让请求立即失败，再轮询等 `aiRefining` 落回 false，避免测试间残留写 `app` 的 goroutine；断言只取 goroutine 不改的三个量（`aiGen` / `aiBusy` / `aiText`），所以稳定。批次 4b 已把 `TestRefineWithAILegacyBehavior` 改名为 `TestRefineWithAIIndependentGeneration`，断言从 `{8, false, "partial"}` 更新为 `{7, 1, true, "partial"}`。

**批次 4b 的实地修订：只修共用代次，不动 `aiText`。** 手册原把 `stopAIPassLocked` 不清 `aiText` 也列进 4b，但实地读 [popup_actions.go:48-64](<../popup_actions.go#L48-L64>) 后发现这是另一个症状：切 tab 后把上一轮半成品当结果展示，与 U6（`restoreTabLocked` 强制置 busy=false、代次不变）同源。一次改两个行为会让差分和复现测试都失去针对性，所以 4b 只修「精翻与卡片流共用 `a.aiGen`」这一条：精翻拆出 `aiRefineGen` / `aiRefineCancel`，`refineWithAI` 不再调 `stopAIPassLocked()`；`stopAIPassLocked()` 反过来要自增 `aiRefineGen` 并取消 `aiRefineCancel`，否则新卡片启动后在跑的精翻仍会把旧译文写回 `a.target`。两个 openPopup Reset 段、switchMode、截图 OCR 都以 `stopAIPassLocked()` 开头，改一处即覆盖全部路径。`aiText` 残留并入 4c，与 U6 一起修，这样 `TestStopAIPassLockedLegacyBehavior`（录「不清 aiText」）在 4b 保持绿。

**批次 4c 的实地修订：只修 `hasContent()`，X1 再推迟。** 手册原把 X1（三份状态手工同步）排在 4c 一起做，并把 4c 写成「`restoreTabLocked` 不自增 gen」。实地读 [popup.go:400-455](<../popup.go#L400-L455>) 后发现两件事：第一，`switchMode` 在 `saveTabLocked()` 之后紧接着调 `stopAIPassLocked()`（[popup_actions.go:52-71](<../popup_actions.go#L52-L71>)）与 `stopNotesLocked()`（[popup_actions.go:76-83](<../popup_actions.go#L76-L83>)），两者已分别自增 `aiGen` / `notesGen` 并取消请求；`restoreTabLocked` 自身不自增 gen 在唯一的生产调用路径上并不构成实际缺陷，补一次自增只是冗余。第二，确凿的用户可见缺陷是 [popup_tabs.go:52-56](<../popup_tabs.go#L52-L56>) 的 `hasContent()` 只看内容不看 busy：切走时 `saveTabLocked` 原样存下 `aiBusy=true` 与半成品 `aiText`，切回来命中缓存后 `restoreTabLocked` 强制置 `aiBusy=false` 并恢复半成品，`renderAIResultState` 把它当最终 markdown 结果展示，且命中缓存会提前 return、不会再补发请求（这正是批次 4b 延后到 4c 的 `aiText` 残留）。`hasContent()` 全库只有 3 处引用（定义、`switchMode:444`、[product_rules_test.go:78](<../product_rules_test.go#L78>)），改一处最局部。X1 的完整重构要同时改 `app` / `snapshot` / `tabViewState` 三套字段清单及其全部读写点，会碰 18 个用户改动里的多个文件，不是「一个读取点」，故再次推迟到阶段 C；U12 / U17 的字段清单遗漏留到各自按需补字段，比一次性大重构安全。

**批次 4d 的实地核查：手册描述与代码吻合，无需改计划。** [popup_actions.go:125-178](<../popup_actions.go#L125-L178>) 的翻译 goroutine 在 `:151` 用 `if gen != a.translateGen` 判定，通过后写 `busy` / `elapsed` / `errText` / `target` / `engine` / `detected` / `dict` / `gdict` 并 `fitHeight()`；`stopAIPassLocked()` 与 `stopNotesLocked()` 只自增各自的 `aiGen` / `notesGen`，**不动** `translateGen`。[popup.go:400-456](<../popup.go#L400-L456>) 的 `switchMode` 只把 `a.busy = false`，所以旧机器翻译回来仍会把旧译文画进新模式视图。修法是在 `a.busy = false` 后自增 `a.translateGen`：早退分支（`:412` 同 mode，用户没切页签）在它之前，不受影响；`rerunCurrentMode` 路径会再自增一次，单调递增计数器无副作用。复现测试要避免真实网络：`switchMode` 末尾的 `rerunCurrentMode(text)` 会发请求，所以测试预先给 `tabStates[modeDictionary]` 塞一个有内容的快照，让 `:444` 命中缓存提前 return，只断言 `translateGen` 变化。**另外两个同类入口**：[popup.go:487-533](<../popup.go#L487-L533>) 的 `openPopupAction` 与 [popup.go:571-609](<../popup.go#L571-L609>) 的 `openPopupWithText` 也设 `a.busy = false` 而不动 `translateGen`。`text != ""` 时 `rerunCurrentMode` 只对 modeTranslate 走 `translateTextWithReflow`（会自增），Dictionary / Grammar / Writing 走 `runAction` 不自增；`text == ""`（手动输入弹窗）根本不启动新 pass。两种情况旧机器翻译都会写回新卡片。两函数都在 `win == nil` 时提前 return，单测构造真实 `mygo.Window` 与 `Screen` 不现实，因此这两处与 switchMode 一起改、但只能靠代码审查 + 全量测试 + 手工走查兜底，对应路径已补进 §7 清单第 2 条。范围收敛：U2 审计还提到 `busy` 无同步读（`renderEngineMenu`），本批只修「切 mode / 开新卡片时旧翻译写回」，其余症状不动。

**批次 4e 的实地核查：只改词库一个文件，「提示文案」改成断言 `ImportFile` 的 error。** 手册原写「让保存失败，断言提示文案」，但 `settings_vocab.go:294-316` 的那段在 `go func()` 里，先弹 `mygo.Dialog.Open` 再写 `a.actionSavedMsg`，单测要构造真实 `mygo.Window` 与文件对话框，不现实。实地读 [vocabulary.go:988-992](<../vocabulary.go#L988-L992>) 后确认：`saveOrLog()` 当前**已经**打日志，但返回 `void`；[vocabulary.go:241-294](<../vocabulary.go#L241-L294>) 的 `AddBatch`、[vocabulary.go:925-943](<../vocabulary.go#L925-L943>) 的 `ImportEntries` 都只返回 `int`，`ImportJSON` / `ImportCSV` / `ImportTXT` 六处一律 `return s.ImportEntries(x), nil`，`ImportFile` 因此拿到 `(n, nil)`；[settings_vocab.go:306-310](<../settings_vocab.go#L306-L310>) 已经有 `if err == nil { 成功导入 } else { 导入失败: ... }`，**改 UI 一个字都不用**。可自动化的等价判据是断言 `ImportFile` 在保存失败时返回非 nil error：修复前日志打着 `vocabulary save failed` 却仍返回 `(1, nil)`（复现测试 FAIL），修复后返回 error（PASS），UI 的失败分支自然生效。范围收敛：公开签名 `AddBatch` / `ImportEntries` 保持不变，新增私有 `addBatch` / `importEntries` 把 `saveOrLog()` 的 error 显式上送，`Import*` 六处改调私有方法；这样既不碰 [regression_test.go](<../regression_test.go>)、[regression_batch_test.go](<../regression_batch_test.go>)、`popup_actions.go:1071` 等既有调用点，也不碰 18 个用户改动。A17 的残余：`Add` / `Delete` / `Clear` / `EnrichAll` 的 `saveOrLog()` 仍只记日志（它们不在「成功导入 ✓」这条路径上），四个 `go func()` 仍无 busy / generation / cancel，一并留到阶段 C。
**批次 4f 的实地修订：审计描述与代码不符，范围收敛到「新截图失效在飞翻译」。** 手册原写「A4：截图 OCR 取消后旧结果仍写回，`ocrBusy` 与 `ocrError` 可同时为真」，但逐行读 [ocr.go:299-360](<../ocr.go#L299-L360>) 后发现三条都不成立：第一，`stopScreenshotOCR()` 自增 `a.ocrGeneration` 并 `cancel()`，goroutine 的 `if generation != a.ocrGeneration || !a.isScreenshotMode` 会丢弃过期结果，[ocr.go:141](<../ocr.go#L141>) 的 `RecognizeImageFileIn` 也尊重 ctx，**取消后旧结果写回已被挡住**，再加守卫属于为假设性并发添复杂逻辑；第二，`ocrBusy` 与 `ocrError` 同时为真在生产序列里不可达（两处启动都 `ocrBusy, ocrError = true, ""`，失败 / 取消先置 busy=false 再写 error，`snap()` 持锁）；第三，单次启动 `ocrGeneration` 多次自增只是状态抖动，不是用户可见缺陷。确凿缺陷是第四处与 U2 同源的入口：`openPinnedScreenshot` / `recognizeScreenshot` 清空 `source` / `target`，**都不让 `translateGen` 失效**，上一轮 OCR 成功后 `a.translateKeepingLines(text)`（[ocr.go:353](<../ocr.go#L353>) → [popup_actions.go:44-46](<../popup_actions.go#L44-L46>)）启动的翻译 goroutine 回来时 `gen == a.translateGen` 仍成立，就把旧译文 / `engine` / `dict` / `gdict` 写进这张刚清空的新卡片。修法是在 `recognizeScreenshot` 锁内 `a.busy = false` 之后、`a.source, a.target = "", ""` 之前加 `a.translateGen++`：`beginScreenshotOCR` 与 `retryScreenshotOCR` 都走 `recognizeScreenshot`，一处覆盖全部启动路径；成功后 `translateKeepingLines` 再自增一次，单调递增计数器无副作用；**不放进 `stopScreenshotOCR()`**，因为那会连带 Esc / 关闭按钮 / 失焦三条路径取消在飞翻译，且 4d 三处已自增会造成重复。测试接缝：截图识别会启动 PowerShell / 加载 Indra 模型，单测不能真跑，所以在 `RecognizeImageFileIn` 上方加 `var recognizeImageFileIn = RecognizeImageFileIn` 并把 goroutine 调用改成该变量，让代次与取消路径可被确定性验证；`a4_ocr_session_test.go` 的 `swapOCRRecognizer` / `waitForOCRGoroutine` 负责替换与等收尾。范围收敛：`ocrBusy` / `ocrError` 的结构性显式化（`ScreenshotOCRSession`）、`translateKeepingLines` 成功后无代次校验、`ocrGeneration` 多次自增的抖动、X1 一并留阶段 C。


**批次 5a 的实地修订：审计描述与代码不符，阶段 C 从「干净文件」起步。** 手册原把阶段 C 首条写成 X1，但 X1 要动 `app` / `snapshot` / `tabViewState` 三套字段清单及全部读写点，必然碰 18 个用户未提交改动里的多个文件，违背「不破坏任何现有功能」与「不动功能」两条硬约束。改选 [STATE_MACHINES.md](STATE_MACHINES.md) §5.1 全局 Top 10 第 8 名的 A8：逐行读 [config.go:241-276](<../config.go#L241-L276>) 与 [config.go:395-468](<../config.go#L395-L468>) 后发现审计两条判断都不成立 —— 三个 `else-if` 分支都调 `saveConfig(s.path, s.Get())` 写全量内存 `cfg`，一次写回就持久化全部迁移，`else-if` 只影响日志文案；`load()` 解析失败 `return false` 会被 `newStore` 的 `if !s.load()` 当成「首次运行」，随即 `saveConfig(s.path, s.cfg)` 用 `DefaultConfig()` **覆盖用户损坏的 config.json**，AI URL / 主题 / 自定义动作永久丢失且无副本可恢复，比审计写的「磁盘不动、每次启动恢复默认」严重。修法仍是「读时派生」：在 `newStore` 第一分支用 `os.Stat(s.path)` 把 `load() bool` 压掉的三态补齐 —— `os.IsNotExist` 才写默认；文件存在但损坏 / 不可读则保留用户文件，仅用内存默认并打一条可执行指引日志。不改 `load() bool` 签名（否则要动 `effort_migrate_test.go` / `popup_ui_test.go` / `regression_test.go` 里 6 处 `if !s.load()` 调用点）、不加新字段、不碰任何测试文件。选它的另一个理由是 [config.go](<../config.go>) 开工时 `git diff` = 0 行，是 18 个用户改动之外的干净文件，本批全部 diff 都是团队自己的，差分最干净。接缝 `resolveConfigPath` 让 `newStore` 的磁盘决策能在 `t.TempDir()` 里被单测，不依赖真实用户数据目录。


**批次 5b 的实地修订：审计结论成立但显式化建议会破坏功能，改用加性恢复副本。** 手册原把阶段 C 余下三条写成 X1 / A17 残余 / A4 结构性，但逐条核查后三条都不适合现在动：X1 要碰 18 个用户未提交改动里的多份字段清单（需用户先提交或 stash）；A17 残余要动 [settings_vocab.go](<../settings_vocab.go>) 四个 `go func()`，那里有 297 行用户 diff；A4 的 `ocrBusy` / `ocrError` 结构性显式化在批次 4f 已证审计两条判断不成立、无用户可见缺陷。改选 A18：它与 5a 的 A8 同为「文件损坏 → 静默按默认 / 空库启动 → 后续写盘覆盖用户数据」，是真实的用户可见数据丢失，且 [vocabulary.go:99-115](<../vocabulary.go#L99-L115>) 的 `load()` 开工时在 `git diff` 里 0 命中（干净落点）、无需接缝（现有测试都直接构造 `&VocabularyStore{path:..., entries:...}`，不碰单例）。逐行读 `save()`（[vocabulary.go:117-163](<../vocabulary.go#L117-L163>)）后补精确了审计的机制：`:159-161` 的 `.bak` 是**滚动单副本**，第一次 `Add` 时它恰好等于损坏原文，第二次 `Add` 就被「加词后的内容」覆盖，损坏原文永久丢失；UI 全程无提示。审计「显式化」建议的「解析失败时不覆盖原文件」若直接实现会让用户无法再「加词」（refusing-to-write），违反不破坏约束，所以改用加性方案：解析失败且 `path+".corrupt"` 不存在时，把原始 `data` 原样另存一份并打可执行指引日志；`return` 前不改 `s.entries`（保留「空库启动」现状）、不改 `s.path`、不动 `save()`、不加 `VocabLoadState` 字段。`a18_vocab_corrupt_test.go` 用 `TestRecoveryCopySurvivesLaterSaves` 直接钉住「两次写盘后 `.corrupt` 仍逐字等于损坏原文」这一数据丢失机制。`VocabLoadState` / `ReadOnly` / `LoadVocabulary` 返回 error 等结构性显式化留阶段 C 后续。

**批次 5c 的实地修订：Top10 剩余条目按「能否确定性单测」筛选，选 C5 显式化而非大重构。** 自我修正：此前把 X1 说成「被 18 个未提交改动阻塞」不准确 —— 过去十几批一直用差分法在这些文件里安全落地；X1 真正难在「三套字段清单合并是大重构、不是一个读取点」，需用户先提交或 stash 才能证明零行为变化。Top10 里 U10（Hover Tip 跨鼠标钩子 goroutine / time.AfterFunc / RunOnMain 三执行上下文，注释自承认 window unclosable）、C10（snip.go 无锁字段 + 全局单例覆盖）、C1（hook.go clickCount 重装不清）都要真实 Win32 消息泵或钩子重装，无法写确定性单测；U8 的 screenshotPinned 兜底需要真实 window。选 C5：同一字段三套同步语义是审计点名的隐式状态机典型，改动面最小、只用标准库 `sync/atomic`、能确定性验证，且「把隐式状态显式化」正对原始诉求。`atomic.Bool` 的 Store/Load 与原读写严格一致，是统一语义 + 减锁而非增加复杂逻辑，UX（退出无未定义行为）/ DX（单一显式状态类型、删冗余锁）/ AX（类型自解释、无隐式契约）三者同时提升。诚实边界：跨回调竞态受 mygo 消息循环调度影响，无法行为级确定性复现，故用「源码契约测试先红后绿 + 改后并发 race 回归」两层，不写虚假的行为级复现测试。

**批次 5d 的实地修订：审计「两入口不对等」属实，但两条入口都要真实 window，改用「纯方法 + 源码契约」两层验证。** 阶段 C 余下 X1（三套字段清单大重构、需用户先提交或 stash）、A17 残余（要动 `settings_vocab.go` 的四个 `go func()`）、A4 结构性（4f 已证审计两条判断不成立），Top10 的 U10 / C10 / C1 需真实 Win32 消息泵 / 钩子重装 / Hover Tip 三执行上下文，均无法确定性单测；U8 是剩余 Top10 里唯一能用纯字段方法验证、不碰消息泵的条目（全局 Top10 #9）。实地读 [popup.go](<../popup.go>)：`openPopupWithText` 的 `:608-614` 有 `if a.screenshotPinned { a.pinned=false; a.screenshotPinned=false }` 兜底，`openPopupAction` 的整个 Reset 段没有，`screenshotPinned` 全库仅 5 处、该兜底段在 `git diff` 里是 context（用户未改），是干净落点。U8 显式化建议本就是「`func (a *app) openPopup(m, text)` 统一收口两条入口」，收口一个具名纯方法是最局部的一步：把内联 if 块原样搬进 `resetScreenshotPinLocked()`（调用方持 `a.mu`、仅在 `screenshotPinned` 时复位 `pinned` 与 `screenshotPinned`，用户手动置顶不受影响），两入口都调用。两条函数都在 `win==nil` 时提前 return、无法 headless 调用，故不编造窗口级行为测试，用「源码契约测试断言两入口都调用同一方法且 `if a.screenshotPinned {` 只收敛到一处（先红后绿）+ 纯方法表驱动行为测试」两层，并在文档写明确定性边界。

**批次 5e 的实地修订：审计把 C1 / C10 / U10 一起判为「需真实 Win32 消息泵 / 钩子重装，无法单测」，拆开后 C1 有可确定性验证的落点。** 手册前几批一直把 Top10 #7（C1）、#5（C10）、#2（U10）归为「需真实 Win32 消息泵 / 钩子重装 / window，无法确定性单测」而整体推迟。本批逐行读 [hook.go](<../hook.go>) 后发现这个判断对 C1 过于笼统：审计风险里的「installHooks/removeHooks 重装不清 lastClickTime/lastClickPos，重装后首次点击可能误判双击」，其会话边界落在 `removeHooks()` 这个**普通 Go 函数**（不是 Win32 回调）里，而 `removeHooks()` 本来就在 `hookMu` 锁内改 `mouseHook`/`keyHook`，加一次纯方法调用即可收口复位，完全能 headless 单测；`hook.go` 开工时 `git diff` = 0 行（干净文件），改它不碰 18 个用户改动。据此选 C1 作为阶段 C 第五条：`removeHooks()` 锁内新增 `resetClickTrackingLocked()` 调用把 `lastClickTime` / `lastClickPos` / `clickCount` 三个包级变量一起归零，新增同名纯方法承载规则；`lowLevelMouseProc` 的 `clickCount++` / `clickCount = 1` / `clickCount == 2` 判定一字不动，复位只在「卸载 = 会话结束」边界发生，下次安装后首次点击因 `info.Time - 0 > doubleClickThreshold()`（系统启动毫秒数远大于 500）必然落入 else 分支从 `clickCount = 1` 起步，零行为变化。复位点选 `removeHooks()` 而非 `installHooks()`：后者有 `already` 早退，且卸载才是语义上的会话结束。C10（snip.go 无锁字段 + 全局单例覆盖）、U10（Hover Tip 跨鼠标钩子 goroutine / time.AfterFunc / RunOnMain 三上下文）确实需要真实 Win32 消息泵或 window，仍留后续；`gesturePhase` / `GestureFSM` / `chipVisibility atomic.Bool` 等结构性显式化与 `chipHoverArmed` 漏复位、双击触发后 clickCount 不复位仍留阶段 C。

**批次 5f 的实地修订：「需真实 Win32 消息泵」是整体判断，拆到具体函数后 C10 有可确定性验证的落点。** 手册前几批一直把 Top10 #5（C10）与 #2（U10）、#7（C1）捆在一起，判为「需真实 Win32 消息泵 / 钩子重装 / window，无法确定性单测」。5e 已把 C1 拆出来；本批逐行读 [snip.go](<../snip.go>) 后发现 C10 的风险里有一条会话边界缺陷同样落在**普通 Go 逻辑**里：`StartNativeSnip` 的 `go func()` 在 `:463-465` 对 `currentSnip` 无条件赋值，但这段在「建窗口 + 消息循环」**之前**，不依赖任何 Win32 回调；`hwnd==0`（`:485-488`）与 GDI / 写盘失败的各 `return` 都发生在消息循环之外，也只需普通 Go 的 defer 就能清理。据此把边界收进两个持锁纯方法：`beginSnipSessionLocked(s)` 在 `currentSnip != nil` 时返回 false（连按截图明确拒绝第二次启动，不再弹两个 overlay 乱序触发回调），否则占位并返回 true；`endSnipSessionLocked(s)` 仅在 `currentSnip == s` 时置 nil（迟到清理不误清新会话）。`StartNativeSnip` 在 `snipMu` 锁内调 `beginSnipSessionLocked`，false 就 `return`（GDI 的 defer 已注册，清理安全），true 则加 defer 覆盖所有早退路径；正常路径的 `:504-507` 保留不动（已置 nil，defer 变 no-op）。行为变化只有两处修 bug：连按不再弹第二个覆盖层、`hwnd==0` 等早退不再残留全局单例；「连按弹两个互相覆盖的结果」本来就是缺陷而非功能。`snipWndProc`(`:239-361`) 字段无锁读写、`snipPhase` / `SnipController` / `withSession` 结构性显式化仍需真实消息泵或较大重构，留阶段 C 后续；U10（Hover Tip 三执行上下文）经复核已有 `gen` 代次 + 三处复查 + 无条件 `win.Hide()` 兜底，审计引用的「window becomes unclosable until restart」注释是在解释为何必须 `gen++`，不是现存缺陷，不再列为待修。验证用「源码契约断言（先红后绿）+ 纯方法会话生命周期测试」两层，不臆造窗口级复现。

**批次 5g 的实地修订：补全按钮是 4e 同一 bug 家族的漏网路径，但改动面比「四个 go func() 并发守卫」小得多。** 阶段 C 余下 X1（大重构）、A17 残余（四个 `go func()` 无 busy/generation/cancel）、A4 结构性（4f 已证无缺陷）、U10（5f 已证无现存缺陷）都不适合动手。逐行读 `settings_vocab.go:377-385` 发现「补全」这条路径 4e 没覆盖到：`EnrichAll()` 返回 `int`、内部 `saveOrLog()` 丢 error、UI 无条件打 ✓，磁盘写失败就是「假成功 + 数据丢失」。修法复用 4e 的「让底层返回 error、UI 已有失败分支生效」——`EnrichAll()` 改 `(int, error)`，补全 `go func()` 按 err 分支。`EnrichAll` 全库仅 UI 一处调用、无测试直接调用，改动最局部。测试用「非 ASCII 词条 `生词` 让 `EnrichVocabularyEntry` 跳过网络」+「blocker 普通文件作父目录让 `os.MkdirAll` 失败」两层确定性注入，不臆造窗口级复现；源码契约断言钉住 error 数据流（补全段在 `go func()` 里、headless 无法触发按钮）。**踩坑**：本批起先把 `saveOrLog()` 注释里的「导入链路」改成「导入与补全链路」，差分严格子序列比对立刻报 `before is subsequence of after: False`——该注释是 4e 已落的 `+` 行，动它会破坏「用户改动逐字保留」证据，遂撤回；撤回后 538 行全部逐字保留、本批只多 6 行 `+`。教训：注释措辞不是需求，别顺手改既有 diff 行。

**批次 6a 的实地修订：用户重新授权后，工作区已干净，四条结构性收尾按「能否确定性验证」排顺序。** 阶段 C 曾由用户「做到成熟即可」收口，用户随后又明确「那就做吧开工」，按 A17 并发守卫 → X1 → A4 → U10 重新授权这四项。d2fe230 提交后工作区干净，本批不再需要改前 / 改后差分来证明没碰用户改动，git diff 全部是本批改动。选 A17 打头：四个 go func（settings_vocab.go:298/323/350/377）连点会并发写同一词库单例与同一条 actionSavedMsg、弹出多个文件对话框，是真实可见缺陷；且四个动作都在 headless 下要开真实 mygo.Dialog，无法行为级复现，故沿用「源码契约（先红后绿）+ 纯方法表驱动 / 并发行为」两层。修复取最局部的单一作业槽位：新增 vocab_job.go 两个纯方法，四个 go func 开头各加 if !a.tryBeginVocabJob() { return } 与 defer endVocabJob()，两把锁从不同时持有（避免与 go func 内部的 a.mu 形成锁序反转），其余逻辑一字不动；忙时在 a.mu 下设 actionSavedMsg 并 cfgWin.Update，与现有 toast 纪律一致。完整 VocabJob 枚举（Kind/State/Generation/Cancel/Progress）与进度 / 取消属更大重构，留阶段 D 后续。

**优先做哪条**：按 [STATE_MACHINES.md](STATE_MACHINES.md) §5.1 的 Top 10 排，但**先挑改动最局部的**。批次 4b–4f 的顺序见上表：U1、U6、U2、A17、A4（已完成）→ 阶段 C 的 A8（5a，已完成）→ A18（5b，已完成）→ C5（5c，已完成）→ U8（5d，已完成）→ C1（5e，已完成）→ C10（5f，已完成）→ A17 补全假成功（5g，已完成）→ 阶段 C 收口，剩余结构性条目刻意不做（见 §10.5）。X1（三份状态手工同步，见 [popup.go:57-249](<../popup.go#L57-L249>)、[popup.go:251-299](<../popup.go#L251-L299>)、[popup_tabs.go:14-50](<../popup_tabs.go#L14-L50>)）是 U6、U12、A24 等多个缺陷的共同根因；按「批次 2a / 4c 的实地修订」与上面「批次 4d 的实地核查」的结论，它再次推迟到阶段 C 单独做，`restoreTabLocked` 的 characterization test 已在批次 4a 补齐。阶段 D 里它由 6b 完成第一刀（clear 委托 restore + 反射同步契约），6c 与 6d 分别把 A4 的 OCR 三态与 U10 的两个时间窗口收成具名谓词，见 §0 阶段 D 与 §5 施工卡。

**批次 6b 的实地修订：X1 只做能确定性验证的一刀。** 逐行读 `popup_tabs.go` 发现 `clearTabLocked()` 的 36 行字段清零与 `restoreTabLocked(&tabViewState{})` 的字段集合、顺序、强制值完全一致（`source` 都不写、`aiCancel` / `notesCancel` 都归零、`notes` 都变 nil），是同一份清单复制两遍 —— 这是三份清单里唯一能用 `reflect.DeepEqual` 证明逐字等价的一对。让 clear 委托 restore，加字段时只需改 save / restore 一处，第三份清单消失；`snap()` 含渲染期派生字段（`isWord: isWord(a.source)`）与 15 个瞬时状态，`tabViewState` 独有 `ocrPath`，两者语义不同，**不做**完整合一（单一 `PopupState` struct 会牵动 app 近百字段与几十处读写点，属需真人 GUI 走查的大重构）。反射契约 `TestTabFieldListsAreSynced` 把剩余 save / restore / snap 三处钉住，防止未来加字段漏同步。这是 X1 里零行为变化、可确定性验证的那一步。

**阶段 D 的优先顺序（用户重新授权）**：A17 并发守卫（6a，已完成）→ X1 字段清单合并（6b，已完成第一刀：clear 委托 restore + 反射同步契约）→ A4 ocrBusy/ocrError 会话显式化（6c，已完成：三态收成具名谓词）→ U10 Hover Tip 阶段枚举化（6d，已完成：两时间窗口收成具名谓词 + 阶段枚举）。工作区已干净，X1 不再受「18 个未提交改动」约束，但这几项自动化测试都收不了口（消息泵 / hover 时序 / 文件对话框 / 窗口置顶要启动 GUI 真人走查），详见 §10.5。

**批次 2a 的实地修订：X1 推迟。** 读了三份字段的实际代码后发现，X1 不能算「一个读取点」：`saveTabLocked`（[popup_tabs.go:58-99](<../popup_tabs.go#L58-L99>)）手工拷 35 个字段进 `tabViewState`，`restoreTabLocked`（[popup_tabs.go:101-138](<../popup_tabs.go#L101-L138>)）又手工写回且**故意不写** `aiBusy` / `notesBusy`（U6 的现状就是把它们强制置 false）。若把三份状态合并成 `tabViewState{snapshot, ocrPath}` 并从 `snapshot` 整体写回，会顺手改掉 U6 的行为，直接违反阶段 A「零行为变化」。因此批次 2 先按 §3 从纯渲染函数开始（2a 切 `popupView` 分发条件）；X1 留到阶段 B 修 U6 时一起做，且必须先为 `restoreTabLocked` 补 characterization test 固定现状。

**批次 2c 的实地核查：哪些 `!= nil` 保留原样。** 全库列了 `result != nil` 类读点后，只切了
`popupContentHeight`（[popup_layout.go:88,133,141](<../popup_layout.go#L88-L141>)）与
`handleQuickSave`（[popup_actions.go:1064,1068,1072](<../popup_actions.go#L1064-L1072>)）
这 6 处——它们都在 `switch s.mode` 的 case 内，语义正是「当前 mode 的卡片就绪」，
与 `cardReadyForMode` 严格等价。另外两类保留：`tabViewState.hasContent()`
（[popup_tabs.go:52-56](<../popup_tabs.go#L52-L56>)）是「任意内容是否存在」的 OR 组合，
还认 `dict` / `gdict` / `aiText` / 截图态，且参数是 `*tabViewState` 不是 `snapshot`；
[popup_actions.go:810](<../popup_actions.go#L810>) 的 `a.aiText != ""` 在 `a.mu.Lock()` 内
判断流结束后的缓存，而 `a.snap()`（[popup.go:301-314](<../popup.go#L301-L314>)）本身要加锁，
锁内调用会死锁。这两处不是「卡片就绪」，不能套同一个查询。

**批次 3 的实地修订：不再「删旧字段」。** 初版表格按「写时镜像」写的，第 3 步是收进显式对象后
删掉旧字段。改成「读时派生」后，`aiBusy` / `aiText` / `grammarResult` 等**仍是唯一事实来源**：
两个分类函数都从 `snapshot` 即时算，写入点一个没动。删旧字段会让派生失去输入，也必然改动
18 个用户文件里的 32 个写入点，违反阶段 A 零行为变化。因此批次 3 改为「全库清点剩余
读取点并归类」，阶段 A 到清点完成即结束；真正的状态收敛（X1 三份字段合并、U1 `aiGen`
拆分、U6 gen 不自增）全部留到阶段 B 批次 4。

---

## 6. 前置条件：当前工作区有一个真实障碍

`git status --short` 显示 18 个用户已修改未提交文件（`actions.go`、`popup.go`、
`popup_actions.go`、`popup_tabs.go`、`ocr.go`、`settings_vocab.go`、`vocabulary.go` 等），
另有 `package.ps1`、`ux_walkthrough_test.go` 两个用户未跟踪文件。团队另外新增了
`ai_pass.go`、若干 `*_test.go` 与 `docs/*.md`，并从批次 2c 起改了原本干净的
`popup_layout.go`（现在也显示为 ` M`）——它的 `git diff` 全部由团队产生，不需要差分隔离。

在这之上直接动手有两个问题：

1. `git diff` 里同时出现你的改动和我的代码，**无法直接看出我改了什么**；
2. 逐批回滚用 `git checkout -- <file>` 会**连你的改动一起丢掉**，回滚不再安全。

**批次 1a 起的绕开办法（不动你的改动，也不替你提交）**：

- 只新增文件的批次（0、1a）：现有文件 `git diff` 零输出，天然隔离。
- 必须改现有文件的批次（1b 起）：改前把 `git diff -- <file>` 存成基线，改后再存一份，
  `diff` 两份即可精确列出「我这次只动了哪几行」；回滚用**精确反向 edit**，把那几行改回原样，
  绝不使用 `git checkout --`。
- 行为等价改由 characterization test 与渲染基线表证明，不依赖工作区干净。

**已完成**：批次 0 的 [state_mirror_test.go](<../state_mirror_test.go>)；批次 1a 的
[ai_pass.go](<../ai_pass.go>) 与 [ai_pass_test.go](<../ai_pass_test.go>)，两者都只新增文件，
不碰任何现有文件，不受上述限制；批次 1b 改 [popup_view.go](<../popup_view.go>) 的
`renderAIResultState` 一个函数，改前/改后各存一份 `git diff` 并 `diff` 两份，差异只有该函数
（见 §7 的记录），18 个已有改动逐行未动。

---

## 7. 验证命令与通过条件

每批结束后跑这四条，`gofmt -d .` 必须无输出：

```bash
cd D:/AI_workspace/translate/fastpot
go test ./...
go test -race ./...
go vet ./...
gofmt -d .
```

阶段 A 额外加两条：

- `git diff` 差分人工检查：本批改动只落在目标读取点内，且只改判断形式、不改判断结果（批次 3 除外，那是删字段）；
- 分类规格与渲染基线测试全绿。

阶段 B / C 额外加一条：每条缺陷的复现测试在修复前失败、修复后通过。

### 批次 1b 验证记录

- 差分：改前 421 行、改后 468 行 `git diff -- popup_view.go`，两份 `diff` 的差异只有
  `renderAIResultState` 一段（`switch classifyAIResult(s)` 四分支）；`index` 行与后续 hunk
  行号变化源于函数体行数变化，其余用户改动逐字未动。
- `gofmt -d .`、`go vet ./...` 均无输出；`go test -count=1 ./...` ok 9.591s；
  `go test -race -count=1 ./...` ok 17.488s。
- 定向渲染测试（`TestClassifyAIResult`、`TestAIResultStateRenderingBaseline`、
  `TestStreamingDoesNotDumpJson`、`TestFailedParseExplainsItself`、
  `TestProseAnswerIsStillShown`、`TestStreamingShowsSingleIndicator`）ok 6.214s；
  `TestAIResultStateRenderingBaseline` 四态 want/absent 文本逐字不变，即零行为变化。
- 手工走查：本批只改一个纯渲染函数，未动事件、goroutine、磁盘写入；渲染文本已由基线表
  逐字覆盖，但**未启动 GUI 做真人点击**，这一项仍是未验证范围，从批次 2 起每批补一次界面走查。

### 批次 2a 验证记录

- 护栏先行：切换前先加 `TestAICardDispatchEquivalence`（4 组 `(aiBusy, aiText)` 组合，断言旧表达式
  `s.aiBusy || s.aiText != ""` 与新表达式 `classifyAIResult(s) != aiPassIdle` 判定相同）与
  `TestAIModeDispatchRenderingBaseline`（三个 AI mode × streaming/idle 两种分发结果，逐字录下
  `aiBusyMessage(mode)` 与各 mode 独有的空态按钮文案）。改前两条全绿。
- 差分：改前/改后 `git diff -- popup_view.go` 均 468 行，两份 `diff` 只有 3 行差异
  （`popup_view.go:160`、`:170`、`:180`），其余用户改动逐字未动。
- `gofmt -d .`、`go vet ./...` 均无输出；`go test -count=1 ./...` ok 15.686s；
  `go test -race -count=1 ./...` ok 17.778s。
- 手工走查：仍未启动 GUI。渲染文本由两条基线表覆盖，界面交互仍是未验证范围。

### 批次 2b 验证记录

- 护栏先行：先在旧 `popupView` 上加 `ai_view_test.go`（`TestClassifyAIViewEquivalence`
  13 组输入，含「卡片优先于 busy」与「grammar mode 不认 dictionary 卡片」两类隔离用例；
  `TestClassifyAIViewFallsBackToResult` 5 组回落用例；`TestAIViewRenderingBaseline`
  三 mode × 卡片/进行中/空态共 9 组渲染文本），切换前定向测试 ok 6.290s。
- 差分：改前 `git diff -- popup_view.go` 468 行、改后 474 行；两份 diff 的差异集中在
  三处 if/else 段（各换成 `switch classifyAIView(s)`），其余差异都是 hunk 行号随函数
  体行数变化的偏移，用户 18 个改动逐字未动。
- `gofmt -d .`、`go vet ./...` 均无输出；`go test -count=1 ./...` ok 14.766s。
- `go test -race -count=1 ./...` 首次 FAIL 且未抓到失败详情；立即原样复测一次
  ok 11.778s，相关测试 `-count=20` 定向压测 ok 3.703s。按 §9 的规则，复测通过
  只说明未再复现，**不据此断言既有偶发竞态已修复**；本批改动只碰纯渲染分发，
  未引入新的共享状态。
- 手工走查：仍未启动 GUI。9 组渲染基线覆盖卡片/进行中/空态的文字，界面交互仍是未验证范围。

### 批次 2c 验证记录

- 护栏先行（新文件 `card_ready_test.go`，切换前在旧代码上全绿 ok 6.494s）：
  `TestCardReadyForModeMatchesLegacyChecks` 11 组（含「grammar mode 不认 dictionary
  卡片」「translate mode 三张卡片都在仍为 false」），断言 `cardReadyForMode(s)` 与旧
  `!= nil` 判定逐字相同；`TestCardReadyMatchesClassifyAIView` 断言渲染判定
  `classifyAIView(s) == aiPassStructured` 与卡片就绪判定同源；
  `TestPopupContentHeightCardGuardBaseline` 把改前 8 组窗口高度录成常量表
  （310 / 262 / 380 / 420 / 436 / 260 / 366 / 260），切换后逐字断言。
  注意 `dictionary/短卡片` 262 < `dictionary/无卡片` 310，所以不能用「有卡片一定更高」
  这类相对断言，必须录具体数值。
- 差分：`popup_layout.go` 改前 `git diff` 为 0 行（原本干净），改后 31 行、只含 3 处
  判断替换；`popup_actions.go` 改前/改后均 136 行，两份 diff 只有 3 行差异
  （`:1064`、`:1068`、`:1072`）+ index 行，用户改动逐字未动。
- `gofmt -d .`、`go vet ./...` 均无输出；`go test -count=1 ./...` ok 23.920s。
- `go test -race -count=1 ./...` 首次 FAIL，**这次抓到了完整堆栈**：
  `WARNING: DATA RACE`，`ai_generation_race_test.go:296` 的 `freshBreakers()` 与
  `services.go:647` 的 `engineBreaker.record()`（由 `services.go:544` 的
  `TranslateRace.func7` 调用）并发写 `0x00c00020c500`；失败用例是
  `TestRaceCancellationIsNotAnEngineFailure/cancelled_in_flight`。
  这是**跨测试的全局 breaker 污染**，与本批改动（纯渲染高度、快捷保存的条件判断）
  没有共享状态关系。按 §9 处理：原样复测全套一次 ok 12.171s；该测试单独跑
  `-count=5` ok 5.370s（单独跑不触发，印证是测试间污染）；本批护栏
  `-race -count=20` ok 1.808s。复测通过只说明未再复现，**不据此断言既有竞态已修复**。
- 手工走查：仍未启动 GUI。窗口高度由 8 组基线表覆盖，界面交互仍是未验证范围。

### 批次 3 验证记录（阶段 A 收尾）

- 本批零源文件改动，产出只有 §10 清点表与手册自洽修订（§0 / §1 / §5 / §8 / §9 中
  「删旧字段」的旧措辞全部改正）。清点用四组全库检索完成，共扫描 227 个 `.go`
  文件，并逐条核对了 `popup_view.go:155-191 / 525-556`、`popup_layout.go:84-145`、
  `popup_actions.go:458-475 / 625-645 / 800-820 / 1064-1073`、
  `popup_tabs.go:50-56`、`ai_pass.go:55-100` 的当前内容，没有依赖历史叙述。
- 阶段 A 收尾验收四条全绿：`gofmt -l .`（空）、`go vet ./...`（clean）、
  `go test -count=1 ./...` ok 8.773s、`go test -race -count=1 ./...` ok 11.219s。
  `.go` 文件自 2c 后未变，这是跨批次的汇总确认，不是对 2c 的复测。
  race 这次没有再出现批次 2c 记录的偶发 FAIL；仍不据此断言那个全局 breaker
  污染已修复，它属于既有测试隔离问题。
- 手工走查：仍未启动 GUI，下面 7 条路径仍是未验证范围。

### 批次 4a 验证记录（阶段 B 前置护栏）

- 只改 [state_mirror_test.go](<../state_mirror_test.go>) 一个文件：新增 `TestRestoreTabLockedLegacyBehavior`、
  `TestRefineWithAILegacyBehavior` 两条，与批次 0 的 `TestStopAIPassLockedLegacyBehavior` 合成 §10.5
  要求的三条入口护栏。源文件、18 个用户改动一个没动。
- 定向验证：`gofmt -d state_mirror_test.go` 无输出；
  `go test -run 'TestStopAIPassLockedLegacyBehavior|TestRestoreTabLockedLegacyBehavior|TestRefineWithAIIndependentGeneration' -count=1 -v .`
  三条全 PASS（0.00s / 0.00s / 0.01s），ok 6.123s。
- 本批四条全绿：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.103s、
  `go test -race -count=1 ./...` ok 17.394s。race 这次没有再出现批次 2c 记录的偶发 FAIL；
  仍不据此断言那个全局 breaker 污染已修复，它属于既有测试隔离问题。
- 护栏压测：`go test -race -run 'TestStopAIPassLockedLegacyBehavior|TestRestoreTabLockedLegacyBehavior|TestRefineWithAIIndependentGeneration' -count=20 .`
  ok 1.776s，20 轮无抖动。
- 手工走查：仍未启动 GUI，下面 7 条路径仍是未验证范围。4a 零行为变化，走查从 4b 起每批补。
- 批次 4b 后：上面两条 `-run` 命令原用 `TestRefineWithAILegacyBehavior`，现已换成 `TestRefineWithAIIndependentGeneration` 以保证命令可执行；测试改名与断言更新见下面批次 4b 验证记录。

### 批次 4b 验证记录（U1 精翻独立代次）

- 复现测试先行：新增 [u1_refine_generation_test.go](<../u1_refine_generation_test.go>) 的 `TestRefineWithAIDoesNotCancelCardStream`，设 `aiGen=7 / aiBusy=true / aiText="partial"` 后调 `refineWithAI()`，期望三者不变。修复前 FAIL：`got {gen:8 busy:false text:partial} want {gen:7 busy:true text:partial}`，与 §4.2 U1 的症状一致。
- 生产改动只落 3 处：[popup.go](<../popup.go>) 新增 `aiRefineGen uint64` / `aiRefineCancel context.CancelFunc` 两个字段；[popup_actions.go](<../popup_actions.go>) 的 `stopAIPassLocked()` 加 7 行让新卡片使在跑的精翻失效；`refineWithAI()` 锁内改用独立代次、goroutine 判定改 `gen != a.aiRefineGen`、收尾清 `aiRefineCancel`。
- 护栏同步：`TestRefineWithAILegacyBehavior` 改名 `TestRefineWithAIIndependentGeneration`，断言从 `{8, false, "partial"}` 更新为 `{7, 1, true, "partial"}`（`aiRefineGen` 自增到 1）；`TestStopAIPassLockedLegacyBehavior`、`TestRestoreTabLockedLegacyBehavior` 不动，仍绿。
- 差分：`git diff -- popup.go popup_actions.go` 改前 263 行 / 改后 318 行，两份 diff 的差异只有「popup.go 新增 2 行 + popup_actions.go 的 stopAIPassLocked 段与 refineWithAI 段」；其余全是 hunk 行号偏移（+2 / +7 / +14）与 index 行变化，18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 14.638s、`go test -race -count=1 ./...` **首次 FAIL 并抓到完整堆栈**，与批次 2c 同一处偶发竞态：`ai_generation_race_test.go:296` 的 `freshBreakers()` 与 `services.go:647` 的 `engineBreaker.record()`（由 `services.go:544` 的 `TranslateRace.func7` 调用，goroutine 起于 `services.go:531` ← `popup_actions.go:460` 的 `prefillActionTranslation.func1`）并发写 `0x00c00012a0e0`，失败用例 `TestRaceCancellationIsNotAnEngineFailure/cancelled_in_flight`。堆栈里没有本批改动的任何函数，判定为跨测试全局 breaker 污染，按规则不接手修复。原样复测全套 ok 11.159s，该用例单独 `-count=5` ok 5.367s，新护栏 `-race -count=20` ok 1.728s。复测通过只说明未再复现，不据此断言既有竞态已修。
- 手工走查：仍未启动 GUI，下面 7 条路径仍是未验证范围；第 5 条的预期已按 4b 更新。

### 批次 4c 验证记录（U6 页签加载态）

- 复现测试先行：新增 [u6_tab_loading_test.go](<../u6_tab_loading_test.go>) 的 `TestSwitchBackDoesNotRestoreLoadingTab`，设 `aiBusy=true / notesBusy=true / aiText="partial"` 后 `saveTabLocked()`，断言这个进行中的快照不应被当作完成态缓存；再置回完成态断言仍可恢复。修复前 FAIL：`进行中的页签不应被当作可恢复的完成态：aiBusy=true notesBusy=true aiText="partial"`，修复后 PASS。
- 生产改动只落一处：[popup_tabs.go](<../popup_tabs.go>) 的 `hasContent()` 加 `!s.aiBusy && !s.notesBusy` 与 3 行注释。`saveTabLocked` / `restoreTabLocked` / `clearTabLocked` 一个字没动，所以批次 4a 的三条 characterization test（`TestStopAIPassLockedLegacyBehavior`、`TestRestoreTabLockedLegacyBehavior`、`TestRefineWithAIIndependentGeneration`）逐字未改且仍绿；`product_rules_test.go` 的 `TestTabStateSaveRestore` 也仍绿，证明「已完成的页签仍可恢复」这条既有功能没被关掉。
- 差分：`git diff -- popup_tabs.go popup.go` 改前 169 行 / 改后 181 行；`diff` 两份 diff 的差异只有新增的 `hasContent()` hunk（3 行注释 + 1 行 return 改动），其余全是 hunk 行号偏移（+3）与 index 行变化（`e903af6..a253089` → `e903af6..ff808d5`），18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 16.029s、`go test -race -count=1 ./...` ok 16.469s（这次未再出现批次 2c / 4b 的偶发 FAIL，仍不据此断言既有全局 breaker 污染已修）。定向 6 条全 PASS ok 0.053s；相关 3 条 `-race -count=20` ok 1.650s。
- 手工走查：仍未启动 GUI，下面 7 条路径仍是未验证范围；第 2 条的预期已按 4c 更新。

### 批次 4d 验证记录（U2 旧翻译写回）

- 复现测试先行：新增 [u2_switch_mode_test.go](<../u2_switch_mode_test.go>) 的 `TestSwitchModeInvalidatesTranslationGen`，设 `translateGen=5`、`busy=true`、`source="hello"`，预置 `tabStates[modeDictionary]` 让 `switchMode` 命中缓存、不发网络请求，断言 `translateGen` 不等于 5。修复前 FAIL：`switchMode 没有让在飞的翻译代次失效：旧 goroutine 回来仍会把旧译文写进新视图`，修复后 PASS。
- 生产改动落在 [popup.go](<../popup.go>) 三处对称位置：`switchMode`（`:430`）、`openPopupAction`（`:506`）、`openPopupWithText`（`:587`）各在 `a.busy = false` 后加中文注释与 `a.translateGen++`。同 mode 早退分支在 switchMode 的自增之前，不受影响；`rerunCurrentMode` 路径会再自增一次，单调递增计数器无副作用。后两处因 `win == nil` 提前 return 无法单测，见 §5 的说明。
- 批次 4a 的三条 characterization test 逐字未改且仍绿；`TestTabStateSaveRestore`、`TestSwitchBackDoesNotRestoreLoadingTab` 也仍绿（本批不动 save / restore / hasContent）。
- 差分：`git diff -- popup.go` 改前 129 行 / 改后 155 行；`diff` 两份 diff 的差异只有三个新增 hunk（switchMode 2 行注释 + 自增、两个 openPopup Reset 段各 1 行注释 + 自增）与 index 行变化（`edeab4b..2f41bd7` → `edeab4b..3781ba2`），其余全是 hunk 行号偏移，18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 14.740s。`go test -race -count=1 ./...` **首次 FAIL 并抓到完整堆栈**：`WARNING: DATA RACE`，`ai_generation_race_test.go:296` 的 `freshBreakers()` 与 `services.go:647` 的 `engineBreaker.record()`（由 `services.go:544` 的 `TranslateRace.func7` 调用，goroutine 创建于 `services.go:531` ← `popup_actions.go:460` 的 `prefillActionTranslation.func1`）并发写 `0x00c00020c3a0`，失败用例 `TestRaceCancellationIsNotAnEngineFailure`（`testing.go:1867: race detected during execution of test`）。堆栈无本批任何函数，判定为既有的跨测试全局 breaker 污染（与批次 2c / 4b 同源），按规则不接手修复。原样复测全套 ok 10.965s（`WARNING` 计数 0）；该用例单独 `-count=5` ok 5.361s；新护栏 `-race -count=20` ok 1.703s。复测通过只说明未再复现。定向 7 条全 PASS ok 0.059s。
- 手工走查：仍未启动 GUI，下面 7 条路径仍是未验证范围；第 2 条的预期已按 4c / 4d 更新，且两个 openPopup 入口没有自动化复现测试，尤其依赖这一条走查。

### 批次 4e 验证记录（A17 假成功提示）

- 复现测试先行：新增 [a17_import_save_test.go](<../a17_import_save_test.go>) 的 `TestImportFileReportsSaveFailure`，把 `VocabularyStore.path` 指向「父目录是普通文件」的路径（`os.MkdirAll` 必失败），再读一个合法的 `words.json`。修复前 FAIL：`导入在磁盘写入失败时仍报成功（n=1），用户重启后会丢生词`（日志同时打出 `vocabulary save failed`，正好证明错误被吞）；修复后 PASS。另写 `TestImportFileSuccessKeepsNoError` 作对照，正常路径仍返回 `(1, nil)`。
- 生产改动只落 [vocabulary.go](<../vocabulary.go>) 一个文件：`saveOrLog()` 改为返回 `error`；`AddBatch` / `ImportEntries` 保持公开签名不变，各自薄包装新增的私有 `addBatch` / `importEntries`（同一次合并与写盘，但把 error 交出去）；`ImportJSON` 三处、`ImportCSV` 一处、`ImportTXT` 一处共 6 处 `return s.ImportEntries(x), nil` 改为 `return s.importEntries(x)`。公开签名没变，所以 `regression_test.go` 的 `TestAddBatchWritesOnceAndMerges`、`regression_batch_test.go` 的 `TestImportEntriesReportsRealAdditions`、`vocab_export_import_test.go` 的 `TestExportImportVocabulary`、`atomic_save_test.go` 的 `TestSaveFailureIsReportedNotSwallowed` 全部逐字未改且仍绿；批次 4a 的三条 characterization test 也不在词库链路上，无需改断言。
- 差分：`git diff -- vocabulary.go` 改前 312 行 / 改后 426 行；`diff` 两份 diff 的差异只有新增的 `AddBatch` / `importEntries` / `saveOrLog` 三个 hunk 与 6 处 `return` 的替换，其余全是 hunk 行号偏移（+5 / +280）与 index 行变化（`5cecc52..8f6fbee` → `5cecc52..7f39936`），18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 15.041s。`go test -race -count=1 ./...` **首次 FAIL 并抓到完整堆栈**：`WARNING: DATA RACE`，`ai_generation_race_test.go:296` 的 `freshBreakers()` 与 `services.go:647` 的 `engineBreaker.record()`（由 `services.go:544` 的 `TranslateRace.func7` 调用，goroutine 创建于 `services.go:531` ← `popup_actions.go:460` 的 `prefillActionTranslation.func1`）并发写 `0x00c00090e0a0`，失败用例 `TestRaceCancellationIsNotAnEngineFailure/cancelled_in_flight`（`testing.go:1867: race detected during execution of test`）。堆栈无本批任何函数，判定为既有的跨测试全局 breaker 污染（与批次 2c / 4b / 4d 同源），按规则不接手修复。原样复测全套 ok 10.950s（`WARNING` 计数 0）；该用例单独 `-count=5` ok 5.357s；新护栏 `-race -count=20` ok 1.730s。复测通过只说明未再复现。定向 6 条全 PASS ok 6.458s。
- 手工走查：仍未启动 GUI，下面 7 条路径仍是未验证范围；第 6 条的预期已按 4e 更新。

### 批次 4f 验证记录（A4 新截图失效在飞翻译）

- 复现测试先行：新增 [a4_ocr_session_test.go](<../a4_ocr_session_test.go>)。`TestScreenshotOCRInvalidatesInFlightTranslation` 用 `swapOCRRecognizer` 把引擎换成可控 fake（`close(entered)` → 等 `<-release` → 返回错误），设 `isScreenshotMode=true / busy=true / translateGen=5 / source="上一轮原文" / target="上一轮译文"`，调 `a.recognizeScreenshot("fake.png")` 后断言 `translateGen != 5`。修复前 FAIL：`新截图没有让在飞的机器翻译代次失效：旧译文仍会写进这张新卡片`；修复后 PASS。`TestCanceledScreenshotOCRDropsLateResult` 是 characterization：fake 返回成功文本，启动后调 `a.stopScreenshotOCR()` 模拟点「取消」，重设哨兵再 `close(release)`，300ms 有界轮询确认 `ocrText` / `source` / `ocrError` 都未被旧结果改写，把「取消不写回」钉成规格（这条修复前后都绿，说明审计描述的写回路径当前并不存在）。
- 生产改动只落 [ocr.go](<../ocr.go>) 一个文件，两处：新增 `var recognizeImageFileIn = RecognizeImageFileIn` 并把 `recognizeScreenshot` goroutine 的调用改成该变量（测试接缝，让取消 / 代次路径不必启动 PowerShell）；在锁内 `a.busy = false` 后补 2 行中文注释 + `a.translateGen++`。`stopScreenshotOCR` / `openPinnedScreenshot` / `beginScreenshotOCR` / `retryScreenshotOCR` 一个字没动，所以 `popup_ui_test.go` 的 `TestScreenshotEditingAndRecognitionState` 与 `TestStopScreenshotOCRInvalidatesLateResult` 逐字未改仍绿，批次 4a 三条 characterization test 也与此链路无关、无需改断言。
- 差分：`git diff -- ocr.go` 改前 23 行 / 改后 53 行；`diff` 两份 diff 的差异只有新增的接缝 hunk（2 行注释 + `var` + 空行）、`a.translateGen++` hunk（2 行注释 + 自增）、调用点 `RecognizeImageFileIn` → `recognizeImageFileIn` 的替换，以及用户原有 `renderScreenshotSource` hunk 的行号偏移；单独比对该 hunk 正文 `IDENTICAL`，18 个用户改动逐字未动。其余文件 diff 行数与开工一致（`popup.go` 155、`popup_actions.go` 189、`popup_tabs.go` 52、`popup_view.go` 474、`vocabulary.go` 426、`popup_layout.go` 31）。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.776s、`go test -race -count=1 ./...` ok 17.632s（本批首次即通过，`WARNING` 计数 0，不像批次 2c / 4b / 4d / 4e 那样抓到偶发 FAIL；仍不据此断言既有的跨测试全局 breaker 污染已修）。定向 4 条 `TestScreenshotOCRInvalidatesInFlightTranslation` / `TestCanceledScreenshotOCRDropsLateResult` / `TestScreenshotEditingAndRecognitionState` / `TestStopScreenshotOCRInvalidatesLateResult` 全 PASS ok 6.520s；两条新护栏 `-race -count=20` ok 7.667s。
- 手工走查：仍未启动 GUI，下面 7 条路径仍是未验证范围；第 4 条的预期已按 4f 更新（新截图不应闪回上一轮 OCR 译文），取消不写回已由 characterization test 覆盖，真人点击仍待补。

### 批次 5a 验证记录（A8 配置损坏不覆盖用户文件）

- 复现测试先行：新增 [a8_config_corrupt_test.go](<../a8_config_corrupt_test.go>)。`swapConfigPath` 把包级 `resolveConfigPath` 换到 `t.TempDir()` 下的路径，让 `newStore()` 的磁盘决策脱离真实用户数据目录；`TestCorruptConfigIsNotOverwritten` 先写一个截断的 config.json，调 `newStore()` 后断言磁盘内容逐字不变。修复前 FAIL：`损坏的 config.json 被默认配置覆盖，用户设置无法找回`（got 是一整份 `DefaultConfig()` JSON）；修复后 PASS。`TestFirstRunStillWritesDefaults` 是对照：文件不存在时仍写出合法默认 JSON 且 `AIURL` / `AIModel` 等于 `DefaultConfig()`，证明没破坏首次运行。
- 生产改动只落 [config.go](<../config.go>) 一个文件，两处：把 `mygo.App.Path` 抽成 `var resolveConfigPath`（行为等价的测试接缝，`newStore` 改成 `s.path = resolveConfigPath()`）；`newStore` 第一分支改用 `os.Stat(s.path)` + `os.IsNotExist` 区分文件缺失与文件损坏，后者跳过写盘、保留用户文件、用内存默认并打可执行指引。`load()` / `saveConfig()` / 三个迁移 `else-if` 分支一个字没动，所以 `effort_migrate_test.go` 的 `TestRetiredEffortLabelIsRewrittenOnDisk` / `TestCurrentEffortLabelIsNotRewritten`、`popup_ui_test.go` 的 notes migration、`regression_test.go` 的重载测试全部逐字未改仍绿；批次 4a 三条 characterization test 与配置加载链路无关，无需改断言。
- 差分：[config.go](<../config.go>) 开工时 `git diff` = 0 行（干净文件），改后 46 行，全部是本批的接缝与 `newStore` 第一分支；没有用户改动混在里面，其余文件 diff 行数与开工一致（`popup.go` 155、`popup_actions.go` 189、`popup_tabs.go` 52、`popup_view.go` 474、`vocabulary.go` 426、`ocr.go` 53、`popup_layout.go` 31、`settings_vocab.go` 297），18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.598s、`go test -race -count=1 ./...` ok 17.497s（本批首次即通过，`WARNING: DATA RACE` 计数 0；仍不据此断言既有的跨测试全局 breaker 污染已修）。新护栏 `-race -count=20` ok 1.732s；定向 5 条（两条新测试 + `TestRetiredEffortLabelIsRewrittenOnDisk` / `TestCurrentEffortLabelIsNotRewritten` / `TestSaveFailureIsReportedNotSwallowed`）全 PASS ok 0.082s。
- 手工走查：仍未启动 GUI，清单新增第 8 条（配置文件损坏）仍待真人验证；此前各批记录里的「7 条」指当时的清单长度。

### 批次 5b 验证记录（A18 词库损坏保留可恢复副本）

- 复现测试先行：新增 [a18_vocab_corrupt_test.go](<../a18_vocab_corrupt_test.go>)。`TestCorruptVocabularyKeepsRecoverableCopy` 把截断 JSON 写进 `t.TempDir()` 的 vocabulary.json，直接构造 `&VocabularyStore{path:..., entries:...}` 调 `s.load()`，断言 `path+".corrupt"` 存在且逐字等于损坏原文、`s.Count()==0`、原文件未被改动；修复前 FAIL：`损坏的词库没有保留可恢复副本 ...`，修复后 PASS。`TestRecoveryCopySurvivesLaterSaves` 在 `load()` 后连做两次 `s.Add`（填 `Phonetic` / `Difficulty` 跳过有道网络补全），断言 `.corrupt` 仍逐字等于损坏原文，直接钉住「.bak 滚动覆盖」这一数据丢失机制；修复前 FAIL。`TestIntactVocabularyLoadsWithoutRecoveryCopy` 与 `TestMissingVocabularyNeedsNoRecoveryCopy` 作对照，证明合法 / 缺失文件不产生副本。日志坐实链路：`failed to parse vocabulary.json: unexpected end of JSON input` → `damaged vocabulary kept at ...; repair or delete the original file to reset`。
- 生产改动只落 [vocabulary.go](<../vocabulary.go>) 的 `load()` 一处：解析失败分支在现有 `log.Printf` 后新增 4 行中文注释 + `os.Stat(recovery)` / `os.IsNotExist` 判断 + `os.WriteFile(recovery, data, 0o644)` + 成功 / 失败两条日志。`return` 前不动 `s.entries` / `s.path`，`save()` / `Add()` / `saveOrLog()` 一个字没动，所以 `regression_test.go` 的 `TestSaveKeepsBackupCopy` / `TestAddBatchWritesOnceAndMerges`、`vocabulary_test.go` 的 `TestVocabularyDeleteDoesNotDeadlock`、批次 4e 的 `TestImportFileReportsSaveFailure` 全部逐字未改仍绿；批次 4a 三条 characterization test 与词库加载链路无关，无需改断言。
- 差分：`vocabulary.go` 改前 426 行 / 改后 444 行 `git diff`；严格比对两份 diff 的「真正改动行」（`+` / `-`，排除 context）证明改前 316 个改动行**全部按序逐字保留**，本批只多出 11 行 `+`（全是恢复副本逻辑）、没有任何 `-` 行，其余差异只是新增 hunk 的 context 与后续 hunk 行号偏移（+12 / +11）；18 个用户改动逐字未动。其余文件 diff 行数与开工一致（`popup.go` 155、`popup_actions.go` 189、`popup_tabs.go` 52、`popup_view.go` 474、`ocr.go` 53、`popup_layout.go` 31、`config.go` 46、`settings_vocab.go` 297）。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.578s、`go test -race -count=1 ./...` **首次 FAIL 并抓到完整堆栈**：`WARNING: DATA RACE`，`ai_generation_race_test.go:296` 的 `freshBreakers()` 与 `services.go:647` 的 `engineBreaker.record()`（由 `services.go:544` 的 `TranslateRace.func7` 调用，goroutine 创建于 `services.go:531` ← `popup_actions.go:460` 的 `prefillActionTranslation.func1`）并发写 `0x00c0000be400`，失败用例 `TestRaceCancellationIsNotAnEngineFailure/cancelled_in_flight`（`testing.go:1867: race detected during execution of test`）。堆栈无本批任何函数，判定为既有的跨测试全局 breaker 污染（与 2c / 4b / 4d / 4e 同源），按规则不接手修复。原样复测全套 ok 11.546s（`WARNING` 计数 0）；该用例单独 `-count=5` ok 5.373s（单独跑不触发，印证测试间污染）；新护栏 `-race -count=20` ok 8.370s、既有词库护栏 `-race -count=20` ok 6.720s。复测通过只说明未再复现，不据此断言既有竞态已修。
- 手工走查：仍未启动 GUI，清单新增第 9 条（词库损坏）仍待真人验证；自动化只覆盖 `load()` 这一层。

### 批次 5c 验证记录（C5 isQuitting 显式化）

- 测试先行：新增 [c5_quitting_state_test.go](<../c5_quitting_state_test.go>)。`TestQuittingStateIsExplicitAndRaceSafe` 用项目既有源码断言范式（`source_assert_test.go` 的 `readSourceFile`/`countOccurrences`/`containsText`）断言 popup.go 声明 `atomic.Bool` 并 import `sync/atomic`、main.go 0 处裸写且 2 处 `.Store(true)`、settings.go 用 `.Load()` 且无裸读；修复前 FAIL：`popup.go 未把 isQuitting 声明为 atomic.Bool / 未引入 sync/atomic`，ok 6.236s。`TestQuittingAtomicConcurrentLoadStore` 用 64 组 goroutine 并发 Store/Load，改后才编译通过，专作 `-race` 回归，不承担先红。
- 生产改动 5 处（全部 `edit` 保持 LF，CRLF 全 0）：[popup.go](<../popup.go>) import 加 `"sync/atomic"`；字段 `isQuitting bool` → 中文注释 + `isQuitting atomic.Bool`（字段上方注释打断 gofmt 对齐组，故用单空格）；[main.go](<../main.go>) `quitApp` 的三行锁块 → 中文注释 + 单行 `.Store(true)`、`OnBeforeQuit` 回调裸写 → `.Store(true)`；[settings.go](<../settings.go>) `if a.isQuitting {` → `if a.isQuitting.Load() {`。前置核查：全库 app 只用 `*app` 指针与 `&app{}`，无值拷贝，`snapshot` 不含该字段，故 noCopy 不触发 `go vet` copylocks；三处访问点均不在用户 diff 区。
- 差分：`git diff -- popup.go main.go settings.go` 改前 369 行 / 改后 410 行；Python 严格子序列法比对「真正 +/- 改动行」证明改前 203 行**全部按序逐字保留 = True**，本批只多出 13 行（5 处编辑的 -/+ 行），18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.452s、定向两测试 PASS ok 6.208s。`go test -race -count=1 ./...` **首次 FAIL 且原样复测第二次仍 FAIL**（两次均 exit=1、`WARNING: DATA RACE` 计数 1，存 `/tmp/r15-race-full.log` 与 `/tmp/r15-race-recheck.log`）：`ai_generation_race_test.go:296` 的 `freshBreakers()` 与 `services.go:647` 的 `engineBreaker.record()` 并发写 `0x00c00089c120`，失败用例 `TestRaceCancellationIsNotAnEngineFailure/cancelled_in_flight`（`testing.go:1867: race detected during execution of test`）。堆栈无本批任何函数，是与 2c/4b/4d/4e/5b 同源的既有跨测试全局 breaker 污染，按规则不接手修复。按「连续两次无新证据换方法」改变方法：① 该污染用例单独 `-race -count=5` ok 5.348s（单独跑不触发，印证测试间污染）；② `go test -race -count=1 -skip 'TestRaceCancellationIsNotAnEngineFailure' ./...` exit=0、DATA RACE 计数 0、ok 10.922s（证明除该既有问题外全库含本批 c5 测试在 race 下干净）；③ 本批两测试 `-race -count=20` ok 1.612s。复测通过只说明未再复现，不据此断言既有竞态已修。
- 手工走查：仍未启动 GUI，清单新增第 10 条（退出流程）仍待真人验证；跨回调竞态只能靠源码契约 + race detector，不是行为级复现。

### 批次 5d 验证记录（U8 两入口置顶复位收口）

- 测试先行：新增 [u8_screenshot_pin_test.go](<../u8_screenshot_pin_test.go>)。`TestBothPopupEntriesResetScreenshotPin` 用项目既有 `source_assert_test.go` 的 `readSourceFile`/`countOccurrences`/`containsText`，断言 [popup.go](<../popup.go>) 声明 `resetScreenshotPinLocked()`、用 `strings.Index` 切出 `openPopupAction` 与 `openPopupWithText` 两个函数体并各断言含 `a.resetScreenshotPinLocked()`、且 `if a.screenshotPinned {` 全库只收敛到 1 处；修复前 FAIL：`缺少统一的截图置顶复位方法 resetScreenshotPinLocked，两条入口仍各自解释 screenshotPinned`，ok 6.205s。`TestResetScreenshotPinLocked` 是改后才编译通过的纯方法表驱动测试（4 组：截图临时置顶被释放 / 用户手动置顶保留 / 本来没置顶不变 / 只置标记未置窗口也归零），每组重复调用验证幂等；定向两条 PASS ok 6.220s。
- 生产改动只落 [popup.go](<../popup.go>) 3 处（Python `read_bytes`/`write_bytes(s.encode("utf-8"))` 保持 LF，CRLF=0）：在 `switchMode` 与 `openPopupAction` 之间新增 `resetScreenshotPinLocked()` 方法（4 行中文注释 + 7 行函数体）；`openPopupWithText` 的 `:608-614` 内联 if 块与 3 行英文注释替换为一行调用（行为逐字不变）；`openPopupAction` Reset 段末尾 `a.ocrCompare, a.ocrExpanded = false, false` 之后、`a.mu.Unlock()` 之前补一次调用（修真实泄漏）。`screenshotPinned` / `pinned` 字段、`ocr.go` 的 `openPinnedScreenshot`、`main.go` 的 OnBlur 一个字没动，三条 characterization test 与词库 / 配置护栏无需改断言。
- 差分：`git diff -- popup.go` 改前 166 行 / 改后 204 行；Python 严格子序列法比对「真正 +/- 改动行」证明改前 60 行**全部按序逐字保留 = True**，本批只多出 20 行（新增方法 10 行、两入口调用 2 行、原内联块 7 行删除 + 1 行新注释），18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空，新建测试文件经 `gofmt -w` 修过 1 处对齐）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 8.733s、定向两条 PASS ok 6.220s。`go test -race -count=1 ./...` **首次 FAIL 并抓到完整堆栈**：`WARNING: DATA RACE`，`ai_generation_race_test.go:296` 的 `freshBreakers()` 与 `services.go:647` 的 `engineBreaker.record()`（由 `services.go:544` 的 `TranslateRace.func7` 调用，goroutine 创建于 `services.go:531` ← `popup_actions.go:460` 的 `prefillActionTranslation.func1`）并发写 `0x00c0008a00c0`，失败用例 `TestRaceCancellationIsNotAnEngineFailure/cancelled_in_flight`（`testing.go:1867: race detected during execution of test`）。堆栈无本批任何函数，是与 2c / 4b / 4d / 4e / 5b / 5c 同源的既有跨测试全局 breaker 污染，按规则不接手修复。按「换方法取证」：① 原样复测全套 exit=0、DATA RACE 计数 0（这次未再复现）；② 该污染用例单独 `-race -count=5` ok 5.356s；③ `-skip 'TestRaceCancellationIsNotAnEngineFailure'` 后全库 `-race` exit=0、DATA RACE 计数 0、ok 10.857s（证明本批 u8 测试在 race 下干净）；④ 本批两测试 `-race -count=20` ok 1.528s。复测通过只说明未再复现，不据此断言既有竞态已修。
- 手工走查：仍未启动 GUI，清单新增第 11 条（截图置顶后走 chip / 托盘打开新卡片）仍待真人验证；两条入口都需真实 window，自动化只覆盖纯方法与源码契约两层。

### 批次 5e 验证记录（C1 钩子卸载复位双击候选）

- 测试先行：新增 [c1_click_tracking_test.go](<../c1_click_tracking_test.go>)。`TestRemoveHooksResetsClickTracking` 用项目既有 `source_assert_test.go` 的 `readSourceFile`/`containsText`，断言 [hook.go](<../hook.go>) 声明 `resetClickTrackingLocked()`、切出 `removeHooks()` 函数体并断言体内含 `resetClickTrackingLocked()` 且调用在 `hookMu.Unlock()` 之前（锁内）、并断言 `clickCount++` 与 `clickCount == 2` 仍在（证明手势识别没被改）；修复前 FAIL：`钩子卸载没有收口的双击候选复位方法 resetClickTrackingLocked`，ok 6.199s。改后追加 `TestResetClickTrackingLocked`（表驱动 3 组：双击后的残留 / 单击后的残留 / 已经是干净状态，每组重复调用验证幂等；测试保存并恢复三个包级变量以隔离其他测试），定向 5 条（含既有 `TestRemoveHooksIsIdempotent` / `TestApplySelectionModeTogglesBothWays` / `TestDoubleClickUsesSystemThreshold`）全 PASS ok 6.215s。
- 生产改动只落 [hook.go](<../hook.go>) 2 处（保持 LF，2 个锚点各命中 1 次，CRLF=0）：在 `disarmChipHover()` 与 `removeHooks` 注释之间新增 `resetClickTrackingLocked()` 方法（4 行中文注释 + 3 行赋值）；`removeHooks()` 的锁块内 `liveMouseHook, liveKeyHook = 0, 0` 之后、`hookMu.Unlock()` 之前补一次调用。`lowLevelMouseProc` 的双击判定、`installHooks()` 的手柄设置、`hook.go` 其余行一个字没动，三条 characterization test 与词库 / 配置护栏无需改断言；`hook.go` 开工时 `git diff` = 0 行（干净文件），改后 29 行全部属于本批。
- 差分：`git diff -- hook.go` 改前 0 行 / 改后 29 行；改前无任何 +/- 改动行（干净文件），本批的 11 行 `+`（新增方法 10 行 + 调用 1 行）全部可逐行核对，18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.330s、定向 5 条 PASS ok 6.215s。`go test -race -count=1 ./...` **首次即通过**（exit=0、DATA RACE 计数 0），未再出现 2c / 4b / 4d / 4e / 5b / 5c / 5d 的偶发 FAIL；仍不据此断言既有跨测试全局 breaker 污染已修复。本批两测试 `-race -count=20` ok 1.532s。
- 手工走查：仍未启动 GUI，清单新增第 12 条（切换触发模式卸载 / 重装钩子后连续点击）仍待真人验证；跨回调竞态只能靠源码契约 + race detector，不是行为级复现。

### 批次 5f 验证记录（C10 截图框选会话边界收口）

- 测试先行：新增 [c10_snip_session_test.go](<../c10_snip_session_test.go>)。`TestStartNativeSnipGuardsSingleton` 用项目既有 `source_assert_test.go` 的 `readSourceFile`/`countOccurrences`/`containsText`，断言 [snip.go](<../snip.go>) 声明 `beginSnipSessionLocked` 与 `endSnipSessionLocked`、切出 `StartNativeSnip` 函数体并断言含 `beginSnipSessionLocked(` 与 `defer` 且 `currentSnip = session` 裸赋值计数为 0、`snipWndProc` 取指针仍在 `snipMu` 锁内；修复前 FAIL：`缺少 beginSnipSessionLocked：连按截图仍会覆盖全局 session`，ok 6.214s。改后追加 `TestBeginSnipSessionLocked`（走一遍会话生命周期：占位成功 / 连按被拒且不覆盖 / 错配清理不误清 / 持有者结束后可重占 / 重复清理幂等；测试用 `t.Cleanup` 保存恢复 `currentSnip` 隔离其他测试），定向两条 PASS ok 6.352s。
- 生产改动只落 [snip.go](<../snip.go>) 3 处（Python `read_bytes`/`write_bytes(s.encode("utf-8"))` 保持 LF，2 个锚点各命中 1 次，CRLF=0，无 `fastpot` 禁串）：`var` 块之后新增两个持锁纯方法（各带中文注释）；`StartNativeSnip` 的 `:463-465` 裸赋值替换为锁内 `beginSnipSessionLocked` 守卫（false 则 log + return）与 defer `endSnipSessionLocked`；`snipWndProc` / `:504-507` / 消息循环 / 裁剪写盘 / 回调一字没动。
- 差分：`git diff -- snip.go` 改前 `/tmp/r18-before.diff` = **0 行（干净文件）** / 改后 51 行，全部属本批：新增行只有两个方法、守卫与 defer 块，唯一 `-` 行是 `currentSnip = session`（被守卫替换）；其余 12 个文件 diff 行数与开工一致，18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空，新建测试与 snip.go 经 `gofmt -w`）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.400s、定向两条 PASS ok 6.352s。`go test -race -count=1 ./...` **首次即通过**（exit=0、DATA RACE 计数 0，存 `/tmp/r18-race-full.log`；本批未再出现 2c / 4b / 4d / 4e / 5b / 5c / 5d 的偶发 FAIL），仍不据此断言既有跨测试全局 breaker 污染已修。本批两测试 `-race -count=20` ok 1.507s。
- 手工走查：仍未启动 GUI，清单新增第 13 条（连按截图快捷键 / 建窗失败后重试）仍待真人验证；`snipWndProc` 的字段迁移需真实 Win32 消息泵，自动化只覆盖会话边界的纯方法与源码契约两层。

### 批次 5g 验证记录（A17 补全按钮假成功）

- 测试先行：新增 [a17_enrich_save_test.go](<../a17_enrich_save_test.go>)。`TestEnrichButtonReportsSaveFailure` 源码契约先红（断言 `EnrichAll()` 返回 `(int, error)`、补全按钮段含 `if err != nil` 与「补全失败」），FAIL 文案 `补全仍返回 int：写盘失败被吞掉，设置页会显示假的「已智能补全 N 条 ✓」`，ok 6.250s；改后追加 `TestEnrichAllReportsSaveFailure`（blocker 普通文件作父目录，日志坐实 `vocabulary save failed: mkdir ...blocker`）与 `TestEnrichAllSuccessKeepsNoError`（正常 `(1, nil)` 且落盘），两条用非 ASCII 词条 `生词` 跳过网络补全，确定性。
- 生产改动 2 文件 5 处（`_patch_5g.py`，`read_bytes`/`write_bytes` 保持 LF，各 `count==1` 断言）：`vocabulary.go` 的 `EnrichAll() int`→`(int, error)`、`return 0`→`return 0, nil`、`s.saveOrLog(); return count`→`return count, s.saveOrLog()`；`settings_vocab.go` 补全 `go func()` 改 `n, err :=` + `if err != nil { 补全失败 } else { 已智能补全 N 条 ✓ }`。`saveOrLog()` 注释的顺手改动已撤回（见 §5 的踩坑）。公开方法名不变，无其他调用点需改。
- 差分：`git diff -- settings_vocab.go vocabulary.go` 改前 `r19-before.diff` 741 行 / 改后 783 行；严格子序列比对（只取 `+`/`-` 改动行）证明改前 538 行**全部按序逐字保留 = True**，本批只多 6 行 `+`（UI 4 行 + 签名 1 行 + `return count, s.saveOrLog()` 1 行）、零 `-` 行，18 个用户改动逐字未动。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.987s、定向 3 条 PASS ok 6.279s。`go test -race -count=1 ./...` **首次 FAIL 并抓到完整堆栈**：`WARNING: DATA RACE`，`ai_generation_race_test.go:296` 的 `freshBreakers()` 与 `services.go:647` 的 `engineBreaker.record()`（由 `services.go:544` 的 `TranslateRace.func7` 调用，goroutine 创建于 `services.go:531` ← `popup_actions.go:460` 的 `prefillActionTranslation.func1`）并发写 `0x00c000116be0`，失败用例 `TestRaceCancellationIsNotAnEngineFailure/cancelled_in_flight`（`testing.go:1867: race detected during execution of test`）。堆栈无本批任何函数，判定为既有跨测试全局 breaker 污染（与 2c/4b/4d/4e/5b/5c/5d 同源），按规则不接手修复。四组换方法取证全绿：① 原样复测全套 exit=0、DATA RACE 0、ok 11.939s；② 污染用例单独 `-race -count=5` ok 5.369s；③ `-skip 'TestRaceCancellationIsNotAnEngineFailure'` 全库 `-race` exit=0、DATA RACE 0、ok 11.149s；④ 新护栏 `-race -count=20` ok 1.954s。复测通过只说明未再复现。
- 手工走查：仍未启动 GUI，清单新增第 14 条（磁盘不可写时点「补全」）仍待真人验证。

### 批次 6a 验证记录（A17 四个 go func 并发守卫）

- 测试先行：新增 [a17_vocab_job_test.go](<../a17_vocab_job_test.go>)。`TestVocabJobsUseSingleGuard` 源码契约先红（断言 settings_vocab.go 有 4 处 a.tryBeginVocabJob() 与 4 处 defer endVocabJob()、vocab_job.go 有两个方法签名），FAIL 文案 `四个生词动作都应由同一个作业守卫收口，当前只有 0 处调用 a.tryBeginVocabJob()`，ok 6.311s；改后追加 `TestVocabJobGuardTransitions`（首次成功 / 第二次拒绝 / 释放后可再 begin / 重复释放幂等）与 `TestVocabJobGuardConcurrentBegin`（24 goroutine 并发 begin 恰好 1 成功，atomic 计数 + -race）。
- 生产改动 3 文件（_patch_6a.py，read_bytes / write_bytes 保持 LF，go func 锚点 count==4 断言）：新增 [vocab_job.go](<../vocab_job.go>)（vocabJobMu + vocabJobRunning + tryBeginVocabJob() / endVocabJob()）；[settings_vocab.go](<../settings_vocab.go>) 四个 go func 开头各加守卫 + defer，对话框 / 导入 / 导出 / 补全逻辑一字不动。
- 差分：工作区在 d2fe230 提交后干净，git diff --stat 只有 settings_vocab.go +16 行（四处守卫各 4 行），另 2 个新增文件 vocab_job.go / a17_vocab_job_test.go；无需「证明没碰用户改动」的改前基线。
- 本批四条：gofmt -l .（空）、go vet ./...（clean）、go test -count=1 ./... ok 9.895s、定向 5 条（3 条新 + 2 条 5g 邻接）PASS ok 1.417s。go test -race -count=1 ./... **首次即通过**（exit=0、DATA RACE 计数 0、ok 17.604s，存 /tmp/r20-race-full.log），本批未再出现 2c/4b/4d/4e/5b/5c/5d/5g 的偶发 FAIL，仍不据此断言既有跨测试全局 breaker 污染已修；新护栏 -race -count=20 ok 1.768s。
- 手工走查：仍未启动 GUI，清单新增第 15 条（连点导入 / 导出 / 补全）仍待真人验证；四个动作要真实 mygo.Dialog，自动化只覆盖作业槽位的纯方法与源码契约两层，不臆造窗口级复现。

### 批次 6b 验证记录（X1 三套字段清单合并·第一刀）

- 测试先行：新增 [x1_tab_field_sync_test.go](<../x1_tab_field_sync_test.go>)。`TestClearTabMatchesZeroValueRestore` 是 characterization（改前即 PASS），用 `reflect.DeepEqual` 证明 `clearTabLocked()` 与 `restoreTabLocked(&tabViewState{})` 对填满 35 字段的 app 结果逐字相等，并钉住「source 保留、cancel 归零」；`TestTabFieldListsAreSynced` 是反射契约（改前即 PASS），断言 save / restore / snap 三处都覆盖各自结构体的字段、同名类型一致；`TestClearTabDelegatesToRestore` 委托契约先红（FAIL 文案 `clearTabLocked 仍自己维护一份字段清零清单；应委托 restoreTabLocked 复用同一份清单`）。
- 先红：`go test -run 'TestClearTabMatchesZeroValueRestore|TestTabFieldListsAreSynced|TestClearTabDelegatesToRestore' -count=1 -v .` → 两条 PASS、一条 FAIL，ok 1.389s。
- 生产改动 1 文件（`_patch_6b.py`，`read_bytes` / `write_bytes` 保持 LF，`old.count("a.")>30` 与 `snipBitmap = nil` 断言）：[popup_tabs.go](<../popup_tabs.go>) 的 `clearTabLocked()` 把 36 行逐字段清零整段换成 5 行中文注释 + `a.restoreTabLocked(&tabViewState{})`；`saveTabLocked` / `restoreTabLocked` / `switchMode` / 页签缓存 / 渲染逻辑一字不动。
- 差分：工作区在 d2fe230 提交后干净，`git diff -- popup_tabs.go` 只有 `clearTabLocked` 这一个 hunk（-40 行清单 +6 行注释与委托），其余文件 diff 是阶段 D 批次 6a 的遗留，无需「证明没碰用户改动」的改前基线。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.345s、定向三测试全 PASS ok 1.375s。`go test -race -count=1 ./...` **首次即通过**（exit=0、DATA RACE 计数 0、ok 18.171s，存 /tmp/r21-race-full.log），本批未再出现既有跨测试全局 breaker 污染的偶发 FAIL，仍不据此断言该污染已修；新护栏 `-race -count=20` ok 1.786s。
- 范围：X1 的「三份清单」本刀减到两份并加反射同步契约；完整合一（单一 `PopupState` struct）因 `snapshot` 含渲染期派生字段、`tabViewState` 独有 `ocrPath`、语义不同而留需真人 GUI 走查的后续。仍未启动 GUI，清单新增第 16 条（页签往返）待真人验证。

### 批次 6c 验证记录（A4 OCR 视图显式化）

- 测试先行：新增 [ocr_view_test.go](<../ocr_view_test.go>)。`TestOCRRenderUsesExplicitPhase` 是源码契约，`TestClassifyOCRView`（7 组）与 `TestOCRViewPhaseString` 钉枚举，`TestOCRRenderDecisionsMatchLegacy` 用 16 组 `busy` / `errText` / `source` / `ocrText` 组合把旧三决策与新三谓词逐项对比。
- 先红：`go test -run 'TestOCRRenderUsesExplicitPhase|TestClassifyOCRView|TestOCRViewPhaseString|TestOCRRenderDecisionsMatchLegacy' -count=1 -v .` → 源码契约 FAIL「renderScreenshotSource 仍用三个裸 if 判断 OCR 状态」，其余 PASS，ok 1.37s。
- 生产改动 1 文件 3 行（`_patch_6c.py`，`read_bytes` / `write_bytes` 保持 LF）：[ocr.go](<../ocr.go>) 的 `renderScreenshotSource` 把 `if s.ocrBusy` / `if s.ocrError != ""` / `if s.source != "" || s.ocrText != "" || s.ocrError != ""` 换成 `classifyOCRView(s) == ocrViewRecognizing` / `ocrShowErrorBox(s)` / `ocrShowEditor(s)`；新增 [ocr_view.go](<../ocr_view.go>) 承载枚举与三个纯函数。
- 差分：工作区在 d2fe230 提交后干净，`git diff -- ocr.go` 只有 3 insertions / 3 deletions 一个 hunk，其余文件 diff 是阶段 D 前几批的遗留。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.442s、定向四测试全 PASS ok 1.371s。`go test -race -count=1 ./...` **首次即通过**（exit=0、`WARNING: DATA RACE` 计数 0、ok 18.079s）。
- 范围：三块是**叠加渲染**（error 态下错误框与编辑区同时显示），不是互斥 switch；本批只把判断收成具名谓词，渲染结构、文案、回调一字未动。仍未启动 GUI，清单新增第 17 条（OCR error 态同时显示错误框与编辑区、busy 只显示 Spinner）。

### 批次 6d 验证记录（U10 Hover Tip 时间窗口谓词化）

- 测试先行：新增 [chip_tip_phase_test.go](<../chip_tip_phase_test.go>)。`TestChipTipUsesNamedPredicates` 是源码契约，`TestChipTipTimeWindowsMatchLegacy` 用 13 组时间边界对比原内联表达式，`TestClassifyChipTipPhase`（6 组）钉阶段优先级，`TestChipTipPhaseString` 钉字符串。
- 先红：`go test -run 'TestChipTipUsesNamedPredicates|TestChipTipTimeWindowsMatchLegacy|TestClassifyChipTipPhase|TestChipTipPhaseString' -count=1 -v .` → 源码契约 FAIL「warm 窗口判断仍未收进 chipTipIsWarm」，其余 PASS。
- 生产改动 1 文件 2 行（`_patch_6d.py`，`read_bytes` / `write_bytes` 保持 LF）：[selicon.go](<../selicon.go>) 的 `onChipPointer` 把 `warm := !t.hiddenAt.IsZero() && now.Sub(t.hiddenAt) <= chipTipWarm` 换成 `warm := chipTipIsWarm(t.hiddenAt, now)`、`due := !t.shown && now.Sub(t.since) >= chipTipDelay` 换成 `due := chipTipIsDue(t.shown, t.since, now)`；新增 [chip_tip_phase.go](<../chip_tip_phase.go>) 承载 `chipTipPhase` 枚举与两个纯谓词。
- 差分：工作区在 d2fe230 提交后干净，`git diff -- selicon.go` 只有 2 insertions / 2 deletions 一个 hunk。
- 本批四条：`gofmt -l .`（空）、`go vet ./...`（clean）、`go test -count=1 ./...` ok 9.323s、定向四测试全 PASS。`go test -race -count=1 ./...` **首次即通过**（exit=0、`WARNING: DATA RACE` 计数 0、ok 17.777s）；6c+6d 八条护栏 `-race -count=20` ok 2.768s。
- 范围：U10 经复核早已有 `mu` + `gen` / `shown` / `timer` / `hits` / `geom` 显式字段与 `hideChipTip()` 的 `gen++` + `win.Hide()` 兜底，**无用户可见缺陷**；本批只把两个时间窗口收成具名谓词并加阶段枚举。跨鼠标钩子 goroutine / `time.AfterFunc` / `RunOnMain` 三上下文的完整迁移表无法行为级确定性复现，真人 hover 时序仍待补（清单第 18 条）。

### 手工走查清单（阶段 A 每批、阶段 B / C 每条；6d 起为 18 条）

1. 划词 → 出现 chip → 打开 AI 卡片 → 流式期间只显示 Spinner → 收尾显示结构化卡片；
2. 翻译 ↔ 长难句 ↔ 写作 来回切 3 次，观察是否有旧结果闪回或「半成品当结果」；批次 4c 后，正在流式输出时切走再切回应重新出结果，而不是把半成品定格成最终答案；批次 4d 后，机器翻译尚未返回时切到别的模式、或划词打开新弹窗 / 手动输入弹窗，旧译文都不应闪回或写进新卡片；
3. 点「重新翻译」与 F5，观察是否真的重跑；
4. 截图 OCR → 中途取消 → 再截一次，观察旧识别结果是否写回；批次 4f 后，取消不写回由 `TestCanceledScreenshotOCRDropsLateResult` 覆盖，再截一次时上一轮 OCR 译文 / 引擎 / 词典也不应闪回或写进这张新卡片；
5. 点「AI 精翻」，观察是否打断正在进行的卡片流（批次 4b 后应不打断，卡片继续输出，精翻只替换译文）。本批未启动 GUI，这一条仍是未验证范围；
6. 词库导入一个损坏文件，观察提示文案；批次 4e 后，磁盘不可写时（只读磁盘 / 磁盘满 / 文件被锁）导入应显示「导入失败: ...」，而不是「成功导入 N 条 ✓」；
7. 设置里切换触发模式（自动 / 仅快捷键 / chip），观察是否需要重启才生效。
8. 把用户数据目录里的 config.json 改坏（截断 JSON）后启动应用，观察设置是否仍可用默认值运行、损坏文件是否原样保留、日志是否给出「Repair or delete the file to reset」指引（批次 5a 新增，自动化已覆盖 `newStore()` 这一层，真人启动仍待补）。
9. 把用户数据目录里的 vocabulary.json 改坏（截断 JSON）后启动应用，观察生词本是否仍可用（空库）运行、加两个词后 `vocabulary.json.corrupt` 是否原样保留损坏原文、日志是否给出「repair or delete the original file to reset」指引（批次 5b 新增，自动化已覆盖 `load()` 这一层，真人启动仍待补）。
10. 从托盘「退出」与从设置窗口关闭按钮两条路径分别退出应用，观察：两条路径都不应出现窗口重复关闭 / 卡死 / 竞态告警；退出中再次触发关闭不会重入（批次 5c 新增，`isQuitting` 已改 `atomic.Bool`，自动化只覆盖源码契约与并发回归，真人退出仍待补）。
11. 截图翻译（窗口临时置顶）后，从划词 chip 或托盘菜单打开一张新卡片，观察新卡片是否恢复正常置顶状态、失焦后能否被 OnBlur 关闭；用户手动点 pin 置顶后走同一路径，置顶应保留（批次 5d 新增，自动化由 `TestResetScreenshotPinLocked` 纯方法表驱动 + `TestBothPopupEntriesResetScreenshotPin` 两入口源码契约覆盖，真人点击仍待补）。
12. 在设置里把触发模式切到「仅快捷键」（卸载钩子）再切回「划词 / 自动」，连续点两次同一位置，观察是否误触发双击手势；切换后首次单击不应被当作双击的另一半（批次 5e 新增，`removeHooks()` 现复位 `lastClickTime`/`lastClickPos`/`clickCount`；自动化由 `TestRemoveHooksResetsClickTracking` 源码契约 + `TestResetClickTrackingLocked` 表驱动覆盖，真人重装钩子仍待补）。
13. 连续快速按两次截图快捷键（或在截图覆盖层还开着时再触发一次），观察是否只出现一个覆盖层、第二次是否被明确忽略；随后正常完成一次框选，再触发截图应能正常打开（`currentSnip` 已被 `defer` 清掉）。建窗失败（如远程桌面 / 锁屏）后重试截图，也应能正常打开而不是被上一次残留的全局单例挡住（批次 5f 新增，自动化由 `TestStartNativeSnipGuardsSingleton` 源码契约 + `TestBeginSnipSessionLocked` 会话生命周期覆盖，真人连按与建窗失败仍待补）。
14. 把用户数据目录设为只读（或让 vocabulary.json 所在磁盘写满），在词库设置里点「✨ 补全」，观察是否显示「补全失败: ...」而不是「已智能补全 N 条 ✓」；恢复正常后再点一次应正常显示成功（批次 5g 新增，自动化由 `TestEnrichAllReportsSaveFailure` / `TestEnrichAllSuccessKeepsNoError` 覆盖，真人磁盘不可写仍待补）。
15. 在生词本设置里快速连点两次「📥 导入」（或「✨ 补全」），观察只弹出一个文件对话框、第二次给「有生词任务正在进行，请稍候…」提示而不是并发跑两次；第一次完成后再点应能正常启动（批次 6a 新增，自动化由 TestVocabJobGuardTransitions / TestVocabJobGuardConcurrentBegin 覆盖槽位迁移，真人连点与对话框仍待补）。
16. 在翻译 / 长难句 / 写作之间来回切页签多次，每次切走前先让结果区有内容（译文、卡片、笔记），观察：切回来内容完整、不残留上一个模式的结果、正在加载时切回来会重跑（批次 4c）、`clearTabLocked` 委托后清空行为与原来一致（批次 6b）；自动化由 `TestClearTabMatchesZeroValueRestore` 的 DeepEqual 与 `TestTabFieldListsAreSynced` 反射契约覆盖，真人页签往返仍待补。
17. 截图 OCR 识别失败（如断网或识别引擎不可用）时，观察错误框与下方编辑区是否**同时出现**（可手改文字再点翻译）；识别进行中只应显示 Spinner 与取消按钮，不应同时出现错误框（批次 6c 新增，自动化由 `TestOCRRenderDecisionsMatchLegacy` 16 组组合覆盖判定，真人渲染仍待补）。
18. 把鼠标停到划词 chip 上：160ms 内移开再移回应立即显示，超过 160ms 才延迟弹出；400ms 内移开再移回算「热」窗口应立即再显示，超过 400ms 重新计时（批次 6d 新增，自动化由 `TestChipTipTimeWindowsMatchLegacy` 13 组边界覆盖，真人 hover 时序仍待补）。

---

## 8. 回滚

- 批次 0、1a：删新增文件即可。
- 批次 1b–2：用精确反向 edit 把改动的那几行改回原样；**不要** `git checkout -- <file>`，那会连工作区里你的 18 个改动一起丢掉。
- 批次 3：只改文档，删掉 §10 与 §7 的批次 3 记录即可；没有源文件可回滚。
- 批次 4a：只改 `state_mirror_test.go`，用精确反向 edit 删掉新增的两段测试块即可回滚。
- 批次 4b（已执行）：用精确反向 edit 还原 [popup.go](<../popup.go>) 新增的两字段、[popup_actions.go](<../popup_actions.go>) 的 `stopAIPassLocked` 段与 `refineWithAI` 段，删 [u1_refine_generation_test.go](<../u1_refine_generation_test.go>)，并把 `TestRefineWithAIIndependentGeneration` 的断言改回 `{8, false, "partial"}`。
- 批次 4c（已执行）：用精确反向 edit 还原 [popup_tabs.go](<../popup_tabs.go>) 的 `hasContent()`（删掉 3 行注释与 `!s.aiBusy && !s.notesBusy &&`），删 [u6_tab_loading_test.go](<../u6_tab_loading_test.go>)；三条护栏无需改动。
- 批次 4d（已执行）：用精确反向 edit 还原 [popup.go](<../popup.go>) 的三处 `a.translateGen++` 与对应注释（`switchMode`、`openPopupAction`、`openPopupWithText`），删 [u2_switch_mode_test.go](<../u2_switch_mode_test.go>)；三条护栏无需改动。
- 批次 4e（已执行）：用精确反向 edit 还原 [vocabulary.go](<../vocabulary.go>) 的 `saveOrLog()` 签名与函数体、删掉 `addBatch` / `importEntries` 两个私有方法、把 `AddBatch` / `ImportEntries` 与 6 处 `return` 改回原样，删 [a17_import_save_test.go](<../a17_import_save_test.go>)；三条护栏无需改动。
- 批次 4f（已执行）：用精确反向 edit 还原 [ocr.go](<../ocr.go>) 的接缝（`var recognizeImageFileIn = RecognizeImageFileIn` 与调用点改回 `RecognizeImageFileIn`）、删掉 `recognizeScreenshot` 锁内的 2 行注释与 `a.translateGen++`，删 [a4_ocr_session_test.go](<../a4_ocr_session_test.go>)；三条护栏与 `popup_ui_test.go` 无需改动。
- 批次 5a（已执行）：用精确反向 edit 还原 [config.go](<../config.go>) 的 `resolveConfigPath` 接缝（`newStore` 改回内联 `mygo.App.Path`）与 `newStore` 第一分支（改回无条件 `saveConfig(s.path, s.cfg)`），删 [a8_config_corrupt_test.go](<../a8_config_corrupt_test.go>)；三条护栏与配置迁移测试无需改动。
- 批次 5b（已执行）：用精确反向 edit 还原 [vocabulary.go](<../vocabulary.go>) 的 `load()` 解析失败分支（删掉 4 行注释与 `recovery := s.path + ".corrupt"` 到 `return` 前的整段），删 [a18_vocab_corrupt_test.go](<../a18_vocab_corrupt_test.go>)；三条护栏与词库既有测试无需改动。
- 批次 5c（已执行）：用精确反向 edit 还原 [popup.go](<../popup.go>) 的 `"sync/atomic"` import 与字段声明（改回 `isQuitting bool`、删中文注释）、[main.go](<../main.go>) 的 `quitApp` 三行锁块与 `OnBeforeQuit` 裸写、[settings.go](<../settings.go>) 的 `if a.isQuitting {`，删 [c5_quitting_state_test.go](<../c5_quitting_state_test.go>)；三条护栏无需改动。
- 批次 5d（已执行）：用精确反向 edit 还原 [popup.go](<../popup.go>) 的 3 处（删新增的 `resetScreenshotPinLocked()` 方法与 4 行注释、把 `openPopupWithText` 的调用改回内联 if 块与 3 行英文注释、删 `openPopupAction` Reset 段末尾的 `a.resetScreenshotPinLocked()`），删 [u8_screenshot_pin_test.go](<../u8_screenshot_pin_test.go>)；三条护栏无需改动。
- 批次 5e（已执行）：用精确反向 edit 还原 [hook.go](<../hook.go>) 的 2 处（删 `resetClickTrackingLocked()` 方法与 4 行中文注释、删 `removeHooks()` 里 `liveMouseHook, liveKeyHook = 0, 0` 之后的 `resetClickTrackingLocked()` 调用），删 [c1_click_tracking_test.go](<../c1_click_tracking_test.go>)；三条护栏与 `hook_lifecycle_test.go` / `regression_batch_test.go` 无需改动。
- 批次 5f（已执行）：用精确反向 edit 还原 [snip.go](<../snip.go>) 的 3 处（删 `beginSnipSessionLocked` / `endSnipSessionLocked` 两个方法与中文注释、把 `StartNativeSnip` 的守卫块改回 `snipMu.Lock(); currentSnip = session; snipMu.Unlock()` 三行、删 defer 块），删 [c10_snip_session_test.go](<../c10_snip_session_test.go>)；三条护栏与 `snip_test.go` / `popup_ui_test.go` / `ocr_quality_test.go` 无需改动。
- 批次 5g（已执行）：用精确反向 edit 还原 [vocabulary.go](<../vocabulary.go>) 的 3 处（`EnrichAll() (int, error)`→`EnrichAll() int`、`return 0, nil`→`return 0`、`return count, s.saveOrLog()`→`s.saveOrLog(); return count`）与 [settings_vocab.go](<../settings_vocab.go>) 补全 `go func()` 的 err 分支，删 [a17_enrich_save_test.go](<../a17_enrich_save_test.go>)；`saveOrLog()` 注释本批未动，无需还原。
- 批次 6a（已执行）：用精确反向 edit 删掉 [settings_vocab.go](<../settings_vocab.go>) 四个 go func 开头各 4 行（if !a.tryBeginVocabJob() { return } 与 defer endVocabJob()），删 [vocab_job.go](<../vocab_job.go>) 与 [a17_vocab_job_test.go](<../a17_vocab_job_test.go>)；四个动作的业务逻辑本批未动，无需还原。
- 批次 6b（已执行）：用精确反向 edit 把 [popup_tabs.go](<../popup_tabs.go>) 的 `clearTabLocked()` 还原成 36 行逐字段清零清单（去掉委托与中文注释），删 [x1_tab_field_sync_test.go](<../x1_tab_field_sync_test.go>)；`saveTabLocked` / `restoreTabLocked` / `snap()` 本批未动，无需还原。
- 批次 6c（已执行）：用精确反向 edit 把 [ocr.go](<../ocr.go>) 的三处判断还原成裸 `if`（`s.ocrBusy` / `s.ocrError != ""` / `s.source != "" || s.ocrText != "" || s.ocrError != ""`），删 [ocr_view.go](<../ocr_view.go>) 与 [ocr_view_test.go](<../ocr_view_test.go>)。
- 批次 6d（已执行）：用精确反向 edit 把 [selicon.go](<../selicon.go>) 的两处还原成内联时间表达式（`warm := !t.hiddenAt.IsZero() && now.Sub(t.hiddenAt) <= chipTipWarm`、`due := !t.shown && now.Sub(t.since) >= chipTipDelay`），删 [chip_tip_phase.go](<../chip_tip_phase.go>) 与 [chip_tip_phase_test.go](<../chip_tip_phase_test.go>)。
- 任何一批出现「测试通过但手工走查不一致」，立即回滚该批，不要用「再加个补丁」掩盖——先核对分类函数的分支顺序是否与旧渲染分支一致。

---

## 9. 常见失败模式

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 渲染基线测试变红 | 分类函数的分支顺序与旧渲染分支不一致 | 回退该点，按 popup_view.go 的 if 顺序重排分类 switch |
| 切读取点后 UI 少显示一个 Spinner | 新 `Phase` 与旧 bool 不是严格等价（例如 `aiBusy=false && aiText!=""` 的完成态） | 回退该点，先把状态集合补全再切 |
| `go vet` 报未使用字段 | 阶段 A 不删旧字段（§1 第 3 步已改为「清点」）；若阶段 B 收敛字段时漏了读取点才会出现 | 补切读取点，不要加 `_ = a.xxx` 掩盖 |
| 回滚后行为仍不同 | 说明改动落在了批次之外，或与工作区未提交改动混在一起 | 用 §6 的差分法比对改前改后的 `git diff` |
| 新增 `.go` 文件让 `TestOneNameEverywhere` 变红 | 注释里写了旧项目名 `fastpot/`；该测试扫描所有 `.go` 文件（批次 1a 实际踩到） | 注释只写文件名或相对路径，不带 `fastpot/` 前缀 |
| `go test -race ./...` 偶发 FAIL 但复测通过 | 工作区既有改动含未修复的并发读写（见 STATE_MACHINES.md C5、C10）；批次 2c 抓到完整堆栈：`ai_generation_race_test.go:296` 的 `freshBreakers()` 与 `services.go:647` 的 `engineBreaker.record()` 并发写同一块内存，属跨测试全局状态污染 | **首次 FAIL 必须重跑并把完整输出存盘**抓 `WARNING: DATA RACE` 堆栈，再判断与本批改动是否相关；原样复测一次 + 单独跑该测试 `-count=5` + 本批护栏 `-race -count=20`；复测通过只说明未再复现，不据此断言既有问题已修 |
| 手册规则与实操自相矛盾（如「无 `if` 条件改动」却把 `if` 收成 `switch`） | 初版规则按「写时镜像」写的，改成「读时派生 + 逐点切读」后失效 | 按实际落地修正规则为「只改判断形式、不改判断结果」，并保留差分与基线表作为证据（批次 1b 已修） |
| 用 Python heredoc 改 `.go` 文件后 `gofmt -d` 报整个文件都变了 | Windows 上 Python 文本模式写文件会把 LF 换行符写成 CRLF，全文件换行符与仓库不一致（批次 4d 实际踩到） | 用 `Path.write_bytes(s.encode("utf-8"))` 写回，或把 CRLF 批量还原成 LF；改 `.go` 优先用 edit 工具，它保留原换行符 |
| 审计条目写「取消后旧结果仍写回」，照着加守卫却发现代码已挡住 | 审计是静态清单，代码在上轮竞态修复里已加 `generation` 校验与 ctx 取消（批次 4f 实际踩到） | 动手前逐行读该会话的启动 / 取消 / 结果采纳三处，只修实地确认可达的缺陷；不加冗余守卫，把「已挡住」写成 characterization test 钉住规格 |
| 审计条目写「解析失败后磁盘文件未被覆盖」，照着以为无风险而跳过 | 审计是静态清单，`newStore` 的 `if !s.load()` 把「文件不存在」和「文件损坏」都当首次运行，后者用 `DefaultConfig()` 覆盖用户文件（批次 5a 实际踩到） | 动手前逐行读调用方如何使用 `bool` 返回值，尤其是被当作三态用的地方；用 `os.Stat` 之类读时派生补齐状态集合，并写「磁盘内容逐字不变」的断言 |
| 审计条目写「解析失败不覆盖原文件并提示」，照着改却破坏了「加词」功能 | 审计「显式化」建议里的 refusing-to-write 会让用户无法再写词库；`save()` 的 `.bak` 是滚动单副本，第二次写盘就覆盖掉损坏原文（批次 5b 实际踩到） | 改用加性方案：另存一份 `path+".corrupt"` 原始副本，不动 `entries` / `save()`；用「两次写盘后副本仍逐字等于原文」的断言钉住机制 |
| 想给 Top10 里的竞态写「行为级复现测试」，却发现触发概率不可控 | C5 的 isQuitting 跨 quitApp / OnBeforeQuit / OnClose 三处，mygo 消息循环把它们调度到同一线程，裸写与加锁写是否交错取决于真实回调时序；U10/C10/C1 更要真实 Win32 消息泵或钩子重装（批次 5c 实际遇到） | 不编造行为级复现；改用「源码契约断言（先红后绿）+ 改后 `-race` 并发回归」两层，并在文档里写明确定性边界。全套 race 若撞上既有跨测试全局 breaker 污染，用 `-skip` 排除该用例证明其余干净，如实记录 FAIL 堆栈与复测结果 |
| 审计写「两条入口不对等」，但两条函数都要真实 window，写不出行为级复现测试 | `openPopupAction` / `openPopupWithText` 在 `win==nil` 时提前 return，置顶复位只能在真实消息循环里触发（批次 5d 实际遇到） | 把状态迁移收进一个纯字段方法（`resetScreenshotPinLocked`），用「源码契约断言两入口都调用 + 纯方法表驱动行为测试」两层验证，不编造窗口级复现；`git diff` 差分证明两条入口之外的改动逐字未动 |
| 想「合并三份字段清单」就直接把三个 struct 合一 | `snapshot` 含 15 个渲染期派生字段（`isWord` 等）、`tabViewState` 独有 `ocrPath`，语义不同，硬合要动几十处读写点并破坏渲染（批次 6b 实地核查） | 先找逐字等价的那一份（`clear` 与零值 `restore` 是同一份清单复制两遍），用委托把三份减到两份；再用反射契约（`TestTabFieldListsAreSynced`）把剩余清单钉住，完整合一留需真人 GUI 走查的后续 |
| 审计说「C1 / C10 / U10 都需真实钩子重装，无法单测」，照着跳过整组 Top10 条目 | 审计把 C1 / C10 / U10 一起归类，但逐行读发现 C1 的会话边界落在 `removeHooks()` 这个普通 Go 函数里（非 Win32 回调），可在 `hookMu` 锁内用纯方法收口（批次 5e 实际遇到） | 动手前把「无法单测」拆到具体函数：能 headless 调用的（普通 Go 函数 + 锁内纯字段）就写「源码契约断言（先红后绿）+ 纯方法表驱动行为测试」两层；真正要消息泵或 window 的才留手工走查，并在文档写明确定性边界 |
| 审计把 C10 / U10 / C1 一起判为「需真实 Win32 消息泵，无法单测」，照着跳过 C10 | 逐行读发现 `StartNativeSnip` 里「检查 + 占位」的会话边界在消息循环之前的普通 Go 逻辑里，不需要驱动窗口（批次 5f 实际遇到）；且 U10 经复核根本无现存缺陷 | 把「无法单测」拆到具体函数：能 headless 调用的（普通 Go 函数 + 锁内纯方法）就写「源码契约（先红后绿）+ 纯方法会话生命周期」两层；`snipWndProc` 字段迁移这类真正要消息泵的才留手工走查，并在文档写明确定性边界 |
| 顺手把既有 diff 行里的注释措辞「导入链路」改成「导入与补全链路」 | 该注释是 4e 已落的 `+` 行，动它让严格子序列比对报 `before is subsequence of after: False`，破坏「用户改动逐字保留」证据（批次 5g 实际遇到） | 注释措辞不是需求就不改；撤回后重跑 gofmt/vet/定向/全套与差分，确认 538 行逐字保留 |
| 用户先让「收口」、随后又「开工」，文档里已写「刻意不做」的条目要重新动手 | 阶段 C 收口是当时的决定，用户「那就做吧开工」是新授权，二者不矛盾（批次 6a 实际遇到） | 保留原收口记录（历史决策与理由不能抹），另起阶段 D 记录新授权与顺序；工作区若已干净，就省掉差分证明，直接以 git diff 当作本批全部改动 |
| 把叠加渲染的三个 `if` 当成互斥 `switch` 收口 | `renderScreenshotSource` 的 error 分支没有 `return`，错误框与编辑区在 error 态同时显示；收成 switch 会让「改字重试」的编辑区消失（批次 6c 实地核查） | 先确认每个分支是否 `return`，再用「具名谓词 + 保持原控制流」的方式收口 |
| 给新枚举值起名 `chipTipWarm` | 与 [selicon.go](<../selicon.go>) 已有的 `chipTipWarm time.Duration` 常量撞名，编译报 `chipTipWarm redeclared in this block` 与 `mismatched types "time".Duration and chipTipPhase`（批次 6d 实际踩到） | 枚举值加 `tip` 前缀（`tipWarm` 等），与常量名区分 |

---

## 10. 阶段 A 收尾：全库读取点清点表（批次 3）

清点方法：对 `fastpot/` 下 227 个 `.go` 文件做四组全库检索
（`aiBusy` 67 处、`aiText` 61 处、`grammarResult|dictResult|writingResult` 123 处、
`classifyAIResult|classifyAIView|cardReadyForMode`），再逐条区分
「状态判断 / 内容读取 / 写入侧 / 结构体定义 / 测试夹具」。
下表只列**生产代码里读旧字段做判断**的位置。

### 10.1 已切到显式函数（阶段 A 的全部成果）

| 读取点 | 原判断 | 现判断 | 批次 |
| --- | --- | --- | --- |
| `renderAIResultState`（popup_view.go:528-556） | `if s.aiBusy` → `if s.aiText == ""` → `if !looksLikeStructuredPayload` 四分支 | `switch classifyAIResult(s)` | 1b |
| `popupView` 三 mode 分发（popup_view.go:158,169,180） | `if result != nil` → `else if s.aiBusy` 或 `s.aiText != ""` → `else` | `switch classifyAIView(s)` | 2a / 2b |
| `popupContentHeight`（popup_layout.go:88,133,141） | `s.dictResult == nil` / `s.grammarResult != nil` / `s.writingResult != nil` | `cardReadyForMode(s)` | 2c |
| `handleQuickSave`（popup_actions.go:1064,1068,1072） | 三个 `result != nil` 守卫 | `cardReadyForMode(s)` | 2c |

切换后，**唯一还读旧字段做状态判断的位置**是 `ai_pass.go` 本身：
`classifyAIResult` 读 `s.aiBusy`(57)、`s.aiText`(59,61)；
`cardReadyForMode` 读 `s.grammarResult / s.dictResult / s.writingResult`(80,82,84)；
`classifyAIView` 组合两者(96,99)。这是读时派生的收敛点，也是有意的：
旧字段仍是唯一事实来源，分类函数只读不写。

### 10.2 刻意保留（不可切，或语义不同）

| 读取点 | 判断 | 保留理由 |
| --- | --- | --- |
| popup_actions.go:810 | `if a.aiText != ""` | 在 `a.mu.Lock()` 内（:819 Unlock），判断流结束后有没有文本可解析 / 缓存；`a.snap()`（popup.go:301-314）自己要加锁，锁内调用会死锁；且不是 UI 状态分类 |
| popup_tabs.go:52-56 `hasContent()` | 9 项 OR：`target / errText / dictResult / grammarResult / writingResult / dict / gdict / aiText / isScreenshotMode` | 语义是「任意内容是否存在」，参数是 `*tabViewState` 不是 `snapshot`；换成 `cardReadyForMode` 会丢掉其余 8 项 |
| popup_view.go:545、popup_layout.go:93/134/142、popup_actions.go:1065/1069/1073、popup_view.go:160/171/182 | 读取卡片字段内容 | 内容读取，且都落在已切的守卫分支内，不是状态判断 |

### 10.3 写入侧 / 阶段 B 才动

| 读取点 | 说明 |
| --- | --- |
| popup_actions.go:467 `a.grammarResult == nil`、:470 `.Translation == ""`、:637 `a.grammarResultPrefill && a.grammarResult != nil` | `a.mu` 内构造 / 回填机器译文，是写入侧判断且带 `grammarResultPrefill` 标志；改它会影响异步回填行为 |
| popup.go:307-309 `a.snap()`、popup_tabs.go:71-76 `saveTabLocked`、:109-115 `restoreTabLocked` | X1 三份状态手工同步；`restoreTabLocked` 故意不写 `aiBusy / notesBusy`（U6），合并会改行为 |
| `aiGen / notesGen / translateGen / ocrGeneration` 的读点 | 异步结果采纳判定（U1 / U6 / A4 / C10），属阶段 B 修缺陷范围 |

### 10.4 阶段 A 完成判定

1. 四类「UI 状态分类」读取点（结果区四态、三 mode 分发、窗口高度、快捷保存守卫）
   全部收进 `ai_pass.go` 的三个纯函数，共 4 批（1b / 2a / 2b / 2c），每批有差分与基线证据。
2. 剩余旧字段读取逐条归类完毕：分类函数自身（事实来源）、内容读取、
   锁内 / OR 组合语义不同、写入侧。没有「本该切却漏切」的点。
3. 32 个写入点一个没动，用户的 18 个改动逐字未动；阶段 A 结束时行为仍是零变化。
4. 阶段 A 到此收尾。X1 三份字段合并、U1 `aiGen` 拆分、U6 gen 不自增、
   A4 取消后写回，全部留到阶段 B 批次 4。

### 10.5 阶段 B 入口清单

按 §5 批次 4，每条先写复现测试再改，单条单提交：

- U1（批次 4b 已修）：`refineWithAI`（popup_actions.go:856）原复用 `a.aiGen`，点「AI 精翻」会静默取消卡片流；现改用独立 `aiRefineGen` / `aiRefineCancel`；
- U6（批次 4c 已修）：`hasContent()` 现排除 `aiBusy / notesBusy` 的进行中页签，切回来会重跑当前模式；`restoreTabLocked` 强制置 false 与不自增 gen 经核查由 `switchMode` 的 `stopAIPassLocked` / `stopNotesLocked` 兜住，不改；
- U2（批次 4d 已修）：`switchMode` 与两个 `openPopup` Reset 段现都自增 `translateGen`，切模式或开新卡片时在飞的旧机器翻译会被丢弃，不再把旧译文写进新视图；
- A17（批次 4e 已修、5g 补齐补全路径）：`saveOrLog()` 现返回 `error`，私有 `addBatch` / `importEntries` 把它沿 `ImportFile` 上送，设置页已有的「导入失败」分支生效；5g 又把 `EnrichAll()` 改为返回 `(int, error)`，补全按钮按 err 显示「补全失败: ...」，不再无条件显示「已智能补全 N 条 ✓」；
- A4（批次 4f 已修）：审计写的「取消后旧结果写回」已被 `ocrGeneration` + `ctx` 挡住，另写 `TestCanceledScreenshotOCRDropsLateResult` 钉住该规格；实际修的是新截图不失效 `translateGen`，`recognizeScreenshot` 锁内现自增一次，上一轮 OCR 译文不再写进新卡片；

进入阶段 B 前，先为 `restoreTabLocked`、`stopAIPassLocked`、`refineWithAI`
各补一条 characterization test 固定现状（模板见 §4）。**批次 4a 已完成**：三条护栏集中在
[state_mirror_test.go](<../state_mirror_test.go>)，分别是 `TestStopAIPassLockedLegacyBehavior`
（批次 0 已有）、`TestRestoreTabLockedLegacyBehavior`、`TestRefineWithAILegacyBehavior`，
验证记录见 §7。**批次 4b 已动生产代码**：U1 已修，`TestRefineWithAILegacyBehavior` 改名为 `TestRefineWithAIIndependentGeneration` 并更新断言，另写 `TestRefineWithAIDoesNotCancelCardStream` 作复现测试（修复前 FAIL、修复后 PASS）。批次 4c / 4d / 4e / 4f 已按「修哪条就先更新对应护栏的断言，并另写复现测试」执行完，阶段 B 收尾；三条护栏逐字未动 —— 4c 的修复落在 `hasContent()`、4d 落在 `switchMode` 的代次自增、4e 落在词库持久化链路、4f 落在截图 OCR 会话，都不在 save / restore / refine 里，护栏录的正是这三者的现状，所以不需要改断言。阶段 C 已用 A8（批次 5a，配置损坏不覆盖用户文件）、A18（批次 5b，词库损坏保留可恢复副本）、C5（批次 5c，isQuitting 显式化为 atomic.Bool）与 U8（批次 5d，两入口置顶复位收口）、C1（批次 5e，钩子卸载复位双击候选）、C10（批次 5f，截图框选会话边界收口）起步：各条都落在开工时无本批改动混入的干净落点（`config.go` 整文件 diff=0；[vocabulary.go](<../vocabulary.go>) 的 `load()` 区间 diff=0 命中），不碰 18 个用户改动，按「一条一复现测试、一条一提交」验证完。阶段 C 收口（用户明确「做到成熟工具软件的程度就可以，不要无止境一直做」）：剩余 X1（三套字段清单合并）、A17 残余（四个 `go func()` 的 `VocabJob` 并发守卫）、A4 的 `ocrBusy`/`ocrError` 结构性显式化、U10 的 `ChipTipPhase` 枚举化，经复核要么无用户可见缺陷（4f 已证 A4 两条审计判断不成立、5f 已证 U10 已有 `gen++` + `win.Hide()` 兜底），要么属大重构且需用户先提交 / stash 那 18 个改动（X1），均刻意不做。Top10 中 9 条已有修复或复核结论，56 条清单与显式化方案已全部交付，功能无回归、三类体验（UX / DX / AX）已按可确定性验证的最局部路径改善。（用户随后「那就做吧开工」重新授权这四项，见 §0 阶段 D 与 §5 批次 6a；已完成 6a；6b 用 X1 的 clear / restore 等价性把三份清单减到两份并补反射同步契约，零行为变化；6c 把 A4 的 OCR 三态收成具名谓词，6d 把 U10 的两个时间窗口收成具名谓词并加阶段枚举，均零行为变化，阶段 D 四项到此全部完成。）

---

## 11. 后续缺陷修复记录

阶段 A–D 收尾后继续做真实缺陷修复，节奏不变：一条一复现测试、一条一提交，提交前跑
`gofmt -l .`、`go vet ./...`、`go test -count=1 ./...`、`go test -race -count=1 ./...`。

| 批次 | 缺陷 | 行为变化 | 记录 |
| --- | --- | --- | --- |
| 7a | A20 词库分类：计数器、筛选、行标签与导出网页各有一份规则，写作条目词数多时被标成长难句；`grammar` / `dict` / `notes` 短条目还会同时出现在两个 Tab | 有（修 bug）：分类收敛到 `classifyVocabEntry` 一处，先看 Source 再看词数 | 见下 |

### 批次 7a 验证记录（A20 词库分类统一）

- 复现测试：`vocab_category_test.go` 的 `TestVocabCategorySurfacesAgree`，遍历 6 种条目检查
  （1）每个条目只出现在一个 Tab，（2）`CategoryCounts` 与 `Filter` 结果一致，
  （3）`ExportHTML` 每张卡的 `data-cat` 与 `classifyVocabEntry` 一致；修复前第三条失败。
- 另加 `TestClassifyVocabEntry`（10 组 source × 词数）、`TestVocabBadgesUseCanonicalClassifier`
  （源码契约，防止行标签与导出再次长出各自的规则）、`TestVocabCategoryKeys`。
- 生产改动：新增 `vocab_category.go`（`vocabCategory` 枚举 + `classifyVocabEntry` 唯一实现 +
  `vocabCategoryFromKey`）；`vocabulary_export.go` 的 `matchesCategory` / `CategoryCounts` /
  `ExportHTML` 改为调用它，并把分类数组 `allCats` 传给页面脚本，Anki 导出不再自己判词数；
  `settings_vocab.go` 的行标签改为 `switch classifyVocabEntry(entry)`。
- 行为边界：`Source` 为 `writing` / `grammar` / `dict` / `notes` 时以 Source 为准；其余
  （`import`、空、未知）按词数 > 2 归长难句，否则归生词。
- 未验证：未启动 GUI 查看生词本四个 Tab 的真实渲染；Excel/Anki 文件未导入第三方工具复核。

