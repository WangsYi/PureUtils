package host

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
)

func bootHostOnPaths(t *testing.T, paths core.Paths, mods core.ModuleConfig, clip clipboard.Service) (*App, *core.Logger) {
	t.Helper()
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
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { app.Shutdown() })
	return app, logger
}

func TestE2ECrossPluginClipboardToPinAndTray(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	t.Cleanup(func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	})

	compiled := CompiledModules()
	if !compiled.History {
		t.Skip("requires PureClip compiled")
	}

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	paths := testHostPaths(t)
	io, watcher, clip := newMockClipboard(t)
	a1, log1 := bootHostOnPaths(t, paths, compiled, clip)
	a1.Start([]string{"--background"})

	if err := io.Write(&core.ClipboardContent{Text: "e2e cross-plugin clipboard text"}); err != nil {
		t.Fatal(err)
	}
	watcher.Fire()
	if err := purekit.CallClipboardHistoryFlush(context.Background(), a1.Bus()); err != nil {
		t.Fatalf("flush text history: %v", err)
	}

	pngBytes := encodeTinyPNG(t)
	if err := io.Write(&core.ClipboardContent{ImagePNG: pngBytes}); err != nil {
		t.Fatal(err)
	}
	watcher.Fire()
	if err := purekit.CallClipboardHistoryFlush(context.Background(), a1.Bus()); err != nil {
		t.Fatalf("flush image history: %v", err)
	}

	recent, err := purekit.CallClipboardHistoryListRecent(context.Background(), a1.Bus(), 10)
	if err != nil {
		t.Fatalf("list recent history: %v", err)
	}
	if len(recent.Entries) != 2 {
		t.Fatalf("expected 2 persisted clipboard entries, got %d (%+v)", len(recent.Entries), recent.Entries)
	}

	if compiled.Capture {
		if err := purekit.CallImagePinText(context.Background(), a1.Bus(), "e2e sticky note", 0); err != nil {
			t.Fatalf("pin text: %v", err)
		}
		img, err := png.Decode(bytes.NewReader(pngBytes))
		if err != nil {
			t.Fatal(err)
		}
		if err := purekit.CallImagePinImage(context.Background(), a1.Bus(), img, 1); err != nil {
			t.Fatalf("pin image: %v", err)
		}

		itemsZH := a1.buildTrayMenu().Items()
		pinZH := findMenuItem(itemsZH, "显示贴图")
		if pinZH == nil || pinZH.Label != "显示贴图 (2)" || !pinZH.Checked || pinZH.Disabled {
			t.Fatalf("ZH tray pin item = %+v, want '显示贴图 (2)' checked & enabled", pinZH)
		}

		i18n.Set(i18n.LangEN)
		itemsEN := a1.buildTrayMenu().Items()
		pinEN := findMenuItem(itemsEN, "Show Pins")
		if pinEN == nil || pinEN.Label != "Show Pins (2)" || !pinEN.Checked || pinEN.Disabled {
			t.Fatalf("EN tray pin item = %+v, want 'Show Pins (2)' checked & enabled", pinEN)
		}

		a1.ExecuteCommand(core.CmdTogglePins)
		pinHidden := findMenuItem(a1.buildTrayMenu().Items(), "Show Pins")
		if pinHidden == nil || pinHidden.Checked {
			t.Fatalf("expected Show Pins unchecked after CmdTogglePins, got %+v", pinHidden)
		}

		a1.ExecuteCommand(core.CmdTogglePinsPassthrough)
		ptEN := findMenuItem(a1.buildTrayMenu().Items(), "Pin Click-Through")
		if ptEN == nil || !ptEN.Checked || ptEN.Disabled {
			t.Fatalf("expected Pin Click-Through checked after CmdTogglePinsPassthrough, got %+v", ptEN)
		}
	}

	a1.Shutdown()
	log1.Close()

	// Reboot a fresh host on the exact same storage paths and verify SQLite history survived.
	a2, _ := bootHostOnPaths(t, paths, compiled, nil)
	a2.Start([]string{"--background"})
	recent2, err := purekit.CallClipboardHistoryListRecent(context.Background(), a2.Bus(), 10)
	if err != nil {
		t.Fatalf("list recent after reboot: %v", err)
	}
	if len(recent2.Entries) != 2 {
		t.Fatalf("expected 2 entries persisted across restart, got %d (%+v)", len(recent2.Entries), recent2.Entries)
	}
}

func TestE2ESettingsLanguageThemeAndModuleRestartPersistence(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	t.Cleanup(func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	})

	compiled := CompiledModules()
	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	paths := testHostPaths(t)
	a1, log1 := bootHostOnPaths(t, paths, compiled, nil)
	a1.Start([]string{"--background"})

	sv := newSettingsView(a1, "general")
	tt := ui.NewTester(sv.view, 860, 640)
	tt.Frame()

	if err := tt.Click("English"); err != nil {
		t.Fatalf("click English language: %v", err)
	}
	tt.Frame()
	if err := tt.Click("深色"); err != nil {
		t.Fatalf("click Dark theme: %v", err)
	}
	tt.Frame()
	if err := tt.Click("高级"); err != nil {
		t.Fatalf("click Advanced tab: %v", err)
	}
	tt.Frame()

	if compiled.Zip {
		tt.Scroll(450, 300, 0, 600)
		tt.Frame()
		tt.Scroll(450, 300, 0, 600)
		tt.Frame()
		if err := tt.Click("开关：启用 PureZip"); err != nil {
			t.Fatalf("toggle PureZip off: %v", err)
		}
		tt.Frame()
	}

	if err := tt.Click("保存"); err != nil {
		t.Fatalf("click Save (ZH): %v", err)
	}
	tt.Frame()

	if got := i18n.Current(); got != i18n.LangEN {
		t.Fatalf("i18n.Current() after save = %q, want %q", got, i18n.LangEN)
	}
	hc1 := a1.HostConfig()
	if hc1.Appearance.Theme != "dark" || hc1.Appearance.Language != i18n.PrefEN {
		t.Fatalf("persisted HostConfig.Appearance = %+v", hc1.Appearance)
	}
	if compiled.Zip && a1.PluginByID("purezip") != nil {
		t.Fatal("expected purezip hot-unloaded after save")
	}
	if !tt.HasText("Advanced & Updates") || !tt.HasText("Save") {
		t.Fatalf("expected live EN switch in Settings UI, texts=%v", tt.Texts())
	}
	if err := tt.Click("General"); err != nil {
		t.Fatalf("click General sidebar in EN: %v", err)
	}
	tt.Frame()
	if !tt.HasText("General & Appearance") {
		t.Fatalf("expected EN General heading, texts=%v", tt.Texts())
	}

	a1.Shutdown()
	log1.Close()

	// Reset global i18n briefly to prove NewApp restores persisted "en-US" from settings.json.
	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	a2, _ := bootHostOnPaths(t, paths, compiled, nil)
	a2.Start([]string{"--background"})

	if got := i18n.Current(); got != i18n.LangEN {
		t.Fatalf("rebooted i18n.Current() = %q, want %q", got, i18n.LangEN)
	}
	if a2.HostConfig().Appearance.Theme != "dark" {
		t.Fatalf("rebooted Theme = %q, want dark", a2.HostConfig().Appearance.Theme)
	}
	if compiled.Zip && (a2.PluginByID("purezip") != nil || a2.ModuleEnabled(core.ModuleZip)) {
		t.Fatal("expected PureZip to remain disabled across restart")
	}

	trayItems := a2.buildTrayMenu().Items()
	if findMenuItem(trayItems, "Settings") == nil || findMenuItem(trayItems, "Quick Launcher") == nil {
		t.Fatalf("expected EN tray items after restart, got %+v", trayItems)
	}

	if compiled.Zip {
		sv2 := newSettingsView(a2, "advanced")
		tt2 := ui.NewTester(sv2.view, 860, 640)
		tt2.Frame()
		tt2.Scroll(450, 300, 0, 600)
		tt2.Frame()
		tt2.Scroll(450, 300, 0, 600)
		tt2.Frame()
		if err := tt2.Click("Toggle: Enable PureZip"); err != nil {
			t.Fatalf("re-enable PureZip toggle: %v", err)
		}
		tt2.Frame()
		if err := tt2.Click("Save"); err != nil {
			t.Fatalf("click Save (EN): %v", err)
		}
		tt2.Frame()
		if a2.PluginByID("purezip") == nil || !a2.ModuleEnabled(core.ModuleZip) {
			t.Fatal("expected PureZip restored after re-enable and save")
		}
	}
}

func TestE2ELauncherContextAcrossAllPluginsBilingual(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	t.Cleanup(func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	})

	compiled := CompiledModules()
	paths := testHostPaths(t)
	a, _ := bootHostOnPaths(t, paths, compiled, nil)
	a.Start([]string{"--background"})

	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "bundle.zip")
	pngPath := filepath.Join(tmpDir, "photo.png")
	mp4Path := filepath.Join(tmpDir, "clip.mp4")
	_ = os.WriteFile(zipPath, []byte("PK"), 0o600)
	_ = os.WriteFile(pngPath, encodeTinyPNG(t), 0o600)
	_ = os.WriteFile(mp4Path, []byte("ftypmp42"), 0o600)
	clipImg := &core.ClipboardContent{ImagePNG: encodeTinyPNG(t)}

	for _, lang := range []i18n.Lang{i18n.LangZH, i18n.LangEN} {
		i18n.Set(lang)
		zipTitle := "用 PureZip 打开"
		imgTitle := "用 PureImage 查看"
		vidTitle := "用 PureVideo 播放"
		clipViewerTitle := "查看剪贴板图片"
		clipPinTitle := "将剪贴板内容贴到桌面"
		ctxHeading := "上下文"
		clearBtn := "清除上下文"
		if lang == i18n.LangEN {
			zipTitle = "Open with PureZip:"
			imgTitle = "with PureImage"
			vidTitle = "Play with PureVideo:"
			clipViewerTitle = "View Clipboard Image in PureImage"
			clipPinTitle = "Pin Clipboard Content to Screen"
			ctxHeading = "Context"
			clearBtn = "Clear"
		}

		if compiled.Zip {
			if items := a.QueryLauncher(purekit.MatchContext{SelectedFiles: []string{zipPath}}); !hasLauncherItem(items, "purezip", zipTitle) {
				t.Fatalf("[%s] missing PureZip contextual match: %+v", lang, items)
			}
		}
		if compiled.Viewer {
			if items := a.QueryLauncher(purekit.MatchContext{SelectedFiles: []string{pngPath}}); !hasLauncherItem(items, "pureimage", imgTitle) {
				t.Fatalf("[%s] missing PureImage file match: %+v", lang, items)
			}
			if items := a.QueryLauncher(purekit.MatchContext{Clipboard: clipImg}); !hasLauncherItem(items, "pureimage", clipViewerTitle) {
				t.Fatalf("[%s] missing PureImage clipboard match: %+v", lang, items)
			}
		}
		if compiled.Video {
			if items := a.QueryLauncher(purekit.MatchContext{SelectedFiles: []string{mp4Path}}); !hasLauncherItem(items, "purevideo", vidTitle) {
				t.Fatalf("[%s] missing PureVideo file match: %+v", lang, items)
			}
		}
		if compiled.Capture {
			if items := a.QueryLauncher(purekit.MatchContext{Clipboard: clipImg}); !hasLauncherItem(items, "pureshot", clipPinTitle) {
				t.Fatalf("[%s] missing PureShot clipboard pin match: %+v", lang, items)
			}
		}

		a.launcherCtxProvider = func() purekit.MatchContext {
			return purekit.MatchContext{
				SelectedFiles: []string{zipPath},
				Clipboard:     clipImg,
			}
		}
		a.ShowLauncher()
		lv := a.launcherView
		if lv == nil {
			t.Fatalf("[%s] expected launcherView open", lang)
		}
		lt := ui.NewTester(lv.view, launcherWidth, launcherHeight)
		lt.Frame()
		if !lt.HasText(ctxHeading) || !lt.HasText(clearBtn) {
			t.Fatalf("[%s] launcher missing context ribbon (%q/%q): %v", lang, ctxHeading, clearBtn, lt.Texts())
		}
		if len(lv.items) >= 2 {
			lt.Key(0, ui.KeyDown)
			lt.Frame()
			if lv.selected != 1 {
				t.Fatalf("[%s] expected KeyDown selection=1, got %d", lang, lv.selected)
			}
			lt.Key(0, ui.KeyUp)
			lt.Frame()
			if lv.selected != 0 {
				t.Fatalf("[%s] expected KeyUp selection=0, got %d", lang, lv.selected)
			}
		}
		if err := lt.Click(clearBtn); err != nil {
			t.Fatalf("[%s] click %s: %v", lang, clearBtn, err)
		}
		lt.Frame()
		if lt.HasText(clearBtn) {
			t.Fatalf("[%s] expected context cleared after clicking %s", lang, clearBtn)
		}
		lv.query = "settings"
		lv.refresh()
		lt.Frame()
		lt.Key(0, ui.KeyEscape)
		lt.Frame()
		if lv.query != "" || lv.closed {
			t.Fatalf("[%s] first Esc should clear non-empty query: query=%q closed=%v", lang, lv.query, lv.closed)
		}
		lt.Key(0, ui.KeyEscape)
		lt.Frame()
		if !lv.closed || a.launcherView != nil {
			t.Fatalf("[%s] second Esc should close launcher", lang)
		}
	}
}
