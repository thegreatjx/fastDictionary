# 工程契约

> 这份文档不是风格建议，而是每次改动完成后要能对得上的检查表。
> 每一条都给出“为什么、怎么做、怎么自动检查”。规则存在的理由见
> `docs/RETROSPECTIVE_2026-11.md`：返工集中在边界，而不是功能内部。

## C1 边界只有一份真相

**为什么。** 一个概念如果同时存在于两份可以各自演化的表示里，漂移只是时间问题。1.4.2 的旧提示词配新解析器、1.4.5 的组件借用语义色、以及“截到旧 exe”的验证返工，都是同一结构。

**怎么做。**

| 边界 | 唯一真相 | 不允许 |
| --- | --- | --- |
| 颜色 / 圆角 / 间距 | `theme.go` 的 `AppTheme` 与语义角色函数 | 组件里 `ui.Hex`/`ui.RGBA`，或借 `Warning`/`Danger` 表达非警告语义 |
| 模型协议 | 提示词常量 + `ParseSentenceAnalysisDetailed` + `ParseOutcome` | 只改提示词不改解析器，或只改解析器不迁移旧配置 |
| 持久化配置 | `Config` 的 json 字段 + `ensureDefaultActions` 迁移 | 读取旧字段却没有迁移路径 |
| 构建身份 | `build.ps1` 的 exe 名 + `CHANGELOG` 的源码提交/包/SHA | 用另一个 exe 名、旧进程或另一份配置做验证 |
| 操作结果 | 返回 error + 后置条件 + 请求 token/generation | 把“函数返回”或“请求已发”当成成功 |

**检查。** 能在代码里消除第二份真相的，优先消除（具名结构、单一入口、迁移函数），不要只靠注释约束。

## C2 设计角色必须可执行

**为什么。** `DESIGN.md` 能描述“难点不是警告”，但拦不住组件写 `at.Warning`。1.4.5 的琥珀难点和 30% 从句底就是散文契约没有执行器。

**怎么做。**

- 新角色先加进 `theme.go` 的语义结构（例如 `grammarClauseColors`、`grammarDifficultyColors`），组件只读字段，不再做颜色运算。
- 角色函数返回具名字段，不用多个位置返回值；调用点取错返回值的错误应无法写出来。
- 半透明色必须先 `Over` 到真实表面再算对比度，不能拿 30% 原始 alpha 当最终色。
- 新组件要覆盖 `normal` / `hover` / `pressed` / `selected` / `disabled` 五态；框架会自动改写的态（`Selected(true)` 刷主题底、`Disabled(true)` 降透明度）要显式验证。
- 内联 `RichText` 子元素不支持 `Padding`/`Radius`/`Border`；需要留白或圆角时改用 `Box` 包一层，或不要写无效调用。

**检查。**

```powershell
go test -run 'TestGrammarPaletteContrastAcrossThemes|TestGrammarClauseWashHierarchy|TestGrammarDifficultyStaysNeutral|TestUIFilesHaveNoEmojiOrHardcodedColors' -count=1 .
```

新增语义角色时必须同时补进 `theme_contract_test.go`，覆盖 4 套强调色 × 明暗。

## C3 模型协议成对升级，宽容输入、严格输出

**为什么。** 模型是外部随机服务，不是内部函数。1.4.0 的精确拼接校验、1.4.2 的旧指令、1.4.4 的错序分段，都是把“模型会按 schema 输出”当成了保证。

**怎么做。**

- 提示词常量、解析器字段、`OutputSchema`、旧配置迁移四者同一次提交升级；版本号写进常量名（V2/V3/V4）。
- 解析器对输入宽容：错序、标点变体、半截从句、伪造主句、字符串/数组译文都要有路径。
- 对输出严格：屏幕上显示的文字必须切自原文；无法对齐就回退并保留 `ParseOutcome`，不显示模型改写的词。
- 每种新失败形态留一个真实响应 fixture，放 `*_test.go`，不要只测手工构造的完美 JSON。
- 迁移必须幂等：第二次启动不能重复改写用户配置；用户自己编辑过且兼容当前协议的提示词不能覆盖。

**检查。**

```powershell
go test -run 'TestParseSentenceAnalysis|TestParseGrammar|TestEnsureDefaultActions' -count=1 .
```

新增协议字段时，至少补：正常、旧格式、错序、缺字段、整段改写五类。

## C4 成功必须有后置条件

**为什么。** 1.1.0 r8 的静默保存失败、r11 的过期精翻、复制成功但界面不重绘，都是把“调用返回”当成了“结果成立”。

**怎么做。**

- 保存：写入成功后读回或校验文件存在；失败向上返回 error，UI 显示一行原因。
- 异步请求：每个结果带 `requestToken`/generation，回来时与当前输入比对，不一致就丢弃。
- 重绘：状态变更后显式 `win.Update`/快照版本递增；不要假设下一帧会自动来。
- 回退：`ParseOutcome`、引擎错误、空结果都要转成用户能看懂的原因；不允许“看起来正常”的假成功。

**检查。** 复用 `STATE_MACHINES_FIX_PLAYBOOK.md` 的契约测试；新功能如果新增异步路径，必须复用现有的 token/generation 辅助，不另造一套。

## C5 优先级与回退写成表

**为什么。** 语言检测 `han > latin`、精翻回声、Google 词典盖有道，都是隐式顺序在不同调用点各自决定。

**怎么做。**

- 多来源决策写一张显式优先级表（例如：有道 > Google；本地检测 > 引擎判断；目标语言回退后必须 `source != target`）。
- 每个回退都有后置条件；回退触发时记录原因，不要静默。
- 同一决策只在一个函数里做，调用点不重复判断。

**检查。** 双来源、回声、中英混排、单来源缺失各留一个 fixture。

## C6 视觉结论必须带身份

**为什么。** 在 1.4.5 的验证中，旧进程、错误 exe 名、另一份配置和另一套主题都曾让截图看起来“验证过了”，但证明的是另一个工件。

**怎么做。**

- 任何界面改动都用 `scripts/ui-evidence.ps1`，不要手工拼 `--shot` 或复用用户配置。
- 证据目录必须包含 `manifest.json`、`screenshot.png`、`control-tree.txt`；manifest 记录 commit、工作区是否干净、exe SHA256、配置、主题/强调色、窗口、时间。
- 改 `theme.go` 或语义色时，至少跑浅色 + 深色；涉及强调色联动的组件跑 4 套强调色。
- 视觉判断用真实原生窗口；`ui.Tester` 只能证明结构/文本，不能证明像素、遮挡、输入区域和窗口圆角。
- 控件状态用 `mygo-automation snapshot -i --full` 核对：`disabled`、`checked`、`selected`、`actions` 是否与预期一致。
- 截图与 manifest 对不上（commit 不同、主题不同、exe 哈希不同）时，结论作废，不解释为“差不多”。

**检查。**

```powershell
.\scripts\ui-evidence.ps1 -View grammar -Theme light -Accent sky
.\scripts\ui-evidence.ps1 -View grammar -Theme dark  -Accent mint
```

详细步骤与 MyGo 限制见 `.claude/skills/fastdictionary-ui-verify/SKILL.md`。

## C7 批量验证，按类别出包

**为什么。** 4 天 24 个包、16 个仍叫 `1.1.0`，说明每个微修复都在重新支付构建、安装、手工检查的成本；用户成了测试执行者。

**怎么做。**

- 同一类问题在一次修复中收口；验证通过后只出一个包。
- 出包前必须：`gofmt`、`go vet ./...`、`go build ./...`、`go test ./...`，以及受影响的证据（UI harness 或协议 fixture）。
- 只改测试、文档、脚本且不改变二进制行为时，不重新出包；在 `CHANGELOG` 记为工程变更。
- 发布记录包含源码提交、包名、SHA256、测试数、证据路径；版本号随行为变化递增，不重复使用同一个版本号发不同行为。
- 出包前确认运行进程已停止；新包启动后用 manifest 或日志确认运行的是新 exe。

**检查。** `build.ps1` 已负责停旧进程、校正快捷方式、生成 zip/sha256；不要绕过它手工 `go build -o` 出包。

## 提交前检查表

- [ ] 这次改动属于哪个边界？那个边界的唯一真相在哪里？
- [ ] 有没有新增第二份颜色/协议/优先级/成功状态？能不能消除？
- [ ] 旧配置、旧响应、错序输入、失败路径是否有 fixture？
- [ ] 界面改动是否跑了 `ui-evidence`，manifest 与当前 commit/exe/theme 一致？
- [ ] 是否只改测试/文档/脚本？如果是，是否避免了一次无意义的出包？
- [ ] `CHANGELOG` 是否记录了源码提交、包名、SHA256、测试数与证据路径？

规则不是永久正确的。发现某条规则与真实约束冲突时，带着证据改这份文档，而不是在提交里默默绕过。
