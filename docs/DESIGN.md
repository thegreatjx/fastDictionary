# fast词典 · Impeccable Design System (DESIGN.md)

> 设计语言规范 · 桌面原生速度 × 考究学习体验

## 1. 品牌基调与产品哲学 (Brand & Personality)
- **定位**：极速响应、原生轻量的桌面伴读词典与翻译工具（媲美 PopClip × 欧陆词典 × Pot）。
- **特质**：
  - **Quiet Craft (克制考究)**：杜绝 AI Slop（花哨紫蓝渐变、千篇一律的卡片套卡片、过饱和阴影）。
  - **Native Precision (原生精准)**：极速唤醒（<50ms）、0 WebView 开销、像素级硬件裁剪。
  - **Learner First (学习者优先)**：单词必带标准 IPA 音标、CEFR 分级、语境原句与词根助记。

## 2. 色彩系统 (Color Tokens)

> 实现以 `theme.go` 的 `newAppTheme` 为准；本节记录语义和禁用项，避免旧版翡翠绿/金书签重新回到界面。

- **底色与基面**：
  - Light Surface: `#FFFFFF` · Background: `#F5F7FA` · Text: `#0F172A`
  - Dark Surface: `#181B21` · Background: `#111317` · Text: `#F1F5F9`
  - Secondary text: `#475569` (Light) / `#94A3B8` (Dark)
  - Muted text: `#5B697A` (Light) / `#8895A7` (Dark)，小字对比度不低于 4.5:1
  - Border: `rgba(15,23,42,.14)` (Light) / `rgba(255,255,255,.14)` (Dark)
- **悬浮条专用冷调色板**（与主界面分开，理由见 §5.1）：
  - 胶囊底：`#F4F8FF` (Light) / `#171D28` (Dark)
  - 发丝边：`#D4E2FA` / `#2B394F`
  - 图标墨色：`#3A4960` / `#B6C5DA`
  - 悬停墨色：`#1D4ED8` / `#E2ECFA`
  - 唯一强调色（仅“划词翻译”）：`#2563EB` / `#60A5FA`
  - 悬停提示气泡：底 `#1E293B` / `#E8EDF5`，字 `#F1F5F9` / `#0F172A`
- **语义色彩**：
  - **Accent**: `#2563EB`（交互焦点、主操作、当前选中）；系统强调色存在时优先级更高。
  - **Success (松绿)**: `#2F6F5E` / `#9BC7B5`，Soft 为 10% 底色。用于保存成功、监听状态点、A 级难度徽章。
  - **Danger (陶红)**: `#A33A34` / `#E39A93`，Soft 为 9% 底色。用于删除、OCR 失败。
  - **Warning (赭石)**: `#8A6435` / `#D6B07A`。只用于暂停和“模型不在列表”等提示。
  - **Save action (墨色主按钮)**: `#1E293B` / `#E2E8F0`，前景 `#FFFFFF` / `#0F172A`。保存按钮不再使用琥珀、金书签或橙色。
  - **难度 CEFR**:
    - A1/A2: 松绿 `#2F6F5E` on 10% tint
    - B1/B2: Accent `#2563EB` on 10% tint
    - C1/C2: 灰紫 `#6B4A8A` / `#C3B2E8`
  - **写作批注**: 原句用陶红 `#9A4C4C` / `#D6A3A0`，改写用松绿 `#2F6F5E` / `#9BC7B5`；分类标签用中性描边，不用整块红/绿。
  - **长难句标注**: 青 `#2F6F7A`、蓝 `#2A57C4`、梅 `#8A4B6B`、紫 `#6F4AA8`、橄榄 `#4C7A3F`、陶土 `#B05A3C`；靠线型（实线/波浪/双线/点线）区分，不只靠颜色。
  - **禁用**：大面积浅绿卡片、荧光绿、琥珀色保存按钮、金书签填充、紫色渐变、饱和红绿并排。

## 3. 排版系统 (Typography Scale)
- **字体族**：Windows 原生无衬线栈 `Segoe UI Variable Text, Segoe UI, Microsoft YaHei UI`；代码/数据用 `Cascadia Mono, Consolas`。
- **阶梯**：
  - Page Title: `20px` · Weight 600
  - Word Head: `18px` · Weight 700
  - Section: `14px` · Weight 600
  - Body / Control: `13px` · Weight 400~500
  - Caption / Helper: `12px` · Weight 400~600
  - Micro / Badge: `10–11px` · Weight 600~700
  - 正文行高 `1.5~1.6`，长文最大阅读宽度约 `600 DIP`。

## 4. 间距与圆角 (Spacing & Geometry)
- **间距模数**：主布局按 4 / 8 / 12 / 16 / 24 / 32 DIP；徽标、图标按钮允许 1–2 DIP 光学修正，不新增大块魔法数。
- **圆角**：
  - 微胶囊 (Badges): `4px`
  - 控件与卡片 (Controls / Cards): `6px`
  - 容器/分组 (Track): `8px`
  - 窗口主体 (Windows): `10px`
  - 悬浮胶囊工具条 (Floating Pill): 完全胶囊圆弧，高度 36 DIP。

## 5. 核心界面设计契约 (Surfaces)
1. **悬浮条 (Selection Chip)**:
   - 冷调低饱和胶囊，高度 36 DIP，硬件级 `CombineRgn` 裁切，0 黑边 0 杂色；
   - **色彩只花一次**：只有“划词翻译”带钢蓝强调色，其余六个图标共用同一冷灰墨色。
     七个高饱和色并排是“低俗”的来源，也让工具栏读起来像七个互不相干的按钮；
     现在靠字形和位置区分，颜色只负责指出主操作；
   - **悬停即显功能名**：指针停在任一按钮上 160ms 后，按钮正上方弹出冷调气泡，写明
     这个按钮做什么（“朗读选中文字”“词典”……）。气泡是悬浮条的 `NewFlyout`
     （`WS_POPUP`），不是内建 Tooltip——内建 Tooltip 浮在窗口外，会被胶囊的窗口
     区域裁掉；而普通窗口受桌面 64px 最小高度限制，22 DIP 的薄条会变成厚板；
   - 沿条移动指针时气泡跟着走，不重新等待（400ms 内为“热”态）；
   - 气泡永不获取键盘焦点（`Focusable: false` + `ShowInactive`）。
2. **查词主弹窗 (Popup)**:
   - 顶部只用三个阅读工具：`[ 翻译 | 长难句 | 写作 ]`，不再放词典/语法 Emoji 页签；
   - 原文可折叠：收起态先合并段落再截断；展开态按段落渲染，段间 8 DIP，不用空行撑高度；
   - 排版三档 `紧凑 / 智能 / 保留换行`，设置页可改，译文下方用段落标记 `¶` 菜单就地切换；
   - 单词自动渲染 IPA 发音、词性胶囊、CEFR 分级；发音与复制使用无边框 ghost 图标按钮；
   - 保存动作统一为墨色主按钮（浅色近黑 / 深色近白），图标、文字同前景色，hover/按下各深一档；已保存状态改为中性描边胶囊。
3. **生词库 (Vocabulary Management)**:
   - 支持全量 JSON / Excel CSV 导入导出；
   - 条目富文本呈现，带词源助记与双语例句。
4. **截图翻译 (Screenshot OCR)**:
   - 全局热键默认 `Alt+C`（设置 → 快捷键可改），也可从托盘或悬浮条唤起；
   - 离线 WinRT OCR 识别，极速翻译。
