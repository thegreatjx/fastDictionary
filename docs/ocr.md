# OCR 引擎选型与实测

记录为什么用 Indra OCR，以及它和另外两套方案的实际差距。
接入过程见 [HANDOFF_INDRA.md](HANDOFF_INDRA.md)。

## 结论

用 **Indra OCR**（`datalab-to/indra-ocr`），纯 Go 接入 onnxruntime。
常规截图 33–35ms，大图 460–640ms，明显优于原来的 Windows.Media.Ocr。

## 三方对比

测试图来自 `mattmahoney.net/ocr/`，那套图是专门设计来难倒 OCR 的：
艺术字、描边、斜纹背景、密集表格。不是日常截图，但能拉开差距。

### 耗时

| 引擎 | plaid（艺术字） | stock（密集表格） | 真实截图尺寸 |
|---|---|---|---|
| Windows.Media.Ocr | 580ms | — | 580ms |
| PP-OCRv6 small（RapidOCR） | 1507–2159ms | — | — |
| **Indra OCR** | **197ms** | **669ms** | **64–123ms** |

### 识别质量

Plaid 海报，正确答案是 `FOR THE`、`January 6-29`、`Fridays & Saturdays at 8 pm`、
`$20 @ $22`、`723-8698`、`Music Theatre International, NY`：

| 引擎 | 实际输出 |
|---|---|
| Windows.Media.Ocr | `C E N T £ R F 0 R T H £`、`Fridays U Saturdays`、`$ 0 s22`、`Music Theatre № tern 0 al` |
| PP-OCRv6 small | 艺术字整体输出 `��`（字符集不覆盖） |
| **Indra** | **全部正确** |

简化版（`crop_line.png`，三行印刷体）：

```
Windows.Media.Ocr: January 6 · 29 Fri days Satu rdays at 8 Ptn Sunday Matinees at 苤皿
Indra            : January 6-29
                   Fridays & Saturdays at 8 pm
                   Sunday Matinees at 2 pm
```

`&`、`-`、数字全部正确，旧版在这些位置全部出错。

## 为什么不选另外两个

**Windows.Media.Ocr**：系统自带，零体积，但艺术字和密集排版全崩。
它是原来的方案，用户反馈「识别错」主要来自这里。

**PP-OCRv6 small**：正常排版极强（`Nasdaq & AMEX`、`Stocks in bold rose or fell 5% or more` 全对），
但慢 3–7 倍，且字符集不覆盖艺术字，直接输出乱码方块。

**Indra** 胜在三点：准确率高、快一个数量级、有独立中文模型 `rec_cjk.onnx`。

## 代价

| 项目 | 体积 |
|---|---|
| `onnxruntime.dll` | 16MB |
| `det.onnx` | 390KB |
| `rec_cjk.onnx` | 9.7MB |
| `rec.onnx` | 4.8MB |
| `script.onnx` | 187KB |
| **合计** | **约 31MB** |

加上 exe 本身 20MB，完整包约 67MB。`build.ps1` 自动复制这些文件。

## 风险

- **发布仅 12 天**（接入时），未经长期验证。
- 权重是修改版 **OpenRAIL-M** 许可：营收低于 $20M 免费，超过需授权。
- **中文识别质量未做专项测试**。所有基准都是英文图，中文是用户主场景但缺少对照数据。
  这一点在 [HANDOFF_INDRA.md](HANDOFF_INDRA.md) 里也标了。

## 已知问题

- 手写体仍会错：`Forever` 认成 `Toneuer`。这是所有 OCR 的难点。
- 密集表格会出现字符重复：`Fridaays`、`Sttocks`、`Nasddaq`。检测框偏大导致。
- `script.onnx` 首次加载约 1.4 秒，之后毫秒级。
- 深底浅字、灰底灰字、等亮度彩色字和贴边裁剪会把首字切成独立框。

## 本地 P0 预处理

`ocr_preprocess.go` 只处理截图与整页文档不同的地方：

1. `adaptivePadForOCR` 用边缘像素向外延展，避免固定白边制造人工黑白交界线；
2. `RecognizeImageFileIn` 在结果可疑且像素条件命中时，才尝试 `colorDistanceForOCR`
   或 `backgroundSubtractForOCR`，并用 `scoreOCRText` 选更完整的候选；
3. 不做分位数对比度拉伸。受控样本里，拉伸会让灰底灰字更碎，还会把
   `efficient` 读成 `effficient`；
4. `resources/ocr.ps1` 先查 `AvailableRecognizerLanguages`。指定的 OCR 语言包
   不存在时直接报错，不再静默回退到用户语言列表。

## 回落

模型或 DLL 缺失、Indra 结果为空或可疑时，`RecognizeImageFileIn` 回落到
`windowsRecognizeFileIn`（PowerShell + Windows.Media.Ocr）。语言包缺失会明确报错；
其他情况下优先返回评分更高的候选。Indra 不可用的回落有测试保证
（`TestIndraFallsBackWhenUnavailable`）。

## 复现基准

测试图和脚本在 `docs/ocr-bench/`：

```
plaid_c150.jpg  plaid_bw200.jpg  stock_gs200.jpg  numbers_gs150.jpg
crop_line.png   crop_block.png   crop_wide.png
run4.py  timing.py  crop_time.py  rapid.py
CASES.md  generate_cases.py  cases.json
```

`CASES.md` 定义深底 UI、贴边小字、艺术字、日文横排/竖排、低对比、双色底和
无字负样本；`generate_cases.py` 生成 PNG 与清单，图片不提交到仓库。
指标见 `CASES.md`：ExactMatchRate、CER/WER、P50/P95、可疑触发率、用户改字率。

Python 环境需装 `Pillow`；跑旧脚本还需 `indra-ocr`、`rapidocr`、`onnxruntime`。
