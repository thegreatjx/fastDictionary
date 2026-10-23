# fast词典 · Impeccable Design System (DESIGN.md)

> 设计语言规范 · 桌面原生速度 × 考究学习体验

## 1. 品牌基调与产品哲学 (Brand & Personality)
- **定位**：极速响应、原生轻量的桌面伴读词典与翻译工具（媲美 PopClip × 欧陆词典 × Pot）。
- **特质**：
  - **Quiet Craft (克制考究)**：杜绝 AI Slop（花哨紫蓝渐变、千篇一律的卡片套卡片、过饱和阴影）。
  - **Native Precision (原生精准)**：极速唤醒（<50ms）、0 WebView 开销、像素级硬件裁剪。
  - **Learner First (学习者优先)**：单词必带标准 IPA 音标、CEFR 分级、语境原句与词根助记。

## 2. 色彩系统 (Color Tokens)
- **底色与基面**：
  - Light Surface: `#FFFFFF` · Background: `#F8FAFC`
  - Dark Surface: `#1E293B` · Background: `#0F172A`
  - Border Hairline: `#E2E8F0` (Light) / `#334155` (Dark)
- **语义色彩**：
  - **Accent (皇家蓝)**: `#2563EB` (交互焦点、主要按钮、外框高亮)
  - **Success (翡翠绿)**: `#10B981` / Light: `#ECFDF5` (保存成功、A级难度徽章)
  - **Bookmark (金书签)**: `#EAB308` / Light: `#FEFCE8` (生词库星标与收藏)
  - **Difficulty CEFR**:
    - A1/A2: 翡翠绿 (`#059669` on `#ECFDF5`)
    - B1/B2: 皇家蓝 (`#2563EB` on `#EFF6FF`)
    - C1/C2: 典雅紫 (`#7E22CE` on `#FAF5FF`)
  - **Danger (警戒红)**: `#DC2626` / Light: `#FEE2E2` (退出、删除)

## 3. 排版系统 (Typography Scale)
- **字体族**：系统最优无衬线体 (`-apple-system, Segoe UI, PingFang SC, Microsoft YaHei`)
- **阶梯**：
  - Title: `18px~20px` · Weight: 700 (词条大字、弹窗标题)
  - Headline: `15px~16px` · Weight: 600 (译文主干、核心定义)
  - Body: `13px~14px` · Weight: 400 · LineHeight: 1.6 (例句、释义正文)
  - Caption / Tag: `11px~12px` · Weight: 600 (音标、词性胶囊、时间戳)
  - Micro / Badge: `10px` · Weight: 700 (CEFR 等级)

## 4. 间距与圆角 (Spacing & Geometry)
- **间距模数**：4px / 8px / 12px / 16px / 20px
- **圆角**：
  - 微胶囊 (Badges): `4px~5px`
  - 卡片与输入框 (Cards): `8px`
  - 窗口主体 (Windows): `10px~12px`
  - 悬浮胶囊工具条 (Floating Pill): `17px~18px` (完全胶囊圆弧)

## 5. 核心界面设计契约 (Surfaces)
1. **悬浮条 (Selection Chip)**:
   - 纯净白/深灰胶囊，高度 36 DIP，硬件级 `CombineRgn` 裁切，0 黑边 0 杂色；
   - 矢量小图标等间距分布，鼠标悬停时顶部气泡精确定位。
2. **查词主弹窗 (Popup)**:
   - 顶部集成一键模式切换器 `[ 翻译 | 📘 词典 | 🔍 语法 | ✏️ 写作 ]`；
   - 单词自动渲染 IPA 发音按钮、词性徽标、CEFR 分级与【⭐ 存入生词库】。
3. **生词库 (Vocabulary Management)**:
   - 支持全量 JSON / Excel CSV 导入导出；
   - 条目富文本呈现，带词源助记与双语例句。
4. **截图翻译 (Screenshot OCR)**:
   - 全局热键 `Ctrl+Alt+S` / 悬浮条快捷图标唤起；
   - 离线 WinRT OCR 识别，极速翻译。
