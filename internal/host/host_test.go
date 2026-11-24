package host

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/platform"
)

func testHostPaths(t *testing.T) core.Paths {
	t.Helper()
	dir := t.TempDir()
	paths := core.Paths{
		Home:     dir,
		DB:       filepath.Join(dir, "pureutils.db"),
		Settings: filepath.Join(dir, "settings.json"),
		Images:   filepath.Join(dir, "images"),
		Logs:     filepath.Join(dir, "logs"),
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	return paths
}

func testHostWithModules(t *testing.T, mods core.ModuleConfig, clip clipboard.Service, quitFn func()) *App {
	t.Helper()
	paths := testHostPaths(t)
	logger, err := core.OpenLogger(paths.Logs, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logger.Close() })

	cfgMgr := config.Load(paths.Settings)
	app, err := NewApp(Options{
		Paths:     paths,
		Config:    cfgMgr,
		Modules:   mods,
		Logger:    logger,
		Headless:  true,
		Clipboard: clip,
		QuitFunc:  quitFn,
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { app.Shutdown() })
	return app
}

func findMenuItem(items []*mygo.MenuItem, prefix string) *mygo.MenuItem {
	for _, it := range items {
		if strings.HasPrefix(it.Label, prefix) {
			return it
		}
	}
	return nil
}

func TestTrayMenuToggleCheckboxes(t *testing.T) {
	compiled := CompiledModules()
	a := testHostWithModules(t, compiled, nil, nil)
	a.Start([]string{"--background"})

	items := a.buildTrayMenu().Items()
	for _, it := range items {
		if strings.Contains(it.Label, "隐藏/显示") || strings.Contains(it.Label, "暂停/恢复") {
			t.Fatalf("unexpected ambiguous tray label %q", it.Label)
		}
	}

	if compiled.Capture {
		pinItem := findMenuItem(items, "显示贴图")
		if pinItem == nil || pinItem.Label != "显示贴图" {
			t.Fatalf("expected 显示贴图 item, got %+v", pinItem)
		}
		if pinItem.Type != mygo.MenuItemCheckbox || pinItem.Checked || !pinItem.Disabled {
			t.Fatalf("zero pins 显示贴图 = %+v, want checkbox Checked=false Disabled=true", pinItem)
		}

		ptItem := findMenuItem(items, "贴图鼠标穿透")
		if ptItem == nil || ptItem.Label != "贴图鼠标穿透" {
			t.Fatalf("expected 贴图鼠标穿透 item, got %+v", ptItem)
		}
		if ptItem.Type != mygo.MenuItemCheckbox || ptItem.Checked || !ptItem.Disabled {
			t.Fatalf("zero pins 贴图鼠标穿透 = %+v, want checkbox Checked=false Disabled=true", ptItem)
		}

		// Create 2 pins via Capability Bus without importing plugins/pureshot.
		if err := purekit.CallImagePinText(context.Background(), a.Bus(), "pin-one", 0); err != nil {
			t.Fatalf("pin 1: %v", err)
		}
		if err := purekit.CallImagePinText(context.Background(), a.Bus(), "pin-two", 1); err != nil {
			t.Fatalf("pin 2: %v", err)
		}
		items = a.buildTrayMenu().Items()
		pinItem = findMenuItem(items, "显示贴图")
		if pinItem == nil || pinItem.Label != "显示贴图 (2)" || !pinItem.Checked || pinItem.Disabled {
			t.Fatalf("2 visible pins 显示贴图 = %+v, want '显示贴图 (2)' Checked=true Disabled=false", pinItem)
		}
		ptItem = findMenuItem(items, "贴图鼠标穿透")
		if ptItem == nil || ptItem.Label != "贴图鼠标穿透" || ptItem.Checked || ptItem.Disabled {
			t.Fatalf("0 passthrough pins 贴图鼠标穿透 = %+v", ptItem)
		}

		a.ExecuteCommand(core.CmdTogglePins)
		items = a.buildTrayMenu().Items()
		pinItem = findMenuItem(items, "显示贴图")
		if pinItem == nil || pinItem.Label != "显示贴图 (2)" || pinItem.Checked || pinItem.Disabled {
			t.Fatalf("hidden pins 显示贴图 = %+v, want Checked=false", pinItem)
		}

		a.ExecuteCommand(core.CmdTogglePinsPassthrough)
		items = a.buildTrayMenu().Items()
		ptItem = findMenuItem(items, "贴图鼠标穿透")
		if ptItem == nil || !ptItem.Checked || ptItem.Disabled {
			t.Fatalf("enabled passthrough 贴图鼠标穿透 = %+v, want Checked=true", ptItem)
		}

		a.ExecuteCommand(core.CmdTogglePinsPassthrough)
		items = a.buildTrayMenu().Items()
		ptItem = findMenuItem(items, "贴图鼠标穿透")
		if ptItem == nil || ptItem.Checked || ptItem.Disabled {
			t.Fatalf("cleared passthrough 贴图鼠标穿透 = %+v, want Checked=false", ptItem)
		}
	}

	if compiled.History {
		monItem := findMenuItem(items, "剪贴板监控")
		if monItem == nil || monItem.Type != mygo.MenuItemCheckbox || !monItem.Checked {
			t.Fatalf("expected 剪贴板监控 checked after Start, got %+v", monItem)
		}
		a.ExecuteCommand(core.CmdToggleMonitoring)
		monItem = findMenuItem(a.buildTrayMenu().Items(), "剪贴板监控")
		if monItem == nil || monItem.Checked {
			t.Fatalf("expected 剪贴板监控 unchecked after toggle, got %+v", monItem)
		}
	}

	if compiled.Viewer {
		spItem := findMenuItem(items, "资源管理器空格预览")
		if spItem == nil || spItem.Type != mygo.MenuItemCheckbox || spItem.Checked {
			t.Fatalf("expected 资源管理器空格预览 unchecked initially, got %+v", spItem)
		}
		a.ExecuteCommand(core.CmdToggleSpacePreview)
		spItem = findMenuItem(a.buildTrayMenu().Items(), "资源管理器空格预览")
		if spItem == nil || !spItem.Checked {
			t.Fatalf("expected 资源管理器空格预览 checked after toggle, got %+v", spItem)
		}
	}
}

func TestShouldStayInTrayByModuleConfig(t *testing.T) {
	compiled := CompiledModules()

	if compiled.Capture {
		aCap := testHostWithModules(t, core.ModuleConfig{Capture: true}, nil, nil)
		if !aCap.ShouldStayInTray() {
			t.Error("PureShot alone should stay in tray")
		}
	}

	if compiled.History {
		aHist := testHostWithModules(t, core.ModuleConfig{History: true}, nil, nil)
		if !aHist.ShouldStayInTray() {
			t.Error("PureClip alone should stay in tray")
		}
	}

	if compiled.Viewer {
		quitCalled := 0
		aView := testHostWithModules(t, core.ModuleConfig{Viewer: true}, nil, func() { quitCalled++ })
		if aView.ShouldStayInTray() {
			t.Error("PureImage-only without SpacePreview should not stay in tray")
		}
		aView.NotifyWindowClosed()
		if quitCalled != 1 {
			t.Fatalf("expected QuitFunc on window close with no tray need, got %d", quitCalled)
		}
		aView.ExecuteCommand(core.CmdToggleSpacePreview)
		if !aView.ShouldStayInTray() {
			t.Error("PureImage-only with SpacePreview=true should stay in tray")
		}
	}

	if compiled.Zip {
		aZip := testHostWithModules(t, core.ModuleConfig{Zip: true}, nil, nil)
		if aZip.ShouldStayInTray() {
			t.Error("PureZip-only should not stay in tray")
		}
	}

	emptyQuit := 0
	aNone := testHostWithModules(t, core.ModuleConfig{}, nil, func() { emptyQuit++ })
	if aNone.ShouldStayInTray() {
		t.Error("0-plugin host should not stay in tray")
	}
	aNone.NotifyWindowClosed()
	if emptyQuit != 1 {
		t.Fatalf("expected QuitFunc on 0-plugin host, got %d", emptyQuit)
	}
}

func TestSettingsWindowUnifiedTabsAndPersistence(t *testing.T) {
	compiled := CompiledModules()
	a := testHostWithModules(t, compiled, nil, nil)
	v := newSettingsView(a, "")
	tt := ui.NewTester(v.view, 860, 640)
	tt.Frame()

	if !tt.HasText("常规与外观") || !tt.HasText("全局热键") {
		t.Fatalf("general tab did not render: %v", tt.Texts())
	}

	// Theme switch dirty & discard
	if v.isDirty() {
		t.Fatal("expected clean state initially")
	}
	if err := tt.Click("浅色"); err != nil {
		t.Fatalf("click 浅色: %v", err)
	}
	tt.Frame()
	if !v.isDirty() || v.hostDraft.Appearance.Theme != "light" {
		t.Fatalf("expected dirty theme=light, got %q dirty=%v", v.hostDraft.Appearance.Theme, v.isDirty())
	}
	if err := tt.Click("放弃更改"); err != nil {
		t.Fatalf("click 放弃更改: %v", err)
	}
	tt.Frame()
	if v.isDirty() || v.hostDraft.Appearance.Theme != "system" {
		t.Fatalf("expected discard to restore system, got %q dirty=%v", v.hostDraft.Appearance.Theme, v.isDirty())
	}

	// Dynamic sidebar tabs & hotkey validation if PureShot compiled
	if compiled.Capture {
		if err := tt.Click("PureShot · 截图贴图"); err != nil {
			t.Fatalf("click PureShot tab: %v", err)
		}
		tt.Frame()
		if !tt.HasText("保存目录") {
			t.Fatalf("PureShot tab missing 保存目录: %v", tt.Texts())
		}
		if err := tt.Click("常规"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		shotIdx := -1
		for i, hk := range v.hotkeys {
			if hk.spec.ID == "pureshot.region" {
				shotIdx = i
				break
			}
		}
		if shotIdx < 0 {
			t.Fatal("missing pureshot.region hotkey entry")
		}
		orig := a.HostConfig().Hotkeys.CaptureRegion
		v.hotkeys[shotIdx].draft = "Bad+Key+++"
		tt.Frame()
		if v.hotkeyError(shotIdx) == "" {
			t.Fatal("expected hotkey validation error")
		}
		if err := tt.Click("保存"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.HostConfig().Hotkeys.CaptureRegion != orig {
			t.Fatalf("invalid hotkey overwrote saved value: %q", a.HostConfig().Hotkeys.CaptureRegion)
		}
		if err := tt.Click("恢复默认区域截图"); err != nil {
			t.Fatalf("restore default hotkey: %v", err)
		}
		tt.Frame()
		if v.hotkeys[shotIdx].draft != "F1" {
			t.Fatalf("restored hotkey = %q, want F1", v.hotkeys[shotIdx].draft)
		}
	}

	// Persistence through plugin SettingsTabView
	if compiled.Viewer {
		if err := tt.Click("PureImage · 看图"); err != nil {
			t.Fatalf("click PureImage tab: %v", err)
		}
		tt.Frame()
		tt.Scroll(450, 300, 0, 300)
		tt.Frame()
		if err := tt.Click("开关：资源管理器空格预览"); err != nil {
			t.Fatalf("toggle SpacePreview: %v", err)
		}
		tt.Frame()
		if err := tt.Click("保存"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		reloaded := config.LoadPureImage(config.Load(a.Paths().Settings).Section(config.PluginPureImage))
		if !reloaded.SpacePreview {
			t.Fatal("expected PureImage SpacePreview=true persisted")
		}
	} else if compiled.History {
		if err := tt.Click("PureClip · 剪贴板"); err != nil {
			t.Fatalf("click PureClip tab: %v", err)
		}
		tt.Frame()
		if err := tt.Click("开关：记录图片"); err != nil {
			t.Fatalf("toggle CaptureImages: %v", err)
		}
		tt.Frame()
		if err := tt.Click("保存"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		reloaded := config.LoadPureClip(config.Load(a.Paths().Settings).Section(config.PluginPureClip))
		if reloaded.History.CaptureImages {
			t.Fatal("expected PureClip CaptureImages=false persisted")
		}
	}

	// Advanced tab renders module matrix
	if err := tt.Click("高级"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("已安装模块") || !tt.HasText(a.Paths().Home) {
		t.Fatalf("advanced tab missing module matrix or data dir: %v", tt.Texts())
	}

	// Subset module build hides disabled tabs cleanly
	aCore := testHostWithModules(t, core.ModuleConfig{Capture: compiled.Capture, History: compiled.History}, nil, nil)
	vCore := newSettingsView(aCore, "")
	ttCore := ui.NewTester(vCore.view, 860, 640)
	ttCore.Frame()
	if ttCore.HasText("PureImage · 看图") || ttCore.HasText("PureZip · 压缩") {
		t.Fatalf("disabled modules should hide sidebar tabs: %v", ttCore.Texts())
	}
}

type spyPlugin struct {
	id         string
	mod        core.ModuleID
	matcher    purekit.Matcher
	commands   []core.Command
	onExecute  func(cmd core.Command, paths []string)
}

func (s *spyPlugin) ID() string { return s.id }
func (s *spyPlugin) Info() purekit.PluginInfo {
	return purekit.PluginInfo{ID: s.id, Name: s.id, ModuleID: s.mod}
}
func (s *spyPlugin) Init(purekit.Host) error      { return nil }
func (s *spyPlugin) Start() error                 { return nil }
func (s *spyPlugin) Stop() error                  { return nil }
func (s *spyPlugin) Features() []purekit.Feature  { return nil }
func (s *spyPlugin) SettingsTab() *purekit.SettingsTab {
	return nil
}
func (s *spyPlugin) Hotkeys() []purekit.HotkeySpec     { return nil }
func (s *spyPlugin) TrayItems() []purekit.TrayItemSpec { return nil }
func (s *spyPlugin) HasOpenWindows() bool              { return false }
func (s *spyPlugin) ShouldStayInTray() bool            { return false }
func (s *spyPlugin) IsBusyForGlobalHooks() bool        { return false }
func (s *spyPlugin) Matchers() []purekit.Matcher {
	if s.matcher == nil {
		return nil
	}
	return []purekit.Matcher{s.matcher}
}
func (s *spyPlugin) CLICommands() []purekit.CLICommandSpec {
	out := make([]purekit.CLICommandSpec, 0, len(s.commands))
	for _, cmd := range s.commands {
		c := cmd
		out = append(out, purekit.CLICommandSpec{
			Command:    c,
			TakesPaths: c.TakesPaths(),
			Execute: func(paths []string) {
				if s.onExecute != nil {
					s.onExecute(c, paths)
				}
			},
		})
	}
	return out
}

type archiveSpyMatcher struct{}

func (archiveSpyMatcher) ID() string    { return "purezip.matcher" }
func (archiveSpyMatcher) Title() string { return "zip spy" }
func (archiveSpyMatcher) Match(ctx purekit.MatchContext) []purekit.MatchResult {
	if len(ctx.SelectedFiles) > 0 && strings.HasSuffix(strings.ToLower(ctx.SelectedFiles[0]), ".zip") {
		return []purekit.MatchResult{{FeatureID: "purezip.open", PluginID: "purezip"}}
	}
	return nil
}

func TestBarePathCLIRouting(t *testing.T) {
	compiled := CompiledModules()
	if compiled.Zip {
		a := testHostWithModules(t, compiled, nil, nil)
		if !a.isArchiveByMatchers("bundle.zip") || !a.isArchiveByMatchers("archive.7z") {
			t.Fatal("expected PureZip matcher to recognize archive paths")
		}
		if a.isArchiveByMatchers("photo.png") {
			t.Fatal("unexpected archive match for .png")
		}
	}

	var gotCmd core.Command
	var gotPaths []string
	var mu sync.Mutex
	record := func(cmd core.Command, paths []string) {
		mu.Lock()
		gotCmd = cmd
		gotPaths = append([]string(nil), paths...)
		mu.Unlock()
	}

	paths := testHostPaths(t)
	app, err := NewApp(Options{
		Paths:    paths,
		Config:   config.Load(paths.Settings),
		Headless: true,
		Plugins: []purekit.Plugin{
			&spyPlugin{id: "pureimage", mod: core.ModuleViewer, commands: []core.Command{core.CmdOpenViewer}, onExecute: record},
			&spyPlugin{id: "purezip", mod: core.ModuleZip, matcher: archiveSpyMatcher{}, commands: []core.Command{core.CmdOpenZip}, onExecute: record},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Shutdown()

	app.HandleArgs([]string{"release.zip"})
	if gotCmd != core.CmdOpenZip || len(gotPaths) != 1 || gotPaths[0] != "release.zip" {
		t.Fatalf("archive bare path routed to (%s, %v), want (%s, [release.zip])", gotCmd, gotPaths, core.CmdOpenZip)
	}

	app.HandleArgs([]string{"shot.png"})
	if gotCmd != core.CmdOpenViewer || len(gotPaths) != 1 || gotPaths[0] != "shot.png" {
		t.Fatalf("image bare path routed to (%s, %v), want (%s, [shot.png])", gotCmd, gotPaths, core.CmdOpenViewer)
	}
}

func TestStartupReadOnlyLegacySettings(t *testing.T) {
	paths := testHostPaths(t)
	legacyJSON := `{
  "hotkeys": {
    "captureRegion": "F2",
    "showHistory": "Alt+V"
  },
  "appearance": {
    "theme": "dark",
    "previewHeight": 144
  },
  "capture": {
    "saveFolder": "D:/LegacyShots",
    "includeCursor": true
  },
  "pins": {
    "opacity": 0.72,
    "borderThickness": 4
  },
  "history": {
    "maxEntries": 123,
    "pasteOnEnter": true
  },
  "viewer": {
    "wheelZoom": false,
    "spacePreview": true,
    "language": "en-US"
  },
  "zip": {
    "defaultFormat": "7z",
    "level": 9
  }
}`
	if err := os.WriteFile(paths.Settings, []byte(legacyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeBytes, err := os.ReadFile(paths.Settings)
	if err != nil {
		t.Fatal(err)
	}

	logger, err := core.OpenLogger(paths.Logs, false)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	compiled := CompiledModules()
	cfgMgr := config.Load(paths.Settings)
	app, err := NewApp(Options{
		Paths:    paths,
		Config:   cfgMgr,
		Modules:  compiled,
		Logger:   logger,
		Headless: true,
	})
	if err != nil {
		t.Fatalf("NewApp with legacy settings: %v", err)
	}
	app.Start([]string{"--background"})
	app.Shutdown()

	afterBytes, err := os.ReadFile(paths.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatalf("startup mutated legacy settings.json on disk!\nbefore:\n%s\nafter:\n%s", beforeBytes, afterBytes)
	}

	hc := app.HostConfig()
	if hc.Hotkeys.CaptureRegion != "F2" || hc.Appearance.Theme != "dark" {
		t.Fatalf("unexpected HostConfig loaded from legacy settings: %+v", hc)
	}
	if compiled.Capture {
		psCfg := config.LoadPureShot(app.Config(config.PluginPureShot))
		if psCfg.Capture.SaveFolder != "D:/LegacyShots" || !psCfg.Capture.IncludeCursor || psCfg.Pins.BorderThickness != 4 {
			t.Fatalf("unexpected legacy PureShot config: %+v", psCfg)
		}
	}
	if compiled.History {
		pcCfg := config.LoadPureClip(app.Config(config.PluginPureClip))
		if pcCfg.History.MaxEntries != 123 || pcCfg.PreviewHeight != 144 {
			t.Fatalf("unexpected legacy PureClip config: %+v", pcCfg)
		}
	}
	if compiled.Viewer {
		piCfg := config.LoadPureImage(app.Config(config.PluginPureImage))
		if piCfg.WheelZoom || !piCfg.SpacePreview || piCfg.Language != "en-US" {
			t.Fatalf("unexpected legacy PureImage config: %+v", piCfg)
		}
		if !app.ShouldStayInTray() {
			t.Error("legacy SpacePreview=true should make PureImage stay in tray")
		}
	}
	if compiled.Zip {
		pzCfg := config.LoadPureZip(app.Config(config.PluginPureZip))
		if pzCfg.DefaultFormat != "7z" || pzCfg.Level != 9 {
			t.Fatalf("unexpected legacy PureZip config: %+v", pzCfg)
		}
	}
	_ = time.Millisecond
}

func TestSettingsHotkeyRecordingCallbackAndSave(t *testing.T) {
	a := testHostWithModules(t, CompiledModules(), nil, nil)
	v := newSettingsView(a, "")
	tt := ui.NewTester(v.view, 860, 640)
	tt.Frame()

	launcherIdx := -1
	for i, hk := range v.hotkeys {
		if hk.spec.ID == "host.launcher" {
			launcherIdx = i
			break
		}
	}
	if launcherIdx < 0 {
		t.Fatal("missing host.launcher hotkey entry")
	}

	v.recordingID = "host.launcher"
	v.onRecording(platform.HotkeyResult{Err: "「A」不能单独作热键"})
	tt.Frame()
	if v.recordingID != "host.launcher" || !tt.HasText("「A」不能单独作热键") {
		t.Fatalf("expected recording error displayed while staying active: recordingID=%q texts=%v", v.recordingID, tt.Texts())
	}

	v.onRecording(platform.HotkeyResult{Gesture: core.Gesture{Modifiers: core.ModAlt, Key: "Q"}})
	tt.Frame()
	if v.recordingID != "" || v.hotkeys[launcherIdx].draft != "Alt+Q" {
		t.Fatalf("after Alt+Q: recordingID=%q draft=%q", v.recordingID, v.hotkeys[launcherIdx].draft)
	}
	if !tt.HasText("已录制: Alt+Q") {
		t.Fatalf("expected recorded status banner, texts=%v", tt.Texts())
	}

	if err := tt.Click("保存"); err != nil {
		t.Fatalf("click save: %v", err)
	}
	tt.Frame()

	if got := a.HostConfig().Launcher.Shortcut; got != "Alt+Q" {
		t.Fatalf("saved Launcher.Shortcut = %q, want Alt+Q", got)
	}
	a.hotkeyMu.RLock()
	reg, ok := a.hotkeys["host.launcher"]
	a.hotkeyMu.RUnlock()
	if !ok || reg.String() != "Alt+Q" {
		t.Fatalf("registered hotkeys[host.launcher] = (%q, %v), want Alt+Q", reg.String(), ok)
	}
}

func TestWinKeyTakeoverFallbackAndSettingsToggle(t *testing.T) {
	a := testHostWithModules(t, CompiledModules(), nil, nil)
	hc := a.HostConfig()
	hc.Hotkeys.TakeoverWinKeys = false
	if err := a.SaveHostConfig(hc); err != nil {
		t.Fatal(err)
	}

	a.registerShortcutFn = func(acc string, fn func()) error {
		if strings.Contains(acc, "Win+") {
			return fmt.Errorf("Hot key is already registered")
		}
		return nil
	}

	v := newSettingsView(a, "")
	tt := ui.NewTester(v.view, 860, 640)
	tt.Frame()

	launcherIdx := -1
	for i, hk := range v.hotkeys {
		if hk.spec.ID == "host.launcher" {
			launcherIdx = i
			break
		}
	}
	if launcherIdx < 0 {
		t.Fatal("missing host.launcher hotkey entry")
	}

	// Recording Win+Space automatically enables TakeoverWinKeys even on an existing config with false.
	v.recordingID = "host.launcher"
	v.onRecording(platform.HotkeyResult{Gesture: core.Gesture{Modifiers: core.ModWin, Key: "SPACE"}})
	tt.Frame()
	if !v.hostDraft.Hotkeys.TakeoverWinKeys {
		t.Fatal("expected TakeoverWinKeys to auto-enable after recording Win+Space")
	}
	if !strings.Contains(v.status, "Win+Space") {
		t.Fatalf("unexpected recording status: %q", v.status)
	}

	if err := tt.Click("保存"); err != nil {
		t.Fatalf("click save: %v", err)
	}
	tt.Frame()

	if !a.HostConfig().Hotkeys.TakeoverWinKeys {
		t.Fatal("expected TakeoverWinKeys=true persisted")
	}
	if got := a.HostConfig().Launcher.Shortcut; got != "Win+Space" {
		t.Fatalf("Launcher.Shortcut = %q, want Win+Space", got)
	}
	if errMsg := a.HotkeyErrorByID("host.launcher"); errMsg != "" {
		t.Fatalf("unexpected HotkeyErrorByID after takeover fallback: %q", errMsg)
	}
	a.hotkeyMu.RLock()
	tk := a.takeoverHotkeys["host.launcher"]
	a.hotkeyMu.RUnlock()
	if tk.String() != "Win+Space" {
		t.Fatalf("takeoverHotkeys[host.launcher] = %q, want Win+Space", tk.String())
	}

	// Toggling TakeoverWinKeys off rejects system-occupied Win combos cleanly.
	tt.Scroll(450, 300, 0, 300)
	tt.Frame()
	if err := tt.Click("开关：接管系统 Win 组合键"); err != nil {
		t.Fatalf("toggle TakeoverWinKeys off: %v", err)
	}
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("click save after toggle off: %v", err)
	}
	tt.Frame()

	if a.HostConfig().Hotkeys.TakeoverWinKeys {
		t.Fatal("expected TakeoverWinKeys=false persisted")
	}
	if errMsg := a.HotkeyErrorByID("host.launcher"); !strings.Contains(errMsg, "接管系统 Win 组合键") {
		t.Fatalf("HotkeyErrorByID after disable = %q, want guidance referencing 接管系统 Win 组合键", errMsg)
	}
	a.hotkeyMu.RLock()
	tkLen := len(a.takeoverHotkeys)
	a.hotkeyMu.RUnlock()
	if tkLen != 0 {
		t.Fatalf("takeoverHotkeys should be empty when disabled, got %d", tkLen)
	}

	// Win+L must always be rejected.
	v.hotkeys[launcherIdx].draft = "Win+L"
	tt.Frame()
	if errMsg := v.hotkeyError(launcherIdx); !strings.Contains(errMsg, "Win+L") {
		t.Fatalf("expected Win+L validation error, got %q", errMsg)
	}
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("click save with Win+L: %v", err)
	}
	tt.Frame()
	if got := a.HostConfig().Launcher.Shortcut; got != "Win+Space" {
		t.Fatalf("Win+L overwrote saved shortcut: %q", got)
	}
}

func TestSettingsHotToggleModules(t *testing.T) {
	compiled := CompiledModules()
	if !compiled.History || !compiled.Capture {
		t.Skip("requires PureClip and PureShot compiled")
	}

	a := testHostWithModules(t, compiled, nil, nil)
	a.Start([]string{"--background"})
	a.ReloadHotkeys()
	a.hotkeyMu.RLock()
	_, hasClipHK := a.hotkeys["pureclip.show"]
	a.hotkeyMu.RUnlock()
	if !hasClipHK {
		t.Fatal("expected pureclip.show hotkey registered before disable")
	}

	initialVer := a.Bus().Version(purekit.CapClipboardHistory)
	if initialVer == 0 {
		t.Fatal("expected initial CapClipboardHistory bus version > 0")
	}

	v := newSettingsView(a, "advanced")
	tt := ui.NewTester(v.view, 860, 640)
	tt.Frame()
	if !tt.HasText("PureClip · 剪贴板") {
		t.Fatalf("expected PureClip sidebar tab before disable, texts=%v", tt.Texts())
	}

	tt.Scroll(450, 300, 0, 500)
	tt.Frame()
	if err := tt.Click("开关：启用 PureClip"); err != nil {
		t.Fatalf("toggle PureClip off: %v", err)
	}
	tt.Frame()
	if !v.isDirty() {
		t.Fatal("expected settingsView dirty after toggling module")
	}
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("save disable PureClip: %v", err)
	}
	tt.Frame()

	if a.ModuleEnabled(core.ModuleHistory) {
		t.Fatal("expected PureClip module disabled immediately after save")
	}
	if a.PluginByID("pureclip") != nil {
		t.Fatal("expected pureclip removed from PluginByID")
	}
	if a.Bus().Has(purekit.CapClipboardHistory) {
		t.Fatal("expected CapClipboardHistory unregistered after hot disable")
	}
	if !a.HostConfig().IsModuleDisabled(core.ModuleHistory) {
		t.Fatalf("expected ModuleHistory persisted in DisabledModules, got %+v", a.HostConfig().DisabledModules)
	}
	if tt.HasText("PureClip · 剪贴板") {
		t.Fatalf("expected PureClip tab removed from sidebar, texts=%v", tt.Texts())
	}
	a.hotkeyMu.RLock()
	_, hasClipHK = a.hotkeys["pureclip.show"]
	a.hotkeyMu.RUnlock()
	if hasClipHK {
		t.Fatal("expected pureclip.show hotkey unregistered after disable")
	}
	for _, it := range a.QueryLauncher(purekit.MatchContext{Query: "clip"}) {
		if it.PluginID == "pureclip" {
			t.Fatalf("launcher still returned disabled pureclip item: %+v", it)
		}
	}

	// Reloading config from disk preserves disabled state across restarts.
	reloadedCfg := config.Load(a.Paths().Settings)
	logger2, err := core.OpenLogger(a.Paths().Logs, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logger2.Close() })
	app2, err := NewApp(Options{
		Paths:    a.Paths(),
		Config:   reloadedCfg,
		Modules:  compiled,
		Logger:   logger2,
		Headless: true,
	})
	if err != nil {
		t.Fatalf("NewApp reload with disabled module: %v", err)
	}
	t.Cleanup(func() { app2.Shutdown() })
	if app2.ModuleEnabled(core.ModuleHistory) || app2.PluginByID("pureclip") != nil {
		t.Fatal("restarted app should respect persisted DisabledModules")
	}

	// Re-enabling PureClip hot-starts it and bumps bus capability version.
	if err := tt.Click("开关：启用 PureClip"); err != nil {
		t.Fatalf("toggle PureClip back on: %v", err)
	}
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("save re-enable PureClip: %v", err)
	}
	tt.Frame()

	if !a.ModuleEnabled(core.ModuleHistory) || a.PluginByID("pureclip") == nil {
		t.Fatal("expected PureClip active after re-enable")
	}
	if ver := a.Bus().Version(purekit.CapClipboardHistory); ver <= initialVer {
		t.Fatalf("expected CapClipboardHistory bus version > %d after restart, got %d", initialVer, ver)
	}
	if a.HostConfig().IsModuleDisabled(core.ModuleHistory) {
		t.Fatalf("expected ModuleHistory cleared from DisabledModules, got %+v", a.HostConfig().DisabledModules)
	}
	if !tt.HasText("PureClip · 剪贴板") {
		t.Fatalf("expected PureClip tab restored in sidebar, texts=%v", tt.Texts())
	}
	a.hotkeyMu.RLock()
	_, hasClipHK = a.hotkeys["pureclip.show"]
	a.hotkeyMu.RUnlock()
	if !hasClipHK {
		t.Fatal("expected pureclip.show hotkey restored after re-enable")
	}

	// Guard against disabling all installed modules.
	v.modDraft = core.ModuleConfig{}
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("save all disabled: %v", err)
	}
	tt.Frame()
	if !strings.Contains(v.status, "至少保留") {
		t.Fatalf("expected minimum-module guard status, got %q", v.status)
	}
	if !a.ActiveModules().Any() {
		t.Fatal("active modules must not become empty")
	}
}
