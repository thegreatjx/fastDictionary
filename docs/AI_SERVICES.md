# AI 精翻 · 模型服务规格（AI_SERVICES.md）

本文是「设置 → AI 精翻」接入多家模型服务的总纲：做什么、不做什么，
哪些规则不能破，每条规则由哪个测试守着。改这块代码前先读这里；
改了行为，同步改这里。

- 平台：Windows（DPAPI 只在 Windows 可用，见 §4）。
- 协议：只走 OpenAI 兼容的 `/chat/completions` 与 `/models`。
- 代码：`ai_presets.go`（预设表与切换）、`ai_client.go`（HTTP 客户端）、
  `ai_settings.go`（设置页异步状态）、`secret_windows.go`（密钥加密）、
  `config.go`（字段、迁移、落盘）、`settings_pages.go`（表单）。

---

## 1. 目标与非目标

**目标**

1. 没装 Magpie 的人，选一家服务商、填一个 Key，就能用 AI 精翻。
2. 装了 Magpie 的人，行为和以前一样：不要 Key，保留推理强度和图片回退。
3. 出错时告诉用户下一步做什么，而不是贴一段 HTTP 原文。
4. Key 不以明文落盘，不出现在日志和界面错误里，不发给别的服务商。

**非目标**（有需要再单独立项）

- Anthropic、Gemini 等原生协议；Azure OpenAI 的 deployment 地址形态。
- 同时保存多个服务商实例、按动作选服务商。
- 手动代理设置（目前跟随系统 / 环境变量）。
- macOS / Linux 上持久保存 Key。

---

## 2. 用户可见行为

设置页只有一个「模型服务」下拉，选项来自 `aiPresets` 表：

| 服务 | 需要 Key | 默认地址 | 默认模型 |
| --- | --- | --- | --- |
| 本机 Magpie | 否 | `http://127.0.0.1:3425/v1/chat/completions` | `workbuddy-ai/deepseek-v4.1-flash` |
| DeepSeek | 是 | `https://api.deepseek.com/v1` | `deepseek-chat` |
| 通义千问（百炼） | 是 | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `qwen-plus` |
| 智谱 GLM | 是 | `https://open.bigmodel.cn/api/paas/v4` | `glm-4-flash` |
| 硅基流动 | 是 | `https://api.siliconflow.cn/v1` | （空，需选择） |
| OpenRouter | 是 | `https://openrouter.ai/api/v1` | `deepseek/deepseek-chat` |
| OpenAI | 是 | `https://api.openai.com/v1` | （空，需选择） |
| Ollama（本机） | 否 | `http://127.0.0.1:11434/v1` | `qwen2.5:7b` |
| 自定义 | 是 | 用户填写 | 用户填写 |

加一家服务商只改这张表，表单不加分支。

**Magpie 模式**：地址、模型、推理强度、测试连接。

**直连模式**：API 地址、API Key（可显示 / 隐藏）、模型（可拉取列表，也可手填）、
「图片输入」开关、测试连接、「去获取 Key」链接、填写说明面板。

**切换服务**（`applyAIPreset`）：

- 地址和模型换成预设值；预设没有默认模型时清空模型，不沿用上一家的模型 ID。
- 切到「自定义」时保留用户填过的地址和模型，只清掉 Magpie 的默认值。
- 图片输入重置为关闭。
- 新服务不要 Key，或主机变了，就清空 Key，并提示「请重新填写 API Key」。
- 在直连表单里改 API 地址，主机一变也清空 Key（同一规则，见 §4）。

---

## 3. 配置与迁移

| 字段 | JSON | 说明 |
| --- | --- | --- |
| `AIProvider` | `ai_provider` | 预设 ID；未知值一律视为 `custom` |
| `AIURL` | `ai_url` | 用户填写的地址，原样保存，请求时再归一化 |
| `AIModel` | `ai_model` | 模型 ID |
| `AIKey` | `-` | 明文 Key，只在内存 |
| `AIKeyEnc` | `ai_key_enc` | DPAPI 密文，base64 |
| `AIVision` | `ai_vision` | 只对直连服务有效；Magpie 恒为允许 |
| `AIEffort` | `ai_effort` | 只对 Magpie 发送 |

**旧配置迁移**（`normalizeAIProviderConfig`）看文件里有没有 `ai_provider` 字段，
而不是看解码后的默认值：

- 没有该字段、地址为空或是 Magpie 端口 → `magpie`。
- 没有该字段、地址是别的 → `custom`。不能把老用户的自定义地址当成 Magpie。
- 有该字段 → `normalizeAIProvider`，未知值变 `custom`。

---

## 4. 密钥

**存储**

- 每次保存（`store.Update` / `RetrySave`）都调用 `sealAIKey`：明文 → DPAPI → `ai_key_enc`。
- 加密失败时不写明文，`ai_key_enc` 置空，并把错误交给现有的保存失败横幅（A9）。
- 加载时解密失败（换了 Windows 账户、文件被改）：记日志但不记密文，清掉 `ai_key_enc`，
  用户重新填写即可。
- 非 Windows 平台的 `protectSecret` 直接报错：Key 只在本次运行有效，不落盘。

**发送**

- 只通过 `Authorization: Bearer <key>` 发送。用户粘贴时带了 `Bearer ` 前缀会被去掉。
- 预设标记为不要 Key 的服务（Magpie、Ollama）即使配置里残留 Key 也不发送（`Config.AIConn`）。
- 一个 Key 绑定一个主机：主机变化就清空 Key（§2）。
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
- 图片只在 `AIVisionAllowed()` 为真时发送。

**流式响应**

- 兼容 `data:` 后有无空格、没有 `[DONE]` 但带 `finish_reason` 的 SSE。
- 普通响应上限 4 MB，流式上限 8 MB，超出即报错，防止异常服务撑爆内存。

**模型列表**：`GET /models`，同时解析 `data[].id` 和 `models[].name`。
404 / 405 返回哨兵错误 `errAIModelsUnsupported`，设置页显示「不支持」，模型仍可手填。

**错误分类**：所有失败都在知道状态码或传输错误的地方变成 `*aiFailure{kind, msg}`。
上层只看 `kind`，不在字符串里找 `"429"`。

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
- Magpie 拉取失败时回退到内置的常用模型；直连服务**不**继承这份列表。

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
| I8 | 换主机清 Key，同主机保留 | `TestApplyAIPresetClearsKeyOnHostChange`、`TestApplyAIPresetKeepsKeyOnSameHost`、`TestApplyAIPresetNoKeyServiceClearsKey` |
| I9 | Key 以密文落盘，能读回 | `TestConfigSealsAIKeyAndLoadsIt`、`TestDPAPIRoundTrip`、`TestDPAPIEmptyAndCorrupt` |
| I10 | 旧配置不会把自定义地址认成 Magpie | `TestNormalizeAIProviderConfig` |
| I11 | 模型列表两种格式、「不支持」是显式状态 | `TestFetchAvailableModelsParsesBothShapes`、`TestFetchAvailableModelsUnsupportedIsSentinel`、`TestAIModelUnsupportedIsExplicit` |
| I12 | 过期的测试 / 模型结果被丢弃 | `TestAIProbeDropsStaleResult`、`TestAIProbeCancelsPreviousRun`、`TestAIModelStaleResultIsDropped` |
| I13 | 配置一改，旧测试结果不再显示 | `TestAIProbeSourceChangeHidesOldResult` |
| I14 | Magpie 回退模型不泄漏给直连服务 | `TestGetAIModelsDoesNotLeakMagpieFallbackToCustom` |
| I15 | 取消的请求不重试 | `TestIsTransientAIError` |
| I16 | 两种表单都能渲染 | `TestSettingsAIPageRendersBothServiceModes`、`TestSettingsAIHelpPanelUsesSelectedService` |

改动这块代码的最小检查集：

```powershell
go build ./... ; go vet ./...
go test -run 'Test(AI|DPAPI|ConfigSeals|ApplyAI|NormalizeAI|StreamAI|FetchAI|SettingsAI|GetAI|IsTransient)' .
go test -race -run 'TestAI(Probe|Model)' .
```

---

## 8. 已知限制与后续

按建议顺序排列。前两项影响新用户第一次使用，其余按需。

1. **新装默认值**：新装仍默认「本机 Magpie + 已启用」，没装 Magpie 的人第一次精翻会失败，
   要靠错误提示引导去换服务。可以改成「未配置」状态，首次精翻直接跳到设置页。
2. **真实服务商冒烟**：表里 8 家只用模拟服务器测过协议。发布前至少拿 DeepSeek
   和一家国内服务商实际跑一次精翻、拉一次模型列表。
3. **`friendlyAIError` 仍在 `msg` 里找 "Magpie"**：区分「Magpie 没启动」和一般网络错误，
   应改成独立的 kind 或在 `aiFailure` 上带服务标识。
4. **`isTransientAIError` 保留了字符串兜底**：只为没经过 `aiFailure` 的旧路径；
   旧路径全部收口后删除。
5. 代理设置、Azure、原生协议、多服务商实例：见 §1 非目标。
