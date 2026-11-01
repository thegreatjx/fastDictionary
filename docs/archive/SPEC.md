# fastpot 产品规格（Spec v1）——融合 pot-desktop × read-frog 的桌面翻译客户端

> 2026-10-10 · 基于两个开源项目的源码级逆向 + 本机网络/音频实测
> 原则：**速度优先、UI 精致**。热路径零 WebView；富界面才上 WebView。

## 0. 两个源项目的优劣结论（源码级证据）

| | pot-desktop (19.4k★, 已归档) | read-frog 阅读蛙 (浏览器插件) |
|---|---|---|
| 形态 | Tauri v1 + React 桌面托盘应用 | WXT + React 1470 文件/26.8 万行浏览器扩展 |
| 优点 | 全局热键/托盘/OCR/剪贴板监听等**系统级能力**；服务插件化架构清晰 | 翻译引擎层最先进：微软免鉴权新端点、谷歌 translateHtml（HTML 结构保留+换行 marker 对）、**Edge TTS 神经语音**（HMAC 签名已破解，纯标准加密）、页面级队列/缓存/术语表/morphdom 增量 DOM、字幕平台适配器（YouTube/Netflix）、AI 自定义动作（词典/长难句/写作改进=可配置 prompt+provider） |
| 缺点 | **Tauri v1 停更**；每次唤起冷建 WebView 窗（慢的根源）；微软接口按旧 token 流程实现已废；前端巨石（TargetArea 880 行） | 受浏览器沙箱限制：**无全局热键**（需页面有焦点）、无 OCR、TTS 播放要绕 offscreen document；26.8 万行的复杂度对个人重构不可搬全量 |
| 结论 | 拿它的**产品骨架**（热键→弹窗→服务并发→历史） | 拿它的**引擎与算法**（端点、payload 形状、签名、行保持、队列思路） |

## 1. 产品定位

**fastpot = 桌面级阅读蛙**：read-frog 的引擎智慧 + pot 的系统级外壳，全部装进一个 ~18MB 的 Go 原生二进制。
核心体验指标（对照欧陆词典/pot 的慢）：
- 热键→弹窗可见：**< 50ms**（原生 UI，无 WebView 冷启动；已验证 mygo native 模式）
- 弹窗→首条译文：**< 600ms P90**（微软+谷歌竞速，实测 195ms–1s）
- 单词→音标+发音可点：**< 300ms**（有道 jsonapi 103ms + dictvoice 音频 108ms）
- 常驻内存：< 40MB（当前 23MB）

## 2. 功能矩阵（按里程碑）

### M1 划词弹窗增强 ✅ 已完成（实测数据）
- [x] 微软⇄谷歌双路竞速（先回先用，输家 cancel）——实测 195ms–1s，微软全胜，谷歌 429 被掩盖
- [x] 本地语言方向启发式（零网络）
- [x] **单词词典卡**：英/美 IPA 音标 + 词性释义（有道 jsonapi **实测 106ms**；谷歌 dt=bd POS 分组互补，双源并发拉取）
- [x] **发音按钮**：🔊英 / 🔊美（有道 dictvoice mp3 **实测 24ms 下载** → winmm.dll mciSendString 直接播放，零外部依赖，无 mpv/ffmpeg）
- [x] **AI 精翻按钮**：magpie 网关 flash+low **实测 2.8s 流式**返回（"敏捷的棕色狐狸跃过那只懒狗"），SSE 逐 chunk 进原生 UI；旧 AI 任务自动 cancel
- [x] 弹窗 body 改 ui.Scroll 可滚动，词典卡/AI 段落任意窗口高度都放得下
- [x] 失焦/Esc/关闭时 stopAudio()，不会出现窗口关了声音还在放
- 测试：6 个单元（含词典卡/AI 段渲染）+ 3 个联网集成全绿；二进制 18.3MB

### M2 真·划词取词
- Windows UIA（purego 调 UIAutomationCore，mygo 已带 purego 依赖）TextPattern 取选中；
- 失败回退模拟 Ctrl+C（pot selection crate 同款方案，兼容一切程序）；
- 剪贴板保护：取词前备份、取词后还原（pot 没做好这点，用户会丢剪贴板）。

### M3 输入翻译 + 历史
- 弹窗内输入模式（pot 的 [INPUT_TRANSLATE] 思路）；
- SQLite 历史（沿用 pot history.db 表结构：text/source/target/service/result/timestamp）；
- 增量翻译/动态翻译开关（pot 的 incremental/dynamic_translate）。

### M4 AI 动作系统（学 read-frog custom-actions）
- 可配置动作列表：默认三件套 = **词典**（面向学习者 prompt）、**长难句分析**（grammar tutor）、**写作改进**（writing teacher）；
- 每动作 = prompt 模板 + magpie 模型 + 推理档位（低/中/高 → flash/low、flash/high、大模型）；
- 弹窗内下拉选择动作，流式渲染。

### M5 网页整页翻译（WebView 窗，学 read-frog page-translation）
- 内置浏览器窗（mygo WebView 模式）或注入外部浏览器（代理模式，二期再议）；
- 双语对照/直接替换两种模式（read-frog 的 morphdom 增量更新思路）；
- 翻译队列 + hash 缓存（read-frog translation-queues/dexie 思路，本地用 SQLite）；
- HTML 感知翻译：谷歌 translateHtml（实测 200，标签保留，但 5.1s 偏慢→只做整页不做划词）；
- 行保持 marker 对（read-frog google.ts 的 <br data-lb> 成对策略，直接移植算法）。

### M6 文件精读
- docx/pdf/epub 导入 → 分段流水线 → magpie 流式精翻 → 双语对照排版 → 导出；
- 术语表（read-frog glossary 思路）。

### M7 视频字幕（远期，桌面独有玩法）
- 桌面端没有 DOM，字幕走**系统级 OCR 悬浮层**：屏幕区域截图→Windows.Media.OCR→翻译→置顶悬浮条；
- read-frog 的 YouTube/Netflix DOM 适配器不可移植，但分段/调度算法（segmentation-pipeline/subtitles-scheduler）可参考。

## 3. 技术栈（已定案+已验证）

| 层 | 选型 | 验证状态 |
|---|---|---|
| 外壳 | mygo v0.3.7（锁版本），原生 UI（划词弹窗）+ WebView（设置/整页/精读） | ✅ 弹窗跑通，23MB 常驻 |
| 快翻译 | 微软 translatetext（免鉴权，195–560ms）⇄ 谷歌 gtx googleapis（间歇429，竞速掩盖） | ✅ 实测 |
| 词典 | 有道 jsonapi（音标+释义，103ms）+ gtx dt=bd 备胎 | ✅ 实测 |
| 发音 | 有道 dictvoice mp3（108ms）→ winmm.dll mciSendString（零依赖） | ✅ 实测 |
| 神经语音(二期) | Edge TTS：HMAC-SHA256 签名（read-frog constants.ts 公开材料）→ dev.microsofttranslator.com 拿 token → {region}.tts.speech.microsoft.com SSML 合成 | 算法已拿到，Go 可实现，M4+ 再接 |
| AI | magpie 网关 http://127.0.0.1:3425/v1/chat/completions，flash+reasoning.effort=low | ✅ 实测 |
| 存储 | SQLite（历史/缓存/术语表） | pot 同款，成熟 |
| UI 打磨 | frontend-design + web-design-guidelines skill（已装） | ✅ |

## 3.5 实现备忘（踩坑与结论）

- **mygo 布局陷阱（重要）**：在 `ui.Column` 里，兄弟元素用 `ui.Row(c).Fill()` 会同时抢占垂直空间，把 `ui.Scroll(...).Grow(1)` 压成 0×0（表现为滚动区内容全部不可见、后续元素整体下移）。**行容器一律用 `FillWidth()`**，只有需要撑满高度的容器才用 `Fill()`/`FillHeight()`。`ui.Divider(c).Fill()` 同理会被纵向撑成一大块灰色矩形，必须用 `FillWidth()`。
- **调试手段**：`ui.NewTester(view, w, h)` + `Tester.Find(text) (Rect, bool)` 能直接读元素几何，比截图快得多；`Tester.Image()` 可渲染成 PNG。应用内置 `--shot <png> [--shot-window settings] [--shot-delay ms]`，用 `Window.CapturePage()` 自截窗口，绕开 DPI 缩放与 z-order 干扰。
- **弹窗高度**：按内容估算行数自适应（CJK 按 1 字宽、拉丁按 0.55 字宽估算换行），150–620px 钳制，超出交给 Scroll。
- **配置路径**：`%AppData%\fastpot\config.json`（开发版为 `fastpot-dev`），首次运行即落盘默认值。
- **不要用测试验证剪贴板副作用**：模拟 Ctrl+C 的取词测试会在无焦点环境阻塞，剪贴板保护逻辑改为人工验证。

## 4. 非目标（明确不做）

- 不做浏览器插件（read-frog 已覆盖该场景，fastpot 是它的桌面互补）；
- 不搬 read-frog 全量 26.8 万行（只移植算法与端点知识）；
- OCR 截图翻译 M7 后再议（pot 的 tesseract/系统 OCR 方案可参考）；
- 不做账号/云同步（pot 的 webdav/aliyun 备份可选做）。