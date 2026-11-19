# fast词典 (fastDictionary)

<p align="center">
  <strong>极速响应、原生轻量的 Windows 桌面划词翻译与学习伴读工具</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Language-Go%201.27-00ADD8?style=flat-square&logo=go" alt="Go Version" />
  <img src="https://img.shields.io/badge/Platform-Windows-0078D6?style=flat-square&logo=windows" alt="Platform" />
  <img src="https://img.shields.io/badge/Runtime-Native%20(No%20WebView)-059669?style=flat-square" alt="Native Runtime" />
  <img src="https://img.shields.io/badge/License-MIT-blue?style=flat-square" alt="License" />
</p>

---

## 📥 下载

[前往 Releases 下载最新版](https://github.com/thegreatjx/fastDictionary/releases/latest)

- **绿色版**：`fastDictionary-vYYYY.MM.DD-win64.zip`，解压后双击 `fastDictionary.exe`，无需安装
- **校验文件**：同名 `.sha256`，可选，用来确认下载完整
- 本项目不提供 `setup.exe` 安装程序，也不写注册表；删除解压目录即可卸载

> GitHub Release 页面底部的 **Assets** 区域才是下载入口；那里如果只看到 `Source code (zip/tar.gz)`，那是源码，不是可执行程序。

---

## 🌟 为什么选择 fast词典？

在桌面阅读与学习英语时，许多现代翻译工具要么依赖笨重的 Electron / WebView 框架（内存占用数百兆、唤醒延迟明显），要么充斥着高饱和渐变与卡片堆砌的视觉噪音。

**fast词典 (fastDictionary)** 秉持三项核心原则打造：
1. **Native Precision（原生精准）**：纯 Go 开发（基于 [mygo](https://github.com/egoist/mygo) `v0.10.0` 原生渲染，默认启用 `-tags mygo_nowebview`），无 cgo、无重量级浏览器内核。二进制体积轻量，划词唤醒延迟低，内存占用极低。
2. **Quiet Craft（克制考究）**：拒绝粗劣套壳与视觉噪音。悬浮条采用硬件级区域裁切（`CombineRgn`），0 杂色 0 黑边；冷调低饱和配色系统，悬停即时提示操作释义。
3. **Learner First（学习者优先）**：不仅仅是查一个中文释义，更提供 CEFR 难度分级、标准 IPA 音标、长难句语法树拆解、AI 写作润色对比与生词本管理。

---

## 📸 界面预览

| 悬浮胶囊工具条 (Selection Chip) | 悬停即显功能释义 (Flyout Tip) |
| :---: | :---: |
| ![Selection Chip](docs/screenshots/chip.png) | ![Chip Tip](docs/screenshots/chip-tip.png) |

| 学习者词典 (IPA 音标 / CEFR 分级 / 语境例句) | 长难句语法拆解 (主谓宾 / 从句高亮) |
| :---: | :---: |
| ![Learner's Dictionary](docs/screenshots/word.png) | ![Grammar Analysis](docs/screenshots/grammar.png) |

| 英文写作改进与润色 (Diff 对比) | 划词翻译面板 (多引擎聚合) |
| :---: | :---: |
| ![Writing Improvement](docs/screenshots/writing.png) | ![Translate Panel](docs/screenshots/translate.png) |

<p align="center">
  <strong>深浅色模式与原生设置面板</strong><br/>
  <img src="docs/screenshots/settings.png" width="80%" alt="Settings Panel" />
</p>

---

## ✨ 核心特性

### 1. ⚡ 原生划词悬浮胶囊 (Selection Chip)
- 鼠标在任意应用中选中文本后，即刻就近呼出中性纸面胶囊工具条。
- **色彩克制**：仅主要操作带低饱和强调色（可选晴空蓝 / 薄荷青 / 薰衣紫 / 暖珊瑚），次要按钮共用中性墨色，避免眼花缭乱。
- **悬停气泡提示**：鼠标移入任一按钮 160ms 即在正上方浮现功能气泡（`WS_POPUP`），移动时顺畅跟随无等待。
- 一键直达：划词翻译、真人发音朗读、查词典、长难句分析、写作润色或复制搜索。

### 2. 📘 学习者深度词典 (Learner's Dictionary)
- 自动解析并展示权威 **IPA 国际音标**，支持一键真人朗读发音。
- **CEFR 欧标难度分级**（中性墨阶徽章，A/B/C 靠字重与墨色区分，不用三色标签），词汇难易一目了然。
- 提炼双语例句、句中词形还原与**词根/词源助记技巧 (Memory Tips)**。
- 支持一键收录至内置生词本。

### 3. 🔍 长难句深度语法分析 (Grammar Tutor)
- **Grammar Analysis (`Grammar Tutor`)**：基于英语教学 SVOCM 骨架法与括号法，区分主谓宾骨架、嵌套从句、介词/非谓语短语（`介短` / `分词` / `不定式` / `动名` 括号微标与虚线底纹），无从句长句也能层层拆解；支持「只看主干」「断句」与「结构树」三种视图。
- 智能识别各级从句类型（宾从、定从、状从、同位从句等）与链式后置修饰指向关系，并在下方给出难点提炼与地道整句精翻。

### 4. ✏️ 英文写作改进与批注 (Writing Improvement)
- 结构化对比输入原文与润色建议。
- 逐项呈现修改批注（语法纠错、用词替换、语气地道化、表达精炼等），并说明修改理由。

### 5. 🌐 多引擎聚合与 AI 翻译 (Multi-Engine & AI)
- 聚合多种翻译服务：**Google 翻译**、**微软 Bing**、**有道词典**、**腾讯翻译**以及 **OpenAI / Claude / 自定义兼容的大模型 API**。
- 支持流式实时打字机回显与 **AI 精翻（Refine）**，进一步提升译文质感。
- **本地持久化查询缓存**：翻译结果、词典、「猜你想存」生词提取以及 AI 精翻 / 长难句 / 写作结果自动落盘复用；弹窗未钉住关闭后重选相同内容直接秒开、零额外 Token 消耗（点「重新翻译 / 重新生成」可强制刷新）。

### 6. 📷 本地高精 Indra OCR 截图识字
- 搭载本地轻量离线 **Indra OCR** 引擎（基于 `onnxruntime_purego` 纯 Go 接口），内置 31MB 轻量高精度模型。
- 毫秒级快速推理，纯本地离线运行，无需外网或云端 API。
- 自带英文常见 OCR 错误自动校正（OCR Spell Correct）。
- 截图翻译支持 AI 精翻：先发送本地 OCR 文字，失败时自动附带原截图校正识别错误。
- 支持在未部署模型时自动回落至系统原生的 `Windows.Media.Ocr`。

### 7. 📚 「生词本」与「句法笔记」分库管理
- **「生词本」(`vocabulary.json`)**：专注生词与例句积累，支持音标与 CEFR 难度补全、**间隔重复复习**（忘记 / 模糊 / 记得，最长 180 天间隔）、JSON / Excel CSV 导入导出及 Anki 卡片生成。
- **「句法笔记」(`notebook.json`)**：独立归档「长难句」结构树（`phrases`/`clauses`/`difficulties` 完整交互复现，支持继续切换主干/断句/结构树）与「写作批注」，旧版条目首次启动自动无损迁移分流。
- 托盘菜单与工作台首页自动统计待复习生词数量，点击即可开启卡片复习。

### 8. 🖥️ 一体化主窗口与查词工作台
- 主窗口内置「查词翻译」工作台：多行输入回车即查、`Shift+回车`换行，粘贴并翻译、截图翻译；译文与词典 / 长难句拆解 / 写作润色三态切换，AI 精翻、朗读与收藏都在同一页。
- 左栏按「工作台 → 常规 → 划词与翻译 → AI 与指令 → 其他」分组，底部常驻监听状态胶囊，随时看得出划词是开着、暂停还是仅快捷键。
- 普通启动直接打开工作台；开机自启（`--autostart`）与 `--minimized` 静默进托盘，再次双击 exe 即可唤醒主窗口。
- 托盘左键直达主窗口，右键菜单首项也是「打开主窗口」；关掉主窗口不影响后台划词。
- 长难句整段拆解：展示主句骨架高亮、从句染色微标、括号法短语标注、核心谓语下划线及语法难点卡片，支持「只看主干」「断句」与缩进「结构树」视图。

---

## ⌨️ 默认快捷键

| 快捷键 | 功能 | 说明 |
| :--- | :--- | :--- |
| `Alt + D` | **划词翻译** | 选中文本后呼出主面板，或直接在光标处查词 |
| `Alt + Q` | **输入查词/翻译** | 快速弹出空输入框，随时键入中英文查询 |
| `Alt + C` | **截图识字翻译** | 框选屏幕任意区域进行 OCR 识别并翻译 |

> 快捷键可在设置界面自定义修改，热键冲突时支持优雅处理。

---

## 🛠️ 本地构建与运行

### 运行环境要求
- **操作系统**：Windows 10 / 11 (x64)
- **编译工具**：Go 1.27.3+
- **构建环境**：PowerShell 5.1+

### 构建步骤

```powershell
# 克隆仓库
git clone https://github.com/thegreatjx/fastDictionary.git
cd fastDictionary

# 拉取并校验 OCR 运行时（onnxruntime.dll + indra-models，固定版本 + SHA256）
# 不跑这一步也能编译，但截图识别会回落到 Windows OCR
.\scripts\fetch-ocr-deps.ps1

# 编译正式版（无控制台黑窗口，生成 fastDictionary.exe）
.\build.ps1

# 编译开发调试版（保留控制台日志输出，生成 fastDictionary-dev.exe）
.\build.ps1 -Dev

# 出包：dist 下的绿色目录、zip 与 SHA256
# 加 -Sign 并用 FD_SIGN_CERT / FD_SIGN_PASSWORD 指定证书时自动签名
.\build.ps1 -Package

# 启动界面场景预览（供微调排版与动效）
.\dev.ps1 -Scene grammar
```

> **依赖校验**：`build.ps1 -Package` 会按 `scripts/ocr-deps.json` 校验模型和 DLL；缺失或哈希不符时直接失败，避免发出没有本地 OCR 的包。

> **命名约定说明**：正式产物名称必须固定为 `fastDictionary.exe`。底层引擎根据可执行文件名称自动推导用户配置与词库目录（`%APPDATA%\fastDictionary\`），并维护单实例互斥锁。

---

## 📂 数据与存储路径

数据均存放于用户标准 AppData 目录下，绿色纯净，卸载或迁移只需备份该目录：

```text
%APPDATA%\fastDictionary\
├── config.json          # 用户全局偏好设置与加密后的 API Key
├── vocabulary.json      # 生词本（生词 / 例句与间隔重复复习计划）
├── notebook.json        # 句法笔记（长难句结构树与写作批注完整归档）
├── logs\                # 运行日志（fastDictionary.log；启动时超过 2 MB 轮转为 .1）
└── notebook.html        # 「网页版」生词本导出
```

本地查询缓存保存在缓存目录（`%LOCALAPPDATA%\fastDictionary\query_cache.json`），可在「设置 → AI 指令 → 查询缓存」查看条数或一键清空。

日志默认只记录字符数，不记录选中的原文与 OCR 结果；排查取词问题可在
「设置 → 关于 → 诊断日志」临时打开，也可以在那里打开日志目录。

---

## 📑 深入文档

项目内部设计与测试基准细节请参阅 `docs/` 目录：

- [docs/README.md](docs/README.md) — 完整文档索引
- [CHANGELOG.md](CHANGELOG.md) — 版本更新记录
- [NOTICE](NOTICE) — 第三方组件与模型许可证
- [docs/DESIGN.md](docs/DESIGN.md) — 视觉系统与微交互设计原则
- [docs/HANDOFF_INDRA.md](docs/HANDOFF_INDRA.md) — Indra OCR 算法适配与本地推理落地方案
- [docs/ocr.md](docs/ocr.md) — 各大 OCR 方案性能与识别率实测数据对比

---

## 📄 开源许可证

本项目基于 [MIT License](LICENSE) 开源。随包分发的 ONNX Runtime 与 Indra OCR
模型另有许可证（模型为 OpenRAIL），详见 [NOTICE](NOTICE)。
