# AI 精翻 · 模型服务规格（AI_SERVICES.md）

本文是「设置 → AI 精翻」接入多家模型服务的总纲：做什么、不做什么，
哪些规则不能破，每条规则由哪个测试守着。改这块代码前先读这里；
改了行为，同步改这里。

- 平台：Windows（DPAPI 只在 Windows 可用，见 §4）。
- 协议：只走 OpenAI 兼容的 `/chat/completions` 与 `/models`。
- 代码：`ai_presets.go`（预设表与切换）、`ai_client.go`（HTTP 客户端）、
  `ai_settings.go`（设置页异步状态）、`secret_windows.go`（密钥加密）、
  `config.go`（字段、迁移、落盘）、`settings_pages.go`（表单）、
  `ai_vision.go`（图片压缩与流式分隔符）、`ai_vision_actions.go`（看图重译流程）、
  `ocr_mode.go`（截图识别模式）。

---

## 1. 目标与非目标

**目标**

1. 没装 Magpie 的人，选一家服务商、填一个 Key，就能用 AI 精翻。
2. 装了 Magpie 的人，行为和以前一样：不要 Key，保留推理强度和图片回退。
3. 出错时告诉用户下一步做什么，而不是贴一段 HTTP 原文。
4. Key 不以明文落盘，不出现在日志和界面错误里，不发给别的服务商。

**非目标**（有需要再单独立项）

- Anthropic 原生协议、Gemini 原生 `generateContent` 协议；Azure OpenAI 的 deployment 地址形态。Gemini 走官方 OpenAI 兼容层，见 §2.1。
- 同时保存多个服务商实例、按动作选服务商。
- 手动代理设置（目前跟随系统 / 环境变量）。
- macOS / Linux 上持久保存 Key。

---

## 2. 用户可见行为

设置页只有一个「模型服务」下拉，选项来自 `aiPresets` 表：

| 服务 | 需要 Key | 默认地址 | 默认模型 |
| --- | --- | --- | --- |
| 本机 Magpie | 否 | `http://127.0.0.1:3425/v1/chat/completions` | `workbuddy-ai/deepseek-v4.1-flash` |
| DeepSeek | 是 | `https://api.deepseek.com/v1` | `deepseek-flash` |
| 通义千问（百炼） | 是 | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `qwen-plus` |
| 智谱 GLM | 是 | `https://open.bigmodel.cn/api/paas/v4` | `glm-4-flash` |
| 硅基流动 | 是 | `https://api.siliconflow.cn/v1` | （空，需选择） |
| OpenRouter | 是 | `https://openrouter.ai/api/v1` | `deepseek/deepseek-chat` |
| OpenAI | 是 | `https://api.openai.com/v1` | （空，需选择） |
| Gemini（Google AI Studio） | 是 | `https://generativelanguage.googleapis.com/v1beta/openai/` | `gemini-3.5-flash-lite` |
| Ollama（本机） | 否 | `http://127.0.0.1:11434/v1` | `qwen2.5:7b` |
| 自定义 | 是 | 用户填写 | 用户填写 |

加一家服务商只改这张表；带 `ModelOptions` 的预设可附带离线模型清单，表单仍按统一规则渲染，不按服务商另写分支。

### 2.1 Gemini（Google AI Studio）

- 使用 Google 官方的 [OpenAI 兼容层](https://aistudio.google.com/docs/openai)，不是原生 `generateContent` 协议。客户端仍走 `/chat/completions`，图片用 OpenAI 格式的 `image_url` data URL 发送。
- API Key 从 [Google AI Studio](https://aistudio.google.com/apikey) 创建。预设地址已经填好，Key 只保存在本机。
- 默认模型 `gemini-3.5-flash-lite` 是 [官方模型 ID](https://aistudio.google.com/docs/models/gemini-3.5-flash-lite)。官方模型卡标注「Text Input and output」和「Image Input only」：它能读截图并输出文字，也能做翻译；它不是专门的 OCR 引擎，截图文字质量仍取决于图片清晰度与提示词。
- 设置页内置一份常用模型清单，默认就是 `gemini-3.5-flash-lite`。不点「获取模型」也能选；点了则尝试用 `/models` 拉账号实时列表，拉不到仍保留内置清单。清单外模型可选「自定义模型…」手填。
- [官方价格页](https://aistudio.google.com/docs/pricing) 有 Free tier，但免费模型范围与配额会变，以 Google AI Studio 控制台为准。
- 截图初始识别是否发图由「截图识别模式」决定：`AI 直接看图` 总是发图，`本地优先` 在本地 OCR 可疑时发图，`本地` 不发图。翻译浮窗里的「AI 精翻」按钮在截图模式下也总是发图，见 §8。模型不支持图片时由服务端报错，不在客户端预禁。
- 2026-11-14 用本机已配置的真实 Gemini Key 实测：`/v1beta/openai/models` 返回 64 个模型且包含 `gemini-3.5-flash-lite`；纯文字 `ping -> pong` 用时 867ms；带 `unfolded` 文字的 PNG 图片请求返回 `[SOURCE] unfolded / [TARGET] 展开`，用时 1.581s。文字流、图片流和 `[SOURCE]` / `[TARGET]` 分隔符都已跑通。计费金额、免费额度余量和不支持图片模型的真实 400 文案仍未在控制台/该模型上核对。

**Magpie 模式**：地址、模型、推理强度、测试连接。

**直连模式**：API 地址、API Key（可显示 / 隐藏）、模型（预设清单、拉取列表或手填）、
兼容旧配置的「图片输入」提示开关、测试连接、「去获取 Key」链接、填写说明面板。
「截图识别模式」在设置 → 快捷键页，与具体模型服务解耦。

**切换服务**（`applyAIPreset`）：

- 地址和模型优先恢复该服务商上次保存的值；没保存过才用预设默认值。预设没有默认模型时清空模型，不沿用上一家的模型 ID。
- 切到「自定义」时保留用户填过的地址和模型，只清掉 Magpie 的默认值。
- 兼容旧配置的「图片输入」开关重置为关闭；它不再阻止用户选择看图模型。
- 切换服务时按服务商分别封存 / 恢复 Key、API 地址和模型选择：不把 Gemini 的 Key 发给 Magpie，也不在切回 Magpie 时把模型重置成 deepseek。新服务商没有保存过 Key 才提示「请重新填写 API Key」。
- 在直连表单里改 API 地址，主机一变仍只清当前服务商的 Key（§4）。

**就绪判定**（`Config.AISetupProblem`）：返回「还差什么」，空串表示可以发请求。
所有 AI 入口都只问这一个问题，不再各自只看 `AIEnabled`。

| 条件 | 返回 |
| --- | --- |
| 未启用 | `AI 精翻未启用` |
| Magpie | 就绪（是否在运行只能靠请求来发现，失败另有提示） |
| 自定义且地址为空 | `还没设置模型服务` |
| 其他直连服务地址为空 | `还没填 API 地址` |
| 预设标记需要 Key 而 Key 为空（「自定义」除外） | `还没填 API Key` |
| 模型为空 | `还没选模型` |

没就绪时各入口的表现：

- 精翻按钮：不发请求，按钮旁显示「精翻失败：还没填 API Key」和「打开 AI 设置」。
- 长难句 / 润色页签：回落到机器翻译，并提示缺什么。
- 生词提取：静默跳过。它在每次划词后自动发起，不能每次都撞 401。
- 设置页「状态」下方：显示「还差一步：…」。

**新装**：配置文件不存在时，AI 保持启用，但服务设为「自定义」、地址和模型留空，
即「未配置」。大多数人没装 Magpie，默认指向本机端口只会让每次划词都发一个必败的请求。
已有配置文件的用户不受影响（见 §3）。

---

## 3. 配置与迁移

| 字段 | JSON | 说明 |
| --- | --- | --- |
| `AIProvider` | `ai_provider` | 预设 ID；未知值一律视为 `custom` |
| `AIURL` | `ai_url` | 用户填写的地址，原样保存，请求时再归一化 |
| `AIModel` | `ai_model` | 模型 ID |
| `AIKey` | `-` | 明文 Key，只在内存 |
| `AIKeyEnc` | `ai_key_enc` | 当前服务商的 DPAPI 密文，base64；兼容旧配置 |
| `AIKeyEncByProvider` | `ai_key_enc_by_provider` | 按服务商 ID 保存的 DPAPI 密文；切换服务后靠它恢复 Key |
| `AISettingsByProvider` | `ai_settings_by_provider` | 按服务商 ID 保存的 API 地址和模型选择；切换服务后靠它恢复设置 |
| `AIVision` | `ai_vision` | 旧配置的兼容提示；不再作为看图请求的硬性开关 |
| `OCRMode` | `ocr_mode` | 截图识别模式：`local` / `vision` / `local_first` |
| `AIEffort` | `ai_effort` | 只对 Magpie 发送 |

**旧配置迁移**（`normalizeAIProviderConfig`）看文件里有没有 `ai_provider` 字段，
而不是看解码后的默认值：

- 没有该字段、地址为空或是 Magpie 端口 → `magpie`。
- 没有该字段、地址是别的 → `custom`。不能把老用户的自定义地址当成 Magpie。
- 有该字段 → `normalizeAIProvider`，未知值变 `custom`。

---

## 4. 密钥

**存储**

- 每次保存（`store.Update` / `RetrySave`）都调用 `sealAIKey`：明文 → DPAPI → 当前服务商的 `ai_key_enc` 和 `ai_key_enc_by_provider[provider]`。
- 旧配置只有 `ai_key_enc` 时，加载时把它迁移到当前 `ai_provider` 对应的 map 项；之后以 map 为准。
- 加密失败时不写明文，当前 `ai_key_enc` 置空，并把错误交给现有的保存失败横幅（A9）；其他服务商已保存的 Key 不动。
- 加载时某个服务商解密失败（换了 Windows 账户、文件被改）：记日志但不记密文，只清掉该服务商的 map 项；其他服务商不受影响。
- 非 Windows 平台的 `protectSecret` 直接报错：Key 只在本次运行有效，不落盘。

**发送**

- 只通过 `Authorization: Bearer <key>` 发送。用户粘贴时带了 `Bearer ` 前缀会被去掉。
- 预设标记为不要 Key 的服务（Magpie、Ollama）即使配置里残留 Key 也不发送（`Config.AIConn`）。
- 一个 Key 绑定一个服务商：切换服务时只恢复该服务商自己的 Key；手动改 API 地址且主机变化时，只清当前服务商的 Key。
- 携带 `Authorization` 或请求正文的请求，不跟随跨主机重定向；HTTPS 不降级到 HTTP。

**显示**

- 服务商返回的错误正文先经 `redactAIKey` 脱敏再进界面和日志。
- 设置页的状态比对只存 Key 的 SHA-256 前 8 字节指纹（`aiKeyFingerprint`），不另存一份明文。

---

## 5. 客户端契约（`ai_client.go`）

**地址归一化**（`aiBaseURL`）：用户可以填服务商根地址，也可以填从文档复制来的完整端点。

- 去掉末尾的 `/chat/completions` 或 `/models`，再拼回需要的那一个。
- 路径为空时补 `/v1`；其他路径原样保留（智谱的 `/api/paas/v4` 不能被改写）。
- 只接受 `http` / `https`，拒绝带用户名或 `#片段` 的地址。
- 直连服务地址为空时报「请先填写 API 地址」，**不回落到 Magpie**。

**请求字段**

- 直连服务不发 `reasoning_effort` / `reasoning`：各家支持不一，严格的服务会回 400。
- 是否发送图片由 `OCRMode`、截图模式的「AI 精翻」按钮以及 `local_first` 的可疑结果复核决定；不再用
  `AIVisionAllowed()` 卡死用户自选模型。模型不支持图片时由服务端报错，
  再经 `friendlyAIError` 提示。

**流式响应**

- 兼容 `data:` 后有无空格、没有 `[DONE]` 但带 `finish_reason` 的 SSE。
- 普通响应上限 4 MB，流式上限 8 MB，超出即报错，防止异常服务撑爆内存。

**模型列表**：`GET /models`，同时解析 `data[].id` 和 `models[].name`。
404 / 405 返回哨兵错误 `errAIModelsUnsupported`，设置页显示「不支持」；模型仍可从预设清单选择或手填。

列表按「服务商 + 地址 + Key 指纹」缓存 5 分钟。设置页打开和服务商切换时，Magpie 或配置就绪的直连服务会自动加载完整列表；切换后再切回来直接复用缓存，不再只剩兜底清单，也不需要重新点「获取模型」。未配置完成的直连服务不自动发请求，只显示预设清单或当前模型。

**错误分类**：所有失败都在知道状态码或传输错误的地方变成 `*aiFailure{kind, msg}`。
上层只看 `kind`，不在字符串里找 `"429"`。没分类的错误（编码失败、地址解析）原样显示，
不猜原因——按字符串猜曾把「503 service available」说成「Key 无效」。

| kind | 来源 | 自动重试一次 |
| --- | --- | --- |
| `Auth` | 401 | 否 |
| `Permission` | 403 | 否 |
| `NotFound` | 404 | 否 |
| `BadRequest` | 400、地址无效 | 否 |
| `RateLimit` | 429 | 是 |
| `Server` | 5xx | 是 |
| `Timeout` | 超时 | 是 |
| `Network` | DNS、拒绝连接、TLS | 是 |
| `MagpieDown` | 本机网关拒绝连接（errno，不匹配文字） | 否 |
| `Protocol` / `Unknown` | 其他 | 否 |

重试只在 AI 精翻（`refineWithAI`）里做一次，间隔 350 ms。用户取消的请求
（`context.Canceled`），以及外层 ctx 已结束的请求，一律不重试。

---

## 6. 设置页状态机（`ai_settings.go`）

两个异步动作各有一个显式阶段枚举，外加「来源」和「代次」。

```text
连接测试  aiProbeIdle ──点击──▶ Running ──▶ Succeeded
                                  │        └─▶ Failed
                                  └─再次点击：取消上一次，代次 +1

模型列表  aiModelsIdle ──▶ Loading ──▶ Ready / Unsupported / Failed
```

- **来源**（`aiProbeSource` / `aiEndpointSource`）= 服务 + 地址 + Key 指纹（+ 模型 + 图片开关）。
- **写回规则**：异步结果回来时，代次和来源都对得上才写；对不上直接丢弃。
- **显示规则**：当前配置算出的来源和状态里的来源不同，就按 Idle 显示。
  用户改了任何一项，旧的「正常 / 失败」自动消失，不需要另设清除标志。
- 模型列表同一来源只拉一次（single-flight），有新鲜缓存就跳过，「刷新」强制重拉。
- Magpie 拉取失败时回退到内置的常用模型；直连服务不继承这份列表。Gemini 只使用自己的 `ModelOptions` 清单。

---

## 7. 不变量与守护测试

| # | 不变量 | 测试 |
| --- | --- | --- |
| I1 | 地址归一化覆盖根地址、完整端点、非 `/v1` 路径 | `TestAIEndpointNormalization` |
| I2 | 鉴权只用 Bearer；兼容的 SSE 变体都能读完 | `TestStreamAISendsBearerAndAcceptsCompatibleSSE` |
| I3 | HTTP 失败都带 kind | `TestStreamAIHTTPFailuresAreTyped` |
| I4 | 错误正文里的 Key 被脱敏 | `TestStreamAIRedactsKeyFromProviderError` |
| I5 | 自定义地址为空不回落 Magpie | `TestStreamAICustomEmptyEndpointDoesNotFallBackToMagpie` |
| I6 | 直连服务不发推理字段 | `TestStreamAICustomNeverSendsReasoning`、`TestAIReasoningEffortOnlyForMagpie` |
| I7 | 不要 Key 的服务不发 Key | `TestAIConnOmitsKeyForNoKeyServices` |
| I8 | 切服务不泄漏别家 Key，切回来能恢复，重启后仍在 | `TestApplyAIPresetKeepsOtherProviderKeySeparate`、`TestApplyAIPresetNoKeyServiceDoesNotLeakOrForgetKey`、`TestApplyAIPresetPreservesKeyAcrossProviderRoundTrip`、`TestProviderKeySurvivesConfigReload` |
| I9 | 看图请求不因 `Vision=false` 被客户端拒绝 | `TestStreamAIWithImageDoesNotGateOnVisionFlag` |
| I10 | `[SOURCE]/[TARGET]` 流和纯译文流都能解析 | `TestVisionStreamParser`、`TestParseVisionOutput` |
| I11 | 大图透明底压缩为长边 1600 的 JPEG | `TestCompressedImageFileDataURL` |
| I9 | Key 以密文落盘，能读回 | `TestConfigSealsAIKeyAndLoadsIt`、`TestDPAPIRoundTrip`、`TestDPAPIEmptyAndCorrupt` |
| I10 | 旧配置不会把自定义地址认成 Magpie | `TestNormalizeAIProviderConfig` |
| I11 | 模型列表两种格式、「不支持」是显式状态 | `TestFetchAvailableModelsParsesBothShapes`、`TestFetchAvailableModelsUnsupportedIsSentinel`、`TestAIModelUnsupportedIsExplicit` |
| I12 | 过期的测试 / 模型结果被丢弃 | `TestAIProbeDropsStaleResult`、`TestAIProbeCancelsPreviousRun`、`TestAIModelStaleResultIsDropped` |
| I13 | 配置一改，旧测试结果不再显示 | `TestAIProbeSourceChangeHidesOldResult` |
| I14 | Magpie 回退模型不泄漏给直连服务 | `TestGetAIModelsDoesNotLeakMagpieFallbackToCustom` |
| I15 | Gemini 官方兼容地址、默认模型及离线清单 | `TestGeminiPresetUsesOpenAICompatEndpointAndDefaultModel`、`TestGeminiCuratedModelsAreSelectable` |
| I16 | Gemini 看图请求走 `image_url`，模型列表去掉 `models/` 前缀 | `TestGeminiOpenAICompatImageRequest`、`TestGeminiModelsEndpointListsModels` |
| I15 | 取消的请求不重试 | `TestIsTransientAIError` |
| I16 | 两种表单都能渲染 | `TestSettingsAIPageRendersBothServiceModes`、`TestSettingsAIHelpPanelUsesSelectedService` |
| I17 | 就绪判定覆盖各种缺项 | `TestAISetupProblem` |
| I18 | 新装是「未配置」，老 Magpie 配置升级后不变 | `TestFirstRunAIIsUnconfigured`、`TestExistingMagpieConfigStaysMagpie`、`TestFirstRunStillWritesDefaults` |
| I20 | 旧 `ai_key_enc` 迁移到按服务商保存的密文表；改主机只清当前服务商 | `TestLegacyAIKeyEncMigratesToProviderMap`、`TestAIURLHostChangeForgetsCurrentProviderKey` |
| I21 | 服务商地址 / 模型选择切走再切回不丢，重启后仍在 | `TestApplyAIPresetPreservesModelAcrossProviderRoundTrip`、`TestMagpieModelSurvivesProviderSwitch`、`TestProviderSettingsSurviveConfigReload` |
| I22 | 完整模型列表切服务后仍缓存，配置就绪的直连服务打开设置页自动加载 | `TestMagpieModelListSurvivesProviderSwitch`、`TestConfiguredDirectProviderLoadsFullModelListOnSettingsOpen` |
| I19 | 没就绪时精翻、页签、生词提取都不发请求，设置页说出缺项 | `TestRefineWithAIExplainsMissingSetup`、`TestSwitchModeFallsBackWhenAINotReady`、`TestNotesSkipWhenAINotReady`、`TestSettingsAIPageNamesMissingSetup` |

改动这块代码的最小检查集：

```powershell
go build ./... ; go vet ./...
go test -run 'Test(AI|DPAPI|ConfigSeals|ApplyAI|NormalizeAI|StreamAI|FetchAI|SettingsAI|GetAI|IsTransient|FirstRun|ExistingMagpie|RefineWithAI|SwitchMode|Notes)' .
go test -race -run 'TestAI(Probe|Model)' .
```

**真实服务冒烟**（默认跳过，不进日常检查）：

```bash
# 本机 Magpie 路径
go test -count=1 -run TestLiveMagpieStream -v .
# 直连路径，借 Magpie 网关当 OpenAI 兼容服务（不需要 Key）
FASTDICT_LIVE_AI_PROVIDER=custom FASTDICT_LIVE_AI_URL=http://127.0.0.1:3425/v1 FASTDICT_LIVE_AI_MODEL=auto/deepseek-v4.1-flash go test -count=1 -run TestLiveDirectProvider -v .
# 真实服务商：Key 放文件里，不上命令行；会额外检查错误 Key 是否识别成鉴权失败
FASTDICT_LIVE_AI_PROVIDER=deepseek FASTDICT_LIVE_AI_KEY_FILE=/path/to/key go test -count=1 -run TestLiveDirectProvider -v .
```

```bash
# 六家 Keyed 预设都用假 Key 打一次真实接口，确认 401/403 被识别成鉴权失败（不需要真 Key）
FASTDICT_LIVE_AI_BADKEY=1 go test -count=1 -run TestLiveProvidersRejectBadKey -v .
```

2026-11-12 实测：Magpie 路径 1.6 s；直连路径经 Magpie 网关，根地址、`/v1`、完整端点三种写法都通，
`/models` 返回 28 个模型，`auto/deepseek-v4.1-flash` 精翻约 1.8–2.4 s。
假 Key 探测：DeepSeek、通义、智谱、硅基流动、OpenAI 的对话与模型列表都正确识别为鉴权失败；
OpenRouter 的 `/models` 是公开接口，不校验 Key（已知差异，不影响精翻）。
Ollama 走本机 `/models` 同样不需要 Key。

有效 Key 实测（DeepSeek，2026-11-12）：`/models` 返回 `deepseek-flash`、`deepseek-v4-pro`；
`deepseek-flash` 精翻 1.35 s / 12 个分块，`deepseek-chat`（旧别名）1.18 s / 10 个分块，都能用。
预设默认模型已改成列表里的 `deepseek-flash`，不再依赖不在列表中的旧别名。
错误 Key 与成功路径都走真实接口验证过。

有效 Key 实测（Gemini，2026-11-14）：`/models` 返回 64 个模型且包含默认模型；纯文字请求和 PNG 图片请求都成功，图片请求按 `[SOURCE]` / `[TARGET]` 返回原文与译文。指标见 §2.1。

---

## 8. 截图 AI 精翻与识别模式

`OCRMode` 决定截图链路：

- `local`：边缘自适应 Padding → Indra → 可疑时像素重试 / Windows OCR → 普通翻译；
- `vision`：跳过本地 OCR，直接把压缩截图发给当前模型；
- `local_first`：先跑本地 OCR，空结果、乱码或碎框时自动调用 AI 看图；失败仍保留本地结果。

翻译浮窗只保留一个「AI 精翻」按钮。截图模式点它就直接复用 `ocrPath` 原图发图，模型以图片为准纠正 OCR 并同时刷新原文和译文；划词模式只发文字。OCR 编辑/错误面板里的「AI 精翻」也走同一个 `visionRefine`，不再另设「看图」动作。
大图在 `compressedImageFileDataURL` 里限制长边为 1600px，透明底铺白后编码为 JPEG 85。

AI 结果生效后，按钮变成高亮的「已 AI 精翻」；处理中显示 spinner 和「AI 处理中…」，普通机翻保持低调样式。引擎标题只用主题强调色显示 `AI 精翻 · 模型 · 耗时`，不再另放「…已生效」徽标。

流式返回用 `[SOURCE]` / `[TARGET]` 分隔符，而不是僵硬 JSON。`visionStreamParser`
缓冲半截标记并实时刷新原文与译文；模型只吐纯译文时，保留本地 OCR 原文。
模型能力不在客户端预检或禁用：文本模型收到图片后返回 400 等错误，再由
`friendlyAIError` 给用户下一步提示。

---

## 9. 已知限制与后续

按建议顺序排列。

1. ~~新装默认值~~：已改成「未配置」，见 §2。
2. **真实服务商冒烟**：DeepSeek 有效 Key 的文字精翻、Gemini 有效 Key 的模型列表 / 文字 / 图片请求都已走真实接口验证。仍待补的是：更多服务商的有效 Key 成功路径、用量字段核对、不支持图片模型的真实 400 文案，以及 Gemini 控制台的费用与配额余量。
3. ~~`friendlyAIError` 在 `msg` 里找 "Magpie"~~：已改为 `aiFailureMagpieDown` 独立 kind。
   顺带修了一个真 bug：Windows 的拒绝连接文字是「actively refused it」，
   原来匹配 `"connection refused"` 在 Windows 上从不命中，这个分支在真实环境里没生效过；
   现在改为匹配 `ECONNREFUSED` / `WSAECONNREFUSED(10061)`。
4. ~~`isTransientAIError` 的字符串兜底~~：已删除，只认 `aiFailure.kind`。
5. 代理设置、Azure、原生协议、多服务商实例：见 §1 非目标。
