package host

import (
	"strings"

	"github.com/egoist/mygo"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/pkg/purekit/platform"
)

func (a *App) createTray() {
	if a.headless || a.tray != nil {
		return
	}
	trayID := mygo.App.Name()
	if trayID == "" {
		trayID = "PureUtils"
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      trayID,
		Icon:    iconPNG,
		ToolTip: trayID,
		Menu:    a.buildTrayMenu(),
	})
	if err != nil {
		if a.log != nil {
			a.log.Errorf("托盘创建失败: %v", err)
		}
		return
	}
	a.tray = tray
	a.updateTrayStatus()
}

func (a *App) buildTrayMenu() *mygo.Menu {
	var primary []*mygo.MenuItem
	var toggles []*mygo.MenuItem

	if hc := a.HostConfig(); hc.Launcher.Enabled {
		primary = append(primary, &mygo.MenuItem{
			Label:       i18n.S("快捷启动器", "Quick Launcher"),
			Accelerator: hc.Launcher.Shortcut,
			Click:       func(*mygo.MenuItem, *mygo.Window) { a.ExecuteCommand(core.CmdOpenLauncher) },
		})
	}

	for _, p := range a.snapshotPlugins() {
		for _, spec := range p.TrayItems() {
			mi := a.menuItemFromSpec(spec)
			if mi == nil {
				continue
			}
			if spec.Section == purekit.TraySectionToggle {
				toggles = append(toggles, mi)
			} else {
				primary = append(primary, mi)
			}
		}
	}

	var items []*mygo.MenuItem
	appendGroup := func(group []*mygo.MenuItem) {
		if len(group) == 0 {
			return
		}
		if len(items) > 0 {
			items = append(items, mygo.Separator())
		}
		items = append(items, group...)
	}

	updateItem := &mygo.MenuItem{
		Label: i18n.S("检查更新", "Check for Updates"),
		Click: func(*mygo.MenuItem, *mygo.Window) { a.ExecuteCommand(core.CmdCheckUpdate) },
	}
	if a.updater != nil {
		snap := a.updater.Snapshot()
		openAdv := func(*mygo.MenuItem, *mygo.Window) { a.OpenSettings("advanced") }
		switch snap.Status {
		case UpdateReady:
			if snap.Release != nil && snap.Release.TagName != "" {
				updateItem = &mygo.MenuItem{
					Label: i18n.F("安装更新 %s", "Install Update %s", snap.Release.TagName),
					Click: openAdv,
				}
			}
		case UpdateAvailable:
			if snap.Release != nil && snap.Release.TagName != "" {
				updateItem = &mygo.MenuItem{
					Label: i18n.F("发现新版本 %s", "Update Available: %s", snap.Release.TagName),
					Click: openAdv,
				}
			}
		}
	}

	appendGroup(primary)
	appendGroup(toggles)
	appendGroup([]*mygo.MenuItem{
		updateItem,
		{
			Label: i18n.S("设置", "Settings"),
			Click: func(*mygo.MenuItem, *mygo.Window) { a.ExecuteCommand(core.CmdShowSettings) },
		},
		{
			Label: i18n.S("退出 PureUtils", "Quit PureUtils"),
			Click: func(*mygo.MenuItem, *mygo.Window) { a.ExecuteCommand(core.CmdExit) },
		},
	})

	return mygo.NewMenu(items)
}

func (a *App) menuItemFromSpec(spec purekit.TrayItemSpec) *mygo.MenuItem {
	if spec.Kind == purekit.TrayItemSeparator {
		return mygo.Separator()
	}
	mi := &mygo.MenuItem{
		Label:       spec.Label,
		Accelerator: spec.Accelerator,
		Checked:     spec.Checked,
		Disabled:    spec.Disabled,
	}
	if spec.Kind == purekit.TrayItemCheckbox {
		mi.Type = mygo.MenuItemCheckbox
	}
	onClick := spec.OnClick
	cmd := spec.Command
	mi.Click = func(*mygo.MenuItem, *mygo.Window) {
		if onClick != nil {
			if a.headless {
				onClick()
			} else {
				mygo.RunOnMain(onClick)
			}
			return
		}
		if cmd != "" {
			a.ExecuteCommand(cmd)
		}
	}
	return mi
}

// RebuildTray refreshes tray items and tooltip on the UI thread.
func (a *App) RebuildTray() {
	if a.headless {
		return
	}
	mygo.RunOnMain(func() {
		if a.shuttingDown.Load() {
			return
		}
		if a.tray == nil {
			if a.ShouldStayInTray() {
				a.createTray()
			}
			return
		}
		a.tray.SetMenu(a.buildTrayMenu())
		a.updateTrayStatus()
	})
}

func (a *App) updateTrayStatus() {
	if a.tray == nil {
		return
	}
	plugins := a.snapshotPlugins()
	for _, p := range plugins {
		for _, spec := range p.TrayItems() {
			if spec.ID == "pureclip.monitoring" {
				status := i18n.S("剪贴板监控中", "Clipboard Monitoring Active")
				if !spec.Checked {
					status = i18n.S("剪贴板监控已暂停", "Clipboard Monitoring Paused")
				}
				a.tray.SetToolTip("PureUtils — " + status)
				return
			}
		}
	}
	var names []string
	for _, p := range plugins {
		if name := strings.TrimSpace(p.Info().Name); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		a.tray.SetToolTip("PureUtils")
		return
	}
	a.tray.SetToolTip("PureUtils — " + strings.Join(names, " · "))
}

// SuspendHotkeys temporarily unregisters active global shortcuts without altering
// stored configuration or error state (used while recording a new shortcut).
func (a *App) SuspendHotkeys() {
	if !a.headless {
		mygo.GlobalShortcut.UnregisterAll()
	}
	if a.winTakeover != nil {
		a.winTakeover.Close()
	}
}

func (a *App) registerHotkeys() {
	if !a.headless {
		mygo.GlobalShortcut.UnregisterAll()
	}

	hc := a.HostConfig()
	nextHotkeys := map[string]core.Gesture{}
	nextTakeover := map[string]core.Gesture{}
	nextErrs := map[core.Command]string{}
	nextErrByID := map[string]string{}
	cmdByID := map[string]core.Command{}
	var takeoverBindings []platform.HotkeyBinding

	recordErr := func(spec purekit.HotkeySpec, msg string) {
		nextErrByID[spec.ID] = msg
		if spec.Command != "" {
			nextErrs[spec.Command] = msg
		}
	}

	regFn := a.registerShortcutFn
	if regFn == nil && !a.headless {
		regFn = mygo.GlobalShortcut.Register
	}

	registerSpec := func(spec purekit.HotkeySpec) {
		if spec.CurrentGesture == nil {
			return
		}
		text := strings.TrimSpace(spec.CurrentGesture())
		if text == "" {
			return
		}
		cmdByID[spec.ID] = spec.Command
		g, err := core.ParseGesture(text)
		if err != nil {
			recordErr(spec, err.Error())
			if a.log != nil {
				a.log.Warnf("热键 %s(%s) 无效: %v", spec.ID, text, err)
			}
			return
		}
		if g.Modifiers == core.ModWin && g.Key == "L" {
			recordErr(spec, i18n.S("Win+L 是系统锁屏，不能占用", "Win+L is reserved by Windows Lock Screen"))
			if a.log != nil {
				a.log.Warnf("热键 %s=%s 不可占用: 系统锁屏", spec.ID, g)
			}
			return
		}
		cb := spec.Callback
		cmd := spec.Command
		trigger := func() {
			if cb != nil {
				if a.headless {
					cb()
				} else {
					mygo.RunOnMain(cb)
				}
			} else if cmd != "" {
				a.ExecuteCommand(cmd)
			}
		}
		if regFn != nil {
			if err := regFn(g.String(), trigger); err != nil {
				if core.RequiresTakeover(g) {
					if hc.Hotkeys.TakeoverWinKeys {
						takeoverBindings = append(takeoverBindings, platform.HotkeyBinding{
							ID:       spec.ID,
							Gesture:  g,
							Callback: trigger,
						})
						nextHotkeys[spec.ID] = g
						nextTakeover[spec.ID] = g
						if a.log != nil {
							a.log.Infof("热键 %s=%s 已被系统占用，改由 Win 钩子接管", spec.ID, g)
						}
						return
					}
					recordErr(spec, i18n.S("系统已占用该 Win 组合键，请在下方开启「接管系统 Win 组合键」开关", "This Win shortcut is reserved by Windows; enable \"Take over system Win hotkeys\" below"))
					if a.log != nil {
						a.log.Warnf("注册热键 %s=%s 失败（未开启 Win 接管）: %v", spec.ID, g, err)
					}
					return
				}
				recordErr(spec, i18n.F("%v（可能已被系统或其他程序占用）", "%v (may already be in use by Windows or another app)", err))
				if a.log != nil {
					a.log.Warnf("注册热键 %s=%s 失败: %v", spec.ID, g, err)
				}
				return
			}
		}
		nextHotkeys[spec.ID] = g
	}

	if hc.Launcher.Enabled {
		shortcut := hc.Launcher.Shortcut
		registerSpec(purekit.HotkeySpec{
			ID:             "host.launcher",
			Command:        core.CmdOpenLauncher,
			Label:          i18n.S("快捷启动器", "Quick Launcher"),
			DefaultGesture: "Alt+Space",
			CurrentGesture: func() string { return shortcut },
			Callback:       a.ToggleLauncher,
		})
	}

	for _, p := range a.snapshotPlugins() {
		for _, spec := range p.Hotkeys() {
			registerSpec(spec)
		}
	}

	if len(takeoverBindings) > 0 && !a.headless {
		if a.winTakeover == nil {
			a.winTakeover = platform.NewHotkeyTakeover(a.IsGlobalHookSuppressed)
		}
		if err := a.winTakeover.SetBindings(takeoverBindings); err != nil {
			for _, b := range takeoverBindings {
				msg := i18n.F("Win 钩子接管失败: %v", "Win key hook failed: %v", err)
				nextErrByID[b.ID] = msg
				delete(nextTakeover, b.ID)
				if cmd := cmdByID[b.ID]; cmd != "" {
					nextErrs[cmd] = msg
				}
			}
			if a.log != nil {
				a.log.Warnf("安装 Win 组合键接管钩子失败: %v", err)
			}
		}
	} else if a.winTakeover != nil {
		a.winTakeover.Close()
	}

	a.hotkeyMu.Lock()
	a.hotkeys = nextHotkeys
	a.takeoverHotkeys = nextTakeover
	a.hotkeyErrs = nextErrs
	a.hotkeyErrByID = nextErrByID
	a.hotkeyMu.Unlock()

	if len(nextErrByID) == 0 && a.log != nil {
		a.log.Infof("热键已注册")
	}
}
