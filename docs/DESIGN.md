# fast词典 · 晴空微蓝设计系统

> 桌面原生速度 × 清新通透阅读体验。本文是实现契约，颜色以 `theme.go` 的
> `newAppTheme`、`accentChoices` 与 `grammarInkFor` 为准。

## 1. 设计原则

1. **冷白通透。** 浅色底是极淡冷云白，卡片是纯白；深色是微冷深海夜色，不用死黑。
2. **一套主题色贯穿表面。** 切换主题时，主色、原文区淡底、选中胶囊、焦点边框一起变化，四套氛围一眼可分。
3. **层级靠字阶、留白和淡色微底。** 不用彩色渐变、发光和大面积高饱和色块。
4. **成功只做瞬时微反馈。** 不弹绿色横幅、不加绿色底；复制/保存成功只在 2 秒内把图标换成青绿小勾。
5. **原生优先。** 所有界面用 `github.com/egoist/mygo/ui` 的 Go API 绘制，禁止 WebView、HTML 和 CSS。

## 2. 基础表面与文字阶梯

### 浅色 Light

| 语义 | Token | 值 | 用途 |
| --- | --- | --- | --- |
| 主窗口底 | `BgCanvas` | `#F7F9FC` | 窗口、侧栏底色 |
| 卡片/浮层 | `BgSurface` | `#FFFFFF` | 卡片、输入框、浮层 |
| 次级柔和区 | `BgSubtle` | 跟随当前主题 `AccentSoftBg` | 原文条、选中标签、提示卡 |
| 悬停底 | `BgHover` | `#F1F5F9` | 悬停行、分段轨道 |
| 按下底 | `SurfacePress` | `#E2E8F0` | 按下态 |
| 分割线/轻描边 | `BorderSubtle` | `#E5EAF2` | 卡片边界、分隔线 |
| 控件描边 | `BorderCtrl` | `#D3DCE8` | 输入框、次级按钮 |
| 悬停描边 | `BorderHover` | `#B8C6D9` | 控件悬停 |
| 主文字 | `TextPrimary` | `#0F172A` | 正文、标题、主干 |
| 次要文字 | `TextSecondary` | `#475569` | 说明、标签 |
| 弱提示/角标 | `TextMuted` | `#5A6B82` | 辅助信息、角标 |

### 深色 Dark

| 语义 | Token | 值 | 用途 |
| --- | --- | --- | --- |
| 主窗口底 | `BgCanvas` | `#111622` | 窗口、侧栏底色 |
| 卡片/浮层 | `BgSurface` | `#192030` | 卡片、输入框、浮层 |
| 次级柔和区 | `BgSubtle` | 跟随当前主题 `AccentSoftBg` | 原文条、选中标签、提示卡 |
| 悬停底 | `SurfaceHover` | `#1E2738` | 悬停行、分段轨道 |
| 按下底 | `SurfacePress` | `#253047` | 按下态 |
| 分割线/轻描边 | `BorderSubtle` | `#263147` | 卡片边界、分隔线 |
| 控件描边 | `BorderCtrl` | `#33415C` | 输入框、次级按钮 |
| 悬停描边 | `BorderHover` | `#4A5C7A` | 控件悬停 |
| 主文字 | `TextPrimary` | `#F1F5F9` | 正文、标题、主干 |
| 次要文字 | `TextSecondary` | `#A8B8CE` | 说明、标签 |
| 弱提示/角标 | `TextMuted` | `#8494AC` | 辅助信息、修饰语 |

**规则：**

- 原文区、提示卡、选中胶囊使用当前主题的 `AccentSoftBg`；长难句阅读纸改用中性 `SurfaceHover`，让从句的淡色块单独承担结构信息，不再叠一层主题微底。
- 卡片靠纯白/深蓝表面 + 发丝边托起，不加厚重投影。

## 3. 四套清新主题色

设置 → 通用 → 主题色。切换主题会同时改变主色、原文区微底、选中胶囊和焦点边框。

| 主题名 | Value | 浅色 Accent | 浅色 Hover | 浅色 Press | 浅色 AccentSoftBg | 浅色 SoftBorder | 深色 Accent | 深色 AccentSoftBg | 深色 SoftBorder |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **晴空蓝（默认）** | `sky` | `#2563EB` | `#1D4ED8` | `#1E40AF` | `#EFF6FF` | `#BFDBFE` | `#60A5FA` | `#1B2A4A` | `#35558A` |
| **薄荷青** | `mint` | `#0F766E` | `#115E59` | `#134E4A` | `#F0FDFA` | `#99F6E4` | `#2DD4BF` | `#133332` | `#1F5A54` |
| **薰衣紫** | `violet` | `#7C3AED` | `#6D28D9` | `#5B21B6` | `#F5F3FF` | `#DDD6FE` | `#A78BFA` | `#262044` | `#453672` |
| **暖珊瑚** | `coral` | `#C2410C` | `#9A3412` | `#7C2D12` | `#FFF7ED` | `#FED7AA` | `#FB923C` | `#36211A` | `#6B3B22` |

规则：

- **不再跟随 Windows 系统强调色。** 系统色可能是荧光绿或高饱和蓝，会破坏整套冷白表面。
- 深色主题的主按钮使用亮色 Accent + 深色前景（`#0B1220`），保证按钮文字对比度。
- 未配置或值未知时回落到 `sky`；旧配置值 `slate`/`indigo`/`graphite` 归入 `sky`，`plum` 归入 `violet`，不会出现零色。
- 选中态使用 `AccentSoftBg` + `SoftBorder` + `Accent` 文字，不再引入第二种色相。
- 主题色样本是 28px 圆点；选中态用 2px `TextSecondary` 圆环 + 对勾（浅色主题用白勾，深色主题用深色勾），未选中用 1px `BorderCtrl`，不用近黑粗环。

## 4. 语义色与微反馈

| 语义 | 浅色 | 深色 | 使用范围 |
| --- | --- | --- | --- |
| Success 青绿 | `#10B981` | `#10B981` | 仅复制/保存成功后的瞬时小勾，2 秒后恢复 |
| Danger 砖红 | `#B42318` | `#F97066` | 保存失败、翻译失败、删除确认；只染文字与图标 |
| Warning 琥珀 | `#B45309` | `#FBBF24` | 仅真正的警告：快捷键冲突、模型不可用 |

**语义规则：**

- 「语法难点」不是错误或警告：用中性卡面 + 主题色圆点，不占 `Warning`。
- 删除/改写标记把 `Danger` 混入 `TextSecondary`（浅色 28% / 深色 18%），保留红色语义但降低刺激；新增标记用 `AccentPress`。

**成功态规则：**

- 不弹顶部绿色通知横幅，不加绿色底块。
- 复制成功：复制图标在 2 秒内换成青绿小勾，提示文案变为「译文已复制」，随后恢复。
- 保存成功：卡片底部显示中性「✓ 已保存」，不重复弹通知；失败才显示一行陶红原因。
- 写错、删除等修正标记用陶红，但只是文字与图标，不铺整块红底。

## 5. 字体

- UI 字体栈：`Segoe UI Variable Text, Segoe UI, Microsoft YaHei UI`
- 音标：`Segoe UI, Lucida Sans Unicode, Arial Unicode MS`
- 等宽：`Cascadia Mono, Consolas`
- 字阶：
  - Page Title `20px` / 600
  - Word Head `18px` / 700
  - Section `14px` / 600
  - Body / Control `13px` / 400–500
  - Helper `12px` / 400
  - Micro / Badge `11px` / 500–600
- 正文行高 1.5–1.6；长文最大阅读宽度 600 DIP。
- 字重最多 600（词头 700），不用 800/900 制造层级。

## 6. 间距与圆角

- 间距模数：4 / 8 / 12 / 16 / 24 / 32 DIP。
- 圆角：
  - 徽章 `4px`
  - 控件 `6px`
  - 卡片 / 分组 `8px`
  - 窗口 `10px`
  - 悬浮胶囊为完整圆弧，高度 36 DIP
- 控件高度：紧凑 28 / 常规 32 / 搜索 34 DIP。
- 主布局不使用 5、10、14 这类散值；图标光学修正允许 1–2 DIP。

## 7. 组件契约

### 按钮

| 类型 | 静止 | 悬停 | 按下 |
| --- | --- | --- | --- |
| 主操作（保存/翻译） | `Accent` + `AccentText` | `AccentHover` | `AccentPress` |
| 柔和推荐（快捷键预设） | `AccentSoft` + `AccentRing` + `AccentPress` | `Accent` 18% 底 + `Accent` 边 | `AccentRing` |
| 次要 | Surface + BorderCtrl | BgHover + BorderHover | SurfacePress |
| 禁用 | SurfaceHover + BorderSubtle | 不变 | `TextDisabled`，关闭自动降透明度 |
| Ghost 图标 | TextSecondary，无边 | BgHover | SurfacePress |

- 主操作使用当前主题色，不再使用近黑墨块。
- 柔和推荐按钮只用于「推荐但非唯一主操作」的长预设，避免整行实心色块像横幅。
- 保存按钮的前景色与图标同色；深色主题下为亮色底 + 深色字。

### 图标按钮

- 尺寸 28×28，图标 16 或 14。
- 静止 `TextSecondary`，悬停 `BgHover`，按下 `SurfacePress`。
- 选中态用 `AccentSoftBg` 底 + `Selected(true)` 语义，补键盘焦点环。
- 复制成功的小勾用 `#10B981`，2 秒后恢复。
- 每个纯图标按钮必须有 tooltip 和可访问名称，不允许只靠颜色表达状态。

### 分段控制器

- 轨道 `BgHover` + `BorderSubtle`，选中滑块 `AccentSoftBg` + `SoftBorder`。
- 选中文字 `Accent` / 600，未选中 `TextSecondary` / 500。
- 只在「翻译 / 长难句 / 写作」和设置页三档排版中使用，不堆叠第二套 Tab。

### 徽章

- CEFR：`BgSubtle` 底，靠墨阶区分难度。
  - C：`TextPrimary`
  - B：`TextSecondary`
  - A：`TextMuted`
- 词性：中性描边胶囊，文字 `TextSecondary`。
- 不使用绿/蓝/紫三色难度徽章。

### 原文区与次级区

- 底色必须是当前主题的 `AccentSoftBg`，边界用 `SoftBorder`。
- 原文与译文靠这一层淡色氛围底区分，不额外加粗边框。
- 提示卡、备忘卡复用同一层微底，不另造颜色。

### 长难句标注

长难句采用纯粹的「手绘矢量线型 + 行间微标签 + 描边微标与手绘圆圈/括号 + 同排修饰弯箭头」体系（`grammar_marks.go`），正文不铺从句/短语/中心词背景色块，避免划线与底色块双重叠加造成视觉杂乱：

- **五类手绘矢量线型（`drawMark`，Quadratic Bézier + 确定性伪随机抖动）**：
  - **主语**（`RoleSubject`）：手绘单铅笔线（`markUnderline`），主句加粗 `TextPrimary` + 全实度墨色，从句内部主语降为 55% 透明度。
  - **谓语**（`RolePredicate`）：手绘波浪线（`markWavy`）+ 当前主题 `Accent` 加粗。
  - **宾语/表语**（`RoleObject`）：手绘双横线（`markDouble`，上粗下细双轨铅笔线）。
  - **非谓语短语**（`分词` / `不定式` / `动名`）：手绘圆点虚线（`markDotted`），配暖琥珀色族（遇 `coral` 主题自动切换橄榄松绿）。
  - **介词短语与状语**（`介短` / 状语修饰）：手绘长短破折虚线（`markDashed`），配浅青鼠尾草色族（遇 `mint` 主题自动切换靛青蓝）。
  - **悬停聚光灯**：悬停词块、从句或短语时，非关联词段自动降为 `0.34` 透明度突出当前结构，不叠加荧光底色块。
- **四类手绘装饰符号**：
  - **连接词手绘椭圆圈**（`drawConnectorOval`）：`RoleConnector`（`that` / `which` / `if` / `while` 等）外围绘制双圈闭合手绘绿椭圆（遇 `mint` 主题自动让位给宝石蓝 `#2A57C4` / `#86A6FF`）。
  - **短语手绘弯括号**（`drawCurvedBracket`）：短语首尾绘制上下微收、中段外拱的手绘弯括号 `(` `)`。
  - **句法难点手绘五角星**（`drawHandStar`）：难点片段下方微标签左侧绘制手绘红色五角星与难点短标签。
  - **断句手绘斜线**（`drawHandSlash`）：开启「断句」时，意群边界绘单斜线 `/`，从句边界绘双斜线 `//`。
- **描边微标、行间微标签与同排修饰弧线**：
  - **纯描边前缀微标**：从句与短语首部仅保留 `Surface` 底 + `1px` 描边的轻量胶囊微标（`定从` / `宾从` / `状从` / `介短` / `分词`），词句本身不铺任何彩色背景块。
  - **行间微标签（Sub-line Micro-Tags）**：通过 `LineHeight(2.68)` 留出行间通道，在手绘线正下方绘制 `9.2px` 的成分微标签（`主语`、`谓语`、`宾语`、`引导词`、`从句主语`、`从句谓语`、`后置定语`、`起点状语`、`补足语` 等），并带同行水平碰撞避让。
  - **同排修饰弯箭头（`drawModifierArrow`）**：从句或后置短语修饰同一行的先行词/中心词（`Target`）时，在行顶空白区绘制三次贝塞尔弧线箭头指向被修饰词。
- **结构树与控制栏**：控制栏左侧使用 `SegmentedControlCompact` 在「括号标注」与「结构树」间切换，右侧配「只看主干」「断句」开关；下方图例条同步绘制与正文完全一致的手绘单线、波浪线、双横线、从句胶囊、破折线、圆点线与手绘括号。
- **异步加载反馈（按耗时分级）**：仅长耗时的 AI 场景（长难句分析与预填等待、写作批改、AI 词典、AI 精翻及设置页联网/词库任务）使用主题色 `ui.Spinner`（与按钮内 `LoadingSecondaryButton`），不使用不定长进度条；预填分析未完成时右上角显示 `Spinner` +「句法分析中…」，隐藏可点击保存。普通机翻查询与截图 OCR 只保留轻量状态文案，不显示转圈或进度条。
- **阅读纸与保存**：中性 `SurfaceHover` + `BorderSubtle`；提示条中性描边，句首保留主题色圆点；分析完成后右上角显示「存入句法笔记 / 已存入句法笔记」，写入独立 `notebook.json`。
- **句式难点**：中性卡面（`SurfaceHover` + `BorderSubtle`）。标题「语法难点」用 `TextSecondary` + info 图标；每条难点前缀一个 4px 主题色圆点，标题胶囊白底中性描边，说明文字用 `TextSecondary`（11px 在暗色下不能降到 `TextMuted`）。不使用琥珀、棕色或彩色底。

### 写作卡

- 「改后版本」用 `BgSubtle` 微底卡，不用浅绿整块。
- 删除文字用 `Danger` 混入 `TextSecondary` 的柔红（浅色 28% / 深色 18%），暖珊瑚主题下改用中性 `TextSecondary` 删除线拉开与珊瑚橙新增态的区分；新增用 `AccentPress`（深色为 `Accent`），字重 600；分类标签用中性描边。不要高饱和纯红 + 粗体并排。
- 主按钮使用主题色「存入写作笔记」，成功只显示中性「已存入写作笔记」，不弹横幅。

### 本地缓存（`query_cache.json`）

- 翻译、词典结果、「猜你想存」原始生词列表（在过滤 `DismissedNoteWords` 之前缓存）及 `AI 精翻 / 长难句 / 写作` 结果按统一有界 LRU（每类 64 条）持久化于用户缓存目录，未钉住关闭或重新选中相同文本时直接命中零等待、不重复消耗 Token；点击「重新翻译 / 重新生成」主动绕过缓存。

### 悬浮条

- 底色冷白 `#FFFFFF` / 深海蓝 `#192030`，发丝边 `#E5EAF2` / `#263147`。
- 图标墨色 `#475569` / `#A8B8CE`；唯一强调图标用 `Accent`。
- 悬停只加深墨色并加中性微底，不按按钮换色相。
- 提示气泡深底浅字 / 浅底深字，与悬浮条同族，不用系统 Inverse。
- 窗口矩形与胶囊完全一致，不保留透明边距；胶囊只用 1px 发丝边和底色与页面区分，不加外投影。
- 不设常驻关闭键；胶囊只承载操作按钮。

## 8. 动效

- 阅读面（弹窗、卡片、设置页）只做 120–160ms 的颜色与位置过渡。
- 不使用弹跳、发光、呼吸和自动播放动画。
- 拖动浮窗时避免整棵树重建；`applyTheme` 在色板未变化时直接返回。

## 9. 禁止项

- 大面积浅绿 / 翡翠绿 / 荧光绿成功态、绿色保存横幅、重复成功通知。
- 琥珀色保存按钮、金书签填充、橙色营销 CTA。
- 长难句「语法难点」使用琥珀/棕色标题或胶囊，把语法说明画成警告。
- 整行实心主题色的推荐/预设按钮，读起来像营销横幅。
- 未跟随主题的浅蓝原文条、浅绿提示卡或第二套氛围底色。
- 长难句六色下划线、彩色整块背景、按角色切换的多种线型。
- 紫色渐变、彩色 icon 行、Emoji 结构图标。
- 跟随系统强调色导致色板失控。
- 用 WebView / HTML / CSS 替代原生 Go UI。

## 10. 验证

- 主文字与次要文字 ≥ 4.5:1；图标与大字号 ≥ 3:1；`TextMuted` 只用于弱提示与角标。
- 语义角色用 `go test -run 'TestGrammarPaletteContrastAcrossThemes|TestGrammarClauseWashHierarchy|TestGrammarDifficultyStaysNeutral'` 覆盖四套强调色 × 明暗。
- 浅色与深色都要实机截图检查：词卡、长难句卡、写作卡、设置页、悬浮条。
- 四套主题各切换一次，确认原文区微底、选中胶囊与焦点边框同步变化。
- 复制成功后小勾 2 秒内为 `#10B981`，随后恢复；不出现顶部绿色横幅。
- `gofmt`、`go vet ./...`、`go build ./...`、`go test ./...` 全部通过。
- 原生控件树用 `mygo-automation snapshot -i` 检查，视觉用 `screenshot` 检查；`impeccable detect` 不覆盖 Go 原生 UI，不能替代截图。
- 统一用 `.\scripts\ui-evidence.ps1` 取证：隔离 `%APPDATA%\fastDictionary-uiverify`，输出 `manifest.json`（commit/exe SHA256/配置/主题）、`screenshot.png` 与 `control-tree.txt`。身份对不上的截图不算验证；流程见 `.claude/skills/fastdictionary-ui-verify/SKILL.md`。
