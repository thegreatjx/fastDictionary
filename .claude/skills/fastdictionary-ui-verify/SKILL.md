---
name: fastdictionary-ui-verify
description: 验证 fastDictionary 的原生 MyGo 界面改动（配色、布局、控件状态、长难句/写作卡、设置页、划词条）。用隔离配置和独立 exe 抓真实窗口截图、控件树与证据清单，避免截到旧进程、旧主题或用户数据。
---

# fastDictionary 原生界面验证

## 何时使用

- 改动 `theme.go`、`grammar.go`、`actions.go`、`settings_pages.go`、`home.go`、
  `popup_*.go`、`selicon.go` 等界面文件。
- 用户说“看起来不对”“颜色/排版/审美有问题”，或要确认某个按钮/状态的真实呈现。
- 要在回复里声称“截图验证过”“在真实窗口里正常”。

不适用：纯后端逻辑、网络、存储，且不影响界面。协议解析用 Go fixture 测试，
不要用界面截图代替。

## 铁律

1. **原生 UI。** 只用 `github.com/egoist/mygo/ui` 的 Go API；禁止 WebView/HTML/CSS。
2. **证据有身份。** 没有 `manifest.json` 的截图不算验证；manifest 必须与当前
   `git rev-parse HEAD`、exe SHA256、主题/强调色一致。
3. **隔离配置。** 不读、不改用户的 `%APPDATA%\fastDictionary`；用 harness 的
   `%APPDATA%\fastDictionary-uiverify`。
4. **真实窗口。** `ui.Tester` 只能证明结构/文本/布局，不能证明像素、遮挡、
   透明输入区域、窗口圆角。视觉结论必须来自原生 `screenshot` + `snapshot`。
5. **主题覆盖。** 改 `theme.go` 或语义角色：浅色 + 深色都看；涉及强调色联动：
   4 套强调色都跑。只截默认 sky 浅色不算完成。
6. **状态覆盖。** 不仅看静止态，还要看 `hover` / `pressed` / `selected` /
   `disabled`。框架会自动改写的态必须用控件树或交互确认。

## 快速流程

在仓库根目录：

```powershell
# 1. 先跑角色/对比度/结构契约
go test -run 'TestGrammarPaletteContrastAcrossThemes|TestGrammarClauseWashHierarchy|TestGrammarDifficultyStaysNeutral|TestDesignSystem|TestUIFilesHaveNoEmojiOrHardcodedColors' -count=1 .

# 2. 抓浅色证据（默认输出到 build/ui-evidence/<时间>-<视图>-<主题>-<强调色>/）
.\scripts\ui-evidence.ps1 -View grammar -Theme light -Accent sky

# 3. 抓深色 / 其他强调色
.\scripts\ui-evidence.ps1 -View grammar -Theme dark -Accent mint

# 4. 工作台、设置页、写作卡
.\scripts\ui-evidence.ps1 -View home     -Theme light -Accent sky
.\scripts\ui-evidence.ps1 -View settings -Theme light -Accent sky -Page general
.\scripts\ui-evidence.ps1 -View writing  -Theme dark  -Accent violet
```

每个目录包含：

| 文件 | 用途 |
| --- | --- |
| `manifest.json` | commit、工作区状态、exe SHA256、配置路径/内容、主题、强调色、窗口、时间、Go 版本 |
| `screenshot.png` | 原生窗口截图（`mygo-automation screenshot`） |
| `control-tree.txt` | 原生控件树（`snapshot -i --full`），含 `disabled`/`checked`/`actions` |
| `windows.txt` | 自动化列出的窗口及 visible/hidden 状态 |

先打开 `manifest.json` 核对身份，再看图。身份不对，图作废，不要“差不多”。

## 手工核对（harness 之外）

- 空输入时“翻译”按钮应为 `disabled`，且没有 `press` 动作。
- 长难句卡的“只看主干 / 断句 / 保存到长难句本”应可见、可聚焦；隐藏窗口也能
  截图，但要用控件树确认它真的渲染了。
- 主题色样本：选中态 2px `TextSecondary` 圆环 + 对勾；未选中 1px `BorderCtrl`。
- 划词关闭键：常驻胶囊右上角；在窗口透明边（例如坐标 195,3）能命中，点击只
  收起胶囊，不改变选区。
- 设置页导航：每页都能渲染；不要用 `--page home --shot-window settings`，`home`
  会被重定向到通用页。

## MyGo 原生限制（踩过的坑）

| 限制 | 后果 | 做法 |
| --- | --- | --- |
| 内联 `RichText` 子元素的 `Padding`/`Radius`/`Border` 不生效 | 写了也没有视觉变化 | 用 `Box` 包一层，或去掉无效调用 |
| `Selected(true)` 会强制刷主题色背景 | 图标与背景同色，整块看不清 | 只染图标，或显式接受框架绘制并验证 |
| `Disabled(true)` 自动降透明度 | 自定义底色 + 白字会变成淡色块 | 用 `DimDisabled(false)` + 显式 `TextDisabled` |
| 透明窗口区域默认点击穿透 | 角落按钮点不到 | 用 `SetInputRegion` 显式声明可点击区 |
| `CreateRoundRectRgn` 参数是宽高，不是半径 | 圆角形状错误 | 传宽高，单独验证像素 |
| 隐藏的弹窗窗口不出现在普通截图里 | 以为没渲染 | 用 `mygo-automation` 连接并选中 `w1` |
| 单实例锁会把参数交给旧进程 | 看起来新版本没生效 | 用独立 exe 名 + `--elevated-takeover`；或先停旧进程 |

## 常见故障

- **截到旧界面**：旧进程仍在运行。确认 `manifest.exe_sha256` 与当前构建一致；
  harness 用独立 exe 名，正常不会撞上用户实例。
- **主题不对**：配置被另一份数据目录读取。harness 每次重写 fixture；手工验证时
  先读 `%APPDATA%\fastDictionary-uiverify\config.json`。
- **窗口选错**：`windows.txt` 里 `w1` 可能是隐藏弹窗；选非 `hidden` 的那一行。
- **截图被遮挡**：原生截图可能拿不到被其他窗口覆盖的帧；把目标窗口置前，
  或让 harness 的 `close_on_blur=false` 生效。
- **中文徽章断行**：内联文本默认可在两个汉字之间断行；用 `NoWrap()` 或改结构。

## 报告格式

回复里至少给出：

```text
证据：build/ui-evidence/2026.../manifest.json
commit=<sha> exe=<sha256 前 12 位> theme=<light|dark> accent=<sky|mint|violet|coral>
截图：.../screenshot.png
控件树：.../control-tree.txt
结论：<看了哪些状态/主题；哪些通过；哪些未验证>
```

未跑的检查、模拟渲染、`ui.Tester` 结果不能写成“真实窗口通过”。

## 相关文件

- `docs/ENGINEERING_CONTRACT.md`：边界契约与提交检查表。
- `docs/RETROSPECTIVE_2026-11.md`：这些规则为什么存在。
- `docs/DESIGN.md`：颜色、字阶、组件契约。
- `scripts/ui-evidence.ps1`：证据采集脚本。
