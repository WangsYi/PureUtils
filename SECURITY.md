# Security Policy / 安全政策

## Supported Versions / 支持版本

| Version / 版本 | Supported / 支持状态 |
|---|---|
| `1.0.x` (`main`) | ✅ Supported / 正在维护 |

## Reporting a Vulnerability / 报告安全漏洞

如果您在 **PureUtils**（包括剪贴板隔离存储、压缩包解析/解压路径穿越防护、热键监听或文件关联等模块）中发现安全漏洞，请**勿**直接在公开 Issue 中披露利用细节。

If you discover a security vulnerability in **PureUtils** (including clipboard storage, archive path traversal protection, hotkey handling, or file associations), please **do not** disclose exploit details in a public issue.

请优先通过以下方式私密反馈 / Please report privately via:

1. **GitHub Security Advisories**: <https://github.com/WangsYi/PureUtils/security/advisories/new>
2. 若无法使用 Security Advisory，请在 [GitHub Issues](https://github.com/WangsYi/PureUtils/issues) 提交不含漏洞细节的简短联系请求，维护者会与您私下对接。

报告时请尽量附带以下信息 / Please include if possible:
- 问题影响的组件（PureShot / PureClip / PureImage / PureZip / Host / pkg/purekit）与版本号；
- 复现步骤、示例文件（如有）及潜在影响说明。

我们会在确认后尽快发布修复版本并在 Release Notes 中致谢（除非报告者要求匿名）。
