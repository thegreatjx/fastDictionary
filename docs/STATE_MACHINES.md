# fastDictionary 隐式状态机清单与显式化方案

审计对象：`fastpot`（Go 1.27.3 + mygo 原生 UI 的 Windows 划词翻译工具 fastDictionary）。
审计方式：只读源码，按模块并行通读，逐处核对状态载体、迁移触发与测试覆盖。
本次只产出清单与方案，**未修改任何运行时代码**（原因见 §1.3）。

---

## 1. 范围与结论

### 1.1 纳入范围

`fastpot/*.go` 中的非测试源码，以及随应用分发的 `resources/ocr.ps1`。

### 1.2 排除范围

| 路径 | 排除理由 |
| --- | --- |
| `pot-desktop/`、`read-frog/` | 克隆的第三方开源项目，用于功能对比与借鉴，不是本应用 |
| `dist/`、`build/` | 构建产物与中间目录 |
| `review-bundles/`、`.research/`、`.magpie-*` | 历史评审材料与调查归档 |
| `*_test.go` | 作为行为证据与覆盖情况引用，不作为状态机主体 |

### 1.3 为什么本次不改运行时代码

- 审计时工作区有 18 个已修改未提交的源文件，属于用户正在进行中的改动；在其上做状态机重构会与未提交改动冲突。
- 多个隐式状态机（代次计数器、缓存开关、prefill 标记）是上一轮专门针对竞态和数据错乱修复时**刻意引入**的，并有 `optimization_regression_test.go`、`ai_generation_race_test.go` 等回归测试覆盖。贸然重写风险高于收益。
- 用户要求「不破坏任何现有功能」。因此本次交付是**完整清单 + 可验证的零风险改进路径**；落地实施按 §6.2 的分阶段路径单独进行，每一步都跑现有测试。

### 1.4 一句话结论

本应用不是「没有状态机」，而是有**一个显式枚举（`popupMode`）+ 几十个布尔标志与代次计数器组成的隐式状态机群**，状态被复制在 `app`、`snapshot`、`tabViewState` 三处手工同步。开发者能靠注释读懂，智能体必须逐个文件推断，用户则承担状态不同步时的可见错乱。

本次共列出 **56 个条目**：三组并行审计报告 52 个（`C` 11 / `U` 17 / `A` 24），另补 4 个跨模块结构性根因（`X1`–`X4`）；风险分布为高 18 / 中 35 / 低 3。按状态对象归并后约 **40 余个独立隐式状态机**。全局风险 Top 10 见 §5.1，机器可读索引见 §8，落地路径见 §6.2。

---

## 2. 判定标准：什么算隐式状态机

同时满足以下三条即计入清单：

1. **有状态**：某个变量/字段的取值组合决定系统「当前处于哪一步」以及「哪些操作合法」。
2. **有迁移**：事件（快捷键、鼠标、定时器、窗口消息、goroutine 回调、网络返回）会改变它。
3. **没有显式抽象**：没有专门的状态类型/枚举，没有集中声明的合法迁移表；状态靠散落在多个函数里的 `a.xxx = true/false`、`gen++`、`if x && !y` 维护，且常常多个 bool 共同表达同一个「阶段」。

典型形态（本项目里都出现了）：

- **布尔标志组合**：`busy / aiBusy / notesBusy / ocrBusy / aiRefining / loadingModels / isQuitting …`，互斥关系只存在于开发者脑中。
- **代次计数器**：`aiGen / notesGen / translateGen / ocrGeneration`，用「数字不等」隐式表达「旧结果作废」，而不是一个 `cancelled` 状态。
- **nil / 空串哨兵**：`aiCancel == nil`、`ocrPath == ""`、`vocabExpandedID == ""`、`pausedUntil.IsZero()` 表示「未开始／已结束」。
- **时间窗口状态**：`pausedUntil` 用「当前时间是否早于某个时刻」表达暂停态，状态本身不可枚举、不可序列化。
- **跨结构体复制**：同一份状态在 `app`（真值）、`snapshot`（渲染快照）、`tabViewState`（页签缓存）里各存一份，靠 `saveTabLocked` / `restoreTabLocked` / `snap()` 手工对齐。

不计入：纯配置开关（`Config` 里的 `AIEnabled` 等）、纯算法局部标志、无状态的解析/渲染函数、单向线性的 `resources/ocr.ps1`（只有语言选择分支，无状态迁移）。

---

## 3. 全局图景：一个应用，三份状态

`fastpot/popup.go:57-249` 的 `app` 结构体是整个客户端的真值源，约 100 个字段，其中 40 余个是状态标志。它没有「状态机」类型，只有 `sync.Mutex` + 手工置位。

同一份状态被复制到两处：

- `fastpot/popup.go:251-299` 的 `snapshot`：`snap()` 在锁内逐字段拷贝，供渲染读取。
- `fastpot/popup_tabs.go:14-50` 的 `tabViewState`：`saveTabLocked()` / `restoreTabLocked()` 在切换页签时逐字段保存与恢复。

三处的字段集合并不完全一致（例如 `snapshot` 有 `isWord`，`tabViewState` 没有；`tabViewState` 有 `ocrPath`，`snapshot` 没有），同步靠人肉维护。这是隐式状态机最典型的放大效应：**加一个状态要改三处，漏一处就出现「页签切回来内容不对」这类用户可见问题**。

`switchMode`（`fastpot/popup.go:398-454`）事实上是这个状态机的迁移中心，但它同时承担「校验 AI 是否启用」「保存旧页签」「停止异步任务」「恢复缓存」「重跑请求」五件事，没有一张声明式的迁移表。

---

## 4. 隐式状态机清单

编号规则：`C` = 核心生命周期与输入，`U` = Popup UI 与悬浮胶囊，`A` = AI / OCR / 设置 / 词库，`X` = 跨模块。
风险等级：**高**＝数据竞争、桌面级影响或已发生过的用户可见错乱；**中**＝竞态窗口、状态语义失真；**低**＝可读性/维护性为主。

### 4.1 核心生命周期与输入（C1–C11）

#### C1 鼠标/键盘钩子手势识别 · 高
- **位置**：`fastpot/hook.go:27-99, 148-290, 298-350, 363, 446`；`fastpot/selection_policy.go:89-107, 140-187`
- **载体**：`dragging bool`(hook.go:58)、`dragStart/dragEnd winPoint`(59-60)、`clickCount int`(65)、`pressTime uint32`(68)、`pressAt/pressLastKey/lastKeyAt time.Time`(73-78)、`keyboardSelecting bool`(363)、`chipHoverArmed uint32`(446)、`hookMu sync.Mutex`(57)、atomic 计数器 `hookEvents/hookDowns/hookUps/hookDrags/hookRejectedDrags`(33-41)、回调 `OnSelectionGesture/OnMouseDown/OnMouseMove/OnKeyDown`(85-98)
- **状态**：空闲 / 拖拽中 / 双击候选 / Shift+方向键选择中 / 打字时钟已启动 / chip 在屏（`chipHoverArmed`）
- **迁移**：`wmLButtonDown`(169-202) 空闲→拖拽中并置起点；`wmMouseMove`(203-208) 更新 `dragEnd`；`wmLButtonUp`(209-241) 拖拽中→空闲、`dragIsSelection` 通过则 fire；`wmRButtonDown/wmMButtonDown`(242-244) →空闲；`lowLevelKeyProc`(298-350) 置/清 `keyboardSelecting`；`armChipHover`(449)/`disarmChipHover`(452) 由 `selicon.go:259/448` 驱动
- **隐性**：11 个包级变量 + 4 个回调 + mutex/atomic 混用共同表达「当前手势」，无 `GestureState` 类型
- **风险**：`clickCount` 双击触发后不复位，`installHooks`/`removeHooks` 重装不清 `lastClickTime/lastClickPos`，重装后首次点击可能误判双击；`chipHoverArmed` 漏复位会让 `hook.go:257` 为每次鼠标移动起 goroutine（注释自述每秒上千次）；两套同步语义并存。`selicon_tip_race_test.go` 已证明这类跨文件不同步真实存在
- **显式化**：`gesturePhase`（`idle/pressed/dragging/keyboardSelecting/doubleClickPending`）+ `GestureFSM.Apply(ev gestureEvent, evData) (fire bool, g selectionGesture)` 唯一入口；`chipVisibility atomic.Bool` 由 `ChipController.SetVisible` 单点写入
- **测试**：`hook_test.go:14/34/54/69/99`、`gesture_test.go:18-354`、`selection_policy_test.go:429/518`。缺口：`clickCount` 跨重装、右键取消、`chipHoverArmed` 漏复位
- **复核备注（批次 5e）**：本条风险列的「installHooks/removeHooks 重装不清 lastClickTime/lastClickPos，重装后首次点击可能误判双击」经复核属实且可确定性修复，但落点与审计「需真实钩子重装无法单测」的判断不同：`removeHooks()` 是普通 Go 函数（非 Win32 回调），已在 `hookMu` 锁内用纯方法 `resetClickTrackingLocked()` 收口「卸载 = 手势会话结束」的复位，双击判定（`clickCount++` / `clickCount == 2`）一字不动、零行为变化；`hook.go` 开工时 diff=0。新增 `c1_click_tracking_test.go`（`TestRemoveHooksResetsClickTracking` 源码契约先红后绿 + `TestResetClickTrackingLocked` 表驱动覆盖残留 / 幂等），既有 `hook_lifecycle_test.go` / `regression_batch_test.go` 未受影响。`clickCount` 双击触发后不复位、`chipHoverArmed` 漏复位、两套同步语义（C1 结构性显式化 `gesturePhase` / `GestureFSM` / `chipVisibility atomic.Bool`）仍留阶段 C 后续；C10 的 snip.go 无锁字段与 U10 的 Hover Tip 三执行上下文因需真实 Win32 消息泵 / window，仍未动。详见 [STATE_MACHINES_FIX_PLAYBOOK.md](STATE_MACHINES_FIX_PLAYBOOK.md) §5 / §7。

#### C2 低层钩子生命周期与命令投递 · 高
- **位置**：`fastpot/hook.go:395-477, 484-581, 583-617`；`fastpot/main.go:99-131, 325-327`
- **载体**：`hookOnce sync.Once`(52)、`mouseHook/keyHook uintptr`(53-54)、`liveMouseHook/liveKeyHook uintptr`(50-51)、`hookThreadOnce sync.Once`(485)、`hookLoopID uintptr`(486)、`doubleClickMSLive uint32`(490)、`hookCommandMu sync.Mutex`(548)、`hookCommandFn func()`(549)、`const wmHookCommand = 0x8000 + 1`(544)
- **状态**：`mouseHook==0 && hookLoopID==0` 未安装 / `mouseHook!=0` 已安装 / `hookLoopID!=0` 线程在跑 / `hookCommandFn!=nil` 有待执行命令
- **迁移**：`installHooks`(395-441) 装钩子并缓存 `doubleClickMSLive`；`removeHooks`(461-477) 句柄清零后 Unhook；`hookLoop`(583-611) 收 `wmHookCommand`→`runHookCommand`；`postHookCommand`(556-570) 写单槽 `hookCommandFn` 并 PostThreadMessage；`runHookCommand`(573-581) 取走并置 nil；`quitApp`(108-110) `postHookCommand(removeHooks)` 失败则直接 `removeHooks()`
- **隐性**：「已安装」用句柄是否为 0、「线程就绪」用 loopID 是否为 0、「有待办」用 `func()` 是否为 nil 表达
- **风险**：`hookCommandFn` 是**单槽**，连续两次 `postHookCommand`（快速切模式 + 退出）第二个覆盖第一个，命令永久丢失；`removeHooks` 把 `live*` 置 0 但 `lowLevelMouseProc`:150/288 仍可能持旧 handle 调 `CallNextHookEx`（`hook_test.go:14` 即为此 bug 而存在）；`main.go:109` 对任意 error 都直接 `removeHooks()`，与「只有钩子线程能卸载」的注释(hook.go:456-459)矛盾
- **显式化**：`hookState`（`stopped/starting/running/stopping`）+ `HookRuntime{ mu; state; loopID; mouse,key uintptr; cmds chan func() }`，用带缓冲 `chan func()` 替代单槽；`Post(fn)` 区分 `ErrHookThreadNotRunning` 与 `ErrPostFailed`；`doubleClickMSLive` 收进 `doubleClickThresholdCache`
- **测试**：`hook_lifecycle_test.go:16/42/68`、`hook_test.go:14/34`、`regression_test.go:104`。缺口：并发 `postHookCommand` 覆盖、Post 失败分支、`quitApp` 降级分支
- **复核备注（批次 7d）**：单槽覆盖属实。已把 `hookCommandMu + hookCommandFn` 换成 `hookCommands = make(chan func(), 16)`，`postHookCommand` 非阻塞入队、`runHookCommand` 排空按序执行。`HookRuntime`/`hookState` 结构体化与 `Post` 错误分类未做，留后续。

#### C3 划词触发模式 / 暂停 / 应用拦截 · 中
- **位置**：`fastpot/main.go:75-96, 155-196, 321-327`；`fastpot/tray.go:76-91`；`fastpot/selection_guard.go:22-127`；`fastpot/selection_policy.go:221-262`
- **载体**：`Config.SelectionTrigger string`（裸字符串 `"icon"/"auto"/"hotkey"`，main.go:85-89/183-195/325、tray.go:80-90）；`pausedUntil time.Time`(`popup.go:158`，selection_guard.go:67/75/87/99)；`a.gestureGate gestureGate`(`popup.go:214`)；`Config.SelectionBlocklist`；`appsWhereDragIsNotSelection map[string]bool`(capture.go:332-358)
- **状态**：模式三态字符串；`pausedUntil.IsZero()` 未暂停 / 未过期 暂停中 / 已过期 逻辑恢复但旧值仍在；拦截理由 `"paused"`/`"blocked:"+owner`/`"blocked-foreground:"+fg`/`""`
- **迁移**：`selectionModeItem`(183-195) 与 `trayFlyoutView` Segmented(tray.go:86-90) 各写一次 mode；`pauseSelection`(75-84) 设 `pausedUntil=now+30min` 并 `go a.rebuildTray()`；`resumeSelection`(87-96) 清零、`gestureGate.reset()`、`go rebuildTray()`；`selectionPausedUntil`(99-106) 惰性判断；`shouldIgnoreSelection`(111-127) 组合 pause + blocklist + 前台进程
- **隐性**：模式用字符串、暂停用时间零值哨兵、三处独立入口能改 mode，无显式 `Paused` 布尔
- **风险**：`pauseSelection` 未调 `gestureGate.reset()`（只有 `resumeSelection` 调），而 `selection_policy.go:256-257` 注释承诺「a pause or a mode change does not leave a cooldown behind」——**mode 切换与暂停开始两条路径都没 reset**，残留 300ms 冷却吞掉首个手势；`pauseSelection`/`resumeSelection` 各起一个 `go rebuildTray()`，无序导致托盘标签停留在旧状态；`tray.go:86-90` 切 mode 不 `rebuildTray()`，与 main.go:75-77 注释不符，托盘 radio 与 flyout Segmented 可能不一致
- **显式化**：`type SelectionTrigger string` + 三个常量；`TriggerGate{ trigger; pausedUntil; paused bool; gate gestureGate }`，`SetTrigger/Pause/Resume` 单点同时更新三者并触发 `rebuildTray`；`IgnoreReason uint8` 替代字符串
- **测试**：`gesture_test.go:122/168/189/206/219/236/257`、`selection_policy_test.go:334`、`regression_test.go:104`、`hook_lifecycle_test.go:68`、`selicon_test.go:143`。缺口：pause 未 reset gate、flyout 切换后托盘 radio 一致性、两个 `go rebuildTray()` 乱序
- **复核备注（批次 7h）**：暂停与切换模式不清手势冷却属实，托盘浮层也不重建托盘。已在 `pauseSelection`、`applySelectionMode` 统一 `gestureGate.reset()`，并在浮层 `OnChange` 补 `rebuildTray()`；`SelectionTrigger` 类型化、`TriggerGate` 结构体化与 `IgnoreReason` 枚举未做，留后续。

#### C4 gestureGate 手势去重与冷却 · 中
- **位置**：`fastpot/selection_policy.go:56-65, 221-262`
- **载体**：`type gestureGate struct { mu sync.Mutex; fired bool; at time.Time; x, y int32 }`(226-231)；`const gestureCooldown = 300 * time.Millisecond`(59)、`sameSpotCooldown = time.Second`(64)、`sameSpotRadius = 8`(65)
- **状态**：`fired=false` 初态 / `fired=true`+`at/x/y` 已接受一个手势；拒绝理由 `"cooldown"`/`"same spot"`/`""`。**`fired` 一旦为 true 永不自动回 false**，冷却靠 `now.Sub(g.at)` 现算
- **迁移**：`allow(now,x,y)`(237-254)：300ms 内拒绝 cooldown、同点（8px）1s 内拒绝 same spot，否则记录并允许；`reset()`(258-262) 是唯一复位路径
- **隐性**：「是否处于冷却」不是状态，而是 `fired` 布尔 + 时间戳现算
- **风险**：`reset()` 全仓仅 `selection_guard.go:93 resumeSelection` 一处调用，模式切换/暂停/窗口关闭都不复位，长时间运行后 `fired` 恒 true、状态语义失真；`at` 由调用方传入，非单调或零值时钟使 `since < 0` 绕过冷却
- **显式化**：`gateState`（`idle/accepted`）+ `AcceptedGesture{at,x,y}` + `GestureGate.Allow(...) (bool, gateRejectReason)`，`gateRejectReason` 枚举化；保留三个常量名
- **测试**：`selection_policy_test.go:247/334/374`、`gesture_test.go:168`。缺口：`fired` 长期不复位、非单调/零值时钟、暂停开始未 reset

#### C5 窗口与浮层显隐 / 退出 · 高
- **位置**：`fastpot/main.go:99-163, 285-329`；`fastpot/tray.go:19-38, 55-59, 68-70, 106-108, 121-123`
- **载体**：`win/cfgWin/tray/trayFlyout/moreFlyout *mygo.Window`(`popup.go:93-96, 138`)、`sel selState`(208)、`chipTip chipTipState`(211)、`pinned bool`(98)、`isQuitting bool`(155)、`buildStamp`(main.go:30)、`registerHotkeysFunc`(32)、`hotkeyMsg string`
- **状态**：句柄 nil/非 nil + `IsDestroyed()` 共同表达显隐；`pinned=true` 失焦不隐藏；`isQuitting=true` 退出中
- **迁移**：`quitApp`(99-131) `a.mu.Lock(); a.isQuitting = true`(100-102)→停音频、卸钩子、逐个 Close/Destroy→`App.Exit(0)`；`mygo.App.OnBeforeQuit`(286-288) **`a.isQuitting = true`（无锁）**；`win.OnBlur`(303-312) 按 `pinned`+`CloseOnBlur` 决定 Hide；`toggleTrayFlyout`(20-38) 按 `!=nil && !IsDestroyed()` 开关；四处按钮回调(tray.go:55-59/68-70/106-108/121-123)各自手动 `a.trayFlyout.Close()`
- **隐性**：无 `WindowSetState`/`QuitState`；显隐靠「指针非空 + IsDestroyed() + OnClosed 回调」三处手工维护
- **风险**：**`isQuitting` 同一字段两处写入，一处加 `a.mu`(quitApp:100-102)、一处不加(OnBeforeQuit:287)，构成明确数据竞争**；`trayFlyout` 生命周期由 7 处维护，任一 `Close()` 与 `IsDestroyed()` 之间重入即重复关闭或漏置 nil；`OnBeforeQuit` 的参数 `e *mygo.QuitEvent` 未使用，无法区分退出来源
- **显式化**：`windowSetState`（`hidden/popupShown/flyoutShown/settingsShown/quittable`）+ `WindowSet{ mu; state; popup,settings,flyout,more *mygo.Window; pinned bool; quitting atomic.Bool }`，`ShowPopup/HidePopup/ToggleFlyout/Quit` 单一入口；`isQuitting` 改 `atomic.Bool` 并删 OnBeforeQuit 裸写；`quitPhase`（`running/drainingHooks/closingWindows/done`）
- **测试**：`main_test.go:18/32/48/66/76/101/183`、`ux_walkthrough_test.go:208`、`selicon_tip_race_test.go:21/54/87`。缺口：`isQuitting` 双写竞态、flyout 多处 Close 一致性、`OnBeforeQuit` 与 `quitApp` 并发
- **复核备注（批次 5c）**：`isQuitting` 的三套同步语义经复核属实（quitApp 加锁写、OnBeforeQuit 无锁写、设置窗口 OnClose 无锁读），已按 §6 建议把字段改为 `atomic.Bool` 并统一为 `Store(true)`/`Load()`、删掉只保护该 bool 的锁块；`main.go:37-40` 注释称回调在 window message-loop thread 同步调用，故竞态能否真实触发存疑，但同一字段三套同步语义本身就是审计点名的隐式状态机。Store/Load 与原读写严格等价，零行为变化。新增 `c5_quitting_state_test.go`（源码契约测试先红后绿 + 64 组并发 `-race` 回归）；`go test -race ./...` 偶发 FAIL 的既有跨测试全局 breaker 污染与本批无关，已用 `-skip` 排除法证明其余干净。`windowSetState` / `quitPhase` 枚举化与 flyout 多处 Close 一致性仍留阶段 C 后续，见手册 §5 / §7。

#### C6 popup 卡片复合状态（布尔矩阵 + 请求代次 token） · 高
- **位置**：`fastpot/main.go:348-537, 553-586`；`fastpot/popup.go:23-54, 57-170, 208-217, 289, 310, 522, 593`；`fastpot/actions.go:382-520, 579-721`
- **载体**：`type popupMode int`(popup.go:23) + 六个常量(25-32) + `const modeSelection = modeTranslate`(34)；`mode popupMode`(61)；`busy/aiBusy/ocrBusy/notesBusy bool`(68/73/88/101)；`aiCancel context.CancelFunc`(74)、`ocrCancel`(91)、`notesCancel`(106)；`aiGen uint64`(78)、`notesGen uint64`(105)、`translateGen uint64`(163)、`ocrGeneration uint64`(90)；`sourceExpanded/snipExpanded/notesExpanded bool`(82-84)；`notesSaved/grammarSaved/writingSaved bool`(152-154)；`skipAIResultCache bool`(122)；`isScreenshotMode bool`(159)；`errText/notice string`(66-67)
- **状态**：`popupMode` 有显式枚举（本应用唯一接近显式的部分），但「卡片处于什么阶段」没有；四个 busy × 四个 Gen × 七个 saved/expanded 共同表达加载中/流式/有结果/出错/已保存/已展开/已被新请求取代；`xxxCancel==nil` 表示无在途请求，`xxxGen` 不匹配表示陈旧结果应丢弃
- **迁移**（core 组可见部分）：`main.go:484-529` demo 分支在 `a.mu.Lock()` 内直接写 `a.mode/a.source/a.target/a.activeActionID/a.grammarResult/a.writingResult`；`main.go:453-459` 写 `a.sel.text` 后 `showSelIcon`；`main.go:476-482` 调 `openPopup`/`translate`/`openPopupWithText`；`popup.go:522/593` 复位 `isScreenshotMode/snipExpanded`；`actions.go:387/496/694` 渲染期读 `saved` 派生状态。完整迁移中心见 §4.4 X1
- **隐性**：不变式（如 `busy && errText==""`、`aiBusy` 时 `aiCancel != nil`）只存在于各函数脑中
- **风险**：`main.go:484-529` demo 绕过正常迁移直接写核心字段（在锁内但无代次递增、无 busy 复位），证明字段可被任意组合篡改而无校验；`xxGen` 与 busy 布尔并存，是否丢弃陈旧结果取决于每个回调是否记得比对——`ai_generation_race_test.go:55/201`、`atomic_save_test.go:178` 正是为「陈旧答案覆盖新卡片」补的测试；`notesSaved/grammarSaved/writingSaved` 拆成三个标志是布尔拆分不彻底的历史遗留（popup.go:148-151 注释自述合并前会串卡）
- **显式化**：`CardPhase`（`idle/translating/streamingAI/ready/error`）+ `cardSnapshot` 收拢布尔矩阵；`type requestToken uint64` + `generationCounter{ n atomic.Uint64 }` 统一四个代次；`func (a *app) transition(to CardPhase, tok requestToken) bool` 作为唯一写入口（demo 分支也走它）
- **测试**：`main_test.go:48/76`、`ai_generation_race_test.go:55/201/314/374`、`atomic_save_test.go:139/162/178`、`optimization_regression_test.go:128/146/161/219`、`popup_streaming_test.go:66`、`chip_states_visual_test.go:18`、`ux_walkthrough_test.go:14/85/123/144/585/629`、`actions_test.go:191`。缺口：demo 直写字段路径、四 busy 组合不变式、`skipAIResultCache` 生命周期
- **复核备注（批次 8a）**：五个忙碌标志的 OR 散落属实，已抽出 `cardActivityOf` / `cardHasPendingWork` 统一判定，`tabViewState` 补记 `busy`/`ocrBusy` 让加载中的页签不进恢复缓存；`skipAIResultCache` 提前返回泄漏已修（入口消费 + 只为 AI 模式置位）。
- **复核备注（批次 8b）**：五个代次已改为具名 `requestToken`，跨代次比较由编译器拦截。完整的 `CardPhase` 状态机与 `transition` 唯一写入口仍未做，留后续。

#### C7 语音播放代次与 MCI 播放器 · 中
- **位置**：`fastpot/audio.go:22-27, 58-100, 107-111, 131-187, 223-252`
- **载体**：`audioMu sync.Mutex`(25)、`curAlias string`(26)、`speakMu sync.Mutex`(108)、`speakGeneration int`(109)、`speakCmd *exec.Cmd`(110)、`const alias = "fastDictionary_snd"`(67)
- **状态**：`curAlias==""` MCI 空闲 / 非空 正在播放；`speakGeneration` 当前请求代次；`speakCmd==nil`/非 nil SAPI 子进程在跑；`gen == speakGeneration` 结果是否仍当前
- **迁移**：`playMP3`(59-75) 先 stop/close 旧 alias 再 open 新 alias 置 `curAlias`(71)；`stopAudio`(83-100) `speakGeneration++`、`speakCmd` 置 nil、kill 子进程、清 `curAlias`；`PlaySpeechLang`(131-187) `speakGeneration++; gen := speakGeneration` 后 goroutine 内三处 `gen == speakGeneration` 判定(166/181/244)；`speakWithSAPI`(223-252) Start 后再判 gen，符合才 `speakCmd = c`(249)
- **隐性**：无 `SpeechSession`/`PlaybackState`，「当前该播谁」靠 int 代次 + 两个互斥量 + 固定别名字符串
- **风险**：**代次检查与播放之间存在竞态窗口**——`audio.go:165-174` 在锁内确认 `current := gen == speakGeneration` 后解锁再调 `playMP3(path)`，而 `playMP3` 不接收 gen；确认后、播放前另一次 `PlaySpeechLang` 递增代次并播放，旧 goroutine 仍会播旧音频覆盖新音频。180-185 SAPI 分支同理；`stopAudio` 无法撤销已进入 `playMP3` 的播放；`curAlias` 固定常量，mciCall 任一步失败可能残留旧值
- **显式化**：`speechState`（`idle/synthesising/playing/cancelled`）+ `SpeechSession{gen,ctx,cancel,cmd}` + `SpeechController{mu;current;state;mciAlias}`；`playMP3` 改为 `func (c *SpeechController) play(path string, gen uint64) error`，在持锁同一临界区内完成 `IsCurrent(gen)` 判断 + stop/close/open/play
- **测试**：`tts_test.go:12/55/77/98`、`live_probe_test.go:23`、`vocab_speak_test.go:32/58`。缺口：代次竞态窗口、并发 `playMP3`、`stopAudio` 与 `PlaySpeechLang` 交错、`curAlias` 失败残留（现有多为 live/需网络）
- **复核备注（批次 7c）**：代次窗口属实，且有道单词快路径连代次检查都没有。已新增 `playSpeechMP3(gen, path)`，把「是否仍是最新请求」的判定与 `playMP3` 调用放进同一 `speakMu` 临界区，两条播放路径统一走它；`playMP3Func` 只作为测试接缝。`SpeechController`/`speechState` 完整枚举化未做，留后续。

#### C8 Edge TTS token 缓存 · 中
- **位置**：`fastpot/tts.go:68-88, 115-165, 188-205, 356-420, 437-452`；`fastpot/main.go:331-334`
- **载体**：`type edgeTTSAuth struct { token string; region string; expiresAt time.Time }`(74-78)；`edgeTTSMu sync.Mutex`(81)、`edgeTTSAuth_ edgeTTSAuth`(82)、`const edgeTTSTokenSlack = 3 * time.Minute`(70)
- **状态**：`token==""` 空/失效 / 非空且 `now < expiresAt-3min` 有效 / 临近过期需刷新 / 401 后复位为零值结构体
- **迁移**：`edgeTTSGetAuth`(190-205) 有效直接返回，否则 `edgeTTSFetchAuth(ctx)`(197) 成功后写回(202)；`fetchEdgeTTSFormat`(392-398) 401/403→`edgeTTSAuth_ = edgeTTSAuth{}` 复位并返回 `"edge tts: token rejected (http %d)"`；`warmEdgeTTSAuth`(442-452) 启动预热；`jwtExpiry`(170-186) 从 JWT exp 推 `expiresAt`
- **隐性**：「缓存是否有效」由 `token != ""` 判空 + 时间表达式现算，无 `TokenCacheState`
- **风险**：`edgeTTSGetAuth` **先解锁(193)再发网络请求(197)**，warmup 与多次朗读会并发判定过期并同时 fetch，后写覆盖先写（惊群，无 singleflight）；401 复位**无代次保护**，旧 token 的慢请求返回 401 会清空刚写入的新 token，导致下次朗读必然多付一次 650ms 握手；失败时不区分「网络失败」与「服务拒绝」
- **显式化**：`tokenCacheState`（`empty/fresh/expiring/rejected`）+ `edgeTTSAuthCache{mu;state;auth;gen;group singleflight.Group}`；`Get(ctx)` 用 singleflight 合并 fetch；`Invalidate(gen uint64)` 只有 `gen == c.gen` 才复位；`authFailureKind`（`network/rejected/malformed`）供 UX 分层提示
- **测试**：`tts_test.go:12/55/77/98`（签名/映射/SSML/live）。缺口：缓存过期、并发刷新惊群、401 复位代次、warmup 与按需请求交错全无确定性单测
- **复核备注（批次 9d）**：三条缺口里的「并发刷新惊群」与「401 复位代次」已修：`edgeTTSGetAuth` 返回 `(auth, gen, error)`，刷新由 `edgeTTSFetchMu` 串行化并在锁内二次检查缓存，`invalidateEdgeTTSAuth(gen)` 只在代次匹配时清空；`tts_token_cache_test.go` 三条测试覆盖。`tokenCacheState` 枚举与 `authFailureKind` 分层提示未做，留给后续。

#### C9 翻译引擎熔断器与竞速阶段 · 中
- **位置**：`fastpot/services.go:388-427, 429-602, 619-676, 710-793`
- **载体**：`type engineBreaker struct { mu sync.Mutex; until map[string]time.Time; fails map[string]int; lastErr map[string]string }`(722-727)；`var breakers = &engineBreaker{...}`(729-733)；`const breakerThreshold = 2`(741)、`breakerCooldown = 60 * time.Second`(748)、`racePreferred = "微软"`(402)、`raceGrace = 350 * time.Millisecond`(405)；`TranslateRace` 内 `preferredPending := false`(521)、`var held *raceResult`(551)、`var grace <-chan time.Time`(552)；`type engineHealth struct { Engine string; Fails int; Open bool; Until time.Time; LastError string }`(652-658)
- **状态**：按 engine name，无 key＝closed / `fails>=2` 且 until 未到＝open / until 已过＝逻辑 half-open；竞速阶段由 `preferredPending`+`held`+`grace` 三局部变量表达
- **迁移**：`TranslateRace`(432-602) 按 `breakers.allow` 组装 cands，contender goroutine 用 `shouldRecordBreaker(ctx2, err)`(543) 判定后 `breakers.record`；主循环首选回答→`preferredPending=false`，非首选有效回答且 preferredPending→`held=&r; grace=time.After(raceGrace)`(576-579)，grace 到(557-560) 或 `held!=nil && !preferredPending`(592-595) 返回 held；`allow`(751-765) 过期清理回 closed；`ok`(768-774) 清三项；`fail`(777-790) `fails[name]++`，`>=2`→`until[name]=now+60s`；`health`(661-676) 过期时**只改返回视图** `Fails=0; LastError=""` 不改真实 map
- **隐性**：三张 map 的「key 是否存在 + 值」共同编码状态，无 `CircuitState`；竞速阶段无 `racePhase`
- **风险**：`health()` 只改显示视图不清 map，设置页 `Fails=0` 与 `allow()` 调用前 `fails[name]` 仍为 2 的真实状态不一致；`services.go:582-584` 注释明确记录历史 bug：「Recording it again here doubled every failure: with a threshold of two, one bad answer was enough to open the breaker for a minute」；`shouldRecordBreaker`(412-427) 的 `context.DeadlineExceeded` 分支(423-425)依赖 `ctx.Err()==nil`，边界微妙；三局部变量无法从外部观测
- **显式化**：`circuitState`（`closed/halfOpen/open`）+ `engineCircuit{state;fails;openUntil;lastErr}` + `EngineBreakers.Allow/Succeed/Fail/Health` 共用同一 `refresh(name)` 消除视图分裂；`racePhase`（`collecting/awaitingPreferred/decided`）+ `raceTracker{phase;held;grace;preferredPending}`
- **测试**：`ai_generation_race_test.go:314/374`、`atomic_save_test.go:139/162`、`tencent_race_test.go:10`、`race_echo_test.go:9`、`live_race_test.go:14`。缺口：`closed→open→half-open→closed` 全周期、`health()` 视图一致性、`grace` 超时分支
- **复核备注（批次 7b）**：`engineBreaker` 本身的 `mu` 保护是完整的；真正的竞争在包级注册表 `breakers` 指针——测试用 `freshBreakers` 无同步替换它，而上一测试留下的请求会并发 `record`。已改为 `atomic.Pointer[engineBreaker]` + `currentBreaker()` 单一读取点，生产语义不变，`go test -race` 的偶发 DATA RACE 消失。熔断器状态枚举化（`BreakerState`）未做，留后续。

#### C10 截图框选会话（snipSession） · 高
- **位置**：`fastpot/snip.go:183-223, 239-361, 377-592`
- **载体**：`type snipSession struct { vx,vy,vw,vh int32; hdcMem,hbm,oldBmp,hdcDark,hbmDark,oldDarkBmp uintptr; selecting bool; startX,startY,curX,curY int32; cropDone bool; cropRect winRect; onSuccess func(cropRect winRect, imgPath string); onError func(err error) }`(183-198)；`var (currentSnip *snipSession; snipMu sync.Mutex; snipClassOnce sync.Once; snipAtom uintptr)`(200-205)
- **状态**：`currentSnip==nil` 空闲 / 非 nil 且 `selecting=false,cropDone=false` 覆盖层待按下 / `selecting=true` 框选拖拽中 / `cropDone=true` 有效框选完成 / 取消时 `selecting=false` 且 cropDone 保持初值
- **迁移**：`StartNativeSnip`(380-) 构造 session→`snipMu.Lock(); currentSnip = session`(463-465)→建全屏窗口→消息循环→`s := currentSnip; currentSnip = nil`(504-507)→`if s != nil && s.cropDone`(510) 触发 `s.onSuccess`(587-589)；`snipWndProc`(239-361)：`WM_LBUTTONDOWN`(245-255) `selecting=true`+记坐标、`WM_MOUSEMOVE`(257-263) 更新 curX/curY、`WM_LBUTTONUP`(265-286) `selecting=false` 且 `w>=10&&h>=10` 时 `cropDone=true`、`WM_RBUTTONDOWN/UP`(288-294) 与 ESC(296-304) 取消、`WM_PAINT`(306-352) 按 `selecting` 决定画选框或暗化
- **隐性**：无 `SnipPhase`；阶段由两个布尔 + 全局单例指针 + 回调是否非 nil 表达，WM_PAINT 与结果判定各自重新解释布尔组合
- **风险**：**`snipWndProc` 只在 240-242 取 `s` 指针时持锁，之后 247-301 对 `s.selecting/startX/.../cropDone/cropRect` 的所有读写都无锁**，而 `StartNativeSnip`:463-465/504-507 对 `currentSnip` 持锁——同一 session 字段实际无保护共享；`currentSnip` 是**全局单例**，连按截图快捷键会直接覆盖旧 session，两个消息循环都可能置 nil 并各自触发 `onSuccess`/`onError`，导致回调乱序与旧 overlay 资源泄漏；取消路径不显式置 `cropDone=false`，依赖初值；`cropDone` 与 `cropRect` 必须成对写入但无不变式
- **显式化**：`snipPhase`（`idle/overlayShown/selecting/cropped/cancelled`）替换两布尔；`snipResult{phase;rect;imgPath;err}`；`SnipController{mu;active *SnipSession;seq uint64}` 在 `active != nil` 时明确拒绝或取消旧会话；`onSuccess/onError` 合并为 `onResult(snipResult)`；`snipWndProc` 通过 `c.withSession(func(s *SnipSession){...})` 持锁访问全部字段
- **测试**：`snip_test.go:15/85`、`popup_ui_test.go:112`、`ocr_quality_test.go:63`、`ux_walkthrough_test.go:193`。缺口：WndProc 各消息迁移、右键/ESC 取消、重复 `StartNativeSnip` 并发、字段无锁访问**完全无单测**
- **复核备注（批次 5f）**：本条「风险」里「`currentSnip` 全局单例被连按截图覆盖、两个消息循环乱序触发回调」与「早退不清全局指针」经复核**成立且可确定性修复**，但落点与审计「WndProc 字段无锁访问需真实 Win32 消息泵」不同：`StartNativeSnip` 对 `currentSnip` 的赋值在建窗口与消息循环**之前**，是普通 Go 逻辑。批次 5f 新增 `beginSnipSessionLocked` / `endSnipSessionLocked` 两个持锁纯方法，让 `StartNativeSnip` 在 `snipMu` 锁内原子完成「检查 + 占位」（已有会话时明确拒绝第二次启动），并用 defer 覆盖 `hwnd==0` / GDI / 写盘失败等所有早退路径；`snipWndProc`(239-361) 的字段无锁读写、`snipPhase` / `SnipController` / `withSession` 结构性显式化仍需真实消息泵或较大重构，留阶段 C 后续。新增 `c10_snip_session_test.go`（`TestStartNativeSnipGuardsSingleton` 源码契约先红后绿 + `TestBeginSnipSessionLocked` 会话生命周期），`snip.go` 开工时 diff=0，两层验证不臆造窗口级复现；`go test -race ./...` 本批首次即通过，既有跨测试全局 breaker 污染偶发 FAIL 与本批无关。

#### C11 取词路径选择与剪贴板事务 · 中
- **位置**：`fastpot/capture.go:140-158, 171-249, 251-279, 288-409`；`fastpot/uia.go:113-130, 207-216, 218-285`
- **载体**：`a.store.Get().SelectionCapture` 字符串（capture.go:260 比 `"copy"`、390/395 比 `"uia"`）；`captureSelection func() selection`(`popup.go:217`，capture.go:387 用 `!= nil` 作测试缝哨兵)；`snapshotClipboard()`(229)、`previous := mygo.Clipboard.ReadText()`(230)、`var got string`(238) + 全局剪贴板这个外部可变状态；`appsWhereDragIsNotSelection`(332-358)；`modifierReleaseTimeout = 600ms`(273)、`clipboardPollTimeout = 500ms`(276)、`clipboardPollInterval = 15ms`(278)、`uiaTimeout = 300ms`(286)；`type selection struct { Text string; Rect uiaRect; Bounds bool }`(uia.go:209-216)；`owned := uint32(hr) == 0`(uia.go:123)
- **状态**：取词模式三态字符串；`Bounds bool` UIA 是否给出坐标；`owned` 是否拥有 COM 单元；剪贴板事务「快照已存→已清空→已发 Ctrl+C→已轮询→已恢复」全靠局部变量顺序
- **迁移**：`CaptureSelection`(259-266) 非 `"copy"` 先 `captureUIA()` 空则 `captureByCopy()`；`captureForGesture`(384-409) 测试桩/UIA/`"uia"` 模式/`neverSimulateCopy` 分支；`captureByCopy`(223-249) `waitForModifiersReleased`→`snapshotClipboard`→`previous`→`Clipboard.Clear()`(234)→`sendCtrlC()`→轮询→`restoreClipboard(snap, previous)`(247)；`captureUIA`(294-314) goroutine + `time.After(uiaTimeout)` 超时；`comApt`(116-130) owned→defer `CoUninitialize`
- **隐性**：无 `CaptureStrategy` 枚举或 `ClipboardTransaction` 类型；剪贴板「被临时占用」这一危险状态完全靠函数内局部变量顺序表达
- **风险**：`captureByCopy`:234 `Clear()` 到 247 `restoreClipboard` **没有 `defer`**——当前无提前 return 侥幸安全，但未来任何提前返回、panic 或 goroutine 被杀都会永久清空用户剪贴板（注释 9-11 行自述要「leave the clipboard exactly as it was」）；`restoreClipboard`(190-216) 富格式 Write 失败回退 `WriteText(previous)`，previous 为空时剪贴板仍可能为空；`captureUIA` 超时后 goroutine 被遗弃（注释 295-297 自述「leaks one pinned thread」）；两处对同一配置字段解释略不同（capture.go:390 额外有 `"uia"` 分支）
- **显式化**：`CaptureStrategy`（`uiaFirst/uiaOnly/copyOnly`）+ `strategyFromConfig(s string)` 一处解析；`ClipboardTransaction{snapshot;previousText;done bool}` + `defer tx.Restore()`；`uiaOutcome` 枚举替代 `selection{Text,Bounds}` 双字段解释
- **测试**：`gesture_test.go:206/219/236/257/275/304/328/354`、`selicon_test.go:143`、`hook_test.go:54/69`。缺口：剪贴板恢复失败/超时/中途异常、`comApt owned=false`、超时后遗弃 goroutine
- **复核备注（批次 9a）**：两处解读不一致属实——`CaptureSelection` 在「模拟复制」时跳过 UIA，「仅 UIA」时仍会回退 Ctrl+C；`captureForGesture` 则总是先试 UIA、忽略「模拟复制」。已新增 `captureStrategy` + `parseCaptureStrategy` + `capturePlan()` 让两条入口共用同一解析。剪贴板 `Clear()` 无 `defer` 也属实，已改为 `clipboardTransaction` 并由 `captureByCopy` 用 `defer tx.restore()` 登记，幂等还原。`uiaOutcome` 枚举与超时 goroutine 回收未做，留后续。

**本组未发现实质隐式状态机的文件**：`icons.go`（全为 const SVG + 只读 `var`）、`theme.go`（`applyTheme` 纯派生，`chipTone int` 已有显式枚举）、`helpers.go`（`inSpace bool` 是函数内局部标志）、`naming.go`（静态表 + 启动时一次性 `migrateUserData`）、`actions.go`（`saved` 为渲染期派生，跨事件载体在 `popup.go`）、`resources/ocr.ps1`（线性脚本，只有语言选择分支）。

**core 组按风险排序 Top 3**：C10 截图框选会话（无锁字段 + 全局单例覆盖，用户可见「连按截图弹出两个结果」）＞ C1 钩子手势（11 变量 + 双套同步，影响整个桌面输入）＞ C5 窗口与退出（`isQuitting` 双写构成明确数据竞争）。紧随：C7 TTS 代次竞态窗口、C9 熔断器（注释自述曾因重复记录失败熔断一分钟）、C2 `hookCommandFn` 单槽覆盖。

### 4.2 Popup UI 与悬浮胶囊（U1–U17）

#### U1 AI 卡片请求代次与流式渲染 · 高（ui 组 Top 1）
- **位置**：`fastpot/popup.go:68, 72-78, 108, 112, 123-126`；`fastpot/popup_actions.go:52-64, 443-480, 623-651, 653-825`；`fastpot/popup_view.go:155-188, 525-594`；`fastpot/popup_tabs.go:58-138`。切换行：`stopAIPassLocked`(52-64)、675-687 清场+取 gen、689-709 缓存命中、711-742 网络态置位、776-793 流式 chunk、795-823 收尾
- **载体**：`aiText string`(popup.go:72)、`aiBusy bool`(73)、`aiCancel context.CancelFunc`(74)、`aiGen uint64`(78)、`grammarResult *SentenceAnalysisResult`(108)、`grammarResultPrefill bool`(112)、`activeActionID string`(123)、`dictResult *DictionaryResult`(124)、`writingResult *ImproveWritingResult`(125)；`lastPaint time.Time` / `gen uint64` 为 runAction goroutine 局部变量
- **状态 / 迁移**：空闲（aiBusy=false 且 aiText=="" 且三结果指针 nil）/ 流式接收 / 结构化卡片完成 / 散文或解析失败 / 在途可取消（`aiCancel!=nil`）。`runAction`(653) 先 `stopAIPassLocked()` 使旧代次失效 → 缓存命中(689-709) 直接完成；否则(711-742) 置 `aiBusy=true, aiText="", aiCancel=cancel`；`StreamMagpie` 回调(776-793) 每次 `if gen != a.aiGen { return }` 后 `aiText += chunk`，40ms 节流 `win.Update`；收尾(795-823) `aiBusy=false, aiCancel=nil, applyParsedAction`；`switchMode`/`openPopupAction`/`openPopupWithText`/`refineWithAI`/`stopNotesLocked` 任一路径调 `stopAIPassLocked()` 强制回空闲
- **风险**：`aiGen` 被 `runAction`、`refineWithAI`(856 复用同一 `a.aiGen`)、`prefillActionTranslation`、`stopAIPassLocked` 共享，点「AI 精翻」会 `stopAIPassLocked()` 静默取消正在进行的卡片流，用户看到卡片停在中途且无提示；`a.aiText += chunk`(784) 在 goroutine 里持 `a.mu` 写入但 `win.Update`(790-792) 在锁外，chunk 与 paint 不同步；`openPopupAction`/`openPopupWithText` 的 Reset 段(482-528、566-604) 手工清零 30+ 字段，漏一处即旧状态泄漏；`stopAIPassLocked` 只置 `aiBusy=false` 不清 `aiText`，切 tab 后 `renderAIResultState`(525-552) 会把上一轮半成品当结果展示
- **显式化**：`type AIPassState uint8` + `const (aiPassIdle AIPassState = iota; aiPassStreaming; aiPassParsing; aiPassDone; aiPassFailed; aiPassCanceled)`；`type AIPass struct{ Gen uint64; State AIPassState; ActionID string; Text string; Cancel context.CancelFunc; Prefill *TranslationPrefill; Err error }`；`func (a *app) transitionAIPass(gen uint64, to AIPassState) bool` 统一收口；`const engineAIRefined = "AI 精翻"` 消除字符串哨兵；`aiRefining` 拆出独立 `RefineSession` 不再共用 `aiGen`
- **测试**：`popup_streaming_test.go:13/31/48/66`（只覆盖渲染三态）、`popup_ui_test.go:82/112`（OCR 代次）。缺口：aiGen 竞态/取消、精翻取消卡片、prefill 消费、Reset 段完整性
- **复核备注（批次 4b）**：精翻复用卡片代次属实——点「AI 精翻」会调 `stopAIPassLocked()` 静默取消在飞的卡片流。已拆出独立 `aiRefineGen requestToken` + `aiRefineCancel context.CancelFunc`，`refineWithAI` 不再碰卡片代次；`TestRefineWithAIDoesNotCancelCardStream` 先红后绿，`TestRefineWithAIIndependentGeneration` 钉住两条代次互不影响。完整 `AIPass` 会话对象与写入侧收敛未做，留后续。

#### U2 翻译请求生命周期 · 高
- **位置**：`fastpot/popup.go:62-69, 163`；`fastpot/popup_actions.go:36-46, 88-184`；`fastpot/popup_translation.go:152-160, 559-598`；`fastpot/popup_layout.go:326-382`。关键行：106 `a.busy = true`、110-112 `translateGen++ / gen / notesGen`、144-151 丢弃 stale、152 `a.busy = false`、180-182 派生生词请求
- **载体**：`busy bool`(popup.go:68)、`translateGen uint64`(163)、`source string`(62)、`target string`(63)、`engine string`(64)、`errText string`(66)、`elapsed time.Duration`(69)；局部 `gen := a.translateGen`(popup_actions.go:111)
- **状态 / 迁移**：空闲 / 翻译中(Spinner) / 有译文 / 失败 / 过期结果丢弃。`translateTextWithReflow`(88) 持锁 reset 全部字段并 `busy=true, translateGen++`；goroutine(118-183) 先 `if gen != a.translateGen { 丢弃; return }`(144-151) 再 `busy=false`(152)、写 target/engine/detected、`fitHeight()`(171)、`extractNotesForGen(notesGen,text)`(180-182)；`rerunCurrentModeFresh`(popup.go:391-396) → `skipAIResultCache=true` 重入；`renderEngineMenu` 以 `!s.busy` 门控再 `a.translate(s.source)`；`switchMode`(427-430) 直接 `a.busy=false` 硬切
- **风险**：**`switchMode`(popup.go:427-430) 直接把 `busy=false` 而不 `translateGen++`，旧翻译 goroutine 回来后(144-151) 的 gen 校验仍通过，会把旧译文写进新模式视图**；`renderEngineMenu` 用 `!s.busy` 门控，但 `busy` 在 goroutine(152) 与 UI 线程(590) 无同步读；`elapsed`/`engine` 与 `target` 分多次赋值，中间帧显示「新 engine + 旧 target」
- **显式化**：`type TranslatePhase uint8`（`translateIdle/translatePending/translateDone/translateFailed`）+ `type TranslateRequest struct{ Gen uint64; Phase TranslatePhase; Source, Target, Engine, Detected, Err string; Elapsed time.Duration }` + `func (a *app) startTranslation(src string) uint64`；`switchMode` 改为 `a.translate.Cancel()`（内部自增 gen）而非手改 busy
- **测试**：`popup_ui_test.go:13`、`layout_test.go` 全为布局纯函数。缺口：translateGen 竞态、switchMode 期间结果到达、`busy` 门控
- **复核备注（批次 4d）**：`switchMode` 与两个 `openPopup` 入口只置 `busy=false`、不失效 `translateGen` 属实，旧机器翻译可写进新模式视图。三处现都在清 `busy` 后自增 `translateGen`；`TranslationSession` 结构体化未做，留后续。

#### U3 生词提取请求（notes） · 中
- **位置**：`fastpot/popup.go:84, 100-105, 152`；`fastpot/popup_actions.go:69-77, 180-182, 206-226, 228-323, 1024-1059`；`fastpot/popup_view.go:283-372`；`fastpot/popup_translation.go:253-291, 509-514`；`fastpot/popup_ocr_pairs.go:185-222`
- **载体**：`notes []NoteItem`(popup.go:100)、`notesBusy bool`(101)、`notesGen uint64`(105)、`notesCancel context.CancelFunc`(106)、`notesExpanded bool`(84)、`notesSaved bool`(152)
- **状态 / 迁移**：无 / 提取中 / 就绪 / 已保存；`notesExpanded` 正交。`translateTextWithReflow`(180-182) 翻译成功后 `if cfg.ShowNotes && shouldExtractNotes(text)` → `extractNotesForGen(notesGen,text)`；`extractNotesForGen`(228-323) 先 `if gen != a.notesGen { return }`(240-243)，再 `stopNotesLocked()`(247) 使 gen 二次自增后重取 `gen = a.notesGen`(248)，置 `notesBusy=true, notes=nil, notesCancel=cancel`(249-252)；收尾(307-318) 校验 gen 后写结果；`saveNotesToVocab`(1024-1059) → `notesSaved=true`
- **风险**：`notesSaved` 在 `saveNotesToVocab`(1029) 置 true 后只有 openPopup* Reset 段才清；切 tab 时 `saveTabLocked`(popup_tabs.go:58-99) 保存 `notesBusy` 原值，但 `restoreTabLocked`(101-138) 强制 `a.notesBusy=false, a.notesCancel=nil` 且**不**自增 notesGen，恢复后旧提取 goroutine 的 gen 校验仍通过 → 旧生词写进新 tab；`notes == nil` 与 `len(notes)==0` 在 `renderNotesSummary`(509-514) 语义不同但常被混用
- **显式化**：`type NotesPhase uint8`（`notesIdle/notesExtracting/notesReady/notesFailed/notesCanceled`）+ `type NotesSession struct{ Gen uint64; Phase NotesPhase; Items []NoteItem; Saved, Expanded bool; Cancel context.CancelFunc }` + `func (s *NotesSession) Invalidate()` 把「二次自增」收进一处
- **测试**：`popup_ui_test.go:21/30`（配置开关）、`layout_test.go:215`（展开高度）。缺口：notesGen 竞态、恢复 tab 后旧结果到达、notesSaved 生命周期

#### U4 AI 精翻请求（与 U1 共用代次） · 中
- **位置**：`fastpot/popup.go:64, 225-226`；`fastpot/popup_actions.go:840-925`；`fastpot/popup_translation.go:204-241`；`fastpot/popup_ocr_pairs.go:146-178`。关键行：856 `gen := a.aiGen`、896-903 stale 丢弃、904-905 复位、906-918 成功/失败
- **载体**：`aiRefining bool`(popup.go:225)、`aiRefineErr string`(226)、`engine string`(64，哨兵值 `"AI 精翻"`)、`aiGen uint64`(78，复用)、`aiCancel context.CancelFunc`(74，复用)
- **状态 / 迁移**：未精翻 / 精翻中 / 已精翻（`engine=="AI 精翻"`）/ 失败。`renderTranslationContent`(228-230) / `renderScreenshotPairActions`(146-167) `btn.Clicked() && !s.aiRefining` → `refineWithAI()`；`refineWithAI`(852-862) 持锁 `stopAIPassLocked()` + `gen := a.aiGen` + `aiRefining=true, aiRefineErr=""`；goroutine(874-880) 瞬态错误 `time.Sleep(350ms)` 重试一次；截图回退 `StreamMagpieWithImage`(881-894)；`if gen != a.aiGen { 丢弃; return }`(896-903)；`aiRefining=false, aiCancel=nil`(904-905)；成功 → `target=flattenTranslation(text), engine="AI 精翻"`(906-918)
- **隐性**：阶段靠「bool + 字符串哨兵 + 错误字符串」三载体；`engine` 本是翻译引擎名，被复用成「是否已精翻」的状态位；重试次数是 goroutine 内硬编码的一次 `if`
- **风险**：与 U1 共用 `a.aiGen`/`a.aiCancel`，`refineWithAI` 调 `stopAIPassLocked()` 会取消卡片流，反之 `runAction`/`switchMode` 也会取消精翻；`stopAIPassLocked`(52-64) 里有 `a.aiRefining = false` 且清 `aiRefineErr`，错误信息会无故消失；切引擎后 `translate()` 覆写 engine 导致「已精翻」标记丢失但 target 仍是精翻结果
- **显式化**：`type RefinePhase uint8`（`refineIdle/refineRunning/refineRetrying/refineDone/refineFailed`）+ `type RefineSession struct{ Gen uint64; Phase RefinePhase; Attempt int; Err string; Cancel context.CancelFunc }` + `const refineMaxAttempts = 2`；用独立 gen
- **测试**：**无专门测试**，全缺口（含重试、与卡片互斥、engine 哨兵）

#### U5 模式与输入视图切换 · 中
- **位置**：`fastpot/popup.go:23-34, 61, 79-81, 97, 343-454, 456-620`；`fastpot/popup_translation.go:62-108, 114-143`；`fastpot/popup_view.go:44-69, 596-709`
- **载体**：`mode popupMode`(popup.go:61)、`inputTargetMode popupMode`(80)、`inputText string`(79)、`source string`(62)、`page string`(97)、`trayInput string`(81)；`var popupModeLabels = [...]string{"翻译","长难句","写作"}`(popup_translation.go:62)
- **状态 / 迁移**：四种结果视图 + `modeInput` 键盘输入态 + `modeSelectionPreview` 选区预览；`modeSelection = modeTranslate` 是别名(34)。`openPopup`(343-374) 无选区→`openPopupWithText(modeInput,"")`，有选区→按 `InitialAIMode()` 选 mode；`switchPopupMode`(82-91) → `inputTargetMode=modes[index], inputText="", switchMode(...)`；TextArea Enter 无 Shift(114-143) → `source=text, inputText="", switchMode(targetMode)`；四个空态按钮(view.go:623-628 等) → `inputTargetMode=modeX; switchMode(modeInput)`；`switchMode`(398-454) 降级、同 mode 早退(410-419)、保存/停止/恢复/重跑（详见 §4.4 X2）
- **风险**：`modeSelection = modeTranslate`(34) 别名导致 `modeName()` 无法区分选区预览；`switchMode` 同 mode 早退(410-419) 时不刷新不重跑，F5 想重跑当前模式可能被早退吞掉；降级(402-408) 只改 `m` 与 notice，若 `tabStates[modeGrammar]` 已有内容，切走再切回会恢复到被禁用的语法模式；`inputTargetMode==0` 才补默认(573-577)，而 `modeTranslate` 恰好是 0，语义重叠
- **显式化**：`type ViewState uint8`（`viewEmpty/viewInput/viewPending/viewResult/viewError`）+ `type ModeContext struct{ Mode popupMode; View ViewState; InputTarget popupMode; InputText string; Page SettingsPage }` + `type SettingsPage string` + `type ModeChangeReason uint8`（`modeChangeTab/modeChangeHotkey/modeChangeEmptyState/modeChangeRerun/modeChangeDegraded`），`switchMode(m, reason)` 显式处理早退/降级
- **测试**：**无**，缺口：降级路径、同 mode 早退、inputTargetMode 默认值、page 路由

#### U6 Tab 快照与恢复 · 高（ui 组 Top 3）
- **位置**：`fastpot/popup.go:167`；`fastpot/popup_tabs.go:14-50, 52-56, 58-99, 101-138, 142-179`；`fastpot/popup.go:398-454`
- **载体**：`tabStates map[popupMode]*tabViewState`(popup.go:167)；`tabViewState` 内 `aiBusy bool`、`notesBusy bool`、`aiText string`、`notes []NoteItem`、`grammarResultPrefill bool`、`source string` 等 30+ 字段(popup_tabs.go:14-50)
- **状态 / 迁移**：`tabStates[m]==nil` 无缓存 / 可恢复（source 相等且 `hasContent()`）/ 过期。`switchMode`(423) 切走前 `saveTabLocked()`(58-99) **原样**保存 `aiBusy: a.aiBusy`、`notesBusy: a.notesBusy`；切回(442-450) 命中 → `restoreTabLocked(st)`(101-138) 恢复大部分字段但强制 `a.aiBusy=false, a.aiCancel=nil, a.notesBusy=false, a.notesCancel=nil` + `fitHeight()`；否则 `clearTabLocked()`(142-179) + `rerunCurrentMode`；openPopup* Reset 段直接 `tabStates=nil`
- **风险**：**确凿的状态不同步**——saveTabLocked(58-99) 原样保存 `aiBusy/notesBusy`，restoreTabLocked(101-138) 却强制置 false 并丢弃 `aiCancel/notesCancel`，且**不**自增 `aiGen/notesGen`；恢复后旧请求 goroutine 的 gen 校验仍通过，会把旧结果写进「看起来已完成」的 tab；反过来若保存时 `aiBusy=true`，恢复后 UI 立刻显示「完成」但结果还在路上；`hasContent()`(52-56) 不检查 busy，缓存命中判定(442-450) 会把「正在加载」的 tab 当作已完成直接恢复
- **显式化**：`type TabPhase uint8`（`tabEmpty/tabLoading/tabStreaming/tabReady/tabFailed`）+ `type TabSnapshot struct{ Mode popupMode; Source string; Phase TabPhase; Result ResultPayload; Options ViewOptions }` + `type TabCache struct{ states map[popupMode]*TabSnapshot }` 提供 `Save(app)/Restore(app, m)/Clear(m)`，Save 与 Restore 共用 `syncSnapshot` 序列化函数；`hasContent()` → `snap.Phase == tabReady`
- **测试**：**无**，缺口：save/restore 不对称、busy 态缓存、gen 未失效全无测试
- **复核备注（批次 4c）**：`hasContent()` 把 `aiBusy` / `notesBusy` 的进行中页签当成有内容会阻止重跑，已改为 `!s.aiBusy && !s.notesBusy && (...)`；`restoreTabLocked` 强制置 false 与不自增 gen 由 `switchMode` 的 `stopAIPassLocked` / `stopNotesLocked` 兜住，经核查无需再改。完整 `TabSnapshot` 结构体未做，留后续。

#### U7 结果卡片三态渲染（nil 哨兵 + 内容嗅探） · 中
- **位置**：`fastpot/popup_view.go:155-188, 525-594`；`fastpot/popup.go:108, 124-125`
- **载体**：`grammarResult *SentenceAnalysisResult`(popup.go:108)、`dictResult *DictionaryResult`(124)、`writingResult *ImproveWritingResult`(125)、`aiBusy bool`(73)、`aiText string`(72)；`snapshot` 同名字段(259-299)
- **状态 / 迁移**：`result!=nil` 结构化卡片 / `result==nil && (aiBusy || aiText!="")` AI 结果态 / 皆否空态；AI 结果态内再由 `looksLikeStructuredPayload(aiText)`(570-584，首字符 `{ [ `` ` 或含 `{"`) 分「散文 markdown」/「解析失败文案」。`popupView`(155-188) 三分支；`renderAIResultState`(525-552) `aiBusy`→Spinner、`aiText==""`→return、非结构化→`renderAiMarkdown(...,false)`；`renderAiMarkdown`(588-594) `busy → markdown.Stream` 否则 `markdown.View`
- **风险**：`aiText` 是流式累加的半成品(784)，首字符恰好是 `{` 时会中途判为结构化 → 显示「这次没能解析成结构化卡片」闪一下再变散文；`result!=nil` 与 `aiText!=""` 可同时成立（applyParsedAction 成功后未清 aiText），三分支靠 if 顺序掩盖不一致；`renderAiMarkdown` 的 `busy` 参数在调用处恒传 false，`markdown.Stream` 分支**不可达状态**
- **显式化**：`type ResultView uint8`（`resultEmpty/resultLoading/resultStreaming/resultStructured/resultProse/resultParseFailed`）+ `func classifyResult(s *snapshot) ResultView` 统一容纳 `looksLikeStructuredPayload`；`renderAIResultState` 改为 `switch classifyResult(s)`；删掉恒 false 的 busy 参数
- **测试**：`popup_streaming_test.go:13/31/48/66`。缺口：`result!=nil && aiText!=""` 组合、流式首字符 `{`、markdown.Stream 不可达分支

#### U8 主窗口生命周期与置顶 · 高
- **位置**：`fastpot/popup.go:93-99, 155, 158-159, 176, 343-374, 456-540, 544-620`；`fastpot/popup_translation.go:39-57`；`fastpot/popup_view.go:44-48`；`fastpot/popup_layout.go:254-322`
- **载体**：`win *mygo.Window`(popup.go:93)、`pinned bool`(98)、`screenshotPinned bool`(176)、`isQuitting bool`(155)、`pausedUntil time.Time`(158)、`popupX/Y/W/H int`(178-181)
- **状态 / 迁移**：`win.Hide()` 隐藏 / `Show()+Focus()` 可见激活 / `pinned=true` 强制置顶 / `screenshotPinned=true` 截图临时置顶。`openPopupAction`(530-535) / `openPopupWithText` → `SetSize/SetPosition/SetAlwaysOnTop/Show/Focus/Update`；关闭(53-57) 与 Escape(view.go:44-48) → `stopAudio(); stopScreenshotOCR(); win.Hide()`；pin 按钮(39-52) → `a.pinned = !a.pinned`；`openPopupWithText`(600-603) `if a.screenshotPinned { a.pinned = false; a.screenshotPinned = false }` 作为兜底
- **风险**：**已有实锤泄漏**——600-603 注释「防止截图翻译置顶状态泄漏到后续每个 popup，OnBlur 不再关闭」说明 `screenshotPinned` 曾漏复位导致后续所有弹窗都置顶且失焦不关；`pinned` 只在 openPopupWithText 里被无条件清，openPopupAction 路径未做同样处理，两条入口不对等；`isQuitting` 与 `win.Hide()` 无关联，Hide 后异步回调仍可能 `win.Update` 把窗口重新画出
- **显式化**：`type PopupWindowState uint8`（`popupHidden/popupVisible/popupPinned/popupScreenshotPinned`）+ `type WindowPolicy struct{ AlwaysOnTop, DismissOnBlur, PinPinned bool; State PopupWindowState }` + `func (a *app) openPopup(m, text)` 统一收口两条入口，把 530-535 与 600-603 合并为 `a.windowPolicy.Reset(m)`；`pausedUntil` → `Paused bool` + `PauseUntil time.Time`
- **测试**：`popup_ui_test.go:171`（仅主题）。缺口：置顶泄漏、Hide 后回调、两入口不一致全无测试
- **复核备注（批次 5d）**：「两入口不对等」经复核**成立**——`openPopupWithText` 有 `screenshotPinned` 兜底、`openPopupAction` 的 Reset 段没有，截图翻译置顶后从 chip / 托盘打开新卡片会继承置顶，OnBlur 不关闭；`screenshotPinned` 全库仅 5 处，兜底段是 `git diff` 的 context（用户未改）。已按「显式化」里 `func (a *app) openPopup(m, text)` 统一收口的思路迈出最局部一步：新增纯方法 `resetScreenshotPinLocked()`（调用方持 `a.mu`、仅在 `screenshotPinned` 时把 `pinned` 与 `screenshotPinned` 一起复位，用户手动置顶不受影响），两入口都调用；`openPopupWithText` 行为逐字不变，`openPopupAction` 补上缺失复位。两条函数都在 `win==nil` 时提前 return、需真实 window 无法 headless 调用，故用 `u8_screenshot_pin_test.go` 的「源码契约断言两入口都调用 + 纯方法表驱动行为测试」两层验证，不编造窗口级复现。`PopupWindowState` / `WindowPolicy` 完整枚举化、`pausedUntil` 拆分与 `isQuitting` / `win.Hide()` 关联（后者已在批次 5c 显式化为 `atomic.Bool`）仍留阶段 C 后续。详见 [STATE_MACHINES_FIX_PLAYBOOK.md](STATE_MACHINES_FIX_PLAYBOOK.md) §5 / §7

#### U9 划词 Chip 显隐与动作 · 中
- **位置**：`fastpot/selicon.go:63-69, 159, 170-200, 204-292, 441-453, 761-778, 1072-1115, 1121-1182, 1234-1258`；`fastpot/popup.go:208, 214-221`；`fastpot/hook.go:449-452`（`chipHoverArmed` 原子量）
- **载体**：`type selState struct { win *mygo.Window; text string; x, y int; shown bool }`(selicon.go:64-69)、`var selIconOnce sync.Once`(159)、`moreFlyout *mygo.Window`(popup.go:138)、`gestureGate gestureGate`(214)、`chipHoverArmed uint32`(hook.go)
- **状态 / 迁移**：`shown=false && win 隐藏` 无 chip / `shown=true && win.ShowInactive()` 可见不抢焦点 / `moreFlyout!=nil` 更多浮层展开。`onSelectionGesture`(967-1015) 经 trigger/typing/blocklist/`gestureGate.allow`(996) 过滤 → `deliverSelection`(1026-1050) 按 `SelectionTrigger=="auto"` 直接开 popup，否则 `showChipAt`(1054-1060) → `showSelIcon`(204-267) 置 `sel.text/x/y/shown=true`、`ShowInactive()`、`armChipHover()`(259)、`go sampleChipPointer()`(266)；`hideSelIcon`(441-453) `was:=shown, shown=false` → `disarmChipHover()` + `hideChipTip()` + 条件 `win.Hide()`；`toggleMoreFlyout`(1234-1258) 用 `moreFlyout!=nil && !IsDestroyed()` 切换
- **风险**：`showSelIcon` 先置 `shown=true`(221) 再 `ShowInactive()`(226) 再 `RunOnMain` 摆放(232-254)，`sampleChipPointer`(274-292) 在 goroutine 里最多等 10×120ms；若此间用户点击别处触发 `hideSelIcon`，`sampleChipPointer` 只在 281-286 检查一次 `shown`，之后仍可能调 `onChipPointer` 把已隐藏 chip 的 tip 抬起；`toggleMoreFlyout` 的 nil 判断与 `OnClosed` 回调(1255-1257) 分两处置 nil，重复点击可能创建两个浮层；`hideSelIcon` 只在 `was==true` 时 `win.Hide()`(450)，状态与窗口不一致时窗口残留（代码注释 695-698 承认）
- **显式化**：`type ChipState uint8`（`chipHidden/chipPlacing/chipVisible/chipActing`）+ `type ChipSession struct{ State ChipState; Text string; X, Y int; Win, MoreFlyout *mygo.Window; Gen uint64 }` + `func (s *ChipSession) Show(text string, x, y int) / Hide() / Act(fn)`；`chipHoverArmed` → `s.State == chipVisible`；所有显示/隐藏入口统一自增 `Gen`，`sampleChipPointer` 校验 Gen
- **测试**：`popup_ui_test.go:127/153`、`selicon_hover_test.go:14/35/73/88`、`chip_states_visual_test.go:18`。缺口：shown 生命周期、show/hide 竞态、moreFlyout 重复创建、Gen 代次
- **复核备注（批次 9h）**：chip 的 pointer 采样已按代次隔离 —— `selState.gen` 在显示/隐藏时自增，`sampleChipPointer(gen)` 用 `chipSampleStillCurrent` 校验，收起或重新显示后旧采样不再抬提示。`hideSelIcon` 的 `was==true` 守卫、`toggleMoreFlyout` 重复创建、完整 `ChipSession` 枚举化仍未做。

#### U10 Hover Tip 计时器与代次 · 高（ui 组 Top 2，本组最复杂）
- **位置**：`fastpot/selicon.go:42-61, 74-109, 345-383, 467-484, 490-580, 590-702, 735-759, 1121-1136, 1199-1227`。关键行：504-517 离开分支、522-533 新按钮/warm、534-544 定时器重排、545-552 到期、648-653 与 663-669 两处 gen 复查、683-688 `gen++`、1134-1136 每帧清空 hits
- **载体**：`type chipTipState struct` 全字段(selicon.go:74-109)：`key, label string`(80)、`since time.Time`(81)、`shown bool`(82)、`hiddenAt time.Time`(85)、`anchor ui.Rect`(87)、`gen uint64`(92)、`hits []chipHit`(98)、`timer *time.Timer`(102)、`geom chipGeom`(108)、`mu sync.Mutex`(75)、`win *mygo.Window`(77)；常量 `chipTipDelay = 160 * time.Millisecond`(51)、`selIconSettle = 120 * time.Millisecond`(57)、`chipTipWarm = 400 * time.Millisecond`(60)
- **状态 / 迁移**：无悬停 / 等待 160ms / 标签已显示 / warm 窗口（≤400ms 下一个按钮立即显示）/ 待触发的延时回调 / 按钮盒就绪。`onChipPointer`(490-553)：`!inside`(504-517)→`key="", shown=false, hiddenAt=now, win.Hide()`；新 key(523-533)→`warm := !hiddenAt.IsZero() && now.Sub(hiddenAt) <= chipTipWarm`，非 warm 则 `since=now`；`due := !shown && now.Sub(since) >= chipTipDelay`(534)，未到期(535-544) `timer.Stop()` + `time.AfterFunc(remaining, ...)` 重排单一 timer，到期(545-552) → `raiseChipTip`；`raiseChipTip`(590-675) 置 `t.label, t.shown=true, gen := t.gen`(601-606)，`t.mu` 下 `stale := t.gen != gen`(648-650) 丢弃，`ShowInactive()`(654)，`RunOnMain` 内再查 `t.gen != gen || !t.shown`(663-669)；`hideChipTip`(677-702) → `shown=false, key="", gen++, hiddenAt=now, timer.Stop(), win.Hide()`；`recordChipHit`(467-484)/`selIconView`(1134-1136) 每帧重建 hits
- **隐性**：七个载体组合表达阶段，迁移散落在鼠标钩子 goroutine、`AfterFunc` 定时器、主线程 `RunOnMain` 三个执行上下文，靠同一把 `chipTip.mu` + 一个 uint64 代次手工拼接
- **风险**：**代码注释自承认不可恢复状态**——selicon.go:683-686「Without it the goroutine's ShowInactive lands after this Hide and the plate stays on screen with t.shown false, so no later hide would ever take it down — the window becomes unclosable until restart」；`t.shown` 在 `onChipPointer`(549) 与 `raiseChipTip`(603) 两处置 true，`t.gen` 在三处校验(648-650、663-669、687)，任一处遗漏即窗口残留；`hits` 每帧清空(1134-1136) 而 `recordChipHit`(467-484) 逐按钮追加，指针事件可能在清空与重建之间读到空 hits → tip 无故消失
- **显式化**：`type ChipTipPhase uint8`（`tipIdle/tipWaiting/tipWarm/tipShown/tipHiding`）+ `type ChipTipEvent uint8`（`tipPointerMove/tipPointerLeave/tipTimerFired/tipHide/tipChipHidden`）+ `type ChipTipSession struct{ Phase ChipTipPhase; Key, Label string; Since, HiddenAt time.Time; Gen uint64; Timer *time.Timer; Win *mygo.Window; Hits []chipHit }` + `func (s *ChipTipSession) Transition(ev ChipTipEvent, label string) tipAction` 唯一迁移入口，返回 `tipActionNone/tipActionSchedule/tipActionShow/tipActionHide`；`selIconView` 的 hits 清空改为整帧原子替换
- **测试**：`selicon_tip_race_test.go:21/54/87`（3 条竞态路径）。缺口：定时器重排/取消、hits 清空与指针事件交错、三执行上下文完整迁移表、warm 窗口边界（恰 400ms）、tipHiding 后再次显示
- **复核备注（批次 6d）**：本条「显式化」建议的 `ChipTipPhase` / `ChipTipSession` / `Transition` 完整迁移表仍是后续方向；实际代码已有 `chipTipState.mu` 与 `gen` / `shown` / `since` / `hiddenAt` / `timer` / `hits` / `geom` 显式字段，`hideChipTip()` 也有 `t.gen++` 与无条件 `win.Hide()` 兜底，**无用户可见缺陷**。批次 6d 只把 `onChipPointer` 里的两个时间窗口判断收成 `chipTipIsWarm()` / `chipTipIsDue()` 两个具名谓词并加 `chipTipPhase` 枚举，零行为变化；三执行上下文的完整迁移表需真人 hover 时序走查，自动化只覆盖纯函数与源码契约（`chip_tip_phase_test.go` 四条）。

#### U11 自动高度收敛（振荡锁定） · 中
- **位置**：`fastpot/popup.go:188-193, 203-205`；`fastpot/popup_view.go:203, 216-281`；`fastpot/popup_layout.go:219, 240-297, 326-382`。关键行：224-229 settled、236-240 未稳定重排、263-281 autofitDecision、273-276 方向翻转锁死、371-378 解锁
- **载体**：`measuredViewportH float32`(popup.go:188)、`measuredContentH float32`(189)、`autofitTarget float32`(190)、`lastAutofitWinH int`(191)、`lastAutofitViewportH float32`(192)、`lastAutofitContentH float32`(193)、`autofitSteps int`(203)、`autofitDir float32`(204)、`autofitLocked bool`(205)
- **状态 / 迁移**：空闲/首帧 / 正在增高(`autofitDir>0`) / 正在降低(`autofitDir<0`) / 检测到振荡永久停止本轮自适应(`autofitLocked=true`)。`scheduleAutofit`(216-255) 持锁写测量值，`settled := h == a.lastAutofitWinH`；`!settled`(236-240) → `win.Update(func(){})` 重排一帧后 return；settled → `autofitDecision`(263-281)：`if a.autofitLocked { return }`(270-272)，`if a.autofitDir != 0 && (delta>0)!=(a.autofitDir>0) { a.autofitLocked = true; return }`(273-276) 锁死，否则 `autofitTarget=delta; autofitDir=delta; autofitSteps++`(277-280)；`applyPopupHeight`(popup_layout.go:254-297) 实际改窗口；`fitHeight`(371-378) 当内容真变化时解锁
- **风险**：`autofitLocked=true` 后只有 `fitHeight`(371-378) 解锁，而流式 chunk 只 `win.Update` 不调 fitHeight（收尾 821-822 才调），整个流式期间高度锁死不跟随内容 → 文字被裁切；`autofitSteps` 只递增不参与任何判定，是死字段；`autofitTarget` 每次 decision 开头被清零(264) 却无人读取；`settled` 用 `h == lastAutofitWinH` 判稳定，两帧恰好同高但内容已变会误判
- **显式化**：`type AutofitPhase uint8`（`autofitIdle/autofitMeasuring/autofitAdjusting/autofitLockedPhase`）+ `type AutofitSession struct{ Phase AutofitPhase; Dir float32; Steps int; Target, LastViewportH, LastContentH float32; LastWinH int; ContentRev uint64 }` + `func (s *AutofitSession) Decide(viewport, content float32, contentRev uint64) (delta float32, ok bool)`，用 `ContentRev` 比较代替「内容变化即解锁」；删除死字段
- **测试**：`layout_test.go:12-215`（全是纯函数）。缺口：autofitDecision 振荡锁定、解锁路径、settled 判定、流式期间锁死
- **复核备注（批次 7e）**：流式期间锁死属实（chunk 只 `win.Update`，`fitHeight` 只在收尾调）。已给锁定加上「内容高度变化超过 8 DIP 即解锁」的判据，`autofitLockContentH` 记录锁定时的内容高度；`AutofitSession` 结构体化、`autofitSteps`/`autofitTarget` 死字段清理留后续。

#### U12 展开/折叠与视图选项布尔组 · 中
- **位置**：`fastpot/popup.go:82-84, 107, 139-141, 170-171`；`fastpot/popup_translation.go:163-173, 284-289, 357-388, 509-514`；`fastpot/popup_ocr_pairs.go:39-125, 131-224`；`fastpot/popup_layout.go:79-215`
- **载体**：`sourceExpanded bool`(popup.go:82)、`snipExpanded bool`(83)、`notesExpanded bool`(84)、`onlyTrunk bool`(107)、`showSlashes bool`(139)、`filterRole string`(140)、`grammarHoverKnowledge string`(141)、`ocrCompare bool`(170)、`ocrExpanded bool`(171)、`vocabExpandedID string`(147，注释明确 `""` 表示每行摘要态)
- **状态 / 迁移**：各 bool 表示展开/折叠；`vocabExpandedID==""` 全部收起，非空＝某行展开（互斥 ID 用字符串编码）；`ocrCompare=true + ocrExpanded=true` 段落对照且展开全部(>8 段)。源文展开按钮(popup_translation.go:367-377)、生词折叠(284-289 / popup_ocr_pairs.go:215-220)、「段落对照」(168-173)、「合并显示」(54-59)、展开按钮(113-123) 各自取反；`popupContentHeight`(popup_layout.go:79-215) 按这些 bool 计算高度；openPopup* 两份 Reset 段(482-528、566-604) 集中置 false/""；`tabViewState` 保存/恢复其中 8 个
- **风险**：`ocrExpanded=true` 但 `ocrCompare=false` 时该标志无消费者，产生**不可达状态**；`switchMode` 保存 tab 时(popup_tabs.go:58-99) 保存了 `sourceExpanded/notesExpanded/isScreenshotMode/ocrCompare/ocrExpanded/snipExpanded` 却**漏了** `onlyTrunk/showSlashes/filterRole/grammarHoverKnowledge`，切回后这些选项被保留或清零，行为不一致
- **显式化**：`type ExpandState uint16`（`expandSource ExpandState = 1<<iota; expandSnippet; expandNotes; expandOCRCompare; expandOCRAll; expandSnip`）+ `type FilterRole string` + `type ViewOptions struct{ Expand ExpandState; Filter FilterRole; OnlyTrunk, ShowSlashes bool; HoverKnowledgeID, VocabExpandedID string }`；Reset 段改为 `a.viewOpts = ViewOptions{}` 一行；tab 快照序列化整个 `ViewOptions`
- **测试**：`layout_test.go:215`（间接覆盖 notesExpanded）。缺口：其余 9 个标志、Reset 两份清单一致性、tab 保存字段遗漏

#### U13 保存反馈与横幅 · 中
- **位置**：`fastpot/popup.go:126, 130-131, 135, 152-154`；`fastpot/popup_actions.go:22-33, 356-432, 985-1087`；`fastpot/popup_view.go:96-145, 283-398`；`fastpot/popup_ocr_pairs.go:185-222`
- **载体**：`savedDictTerm string`(popup.go:130)、`copiedMsg string`(131)、`actionSavedMsg string`(126)、`actionSaveFeedback string`(135)、`notesSaved bool`(152)、`grammarSaved bool`(153)、`writingSaved bool`(154)
- **状态 / 迁移**：`savedDictTerm` 非空且 `EqualFold(trim(savedDictTerm), trim(s.source))`(popup_view.go:375-398)＝当前词已保存；三个 saved bool 各自表示对应卡片已入库；四条横幅字符串非空＝正在展示反馈。`copyToClipboard`(22-33) → `copiedMsg`；`saveDictToVocab`(378-383) → `actionSavedMsg` + `savedDictTerm = term`；`saveGrammarToVocab`(403)/`saveWritingToVocab`(426) → 对应 saved bool；`saveNotesToVocab`(1029) → `notesSaved=true`；`handleQuickSave`(1061-1087) 按 `s.mode` 检查四个载体；`popupView`(96-145) 按字符串非空渲染四条横幅
- **隐性**：无 `SaveState`/`FeedbackBanner`；「已保存」用「字符串相等」内容嗅探推导；四条横幅用字符串是否为空表示显隐，与 saved bool 是两套独立载体
- **风险**：`savedDictTerm` 用 `EqualFold(trim(...))` 与 `s.source` 比较，用户改了大小写或前后空格，同一词条会被判「未保存」→ 重复入库；四个横幅字符串**没有任何自动清除路径**（全文无清空 `copiedMsg/actionSavedMsg` 的定时器，只在 openPopup* Reset 时清），反馈一直挂到下次开弹窗；`actionSaveFeedback`(135) 与 `actionSavedMsg`(126) 语义重叠易漏更新；`handleQuickSave` 同时检查 bool 与字符串，两套载体可能不同步
- **显式化**：`type SaveState uint8`（`saveNone/saveSaving/saveSaved/saveFailed`）+ `type SaveBadge struct{ State SaveState; TargetKey string; Message string; ShownAt time.Time }` + `type BannerKind uint8` + `type Banner struct{ Kind BannerKind; Text string; ExpiresAt time.Time }` + `const feedbackTTL = 3 * time.Second` + `func (a *app) showBanner(k BannerKind, text string)`；`renderWordSaveButton` 改读 `SaveBadge.State`
- **测试**：`popup_streaming_test.go:88/101/115`（savedDictTerm 三条路径）。缺口：横幅过期/清除、三 saved bool 与 handleQuickSave 组合、大小写/空格边界
- **复核备注（批次 9k）**：四条横幅「没有自动清除路径」成立，已加 `feedbackTTL = 3 * time.Second` 与 `snap` 侧过期过滤、到期定时重绘；新消息重置计时，`openPopup` Reset 同步清 `feedbackText`。`savedDictTerm` 的 `EqualFold(trim(...))` 判定与 `actionSaveFeedback` / `actionSavedMsg` 语义重叠未改，完整 `SaveBadge`/`Banner` 结构体留后续。

#### U14 AI 结果缓存与一次性跳过标志 · 中
- **位置**：`fastpot/popup.go:118-122, 391-396`；`fastpot/popup_actions.go:485-618, 653-709`。关键行：popup.go:391-396 `skipAIResultCache=true`、popup_actions.go:485 `aiResultCacheLimit = 12`、675-687 `useCache := !a.skipAIResultCache; a.skipAIResultCache = false`、612-618 clearAICache
- **载体**：`aiResultCache map[string]string`(popup.go:118)、`aiResultOrder []string`(119)、`skipAIResultCache bool`(122)；`const aiResultCacheLimit = 12`(popup_actions.go:485)；`useCache bool` 为 runAction 局部变量(678)
- **状态 / 迁移**：空缓存 / 可复用（key 命中且 `len(order)<=12`）/ 下一次请求强制绕过缓存（一次性）。`rerunCurrentModeFresh`(popup.go:391-396，页脚「重新翻译」/F5/Ctrl+R) → `skipAIResultCache = true` 后 `rerunCurrentMode()` → `runAction`(653-825)；`runAction`(675-687) 读 `useCache := !a.skipAIResultCache` 并立即清零，命中走 689-709 缓存路径，未命中走 711-742 网络路径；`storeAIResult`(583-599) 按 `aiResultOrder` 做 LRU 写入；`clearAICache`(612-618) 清三者
- **风险**：`skipAIResultCache` 在 `runAction` 里被无条件清零(678)，若 `rerunCurrentModeFresh` 之后因 `switchMode` 同 mode 早退(410-419) 或 AI 未启用降级而**未**走到 `runAction`，标志残留到下一次不相关的 AI 请求，导致意外绕过缓存；`aiResultCache` 与 `aiResultOrder` 分别置 nil，中途 panic 或只清其一会不一致；`actionFingerprint`(543-554)/`actionCacheVariant`(560-571) 生成的 key 未纳入版本，旧格式结果可能被复用
- **显式化**：`type AICachePolicy struct{ Entries map[string]string; Order []string; Limit int; SkipOnce bool; Version uint8 }` + `func (p *AICachePolicy) ConsumeSkipOnce() bool` + `func (p *AICachePolicy) Get/Put/Clear()` 保证 map 与 Order 同步；`rerunCurrentModeFresh` 改为 `a.cache.RequestFresh()` 返回一次性 token 随请求传递
- **测试**：**无**，缺口：SkipOnce 残留、LRU 淘汰、key 版本、缓存命中路径

#### U15 OCR / 截图会话 · 高（与 A4 是同一状态机的渲染侧视角，见 §4.4）
- **位置**：`fastpot/popup.go:85-91, 159, 170-171, 176-177`；`fastpot/popup_translation.go:53-57, 163-173, 204-241`；`fastpot/popup_ocr_pairs.go:39-224`；`fastpot/popup_view.go:44-48`；`fastpot/popup.go:398-405, 482-528, 566-604`
- **载体**：`ocrText string`(85)、`ocrPath string`(86)、`ocrCrop winRect`(87)、`ocrBusy bool`(88)、`ocrError string`(89)、`ocrGeneration uint64`(90)、`ocrCancel context.CancelFunc`(91)、`isScreenshotMode bool`(159)、`ocrCompare bool`(170)、`ocrExpanded bool`(171)、`screenshotPinned bool`(176)、`snipBitmap *ui.Bitmap`(177)
- **状态 / 迁移**：无截图 / 识别中 / 有结果 / 失败 / 已取消；`ocrGeneration` 防迟到结果代次。`TriggerScreenshotOCR`（定义在别组，selicon.go:1157-1160 调用）→ 置 `ocrBusy=true`、自增 `ocrGeneration`、抓图写 `snipBitmap`；识别回调按 `ocrGeneration` 判定迟到（测试 `popup_ui_test.go:112` 佐证）；`stopScreenshotOCR()` 在关闭按钮(popup_translation.go:53-57)、Escape(popup_view.go:44-48)、`switchMode`(popup.go:405) 三处调用；openPopup* Reset 段(482-528、566-604) 全清；`openPopupWithText`(600-603) 额外清 `pinned/screenshotPinned`
- **风险**：`screenshotPinned` 泄漏已有实锤(popup.go:600-603 兜底注释)；`stopScreenshotOCR()` 在三处调用但 `openPopupAction` 路径未调用，截图会话可能被新弹窗打断而不取消；`snipBitmap` nil 哨兵与 `ocrText` 空串可组合出「有图无文」「有文无图」等无意义状态，`popupContentHeight`(popup_layout.go:175-180) 却按 `isScreenshotMode && snipExpanded` 直接参与高度计算
- **显式化**：与 A4 合并为 `ScreenshotOCRSession`（`OCRSessionState` + `Invalidate()`），四个代次计数器统一命名为 `aiGen/translateGen/notesGen/ocrGen` 并抽 `type generation uint64; func (g *generation) Next() generation`
- **测试**：`popup_ui_test.go:82/112`。缺口：screenshotPinned 泄漏、openPopupAction 未取消会话、OCR 与 AI 精翻串联(popup_ocr_pairs.go:146-178)
- **复核备注（批次 5d / 6c）**：`screenshotPinned` 泄漏已由批次 5d 的 `resetScreenshotPinLocked()` 修掉（`openPopupAction` 也调用）；渲染侧三块是叠加而非互斥，已按 A4 的 `ocrViewPhase` / `classifyOCRView` / `ocrShowErrorBox` / `ocrShowEditor` 收口，控制流未动；`ScreenshotOCRSession` 与 `snipBitmap` 的完整会话对象留后续。

#### U16 选择手势触发策略 · 中（与 C1/C3/C4 重叠，见 §4.4）
- **位置**：`fastpot/selicon.go:934-958, 960-1015, 1026-1050, 1072-1095`；`fastpot/popup.go:158, 214`；载体定义 `fastpot/selection_policy.go:226-258`、`fastpot/hook.go:449-452`
- **载体**：`SelectionTrigger string`（取值 `"auto"`/`"hotkey"`/其它＝chip）、`gestureGate gestureGate`(popup.go:214)、`pausedUntil time.Time`(158)、`chipHoverArmed uint32`(hook.go)、`selectionGesture.Kind string`(selicon.go:973)
- **状态 / 迁移**：`"hotkey"` 仅快捷键 / `"auto"` 选区直接开弹窗 / 其它显示 chip；`gestureGate.allow` 返回 `(bool, why string)`。`applySelectionMode`(934-958) → `wantHooks := SelectionTrigger != "hotkey"` → `installHooks()` 或 `removeHooks()`；`onSelectionGesture`(967-1015) 依次过滤：`"hotkey"` 丢弃(975-978) → `typingSuppresses(g)`(984-988) → `shouldIgnoreSelection(owner)`(989-992) → `gestureGate.allow(now,px,py)`(996-999) → `captureForGesture` → `selectionTextProblem` → `deliverSelection`；`deliverSelection`(1037-1049) `"auto"` → `showPopupText(modeSelection, text)`，否则 `showChipAt`
- **隐性**：触发模式用裸字符串比较；拒绝原因 `(bool, string)` 的 `why` 只进日志不参与状态；门控顺序是隐式优先级链，散落在 7 个 early return
- **风险**：`applySelectionMode`(942-958) 注释自承认历史 bug：「`hookThreadOnce` (already done) ... `仅快捷键` was a one-way door that killed selection translation until the app was restarted」——hook 安装状态由 `sync.Once` 隐式持有且与 `SelectionTrigger` 字符串不同步；`onGlobalKeyDown`(1090-1095) 无条件隐藏 chip，与应用自身热键同时触发时会先隐藏再执行热键，用户可见闪烁
- **显式化**：`type SelectionTrigger uint8`（`triggerAuto/triggerChip/triggerHotkey/triggerPaused`）+ `type GestureVerdict uint8`（`gestureAccept/gestureSkipHotkeyOnly/gestureSkipTyping/gestureSkipBlocked/gestureSkipCooldown/gestureSkipEmpty/gestureSkipUnreadable`）+ `type HookState uint8`（`hooksUninstalled/hooksInstalling/hooksInstalled/hooksFailed`）替代 `hookThreadOnce`；`onSelectionGesture` 改为 `switch verdict := a.classifyGesture(g); verdict { ... }`
- **测试**：`selicon_hover_test.go`、`popup_ui_test.go:127/153`。缺口：trigger 字符串三分支、门控顺序、hook 安装单向门、typing 抑制

#### U17 grammarResultPrefill 预填合并标志 · 中
- **位置**：`fastpot/popup.go:112`；`fastpot/popup_actions.go:443-480, 623-651, 653-759`；`fastpot/popup_tabs.go:14-50, 58-138`。关键行：467-473 置 true、637-643 消费置 false
- **载体**：`grammarResultPrefill bool`(popup.go:112)、`grammarResult *SentenceAnalysisResult`(108)、`aiGen uint64`(78)、`tabViewState.grammarResultPrefill bool`
- **状态 / 迁移**：无预填 / 存在「由翻译结果预填的译文」等待 AI 结构化结果合并 / 过期预填。`runAction`(757-759) 仅 sentence-analysis 先 `prefillActionTranslation`；`prefillActionTranslation`(443-480) `if gen != a.aiGen { return }`(460-463)，若 `grammarResult == nil` 新建，`Translation == ""` 填入并 `grammarResultPrefill = true`(467-473)；`applyParsedAction`(637-643) `if parsed.Translation == "" && a.grammarResultPrefill && a.grammarResult != nil { parsed.Translation = a.grammarResult.Translation }` 后置 false；缓存命中路径(689-709) 直接置 false；`saveTabLocked`/`restoreTabLocked` 原样保存/恢复该 bool
- **隐性**：一个 bool 承载「预填来源 + 待合并 + 可覆盖」三重语义，与 `grammarResult.Translation != ""` 内容判断联合使用
- **风险**：`grammarResultPrefill` 只由 `applyParsedAction` 消费，若解析失败未调用（810-817 仅 `aiText != ""` 时调用），标志残留为 true 并被 `saveTabLocked` 存进 tab；下次恢复后若 `grammarResult.Translation` 已被别处改动，会把过期译文合并进新结果；`tabViewState` 保存该 bool 却不保存 `aiGen`，恢复后预填与代次脱钩
- **显式化**：`type TranslationPrefill struct{ Gen uint64; Text string; Consumed bool }` + `func (p *TranslationPrefill) Set(gen uint64, text string) / Consume(gen uint64) (string, bool)`；tab 快照保存整个 `TranslationPrefill`
- **测试**：**无**，全缺口（预填、消费、跨 tab 残留）

**ui 组补充**：下列字段声明于 `popup.go`、迁移在其他组文件，已由 A 组对应条目覆盖：`editingActionID`(132)、`customizingAction`(133)、`vocabSearchQuery`(134)、`vocabCategoryIdx`(142)、`vocabList`(143)、`vocabExpandedID`(147)、`selHoverTip`(136)、`selHoverIdx`(137)、`showNotes`(99)、`hotkeyDraftsInit`(231)、`pendingHotkeySelection/Input/Screenshot`(228-230)、`loadingModels`(233)、`magpieModels`(232)、`hotkeyMsg`(227)、`confirmClearVocab`(238)、`vocabConfirmDeleteID`(239)、`lastDeletedVocab`(240)、`newBlockedApp`(236)、`settingsScroll`(245)、`settingsPageSeen`(248)、`aiTest`(224)、`isQuitting`(155)、`pausedUntil`(158)、`actionSaveFeedback`(135)。ui 组建议统一为 `SettingsNavState`、`VocabSelectionState`、`HotkeyDraftState`、`AppSessionState{Quitting bool; PauseUntil time.Time}`。

**ui 组按风险排序 Top 3**：U1（一个 `aiGen` 被 6+ 处共享，点「AI 精翻」静默取消卡片流，无一条代次竞态测试）＞ U10（`selicon.go:683-686` 注释自承认「window becomes unclosable until restart」，迁移横跨三个执行上下文）＞ U6（`saveTabLocked` 原样保存 `aiBusy/notesBusy`，`restoreTabLocked` 强制置 false 且不自增 gen，切 tab 后旧请求写进新视图）。紧随：U8 窗口置顶已有真实泄漏事故、U3 生词提取与 U6 同源不同步、U11 流式期间高度可能锁死、U2 `switchMode` 直改 `busy=false` 而不失效 `translateGen`。

**ui 组共性结论**：本组共 4 套独立代次计数器（`aiGen`/`translateGen`/`notesGen`/`ocrGeneration`）、约 30 个状态布尔、多处 `""`/nil 哨兵与字符串状态值（`"AI 精翻"`、`"auto"`、`"hotkey"`、`page`），没有任何 `XxxState` 枚举或统一 transition 入口；两份 openPopup Reset 段手工列举 30+ 字段是状态泄漏的主要来源（已有 screenshotPinned 实证）。优先抽 `AIPass`/`ChipTipSession`/`TabSnapshot` 三个显式状态对象即可覆盖 Top 3 风险。


### 4.3 AI / OCR / 设置 / 词库（A1–A24）

#### A1 AI 流式响应生命周期（Magpie SSE） · 中
- **位置**：`fastpot/magpie.go:157-162, 169-181, 183-281`；切换点 200（effort 三哨兵）、217-225、227-230、237、248-250、251-257、259-264、269-279
- **载体**：`sawDone := false`(228)、`var sb strings.Builder`(227)、`effort string`(157 形参)、`ctx.Err()`/`sc.Err()`；`var magpieClient = &http.Client{Timeout: 120 * time.Second}`(25)
- **状态 / 迁移**：`sawDone=false + sb 空` 未收数据 / `false + sb 非空` 流式或提前截断 / `true` 完成 / `ctx.Err()!=nil` 取消 / `sc.Err()!=nil` 传输错 / `out==""` 空流；`data: ` 帧累积 sb 并 `onDelta`，`[DONE]`→`sawDone=true;break`(237)，坏 JSON→`continue`(248-250)，扫描结束按 sb/sawDone/ctx 返回 5 种结果串(269-279)。`onDelta` 在 `[DONE]` 前已把片段推给 UI
- **风险**：坏帧静默吞；提前结束错误在返回时才产生，但 UI 已把不完整答案当完成（注释 265-268 自认曾把截断 JSON 当 done）；`effort!="" && !="off" && !="none"` 三重字符串哨兵(200)，新值 `"0"` 会误发 reasoning
- **显式化**：`StreamPhase`（`idle/prefill/body/done/incomplete/canceled/failed`）+ `streamResult{Phase, Text, Err}`；`type ReasoningEffort string` + `EffortNone/EffortMinimal/...` + `NormalizeReasoningEffort()`。UX 按 Phase 显示「生成中 / 已截断可重试」
- **测试**：`ocr_quality_test.go:150`、`popup_streaming_test.go:13/31/48/66`、`live_probe_test.go:34`。缺口：`[DONE]` 缺失、坏帧、onDelta 已推送后报错

#### A2 推理强度与模型能力解析 · 中
- **位置**：`fastpot/magpie.go:60-66, 71-79, 83-85, 91-94, 97-112, 122-133, 432-479`；`fastpot/config.go:77, 369-387`
- **载体**：`var fastEfforts = []struct{ match string; level string }{...}`(60-66)、`var effortRank = map[string]int{...}`(83-85)、`var modelEfforts = struct{ sync.RWMutex; m map[string][]string }{...}`(91-94)、`AIEffort string // fast | deep`(config.go:77)
- **状态 / 迁移**：未探测（m 空）/已探测档位；偏好 `fast`/`deep`/`""`/`"off"`/`"none"`/`"high"`/任意未知值。`ResolveEffort`(122-133) deep→`"high"`，fast→`fastEffortFor`→`lowestReasoningLevel`→`""`；`FetchAvailableModels`(432-479) 474-476 整体 `modelEfforts.m = levels`；`normalizeAIEffort`(378-386) 未知值一律归 fast
- **风险**：`ResolveEffort` 在 `FetchAvailableModels` 前/后对同一模型返回不同 wire 值（探测有时序依赖，settings.go:370-388 每次 showSettings 才拉）；474-476 整体覆盖丢弃旧缓存已探测模型；未知配置值被吞
- **显式化**：`type EffortPreference string` + `EffortFast/EffortDeep`；`type ModelCapability struct{ Model string; Levels []ReasoningEffort; ProbedAt time.Time }`；`EffortResolver.Resolve(pref,model) ReasoningEffort`；`ParseEffortPreference(s) (EffortPreference,error)` 取代 default 兜底
- **测试**：`effort_test.go:16/49/64/93`、`effort_migrate_test.go:13/59`、`effort_startup_test.go:15`。缺口：探测前后不一致、整体覆盖丢数据、未知值

#### A3 OCR 引擎降级流水线（含预处理静默降级） · 中
- **位置**：`fastpot/ocr.go:82-121, 141-157, 164-175, 178-226`；`fastpot/indra_ocr.go:342-380, 672-688`
- **载体**：`noop := func() {}`(ocr.go:83) 与返回的清理闭包；`indraLoaded bool` + `indraErr error` + `indraInst *indraEngine`(indra_ocr.go:76-81)
- **状态 / 迁移**：Indra 成功 / 无文本 / 失败→Windows OCR；预处理缩放成功（临时文件 + remove 闭包）vs 四种失败（原图 + noop）。`RecognizeImageFileIn`(141-157) `indraAvailable()`→`indraRecognizeFile`→`err==nil && CleanOcrText(text)!=""` 成功，否则 log 后 `windowsRecognizeFileIn`；`prepareOCRImage`(82-121) 95-99/104-109/111-115/116-119 四处失败均 `return path, noop, nil`；`windowsRecognizeFileIn`(178-226) 15s timeout、`ctx.Err()`→「已取消或超时」、ExitError→stderr 首行、空文本→「没有识别到文字」
- **风险**：四处预处理失败静默降级为大图，用户只看到「没有识别到文字」而非「图片过大」；`indraRecognize` 360-362 `len(boxes)==0 → "", nil` 空文本与成功同形；374-376 单行失败被吞；`classifyScript` 541-562 任一失败 `return scriptCJK` 把错误吞成默认中文；降级原因只进 log 不进返回值
- **显式化**：`OCREngine`（Indra/Windows）+ `OCRStage`（Preprocess/Detect/Recognize/Postprocess）+ `type OCRResult struct{ Text string; Engine OCREngine; Stage OCRStage; Degraded bool; Cause error }`；预处理返回 `PreprocessResult{Path, Cleanup, Degraded, Err}`。UX 结果角标显示「Indra / Windows OCR（已降级）」
- **测试**：`indra_test.go:150`、`ocr_quality_test.go:25/63`、`ocr_text_test.go:8`。缺口：降级原因透传、四种预处理失败、空文本 vs 失败区分
- **复核备注（批次 9g）**：Windows OCR 预处理的四条静默降级已显式记录（`ocrPreprocessResult.Degraded` / `Reason`），空结果提示按降级区分并提示缩小截图范围；`prepareOCRImage` 原签名保留。Indra 空文本与成功同形、`classifyScript` 失败吞成默认中文、`OCRResult` 完整结构仍未做。

#### A4 截图 OCR 会话状态机 · 高（ai 组 Top 1）
- **位置**：`fastpot/ocr.go:230-237, 239-251, 255-297, 301-311, 313-356, 358-388, 392-472`；声明 `fastpot/popup.go:85-91, 159, 167, 170-171, 176, 289-298`
- **载体**：`ocrBusy bool`(popup.go:88)、`ocrGeneration uint64`(90)、`ocrCancel context.CancelFunc`(91)、`ocrError string`(89)、`ocrText string`(85)、`isScreenshotMode bool`(159)、`screenshotPinned bool`(176)、`tabStates map[popupMode]*tabViewState`(167)
- **状态 / 迁移**：空闲 / 框选中 / 已固定待识别(`ocrBusy=true`) / 识别中 / 成功 / 失败(`ocrError!=""`) / 已取消 / 编辑修正中。`TriggerScreenshotOCR`(232)→`beginScreenshotOCR`(248 stop、249 open、250 recognize)→`openPinnedScreenshot`(274-288 **一次裸赋值 30+ 字段**)→`recognizeScreenshot`(314 stop、317 `ocrGeneration++`、318 取 generation、320 `ocrBusy=true`、329 goroutine)；332 `if generation != a.ocrGeneration || !a.isScreenshotMode` 丢弃过期结果；336-350 写成功/失败；425-430 取消按钮 `stopScreenshotOCR()` + 写 `ocrError`；421/434/449 `renderScreenshotSource` 依 if 顺序渲染三态
- **隐性**：注释 253-254 自称 "structured OCR state"，实际是 `a.mu` 下 30+ 裸字段赋值；无 `ScreenshotOCRSession`/`SessionPhase`；`ocrError` 同时承担状态与文案
- **风险**：**单次截图启动 `ocrGeneration` 自增 3 次**（248 stop、283 设 busy、314 stop、317 再++），靠「最后一次赋值恰好一致」维持正确；取消路径 425-430 只写 `ocrError`，goroutine 332 只校验 generation 与 isScreenshotMode，**取消后旧结果仍可写回**；421/434/449 三态靠 if 优先级，`ocrBusy` 与 `ocrError` 可同时为真导致渲染混乱；248 与 314 重复 stop 造成状态抖动
- **显式化**：`ScreenshotOCRPhase`（`Idle/Snipping/Pinned/Recognizing/Editing/Error/Canceled`）+ `type ScreenshotOCRSession struct{ Phase; Generation uint64; Cancel context.CancelFunc; Path, Text string; Cause error; Compare, Expanded bool }`；`openPinnedScreenshot` 收成一次 `a.session = ScreenshotOCRSession{...}`；`stopScreenshotOCR` 迁到 `OCRCanceled` 且只递增一次代次
- **测试**：`popup_ui_test.go:82/112`、`ai_generation_race_test.go:55/201/314/374`。缺口：取消后旧 goroutine 写回、generation 多次自增、busy+error 同时为真的渲染
- **复核备注（批次 4f）**：本条「风险」、§5.1 ai 组 Top 3、§8 JSON 的 A4 anchor 里「取消后旧结果仍可写回」与「`ocrBusy` / `ocrError` 可同时为真」是审计当时的判断，经代码复核**在当前代码不成立**，请勿据此再加守卫。`stopScreenshotOCR()` 会自增 `ocrGeneration` 并 `cancel()`，goroutine 的 `generation != a.ocrGeneration || !a.isScreenshotMode` 已丢弃过期结果，`RecognizeImageFileIn` 也尊重 ctx；两处启动都置 `ocrBusy, ocrError = true, ""`，失败 / 取消先置 busy=false 再写 error，`snap()` 持锁，两标志同时为真不可达。批次 4f 实地确认的缺陷是另一入口：新截图清空 `source` / `target` 却不失效 `translateGen`，上一轮 OCR 译文的在飞翻译会把旧译文写进新卡片；已在 `recognizeScreenshot` 锁内补一次自增，并加 `TestScreenshotOCRInvalidatesInFlightTranslation`（先红后绿）与 `TestCanceledScreenshotOCRDropsLateResult`（把「取消不写回」钉成规格）。`ScreenshotOCRSession` 结构性显式化与 X1 留阶段 C。详见 [STATE_MACHINES_FIX_PLAYBOOK.md](STATE_MACHINES_FIX_PLAYBOOK.md) §5 / §7
- **复核备注（批次 6c）**：`renderScreenshotSource` 的三块是叠加渲染而非互斥三态（只有 `s.ocrBusy` 分支 `return`，错误框与编辑区在 error 态同时显示），故未按审计建议收成 `ScreenshotOCRPhase` 的互斥 switch，而是新增 `ocrViewPhase` 枚举 + `classifyOCRView` / `ocrShowErrorBox` / `ocrShowEditor` 三个纯谓词，控制流与渲染一字未动，零行为变化。`ocr_view_test.go` 的 `TestOCRRenderDecisionsMatchLegacy` 用 16 组组合钉住新旧判定逐项一致；`ScreenshotOCRSession` 的完整会话对象仍留后续。

#### A5 Indra 引擎单例懒加载 · 中
- **位置**：`fastpot/indra_ocr.go:58-74, 76-81, 179-191, 194-274, 277-322, 672-688`；`fastpot/indra_image.go:34-37`
- **载体**：`var ( indraMu sync.Mutex; indraInst *indraEngine; indraErr error; indraLoaded bool )`(76-81)；`closeOnce sync.Once`(58-74)；`var indraPadX = 0.3`(indra_image.go:37，注释 34-36 明说「variable, not a constant, because it is the knob the doubling sweep turns」)
- **状态 / 迁移**：未加载 / 就绪 / 永久失败；`recLatin==nil`、`script==nil`、`vocabLatin==nil`、`window<1→1`(317-320) 表达可选模型缺失。`indraGet()`(179-191) `182-184 if indraLoaded { return indraInst, indraErr }`；**185 加载前就置 `indraLoaded = true`**；186 `indraInst, indraErr = indraLoad()`；187-189 失败仅 log；`indraLoad` 262-264/265-267 可选模型失败静默、308-311 Latin 词表失败静默
- **风险**：**首次加载失败被永久缓存，进程生命周期内不再重试**；`indraAvailable()`(672-675) 与 `indraStatus()`(678-688) 都调 `indraGet`，设置页「未启用/已就绪」取决于「何时第一次被调用」，先开设置页会提前触发加载；`indraStatus` 684-686 只用 `recLatin==nil` 判「仅中文识别器」，与 `readLine` 505 的三条件不一致；`indraPadX` 全局可变无同步、测试可改写
- **显式化**：`IndraLoadState`（`Unloaded/Loading/Ready/Degraded/Failed`）+ `type IndraRuntime struct{ State; Inst; Err; FailedAt time.Time; RetryCount int; Caps IndraCaps }` + `type IndraCaps struct{ HasLatin, HasScript, LatinVocabReady bool; Window int }`；`indraGet` 对 `IndraFailed` 按冷却重试；`indraPadX` 收进只读 `indraConfig`
- **测试**：`indra_test.go:150/20/39/75/108/179/209/251`。缺口：失败后重试成功、可选模型缺失、设置页提前触发加载、indraPadX 并发
- **复核备注（批次 7g）**：首败永久缓存属实。已加 `indraFailedAt` + 30 秒冷却重试，`indraLoadEngine` 作测试接缝；`IndraLoadState`/`IndraCaps` 结构化与 `indraStatus`、`readLine` 能力判断统一未做，留后续。

#### A6 Indra 识别器 / 脚本选择能力矩阵 · 中
- **位置**：`fastpot/indra_ocr.go:500-522, 526-531, 538-570, 576-610, 634-669, 678-688`
- **载体**：`const ( scriptBase = iota; scriptCJK; scriptRTL; scriptWorld )`(526-531，裸 int 无类型名)；`e.script *ort.Session`、`e.recLatin *ort.Session`、`e.vocabLatin []string`(58-74)
- **状态 / 迁移**：505 三哨兵 `script!=nil && recLatin!=nil && len(vocabLatin)>0`＝启用脚本分类，否则纯 CJK；`scriptWorld/scriptRTL`→Latin 识别器；`scriptCJK || large`→CJK；default→Latin。`readLine`(500-522) 默认 CJK 参数(501-503)→505 判断→`classifyScript`(538-570)→510-517 `switch` 覆盖 sess/height/vocab/window→520-521 crop+run
- **风险**：错误吞为默认 CJK 会让拉丁文本被 CJK 识别器读成乱码且不报错；`scriptBase` 从未作为返回值使用（不可达状态）；`lineLooksCJK`(634-669) 注释 632-633 称它 "decides which recognizer reads it"，但 `readLine` 实际未调用——**决策逻辑与注释不同步（疑似死代码）**；`indraStatus` 与 readLine 能力判断不一致
- **显式化**：`type ScriptKind uint8`（`Unknown/CJK/RTL/World`）+ `type RecognizerKind uint8`；`func (e *indraEngine) chooseRecognizer(s ScriptKind, box lineBox) RecognizerChoice` 返回显式结构；删除或接入 `lineLooksCJK`；`indraStatus()` 直接读 `IndraCaps`
- **测试**：`indra_test.go:251/20/39/75/179/209`。缺口：scriptWorld/RTL 分支、classifyScript 失败路径、Caps 与 status 一致性
- **复核备注（批次 9m）**：脚本组已改为 `scriptKind` 具名类型 + `String()`，`classifyScript` 返回该类型，`readLine` 统一走 `preferCJKRecognizer`（严格复刻旧 switch：大行或 cjk 组走 CJK，base/rtl/world 走 Latin）；死代码 `lineLooksCJK` 已删除；`indraStatus` 改为要求 `recLatin`、`script`、`vocabLatin` 三者齐备。完整 `RecognizerKind` / `IndraCaps` 结构与 `chooseRecognizer` 返回显式结构未做，留后续。

#### A7 拼写修复词典懒加载 · 低
- **位置**：`fastpot/ocr_spell.go:28-48, 116-142`
- **载体**：`var ( spellOnce sync.Once; spellSet map[string]struct{} )`(31-34)；`changed := false`(121)
- **状态 / 迁移**：`spellSet==nil` 未加载 / 非 nil 已加载（一次性不可重置）。`spellWords()`(36-48) `spellOnce.Do(...)` 内解析 `spellData`；`fixDoubledLetter`(68-94) 87/90 判词
- **风险**：加载失败（embed 数据格式错）会把空 map 永久固定，`fixDoubledLetter` 87/90 判词全失败但静默；`sync.Once` 不可重置使测试无法隔离
- **显式化**：`type SpellDict struct{ Words map[string]struct{}; Loaded bool; Err error }` + `func LoadSpellDict(data string) (*SpellDict, error)` 纯函数 + `var spellDict atomic.Pointer[SpellDict]`
- **测试**：`ocr_spell_test.go:16/40/55/76/88/105/119/135`。缺口：加载失败、Once 不可重置、并发首次加载

#### A8 配置加载 / 迁移多阶段 · 高（ai 组 Top 2）
- **位置**：`fastpot/config.go:31-112, 148-170, 213-270, 369-387, 389-462, 588-632, 637-645`
- **载体**：`type store struct { mu sync.RWMutex; saveMu sync.Mutex; cfg Config; path string; lastSaveErr error; notesMigrated bool; effortMigrated bool; retiredViewMigrated bool }`(213-239)；`NotesMigrated int`(100)、`AIModeIsChoice bool`(107)、`Onboarded bool`(111)；`var activeStore *store`(637)
- **状态 / 迁移**：首次运行(`!load()`)/正常加载/JSON 解析失败→默认值/三种迁移标志任意组合/已回写/未回写（下次启动重跑）。`newStore()`(241-270) **else-if 链**：248 首次运行写默认、253 notesMigrated 回写、259 effortMigrated 回写、263 retiredViewMigrated 回写。`load()`(389-462)：400-403 解析失败 log+`return false`；404-408 `ensureDefaultActions` 置 retiredViewMigrated；409 `normalizeInitialAIMode`（`"dict"`→`""`+IsChoice=false）；412-419 rawMode 比较；431-440 `marker.NotesMigrated == nil` 键存在性判断；441 `normalizeAIEffort`；444-451 rawEffort 比较置 effortMigrated；452-457 数值兜底
- **隐性**：迁移无 `MigrationStep` 枚举或版本号，靠「键是否出现」、字符串比较、三个 bool、`AIModeIsChoice` 区分用户选择 vs 继承 shipped default
- **风险**：**一次启动最多回写一种迁移**（else-if），同一旧文件若同时触发 notes+effort+retiredView，后两种不回写、下次启动重复清理（注释 223-225/236-237 自认该缺陷）；400-403 解析失败被吞为 `DefaultConfig()` 且 `return false`，磁盘文件未被覆盖，用户设置「每次启动都恢复默认」；379-386 任意未知值静默归 fast
- **显式化**：`type ConfigVersion uint16` + `ConfigV1/ConfigV2/...`；`type Migration struct{ From, To ConfigVersion; Apply func(*Config)(bool,error) }` + `var migrations []Migration`；`func Migrate(cfg *Config, raw map[string]json.RawMessage) (ConfigVersion, []Migration, error)` 一次跑完并**一次**回写；`Config.SchemaVersion`
- **测试**：`effort_migrate_test.go:13/59`、`effort_test.go:93`、`popup_ui_test.go:39/61/228/264`、`regression_batch_test.go:333/346/361`、`actions_test.go:165`。缺口：**三种迁移同时触发**、解析失败后磁盘未覆盖、未知 AIEffort
- **复核备注（批次 5a）**：本条「风险」、§5.1 Top 10 第 8 行、§4.3 ai 组 Top 3、§8 JSON 的 A8 anchor 里「一次启动最多回写一种迁移」与「解析失败后磁盘文件未被覆盖」是审计当时的判断，经代码复核**都不成立**，请勿据此再加守卫。三个 `else-if` 分支都调 `saveConfig(s.path, s.Get())` 写全量内存 `cfg`，一次写回就持久化全部迁移，`else-if` 只影响日志文案；`load()` 解析失败 `return false` 会让 `newStore` 的 `if !s.load()` 当成首次运行，随即 `saveConfig(s.path, s.cfg)` 用 `DefaultConfig()` **覆盖用户损坏的 config.json**，AI URL / 主题 / 自定义动作永久丢失且无副本可恢复，比审计写的「每次启动都恢复默认」更严重。批次 5a 已在 `newStore` 第一分支用 `os.Stat` 区分「文件不存在」（写默认）与「文件存在但损坏 / 不可读」（保留文件、仅用内存默认、打可执行指引日志），并加 `TestCorruptConfigIsNotOverwritten`（先红后绿）与 `TestFirstRunStillWritesDefaults`（对照，证明首次运行不受影响）。`ConfigVersion` / `Migration` 枚举化与 X1 等仍留阶段 C。详见 [STATE_MACHINES_FIX_PLAYBOOK.md](STATE_MACHINES_FIX_PLAYBOOK.md) §5 / §7

#### A9 配置保存错误 / 重试与一次性提示 · 中
- **位置**：`fastpot/config.go:213-239, 306-310, 315-331, 344-367`；`fastpot/settings.go:117-135, 167-195`；`fastpot/vocabulary.go:988-992`
- **载体**：`lastSaveErr error`(config.go:220，注释「records the most recent persistence failure so settings can tell the user their change is only in memory」)；`actionSaveFeedback string`(popup.go:135)、`actionSavedMsg string`(popup.go:126)
- **状态 / 迁移**：无错误 / 保存中（「正在重试保存设置…」）/失败（红色横幅 + 重试）/重试成功/Toast 待消费/已消费。`Update()`(344-367) 352 先 `s.lastSaveErr = nil` 再 `saveConfig`，失败 363-365 写回；`RetrySave()`(315-331) 317 `saveMu.Lock`、328 写 err；`settingsView`(117-135) 依据 `LastSaveError()` 出横幅；167-176 加锁读两个消息串并立即清空（读后清空的一次性消息）
- **风险**：`Update` 352 先清 `lastSaveErr` 再保存，连续 Update 时失败横幅短暂消失再出现（文案闪烁）；settings.go 注释 164-166 自认不加锁曾是 data race；**`vocabulary.go:988-992 saveOrLog()` 保存失败只 `log.Printf`，UI（settings_vocab.go:307/332/359/380/408）仍显示「成功导入 ✓ / 已成功导出」——config 有 lastSaveErr 而词库没有，两套持久化状态不一致**
- **显式化**：`SaveStatus`（`Idle/Pending/Failed/Saved`）+ `type SaveState struct{ Status; Err error; At time.Time; Generation uint64 }`；`type UserNotice struct{ Kind NoticeKind; Text string; ID uint64 }` 取代两个消息串（按 ID 消费）；`VocabularyStore.LastSaveError()` + `saveOrReport() error`
- **测试**：`atomic_save_test.go:120/74/108`、`regression_test.go:40`。缺口：lastSaveErr 与横幅一致性、Toast 并发消费、词库保存失败透传
- **复核备注（批次 9f）**：词库保存失败现已透传到界面 —— `VocabularyStore.lastSaveErr` + `LastSaveError()`，设置页撤销/删除与卡片三个快速保存在提示成功前检查，失败显示原因且不置已保存标志。`config.Update()` 的 lastSaveErr 先清后写造成横幅闪烁、两个消息串的 Toast 并发消费仍未动，`SaveStatus` / `UserNotice` 枚举化留后续。

#### A10 配置 / 词库原子保存局部阶段 · 中
- **位置**：`fastpot/config.go:464-520`；`fastpot/vocabulary.go:117-163`
- **载体**：`closed := false`(config.go:480)；`tmp := path + ".tmp"`(vocabulary.go:136)；`for attempt := 0; attempt < 6; attempt++`(config.go:509)
- **状态 / 迁移**：临时文件打开/已写/已 Sync/已 Close(`closed=true`)/rename 重试 0..5/成功/失败并 Remove。`saveConfig` 481-486 cleanup 闭包依据 `closed` 决定 Close+Remove；487-503 Write/Sync/Chmod/Close；504 `closed=true`；509-517 rename 失败 sleep 10~50ms 重试；518-519 最终 Remove+返回。`vocabulary.save()` 同类但**无重试**，仅 162 `os.Rename`，另 159-161 写 `.bak`
- **风险**：Windows 文件被占用时 config 重试 6 次、词库直接丢写（注释 vocabulary.go:41-45 说加 `saveMu` 只解决并发写同一 .tmp，没解决外部占用）；499-503 Close 失败时手动 Remove，`closed=true` 却在 504 之后置位，路径分叉易漏；160 `os.WriteFile(path+".bak", ...)` 错误被 `_ =` 吞
- **显式化**：`AtomicSavePhase`（Open/Written/Synced/Closed/Renaming/Done/Failed）+ `func atomicWrite(path string, data []byte, opts AtomicWriteOptions{Retries int; Backup bool}) error` 抽出共用实现，`config` 与 `vocabulary` 都调
- **测试**：`atomic_save_test.go:46/74/108/120`、`regression_test.go:40`、`vocabulary_test.go:14`。缺口：词库 rename 被占用、closed 分叉、.bak 写失败
- **复核备注（批次 8d）**：两套实现与「词库无 rename 重试」属实。已抽出 `atomicWriteFile`（唯一临时名 + Write/Sync/Chmod/Close + 可选备份 + 6 次 rename 重试），`saveConfig` 与 `vocabulary.save()` 都改调它，词库因此获得与配置一致的抗瞬时占用能力，两处共减少约 80 行重复。`AtomicSavePhase` 枚举化与 `.bak` 写失败上报未做，留后续。

#### A11 Config 字符串枚举族与 UI idx 映射 · 中
- **位置**：`fastpot/config.go:31-112`；`fastpot/settings_pages.go:89-120, 183-200, 291-374`；`fastpot/settings.go:96-106, 136-155`
- **载体**：`EngineMode string // race | microsoft | google | tencent`(50)、`SelectionTrigger string`(59)、`SelectionCapture string // auto | copy | uia`(61)、`AutoPronounce string // off | uk | us`(70)、`ThemeMode string`(83)、`AIMode string // "translate" | "grammar" | "dict" | "polish"`(93)、`AIEffort string`(77)、`page string`(popup.go:97)
- **状态 / 迁移**：每个字段用注释里的裸字符串集合；UI 侧 `idx := 0` + `switch cfg.X` + `mode := []string{...}[idx]` 在 5 处重复（93-102、108-117、187-198、311-338、363-373）；`a.page` 由 `ui.Sidebar(c, &a.page, ...)`(96-106) 直接绑定 9 个字符串。`pageAI`(311-339) 312-319 读 `AIModeIsChoice`/`AIMode` 算 idx，320 Segmented 的 OnChange 330 `modes := []string{"", "grammar", "translate"}`、332-337 写回；`settingsView` 136-155 `switch a.page` 分发 9 页，default→`pageAbout`
- **风险**：**`settings_pages.go:313-318` 把 `"polish"` 映射到 idx=2，但 330 的 modes 没有 polish，写回变成 `"translate"`——`"polish"` 值不可逆丢失**；`normalizeInitialAIMode`(config.go:165-170) 只处理 `"dict"` 不处理 polish；`AIModeIsChoice=false` 时 UI 显示「跟随翻译」但配置里仍留旧值；136-155 `default→pageAbout` 让拼错/未来 page 静默落页
- **显式化**：`type EngineMode string` + `EngineRace/EngineMicrosoft/...`；同样定义 `SelectionTrigger/SelectionCapture/AIMode`（含 `AIModePolish`）、`ThemeMode`、`SettingsPage`；`func (m AIMode) SegmentedIndex() int` 与 `AIModeFromIndex(i int) AIMode` 消除 5 处重复映射
- **测试**：`regression_batch_test.go:292/333/346/361`、`regression_test.go:104/120`、`selicon_test.go:131/143/152`。缺口：**polish 往返保持**、未知 AIMode/page/EngineMode

#### A12 设置页导航与窗口生命周期 · 中
- **位置**：`fastpot/settings.go:68-72, 75-177, 410-445`；`fastpot/settings_pages.go:506-530`
- **载体**：`page string`(popup.go:97)、`settingsPageSeen string`(popup.go:248)、`settingsScroll`、`cfgWin *mygo.Window`、`isQuitting bool`
- **状态 / 迁移**：`page==""`（首次→「general」）/9 个页面/切换未刷新滚动/窗口已存在（Show+Focus）/窗口 Hide（OnClose PreventDefault）/正在退出（不拦截关闭）。`settingsView`(80-86) `if a.page == "" { a.page = "general" }`、`if a.settingsPageSeen != a.page { a.settingsPageSeen = a.page; a.settingsScroll.Y = 0 }`；96-106 Sidebar 直接改 `a.page`；`openAISettings`(411) `a.page = "ai"`；`showSettings`(417-445) 418 `hotkeyDraftsInit=false`、420 `loadMagpieModels()`、421 清 `vocabConfirmDeleteID`、435-441 OnClose 依 `isQuitting`
- **风险**：`settingsPageSeen` 靠 UI 帧轮询比对，同一帧内两次改 page（411 赋值后 413 `settingsRefresh`）可能丢滚动重置；`showSettings` 每次调用都 `loadMagpieModels()`(420) 发起 3s 网关请求，反复打开设置堆积 goroutine 并互相覆盖 `magpieModels`
- **显式化**：`type SettingsPage string` + `PageGeneral/PageSelection/...`；`type SettingsNav struct{ Current, Previous SettingsPage; Scroll ui.ScrollState }` + `func (n *SettingsNav) Go(p SettingsPage) bool`；`SettingsWindowState`（Hidden/Showing/Closing/Quitting）
- **测试**：`settings_layout_test.go:24/65`、`regression_batch_test.go:31`、`actions_test.go:211`。缺口：同帧切页、重复 showSettings 并发探测、isQuitting 与 Hide 组合
- **复核备注（批次 9e）**：重复 `showSettings` 的并发探测已修：`loadMagpieModelsWithPolicy` 在 `loadingModels` 时直接返回，五分钟内且网关地址未变时复用缓存，并用 `magpieModelsGen` 丢弃旧代次结果；「🔄 刷新」仍强制拉取。同帧切页与滚动重置未动，`SettingsPage`/`SettingsNav` 枚举化留给后续。

#### A13 热键草稿 / 应用状态 · 中
- **位置**：`fastpot/settings.go:241-304, 306-368`；`fastpot/settings_pages.go:401-503`
- **载体**：`hotkeyDraftsInit bool`(popup.go:231)、`pendingHotkeySelection/Input/Screenshot string`(228-230)、`hotkeyMsg string`(227)
- **状态 / 迁移**：草稿未初始化/已初始化（cfg 拷贝）/已编辑/冲突/应用成功/应用失败。`initHotkeyDrafts()`(351-360) `352 if a.hotkeyDraftsInit { return }`、356-359 拷贝 cfg、359 置 true；`showSettings`(418) 置 false 强制重初始化；`applyHotkeys()`(306-349) 308 初始化、校验→冲突写 `hotkeyMsg` 并 return、成功 332-336 `store.Update` + 340-348 `registerHotkeys`；`pageHotkey`(403) 每帧初始化、405 算 `conflictMsg`
- **风险**：用文案子串反推状态，`strings.Contains(a.hotkeyMsg, "冲突")||Contains(...,"失败")`(settings_pages.go:492) 改字就误判颜色；`showSettings` 每次重置 `hotkeyDraftsInit=false`，**未保存草稿被静默丢弃**；`applyHotkeys` 成功分支 340-348 `registerHotkeys(prev...)` 传的是更新前的 `prev`，需核对是否注册旧值；`regression_test.go:120 TestHotkeySaveIsNotAlwaysTrue` 说明历史上存在「总是报成功」缺陷
- **显式化**：`type HotkeyDraft struct{ Selection, Input, Screenshot string; Initialized bool }` + `type HotkeyApplyResult struct{ OK, Conflict bool; Registered [3]string; Err error }` + `func (a *app) applyHotkeys() HotkeyApplyResult`；`hotkeyMsg` 只作 `UserNotice`
- **测试**：`regression_test.go:120`、`main_test.go:66`。缺口：文案反推、草稿丢弃、registerHotkeys 用新值验证
- **复核备注（批次 8c）**：`strings.Contains` 文案反推属实；已改为 `hotkeyMsgKind`（none/success/error）+ `setHotkeyMsgLocked`，渲染只看枚举，成功仍用强调色。`HotkeyDraft` 与 `HotkeyApplyResult` 结构体化、草稿丢弃问题未做，留后续。
- **复核备注（批次 10a）**：「`registerHotkeys(prev...)` 是否注册旧值」已核对：失败分支确实回滚到更新前的旧值，且不写盘。缺口在回滚本身：旧键被别的程序占走时回滚失败被丢弃，用户只看到「新键注册失败」，实际三个快捷键可能全部失效。已把回滚结果并入提示（「原快捷键也未能恢复：…」），成功分支补 `rebuildTray()` 让托盘菜单里的快捷键标签跟着更新；三处「加锁写提示 + 刷新窗口」收成 `setHotkeyMsg`。新增 `TestHotkeyRollbackFailureIsReported`（先红后绿）。草稿丢弃与结构体化仍未做。

#### A14 网关测试与模型列表探测 · 中
- **位置**：`fastpot/settings.go:209-239, 370-405`；`fastpot/settings_pages.go:375-395`
- **载体**：`aiTest string`(popup.go:224)、`magpieModels []string`(popup.go:232)
- **状态 / 迁移**：`aiTest==""`/「测试中…」/「正常（…）」/「失败：…」；`len(magpieModels)>0`＝已探测，否则用两个硬编码模型（settings.go:396/400）。`testGateway()`(209-239) 212 写「测试中…」→217 goroutine（20s timeout）→221 `StreamMagpie`→231-233 写结果；`loadMagpieModels()`(370-388) 376 `FetchAvailableModels`→380 `a.magpieModels = models`→384 `cfgWin.Update`；`getAvailableModels()`(389-405) 392 `if len(a.magpieModels) > 0` 否则返回硬编码
- **风险**：**连点两次「测试网关」，先发后至的旧结果覆盖新结果**（无代次、无 cancel）；`loadMagpieModels` 每次 showSettings 都跑，多次并发返回顺序不定、380 后写覆盖先写；392 把「探测失败」降级为硬编码模型，355 的「当前已加载 %d 个模型」把回退列表算作真实探测
- **显式化**：`GatewayProbeState`（`Idle/Running/OK/Failed`）+ `type GatewayProbe struct{ State; Generation uint64; Text string; Latency time.Duration; Err error }`；`type ModelCatalog struct{ Models []string; Source ModelSource; ProbedAt time.Time }` 区分「探测结果/回退列表」；点测试递增 Generation 并 cancel 旧 ctx
- **测试**：`live_probe_test.go:34`、`effort_test.go:64`、`effort_startup_test.go:15`。缺口：连点竞态、探测失败标注、aiTest 三态
- **复核备注（批次 9i）**：`testGateway` 的连点竞态已修 —— `aiTestGen` 代次 + `aiTestCancel` 取消上一次探测，写回前比对代次，`streamMagpieProbe` 作为测试接缝。`GatewayProbeState` 完整枚举化与模型目录来源标注仍未做。
- **复核备注（批次 10a，多服务商接入）**：本条已整体显式化，随「AI 精翻接入多家模型服务」一起落地。`aiTest`/`aiTestGen` 换成 `aiProbeState{phase: Idle/Running/Succeeded/Failed, source, gen, cancel}`；`magpieModels` 换成 `aiModelsState{phase: Idle/Loading/Ready/Unsupported/Failed, source, models, at, gen}`。来源 = 服务 + 地址 + Key 指纹，写回前比对代次与来源，配置一变旧结果按 Idle 显示；Magpie 回退模型与真实探测结果分开，且不泄漏给直连服务。规格与守护测试见 [AI_SERVICES.md](AI_SERVICES.md) §6–§7。

#### A15 AI 指令定制编辑视图 · 中
- **位置**：`fastpot/settings_vocab.go:10-245`；声明 `fastpot/popup.go:132-135`
- **载体**：`editingActionID string`(popup.go:132)、`customizingAction CustomActionConfig`(133)、`actionSaveFeedback string`(135)
- **状态 / 迁移**：`editingActionID==""` 列表视图 / `!= ""` 定制视图；`customizingAction` 是编辑中副本；feedback 空/恢复默认/已保存。71-75 点「定制化」→设 ID+拷贝+清 feedback；103-105 返回→`editingActionID=""`；111-118 恢复默认→`ResetAction`+覆盖副本+写 feedback；228-238 保存→`store.Update` 按 ID 写回 `CustomActions[i]`+feedback+`editingActionID=""`；239-241 取消→只清 ID
- **风险**：**点「取消」只清 `editingActionID`，`customizingAction` 仍留脏副本**，下次点另一条会覆盖(72-73)；109 `strings.HasPrefix(a.editingActionID, "default-")` 把「能否恢复默认」绑在 ID 命名约定上；131/171/185/212 直接绑 `&a.customizingAction.X`，编辑中途切页不丢草稿也无提示；228-238 保存后不校验 ID 是否仍存在
- **显式化**：`ActionEditorState`（List/Editing/Dirty/Saving）+ `type ActionEditor struct{ State; TargetID string; Draft, Original CustomActionConfig; Dirty bool }` + `Begin/Commit/Cancel`；`CustomActionConfig.Builtin bool` 取代 ID 前缀判断
- **测试**：`actions_test.go:126/211/165`。缺口：取消后脏副本、ID 失效保存、脏状态提示

#### A16 生词本页交互状态 · 中
- **位置**：`fastpot/settings_vocab.go:247-631`；`fastpot/settings.go:421`；声明 `fastpot/popup.go:134, 142, 147, 238-240`
- **载体**：`vocabCategoryIdx int`(142)、`vocabSearchQuery string`(134)、`vocabConfirmDeleteID string`(239)、`vocabExpandedID string`(147)、`confirmClearVocab bool`(238)、`lastDeletedVocab *VocabularyEntry`(240)
- **状态 / 迁移**：4 分类（431-439 `catKey` "all"/"words"/"sentences"/"writing"）/搜索有无/列表空/展开某条/删除待确认/清空待确认/可撤销删除。272 Segmented 改 idx（273 清 confirmDelete）；278 搜索改 query；282-292 撤销（Add 后置 nil）；401-425 清空两态；484/551-557 展开切换；561-588 删除两态+567 记 `lastDeletedVocab`；`showSettings`(421) 只清 `vocabConfirmDeleteID`
- **风险**：`vocabConfirmDeleteID` 可能指向已删除 ID（列表刷新后 561 不匹配，字段残留）；**`showSettings` 421 只清 `vocabConfirmDeleteID`，不清 `confirmClearVocab`/`vocabExpandedID`/`lastDeletedVocab`——关闭再打开设置页「确认清空」仍可能显示**；273/278 只清 confirmDelete 不清 `confirmClearVocab`；485 展开与删除两态可同时命中同一行
- **显式化**：`VocabCategory`（`All/Words/Sentences/Writing`）+ `type VocabularyViewState struct{ Category; Query, ExpandedID, PendingDeleteID string; PendingClear bool; Undo *VocabularyEntry }` + `Reset()`；`showSettings` 调 `Reset()`
- **测试**：`regression_batch_test.go:77/95`、`actions_test.go:211`、`vocab_speak_test.go:32/58`。缺口：状态组合残留、showSettings 后残留确认、展开+删除同时命中
- **复核备注（批次 9j）**：`showSettings` 不清 `confirmClearVocab`/`vocabExpandedID` 成立，已用 `resetVocabularyViewState()` 在打开设置、切换分类、切换搜索三处统一复位；`lastDeletedVocab` 有意保留（撤销删除在同一会话内仍可用）。`vocabConfirmDeleteID` 指向已删除 ID、485 展开与删除同时命中这两条仍靠代码审查，完整 `VocabularyViewState` 结构体留后续。

#### A17 词库导入 / 导出 / 补全异步任务 · 高（ai 组 Top 3）
- **位置**：`fastpot/settings_vocab.go:294-317, 319-344, 346-371, 373-386`；`fastpot/vocabulary.go:720-943, 951-992`
- **载体**：**无显式状态载体**——唯一「结果」是 `a.actionSavedMsg string`(popup.go:126)；四个 `go func()`（298、323、350、377）
- **状态 / 迁移**：空闲/导入中/导出 CSV 中/导出 Anki 中/补全中/完成（成功或失败都写 `actionSavedMsg`）/文件对话框取消（goroutine 直接退出且无任何状态）。按钮 Clicked→起 goroutine→`mygo.Dialog.Open/Save`→取消则静默退出(303/329/356)→`ImportFile/ExportCSV/ExportAnki/EnrichAll`→305-311/331-337/358-364/379-381 写 `actionSavedMsg`→`cfgWin.Update`
- **隐性**：**没有 busy、没有 generation、没有 cancel**，任务生命周期完全靠「goroutine 存在」这一隐式事实；成功/失败靠字符串前缀（「导入失败: 」、「导出 CSV 失败: 」）
- **风险**：连点两次导入开两个 goroutine、两次 `ImportEntries`(925-943)→`AddBatch` 并发写，**谁最后完成谁覆盖 toast**；连点「✨ 补全」并发 `EnrichAll`(951-983) 各做 N 次网络请求并各写一次文件；**`saveOrLog()`(988-992) 保存失败只 log，UI 307 仍显示「成功导入 N 条 ✓」——持久化失败被吞、用户不可重建的数据丢失**；`ImportEntries` 931-933 同步网络补全无错误返回、大文件无进度不可取消
- **显式化**：`VocabJobKind`（Import/ExportCSV/ExportAnki/Enrich/ExportHTML）+ `VocabJobState`（Idle/Running/Succeeded/Failed/Canceled）+ `type VocabJob struct{ Kind; State; Generation uint64; Cancel context.CancelFunc; Progress int; Err error }`；`a.vocabJob atomic.Pointer[VocabJob]`，按钮点击先 `if job.State == Running { return }`；`saveOrLog` 改 `saveOrReport() error`
- **测试**：`vocab_export_import_test.go:10`、`regression_batch_test.go:145`、`regression_test.go:14/73/159`、`regression_batch_test.go:110`。缺口：**连点并发**、保存失败透传、进度/取消、toast 覆盖
- **复核备注（批次 5g）**：导入 / 导出 CSV / 导出 Anki 三条路径的失败分支在 4e 及更早批次已生效；补全按钮（`:377-385`）曾漏网 —— `EnrichAll()` 返回 `int`、内部 `saveOrLog()` 丢 error、UI 无条件显示「已智能补全 N 条 ✓」，磁盘写失败时是「假成功 + 重启后数据丢失」。5g 已把 `EnrichAll()` 改为返回 `(int, error)`、补全按钮按 err 显示「补全失败: ...」，并补 `a17_enrich_save_test.go`（源码契约先红后绿 + 写盘失败 / 正常落盘两条行为测试）。四个 `go func()` 的 `VocabJob` 并发守卫属无用户可见即时缺陷的大重构，需真实文件对话框与窗口，刻意不做（阶段 C 收口，见手册 §10.5）。
- **复核备注（批次 6a）**：用户随后「那就做吧开工」重新授权四项结构性收尾，A17「四个 go func 无 busy/generation/cancel」由阶段 D 批次 6a 处理。修复取最局部的单一作业槽位：新增 vocab_job.go 的 vocabJobMu + vocabJobRunning 与 tryBeginVocabJob() / endVocabJob()，四个 go func 开头各加守卫 + defer，连点第二次不再并发执行、忙时给出「有生词任务正在进行，请稍候…」提示；四个动作的业务逻辑一字不动，单点行为不变。a17_vocab_job_test.go 用「源码契约（先红后绿）+ 表驱动槽位迁移 + 24 goroutine 并发恰好一个成功」两层，四个动作要真实 mygo.Dialog，未臆造窗口级复现。完整 VocabJob 枚举（Kind/State/Generation/Cancel/Progress）与进度 / 取消仍留阶段 D 后续；连点并发这条风险已收口。

#### A18 词库存储单例与加载 / 保存 · 高
- **位置**：`fastpot/vocabulary.go:37-60, 62-97, 99-163, 181-340, 988-992`；`fastpot/vocabulary_export.go:89-101`
- **载体**：`var ( globalVocabStore *VocabularyStore; globalVocabOnce sync.Once )`(49-52)；`type VocabularyStore struct { mu sync.RWMutex; path string; entries []VocabularyEntry; saveMu sync.Mutex }`(37-47)
- **状态 / 迁移**：`globalVocabStore==nil` 未初始化/已初始化；`path==""` 内存 store（不持久化，92-97/100-102/118-120）/有路径；`load()` 成功/文件不存在（return）/**JSON 解析失败（log 后 return，entries 保持空）**。`GetVocabularyStore()`(55-60) `globalVocabOnce.Do`；`newVocabularyStore()`(83-90) 设 path+空 entries+`load()`；`load()`(99-115) 103-110 失败静默；`save()`(117-163) 拷贝→MkdirAll→Marshal→`saveMu.Lock`→写 .tmp→Sync→Close→`.bak`(159-161)→Rename(162)；`Add/AddBatch/Delete/Clear`(181-340) 都是「改内存→解锁→`saveOrLog()`」
- **风险**：**`108-110` JSON 解析失败只 log 后 return，`s.entries` 保持空——词库文件损坏时应用表现为「生词本空了」，随后任何 Add 都会 `save()` 覆盖原文件，损坏数据不可恢复**（159-161 的 `.bak` 是覆盖前的好副本，但此处保存的是空列表覆盖损坏文件）；`path==""` 在 `ExportHTML`(99-101) 用另一套判断拒绝；`globalVocabOnce` 不可重置，测试无法隔离单例
- **显式化**：`VocabLoadState`（`Unloaded/Loaded/ParseFailed/ReadOnly`）+ `VocabularyStore` 增 `state VocabLoadState; lastErr error`；`func LoadVocabulary(path string) (*VocabularyStore, error)` 返回显式错误，解析失败时**不覆盖**原文件并提示「文件损坏，已保留副本」；`ReadOnly bool` 取代 `path==""`
- **测试**：`actions_test.go:80`、`vocabulary_test.go:14`、`regression_test.go:14/40/159`、`vocab_export_import_test.go:10`、`regression_batch_test.go:136/251/270`。缺口：**JSON 解析失败**、单例重置、path=="" 与 ExportHTML 一致性
- **复核备注（批次 5b）**：本条「风险」里「损坏数据不可恢复」的结论**成立**，但机制要补精确，§5.1 Top 10 紧随段与 §8 JSON 的 A18 anchor 沿用审计描述未展开：`save()`(159-161) 的 `.bak` 是**滚动单副本**，第一次 `Add` 时它恰好等于损坏原文，第二次 `Add` 就被「加词后的内容」覆盖，损坏原文永久丢失；UI 全程无提示。「显式化」建议里的「解析失败时不覆盖原文件」若直接实现会让用户无法再「加词」（refusing-to-write），违反「不破坏现有功能」，故批次 5b 改用加性方案：`load()` 解析失败且 `path+".corrupt"` 不存在时，把原始 `data` 原样另存一份并打可执行指引日志，`return` 前不改 `s.entries`（保留「空库启动」现状）、不改 `s.path`、不动 `save()`。新增 `TestCorruptVocabularyKeepsRecoverableCopy`（先红后绿）、`TestRecoveryCopySurvivesLaterSaves`（钉住「两次写盘后 `.corrupt` 仍逐字等于损坏原文」）、`TestIntactVocabularyLoadsWithoutRecoveryCopy` 与 `TestMissingVocabularyNeedsNoRecoveryCopy`（对照）。`VocabLoadState` / `ReadOnly` / `LoadVocabulary` 返回 error 等结构性显式化仍留阶段 C 后续。详见 [STATE_MACHINES_FIX_PLAYBOOK.md](STATE_MACHINES_FIX_PLAYBOOK.md) §5 / §7

#### A19 词库导入格式探测与列映射 · 中
- **位置**：`fastpot/vocabulary.go:720-730, 733-795, 798-883, 886-916, 925-943`
- **载体**：`ext := strings.ToLower(filepath.Ext(filePath))`(721)；`termCol, phoneCol, posCol, defCol, diffCol, ctxCol, ctxTransCol, tipsCol := -1, -1, -1, -1, -1, -1, -1, -1`(819)；`startRow := 1`(842)
- **状态 / 迁移**：扩展名 `.csv`/`.txt`/其他→JSON；JSON 三格式（标准数组/字符串数组/map 数组）/无法识别；CSV 8 个列索引 `-1`=未找到、`termCol==-1`→无表头模式（844-845 termCol=0, startRow=0）。`ImportFile`(720-730) switch ext；`ImportJSON`(733-795) 三段 `json.Unmarshal` 依次尝试(741/747/759)，任一成功即 `ImportEntries`，全失败→「无法识别的 JSON 数据格式」(794)；`ImportCSV`(798-883) BOM 剥离(804-806)、表头扫描(820-840)、842-846 无表头推断、848-880 逐行构造；`ImportTXT`(886-916) 三种分隔符 SplitN；`ImportEntries`(925-943) 清洗+补全+一次 `AddBatch`
- **风险**：741 判「标准格式」只看 `entries[0].Term != ""`——首条空 Term 会误判为格式 2/3 或「无法识别」；764-767 map 里非字符串字段（数字 ID、布尔）静默丢弃；819 的 8 个 -1 与 858-878 的 `>=0 && <len(row)` 重复表达「列是否存在」；891 `strings.Split(string(data), "\n")` 不处理 `\r\n`（Windows 文件每行末留 `\r`）；`EnrichVocabularyEntry` 无返回值，补全失败被吞
- **显式化**：`ImportFormat`（`VocabularyJSON/WordList/KVMap/CSV/TXT`）+ `type ImportPlan struct{ Format ImportFormat; ColumnMap map[VocabField]int; HasHeader bool; Entries []VocabularyEntry }` + `func DetectImportFormat(data []byte) (ImportFormat, error)` + `func ParseCSV(data []byte) (ImportPlan, error)`；`type VocabField uint8` 取代 768-785 字符串 key switch
- **测试**：`vocab_export_import_test.go:10`、`regression_batch_test.go:145`、`regression_test.go:73`。缺口：首条空 Term、CRLF、非字符串 map 值、无表头 CSV
- **复核备注（批次 7f）**：BOM 不一致属实——CSV 剥了，JSON/TXT 没剥。已抽 `readImportFile` 统一剥 UTF-8 BOM，三个导入器共用；`ImportFormat`/`ImportPlan` 的结构化探测、首条空 Term、CRLF、非字符串 map 值仍未做，留后续。

#### A20 词库分类判定（三处规则不一致） · 中
- **位置**：`fastpot/vocabulary_export.go:32-50, 52-63, 66-87, 148-154, 199-202`；`fastpot/settings_vocab.go:431-441, 494-497, 534, 596`；`fastpot/vocabulary.go:32`
- **载体**：`Source string // "dict" | "grammar" | "writing" | "notes" | "import"`(vocabulary.go:32)、`catKey string`(settings_vocab.go:431)、HTML `data-cat` 属性(160)、JS `let curCat='all';`(200)
- **状态 / 迁移**：分类 "all"/"words"/"sentences"/"writing"，靠 `Source` 字符串 + `len(strings.Fields(e.Term)) <= 2` 词数启发式。`matchesCategory`(52-63) 三分支；`CategoryCounts`(32-50) `switch` 强制 `Source=="writing"` 优先（注释 37-39 承认规则重叠）；`Filter`(66-87) 调 matchesCategory；`ExportHTML`(148-154) `Source=="grammar"||词数>2 → sentences` **else if** writing；`settings_vocab.go:494-497` 同样 grammar/词数优先；`534/596` 又用 `Source!="grammar" && Source!="writing" && 词数<=2` 判词卡
- **风险**：**分类不一致**：`ExportHTML`(150-153) 与 `settings_vocab.go:494-497` 先判 `grammar||词数>2` 再判 writing，一条 `Source=="writing"` 且词数>2 的写作条目在网页版/卡片标签被标成「长难句」，而 `CategoryCounts`(41-42) 与 `matchesCategory`(59) 算作 writing——**桌面与网页版同一条落在不同分类**；`431-439` catKey 字符串与 `matchesCategory` 的 case 靠约定同步
- **显式化**：`type VocabSource string`（`SourceDict/SourceGrammar/SourceWriting/SourceNotes/SourceImport`）+ `type VocabCategory uint8`（`CatAll/CatWords/CatSentences/CatWriting`）+ `func Classify(e VocabularyEntry) VocabCategory` **唯一实现**（先 Source 后词数）；`CategoryCounts/Filter/ExportHTML/设置页/JS data-cat` 全调它；JS 侧改 `const CAT = Object.freeze({...})`
- **测试**：`regression_batch_test.go:95/77/110/145`。缺口：**三处分类一致性**、writing+长句边界、Source 非法值
- **复核备注（批次 7a）**：三处规则不一致属实，已收敛到 `fastpot/vocab_category.go` 的 `classifyVocabEntry` 唯一实现；现在 `Source` 优先（writing/grammar/dict/notes），其余按词数。`CategoryCounts`、`Filter`、设置页行标签、`ExportHTML` 的 `data-cat` 与 Anki 导出共用同一判定，`vocab_category_test.go` 钉住四个面的一致性。`VocabSource` 枚举化与 JS 侧常量表留后续。

#### A21 网页版 HTML 内嵌 JS 状态 · 中
- **位置**：`fastpot/vocabulary_export.go:139-146, 193-206`（140 初始 `class="tab-btn active"`、200 `let curCat='all';`、201 `filterCat`、202 `doSearch`、206 `handleImport`）
- **载体**：`let curCat='all';`(200)、`let allEntries = ...`(199，生成时快照)、`.tab-btn.active` CSS class(140/201)、`x.style.display`(202)、`noResult.style.display`(193/202)、`document.getElementById('search').value`(202)
- **状态 / 迁移**：当前分类（4 值字符串）/搜索词（DOM input.value）/卡片 show-hide/无结果提示 show-hide；`allEntries` 是**生成时刻快照**，与桌面词库此后变化无关。`filterCat(c,b)`(201) 设 curCat+移除所有 `.active`+给 b 加 `.active`+`doSearch()`；`doSearch()`(202) 读 input.value→遍历 `.card` 按 `data-cat`/`data-text` 算 show→设 display+更新 noResult；`exportJSON/CSV/Anki`(203-205) 用 `allEntries`；`handleImport`(206) 只 `alert('…离线静态快照…')`
- **风险**：**快照与桌面词库不同步**——导出后在桌面新增/删除生词，网页版导出按钮仍给旧数据（206 的 alert 只提示导入，不提示导出也是快照）；`filterCat` 依赖 `this` 传参，DOM 结构一改 b 为 undefined 即抛错；202 `getAttribute('data-text').includes(q)` 特殊字符边界未测；**`handleImport` 是「假按钮」**，点导入只弹 alert，用户可能以为成功
- **显式化**：`const CAT = Object.freeze({ALL:'all', WORDS:'words', SENTENCES:'sentences', WRITING:'writing'})`；`const state = Object.freeze({cat, query})` 单一状态对象 + `render()` 纯函数；`export*()` 标注 `data-snapshot-at` 与「导出的是快照」；`handleImport` 改禁用并说明原因
- **复核备注（批次 9c）**：导出页现在写明导出时间与「静态快照」，分类值收进 `const CAT=Object.freeze(...)`，假导入按钮改为不可用的「导入仅桌面端」并删除 `handleImport` 与 `type=file`；`vocabulary_export_html_test.go` 三条钉住。`state` 单一对象 + `render()` 纯函数与浏览器实际走查仍留后续。
- **测试**：`regression_batch_test.go:110/136`、`ux_walkthrough_test.go` 相关。缺口：JS 状态一致性、快照过期提示、假导入按钮

#### A22 语法分析解析多级 fallback · 中
- **位置**：`fastpot/grammar.go:12-29, 32-87`
- **载体**：`func ParseSentenceAnalysis(raw string) *SentenceAnalysisResult`(32) 返回 `nil` 表示失败；`type rawGrammar struct{ Annotations any; AnnotationsZH any; AnnotationsDol any; Translation string; TranslationZH string; TranslationDol string }`(38-45) 兼容三套键名
- **状态 / 迁移**：`cleaned==""`→nil / 一级解析成功（`len(list)>0 || trans!=""`）→结构化结果 / Fallback 1(71-74 直接 unmarshal 且 `len(res.Annotations)>0`) / Fallback 2(77-84 纯标注数组) / 全失败→nil。`CleanJSONText`(33)→`json.Unmarshal`(48)→`firstNonNil`/`firstNonEmpty`(49-50)→60-66 `if len(list)>0||trans!=""`→返回；否则 69-84 两级 fallback→86 `return nil`
- **隐性**：**`nil` 同时表示「空输入」「模型输出不是 JSON」「解析出但无有效标注」三种失败**，调用方无法区分；错误全被 `_ =` 吞(54/57)
- **风险**：54/57 `_ = json.Unmarshal` 把「annotations 是字符串但内容非法 JSON」静默吞掉，用户只见卡片不出现；86 不带原因，UI 无法提示「格式不对可重试」；60-66 只要 `trans!=""` 就返回成功（零标注），下游 `renderGrammarVisualCard`(288) 可能渲染空语法树
- **显式化**：`ParseOutcome`（`Empty/Structured/FallbackDirect/FallbackArray/Failed`）+ `type SentenceParseResult struct{ Outcome; Result *SentenceAnalysisResult; Cause error }` + `func ParseSentenceAnalysis(raw string) SentenceParseResult`
- **复核备注（批次 9b）**：`ParseOutcome` 六态与 `SentenceParseResult` 已落地，`ParseSentenceAnalysis` 保留为薄包装所以既有调用行为不变；`translation_only` 在卡片上给出「只有译文、没有语法标注」的说明，`failed` 在 `applyParsedAction` 记录原因日志。`CleanJSONText` 的清理边界与卡片提示位置仍需真人走查。
- **测试**：`actions_test.go:39`、`actions_parse_test.go:12/37/58/71/102/127/144/162/184`、`popup_streaming_test.go:31`。缺口：三级 fallback 分别命中、nil 原因区分、坏 annotations 字符串

#### A23 语法角色高亮过滤 · 低
- **位置**：`fastpot/grammar.go:179, 288-390, 1009-1065`；声明 `fastpot/popup.go:140-141, 167, 284-285`
- **载体**：`filterRole string`(popup.go:140)、`grammarHoverKnowledge string`(141)、`var grammarExplanations = map[string]string{...}`(grammar.go:179)、`tabStates map[popupMode]*tabViewState`(popup.go:167)
- **状态 / 迁移**：`filterRole==""` 无过滤 / `"subject"`/`"predicate"`/`"object"` 高亮某角色；`grammarHoverKnowledge==""` 不显示知识卡 / 非空显示解释。`toggleGrammarRole(role)`(1057-1065) `if a.filterRole == role`→清空两者(1059-1060)，否则 1063 设 filterRole、1064 `grammarHoverKnowledge = grammarExplanations[role]`；`grammarLegend`(1028-1053) 用 `active == role`(1031) 判高亮；368-386 依 `grammarHoverKnowledge != ""` 渲染知识卡；`openPinnedScreenshot`(ocr.go:281) `a.tabStates = nil` 连带清掉
- **风险**：`role` 不在 `grammarExplanations` 时 1064 写空串但 1063 已设 filterRole，产生「高亮生效但知识卡不显示」的**状态不同步**；`tabStates` 被截图会话置 nil 导致语法卡状态丢失
- **显式化**：`type GrammarRole string`（`RoleSubject/RolePredicate/RoleObject`）+ `type GrammarFilter struct{ Role *GrammarRole; Knowledge string }` + `Toggle(r GrammarRole)` 原子更新；`grammarExplanations` 改 `map[GrammarRole]string` 编译期约束 key
- **测试**：`grammar_render_test.go:60/98/139/158`、`grammar_trunk_test.go:13/58`、`actions_test.go:191`。缺口：Toggle 二次点击、未知 role、filterRole 与 knowledge 一致性

#### A24 腾讯引擎能力表与可变 URL · 低
- **位置**：`fastpot/tencent.go:48-53, 59-72, 78-87, 100-168`；`fastpot/settings_pages.go:201-220`
- **载体**：`var tencentAPIURL = tencentDefaultURL`(52-53，注释「测试可改的可变全局」)、`var tencentTargets = map[string]bool{...}`(59-65)、`var tencentClient = &http.Client{Timeout: 10 * time.Second}`(48)
- **状态 / 迁移**：目标语言在/不在能力表（决定是否加入竞速）、URL 被测试改写/默认、`source==""→"auto"`(108-111)、`Header.RetCode == "succ"`(156)。`tencentSupportsTarget`(70-72) 读 map；`tencentTranslate`(100-168) 108-111 空 source→auto、146 非 200→「腾讯: http %d」、156 RetCode 判定、163-166 空结果→「腾讯: empty result」
- **风险**：`tencentAPIURL` 包级可变全局，测试并行改写会串扰（无 mutex）；`tencentTargets` 硬编码静态能力表与 `settings_pages.go:201-220` 的「引擎状态」（`h.LastError != ""`，220）是两套数据，UI 健康状态与实际支持语言不同步；156 只判 `"succ"`，其他错误码统一「腾讯: %s」无分级
- **显式化**：`type TranslateTarget string` + `type EngineCapability struct{ Engine EngineKind; Targets map[TranslateTarget]bool; MeasuredAt time.Time }`；用注入式 `TencentClient{URL, Client}` 取代可变全局；`type TencentRetCode string` + 常量
- **测试**：`tencent_test.go:12/35/48/67/85/138`、`tencent_race_test.go:10/21`、`engine_caption_test.go` 系列。缺口：并行改写 URL 串扰、能力表与 UI 健康一致性、非 succ 错误码分级

**ai 组已显式化的范式（不计入清单）**：`fastpot/grammar_marks.go:25-50` 的 `type markKind int` + `const ( markUnderline markKind = iota; ... )` + `var markBoxes = map[markKind]markBox{...}` 是带类型名与常量名的显式枚举，配置与绘制分离，可作为上述改造的范式。

**ai 组按风险排序 Top 3**：A4 截图 OCR 会话（单次启动 `ocrGeneration` 自增 3 次、取消后旧结果仍可写回、busy 与 error 可同时为真）＞ A8 配置加载/迁移（else-if 一次只回写一种迁移、解析失败吞成默认且不覆盖磁盘）＞ A17 词库异步任务（四个 goroutine 无 busy/generation/cancel，`saveOrLog` 吞掉持久化失败仍显示「成功导入 ✓」）。紧随：A18 词库文件损坏静默按空库启动后覆盖原文件、A11 `"polish"` 值经 Segmented 往返不可逆丢失。


### 4.4 跨模块（X1–X4）

#### X1 三份状态手工同步 · 高
- **位置**：`fastpot/popup.go:57-249`（`app`）、`fastpot/popup.go:251-299`（`snapshot`）、`fastpot/popup_tabs.go:14-50`（`tabViewState`）；`snap()`、`saveTabLocked()`(58-99)、`restoreTabLocked()`(101-138)、`clearTabLocked()`(142-179)
- **载体**：同一份业务状态在三个结构体里各存一份字段清单，靠 `a.mu` + 手工逐字段赋值对齐。C6 是这份矩阵的总览条目，U6 / U12 是它在页签场景的具体破裂点
- **状态 / 迁移**：`app` 为真值源；每帧 `snap()` 拷贝到 `snapshot` 供渲染；`switchMode` 切页签时 `saveTabLocked` 把 `app` 拷进 `tabStates[mode]`，切回时 `restoreTabLocked` 反向拷贝
- **风险（这是全篇最主要的结构性根因）**：三处字段集合不一致（`snapshot` 有 `isWord`，`tabViewState` 没有；`tabViewState` 有 `ocrPath`，`snapshot` 没有）；保存与恢复对同一字段采取不同策略（U6 的 `aiBusy/notesBusy` 原样存、强制置 false 读）；`tabViewState` 保存 `grammarResultPrefill` 却不保存 `aiGen`（U17）；保存 6 个视图选项却漏 4 个（U12）。**加一个状态要改三处，漏一处就是用户可见的「切回来内容不对」**
- **显式化**：一个 `type PopupState struct{ ... }` 作为唯一真值源，`snapshot` 与 `tabViewState` 不再是字段清单，而是 `PopupState` 的值拷贝或 `TabSnapshot`（含 `Phase`）；`func (s PopupState) Snapshot() PopupState`、`func (s PopupState) ToTab() TabSnapshot`、`func (t TabSnapshot) Restore() PopupState` 三个纯函数共用一份 `syncState` 序列化，禁止两边各写字段清单
- **测试**：无「三处字段集合一致」的断言。建议先补一条反射对比测试：断言 `snapshot` 与 `tabViewState` 的字段名集合是 `app` 状态字段的子集
- **复核备注（批次 6b）**：X1「加一个状态要改三处」确认属实，但完整合一（单一 `PopupState` struct）会牵动 `app` 近百字段、`snapshot` 含 15 个渲染期派生字段、`tabViewState` 独有 `ocrPath`，语义不同不能硬合。阶段 D 批次 6b 先做可确定性验证的一刀：`clearTabLocked()` 与 `restoreTabLocked(&tabViewState{})` 的字段集合逐字相同（同一份清单复制两遍），让 clear 委托 restore，三份手工清单减到两份；`snap()` 因含派生 `isWord` 保留，但纳入 `x1_tab_field_sync_test.go` 的反射同步契约（`TestTabFieldListsAreSynced` 断言 save / restore / snap 三处字段齐全、同名类型一致，`TestClearTabMatchesZeroValueRestore` 用 DeepEqual 证明 clear 与零值 restore 等价）。零行为变化。完整 `PopupState` 合一仍需真人 GUI 走查，留阶段 D 后续。

#### X2 代次计数器家族与 switchMode 迁移中心 · 高
- **位置**：`fastpot/popup.go:78, 90, 105, 163`（`aiGen` / `ocrGeneration` / `notesGen` / `translateGen`）；`fastpot/popup_actions.go:52-64, 69-77`（`stopAIPassLocked` / `stopNotesLocked`）；`fastpot/popup.go:398-454`（`switchMode`）；`fastpot/popup.go:391-396`（`rerunCurrentModeFresh`）
- **载体**：4 套独立 `uint64` 代次 + 4 个 `context.CancelFunc` + 4 个 `busy` 布尔 + 1 个一次性 `skipAIResultCache`，命名不统一（`Generation` vs `Gen`），失效语义靠「`gen != a.gen` 就丢弃」而不是 `cancelled` 状态
- **状态 / 迁移**：`switchMode` 是事实上的迁移中心，一次调用依次做 `saveTabLocked` → `stopAIPassLocked` → `stopNotesLocked` → 清 `busy/errText` → `restoreTabLocked` 或 `clearTabLocked` + `rerunCurrentMode`；每条异步路径各自记 `gen := a.xxxGen`，回来时比对
- **风险**：代次之间会互相伤害——`refineWithAI`(`popup_actions.go:856`) 复用 `aiGen`，点「AI 精翻」会 `stopAIPassLocked()` 静默取消卡片流（U1/U4）；`switchMode`(`popup.go:427-430`) 直改 `busy=false` 而不 `translateGen++`，旧译文会写进新模式（U2）；`restoreTabLocked` 不自增 gen 却丢弃 cancel（U6）；`ocrGeneration` 单次启动自增 3 次（A4）；`skipAIResultCache` 由 `rerunCurrentModeFresh` 生产、只在 `runAction` 消费，中途早退就残留（U14）。**「代次」是上一轮竞态修复刻意引入的机制，不能简单删除，只能收拢**
- **显式化**：`type generation uint64; func (g *generation) Next() generation` + `type Session struct{ Gen generation; State XxxState; Cancel context.CancelFunc }`；每个生命周期一份独立 `Session`，`func (s *Session) Invalidate()` 统一「自增 + cancel + 置 cancelled」；`switchMode` 拆成 `func (a *app) switchMode(m popupMode, reason ModeChangeReason)`，把五件事变成一张可读的迁移表
- **测试**：`ai_generation_race_test.go`、`optimization_regression_test.go`、`popup_ui_test.go:112` 覆盖部分代次；缺「任意事件序列下新旧表示一致」的迁移表测试
- **复核备注（批次 4b / 4d / 8b）**：代次家族已统一为 `type requestToken uint64`（`aiGen` / `aiRefineGen` / `notesGen` / `translateGen` / `ocrGeneration`），`switchMode` 与两个 `openPopup` 入口补自增 `translateGen`，`refineWithAI` 改用独立代次。完整的 `modeChangeReason` 迁移中心与单一 `Session` 对象未做，留后续。

#### X3 两份 openPopup Reset 段 · 高
- **位置**：`fastpot/popup.go:482-528`（`openPopupAction`）、`fastpot/popup.go:566-604`（`openPopupWithText`）；另有 `fastpot/popup.go:600-603` 的 `screenshotPinned` 兜底
- **载体**：两段各自手工列举 30+ 字段置 false / `""` / nil，外加 `tabStates = nil`
- **状态 / 迁移**：每次开弹窗把 `app` 的「本次会话状态」全部清回初值；两段逻辑应等价却不等价
- **风险**：**已有实锤事故**——`popup.go:600-603` 的注释「防止截图翻译置顶状态泄漏到后续每个 popup，OnBlur 不再关闭」说明 `openPopupWithText` 曾漏清 `screenshotPinned`，导致后续所有弹窗都置顶且失焦不关；该兜底只在 `openPopupWithText` 里，`openPopupAction` 路径没有对应处理（U8）；`stopScreenshotOCR()` 也只在部分路径调用（U15）。两段清单一旦新增字段漏改一处，就会重现同类泄漏
- **显式化**：`func (a *app) openPopup(m popupMode, text string)` 单一入口，Reset 改为 `a.session = Session{}`（或 `a.viewOpts = ViewOptions{}` + `a.windowPolicy = WindowPolicy{}` 两行），彻底删掉两份字段清单；截图相关清场统一进 `a.screenshotOCR.Invalidate()`
- **测试**：无「两条入口 Reset 结果一致」的测试。建议补一条：分别调用两条入口后，比较 `app` 的状态字段集合是否相同
- **复核备注（批次 9l）**：两段 Reset 的字段差异已实地比对——`openPopupAction` 复位四个视图开关（`onlyTrunk` / `showSlashes` / `filterRole` / `grammarHoverKnowledge`），`openPopupWithText` 全部漏掉，这正是「注释说曾泄漏」的同一类问题在另一条入口重演；反向 `activeActionID` 只在 `openPopupWithText` 里清。已抽出 `resetPopupTransientLocked()` 让两条入口共用一份清单（`popup_reset_consistency_test.go` 行为 + 源码契约两条）。完整 `Session{}` / `ViewOptions{}` 结构体化仍留后续，因为 `source` / `mode` / `inputTargetMode` 两条入口语义不同。

#### X4 跨组去重映射（52 条为何保留）
三组按文件边界并行审计，同一状态机可能被不同组从不同侧面各报一次。以下列出已知重叠，**主条目**是迁移逻辑所在处，**关联条目**是同一状态机的视图侧、上游或下游，两者记录的触发路径与测试缺口不同，故全部保留：

| 主条目 | 关联条目 | 关系 |
| --- | --- | --- |
| A4 截图 OCR 会话（`ocr.go`） | U15 OCR / 截图会话（`popup_ocr_pairs.go`） | 前者是流水线与代次，后者是渲染侧与置顶兜底；显式化时合并为一个 `ScreenshotOCRSession` |
| C10 截图框选会话（`snip.go`） | A4 / U15 | 独立的框选窗口会话，与 OCR 流水线通过 `snipBitmap` 衔接，三者不是同一个状态对象 |
| C1 钩子手势（`hook.go`） | C3 触发模式、C4 gestureGate、U16 选择手势触发策略 | 同一条「鼠标事件 → 选区」输入管线的四个环节；建议统一为 `SelectionInputState` + `GestureVerdict` |
| C2 钩子生命周期 | C1、C5、U8 | 进程/线程/窗口三层生命周期，互相衔接但各自独立 |
| C6 popup 卡片复合状态 | U1 / U2 / U3 / U4 / U7 / U14 / U17 | C6 是 `app` 字段矩阵的总览条目，U 组是各生命周期的细分；总览保留，细分各自成立 |
| U6 Tab 快照恢复 | X1、U12、U17 | U6 是迁移中心，X1 是结构性根因，U12 / U17 是字段清单遗漏的具体实例 |
| U1 AI 卡片代次 | U4 精翻、U14 缓存跳过、U17 prefill | 共用 `aiGen` 或依附同一 pass，但各自有独立的生命周期与失败模式 |
| A8 配置加载迁移 | A9 保存错误、A10 原子保存、A11 字符串枚举族 | 同一份 `Config` 的读/写/UI 映射三面，故障模式不同 |
| A17 词库异步任务 | A18 存储、A16 生词本页交互 | 任务、持久化、UI 三层 |
| U13 保存反馈 | A9 | 「已保存」状态与「保存失败」提示是两条路径 |

按状态对象归并后约 **40 余个独立隐式状态机**；本报告仍保留全部 **52 条审计条目**（C 11 / U 17 / A 24），因为每条都记录了不同的触发路径、风险证据与测试缺口，删掉任何一条都会丢失可执行的修复信息。本节的 `X1`–`X4` 是额外补充的跨模块结构性根因，不计入三组审计条目数，故全文与 §8 附录合计 56 条。

---

## 5. 全局风险排序

排序依据：**明确数据竞争 / 不可恢复状态 ＞ 已经发生过的用户可见错乱 ＞ 可能丢失用户数据 ＞ 状态语义失真 ＞ 可维护性**。等级沿用 §4 开头的高/中/低。

### 5.1 Top 10

| # | ID | 状态机 | 用户会看到什么 | 关键证据 | 测试 |
| --- | --- | --- | --- | --- | --- |
| 1 | U1 | AI 卡片请求代次与流式渲染 | 点「AI 精翻」后正在输出的长难句/写作卡片停在中途，没有提示 | `popup_actions.go:856` 复用 `aiGen`；`stopAIPassLocked` 清 `aiBusy` 不清 `aiText` | 无代次竞态测试 |
| 2 | U10 | Hover Tip 计时器与代次 | 悬浮提示关不掉，必须重启应用 | `selicon.go:683-686` 注释自承认 `window becomes unclosable until restart`；`shown` 两处置位、`gen` 三处校验，跨三个执行上下文 | 仅 3 条竞态测试 |
| 3 | A4 | 截图 OCR 会话 | 取消截图后旧结果仍写回、`ocrBusy` 与错误同时显示 | `ocr.go:230-472` 单次启动 `ocrGeneration` 自增 3 次，取消路径不去活 | `popup_ui_test.go:112` 部分覆盖 |
| 4 | U6 | Tab 快照与恢复 | 切页签后上一页的译文/生词出现在新页签，或显示「完成」但结果还在路上 | `popup_tabs.go:58-99` 原样存 `aiBusy/notesBusy`，`:101-138` 强制置 false 且不自增 gen | 全无测试 |
| 5 | C10 | 截图框选会话 | 重复触发截图框选时框选状态互相覆盖 | `snip.go:240-242` 只在取指针时持锁、`:247-301` 字段读写无锁；`currentSnip` 全局单例 | 未覆盖数据竞争 |
| 6 | A17 | 词库导入/导出/补全异步任务 | 显示「成功导入 ✓」但数据没落盘；重复点按钮会并发跑多个任务 | `settings_vocab.go:294-386` 四个 `go func()` 无 busy/generation/cancel；`vocabulary.go:988-992` `saveOrLog()` 吞掉持久化失败 | 全无测试 |
| 7 | C5 | 窗口与浮层显隐 / 退出 | 退出过程中回调仍操作窗口 | `main.go:100-102` 写 `isQuitting` 加锁、`:287` 读不加锁，明确数据竞争 | 未覆盖 |
| 8 | A8 | 配置加载 / 迁移多阶段 | 一次设置变更只有一项被回写；配置文件损坏后静默变回默认且磁盘不动 | `config.go:241-270` else-if 一次只回写一种迁移；`:400-403` 解析失败吞成 `DefaultConfig()` | `atomic_save_test.go` 部分覆盖 |
| 9 | U8 | 主窗口生命周期与置顶 | 截图翻译之后后续每个弹窗都置顶、失焦不关（已发生过） | `popup.go:600-603` 兜底注释；兜底只加在 `openPopupWithText`，`openPopupAction` 未做 | 全无测试 |
| 10 | C1 | 鼠标/键盘钩子手势识别 | 切换「仅快捷键」后划词翻译失效到重启；双击手势重装钩子后计数不清 | 11 个包级变量 + mutex/atomic 双套同步；`hook.go` 的 `hookThreadOnce` 单向门（U16 有对应注释） | `hook_lifecycle_test.go` 部分覆盖 |

紧随其后：U2（`switchMode` 直改 `busy=false` 不失效 `translateGen`）、A18（词库文件损坏静默按空库启动后覆盖原文件）、U11（流式期间 `autofitLocked` 锁死导致文字裁切）、U12（tab 保存漏 4 个视图选项）、U14（`skipAIResultCache` 残留误伤下一次请求）、A11（`"polish"` 值经 Segmented 往返不可逆丢失）、A20（桌面与网页版分类规则不一致）、A21（`handleImport` 是假按钮）、C7（`audio.go:165-174` 代次检查与 `playMP3` 之间竞态可致朗读串音）、C9（`services.go:582-584` 注释自述「one bad answer was enough to open the breaker for a minute」）。

### 5.2 用户可见症状 → 状态机索引

| 症状 | 相关条目 |
| --- | --- |
| 点「AI 精翻」后卡片停在中途 | U1, U4 |
| 悬浮提示关不掉、要重启 | U10 |
| 切页签后旧结果/半成品出现在新页签 | U6, U1, U3, U17 |
| 截图翻译后弹窗一直置顶、失焦不关 | U8, A4, X3 |
| 切换模式后旧译文闪回 | U2, U5 |
| 流式回答被裁切 | U11 |
| 导入词库显示成功但数据没保存 | A17, A18 |
| 改了几项设置只有一项生效 | A8, A11 |
| 切「仅快捷键」后划词翻译失效到重启 | C1, U16, C3 |
| 词库分类在网页版与桌面版不一致 | A20 |
| 设置里的「导入」按钮点了没反应 | A21 |
| 朗读串音 / 熔断误开一分钟 | C7, C9 |
| 「已复制」「已保存」横幅一直挂着 | U13 |
| 重新翻译后下一次请求意外绕过缓存 | U14 |

### 5.3 按根因分类

| 根因 | 条目 | 收敛方向 |
| --- | --- | --- |
| 代次 / 取消语义隐式 | U1 U2 U3 U4 U6 U14 U15 A4 C7 C9 C10 | `Session{Gen, State, Cancel}` + `Invalidate()`；`cancelled` 成为可枚举状态 |
| 布尔组合 / nil 与空串哨兵 | C6 U7 U8 U9 U11 U12 U13 A5 A7 A9 A10 A21 | 互斥 bool 收为一个 `XxxState` 枚举；空串哨兵换成显式字段 |
| 时间窗口状态 | U10 U16 C4 | `time.Time` 哨兵换成 `Phase` + `Since`/`ExpiresAt`，消除「零值即未暂停」 |
| 跨结构体复制 | X1 U6 U12 U17 | 单一 `PopupState` 真值源，快照与页签存值拷贝 |
| 字符串当状态值 | U4 U5 U16 A8 A11 A20 A21 | `type Xxx uint8` 枚举 + `String()`；`page` 改 `SettingsPage` |
| 明确数据竞争 | C1 C5 C10 C7 | 统一锁粒度；`generation` 收进同一把锁保护 |
| 不可达 / 假状态 | U7（`markdown.Stream` 恒 false）、U12（`ocrExpanded` 无消费者）、A21（假按钮） | 显式枚举让编译器与穷举测试暴露不可达分支 |
| 代码注释自承认的故障 | U10、U8、U16、C9、A4 | 这些是「显式化收益最确定」的落点，优先做镜像（§6.2 第 1 步） |

### 5.4 测试覆盖总览

- **有针对性回归测试**：U1 渲染三态（`popup_streaming_test.go`）、U10 三条竞态（`selicon_tip_race_test.go`）、A4 迟到结果（`popup_ui_test.go:112`）、C1/C2 钩子生命周期（`hook_lifecycle_test.go`）、C4 手势（`gesture_test.go` / `selection_policy_test.go`）、A8/A10（`atomic_save_test.go`）、C7/C9（`ai_generation_race_test.go` / `optimization_regression_test.go`）
- **完全无测试**：U2 U4 U5 U6 U11 U14 U17 A17（部分）A21，以及 X1（三份状态一致性）、X3（两条入口 Reset 一致性）
- **结构性缺口**：没有任何测试断言「`app` / `snapshot` / `tabViewState` 字段集合一致」，也没有「任意事件序列下新旧表示一致」的迁移表测试。这两条正是 §6.2 第 2 步要补的护栏



---

## 6. 最佳方案：显式化，同时提升 UX / DX / AX

用户要求「不破坏任何现有功能」，并「始终选择能同时提升 UX、DX、AX 的最佳方案」。下面给出唯一满足这两个约束的方案：**把隐式状态收敛成显式枚举 + 集中迁移，采用镜像式三步迁移，全程不改变外部行为**。

### 6.1 目标形态

每个独立生命周期一个状态类型，互斥的 bool 组合收拢为一个字段：

```go
// 示例：AI pass（现状是 aiBusy / aiText / aiCancel / aiGen / aiRefineErr 五个字段隐式表达）
type aiPassState uint8

const (
    aiPassIdle aiPassState = iota
    aiPassStreaming
    aiPassDone
    aiPassCancelled
    aiPassFailed
)

func (s aiPassState) String() string { /* 固定字符串，供日志与诊断使用 */ }

// 迁移只有一个入口；非法迁移在同一处拒绝并记录，而不是散落在 20 个文件里。
func (a *app) transitionAI(from aiPassState, ev aiEvent) aiPassState
```

命名的落点（`popupMode` 已经是这个模式的范例，照它扩展即可）：

| 现状（隐式） | 建议显式类型 | 建议状态值 |
| --- | --- | --- |
| `aiBusy/aiText/aiCancel/aiGen/aiRefineErr` | `aiPassState` | `idle / streaming / done / cancelled / failed` |
| `notesBusy/notes/notesGen/notesCancel` | `notesPassState` | 同上 |
| `ocrBusy/ocrText/ocrError/ocrGeneration/ocrCancel` | `ocrPipelineState` | `idle / capturing / recognizing / translating / refining / done / failed` |
| `busy/engine/target/translateGen` | `translateState` | `idle / racing / done / failed / superseded` |
| `sel.shown / chipTip.shown / moreFlyout / hover` | `chipLifecycleState` | `hidden / visible / tipVisible / menuOpen` |
| `pinned / screenshotPinned / closeOnBlur` | `windowPinState` | `normal / pinned / screenshotPinned` |
| `pausedUntil` | `selectionTriggerState` | `armed / paused / suppressed` |
| `autofitLocked / autofitSteps / autofitDir` | `autofitState` | `settled / correcting / locked` |
| `isQuitting` | `appLifecycleState` | `running / draining / exited` |
| `confirmClearVocab / vocabConfirmDeleteID` | `destructiveConfirmState` | `idle / awaitingConfirm` |
| `settingsPageSeen / page` | 已有字符串，补枚举与合法迁移表 | — |

### 6.2 零风险迁移路径（关键约束）

不做「一次性重写」，按 strangler 方式三步走，每步都能单独通过测试：

1. **镜像**：新增枚举字段，与旧 bool/代次**并行写入**；在 `snap()` 里同时输出两种表示。此步不改变任何读取方，行为完全不变。
2. **切换读取**：把渲染与判断逻辑从 `if a.aiBusy && …` 改为 `switch a.aiPass`，旧字段保留作为断言依据；补迁移表测试，断言「任意事件序列下新旧表示始终一致」。
3. **清理**：确认全部读取方切换完毕、测试全绿后，删除旧 bool 与重复字段，合并 `snapshot` / `tabViewState` 的重复字段。

每一步的验收命令沿用项目现有流程（见 `build.ps1` / 历史记录）：

```
go test ./...
go test -race ./...
go vet ./...
gofmt -d .
```

`snapshot` 与 `tabViewState` 的字段语义在第 1、2 步保持不变，现有 UI 测试（`popup_ui_test.go`、`layout_test.go`、`ux_walkthrough_test.go`、`optimization_regression_test.go` 等）无需改动即可继续作为护栏。

### 6.3 三者收益对照

| 维度 | 现状痛点 | 显式化后的收益 |
| --- | --- | --- |
| **UX**（用户） | `busy` 类布尔只能渲染「转圈」，用户分不清是「正在请求」「已被取消」还是「失败」；状态不同步时出现空卡片、上一页签的译文、按钮仍亮着等错乱 | 每个阶段有确定语义，UI 能给出准确文案（如「正在等待模型返回」「已取消，可重新翻译」）；集中迁移杜绝「A 页签的结果画到 B 页签」；代次作废变成显式 `cancelled` 状态，旧结果不再覆盖新结果 |
| **DX**（开发者） | 40+ 个互斥 bool 散落在 20 个文件；加一个状态要改 `app`/`snapshot`/`tabViewState` 三处；竞态修复靠注释和记忆 | 状态集合与合法迁移表可枚举、可 review；`String()` 让日志自解释；新增状态只改一处定义；`transition*` 是唯一的置位入口，`go vet` 与穷举测试能覆盖非法迁移 |
| **AX**（智能体） | 智能体必须通读全仓库才能推断「现在处于什么状态、哪个事件会改哪个字段」；`gen++` 与 `time.Time` 哨兵无法被静态检索 | 状态名与迁移表是结构化、可 grep 的符号；`String()` 输出进入日志后，智能体可直接从一条日志判定现场；本清单文档（含 §8 机器可读附录）让后续任务能按 ID 定位，不必重新全量扫描 |

### 6.4 为什么这是「最佳方案」而不是别的

- 只做文档、不碰代码：DX/AX 有收益，但 UX 不变，不满足「同时提升三者」。
- 一次性大重构：UX/DX 收益最大，但工作区有 18 个未提交改动，且多个状态机是竞态修复的产物，违反「不破坏任何现有功能」。
- 引入第三方状态机库：新增依赖、改变调用模型，收益不高于标准库枚举 + 集中 `switch`，且 mygo 的消息循环与 goroutine 模型需要定制，不合适。
- **镜像式显式化**：唯一同时满足「行为零变化」「三者都有即时或近即时收益」「可逐步验证」的路径；第 1 步落地后，文档与日志立刻提升 DX/AX，第 2 步起 UX 随阶段文案与迁移收敛改善。

### 6.5 回归护栏（现有测试可直接复用）

项目已有针对这些隐式状态机的测试，显式化时它们就是验收标准，不需要新建平行测试体系：

- `optimization_regression_test.go` — Q1～Q7 竞态修复的 8 个回归用例
- `ai_generation_race_test.go`、`ai_cache_test.go` — AI pass 代次与缓存
- `popup_streaming_test.go`、`popup_ui_test.go` — 流式与渲染状态
- `selicon_tip_race_test.go`、`chip_states_visual_test.go` — 胶囊与提示状态
- `hook_lifecycle_test.go`、`gesture_test.go`、`selection_policy_test.go` — 输入手势状态
- `atomic_save_test.go`、`effort_*_test.go` — 配置保存与迁移
- `ux_walkthrough_test.go` — 端到端交互路径

缺口统一在 §4 每个状态机的「现有测试覆盖」里列出。

---

## 7. 实施优先级建议

| 优先级 | 判据 | 动作 |
| --- | --- | --- |
| **P0** | 本次即可做，零行为风险 | 落地本文档；为 §4 中「无测试覆盖」的高风险状态机补一条状态一致性断言 |
| **P1** | 收益明确、改动局部 | 按 §6.2 第 1 步对 1～2 个最高风险状态机做镜像；补充 `String()` 日志 |
| **P2** | 需要跨模块协同 | 第 2、3 步：切换读取方、合并 `snapshot`/`tabViewState` 重复字段 |
| **不推荐** | 风险大于收益 | 在未提交改动之上做全量重写；引入外部状态机框架 |

---

## 8. 机器可读附录（JSON）

供智能体与脚本按 ID 定位，不必重新全量扫描源码。`files` 相对 `fastpot/`；`anchor` 是最关键的证据位置或现象；`tests` 为 `none`（完全无针对性测试）或 `partial`（有测试但存在 §4 列出的缺口）。详细迁移表、状态集合与显式化方案见对应章节。

```json
{
  "schema": "fastdictionary-implicit-state-machines/v1",
  "scope": "fastpot/*.go (non-test) + fastpot/resources/ocr.ps1",
  "method": "只读源码，按模块并行审计；未修改任何运行时代码",
  "total": 56,
  "audit_entries": 52,
  "cross_module_entries": 4,
  "by_group": {"C": 11, "U": 17, "A": 24, "X": 4},
  "by_risk": {"high": 18, "medium": 35, "low": 3},
  "dedup_note": "三组审计共 52 条（C/U/A），另补 4 条跨模块结构性根因（X1-X4）；含跨组视角重复（见 §4.4 X4），按状态对象归并后约 40 余个独立隐式状态机",
  "top10": ["U1", "U10", "A4", "U6", "C10", "A17", "C5", "A8", "U8", "C1"],
  "recommended_path": "§6.2 镜像 → 切换读取 → 清理；每步 go test ./... / go test -race ./... / go vet ./... / gofmt -d .",
  "existing_explicit_patterns": [
    "fastpot/popup.go:23-54 popupMode + modeName()",
    "fastpot/grammar_marks.go:25-50 markKind + markBoxes map[markKind]markBox"
  ],
  "items": [
    {"id":"C1","name":"鼠标/键盘钩子手势识别","risk":"high","group":"C","section":"§4.1","files":["hook.go","selection_policy.go"],"tests":"partial","anchor":"11 个包级变量 + mutex/atomic 双套同步；clickCount 重装不清；hookThreadOnce 单向门","top":10},
    {"id":"C2","name":"低层钩子生命周期与命令投递","risk":"high","group":"C","section":"§4.1","files":["hook.go","main.go"],"tests":"partial","anchor":"postHookCommand 并发覆盖、Post 失败分支"},
    {"id":"C3","name":"划词触发模式 / 暂停 / 应用拦截","risk":"medium","group":"C","section":"§4.1","files":["tray.go","selection_guard.go","selicon.go"],"tests":"partial","anchor":"pause 未 reset gate；两个 go rebuildTray() 乱序"},
    {"id":"C4","name":"gestureGate 手势去重与冷却","risk":"medium","group":"C","section":"§4.1","files":["selection_policy.go"],"tests":"partial","anchor":"fired 长期不复位；非单调/零值时钟"},
    {"id":"C5","name":"窗口与浮层显隐 / 退出","risk":"high","group":"C","section":"§4.1","files":["main.go","popup.go"],"tests":"partial","anchor":"main.go:100-102 写 isQuitting 加锁、:287 读不加锁，明确数据竞争","top":7},
    {"id":"C6","name":"popup 卡片复合状态（布尔矩阵 + 请求代次 token）","risk":"high","group":"C","section":"§4.1","files":["popup.go","popup_actions.go"],"tests":"partial","anchor":"app 约 40 个状态布尔的总览；四 busy 组合不变式无测试"},
    {"id":"C7","name":"语音播放代次与 MCI 播放器","risk":"medium","group":"C","section":"§4.1","files":["audio.go","tts.go"],"tests":"partial","anchor":"audio.go:165-174 代次检查与 playMP3 之间竞态窗口可致串音"},
    {"id":"C8","name":"Edge TTS token 缓存","risk":"medium","group":"C","section":"§4.1","files":["tts.go"],"tests":"partial","anchor":"缓存过期/401 复位代次/并发刷新惊群无确定性单测"},
    {"id":"C9","name":"翻译引擎熔断器与竞速阶段","risk":"medium","group":"C","section":"§4.1","files":["services.go"],"tests":"partial","anchor":"services.go:582-584 注释自述 one bad answer was enough to open the breaker for a minute"},
    {"id":"C10","name":"截图框选会话（snipSession）","risk":"high","group":"C","section":"§4.1","files":["snip.go"],"tests":"partial","anchor":"snip.go:240-242 只在取指针时持锁、247-301 字段读写无锁；currentSnip 全局单例被重复 StartNativeSnip 覆盖","top":5},
    {"id":"C11","name":"取词路径选择与剪贴板事务","risk":"medium","group":"C","section":"§4.1","files":["capture.go","uia.go"],"tests":"partial","anchor":"剪贴板恢复失败/超时/中途异常；超时后遗弃 goroutine"},
    {"id":"U1","name":"AI 卡片请求代次与流式渲染","risk":"high","group":"U","section":"§4.2","files":["popup_actions.go","popup.go","popup_view.go"],"tests":"partial","anchor":"popup_actions.go:856 refineWithAI 复用 a.aiGen，点 AI 精翻静默取消卡片流；stopAIPassLocked 清 aiBusy 不清 aiText","top":1},
    {"id":"U2","name":"翻译请求生命周期","risk":"high","group":"U","section":"§4.2","files":["popup_actions.go","popup.go"],"tests":"partial","anchor":"popup.go:427-430 switchMode 直改 busy=false 而不 translateGen++，旧译文写进新模式"},
    {"id":"U3","name":"生词提取请求（notes）","risk":"medium","group":"U","section":"§4.2","files":["popup_actions.go","popup_tabs.go"],"tests":"partial","anchor":"restoreTabLocked 强制置 notesBusy=false 且不自增 notesGen，旧生词写进新 tab"},
    {"id":"U4","name":"AI 精翻请求（与 U1 共用代次）","risk":"medium","group":"U","section":"§4.2","files":["popup_actions.go","popup_translation.go"],"tests":"none","anchor":"engine 字符串哨兵 AI 精翻；重试次数硬编码一次 if；全缺口"},
    {"id":"U5","name":"模式与输入视图切换","risk":"medium","group":"U","section":"§4.2","files":["popup.go","popup_view.go"],"tests":"none","anchor":"modeSelection = modeTranslate 别名；switchMode 同 mode 早退吞掉 F5 重跑；page 为无校验字符串"},
    {"id":"U6","name":"Tab 快照与恢复","risk":"high","group":"U","section":"§4.2","files":["popup_tabs.go","popup.go"],"tests":"none","anchor":"popup_tabs.go:58-99 原样存 aiBusy/notesBusy，:101-138 强制置 false 且不自增 gen；hasContent 不看 busy","top":4},
    {"id":"U7","name":"结果卡片三态渲染（nil 哨兵 + 内容嗅探）","risk":"medium","group":"U","section":"§4.2","files":["popup_view.go"],"tests":"partial","anchor":"renderAiMarkdown busy 参数恒 false，markdown.Stream 分支不可达；looksLikeStructuredPayload 用字符串首字符嗅探"},
    {"id":"U8","name":"主窗口生命周期与置顶","risk":"high","group":"U","section":"§4.2","files":["popup.go","popup_translation.go"],"tests":"partial","anchor":"popup.go:600-603 兜底注释承认 screenshotPinned 曾泄漏到后续每个 popup；兜底只加在 openPopupWithText","top":9},
    {"id":"U9","name":"划词 Chip 显隐与动作","risk":"medium","group":"U","section":"§4.2","files":["selicon.go","popup.go"],"tests":"partial","anchor":"sampleChipPointer 只检查一次 shown；toggleMoreFlyout nil 判断与 OnClosed 回调分两处置 nil"},
    {"id":"U10","name":"Hover Tip 计时器与代次","risk":"high","group":"U","section":"§4.2","files":["selicon.go"],"tests":"partial","anchor":"selicon.go:683-686 注释自承认 window becomes unclosable until restart；迁移横跨鼠标钩子 goroutine / time.AfterFunc / RunOnMain","top":2},
    {"id":"U11","name":"自动高度收敛（振荡锁定）","risk":"medium","group":"U","section":"§4.2","files":["popup_view.go","popup_layout.go","popup.go"],"tests":"partial","anchor":"autofitLocked 只有 fitHeight 解锁，流式 chunk 不调 fitHeight 导致高度锁死文字裁切；autofitSteps 是死字段"},
    {"id":"U12","name":"展开/折叠与视图选项布尔组","risk":"medium","group":"U","section":"§4.2","files":["popup.go","popup_tabs.go","popup_ocr_pairs.go"],"tests":"partial","anchor":"tab 保存 6 个视图选项却漏 onlyTrunk/showSlashes/filterRole/grammarHoverKnowledge；ocrExpanded 无消费者时不可达"},
    {"id":"U13","name":"保存反馈与横幅","risk":"medium","group":"U","section":"§4.2","files":["popup_actions.go","popup_view.go"],"tests":"partial","anchor":"四条横幅字符串无任何自动清除路径；已保存用 EqualFold(trim(...)) 内容嗅探推导"},
    {"id":"U14","name":"AI 结果缓存与一次性跳过标志","risk":"medium","group":"U","section":"§4.2","files":["popup_actions.go","popup.go"],"tests":"none","anchor":"skipAIResultCache 由 rerunCurrentModeFresh 生产、只在 runAction 消费，中途早退残留到下一次请求"},
    {"id":"U15","name":"OCR / 截图会话（渲染侧，与 A4 同一状态机）","risk":"high","group":"U","section":"§4.2","files":["popup_ocr_pairs.go","popup.go"],"tests":"partial","anchor":"stopScreenshotOCR 三处调用但 openPopupAction 路径未调用；screenshotPinned 泄漏实锤"},
    {"id":"U16","name":"选择手势触发策略（与 C1/C3/C4 重叠）","risk":"medium","group":"U","section":"§4.2","files":["selicon.go","selection_policy.go"],"tests":"partial","anchor":"applySelectionMode 注释自承认 仅快捷键 was a one-way door ... until the app was restarted"},
    {"id":"U17","name":"grammarResultPrefill 预填合并标志","risk":"medium","group":"U","section":"§4.2","files":["popup_actions.go","popup_tabs.go"],"tests":"none","anchor":"一个 bool 承载预填来源+待合并+可覆盖；tabViewState 存该 bool 却不存 aiGen"},
    {"id":"A1","name":"AI 流式响应生命周期（Magpie SSE）","risk":"medium","group":"A","section":"§4.3","files":["magpie.go","grammar.go"],"tests":"partial","anchor":"[DONE] 缺失、坏帧、onDelta 已推送后报错无测试"},
    {"id":"A2","name":"推理强度与模型能力解析","risk":"medium","group":"A","section":"§4.3","files":["config.go","magpie.go"],"tests":"partial","anchor":"探测前后不一致、整体覆盖丢数据、未知值"},
    {"id":"A3","name":"OCR 引擎降级流水线（含预处理静默降级）","risk":"medium","group":"A","section":"§4.3","files":["ocr.go","ocr_spell.go"],"tests":"partial","anchor":"降级原因不透传；四种预处理失败；空文本 vs 失败无区分"},
    {"id":"A4","name":"截图 OCR 会话状态机","risk":"high","group":"A","section":"§4.3","files":["ocr.go"],"tests":"partial","anchor":"ocr.go:230-472 单次启动 ocrGeneration 自增 3 次、取消后旧结果仍可写回、ocrBusy 与 ocrError 可同时为真","top":3},
    {"id":"A5","name":"Indra 引擎单例懒加载","risk":"medium","group":"A","section":"§4.3","files":["indra.go"],"tests":"partial","anchor":"失败后重试成功、可选模型缺失、设置页提前触发加载、indraPadX 并发"},
    {"id":"A6","name":"Indra 识别器 / 脚本选择能力矩阵","risk":"medium","group":"A","section":"§4.3","files":["indra.go"],"tests":"partial","anchor":"scriptWorld/RTL 分支、classifyScript 失败路径、Caps 与 status 一致性"},
    {"id":"A7","name":"拼写修复词典懒加载","risk":"low","group":"A","section":"§4.3","files":["ocr_spell.go"],"tests":"partial","anchor":"加载失败、Once 不可重置、并发首次加载"},
    {"id":"A8","name":"配置加载 / 迁移多阶段","risk":"high","group":"A","section":"§4.3","files":["config.go"],"tests":"partial","anchor":"config.go:241-270 else-if 一次只回写一种迁移；:400-403 解析失败吞成 DefaultConfig() 且不覆盖磁盘","top":8},
    {"id":"A9","name":"配置保存错误 / 重试与一次性提示","risk":"medium","group":"A","section":"§4.3","files":["config.go","settings.go"],"tests":"partial","anchor":"lastSaveErr 与横幅一致性、Toast 并发消费、词库保存失败透传"},
    {"id":"A10","name":"配置 / 词库原子保存局部阶段","risk":"medium","group":"A","section":"§4.3","files":["config.go","vocabulary.go"],"tests":"partial","anchor":"词库 rename 被占用、closed 分叉、.bak 写失败"},
    {"id":"A11","name":"Config 字符串枚举族与 UI idx 映射","risk":"medium","group":"A","section":"§4.3","files":["config.go","settings_pages.go"],"tests":"partial","anchor":"settings_pages.go:313-318 把 polish 映射到 idx=2 但 330 的 modes 无 polish，往返不可逆丢失"},
    {"id":"A12","name":"设置页导航与窗口生命周期","risk":"medium","group":"A","section":"§4.3","files":["settings.go","settings_pages.go"],"tests":"partial","anchor":"同帧切页、重复 showSettings 并发探测、isQuitting 与 Hide 组合"},
    {"id":"A13","name":"热键草稿 / 应用状态","risk":"medium","group":"A","section":"§4.3","files":["settings.go","hotkey.go"],"tests":"partial","anchor":"文案反推、草稿丢弃、registerHotkeys 用新值验证"},
    {"id":"A14","name":"网关测试与模型列表探测","risk":"medium","group":"A","section":"§4.3","files":["settings_pages.go","magpie.go"],"tests":"partial","anchor":"连点竞态、探测失败标注、aiTest 三态"},
    {"id":"A15","name":"AI 指令定制编辑视图","risk":"medium","group":"A","section":"§4.3","files":["settings_pages.go","actions.go"],"tests":"partial","anchor":"取消后脏副本、ID 失效保存、脏状态提示"},
    {"id":"A16","name":"生词本页交互状态","risk":"medium","group":"A","section":"§4.3","files":["settings_vocab.go","vocabulary.go"],"tests":"partial","anchor":"状态组合残留、showSettings 后残留确认、展开+删除同时命中"},
    {"id":"A17","name":"词库导入 / 导出 / 补全异步任务","risk":"high","group":"A","section":"§4.3","files":["settings_vocab.go","vocabulary.go","vocabulary_export.go"],"tests":"partial","anchor":"settings_vocab.go:294-386 四个 go func() 无 busy/generation/cancel；vocabulary.go:988-992 saveOrLog() 吞掉持久化失败仍显示 成功导入 ✓","top":6},
    {"id":"A18","name":"词库存储单例与加载 / 保存","risk":"high","group":"A","section":"§4.3","files":["vocabulary.go"],"tests":"partial","anchor":"vocabulary.go:108-110 JSON 解析失败吞掉并按空库启动，随后覆盖原文件"},
    {"id":"A19","name":"词库导入格式探测与列映射","risk":"medium","group":"A","section":"§4.3","files":["vocabulary_export.go"],"tests":"partial","anchor":"首条空 Term、CRLF、非字符串 map 值、无表头 CSV"},
    {"id":"A20","name":"词库分类判定（三处规则不一致）","risk":"medium","group":"A","section":"§4.3","files":["vocabulary.go","vocabulary_export.go"],"tests":"partial","anchor":"三处分类一致性、writing+长句边界、Source 非法值；桌面与网页版规则不一致"},
    {"id":"A21","name":"网页版 HTML 内嵌 JS 状态","risk":"medium","group":"A","section":"§4.3","files":["vocabulary_export.go"],"tests":"partial","anchor":"handleImport 是假按钮；JS 状态一致性、快照过期提示"},
    {"id":"A22","name":"语法分析解析多级 fallback","risk":"medium","group":"A","section":"§4.3","files":["grammar.go","actions.go"],"tests":"partial","anchor":"三级 fallback 分别命中、nil 原因区分、坏 annotations 字符串"},
    {"id":"A23","name":"语法角色高亮过滤","risk":"low","group":"A","section":"§4.3","files":["grammar_marks.go","popup_view.go"],"tests":"partial","anchor":"Toggle 二次点击、未知 role、filterRole 与 knowledge 一致性"},
    {"id":"A24","name":"腾讯引擎能力表与可变 URL","risk":"low","group":"A","section":"§4.3","files":["tencent.go"],"tests":"partial","anchor":"并行改写 URL 串扰、能力表与 UI 健康一致性、非 succ 错误码分级"},
    {"id":"X1","name":"三份状态手工同步（app / snapshot / tabViewState）","risk":"high","group":"X","section":"§4.4","files":["popup.go","popup_tabs.go"],"tests":"none","anchor":"三处字段集合不一致；加一个状态要改三处，漏一处即用户可见错乱"},
    {"id":"X2","name":"代次计数器家族与 switchMode 迁移中心","risk":"high","group":"X","section":"§4.4","files":["popup.go","popup_actions.go"],"tests":"partial","anchor":"aiGen/translateGen/notesGen/ocrGeneration 四套独立代次 + 四个 cancel，命名不统一；switchMode 一次做五件事"},
    {"id":"X3","name":"两份 openPopup Reset 段","risk":"high","group":"X","section":"§4.4","files":["popup.go"],"tests":"none","anchor":"popup.go:482-528 与 :566-604 两份 30+ 字段清单不等价；:600-603 的 screenshotPinned 兜底只加在一侧"},
    {"id":"X4","name":"跨组去重映射","risk":"medium","group":"X","section":"§4.4","files":["docs/STATE_MACHINES.md"],"tests":"none","anchor":"52 条含 10 组跨组视角重叠；归并后约 40 余个独立状态对象"}
  ]
}
```

附录使用方式：按 `risk` 与 `top` 字段排优先级；按 `id` 回查 `section` 得到完整迁移表与显式化方案；按 `anchor` 中的文件与行号直接 grep 源码。`tests: "none"` 的条目对应 §7 的 P0（先补一条状态一致性断言）。

