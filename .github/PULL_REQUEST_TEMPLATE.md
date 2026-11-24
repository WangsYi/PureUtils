## 变更摘要 / Summary

<!-- 简要描述本次改动的内容与动机 / Briefly describe what this PR changes and why -->

## 相关组件 / Affected Scope

- [ ] `pkg/purekit`
- [ ] `internal/host` (Host / Launcher / Settings / Tray / Hotkeys)
- [ ] `plugins/pureshot`
- [ ] `plugins/pureclip`
- [ ] `plugins/pureimage`
- [ ] `plugins/purezip`
- [ ] Docs / Build / Installer

## 检查清单 / Checklist

- [ ] 本地已通过 / Passed `go vet -buildvcs=false -unsafeptr=false ./...`
- [ ] 本地已通过 / Passed `.\build.ps1 test`（含架构依赖边界检查）
- [ ] 本地已通过 / Passed `.\build.ps1 test -CoreOnly`
- [ ] 未破坏跨插件零耦合原则（`plugins/*` 间零互相依赖，`pkg/purekit` 零反向依赖）
- [ ] 如涉及新增配置、快捷键或插件，已同步更新中英文文档
- [ ] 确认本次贡献遵循 GNU AGPL-3.0 协议 / Contribution complies with GNU AGPL-3.0
