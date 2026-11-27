package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
)

func hasLauncherItem(items []LauncherItem, pluginID, titleSub string) bool {
	for _, it := range items {
		if (pluginID == "" || it.PluginID == pluginID) && strings.Contains(it.Title, titleSub) {
			return true
		}
	}
	return false
}

func TestLauncherKeywordSearchAcrossPlugins(t *testing.T) {
	compiled := CompiledModules()
	a := testHostWithModules(t, compiled, nil, nil)
	a.Start([]string{"--background"})

	if compiled.Capture {
		items := a.QueryLauncher(purekit.MatchContext{Query: "截图"})
		if !hasLauncherItem(items, "pureshot", "区域截图") {
			t.Fatalf("expected pureshot 区域截图 for query 截图, got %+v", items)
		}
	}
	if compiled.History {
		items := a.QueryLauncher(purekit.MatchContext{Query: "剪贴板"})
		if !hasLauncherItem(items, "pureclip", "剪贴板历史") {
			t.Fatalf("expected pureclip 剪贴板历史 for query 剪贴板, got %+v", items)
		}
	}
	if compiled.Zip {
		items := a.QueryLauncher(purekit.MatchContext{Query: "zip"})
		if !hasLauncherItem(items, "purezip", "PureZip") {
			t.Fatalf("expected purezip feature for query zip, got %+v", items)
		}
	}
	if compiled.Viewer {
		items := a.QueryLauncher(purekit.MatchContext{Query: "看图"})
		if !hasLauncherItem(items, "pureimage", "PureImage") {
			t.Fatalf("expected pureimage feature for query 看图, got %+v", items)
		}
	}
	if compiled.Video {
		items := a.QueryLauncher(purekit.MatchContext{Query: "视频"})
		if !hasLauncherItem(items, "purevideo", "PureVideo") {
			t.Fatalf("expected purevideo feature for query 视频, got %+v", items)
		}
	}

	hostItems := a.QueryLauncher(purekit.MatchContext{Query: "设置"})
	if !hasLauncherItem(hostItems, "host", "设置") {
		t.Fatalf("expected host.settings for query 设置, got %+v", hostItems)
	}
	updItems := a.QueryLauncher(purekit.MatchContext{Query: "更新"})
	if !hasLauncherItem(updItems, "host", "检查软件更新") {
		t.Fatalf("expected host.check_update for query 更新, got %+v", updItems)
	}
}

func TestLauncherContextualMatchingClipboardAndFiles(t *testing.T) {
	compiled := CompiledModules()
	a := testHostWithModules(t, compiled, nil, nil)
	a.Start([]string{"--background"})

	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "release.zip")
	_ = os.WriteFile(zipPath, []byte("PK"), 0o600)

	pngPath := filepath.Join(tmpDir, "sample.png")
	_ = os.WriteFile(pngPath, encodeTinyPNG(t), 0o600)

	if compiled.Zip {
		items := a.QueryLauncher(purekit.MatchContext{SelectedFiles: []string{zipPath}})
		if !hasLauncherItem(items, "purezip", "用 PureZip 打开") || !hasLauncherItem(items, "purezip", "智能解压") {
			t.Fatalf("expected PureZip contextual matches for .zip selection, got %+v", items)
		}
		if len(items) == 0 || !items[0].IsMatch {
			t.Fatalf("expected contextual match ranked first, got %+v", items)
		}

		// Narrowing query with "clip" must filter out unrelated zip selection.
		narrow := a.QueryLauncher(purekit.MatchContext{
			Query:         "clip",
			SelectedFiles: []string{zipPath},
		})
		if hasLauncherItem(narrow, "purezip", "用 PureZip 打开") {
			t.Fatalf("expected unrelated zip match filtered out when querying clip, got %+v", narrow)
		}
		if compiled.History && !hasLauncherItem(narrow, "pureclip", "剪贴板历史") {
			t.Fatalf("expected PureClip feature for query clip, got %+v", narrow)
		}
	}

	if compiled.Viewer {
		imgItems := a.QueryLauncher(purekit.MatchContext{SelectedFiles: []string{pngPath}})
		if !hasLauncherItem(imgItems, "pureimage", "用 PureImage 查看") {
			t.Fatalf("expected PureImage contextual match for .png selection, got %+v", imgItems)
		}
	}
	if compiled.Video {
		mp4Path := filepath.Join(tmpDir, "clip.mp4")
		_ = os.WriteFile(mp4Path, []byte("MP4"), 0o600)
		vidItems := a.QueryLauncher(purekit.MatchContext{SelectedFiles: []string{mp4Path}})
		if !hasLauncherItem(vidItems, "purevideo", "用 PureVideo 播放") {
			t.Fatalf("expected PureVideo contextual match for .mp4 selection, got %+v", vidItems)
		}
	}

	clipImg := &core.ClipboardContent{ImagePNG: encodeTinyPNG(t)}
	clipItems := a.QueryLauncher(purekit.MatchContext{Clipboard: clipImg})
	if compiled.Viewer && !hasLauncherItem(clipItems, "pureimage", "查看剪贴板图片") {
		t.Fatalf("expected PureImage clipboard image match, got %+v", clipItems)
	}
	if compiled.Capture && !hasLauncherItem(clipItems, "pureshot", "将剪贴板内容贴到桌面") {
		t.Fatalf("expected PureShot clipboard pin match, got %+v", clipItems)
	}
}

type launcherSpyPlugin struct {
	executed []string
}

func (p *launcherSpyPlugin) ID() string    { return "spy" }
func (p *launcherSpyPlugin) Title() string { return "Spy" }
func (p *launcherSpyPlugin) Info() purekit.PluginInfo {
	return purekit.PluginInfo{ID: "spy", Name: "SpyPlugin", ModuleID: core.ModuleCapture}
}
func (p *launcherSpyPlugin) Init(purekit.Host) error             { return nil }
func (p *launcherSpyPlugin) Start() error                        { return nil }
func (p *launcherSpyPlugin) Stop() error                         { return nil }
func (p *launcherSpyPlugin) SettingsTab() *purekit.SettingsTab   { return nil }
func (p *launcherSpyPlugin) Hotkeys() []purekit.HotkeySpec       { return nil }
func (p *launcherSpyPlugin) TrayItems() []purekit.TrayItemSpec   { return nil }
func (p *launcherSpyPlugin) CLICommands() []purekit.CLICommandSpec {
	return nil
}
func (p *launcherSpyPlugin) HasOpenWindows() bool       { return false }
func (p *launcherSpyPlugin) ShouldStayInTray() bool     { return true }
func (p *launcherSpyPlugin) IsBusyForGlobalHooks() bool { return false }
func (p *launcherSpyPlugin) Matchers() []purekit.Matcher {
	return []purekit.Matcher{p}
}
func (p *launcherSpyPlugin) Match(ctx purekit.MatchContext) []purekit.MatchResult {
	if len(ctx.SelectedFiles) == 0 {
		return nil
	}
	f := ctx.SelectedFiles[0]
	return []purekit.MatchResult{{
		FeatureID: "spy.open_file",
		PluginID:  "spy",
		Title:     "使用 Spy 打开 " + filepath.Base(f),
		Subtitle:  f,
		Badge:     "Spy",
		Score:     95,
		Execute: func(_ context.Context) error {
			p.executed = append(p.executed, "match:"+filepath.Base(f))
			return nil
		},
	}}
}
func (p *launcherSpyPlugin) Features() []purekit.Feature {
	return []purekit.Feature{
		{
			ID:       "spy.alpha",
			PluginID: "spy",
			Title:    "Alpha 动作",
			Subtitle: "第一项测试动作",
			Keywords: []string{"alpha"},
			Badge:    "Spy",
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.executed = append(p.executed, "alpha")
				return nil
			},
		},
		{
			ID:       "spy.beta",
			PluginID: "spy",
			Title:    "Beta 动作",
			Subtitle: "第二项测试动作",
			Keywords: []string{"beta"},
			Badge:    "Spy",
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.executed = append(p.executed, "beta")
				return nil
			},
		},
	}
}

func TestLauncherHeadlessUIInteraction(t *testing.T) {
	paths := testHostPaths(t)
	spy := &launcherSpyPlugin{}
	app, err := NewApp(Options{
		Paths:    paths,
		Config:   config.Load(paths.Settings),
		Headless: true,
		Plugins:  []purekit.Plugin{spy},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Shutdown()

	app.launcherCtxProvider = func() purekit.MatchContext {
		return purekit.MatchContext{
			SelectedFiles: []string{`D:\docs\demo.zip`},
			Clipboard:     &core.ClipboardContent{Text: "hello clipboard"},
		}
	}

	app.ShowLauncher()
	if app.launcherView == nil || app.launcherView.closed {
		t.Fatal("expected launcherView open in headless mode")
	}
	lv := app.launcherView
	tt := ui.NewTester(lv.view, launcherWidth, launcherHeight)

	for _, dark := range []bool{true, false} {
		tt.SetDark(dark)
		tt.Frame()
		if !tt.HasText("上下文") || !tt.HasText("清除上下文") || !tt.HasText("使用 Spy 打开 demo.zip") {
			t.Fatalf("dark=%v launcher missing context or matched row: %v", dark, tt.Texts())
		}
	}

	// Clear context ribbon removes contextual match and shows static features
	if err := tt.Click("清除上下文"); err != nil {
		t.Fatalf("click 清除上下文: %v", err)
	}
	tt.Frame()
	if tt.HasText("清除上下文") {
		t.Fatalf("expected context ribbon cleared, texts=%v", tt.Texts())
	}
	if !tt.HasText("Alpha 动作") || !tt.HasText("Beta 动作") {
		t.Fatalf("expected static plugin features after clearing context: %v", tt.Texts())
	}

	// Arrow Down / Up changes selected index and Enter executes & closes launcher
	if lv.selected != 0 {
		t.Fatalf("initial selected = %d, want 0", lv.selected)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if lv.selected != 1 {
		t.Fatalf("after KeyDown selected = %d, want 1", lv.selected)
	}
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if lv.selected != 0 {
		t.Fatalf("after KeyUp selected = %d, want 0", lv.selected)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(spy.executed) != 1 || spy.executed[0] != "alpha" {
		t.Fatalf("executed = %v, want [alpha]", spy.executed)
	}
	if !lv.closed || app.launcherView != nil {
		t.Fatal("expected launcher closed after executing action")
	}

	// Reopen and verify search filtering + Esc close
	app.ToggleLauncher()
	if app.launcherView == nil {
		t.Fatal("expected ToggleLauncher to open launcher")
	}
	lv2 := app.launcherView
	tt2 := ui.NewTester(lv2.view, launcherWidth, launcherHeight)
	tt2.Frame()
	lv2.query = "beta"
	lv2.refresh()
	tt2.Frame()
	if !tt2.HasText("Beta 动作") {
		t.Fatalf("expected Beta 动作 after query=beta, got %v", tt2.Texts())
	}
	// First Esc clears non-empty SearchField query back to full list.
	tt2.Key(0, ui.KeyEscape)
	tt2.Frame()
	if lv2.query != "" || lv2.closed {
		t.Fatalf("first Esc with non-empty query: query=%q closed=%v", lv2.query, lv2.closed)
	}
	// Second Esc closes the launcher window.
	tt2.Key(0, ui.KeyEscape)
	tt2.Frame()
	if !lv2.closed || app.launcherView != nil {
		t.Fatal("expected second Esc to close launcher")
	}
}

func TestLauncherSettingsPersistenceAndTrayToggle(t *testing.T) {
	compiled := CompiledModules()
	a := testHostWithModules(t, compiled, nil, nil)
	a.Start([]string{"--background"})

	if got := a.HostConfig().Launcher.Shortcut; got != "Alt+Space" {
		t.Fatalf("default Launcher.Shortcut = %q, want Alt+Space", got)
	}
	if findMenuItem(a.buildTrayMenu().Items(), "快捷启动器") == nil {
		t.Fatal("expected 快捷启动器 in tray menu when enabled")
	}

	// Disable launcher via Settings UI
	v := newSettingsView(a, "")
	tt := ui.NewTester(v.view, 860, 640)
	tt.Frame()
	tt.Scroll(450, 300, 0, 600)
	tt.Frame()
	if err := tt.Click("开关：启用快捷启动器"); err != nil {
		t.Fatalf("click launcher toggle: %v", err)
	}
	tt.Frame()
	if !v.isDirty() {
		t.Fatal("expected dirty=true after toggling launcher switch")
	}
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("click save: %v", err)
	}
	tt.Frame()

	hc := a.HostConfig()
	if hc.Launcher.Enabled {
		t.Fatal("expected HostConfig().Launcher.Enabled=false after save")
	}
	if findMenuItem(a.buildTrayMenu().Items(), "快捷启动器") != nil {
		t.Fatal("expected 快捷启动器 hidden from tray when disabled")
	}

	// Re-enable and verify Alt+Space registered in hotkeys map
	if err := tt.Click("开关：启用快捷启动器"); err != nil {
		t.Fatalf("re-enable launcher toggle: %v", err)
	}
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("click save after re-enable: %v", err)
	}
	tt.Frame()

	a.hotkeyMu.RLock()
	g, ok := a.hotkeys["host.launcher"]
	a.hotkeyMu.RUnlock()
	if !ok || g.String() != "Alt+Space" {
		t.Fatalf("hotkeys[host.launcher] = (%q, %v), want Alt+Space", g.String(), ok)
	}
}
