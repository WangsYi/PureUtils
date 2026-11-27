# PureUtils 可插拔桌面平台架构（Go 1.27 + MyGo 原生 GPU UI）

> **简体中文** | [English](ARCHITECTURE.en.md) | [返回 README](../README.md)

PureUtils 已从单体桌面工具演进为以 **`pkg/purekit` 可复用核心库**为底座、以 **`plugins/*` 完全解耦内建插件**为能力单元、以 **`internal/host` 薄宿主外壳**装配托盘/全局热键/设置中心、应用内自动更新（[`updater.go`](../internal/host/updater.go)）与 **全局启动器（`Alt+Space`）**的模块化平台。全栈保持 **100% 纯 Go (`CGO_ENABLED=0`) + MyGo 原生 GPU UI**，无 WebView、无 JavaScript、无 cgo。

---

## 1. 四层架构与严格依赖边界

```text
main.go                       MyGo 单实例生命周期、crash.log 崩溃日志挂载、config/modules 读取、CLI 路由
  └─ internal/host            薄宿主外壳（Host 实现、动态托盘、热键调度、统一设置、自动更新、Alt+Space 启动器）
       ├─ builtin_*.go        条件编译注册桥（!nopureshot / !nopureclip / !nopureimage&&!nofreeimage / !nopurevideo / !nopurezip / !nopuremonitor）
       └─ plugins/<id>        完全独立的功能插件（零跨插件 import、零 internal/* import）
            ├─ pureshot       PureShot：区域/整屏截图、9 种矢量标注（pureshot/annotate）、悬浮贴图
            ├─ pureclip       PureClip：剪贴板历史监控、SQLite/Blob 仓储、Ditto 风格浮窗、只读 legacy fallback
            ├─ pureimage      PureImage：40+ 格式与 RAW/动图解码、TurboJPEG、空格预览、系统打开方式关联
            ├─ purevideo      PureVideo：内置裁剪版 libmpv 硬件解码、21 种视频格式、字幕与系统打开方式关联
            ├─ purezip        PureZip：archive/ 纯 Go 压缩引擎、zipshell/ 右键与关联、管理器与进度窗口
            └─ puremonitor    PureMonitor：任务栏/边缘胶囊监控、ADL/NVML/PawnIO 传感器及 ETW 游戏帧率 HUD
  └─ pkg/purekit              可抽离复用的插件平台核心 SDK（不依赖 internal/* 或 plugins/*）
       ├─ purekit             Plugin / Host / Feature / Matcher / SettingsTab / Bus / Typed Contracts
       ├─ purekit/core        Paths / Logger / SetupCrashOutput / Version / Command / Hotkey / ModuleConfig / Color
       ├─ purekit/config      settings.json v2 Manager（plugins.<id> 分区）+ v1 顶层键只读回退
       ├─ purekit/storage     隔离插件存储（plugins/<id>/*.db / blobs）+ legacy DB/images 严格只读探测
       ├─ purekit/clipboard   多消费者带来源标签剪贴板总线（WriteImage / Subscribe / 自写抑制）
       ├─ purekit/platform    Win32 GDI/HWND/剪贴板/热键录制/Explorer COM 选中项/开机自启/任务栏身份
       └─ purekit/ui          统一 Obsidian/Porcelain 设计 token、图标 SVG 与设置卡片组件
```

依赖合法性由 [`internal/host/arch_test.go`](../internal/host/arch_test.go) 使用 Go AST (`go/parser`) 自动强制校验：
1. `pkg/purekit/...` 严禁 import `pureutils/internal/...` 或 `pureutils/plugins/...`；
2. 每个 `plugins/<name>/...` 严禁 import `pureutils/internal/...` 或任何兄弟插件 `pureutils/plugins/<other>/...`；
3. `internal/host/*.go`（含所有 `*_test.go`）严禁直接 import `pureutils/plugins/...`，唯一例外是 `builtin_*.go` 条件编译胶水文件；
4. 旧单体目录（`internal/{app,viewer,archive,zipshell,store,annotate,core,win32}`）已全部移除。

---

## 2. 三层模块化：编译期裁剪 + 安装期选择 + 运行时热开关

PureUtils 支持三层自由组合与裁剪：

1. **编译期（Go Build Tags）**：
   - `nofreeimage`（等价于 `nopureimage`）：完全排除 PureImage 及 AVIF/HEIC/JXL/WebP 解码与 `wazero` 依赖（体积缩减约 10 MB）；
   - `nopureshot`：排除 PureShot 截图、标注与贴图；
   - `nopureclip`：排除 PureClip 剪贴板历史；
   - `nopurevideo`：排除 PureVideo 视频浏览器及 `libmpv` 调用；
   - `nopurezip`：排除 PureZip 压缩包引擎与管理器；
   - `nopuremonitor`：排除 PureMonitor 硬件监控与帧率 HUD；
   - 六个全部排除（`-tags "nopureshot nopureclip nopureimage nopurevideo nopurezip nopuremonitor"`）仍可编出纯宿主外壳。
   - 也可通过 `.\build.ps1 build -CoreOnly` 或 `.\build.ps1 build -Exclude purezip,pureimage,purevideo` 一键构建。
2. **安装期（`modules.json`）**：
   - 安装包或绿色版目录的 `modules.json` 控制 `{capture, history, viewer, video, zip, monitor}` 开关，宿主按 `CompiledModules() ∩ modules.json` 决定启动装配集合。
3. **运行时（设置页实时热开关 / `DisabledModules`）**：
   - 用户可在「设置 → 常规」随时开启或关闭已编译模块；宿主通过 `applyPluginModules(hc)` 实时启停对应插件并重建托盘、热键与启动器条目，配合 `Bus.Version()` 能力代次递增，使其他插件（如 PureShot / PureClip / PureZip）在提供方恢复后自动重新绑定能力；「设置 → 高级」同时集成自动更新状态检查、镜像切换与日志诊断（含 `debug.SetCrashOutput` 写入的 `logs/crash.log`）。

---

## 3. 跨插件零耦合通信：Capability Bus 与剪贴板总线

当多个插件共存时，它们**彼此不知道对方 Go 类型**，所有协作全部通过 `pkg/purekit` 强类型契约完成：

| 机制 | 契约 / 接口 | 提供方 | 消费方 | 降级行为（提供方未启用时） |
|---|---|---|---|---|
| `CapClipboardHistory` | `clipboard.history` | PureClip | PureShot（选区前后历史贴图队列 `PinClipboard`） | 回退为直接读取当前系统剪贴板文本/图片 |
| `CapImagePreview` | `image.preview` | PureImage | PureClip / PureShot（右键查看图片）、PureZip（双击包内图片预览） | 右键/按钮隐藏，PureZip 回退系统默认程序打开 |
| `CapImagePin` | `image.pin` | PureShot | PureImage（一键贴图）、PureClip（条目「贴成图片」） | 右键/快捷按钮自动隐藏 |
| `CapVideoPreview` | `video.preview` | PureVideo | Host 启动器 / 裸路径调度 | 自动回退由系统默认播放器打开 |
| `host.Clipboard()` | `clipboard.Service` | Host | PureShot / PureImage / PureZip 写入并标明来源；PureClip 订阅事件 | 插件可正常写系统剪贴板，`Source=="pureshot"` 自动映射为来源 `PureShot` |

此外，在打开命令行裸路径（如 `PureUtils.exe foo.zip`）时，宿主通过 `Plugin.Matchers()` 识别 `purezip.open` 匹配结果路由至 `CmdOpenZip`，完全无需硬编码引入 `purezip/archive`。

---

## 4. 全局插件启动器（`Alt+Space` / `--launcher`）

[`internal/host/launcher.go`](../internal/host/launcher.go) 提供基于 MyGo `Panel` 窗口的轻量全局启动器：
- **上下文捕获**：唤起瞬间自动采集前景 Windows 资源管理器选中文件（`platform.ExplorerSelectedFiles`）、当前前景进程及当前剪贴板内容（`host.Clipboard().Read()`），并支持点击「清除上下文」切换为纯关键词检索。
- **双通道聚合检索 (`QueryLauncher`)**：
  1. **动态上下文匹配 (`Plugin.Matchers()`)**：例如选中 `.zip` 时优先推荐「用 PureZip 打开 / 智能解压 / 压缩」、选中图片时推荐 PureImage 查看与 PureShot 贴图、剪贴板有图时推荐查看/贴图，输入关键词时还能穿透查询 PureClip SQLite 历史（当输入非当前上下文关键词如 `"clip"` 时自动过滤无关选中文件推荐）。
  2. **静态功能检索 (`Plugin.Features()` + Host 内建命令)**：支持标题、关键词、副标题多词前缀/子串/有序子序列打分并按插件 + 标题去重排序。
- **即关即执行**：按下 `Enter` 或 `Ctrl+1–9` 时先关闭启动器窗口再派发动作，确保截图遮罩或粘贴等对屏幕/焦点敏感的操作不会采到启动器本身。

---

## 5. 存储隔离与只读 Legacy 兼容策略

用户数据目录仍位于 `%LOCALAPPDATA%\PureUtils`（可用 `PUREUTILS_HOME` 覆盖）：

- **配置文件（`settings.json` v2）**：
  - 顶层保留宿主配置（`schemaVersion`, `hotkeys`, `appearance`, `advanced`, `launcher`）及各插件独立命名空间 `plugins.<pluginID>`。
  - 若磁盘仍为旧版 v1（顶层含 `capture` / `pins` / `history` / `viewer` / `zip`），读取阶段在内存中只读合成对应 `Section`，**启动过程绝不重写或修改旧版 `settings.json`**；仅在用户显式点击设置「保存」后写入 v2 结构（并保留未知键）。
- **数据库与 Blob 存储**：
  - 新数据写入隔离路径 `<Home>\plugins\pureclip\clip.db`、`plugins\pureshot\shots.db` 及内容寻址 `blobs\ab\...png`；
  - 若存在旧版 `<Home>\pureutils.db` / `shotclip.db` / `images\`，仅以 SQLite只读模式（`?mode=ro&immutable=1` 或只读探测，绝不产生 `-wal`/`-shm` 副作用，绝不迁移改写原文件）挂载为只读兜底历史（以负 ID 区分，禁止修改或删除）。
- **Windows 原子保存硬化**：`config.Manager` 在 `os.Rename(tmp, settings.json)` 遇到 Windows Defender / 索引服务短暂 `Access is denied` 时自动进行短间隔退避重试。

---

## 6. 线程与生命周期模型

- **主线程 UI**：`mygo.App.Run()` 运行在 OS 主线程，托盘（`tray.go`）、设置窗（`settings_window.go`）、启动器（`launcher.go`）及各插件窗口状态均通过 `mygo.RunOnMain` 在主线程访问（`Headless: true` 单测可直接同步执行）。
- **全局钩子仲裁**：任何插件或宿主自身（截图全屏遮罩、`StartHotkeyRecorder()` 热键录制）通过 `host.SetGlobalHookSuppressed(source, true/false)` 与 `Plugin.IsBusyForGlobalHooks()` 声明占用，PureImage 空格预览钩子据此自动避让。
- **退出策略 (`ShouldStayInTray`)**：任一已激活插件声明 `ShouldStayInTray() == true`（如 PureShot、PureClip，或开启了空格预览的 PureImage）时关闭窗口保留托盘；全部窗口关闭且无插件需驻留时调用 `NotifyWindowClosed()` 自动退出进程。
- **逆序清理 (`Shutdown`)**：注销全局热键 -> 关闭宿主窗 -> 按初始化逆序调用 `Plugin.Stop()`（释放各自总线能力、窗口与 SQLite 连接） -> 关闭共享 `Clipboard()` -> 销毁托盘 -> 输出 `PerfReport()` 日志。

---

## 7. 新增一个内建/可复用插件的步骤

1. **新建包 `plugins/<myplugin>/`**：仅 import `pureutils/pkg/purekit/...`、`github.com/egoist/mygo` 与标准库，绝不 import `internal/*` 或其他 `plugins/*`；
2. **实现 `purekit.Plugin` 接口**：定义 `ID()`、`Info()`、`Init(host)` / `Start()` / `Stop()`、可搜索 `Features()`、上下文 `Matchers()`、托盘 `TrayItems()`、热键 `Hotkeys()`、命令行 `CLICommands()` 以及自渲染 `SettingsTab()`；
3. **注册配置与存储**：通过 `host.Config("<myplugin>")` 与 `host.Storage("<myplugin>")` 隔离读写配置、DB 与二进制文件；若需跨插件能力，在 `pkg/purekit/contracts.go` 声明契约并通过 `host.Bus()` 注册或调用；
4. **接入条件编译桥**：在 `internal/host/` 新增 `builtin_<myplugin>.go`（`//go:build !no<myplugin>`）与 `builtin_<myplugin>_disabled.go`（`//go:build no<myplugin>`）并在 `builtin.go` 加入列表；
5. **运行边界与单测**：执行 `go vet -unsafeptr=false ./...` 与 `.\build.ps1 test`，`arch_test.go` 将自动校验模块边界合规性。
