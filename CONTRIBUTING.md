# 维护与贡献政策 / Maintenance & Contribution Policy

**PureUtils** 目前由作者个人独立维护，**暂不接受外部 Pull Request（PR）或外部代码贡献**，GitHub 仓库已关闭 Pull Requests 功能。

**PureUtils** is currently maintained solely by the author and **does not accept external Pull Requests (PRs) or code contributions**. Pull Requests are disabled on this GitHub repository.

---

## 问题反馈与自用派生 / Issues & Forks

- **问题反馈 / Bug Reports**：欢迎通过 GitHub **Issues** 提交 Bug 报告、崩溃日志或功能建议。
- **自由派生 / Forks**：欢迎依据 [**GNU Affero General Public License v3.0 (AGPL-3.0)**](LICENSE) 条款 Fork 本仓库、在本地查阅或自行从源码构建修改版。

---

## 本地构建与验证 / Local Build & Verification

- **OS**: Windows 10 / Windows 11 (x64)
- **Go**: **1.27.3+**（`CGO_ENABLED=0`，零 cgo / 零 WebView / 零 Node）

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

## 开源协议 / License

本项目采用 **GNU Affero General Public License v3.0 (AGPL-3.0)** 开源发布，详见 [LICENSE](LICENSE)。
