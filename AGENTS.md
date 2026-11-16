# fastDictionary 交付流程

每次改变程序行为的代码改动，都要完整走完下面四步，不只停在 `go test`。

1. 检查：`gofmt -w` 改动文件，`go vet ./...`，`go build ./...`，`go test ./...`。
2. 提交并推送：`git push origin main`。
3. 出包并校正快捷方式：`.\build.ps1 -Package`（重新构建 GUI exe、把桌面和开始菜单
   快捷方式指回 `fastDictionary.exe`、在 `dist/` 生成 zip 与 sha256）。
4. 收尾记录：在 `CHANGELOG.md` 写入源码提交、包名、SHA256、测试数；再提交并推送。
   分发包留在仓库外的 `dist/`，不纳入 Git。

## 验证与发布契约

- **界面改动先取证。** 改 `theme.go`、`grammar.go`、`actions.go`、`settings*`、
  `home.go`、`popup*`、`selicon.go` 时，先用 `.\scripts\ui-evidence.ps1` 抓浅色/
  深色与受影响的强调色，核对 `manifest.json` 的 commit、exe SHA256、配置和主题。
  没有 manifest 的截图不算验证；`ui.Tester` 不能替代真实原生窗口。
- **协议与优先级改动先补 fixture。** 提示词、解析器、持久化配置同一次升级；
  旧格式、错序、缺字段、整段改写、双来源/回声都要有测试。
- **同一类问题一次收口，批量出包。** 不按单个微修复出包；一个包对应一个已验证
  的类别。行为变化时递增版本号，不重复用同一个版本号发不同行为。
- **只改测试、文档、脚本且不改变二进制行为时，不重新出包。** 跑完第 1 步检查后
  直接提交；`CHANGELOG` 记为工程变更，不分发新 zip。
- **出包前确认旧进程已停止。** `build.ps1` 已负责停进程、校正快捷方式和生成
  SHA256；不要手工 `go build -o` 绕过它。

详细边界契约见 `docs/ENGINEERING_CONTRACT.md`，这些规则的来由见
`docs/RETROSPECTIVE_2026-11.md`，原生 UI 验证流程见
`.claude/skills/fastdictionary-ui-verify/SKILL.md`。
