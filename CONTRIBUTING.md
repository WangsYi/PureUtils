# Contributing to PureUtils / 参与贡献

感谢对 **PureUtils** 的关注与贡献！ / Thank you for contributing to **PureUtils**!

---

## 环境要求 / Prerequisites

- **OS**: Windows 10 / Windows 11 (x64)
- **Go**: **1.27.3+**（`CGO_ENABLED=0`，零 cgo / 零 WebView / 零 Node）
- **Optional (安装包打包)**: [Inno Setup 6/7](https://jrsoftware.org/isinfo.php) (`ISCC.exe`)

---

## 开发与测试 / Development & Testing

```powershell
# 运行静态检查 / Static checks
go vet -buildvcs=false -unsafeptr=false ./...

# 运行全部测试（含架构依赖边界检查 & 无头 UI 测试） / Run full tests
.\build.ps1 test

# 验证核心精简构建测试 / Verify -tags nofreeimage test suite
.\build.ps1 test -CoreOnly

# 本地构建 PureUtils.exe / Build executable
.\build.ps1 build
```

---

## 插件架构边界 / Architecture Boundaries

本项目通过 [`internal/host/arch_test.go`](internal/host/arch_test.go) 与 [`pkg/purekit/purekit_test.go`](pkg/purekit/purekit_test.go) 在每次 `go test` 时利用 Go AST 自动校验依赖规则：

1. `pkg/purekit/...` **严禁** import `pureutils/internal/...` 或 `pureutils/plugins/...`。
2. `plugins/<name>/...` **严禁** import `pureutils/internal/...` 或任何其他兄弟插件 `pureutils/plugins/<other>/...`。
3. `internal/host/*.go`（含 `*_test.go`）除 `builtin_*.go` 条件编译胶水文件外，**严禁**直接 import `pureutils/plugins/...`。
4. 跨插件协作请统一通过 `pkg/purekit/contracts.go` 定义的 Capability Bus 契约与剪贴板事件总线完成，并做好缺省插件时的优雅降级。

详细架构与新插件接入步骤请参阅 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) / [docs/ARCHITECTURE.en.md](docs/ARCHITECTURE.en.md)。

---

## 提交流程 / Pull Request Workflow

1. Fork 本仓库并从 `main` 创建特性分支（`feat/...`、`fix/...`）。
2. 保持改动聚焦，并为新增逻辑补充或更新测试（UI 交互可通过 `github.com/egoist/mygo/ui.NewTester` 编写无头测试）。
3. 提交前确认 `go vet -buildvcs=false -unsafeptr=false ./...`、`.\build.ps1 test` 与 `.\build.ps1 test -CoreOnly` 全部通过。
4. 填写 PR 模板中的变更说明与验证记录。

---

## 开源协议 / License Agreement

本项目采用 **GNU Affero General Public License v3.0 (AGPL-3.0)** 开源协议，详见 [LICENSE](LICENSE)。提交 Pull Request 即表示您同意您的贡献以相同协议发布。
