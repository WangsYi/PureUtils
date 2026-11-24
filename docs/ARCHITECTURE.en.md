# PureUtils Pluggable Desktop Architecture (Go 1.27 + MyGo Native GPU UI)

> [简体中文](ARCHITECTURE.md) | **English** | [Back to README](../README.en.md)

PureUtils is structured around a reusable core SDK (**`pkg/purekit`**), four completely decoupled built-in plugins (**`plugins/*`**), and a thin host shell (**`internal/host`**) coordinating the system tray, global hotkeys, unified Settings window, and the **`Alt+Space` Global Plugin Launcher**. The entire stack is **100% Pure Go (`CGO_ENABLED=0`) + MyGo native GPU UI** — no WebView, no JavaScript, no cgo.

---

## 1. Four-Layer Architecture & Strict Dependency Boundaries

```text
main.go                       MyGo single-instance entry, read-only config/modules load, CLI routing
  └─ internal/host            Thin host shell (Host impl, dynamic tray, hotkeys, unified Settings, Alt+Space Launcher)
       ├─ builtin_*.go        Conditional build bridges (!nopureshot / !nopureclip / !nopureimage&&!nofreeimage / !nopurezip)
       └─ plugins/<id>        Self-contained feature plugins (zero cross-plugin imports, zero internal/* imports)
            ├─ pureshot       PureShot: region/screen capture, 9 vector annotation tools (pureshot/annotate), floating pins
            ├─ pureclip       PureClip: clipboard watcher, SQLite/Blob store, Ditto-style popup, read-only legacy fallback
            ├─ pureimage      PureImage: 40+ image/RAW/animated codecs, TurboJPEG scaling, Spacebar preview, HKCU associations
            └─ purezip        PureZip: archive/ pure-Go engine, zipshell/ Explorer context menu & associations, UI manager
  └─ pkg/purekit              Reusable plugin platform SDK (never imports internal/* or plugins/*)
       ├─ purekit             Plugin / Host / Feature / Matcher / SettingsTab / Bus / Typed Contracts
       ├─ purekit/core        Paths / Logger / Command / Hotkey / ModuleConfig / ClipboardContent / Geometry / Color
       ├─ purekit/config      settings.json v2 Manager (plugins.<id> sections) + read-only v1 fallback
       ├─ purekit/storage     Isolated plugin storage (plugins/<id>/*.db & blobs/) + strict read-only legacy probe
       ├─ purekit/clipboard   Source-tagged multi-consumer clipboard bus (WriteImage / Subscribe / self-write suppression)
       ├─ purekit/platform    Win32 GDI/HWND/clipboard/low-level hook/hotkey recorder/Explorer COM/startup helpers
       └─ purekit/ui          Unified Obsidian/Porcelain design tokens, SVG icons & shared Settings components
```

Dependency boundaries are automatically enforced on every test run by [`internal/host/arch_test.go`](../internal/host/arch_test.go) and [`pkg/purekit/purekit_test.go`](../pkg/purekit/purekit_test.go) using Go's AST parser (`go/parser`):
1. `pkg/purekit/...` must **never** import `pureutils/internal/...` or `pureutils/plugins/...`.
2. Any `plugins/<name>/...` must **never** import `pureutils/internal/...` or any sibling plugin `pureutils/plugins/<other>/...`.
3. `internal/host/*.go` (including all `*_test.go`) must **never** directly import `pureutils/plugins/...`, except for the conditional `builtin_*.go` glue files.

---

## 2. Three-Layer Modularity: Compile-Time, Install-Time & Runtime Hot-Toggle

1. **Compile Time (Go Build Tags)**:
   - `nofreeimage` / `nopureimage`: strips PureImage codecs (`avif`, `heic`, `jxl`, `webp`, `wazero`) and optional `turbojpeg.dll` usage (~10 MB binary reduction);
   - `nopureshot`, `nopureclip`, `nopurezip`: individually omit capture, clipboard history, or archive functionality;
   - Controlled conveniently via `.\build.ps1 build -CoreOnly` or `.\build.ps1 build -Exclude purezip,pureimage`.
2. **Install Time (`modules.json`)**:
   - The Inno Setup installer and portable builds write `{capture, history, viewer, zip}` into `modules.json`; the host starts `CompiledModules() ∩ modules.json`.
3. **Runtime Hot-Toggle (`HostConfig.DisabledModules`)**:
   - Users can toggle individual plugins live inside **Settings → General**. The host calls `applyPluginModules(hc)` to start/stop affected plugins cleanly, refreshes tray menus, global hotkeys, and launcher providers, and relies on `Bus.Version()` capability generation counters so plugins automatically re-subscribe when a sibling provider comes back online.

---

## 3. Zero-Coupling Inter-Plugin Communication: Capability Bus

Plugins have **zero knowledge of sibling Go types**. All cross-plugin workflows flow through strongly typed `pkg/purekit` Capability Bus contracts:

| Capability | Bus Key | Provider | Consumers | Graceful Degradation (When Provider Disabled) |
|---|---|---|---|---|
| `CapClipboardHistory` | `clipboard.history` | PureClip | PureShot (`PinClipboard` sequential queue) | Falls back to pinning current OS clipboard text/image directly |
| `CapImagePreview` | `image.preview` | PureImage | PureClip / PureShot (open in PureImage), PureZip (double-click preview) | Action button/menu item hides; PureZip falls back to OS default viewer |
| `CapImagePin` | `image.pin` | PureShot | PureImage (pin image `P`), PureClip (pin entry) | Action button/menu item hides cleanly |
| `host.Clipboard()` | `clipboard.Service` | Host | PureShot / PureImage / PureZip writers; PureClip subscriber | Writers continue working independently; `Source` tags map display origins automatically |

---

## 4. Global Context-Aware Launcher (`Alt+Space` / `--launcher`)

[`internal/host/launcher.go`](../internal/host/launcher.go) implements a fast frameless `Panel` window:
- **Context Capture**: Captures foreground Windows Explorer selected files (`platform.ExplorerSelectedFiles`), active window process, and clipboard content (`host.Clipboard().Read()`), with a one-click `Clear Context` toggle.
- **Dual-Channel Ranking (`QueryLauncher`)**: Combines contextual `Plugin.Matchers()` (e.g. `.zip` smart extract, image quick view, live PureClip SQLite search) with keyword/subsequence scoring across `Plugin.Features()` and built-in host actions.
- **Close-Before-Run**: Closes the launcher panel before dispatching the chosen action so region screenshots and paste operations never target the launcher overlay itself.

---

## 5. Storage Isolation & Read-Only Legacy Compatibility

User data lives in `%LOCALAPPDATA%\PureUtils` (`PUREUTILS_HOME` override supported):
- **`settings.json` (v2)**: Top-level host settings (`schemaVersion`, `hotkeys`, `appearance`, `advanced`, `launcher`, `disabledModules`) plus isolated `plugins.<pluginID>` namespaces. Legacy v1 files are mapped read-only in memory at startup and only migrated when the user explicitly clicks Save.
- **Per-Plugin DBs & Blobs**: New writes go exclusively to `plugins\pureclip\clip.db`, `plugins\pureshot\shots.db`, and SHA-1 sharded `blobs\`. Legacy `pureutils.db`, `shotclip.db`, and `images\` are opened strictly in read-only mode (`?mode=ro&immutable=1`, zero `-wal`/`-shm` sidecars) and never modified or migrated destructively.

---

## 6. Threading & Lifecycle Model

- **Main UI Thread**: `mygo.App.Run()` owns the OS UI thread; background workers marshal UI state updates via `mygo.RunOnMain` (executed synchronously under `Headless: true` in unit tests).
- **Global Hook Arbitration**: Full-screen overlay and hotkey recording declare suppression via `host.SetGlobalHookSuppressed(source, on)` and `Plugin.IsBusyForGlobalHooks()`, preventing PureImage Spacebar hooks or `Win+` key capture conflicts.
- **Shutdown Order**: Unregister hotkeys -> close host windows -> stop plugins in reverse init order (`Plugin.Stop()`) -> close shared clipboard service -> destroy tray -> flush `PerfReport()`.

---

## 7. Authoring a New Built-in Plugin

1. Create `plugins/<myplugin>/` importing only `pureutils/pkg/purekit/...`, `github.com/egoist/mygo`, and the Go standard library (never `internal/*` or sibling `plugins/*`).
2. Implement `purekit.Plugin` (`ID`, `Info`, `Init`, `Start`, `Stop`, `Features`, `Matchers`, `TrayItems`, `Hotkeys`, `CLICommands`, `SettingsTab`).
3. Store config via `host.Config("<myplugin>")` and files/SQLite via `host.Storage("<myplugin>")`; expose optional cross-plugin capabilities in `pkg/purekit/contracts.go` via `host.Bus()`.
4. Add conditional build bridges `internal/host/builtin_<myplugin>.go` (`//go:build !no<myplugin>`) and `builtin_<myplugin>_disabled.go` (`//go:build no<myplugin>`), and register it in `internal/host/builtin.go`.
5. Run `go vet -buildvcs=false -unsafeptr=false ./...` and `.\build.ps1 test` to verify AST dependency compliance and headless UI tests.
