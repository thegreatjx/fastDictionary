# fastDictionary 交付流程

每次完成代码改动，都要完整走完下面四步，不只停在 `go test`。

1. 检查：`gofmt -w` 改动文件，`go vet ./...`，`go build ./...`，`go test ./...`。
2. 提交并推送：`git push origin main`。
3. 出包并校正快捷方式：`.\build.ps1 -Package`（重新构建 GUI exe、把桌面和开始菜单
   快捷方式指回 `fastDictionary.exe`、在 `dist/` 生成 zip 与 sha256）。
4. 收尾记录：在 `CHANGELOG.md` 写入源码提交、包名、SHA256、测试数；再提交并推送。
   分发包留在仓库外的 `dist/`，不纳入 Git。
