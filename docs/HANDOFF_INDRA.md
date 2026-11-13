# HANDOFF — Indra OCR 移植（选项 A：纯 Go 完整实现）

> 写入时间：2026-10-25。压缩上下文前保存的项目状态。
> 读这一份即可继续工作，不需要回溯对话。

## 目标

把本项目（module `fastDictionary`）的截图 OCR 从 **Windows.Media.Ocr**（PowerShell 调 WinRT，
580ms，非标准字体识别极差）换成 **Indra OCR**（纯 Go + onnxruntime，目标 60–120ms）。

## 为什么是 Indra

用户实测对比（`docs/ocr-bench/`，图源 `mattmahoney.net/ocr`）：

| 项目 | Windows.Media.Ocr | PP-OCRv6 small | **Indra OCR** |
|---|---|---|---|
| 机构名 `FOR THE` | `C E N T £ R F 0 R T H £` | ✓ | ✓ |
| `Fridays & Saturdays` | `Fridays U Saturdays` | ✓ | ✓ |
| 票价 `$20 @ $22` | `$ 0 s22` | 截断 | ✓ |
| `Music Theatre International, NY` | `Music Theatre № tern 0 al` | 截断 | ✓ |
| 耗时（640×940 整图） | 580ms | 1507ms | **197ms** |
| 耗时（480×140 真截图尺寸） | — | — | **86ms** |

用户痛点就是「非典型字体识别错误」，Indra 是三者中唯一能读出 `& @ - International` 的。
`rec_cjk.onnx`（10.2MB）是**独立中文模型**，比 PP-OCRv6 的「一个模型通吃 50 语言」更适合中英混排。

## 环境事实（已验证）

- 项目：`D:\AI_workspace\translate\fastDictionary`，module `fastDictionary`，Go 1.27.3，mygo v0.9.0
- **`github.com/ebitengine/purego v0.11.1` 已在 go.sum**（间接依赖）
- **`purego.Dlopen` 不支持 Windows**（`dlfcn.go` 的 build tag 只有 darwin/freebsd/linux/netbsd）
  → 必须用 `syscall.NewLazyDLL("kernel32.dll")` 的 `LoadLibraryW` + `GetProcAddress`
- ONNX Runtime DLL 两个版本：
  - `C:\Windows\System32\onnxruntime.dll` = **1.17**（可能太旧，未测）
  - `C:\Users\<user>\AppData\Local\Programs\Python\Python312\Lib\site-packages\onnxruntime\capi\onnxruntime.dll` = **1.25.1**（已验证可用）
  - 正式分发需要把 16.6MB 的 DLL 随包带
- **已验证**：Go 用 LoadLibrary + purego.RegisterFunc 拿到 `OrtGetApiBase` → `OrtApi(17)` 非空。探针代码在 `/tmp/orttest/main.go`

## 模型文件（已下载到本地）

```
C:\Users\<user>\.cache\huggingface\hub\models--datalab-to--indra-ocr\snapshots\bd02167fdf2005a19962cc15188f8496a2c87d3b\
```

实际用到 4 个（总计约 18MB）：

| 文件 | 大小 | 用途 |
|---|---|---|
| `det.onnx` | 390 KB | 行检测。输入 `x:[1,1,H,W]` float，输出 `out:[1,3,h,w]` |
| `rec_cjk.onnx` | 10.2 MB | 中文识别。输入 `x:[B,1,32,W]`、`widths:[B]` int64 |
| `rec.onnx` | 5.04 MB | 拉丁识别。输入 `x:[B,1,96,W]`（注意高度 **96**，cjk 是 32） |
| `script.onnx` | 192 KB | 文字系统分类。输入 `x:[B,1,32,W]`、`widths:[B]`，输出 `p:[B,4]` |
| `meta.json` | 212 KB | 字典 + 参数 |

**`rec_cjk` 输出**：`ids:[B,T]` int64、`conf:[B,T]` float、`lengths:[B]` int64、
`blank:[B,T]` float、`topv:[B,T,8]` float、`topi:[B,T,8]` int64
→ **CTC 解码在模型内部完成**，Go 只需按 `lengths` 截断 `ids` 再查字典。

**`det` 输出** `out[0]`=核心带 logit，`out[1]`=上距离，`out[2]`=下距离。

## meta.json 参数（已读出）

```
det_long_side = 1024
rec_height    = 32
rec_window    = 3
vocab         = "{\"blank\": 0, \"chars\": [...]}"   (9585 字符, 拉丁)
vocab_cjk     = 同格式 (58770 字符)
vocab_world   = 14700
vocab_rtl     = 6800
script_groups = [4 项]
lm_params     = {2 项}
```

## 算法（源码已通读，路径 `C:\Users\<user>\AppData\Local\Programs\Python\Python312\Lib\site-packages\indra_ocr\`）

### 1. 识别前处理 — `rec_preproc.py`（照抄即可）

```
LINE_HEIGHT = 32, PAD_Y = 0.2, PAD_X = 0.3, WIDTH_MULTIPLE = 8

crop_line(gray, box, height=32, pad_y=0.2, pad_x=0.3, max_width=4096, window=1, dark=None):
    pad_y_eff = (window*(1+2*pad_y) - 1) / 2      # window=1 时 = 0.2
    height_eff = height * window                   # window=1 时 = 32
    h = y1 - y0
    X0,X1 = x0 - pad_x*h, x1 + pad_x*h
    Y0,Y1 = y0 - pad_y_eff*h, y1 + pad_y_eff*h
    裁切（越界填 0 或 255 —— dark 则 0，否则 255）
    out_w = max(8, min(4096, round(cw*height_eff/ch)))
    缩放：ch > height_eff 用 INTER_AREA，否则 INTER_LINEAR
    ink = resized/255.0
    return ink if dark else 1.0 - ink     ← 注意：非深底要反色
```

`is_dark_line`：取框内灰度直方图的 2%/50%/98% 分位，
`hi - med > 2*(med - lo) + 8` 即为浅色文字深色背景。

`pad_batch`：右侧补零到 8 的倍数，返回 `[B,1,H,W]` + `widths`。

**拉丁识别器高度是 96 不是 32**（`rec.onnx` 输入 `[B,1,96,W]`），要用
`crop_line(height=96)`。`rec_cjk` 用 32。

### 2. 检测前处理 — `det_post.py`

```
CORE_FRAC = 0.4, CORE_X_SHRINK = 0.15, OUT_STRIDE = 2, INPUT_MULTIPLE = 32

page_to_input(gray, long_side=1024):
    scale = 1024 / max(h,w)
    nh,nw = round(h*scale), round(w*scale)；缩放（scale<1 用 INTER_AREA 否则 LINEAR）
    ph,pw = ceil(nh/32)*32, ceil(nw/32)*32        # 补到 32 的倍数
    out = 全 0 的 float32 [ph,pw]                  # ink map: 0 = 白
    out[:nh,:nw] = 1.0 - gray/255.0                ← 反色
    return out, scale
```

### 3. 检测后处理 — `det_post.py: decode()`

```
prob = sigmoid(out[0])
mask = prob > 0.5
连通域标记（4 连通）→ 每个连通域：
  y0 = mean( (y+0.5)*2 - out[1] )     # 像素中心换算回输入像素
  y1 = mean( (y+0.5)*2 + out[2] )
  score = mean(prob)
  lh = max(1, y1-y0)
  ex = min(0.15*lh, 0.5*lh)
  x0 = left*2 - ex ;  x1 = (left+width)*2 + ex
  跳过面积 < 2 的连通域
```

**注意**：Go 里 `cv2.connectedComponentsWithStats` 没有等价物，要自己写
4 连通的两遍扫描标记算法（union-find 或 BFS）。

### 4. 识别后处理

`ids` 已经解码，按 `lengths[b]` 截断，每项是字典索引（0 = blank）。
跳过 0，其余查 `vocab_cjk.chars[i]`。`conf` 是该字符置信度。

## 已废弃/不采用的方案（不要再走一遍）

- **DeepSeek-OCR**：3.4B 参数、6.8GB、CPU 数十秒。面向整页文档压缩研究，不是精确 OCR。
- **PP-OCRv6 tiny**：我上一轮为了延迟选的，错了。用户痛点是准确率。
- **PP-OCRv6 small**：正常排版不错，但艺术字输出 `��` 乱码，且一个模型通吃 50 语言。
- **SVG 方案画长难句底线**：mygo 不支持 `preserveAspectRatio`，等比缩放会毁掉线型。已用手写 Path 实现。
- **把截图翻译改成发 AI**：VLM 在 CPU 上 4.8–9.5 秒，比专用 OCR 慢 100–1000 倍。AI 只作兜底。

## 实现计划（900 行左右，分 4 块）

1. **`indra_ort.go`**（~250 行）：ORT 绑定
   - `LoadLibraryW` + `GetProcAddress` 封装
   - `OrtApi` 函数表按**偏移**取指针（这是最难的部分，偏移错一个就崩）
   - 需要用到：CreateEnv(3)、CreateSessionOptions、SetIntraOpNumThreads、
     CreateSession、CreateTensorWithDataAsOrtValue、Run、
     GetTensorMutableData、GetTensorTypeAndShape/GetDimensions、
     Release* 系列
   - **参考实现**：`github.com/getcharzp/onnxruntime_purego`（子代理验证可用）、
     `github.com/yalue/onnxruntime_go`（732 star，但用 cgo，可读其偏移定义）
2. **`indra_image.go`**（~200 行）：灰度化、双线性/面积缩放、`crop_line`、`is_dark_line`
3. **`indra_detect.go`**（~200 行）：Sigmoid、4 连通标记、距离场、框提取
4. **`indra_ocr.go`**（~250 行）：宽度分批、字典加载、主流程、结果组装
5. 接入点：替换 `ocr.go` 里 `RecognizeImageFileIn`

## 接入点

当前 OCR 入口（不要破坏现有签名与回落）：

- `ocr.go:RecognizeImageFileIn(ctx, path, language) (string, error)` —
  截图识别主入口，内部 `prepareOCRImage` 做 2x 放大 + 临时文件
- `snip.go` 的框选流程产出 PNG 后调它
- 设置项 `Config.OCRLang`（"auto" / "zh-Hans-CN" / "en-US" / ...）
- 失败要能回落到 Windows.Media.Ocr，不能因为 Indra 有问题就完全不能截图翻译

## 项目当前状态

- build `2026-10-25T20:30`，316 个测试全绿，`gofmt`/`go vet` 干净
- 有 5 个 gesture 测试失败是**既有问题**（用 git stash 验证过），与 OCR 无关
- 本轮已完成：
  - 翻译引擎熔断（谷歌 429 时跳过 60s，实测 502ms → 169ms）
  - AI 精翻按钮（✨，译文行，1.1s，原地替换）
  - 34 种语言 + `detectLocal` 按文字系统识别
  - TTS 34 种语音 + SAPI 按语言选voice
  - DeepL 引擎接入（可选 key）
  - 长难句手绘底线（实线/波浪/双线/点线，按 span 连续绘制）
  - 写作改进 prompt 补全（380 → 7792 字符）
  - 划词气泡悬停竞态修复

## 开发工具

`./dev.ps1 <场景> [-Zoom N] [-CropFrom f] [-CropTo f] [-Watch] [-Delay ms]`
场景：grammar / dict / writing / chip / chip-tip / settings / ocr
只写 `build\` 下的临时 exe 和截图，不碰正式版。

## 待验证（做之前先确认）

1. `rec_cjk.onnx` 的**中文实际效果**（之前测的全是英文）
2. 纯 Go 跑通一次完整推理（目前只验证了加载 DLL + 拿到 API 表）
3. 拉丁识别器高度 96 vs 中文 32 —— 是否影响中英混排行
