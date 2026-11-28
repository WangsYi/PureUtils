package host

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/pkg/purekit/platform"
	pureui "pureutils/pkg/purekit/ui"
)

type settingsNavPill struct{}

type settingsSection struct {
	id         string
	pluginID   string
	label      string
	title      string
	sub        string
	icon       *ui.SVG
	tab        *purekit.SettingsTab
	pluginView purekit.SettingsTabView
}

type hotkeyEntry struct {
	pluginID string
	spec     purekit.HotkeySpec
	orig     string
	draft    string
}

type settingsView struct {
	app    *App
	win    *mygo.Window
	closed bool

	section  string
	tab      int
	sections []settingsSection

	hostOrig    config.HostConfig
	hostDraft   config.HostConfig
	modOrig     core.ModuleConfig
	modDraft    core.ModuleConfig
	themeIdx    int
	langIdx     int
	launcherMax float64

	hotkeys []hotkeyEntry

	updateIntervalIdx int

	recorder         *platform.HotkeyRecorder
	recordingID      string
	recordingErr     string
	status           string
	statusIsRecorded bool
	saving           bool
}

// ShowSettings opens or focuses the unified Settings window, optionally navigating to tabID.
func (a *App) ShowSettings(tabID string) {
	if a.log != nil {
		a.log.Infof("打开设置窗口: tab=%q reuse=%v", tabID, a.settingsWin != nil)
	}
	if a.settingsWin != nil {
		if tabID != "" && a.settingsView != nil {
			a.settingsView.selectSection(tabID)
		}
		a.settingsWin.Show()
		a.settingsWin.Focus()
		a.settingsWin.Invalidate()
		return
	}

	h := newSettingsView(a, tabID)
	a.settingsView = h
	if a.headless {
		return
	}

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:          i18n.S("PureUtils 设置", "PureUtils Settings"),
		Width:          920,
		Height:         660,
		MinWidth:       720,
		MinHeight:      540,
		StateKey:       "settings",
		Vibrancy:       mygo.VibrancyTabbed,
		TitleBarStyle:  mygo.TitleBarHidden,
		TitleBarHeight: 40,
		Content:        ui.View(h.view),
	})
	_ = win.SetIcon(iconPNG)
	h.win = win
	win.OnClose(func(_ *mygo.CloseEvent) {
		h.closed = true
		h.stopRecorder()
		if a.settingsWin == win {
			a.settingsWin = nil
			a.settingsView = nil
		}
		a.NotifyWindowClosed()
	})
	a.settingsWin = win
	if a.log != nil {
		a.log.Infof("设置窗口已创建: section=%q", h.section)
	}
	win.Focus()
}

func newSettingsView(a *App, initialTabID string) *settingsView {
	h := &settingsView{
		app:     a,
		section: "general",
	}
	h.buildSections()
	h.loadFromHost()
	if initialTabID != "" {
		h.selectSection(initialTabID)
	}
	return h
}

func (h *settingsView) buildSections() {
	type collectedTab struct {
		pluginID string
		tab      *purekit.SettingsTab
	}
	type collectedHotkey struct {
		pluginID string
		spec     purekit.HotkeySpec
	}

	var pluginTabs []collectedTab
	specs := []collectedHotkey{
		{
			spec: purekit.HotkeySpec{
				ID:             "host.launcher",
				Command:        core.CmdOpenLauncher,
				Label:          i18n.S("快捷启动器", "Quick Launcher"),
				DefaultGesture: "Alt+Space",
				CurrentGesture: func() string {
					return h.app.HostConfig().Launcher.Shortcut
				},
				SetGesture: func(g string) error {
					hc := h.app.HostConfig()
					hc.Launcher.Shortcut = g
					hc.Hotkeys.Launcher = g
					return h.app.SaveHostConfig(hc)
				},
				Callback: h.app.ToggleLauncher,
			},
		},
	}

	for _, p := range h.app.snapshotPlugins() {
		if tab := p.SettingsTab(); tab != nil && tab.NewView != nil {
			pluginTabs = append(pluginTabs, collectedTab{pluginID: p.ID(), tab: tab})
		}
		for _, hk := range p.Hotkeys() {
			if hk.SetGesture != nil {
				specs = append(specs, collectedHotkey{pluginID: p.ID(), spec: hk})
			}
		}
	}
	sort.Slice(pluginTabs, func(i, j int) bool {
		if pluginTabs[i].tab.Order != pluginTabs[j].tab.Order {
			return pluginTabs[i].tab.Order < pluginTabs[j].tab.Order
		}
		return pluginTabs[i].tab.ID < pluginTabs[j].tab.ID
	})

	secs := []settingsSection{
		{
			id:    "general",
			label: i18n.S("常规", "General"),
			title: i18n.S("常规与外观", "General & Appearance"),
			sub:   "",
			icon:  pureui.IconSliders,
		},
	}
	for _, pt := range pluginTabs {
		secs = append(secs, settingsSection{
			id:         pt.tab.ID,
			pluginID:   pt.pluginID,
			label:      pt.tab.Title,
			title:      pt.tab.Title,
			sub:        pt.tab.Subtitle,
			icon:       pt.tab.Icon,
			tab:        pt.tab,
			pluginView: pt.tab.NewView(h.app),
		})
	}
	secs = append(secs, settingsSection{
		id:    "advanced",
		label: i18n.S("高级", "Advanced"),
		title: i18n.S("高级与维护", "Advanced & Updates"),
		sub:   "",
		icon:  pureui.IconWrench,
	})
	h.sections = secs

	h.hotkeys = make([]hotkeyEntry, 0, len(specs))
	for _, ch := range specs {
		var cur string
		if ch.spec.CurrentGesture != nil {
			cur = strings.TrimSpace(ch.spec.CurrentGesture())
		}
		h.hotkeys = append(h.hotkeys, hotkeyEntry{
			pluginID: ch.pluginID,
			spec:     ch.spec,
			orig:     cur,
			draft:    cur,
		})
	}
}

func (h *settingsView) loadFromHost() {
	hc := h.app.HostConfig()
	h.hostOrig = hc.Clone()
	h.hostDraft = hc.Clone()
	active := h.app.ActiveModules()
	h.modOrig = active
	h.modDraft = active
	h.themeIdx = themeIndex(hc.Appearance.Theme)
	h.langIdx = langIndex(hc.Appearance.Language)
	h.launcherMax = float64(hc.Launcher.MaxResults)
	h.updateIntervalIdx = updateIntervalIndex(hc.Update.CheckIntervalHours)
	for i := range h.hotkeys {
		var cur string
		if h.hotkeys[i].spec.CurrentGesture != nil {
			cur = strings.TrimSpace(h.hotkeys[i].spec.CurrentGesture())
		}
		h.hotkeys[i].orig = cur
		h.hotkeys[i].draft = cur
	}
}

func (h *settingsView) refreshForLanguageChange() {
	if h.closed {
		return
	}
	prevSection := h.section
	h.buildSections()
	h.selectSection(prevSection)
	h.loadFromHost()
	if h.win != nil {
		h.win.SetTitle(i18n.S("PureUtils 设置", "PureUtils Settings"))
	}
	h.invalidate()
}

func (h *settingsView) selectSection(id string) {
	for i, s := range h.sections {
		if s.id == id {
			h.section = id
			h.tab = i
			return
		}
	}
	if len(h.sections) > 0 {
		h.section = h.sections[0].id
		h.tab = 0
	}
}

func (h *settingsView) tabIndex(id string) int {
	for i, s := range h.sections {
		if s.id == id {
			return i
		}
	}
	return 0
}

func (h *settingsView) view(c *ui.Context) {
	pal := pureui.ApplyTheme(c)
	if idx := h.tabIndex(h.section); idx != h.tab {
		h.tab = idx
	}
	ui.Column(c).Fill().Background(pal.Bg).Children(func() {
		pureui.WindowTitleBar(c, pal, pureui.IconSettings, pal.Accent, pal.AccentEnd,
			i18n.S("PureUtils 设置", "PureUtils Settings"), core.ShortVersion(core.AppVersion), 40, nil)
		ui.Row(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
			h.sidebar(c, pal)
			if !pal.Vibrant {
				ui.Box(c).Width(1).FillHeight().Background(pal.Border)
			}
			h.contentPane(c, pal)
		})
	})
}

func (h *settingsView) sidebar(c *ui.Context, pal pureui.Palette) {
	sideBg := pal.SidebarBg
	if pal.Vibrant {
		sideBg = ui.Transparent
	}
	pillCol := pal.Accent
	if pal.Dark {
		pillCol = pal.AccentHover
	}
	hoverBg := pal.CardBg
	if !pal.Dark && !pal.Vibrant {
		hoverBg = pal.CardHover
	}

	ui.Column(c).Width(220).FillHeight().Background(sideBg).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(4, 10, 10, 10).Gap(3).Children(func() {
				ui.Text(c, i18n.S("设置分类", "Sections")).FontSize(10.5).FontWeight(600).Padding(4, 12, 4, 12).TextColor(pal.TextMuted)
				for i, sec := range h.sections {
					i := i
					sec := sec
					active := h.section == sec.id
					btn := ui.ButtonBase(c.Key(sec.id)).FillWidth().Height(36).Padding(0, 12, 0, 8).Radius(7).
						Gap(10).AlignItems(ui.Center).Label(sec.label).Transition(pureui.HoverTransition)

					bg := ui.Transparent
					fg := pal.TextSecondary
					iconFg := pal.TextMuted
					weight := 500
					switch {
					case active:
						bg = pal.CardHover
						fg = pal.Text
						iconFg = pillCol
						weight = 600
						if btn.Pressed() {
							bg = pal.CardActive
						}
					case btn.Pressed():
						bg = pal.CardActive
						fg = pal.Text
						iconFg = pal.TextSecondary
					case btn.Hovered():
						bg = hoverBg
						fg = pal.Text
						iconFg = pal.TextSecondary
					}
					btn.Background(bg)
					btn.Children(func() {
						ui.Box(c).Size(3, 16).Shrink(0).Center().Children(func() {
							if active {
								ui.Box(c.Key("nav-pill")).Size(3, 16).Radius(1.5).Background(pillCol).
									Transition(ui.ElementTransition{Match: settingsNavPill{}, Animation: &pureui.NavPillSpring})
							}
						})
						ui.Icon(c, sec.icon).Size(15, 15).TextColor(iconFg).Shrink(0)
						ui.Text(c, sec.label).FontSize(13).FontWeight(weight).SingleLine().TextColor(fg).Grow(1).MinWidth(0)
					})
					btn.OnClick(func() {
						h.section = sec.id
						h.tab = i
						h.invalidate()
					})
				}
			})
		})

		ui.Column(c).Margin(6, 10, 12, 10).Padding(10, 12).Gap(4).Radius(10).
			Background(pal.CardBg).Border(1, pal.Border).InsetShadow(0, 1, 0, 0, pal.Specular).Children(func() {
			h.renderSidebarStatus(c, pal)
			ui.Text(c, h.app.Paths().Home).FontSize(10).SingleLine().TextColor(pal.TextMuted)
		})
	})
}

func (h *settingsView) renderSidebarStatus(c *ui.Context, pal pureui.Palette) {
	plugins := h.app.snapshotPlugins()
	for _, p := range plugins {
		for _, item := range p.TrayItems() {
			if item.ID == "pureclip.monitoring" {
				dot := pal.Success
				monText := i18n.S("剪贴板监听中", "Clipboard Monitoring Active")
				if !item.Checked {
					dot = pal.Warning
					monText = i18n.S("剪贴板监听已暂停", "Clipboard Monitoring Paused")
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
					ui.Text(c, monText).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.TextSecondary)
				})
				return
			}
		}
	}
	var names []string
	for _, p := range plugins {
		names = append(names, p.Info().Name)
	}
	summary := i18n.S("无启用插件", "No active plugins")
	if len(names) > 0 {
		summary = strings.Join(names, " · ")
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.Accent)
		ui.Text(c, summary).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.TextSecondary)
	})
}

func (h *settingsView) contentPane(c *ui.Context, pal pureui.Palette) {
	sec := h.sections[clampInt(h.tab, 0, len(h.sections)-1)]
	ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(pal.Bg).Children(func() {
		ui.Column(c).Padding(6, 26, 10, 26).Gap(3).Children(func() {
			ui.Text(c, sec.title).FontSize(24).FontWeight(600).TextColor(pal.Text).ContentTransition(ui.ContentTransition{})
			if sec.sub != "" {
				ui.Text(c, sec.sub).FontSize(12).TextColor(pal.TextMuted)
			}
		})
		if !pal.Vibrant {
			ui.Box(c).Height(1).FillWidth().Background(pal.Border)
		}

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c.Key(sec.id)).Padding(6, 26, 18, 26).Gap(14).Transition(pureui.PageEnterTransition).Children(func() {
				switch sec.id {
				case "general":
					h.general(c, pal)
				case "advanced":
					h.advanced(c, pal)
				default:
					if sec.pluginView != nil {
						sec.pluginView.Render(c, pal)
					}
				}
			})
		})

		h.bottomSaveBar(c, pal)
	})
}

func (h *settingsView) bottomSaveBar(c *ui.Context, pal pureui.Palette) {
	dirty := h.isDirty()
	ui.Box(c).Height(1).FillWidth().Background(pal.Border)
	ui.Row(c).Padding(10, 20).Gap(8).AlignItems(ui.Center).Background(pal.SidebarBg).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
			switch {
			case h.recordingID != "":
				if h.recordingErr != "" {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.Danger).Shrink(0)
					ui.Text(c, h.recordingErr).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.Danger).ContentTransition(ui.ContentTransition{})
				} else {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.Accent).Shrink(0)
					ui.Text(c, i18n.S("录制中…请按下组合键（按 Esc 取消）", "Recording… press a key combination (Esc to cancel)")).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.Accent).ContentTransition(ui.ContentTransition{})
				}
			case dirty:
				if h.statusIsRecorded && h.status != "" {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.Accent).Shrink(0)
					ui.Text(c, h.status).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.Accent).ContentTransition(ui.ContentTransition{})
				} else {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.Warning).Shrink(0)
					ui.Text(c, i18n.S("有未保存的更改", "Unsaved changes")).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.Warning).ContentTransition(ui.ContentTransition{})
				}
			case h.status != "":
				ui.Text(c, h.status).FontSize(11).SingleLine().TextColor(pal.TextMuted).ContentTransition(ui.ContentTransition{})
			default:
				ui.Text(c, i18n.S("配置保存后立即生效", "Changes take effect immediately after saving")).FontSize(11).SingleLine().TextColor(pal.TextMuted).ContentTransition(ui.ContentTransition{})
			}
		})
		ui.Button(c, i18n.S("放弃更改", "Discard")).Disabled(!dirty).Shrink(0).OnClick(func() {
			h.stopRecorder()
			h.loadFromHost()
			for i := range h.sections {
				if h.sections[i].tab != nil && h.sections[i].tab.NewView != nil {
					h.sections[i].pluginView = h.sections[i].tab.NewView(h.app)
				}
			}
			h.statusIsRecorded = false
			h.status = i18n.S("已放弃未保存的更改", "Unsaved changes discarded")
			h.invalidate()
		}).Border(0, ui.Transparent)
		ui.Button(c, i18n.S("关闭", "Close")).Shrink(0).OnClick(func() { h.close() }).Border(0, ui.Transparent)
		ui.PrimaryButton(c, i18n.S("保存", "Save")).Shrink(0).OnClick(func() { h.save(c) })
	})
}

func (h *settingsView) general(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, i18n.S("外观与显示", "Appearance & Display"), "", pal, func() {
		pureui.SettingRow(c, i18n.S("主题模式", "Theme"), "", pal, func() {
			seg := ui.Segmented(c, &h.themeIdx, i18n.S("跟随系统", "System"), i18n.S("浅色", "Light"), i18n.S("深色", "Dark")).Label(i18n.S("主题", "Theme"))
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				h.hostDraft.Appearance.Theme = themeKeyFromIndex(h.themeIdx)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("界面语言", "Language"), "", pal, func() {
			seg := ui.Segmented(c, &h.langIdx, i18n.S("跟随系统", "Auto"), "简体中文", "English").Label(i18n.S("界面语言", "Language"))
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				h.hostDraft.Appearance.Language = langPrefFromIndex(h.langIdx)
			}
		})
	})

	pureui.SettingCardTip(c, i18n.S("全局热键", "Global Hotkeys"), i18n.S("冲突或无效热键会在对应行下方提示，保存后立即生效", "Conflicts or invalid shortcuts appear below each row and take effect on Save"), pal, func() {
		if len(h.hotkeys) == 0 {
			ui.Text(c, i18n.S("当前没有启用提供全局热键的功能插件", "No enabled modules provide global hotkeys")).FontSize(12).TextColor(pal.TextMuted)
			return
		}
		for i := range h.hotkeys {
			if i > 0 {
				ui.Divider(c)
			}
			h.hotkeyRow(c, i, pal)
		}
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("接管系统 Win 组合键", "Take Over System Win Hotkeys"), i18n.S("当 Win+V、Win+Shift+S、Win+Space 等被系统占用时，自动通过底层键盘钩子接管并屏蔽开始菜单（不支持 Win+L）", "Intercept reserved Win shortcuts like Win+V or Win+Shift+S via low-level hook (Win+L excluded)"), pal, func() {
			ui.Switch(c, &h.hostDraft.Hotkeys.TakeoverWinKeys).Label(i18n.S("开关：接管系统 Win 组合键", "Toggle: Take over system Win hotkeys"))
		})
	})

	pureui.SettingCardTip(c, i18n.S("快捷启动器", "Quick Launcher"), i18n.S("按下全局热键（默认 Alt+Space）快速搜索插件能力并自动识别当前剪贴板或资源管理器选中项", "Search all plugin actions, clipboard history, and Explorer selections (default Alt+Space)"), pal, func() {
		pureui.SettingRow(c, i18n.S("启用快捷启动器", "Enable Quick Launcher"), "", pal, func() {
			ui.Switch(c, &h.hostDraft.Launcher.Enabled).Label(i18n.S("开关：启用快捷启动器", "Toggle: Enable Quick Launcher"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("最多显示结果数", "Max Results"), "", pal, func() {
			if ui.NumberInput(c, &h.launcherMax, 4, 30, 1).Label(i18n.S("数值：启动器最大结果数", "Number: Launcher max results")).Changed() {
				h.hostDraft.Launcher.MaxResults = int(h.launcherMax)
			}
		})
	})
}

func (h *settingsView) hotkeyRow(c *ui.Context, idx int, pal pureui.Palette) {
	entry := &h.hotkeys[idx]
	spec := entry.spec
	recording := h.recordingID == spec.ID
	errMsg := h.hotkeyError(idx)

	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, spec.Label).FontSize(13).FontWeight(500).SingleLine().TextColor(pal.Text)
				if spec.DefaultGesture != "" {
					ui.Text(c, i18n.F("默认 %s", "Default: %s", spec.DefaultGesture)).FontSize(10.5).SingleLine().TextColor(pal.TextMuted)
				}
			})
			ui.TextInput(c, &entry.draft).Width(148).Label(i18n.S("快捷键：", "Shortcut: ") + spec.Label).Border(0, ui.Transparent).FocusRing(false)

			recBtn := ui.ButtonBase(c).Padding(5, 11).Radius(7).Gap(5).AlignItems(ui.Center).Shrink(0).
				Label(i18n.S("录制", "Record ") + spec.Label).Tooltip(i18n.S("按下组合键录制热键", "Press key combination to record")).
				Transition(pureui.HoverTransition)
			recBg := pal.ControlBg
			recFg := pal.TextSecondary
			recText := i18n.S("录制", "Record")
			if recording {
				recBg = pal.AccentSoft
				recFg = pal.Accent
				recText = i18n.S("停止", "Stop")
			} else {
				recBtn.Border(1, pal.Border)
				if recBtn.Pressed() {
					recBg = pal.CardActive
				} else if recBtn.Hovered() {
					recBg = pal.CardHover
					recFg = pal.Text
				}
			}
			recBtn.Background(recBg)
			recBtn.Children(func() {
				ui.Icon(c, pureui.IconRecord).Size(12, 12).TextColor(recFg)
				ui.Text(c, recText).FontSize(11).FontWeight(500).SingleLine().TextColor(recFg)
			})
			recBtn.OnClick(func() {
				if recording {
					h.stopRecorder()
					h.statusIsRecorded = false
					h.status = i18n.S("已取消录制", "Recording canceled")
					h.invalidate()
				} else {
					h.startRecording(spec.ID)
				}
			})

			resetTooltip := i18n.F("恢复默认热键 (%s)", "Reset to default (%s)", spec.DefaultGesture)
			if spec.DefaultGesture == "" {
				resetTooltip = i18n.S("清空热键", "Clear shortcut")
			}
			pureui.IconButton(c, pureui.IconRotateCcw, i18n.S("恢复默认", "Reset ")+spec.Label, resetTooltip, false, false, pal).
				Disabled(entry.draft == spec.DefaultGesture).
				OnClick(func() {
					entry.draft = spec.DefaultGesture
					h.invalidate()
				})
		})
		if errMsg != "" {
			ui.Text(c, errMsg).FontSize(11).TextColor(pal.Danger)
		}
	})
}

func (h *settingsView) hotkeyError(idx int) string {
	entry := &h.hotkeys[idx]
	text := strings.TrimSpace(entry.draft)
	if text == "" {
		return ""
	}
	g, err := core.ParseGesture(text)
	if err != nil {
		return err.Error()
	}
	if g.Modifiers == core.ModWin && g.Key == "L" {
		return i18n.S("Win+L 是系统锁屏，不能占用", "Win+L is reserved by Windows Lock Screen")
	}
	if msg := h.app.HotkeyErrorByID(entry.spec.ID); msg != "" {
		return msg
	}
	if entry.spec.Command != "" {
		return h.app.HotkeyError(entry.spec.Command)
	}
	return ""
}

func (h *settingsView) advanced(c *ui.Context, pal pureui.Palette) {
	h.renderUpdateCard(c, pal)

	pureui.SettingCard(c, i18n.S("系统与日志", "System & Logs"), "", pal, func() {
		pureui.SettingRow(c, i18n.S("开机自动启动", "Launch on Startup"), "", pal, func() {
			ui.Switch(c, &h.hostDraft.Advanced.RunOnStartup).Label(i18n.S("开关：开机自动启动", "Toggle: Launch on Startup"))
		})
		ui.Divider(c)
		pureui.SettingRowTip(
			c,
			i18n.S("开机以管理员身份自启（免 UAC）", "Launch Elevated at Logon (No UAC)"),
			i18n.S("通过 Windows 计划任务在登录时以最高权限静默自启，避免每次弹窗且让全局热键在管理员窗口上也可生效", "Register a Windows logon scheduled task with highest privileges to avoid UAC prompts and keep global hotkeys active over elevated windows"),
			pal,
			func() {
				sw := ui.Switch(c, &h.hostDraft.Advanced.RunElevatedOnStartup).Label(i18n.S("开关：开机以管理员身份自启", "Toggle: Launch Elevated on Startup"))
				if sw.Changed() && h.hostDraft.Advanced.RunElevatedOnStartup {
					h.hostDraft.Advanced.RunOnStartup = true
				}
			},
		)
		ui.Divider(c)
		pureui.SettingRowTip(
			c,
			i18n.S("当前运行权限", "Privilege Level"),
			i18n.S("普通权限下，当焦点位于以管理员运行的程序（如 Argus / 任务管理器）上时，Windows 会拦截全局热键（UIPI）", "When running as a standard user, Windows UIPI blocks global hotkeys while an elevated window has focus"),
			pal,
			func() {
				if h.app.IsElevated() {
					pureui.PillBadge(c, i18n.S("管理员权限", "Administrator"), pal.Success, pal.SuccessSoft)
				} else {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						pureui.PillBadge(c, i18n.S("普通权限", "Standard User"), pal.TextMuted, pal.ControlBg)
						ui.Button(c, i18n.S("以管理员身份重启", "Restart as Admin")).OnClick(func() {
							if err := h.app.RestartAsAdmin(); err != nil {
								h.statusIsRecorded = false
								h.status = i18n.S("提权重启失败: ", "Elevated restart failed: ") + err.Error()
								h.invalidate()
							}
						}).Border(0, ui.Transparent)
					})
				}
			},
		)
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("记录详细日志", "Verbose Logging"), i18n.S("写入更多调试上下文到每日日志文件以便排查问题", "Write detailed diagnostic context to daily log files"), pal, func() {
			ui.Switch(c, &h.hostDraft.Advanced.VerboseLogging).Label(i18n.S("开关：记录详细日志", "Toggle: Verbose Logging"))
		})
	})

	pureui.SettingCard(c, i18n.S("存储与维护", "Storage & Data"), i18n.S("数据目录：", "Data directory: ")+h.app.Paths().Home, pal, func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c, i18n.S("打开数据目录", "Open Data Folder")).OnClick(func() {
				if err := mygo.Shell.OpenPath(h.app.Paths().Home); err != nil {
					h.statusIsRecorded = false
					h.status = i18n.S("打开目录失败: ", "Failed to open folder: ") + err.Error()
				}
				h.invalidate()
			}).Border(0, ui.Transparent)
		})
	})

	pureui.SettingCardTip(c, i18n.S("已安装模块", "Installed Modules"), i18n.S("可实时开启或关闭已安装的组件，保存后立即生效（无需重启）；精简构建未包含的组件无法开启", "Enable or disable modules live without restarting; excluded modules in Core builds cannot be enabled"), pal, func() {
		type builtinRow struct {
			id        string
			shortName string
			mod       core.ModuleID
			title     string
			desc      string
			compiled  bool
			draftPtr  *bool
		}
		builtins := []builtinRow{
			{"pureshot", "PureShot", core.ModuleCapture, "PureShot", i18n.S("区域/整屏截图、9 种标注与置顶悬浮贴图", "Region/full-screen capture, 9 annotations & always-on-top pins"), h.app.compiled.Capture, &h.modDraft.Capture},
			{"pureclip", "PureClip", core.ModuleHistory, "PureClip", i18n.S("自动监听文本/富文本/图片/文件并持久化检索", "Auto-capture text/rich text/images/files with instant search"), h.app.compiled.History, &h.modDraft.History},
			{"pureimage", "PureImage", core.ModuleViewer, "PureImage", i18n.S("支持 JPEG Turbo/HEIC/AVIF/JXL/RAW、动图与资源管理器空格预览", "JPEG Turbo/HEIC/AVIF/JXL/RAW, animations & Explorer space preview"), h.app.compiled.Viewer, &h.modDraft.Viewer},
			{"purevideo", "PureVideo", core.ModuleVideo, "PureVideo", i18n.S("内置 libmpv 硬件解码，支持 21 种常见视频格式与外挂字幕", "Built-in libmpv hardware decoding for 21 formats & external subtitles"), h.app.compiled.Video, &h.modDraft.Video},
			{"purezip", "PureZip", core.ModuleZip, "PureZip", i18n.S("浏览/智能解压/多线程压缩/加密/右键菜单与文件关联", "Browse, smart extract, multi-threaded compress, encrypt & shell integration"), h.app.compiled.Zip, &h.modDraft.Zip},
			{"puremonitor", "PureMonitor", core.ModuleMonitor, "PureMonitor", i18n.S("任务栏/悬浮窗系统状态监控、硬件配置详情与游戏帧率 HUD", "Taskbar/floating system monitor, hardware specs & game FPS HUD"), h.app.compiled.Monitor, &h.modDraft.Monitor},
		}
		installed := h.app.InstalledModules()
		seen := map[string]bool{}
		for i, b := range builtins {
			if i > 0 {
				ui.Divider(c)
			}
			seen[b.id] = true
			active := h.app.ModuleEnabled(b.mod)
			pureui.SettingRow(c, b.title, b.desc, pal, func() {
				switch {
				case !b.compiled:
					pureui.PillBadge(c, i18n.S("精简构建未包含", "Not in Core build"), pal.TextMuted, pal.Bg)
				case h.app.custom:
					if active {
						pureui.PillBadge(c, i18n.S("已启用", "Enabled"), pal.Success, pal.SuccessSoft)
					} else {
						pureui.PillBadge(c, i18n.S("未启用", "Disabled"), pal.TextMuted, pal.Bg)
					}
				case !installed.Enabled(b.mod):
					pureui.PillBadge(c, i18n.S("未安装", "Not Installed"), pal.TextMuted, pal.Bg)
				default:
					ui.Switch(c, b.draftPtr).Label(i18n.S("开关：启用 ", "Toggle: Enable ") + b.shortName)
				}
			})
		}
		for _, p := range h.app.snapshotPlugins() {
			info := p.Info()
			if seen[info.ID] {
				continue
			}
			ui.Divider(c)
			pureui.SettingRow(c, info.Name, info.Description, pal, func() {
				pureui.PillBadge(c, i18n.S("已启用", "Enabled"), pal.Success, pal.SuccessSoft)
			})
		}
	})
}

func (h *settingsView) renderUpdateCard(c *ui.Context, pal pureui.Palette) {
	var snap UpdateSnapshot
	if h.app.updater != nil {
		snap = h.app.updater.Snapshot()
	} else {
		snap = UpdateSnapshot{
			Status:         UpdateIdle,
			CurrentVersion: core.AppVersion,
		}
	}

	sub := i18n.F("当前版本 v%s", "Current v%s", core.ShortVersion(snap.CurrentVersion))
	if !snap.LastCheck.IsZero() {
		sub += i18n.S(" · 上次检查 ", " · Last checked ") + snap.LastCheck.Format("01-02 15:04")
	}

	pureui.SettingCard(c, i18n.S("软件更新", "Software Updates"), sub, pal, func() {
		badgeLabel := i18n.S("未检查", "Not Checked")
		badgeFg, badgeBg := pal.TextMuted, pal.Bg
		statusTitle := i18n.S("版本状态", "Version Status")
		statusDesc := ""
		switch snap.Status {
		case UpdateChecking:
			badgeLabel = i18n.S("检查中…", "Checking…")
			badgeFg, badgeBg = pal.Accent, pal.AccentSoft
			statusDesc = snap.Stage
		case UpdateUpToDate:
			badgeLabel = i18n.S("已是最新", "Up to Date")
			badgeFg, badgeBg = pal.Success, pal.SuccessSoft
			statusDesc = i18n.S("当前已是最新版本 v", "You are on the latest version v") + core.ShortVersion(snap.CurrentVersion)
		case UpdateAvailable:
			badgeLabel = i18n.S("有新版本", "Update Available")
			badgeFg, badgeBg = pal.Accent, pal.AccentSoft
			if snap.Release != nil {
				statusTitle = i18n.S("发现新版本 ", "New Version ") + snap.Release.TagName
				if snap.Release.AssetSize > 0 {
					statusDesc = i18n.F("安装包：%s（%.1f MB）", "Package: %s (%.1f MB)", snap.Release.AssetName, float64(snap.Release.AssetSize)/(1024*1024))
				} else {
					statusDesc = i18n.S("安装包：", "Package: ") + snap.Release.AssetName
				}
			}
		case UpdateDownloading:
			badgeLabel = i18n.S("正在下载", "Downloading")
			badgeFg, badgeBg = pal.Accent, pal.AccentSoft
			if snap.Release != nil {
				statusTitle = i18n.S("正在下载 ", "Downloading ") + snap.Release.TagName
			}
			statusDesc = snap.Stage
		case UpdateReady:
			badgeLabel = i18n.S("待安装", "Ready")
			badgeFg, badgeBg = pal.Success, pal.SuccessSoft
			if snap.Release != nil {
				statusTitle = i18n.F("新版本 %s 已就绪", "Version %s Ready", snap.Release.TagName)
			}
			statusDesc = i18n.S("点击「立即安装并重启」将自动完成升级并重新启动", "Click Install & Restart to upgrade automatically")
		case UpdateInstalling:
			badgeLabel = i18n.S("正在安装…", "Installing…")
			badgeFg, badgeBg = pal.Accent, pal.AccentSoft
			statusDesc = snap.Stage
		case UpdateError:
			badgeLabel = i18n.S("检查异常", "Error")
			badgeFg, badgeBg = pal.Danger, pal.DangerSoft
			if snap.Err != "" {
				statusDesc = snap.Err
			} else {
				statusDesc = snap.Stage
			}
		}

		pureui.SettingRow(c, statusTitle, statusDesc, pal, func() {
			pureui.PillBadge(c, badgeLabel, badgeFg, badgeBg)
		})

		if snap.Release != nil && strings.TrimSpace(snap.Release.Body) != "" &&
			(snap.Status == UpdateAvailable || snap.Status == UpdateDownloading || snap.Status == UpdateReady) {
			notes := truncateUpdateNotes(snap.Release.Body, 240)
			ui.Column(c).FillWidth().Padding(8, 10).Radius(8).Background(pal.Bg).Children(func() {
				ui.Text(c, notes).FontSize(11.5).TextColor(pal.TextSecondary)
			})
		}

		if snap.Status == UpdateDownloading {
			pct := 0.0
			if snap.TotalBytes > 0 {
				pct = float64(snap.DoneBytes) / float64(snap.TotalBytes)
				if pct > 1 {
					pct = 1
				}
			}
			ui.Column(c).Gap(6).FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, snap.Stage).FontSize(11.5).FontWeight(500).TextColor(pal.Text).Grow(1).MinWidth(0)
					if snap.TotalBytes > 0 {
						ui.Text(c, fmt.Sprintf("%.1f MB / %.1f MB",
							float64(snap.DoneBytes)/(1024*1024),
							float64(snap.TotalBytes)/(1024*1024),
						)).FontSize(11).TextColor(pal.TextSecondary).Shrink(0)
					} else if snap.DoneBytes > 0 {
						ui.Text(c, fmt.Sprintf("%.1f MB", float64(snap.DoneBytes)/(1024*1024))).
							FontSize(11).TextColor(pal.TextSecondary).Shrink(0)
					}
				})
				ui.Progress(c, pct).FillWidth().Height(6).Radius(3)
			})
		}

		ui.Row(c).Gap(8).Wrap().Children(func() {
			switch snap.Status {
			case UpdateDownloading:
				ui.Button(c, i18n.S("取消下载", "Cancel Download")).OnClick(func() {
					h.app.CancelUpdateDownload()
				}).Border(0, ui.Transparent)
			case UpdateReady:
				ui.PrimaryButton(c, i18n.S("立即安装并重启", "Install & Restart")).OnClick(func() {
					if err := h.app.InstallUpdate(); err != nil {
						h.statusIsRecorded = false
						h.status = i18n.S("安装更新失败: ", "Failed to install update: ") + err.Error()
						h.invalidate()
					}
				})
				if snap.Release != nil && snap.Release.HTMLURL != "" {
					urlStr := snap.Release.HTMLURL
					ui.Button(c, i18n.S("查看发布页", "Release Notes")).OnClick(func() {
						if !h.app.headless {
							_ = mygo.Shell.OpenExternal(urlStr)
						}
					}).Border(0, ui.Transparent)
				}
			case UpdateAvailable:
				ui.PrimaryButton(c, i18n.S("下载更新", "Download Update")).OnClick(func() {
					h.app.DownloadUpdate()
				})
				if snap.Release != nil && snap.Release.TagName != "" {
					tag := snap.Release.TagName
					ui.Button(c, i18n.S("忽略此版本", "Skip Version")).OnClick(func() {
						h.statusIsRecorded = false
						if err := h.app.SkipUpdateVersion(tag); err != nil {
							h.status = i18n.S("记录忽略版本失败: ", "Failed to skip version: ") + err.Error()
						} else {
							h.status = i18n.S("已忽略后台提示：", "Skipped background prompt for ") + tag
							h.hostOrig.Update.SkippedVersion = tag
							h.hostDraft.Update.SkippedVersion = tag
						}
						h.invalidate()
					}).Border(0, ui.Transparent)
				}
				if snap.Release != nil && snap.Release.HTMLURL != "" {
					urlStr := snap.Release.HTMLURL
					ui.Button(c, i18n.S("查看发布页", "Release Notes")).OnClick(func() {
						if !h.app.headless {
							_ = mygo.Shell.OpenExternal(urlStr)
						}
					}).Border(0, ui.Transparent)
				}
				ui.Button(c, i18n.S("重新检查", "Recheck")).OnClick(func() {
					h.app.CheckForUpdates(true)
				}).Border(0, ui.Transparent)
			default:
				busy := snap.Status == UpdateChecking || snap.Status == UpdateInstalling
				ui.PrimaryButton(c, i18n.S("检查更新", "Check for Updates")).Disabled(busy).OnClick(func() {
					h.app.CheckForUpdates(true)
				})
			}
		})

		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("自动检查更新", "Auto-Check Updates"), i18n.S("定期在后台静默检查 GitHub Releases 新版本（仅在有新版本时提醒）", "Silently check GitHub Releases on schedule and notify only when a new version appears"), pal, func() {
			ui.Switch(c, &h.hostDraft.Update.AutoCheck).Label(i18n.S("开关：自动检查更新", "Toggle: Auto-Check Updates"))
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("发现新版本后自动下载", "Auto-Download Updates"), i18n.S("在后台通过所选镜像提前下载并校验安装包，待您点击安装时一键升级", "Pre-download and verify installers in the background for one-click install"), pal, func() {
			ui.Switch(c, &h.hostDraft.Update.AutoDownload).Label(i18n.S("开关：自动下载更新", "Toggle: Auto-Download Updates"))
		})
		ui.Divider(c)
		opts := UpdateDownloadSourceLabels()
		curSourceLabel := UpdateSourceLabel(h.hostDraft.Update.DownloadSource)
		pureui.SettingRowTip(c, i18n.S("更新下载源", "Download Source"), i18n.S("推荐保持「自动优选」，依次尝试加速镜像与 GitHub 官方源", "Auto tries mirrors before official GitHub"), pal, func() {
			sel := ui.Select(c, &curSourceLabel, opts).Width(220).Label(i18n.S("更新下载源", "Download Source"))
			sel.Border(0, ui.Transparent)
			if sel.Changed() {
				h.hostDraft.Update.DownloadSource = UpdateSourceIDFromLabel(curSourceLabel)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("检查频率", "Check Interval"), "", pal, func() {
			seg := ui.Segmented(c, &h.updateIntervalIdx, i18n.S("6 小时", "6h"), i18n.S("每天", "Daily"), i18n.S("3 天", "3d"), i18n.S("每周", "Weekly")).Label(i18n.S("更新检查频率", "Update Check Interval"))
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				h.hostDraft.Update.CheckIntervalHours = updateIntervalFromIndex(h.updateIntervalIdx)
			}
		})
	})
}

func (h *settingsView) startRecording(id string) {
	if h.recorder != nil {
		h.stopRecorder()
	}
	h.app.SuspendHotkeys()
	rec, err := platform.StartHotkeyRecorder()
	if err != nil {
		h.app.ReloadHotkeys()
		h.statusIsRecorded = false
		h.status = i18n.S("无法录制: ", "Cannot start recorder: ") + err.Error()
		h.invalidate()
		return
	}
	h.recorder = rec
	h.recordingID = id
	h.recordingErr = ""
	h.app.SetGlobalHookSuppressed("host:hotkey-recorder", true)
	h.statusIsRecorded = false
	h.status = i18n.S("正在录制，请按下组合键（Esc 取消）", "Recording… press keys (Esc to cancel)")
	h.invalidate()
	go func() {
		for res := range rec.Results() {
			mygo.RunOnMain(func() {
				if h.recorder == rec {
					h.onRecording(res)
				}
			})
		}
		mygo.RunOnMain(func() {
			if h.recorder != rec {
				return
			}
			h.recorder = nil
			h.recordingID = ""
			h.recordingErr = ""
			h.app.SetGlobalHookSuppressed("host:hotkey-recorder", false)
			h.app.ReloadHotkeys()
			h.invalidate()
		})
	}()
}

func (h *settingsView) onRecording(res platform.HotkeyResult) {
	if h.closed || h.recordingID == "" {
		return
	}
	switch {
	case res.Canceled:
		h.statusIsRecorded = false
		h.status = i18n.S("已取消录制", "Recording canceled")
		h.recordingID = ""
		h.recordingErr = ""
	case res.Err != "":
		h.statusIsRecorded = false
		h.recordingErr = res.Err
		h.status = res.Err
	case !res.Gesture.Empty():
		for i := range h.hotkeys {
			if h.hotkeys[i].spec.ID == h.recordingID {
				h.hotkeys[i].draft = res.Gesture.String()
				break
			}
		}
		h.statusIsRecorded = true
		if core.RequiresTakeover(res.Gesture) && !h.hostDraft.Hotkeys.TakeoverWinKeys {
			h.hostDraft.Hotkeys.TakeoverWinKeys = true
			h.status = i18n.F("已录制: %s（已自动开启 Win 组合键接管，点击保存生效）", "Recorded: %s (Win hotkey takeover enabled; click Save)", res.Gesture.String())
		} else {
			h.status = i18n.F("已录制: %s（点击保存生效）", "Recorded: %s (click Save to apply)", res.Gesture.String())
		}
		h.recordingID = ""
		h.recordingErr = ""
	}
	if h.recordingID == "" {
		h.app.SetGlobalHookSuppressed("host:hotkey-recorder", false)
		h.app.ReloadHotkeys()
	}
	h.invalidate()
}

func (h *settingsView) stopRecorder() {
	if h.recorder != nil {
		rec := h.recorder
		h.recorder = nil
		h.recordingID = ""
		h.recordingErr = ""
		rec.Close()
		h.app.SetGlobalHookSuppressed("host:hotkey-recorder", false)
		h.app.ReloadHotkeys()
	}
}

func (h *settingsView) isDirty() bool {
	if h.hostDraft.Appearance.Theme != h.hostOrig.Appearance.Theme ||
		h.hostDraft.Appearance.Language != h.hostOrig.Appearance.Language ||
		h.hostDraft.Hotkeys.TakeoverWinKeys != h.hostOrig.Hotkeys.TakeoverWinKeys ||
		h.hostDraft.Advanced.RunOnStartup != h.hostOrig.Advanced.RunOnStartup ||
		h.hostDraft.Advanced.RunElevatedOnStartup != h.hostOrig.Advanced.RunElevatedOnStartup ||
		h.hostDraft.Advanced.VerboseLogging != h.hostOrig.Advanced.VerboseLogging ||
		h.hostDraft.Launcher.Enabled != h.hostOrig.Launcher.Enabled ||
		int(h.launcherMax) != h.hostOrig.Launcher.MaxResults ||
		h.hostDraft.Update.AutoCheck != h.hostOrig.Update.AutoCheck ||
		h.hostDraft.Update.AutoDownload != h.hostOrig.Update.AutoDownload ||
		h.hostDraft.Update.CheckIntervalHours != h.hostOrig.Update.CheckIntervalHours ||
		h.hostDraft.Update.DownloadSource != h.hostOrig.Update.DownloadSource ||
		(!h.app.custom && h.modDraft != h.modOrig) {
		return true
	}
	for i := range h.hotkeys {
		if strings.TrimSpace(h.hotkeys[i].draft) != strings.TrimSpace(h.hotkeys[i].orig) {
			return true
		}
	}
	for _, sec := range h.sections {
		if sec.pluginView != nil && sec.pluginView.Dirty() {
			return true
		}
	}
	return false
}

func (h *settingsView) pluginActiveInDraft(pluginID string) bool {
	if pluginID == "" {
		return true
	}
	p := h.app.PluginByID(pluginID)
	if p == nil {
		return false
	}
	if h.app.custom {
		return true
	}
	mod := p.Info().ModuleID
	if mod == "" {
		return true
	}
	return h.modDraft.Enabled(mod)
}

func (h *settingsView) save(c *ui.Context) {
	h.saving = true
	defer func() { h.saving = false }()
	h.stopRecorder()
	h.statusIsRecorded = false
	prevLang := i18n.Current()

	modulesChanged := !h.app.custom && h.modDraft != h.modOrig
	if modulesChanged && h.app.InstalledModules().Any() && !h.modDraft.Any() {
		h.status = i18n.S("请至少保留一个已启用的功能组件", "Keep at least one module enabled")
		h.invalidate()
		return
	}

	normalizedHotkeys := make([]string, len(h.hotkeys))
	hasWinHotkey := false
	for i := range h.hotkeys {
		if !h.pluginActiveInDraft(h.hotkeys[i].pluginID) {
			normalizedHotkeys[i] = strings.TrimSpace(h.hotkeys[i].orig)
			continue
		}
		raw := strings.TrimSpace(h.hotkeys[i].draft)
		if raw == "" {
			normalizedHotkeys[i] = ""
			continue
		}
		g, err := core.ParseGesture(raw)
		if err != nil {
			h.status = i18n.S("热键无效: ", "Invalid shortcut: ") + err.Error()
			h.invalidate()
			return
		}
		if g.Modifiers == core.ModWin && g.Key == "L" {
			h.status = i18n.S("热键无效: Win+L 是系统锁屏，不能占用", "Invalid shortcut: Win+L is reserved by Windows Lock Screen")
			h.invalidate()
			return
		}
		if core.RequiresTakeover(g) {
			hasWinHotkey = true
		}
		normalizedHotkeys[i] = g.String()
	}

	for _, sec := range h.sections {
		if sec.pluginView != nil && h.pluginActiveInDraft(sec.pluginID) {
			if err := sec.pluginView.Validate(); err != nil {
				h.status = err.Error()
				h.invalidate()
				return
			}
		}
	}

	nextHost := h.app.HostConfig()
	nextHost.Appearance.Theme = themeKeyFromIndex(h.themeIdx)
	nextHost.Appearance.Language = langPrefFromIndex(h.langIdx)
	nextHost.Hotkeys.TakeoverWinKeys = h.hostDraft.Hotkeys.TakeoverWinKeys
	if hasWinHotkey && !h.hostOrig.Hotkeys.TakeoverWinKeys && !h.hostDraft.Hotkeys.TakeoverWinKeys {
		nextHost.Hotkeys.TakeoverWinKeys = true
	}
	nextHost.Advanced.RunOnStartup = h.hostDraft.Advanced.RunOnStartup
	nextHost.Advanced.RunElevatedOnStartup = h.hostDraft.Advanced.RunElevatedOnStartup
	nextHost.Advanced.VerboseLogging = h.hostDraft.Advanced.VerboseLogging
	nextHost.Launcher.Enabled = h.hostDraft.Launcher.Enabled
	nextHost.Launcher.MaxResults = int(h.launcherMax)
	nextHost.Update.AutoCheck = h.hostDraft.Update.AutoCheck
	nextHost.Update.AutoDownload = h.hostDraft.Update.AutoDownload
	nextHost.Update.CheckIntervalHours = updateIntervalFromIndex(h.updateIntervalIdx)
	nextHost.Update.DownloadSource = h.hostDraft.Update.DownloadSource
	if !h.app.custom {
		installed := h.app.InstalledModules()
		for _, mod := range []core.ModuleID{core.ModuleCapture, core.ModuleHistory, core.ModuleViewer, core.ModuleVideo, core.ModuleZip, core.ModuleMonitor} {
			if installed.Enabled(mod) {
				nextHost.SetModuleDisabled(mod, !h.modDraft.Enabled(mod))
			}
		}
	}
	if err := h.app.SaveHostConfig(nextHost); err != nil {
		h.hostOrig = h.app.HostConfig()
		h.hostDraft.Advanced.RunElevatedOnStartup = h.hostOrig.Advanced.RunElevatedOnStartup
		h.status = i18n.S("保存失败: ", "Failed to save: ") + err.Error()
		h.invalidate()
		return
	}

	for i := range h.hotkeys {
		if h.hotkeys[i].pluginID != "" && h.app.PluginByID(h.hotkeys[i].pluginID) == nil {
			continue
		}
		var cur string
		if h.hotkeys[i].spec.CurrentGesture != nil {
			cur = strings.TrimSpace(h.hotkeys[i].spec.CurrentGesture())
		}
		if normalizedHotkeys[i] != cur && h.hotkeys[i].spec.SetGesture != nil {
			if err := h.hotkeys[i].spec.SetGesture(normalizedHotkeys[i]); err != nil {
				h.status = i18n.F("保存热键 %s 失败: %v", "Failed to save shortcut %s: %v", h.hotkeys[i].spec.Label, err)
				h.invalidate()
				return
			}
		}
		h.hotkeys[i].draft = normalizedHotkeys[i]
	}

	for _, sec := range h.sections {
		if sec.pluginView != nil && sec.pluginView.Dirty() {
			if sec.pluginID != "" && h.app.PluginByID(sec.pluginID) == nil {
				continue
			}
			if err := sec.pluginView.Save(); err != nil {
				h.status = i18n.F("保存 %s 设置失败: %v", "Failed to save %s settings: %v", sec.title, err)
				h.invalidate()
				return
			}
		}
	}

	h.app.ReloadHotkeys()
	h.app.RebuildTray()
	langChanged := i18n.Current() != prevLang
	if modulesChanged || langChanged {
		prevSection := h.section
		h.buildSections()
		h.selectSection(prevSection)
		if h.win != nil {
			h.win.SetTitle(i18n.S("PureUtils 设置", "PureUtils Settings"))
		}
	}
	h.loadFromHost()
	h.status = i18n.S("设置已保存", "Settings saved")
	if c != nil {
		c.Toast(h.status)
	}
	h.invalidate()
}

func (h *settingsView) close() {
	h.stopRecorder()
	h.closed = true
	if h.win != nil {
		h.win.Close()
	}
}

func (h *settingsView) invalidate() {
	if h.win != nil {
		h.win.Invalidate()
	}
}

func langIndex(pref string) int {
	switch i18n.NormalizePref(pref) {
	case i18n.PrefZH:
		return 1
	case i18n.PrefEN:
		return 2
	default:
		return 0
	}
}

func langPrefFromIndex(idx int) string {
	switch idx {
	case 1:
		return i18n.PrefZH
	case 2:
		return i18n.PrefEN
	default:
		return i18n.PrefAuto
	}
}

func themeIndex(theme string) int {
	switch strings.ToLower(theme) {
	case "light":
		return 1
	case "dark":
		return 2
	default:
		return 0
	}
}

func themeKeyFromIndex(idx int) string {
	switch idx {
	case 1:
		return "light"
	case 2:
		return "dark"
	default:
		return "system"
	}
}

func clampInt(v, lo, hi int) int {
	if lo > hi {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func updateIntervalIndex(hours int) int {
	switch hours {
	case 6:
		return 0
	case 72:
		return 2
	case 168:
		return 3
	default:
		return 1
	}
}

func updateIntervalFromIndex(idx int) int {
	switch idx {
	case 0:
		return 6
	case 2:
		return 72
	case 3:
		return 168
	default:
		return 24
	}
}

func truncateUpdateNotes(body string, maxRunes int) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(body), "\r\n", "\n"), "\n")
	if len(lines) > 6 {
		lines = append(lines[:6], "…")
	}
	s := strings.Join(lines, "\n")
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return s
}
