# fastpot 划词功能 —— 交接文档

> 交接时间：2026-10-11 · 交接原因：划词小图标功能未完成，用户要求交给下一个模型
> 项目路径：`D:\AI_workspace\translate\fastpot`（**不是 git 仓库**，无版本回退）
> 构建产物：`fastpot-dev.exe`（`go build -o fastpot-dev.exe .`）

---

## 一、用户报告的原始问题（原话）

1. **"划词翻译都有问题，居然要自己输入，而不是划词了直接翻译"**
   —— 按 Ctrl+Alt+F 后弹出的是**输入框**，不是译文。
2. **"划词这图标都不出现在屏幕上而是缩小在后台任务栏"**
   —— 划词小图标没有浮在鼠标旁，而是掉到任务栏里去了。

第 1 个问题**已定位并修复**（见第三节）。第 2 个问题**部分修复，未完成**（见第四节，这是交接重点）。

---

## 二、当前状态一览

| 项 | 状态 |
|---|---|
| `go build ./...` | ✅ 通过 |
| `go tool mygo vet .` | ✅ 干净（0 问题） |
| 离线单元测试 | ✅ 全绿（`go test -run 'TestGesture\|TestChip\|TestPlaceChip\|TestSelection\|TestPopup\|TestSettings\|TestWaitFor\|TestKeyDown\|TestModifier\|TestDetect\|TestIsWord' .`） |
| 划词取词（快捷键路径） | ✅ 已修，**待用户实机确认** |
| 划词小图标显示 | ❌ **未完成**：窗口能创建、能置顶、不再进任务栏，但**位置固定在 (0,0)** |
| 划词小图标交互（点击翻译） | ⚠️ 代码已写，因窗口位置问题**未验证** |

---

## 三、已修复的问题（含根因，源码级）

### 3.1 【核心 bug】划词变输入框 —— 快捷键的修饰键还按着就模拟 Ctrl+C

- **文件**：`capture.go`
- **根因**：用户按 `Ctrl+Alt+F` 触发时，**Ctrl 和 Alt 在物理上仍处于按下状态**。此时合成 `Ctrl+C`，目标程序收到的是 `Ctrl+Alt+C`，**什么都没复制** → 取词为空 → 代码回退到输入模式（用户看到的"填词翻译"）。
- **修复**：新增 `waitForModifiersReleased()`，用 `GetAsyncKeyState` 轮询 Ctrl/Alt/Shift/LWin/RWin 全部松开后才发 Ctrl+C。
  ```go
  // capture.go
  var modifierKeys = []uintptr{vkShift, vkControl, vkMenu, vkLWin, vkRWin,
      vkLShift, vkRShift, vkLControl, vkRControl, vkLMenu, vkRMenu}
  func keyDown(vk uintptr) bool { s,_,_ := procGetAsyncKeyState.Call(vk); return int16(uint16(s)) < 0 }
  func waitForModifiersReleased(timeout time.Duration) bool { /* 轮询直到全部松开 */ }
  // captureByCopy() 第一行：waitForModifiersReleased(600*time.Millisecond)
  ```
- **参考实现**：pot-desktop `src-tauri/src/window.rs:226` 的 `selection_translate()` —— 先 `get_text()` 再 `translate_window()`，顺序不能反。

### 3.2 取词永远为空 —— 先弹窗再取词，焦点被自己抢走

- **文件**：`popup.go` `openPopup()`
- **根因**：原代码先 `win.Show()` + `win.Focus()`，**然后**才调 `CaptureSelection()`。此时前台已变成 fastpot 自己，UIA 和 Ctrl+C 都只能拿到自己的东西 → 永远取空。
- **修复**：改为**先取词、后弹窗**。拆出 `openPopupWithText(m, text)`，取词在它之前完成。
  ```go
  func (a *app) openPopup(m popupMode) {
      a.hideSelIcon()
      if m == modeSelection {
          sel := a.CaptureSelection()          // 先取词
          if sel.Text == "" { a.openPopupWithText(modeInput, ""); return }
          a.openPopupWithText(modeSelection, sel.Text)  // 后弹窗
          return
      }
      a.openPopupWithText(m, "")
  }
  ```

### 3.3 剪贴板被破坏

- **文件**：`capture.go` `snapshotClipboard()` / `restoreClipboard()`
- **根因**：原实现只备份/还原纯文本，用户剪贴板里的图片、富文本会丢。
- **修复**：`mygo.Clipboard.Read()` 全格式快照 + `transfer.Bytes` 逐格式还原，跳过进程内私有格式（`DataObject`/`Ole Private Data`/`MyGo.Transfer.Length:`）。富文本还原失败时回退纯文本。

### 3.4 取词失败时静默变输入框，用户不知道发生了什么

- **文件**：`popup.go`（新增 `notice` 字段）
- **修复**：取词为空时在弹窗顶部显示橙色提示，并注明是哪个程序，指引用户改「取词方式」。

### 3.5 【诊断代码自身的 bug，务必注意】

- **文件**：`capture.go`
- **问题**：我最初把 `GWL_EXSTYLE` 写成 `-16`，实际 `-16` 是 `GWL_STYLE`，`GWL_EXSTYLE` 是 **-20**。这导致我一度得出"窗口是 layered 的"这个**完全错误的结论**，浪费了大量时间。
- **现状**：已修正为 `gwlExStyle = ^uintptr(19)`（-20）、`gwlStyle = ^uintptr(15)`（-16）。

---

## 四、未解决的问题（交接重点）—— 划词小图标位置固定 (0,0)

### 4.1 已确凿的实测证据

用 `fastpot-dev.exe --chip` 启动（`main.go` 里的调试开关，会在 (760,520) 显示图标并打印窗口真实状态）：

```
chip hwnd=0x250e60 rect=(0,0 263x67) visible=true iconic=false
  exstyle=0x8000188 toolwindow=true layered=false topmost=true noactivate=true appwindow=false
  style=0x6ca0000 child=false popup=false visibleBit=true owner=0x0
fastpot: raw SetWindowPos ret=1 err=The operation completed successfully. -> (0,0), moved=false
fastpot: MoveWindow ret=1 err=The operation completed successfully. -> (0,0), moved=false
```

**关键矛盾（这就是没解决的问题）**：
- 期望位置 `(434,305)`（DIP），窗口真实矩形是 `(0,0 263x67)`
- **尺寸生效了**：263x67 = 150x38 @ 175% DPI（150×1.75=262.5）
- **位置没生效**：始终 (0,0)
- `SetWindowPos` **返回 1（成功）**、`MoveWindow` **返回 1（成功）**，`GetLastError` = "操作成功完成"
- 但紧接着 `GetWindowRect` 读回来还是 `(0,0)`
- **连 mygo 自己的 `win.SetPosition()` / `win.SetBounds()` 也无效**（`win.Bounds()` 一直返回 `{X:0 Y:0 Width:150 Height:38}`）

### 4.2 已经排除的原因

| 假设 | 结论 | 证据 |
|---|---|---|
| `Transparent: true` 导致 layered 窗口 | **已排除** | 已移除该选项；修正 GWL_EXSTYLE 常量后实测 `layered=false` |
| 窗口进任务栏 | **已修复** | 强制设置 `WS_EX_TOOLWINDOW` 后 `toolwindow=true`，任务栏不再出现 |
| 置顶失效 | **已修复** | `topmost=true` 实测确认 |
| 先设位置后 Show 导致被忽略 | **已排除** | 已改为 Show 之后、在 `mygo.RunOnMain` 里设置，仍在 (0,0) |
| 父窗口/所有者窗口 | **已排除** | `owner=0x0` |
| 是子窗口 | **已排除** | `child=false`、`popup=false` |

### 4.3 我的下一步假设（按可能性排序，请下一个模型验证）

1. **mygo 内部有代码在每帧重新定位窗口。**
   嫌疑点：`internal/windows/surface.go:173 fit()` 会调 `SetWindowPos`；`internal/windows/window.go:214 placeInitially()`；`window.go:327`。
   注意 `fit()` 操作的是子 surface 窗口（`wsChild`），理论上不该影响顶层窗口，但**值得用日志确认**。
   **验证方法**：在 `mygo` 源码里给每一处 `procSetWindowPos.Call` 加日志（改 `GOMODCACHE` 里的源码，或 `go mod edit -replace` 指向本地副本），看谁在把窗口拉回 (0,0)。

2. **窗口其实移动了，但 `GetWindowRect` 读到的是别的窗口。**
   已用 `EnumWindows` 列出本进程全部顶层窗口，只有一个 "fastpot selection"（hwnd=0x250e60），所以可能性较低，但**未 100% 排除句柄复用**。

3. **DPI 虚拟化 / 进程 DPI 感知模式问题。**
   进程若是 DPI-unaware，`SetWindowPos` 的坐标会被系统按虚拟化规则改写。fastpot 用 mygo（DWM/DirectComposition + `GetDpiForMonitor`），理论上 per-monitor aware，但**未验证**。
   **验证方法**：`GetProcessDpiAwareness` 读实际感知级别。

4. **窗口在非交互式桌面 / 不同虚拟桌面。**
   可能性低（窗口能显示、能置顶），但我的测试是 `Start-Process` 拉起的，**建议用户手动双击 exe 再测一次**以排除。

### 4.4 给下一个模型的建议

- **先做第 4.3.1 条**（给 mygo 的 SetWindowPos 加日志），这是最可能的根因，能一次性定位。
- 或者**绕开 mygo 的定位机制**：不移动窗口，而是**创建一个覆盖全屏的透明/穿透窗口**，图标画在这个全屏窗口内部对应的坐标上（很多划词工具就是这么做的，避免窗口移动的坑）。用 `SetIgnoreMouseEvents` 做穿透 + 在图标区域响应点击。
- **不要再重复我的错误**：不要用截图验证（我早期调用截图工具留下了 `ScreenCapture.exe` 反复抢前台，污染了后续所有端到端测试，浪费大量时间）。用 `--chip` 开关 + Win32 状态日志来量。

---

## 五、已经写好的功能代码（可直接复用）

### 5.1 `uia.go` —— UI Automation 取词（COM 直调，无 cgo）

- 通过 `syscall.SyscallN` 直接调 COM vtable，零依赖。
- 常量来源：`uiautomationclient.h`，本地副本在 `.research/uiautomationclient.idl`（Wine 镜像，权威）。
- **vtable 槽位务必注意 IUnknown 占 0/1/2**，接口自己的第一个方法从 **3** 开始。已按此实现并在注释里逐个标注。
- 已实现：`GetFocusedElement`、`ElementFromHandle`、`ElementFromPoint`、`GetCurrentPattern(TextPattern)`、`GetSelection`、`GetText`、`GetBoundingRectangles`（含 SAFEARRAY 解析）、密码框跳过（`UIA_IsPasswordPropertyId`）。
- **实测结论（重要）**：**Chrome 不暴露 TextPattern** —— `FindAll(Subtree)` 返回 0 个元素。所以**浏览器里只能走模拟复制回退路径**。Notepad/Word/WinUI 走 UIA 快路径。

### 5.2 `hook.go` —— 全局低级钩子

- `WH_MOUSE_LL` + `WH_KEYBOARD_LL`，识别三种划词手势：
  - 鼠标拖拽（位移 > 6px）
  - **双击选词**（500ms 内、位移 < 4px 的第二次按下）
  - Shift + 方向键
- 回调里只记录状态并起 goroutine，绝不阻塞钩子。

### 5.3 `selicon.go` —— 小图标（窗口定位未解决，其余可用）

- 三种模式：`icon`（显示小图标，默认）/ `auto`（直接翻译）/ `hotkey`（仅快捷键）。
- `placeChip` 是**纯函数**，已单测覆盖：贴边翻转、跨屏 DPI、多显示器。
- 图标按钮：译 / AI / 复 / ✕。

### 5.4 测试文件

| 文件 | 覆盖内容 |
|---|---|
| `gesture_test.go` | 三种触发模式的策略、空选区不动作、锚点优先用选区包围盒、无包围盒回退鼠标位置 |
| `modifier_test.go` | 修饰键释放等待、`keyDown` 正确性、修饰键清单完整性 |
| `selicon_test.go` | 图标渲染、AI 关闭时隐藏 AI 按钮、圆角透明、跨屏定位、设置页渲染 |

---

## 六、环境与配置

- 配置路径：`%AppData%\fastpot-dev\config.json`（开发版）
- **关键配置项**：
  - `selection_trigger`: `icon` | `auto` | `hotkey`
  - `selection_capture`: `auto` | `copy` —— **取词失败时先让用户改成 `copy` 试**
- 热键：`Ctrl+Alt+F`（划词）、`Ctrl+Alt+S`（输入）
- 调试开关（`main.go`）：`--settings`、`--input`、`--demo <text>`、`--demo-selection <text>`、**`--chip`**（本次新增，显示图标并打印窗口状态）、`--shot <png>`（**不要用**，会留下抢前台的进程）

---

## 七、用户明确表达过的偏好（务必遵守）

1. **不要动不动截图** —— 用日志和 Win32 状态量测。
2. **不要反复循环尝试同一个动作** —— 想清楚再动手。
3. **参考已有实现** —— `pot-desktop` 和 `read-frog` 两个源码都在本地（`D:\AI_workspace\translate\pot-desktop`、`D:\AI_workspace\translate\read-frog`），**先读它们怎么做，再写代码**。
4. 用户已经因为反复失败而更换模型，**请一次做对，先验证根因再改代码**。
