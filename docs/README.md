# 文档索引

| 文件 | 内容 |
|---|---|
| [DESIGN.md](DESIGN.md) | 视觉与交互契约：主题、组件、悬浮条、验证 |
| [AI_SERVICES.md](AI_SERVICES.md) | AI 服务接入、密钥、客户端与设置页契约 |
| [HANDOFF_INDRA.md](HANDOFF_INDRA.md) | Indra OCR 选型、接入与踩坑 |
| [ocr.md](ocr.md) | OCR 引擎实测对比 |
| [ENGINEERING_CONTRACT.md](ENGINEERING_CONTRACT.md) | 边界契约与提交前检查 |
| [RETROSPECTIVE_2026-11.md](RETROSPECTIVE_2026-11.md) | 2026-11 返工复盘 |
| [../CHANGELOG.md](../CHANGELOG.md) | 版本记录 |

## 基准与截图

`ocr-bench/` 是 OCR 测试图与脚本，文档里的实测数字都来自这里。

`screenshots/` 是当前版本截图；界面改版后直接替换，不保留旧图。

原生 UI 验证统一用 `scripts/ui-evidence.ps1`，证据必须带 `manifest.json`。
