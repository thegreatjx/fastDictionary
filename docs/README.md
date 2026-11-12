# 文档索引

这个目录放项目过程中产生的**文字材料**：设计、交接、变更记录、调研。
代码在仓库根目录（Go 要求包内所有 `.go` 同目录），这里只有文档和截图。

## 现行文档

| 文件 | 内容 | 什么时候看 |
|---|---|---|
| [DESIGN.md](DESIGN.md) | 视觉与交互设计原则：悬浮条配色、长难句教学配色、气泡行为 | 改界面、改配色、改动效前 |
| [AI_SERVICES.md](AI_SERVICES.md) | AI 精翻接入多家模型服务的规格：预设表、密钥规则、客户端契约、设置页状态机、不变量与守护测试 | 改 AI 精翻、加服务商、排查 Key 或连接问题前 |
| [HANDOFF_INDRA.md](HANDOFF_INDRA.md) | Indra OCR 接入全过程：选型依据、实测数据、算法逆向、踩坑记录 | 改 OCR、换模型、排查识别质量前 |
| [CHANGES_2026-10-18.md](CHANGES_2026-10-18.md) | 那一轮改动的详细清单 | 追溯某次改动的动机时 |
| [ocr.md](ocr.md) | OCR 引擎选型对比与实测数据 | 换 OCR 方案前 |
| [../CHANGELOG.md](../CHANGELOG.md) | 版本更新记录（新功能、修复、发布方式） | 回答「这个版本改了什么」时 |

## 可复现基准

`ocr-bench/` 是 OCR 测试图和脚本，文档里所有实测数字都用它跑出来。
图源 `mattmahoney.net/ocr`，那套图专门设计来难倒 OCR：艺术字、描边、斜纹背景、密集表格。

```
plaid_c150.jpg  stock_gs200.jpg  numbers_gs150.jpg  crop_line.png
run4.py  timing.py  rapid.py
```

跑之前装 `indra-ocr`、`rapidocr`、`onnxruntime`（Python 3.12）。

## 归档

`archive/` 放已经过时的材料。不删是因为它们记录了**为什么没选另一条路**，
但不要照着做——现行做法以根目录代码和上面三份文档为准。

| 文件 | 内容 | 为什么归档 |
|---|---|---|
| [archive/HANDOFF.md](archive/HANDOFF.md) | 最早一轮交接（划词小图标） | 功能已完成，被后续设计取代 |
| [archive/SPEC.md](archive/SPEC.md) | 产品规格 v1 | 大量条目已实现或改向，与现状不符 |
| [archive/HANDOFF_AI_ACTIONS.md](archive/HANDOFF_AI_ACTIONS.md) | AI 指令复刻规范 | 已落地，细节以代码为准 |
| [archive/重构方案.md](archive/重构方案.md) | 基于 pot-desktop 的初版方案 | 实施路径已走完 |
| `archive/screenshots/` | 早期开发截图（2026-10-11 前后） | 界面已多次改版，仅作历史参考 |

## 截图

`screenshots/` 放**当前版本**的界面截图。命名一律用「主题-变体」：

| 文件 | 内容 |
|---|---|
| `chip.png` | 悬浮条（划词后的小工具条） |
| `chip-tip.png` | 悬停气泡，浅色 |
| `chip-tip-dark.png` | 悬停气泡，深色 |
| `translate.png` | 翻译面板 |
| `word.png` | 词典面板（460px 窄宽） |
| `grammar.png` | 长难句语法标记 |
| `grammar-zoom.png` | 长难句局部放大，看标记细节 |
| `writing.png` | 写作改进 |
| `ai-refine.png` | AI 精翻按钮 |
| `settings.png` | 设置页 |

界面改版后旧截图不要就地覆盖，移到 `archive/screenshots/`：
截图是**当时的证据**，覆盖掉就看不出改了什么。

重新生成用 `dev.ps1`，它构建 `build/dev-preview.exe`，不碰正式版：

```powershell
.\dev.ps1 -Scene grammar -Shot build/grammar.png
.\dev.ps1 -Scene chip-tip -Shot build/tip.png
```

场景名：`grammar` `dict` `writing` `chip` `chip-tip` `settings` `ocr`。
加 `-Zoom 3` 放大，`-CropFrom 100,200 -CropTo 400,300` 裁切。
