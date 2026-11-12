# 自定义 AI 指令与阅读蛙 1:1 复刻落地规范文档 (HANDOFF_AI_ACTIONS.md)

> 更新时间：2026-10-11 · 本项目：D:\AI_workspace\translate\fastDictionary
> 编译产物：Dictionary.exe（正式版，无控制台，常驻托盘，内存 ~20MB）

---

## 一、当前已攻克的硬核成果

1. **Win32 窗口底层三大坑已彻底铲平**：
   - 彻底解决上一代模型卡壳的 `(0,0)` 坐标锁定问题（根因是 `DisableMove: true` 在消息循环中强加 `swpNoMove`，现已修复为 `false`）；
   - 彻底消除了悬浮窗方形黑边/杂框（利用 Win32 `CreateRoundRectRgn` + `SetWindowRgn` 进行硬件层圆角裁切）；
   - 解决了悬浮条按钮截断问题（全部改为纯矢量小方形图标 28x26px，总宽度 180px，完整展示）。
2. **顶栏与悬浮窗 1:1 对标阅读蛙 & 谷歌扁平设计**：
   - 顶栏：`[中/A Translation]  ::: (居中拖拽把手)  [简体中文 ▾]  [📌 固定钉]  [✕ 关闭]`；
   - 窗口可随把手在屏幕任意拖动（`DragWindow()`）；
   - 支持全局透明度（默认 95%，设置中 60%~100% 可调）。
3. **双重发音系统打通**：
   - 单词走有道真人 mp3，长句走 Windows 原生 SAPI TTS 兜底。
4. **托盘与图标**：
   - 已制作并嵌入高质感【📘 深蓝词典（带金色书签折角）】图标。

---

## 二、本次核心开发目标：自定义 AI 指令系统（词典 / 长难句 / 写作改进）

用户要求：**词典、长难句分析、写作改进不是写死的代码，而是像阅读蛙（options.html#/custom-actions）一样，作为独立的【自定义 AI 指令】存在，有独立的开关、独立 Prompt、输出 Schema 和专属渲染版式，用户可开关、可自由修改模型与提示词。**

### 1. 指令 1：词典 (Learner's Dictionary)
- **ID**: `default-dictionary`
- **图标**: 📘 蓝色书本 (`iconDict`)
- **System Prompt**:
  ```
  你是一个面向语言学习者的词典助手。
  根据给定的词语及其周围段落，生成简洁的词典条目，匹配所需的输出对象。
  规则：
  1. 聚焦于最匹配所提供段落内容的含义。
  2. 将词条规范化为其基本/标准形式。
  3. 保持释义精确且适合学习者。
  4. “句子”字段只提取所提供段落中包含选中词语的完整句子，不要包含前后句。
  5. “句子翻译”字段只翻译所提取的句子。
  6. 音标必须使用该语言的标准标注（如英语用 IPA，中文用拼音，日语用罗马字）。
  7. 词性使用英文（noun, verb, adjective 等）。
  8. 难度必须是 CEFR 等级（A1, A2, B1, B2, C1, 或 C2）。
  9. 如果某个字段未知，返回空字符串而非猜测。
  10. 除非需要保留原文形式，所有文本字段均使用 {{targetLanguage}} 作答。
  11. “句中词形”字段标出选中词语在“句子”中的位置：紧凑的 JSON 数组 [{"text": ...}]。
  12. “记忆技巧”针对词条特点教学习者记住它（最多 3 条，每条一行）：词根词缀、拆字、词源、近反义词、易混词、搭配。严禁谐音或硬凑。
  ```
- **Output Schema**:
  - `term` (词条)
  - `phonetic` (音标)
  - `partOfSpeech` (词性)
  - `definition` (释义)
  - `context` (句子)
  - `contextTerm` (句中词形)
  - `contextTranslation` (句子翻译)
  - `memoryTips` (记忆技巧)
  - `difficulty` (难度)
- **专属版式 (Layout)**:
  - 词条大字 + 音标 + CEFR 难度徽章；
  - 词性标签 + 中文释义；
  - 原句展示（高亮匹配的词形）+ 原句译文；
  - 记忆技巧卡片；
  - 右下角【保存到笔记本】金色按钮。

### 2. 指令 2：长难句分析 (Sentence Analysis)
- **ID**: `default-sentence-analysis`
- **图标**: 🔍 带四个直角边框的放大镜 (`iconGrammar`)
- **System Prompt**:
  ```
  You are a grammar tutor. Mark up the Selection (a sentence in any language) so a learner sees its structure at a glance: the core of the main clause, every clause and longer phrase, and whatever makes the sentence hard to read.
  Each annotation quotes one span:
  - "text": exact span from Selection
  - "type": subject, predicate, object, complement, attributive, adverbial, appositive, connector
  - "form": clause, infinitive, gerund, present-participle, past-participle
  - "sense": time, place, cause, condition, concession, purpose, result, manner, comparison
  - "obstacle": inversion, fronting, ellipsis, passive, subjunctive, parenthesis, dummy-it, postponed-subject, cleft, split
  - "restore": plain word order
  Output format: {"annotations": [...], "translation": "..."}
  ```
- **Output Schema**:
  - `annotations` (标注切片列表)
  - `translation` (自然流畅中文译文)
- **专属版式 (Layout)**:
  - 顶部“○ 思考完成”胶囊；
  - 原句富文本：
    - 主语：加粗实线下划线；
    - 谓语：深蓝加粗文字 + `WavyUnderline`（蓝色波浪线）；
    - 宾语：双实线下划线；
    - 从句：彩色微型胶囊微标（`[表语从句]`、`[结果状从]`）；
    - 连词：绿色微胶囊圆圈 `(like)`、`(so)`、`(but)`；
    - 被动语态：标注红色星号 `★` 与 `被动` 小标；
  - 控制条：`[ 只看主干 ]` 切换按钮 + `[ 断句 ]` + 图例（主语、谓语、宾语）；
  - 下方自然段落展示中文精译；
  - 底栏：`[ 保存到笔记本 ]`。

### 3. 指令 3：写作改进 (Improve Writing)
- **ID**: `default-improve-writing`
- **图标**: ✏️ 批改修饰笔 (`iconPolish`)
- **System Prompt**:
  ```
  You are a writing teacher. Judge the setting of the Selection first, then mark what a native writer would change for that setting, and give the Selection back corrected.
  $1: Setting formality ("Formal · email to professor", "Casual · tweet")
  $2: Annotations ([{"text": ..., "fix": ..., "type": "grammar/word-choice/unnatural...", "note": ...}])
  $3: Corrected version as a native writer would write it
  $4: One-sentence summary critique in target language
  Output format: {"setting": "...", "annotations": [...], "improved": "...", "summary": "..."}
  ```
- **Output Schema**:
  - `setting` (场合)
  - `annotations` (批注列表)
  - `improved` (改后版本)
  - `summary` (总评)
- **专属版式 (Layout)**:
  - 场合标签条；
  - 原文改动对比视图；
  - 改后完整版本（一键复制按钮）；
  - 导师一句话总评。

---

## 三、设置中心中的【自定义 AI 指令】管理页（options 对标）

- 在设置中心增加独立的【AI 指令】导航项；
- 列表展示三大内置指令（词典、长难句分析、写作改进），每项带开关 Toggle；
- 点击【定制化】可自由修改：
  - 指令名称
  - 调用的模型（支持从 Magpie 模型列表自由选择）
  - 系统提示词编辑框
  - 用户提示词编辑框。
- 划词悬浮条根据开启的指令列表动态显示对应图标。

---

## 四、本地生词本（Notebase 笔记库）

- 路径：`%AppData%\Dictionary\vocabulary.json`
- 点击任意卡片底部的【保存到笔记本】时自动追加入库；
- 设置中心提供【生词本】浏览表格（词条、音标、词性、释义、句子、记忆技巧）。
