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
	"pureutils/pkg/purekit/platform"
	pureui "pureutils/pkg/purekit/ui"
)

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
	launcherMax float64

	hotkeys []hotkeyEntry

	recorder     *platform.HotkeyRecorder
	recordingID  string
	recordingErr string
	status       string
}

// ShowSettings opens or focuses the unified Settings window, optionally navigating to tabID.
func (a *App) ShowSettings(tabID string) {
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
		Title:     "PureUtils 设置",
		Width:     860,
		Height:    640,
		MinWidth:  720,
		MinHeight: 540,
		StateKey:  "settings",
		Content:   ui.View(h.view),
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
				Label:          "快捷启动器",
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
			label: "常规",
			title: "常规与外观",
			sub:   "主题外观与全局快捷键",
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
		label: "高级",
		title: "高级与维护",
		sub:   "开机自启、日志诊断与已安装模块",
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
	h.launcherMax = float64(hc.Launcher.MaxResults)
	for i := range h.hotkeys {
		var cur string
		if h.hotkeys[i].spec.CurrentGesture != nil {
			cur = strings.TrimSpace(h.hotkeys[i].spec.CurrentGesture())
		}
		h.hotkeys[i].orig = cur
		h.hotkeys[i].draft = cur
	}
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
	ui.Row(c).Fill().Background(pal.Bg).Children(func() {
		h.sidebar(c, pal)
		ui.Box(c).Width(1).FillHeight().Background(pal.Border)
		h.contentPane(c, pal)
	})
}

func (h *settingsView) sidebar(c *ui.Context, pal pureui.Palette) {
	ui.Column(c).Width(208).FillHeight().Background(pal.SidebarBg).Children(func() {
		ui.Row(c).Padding(18, 16, 12, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(30, 30).Radius(8).Gradient(pal.Accent, pal.AccentEnd, 135).
				InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.25)).Center().Children(func() {
				ui.Icon(c, pureui.IconSettings).Size(15, 15).TextColor(pal.AccentText)
			})
			ui.Column(c).Gap(1).Children(func() {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "PureUtils").FontSize(14).FontWeight(700).TextColor(pal.Text)
					pureui.PillBadge(c, "1.0", pal.Accent, pal.AccentSoft)
				})
				ui.Text(c, "系统设置").FontSize(11).TextColor(pal.TextMuted)
			})
		})

		sb := ui.Sidebar(c, &h.section, func() {
			ui.SidebarSection(c, "设置分类", nil, func() {
				for _, sec := range h.sections {
					ui.SidebarItem(c, sec.id, sec.icon, sec.label)
				}
			})
		}).Grow(1).Background(pal.SidebarBg)
		if sb.Changed() {
			h.tab = h.tabIndex(h.section)
		} else if idx := h.tabIndex(h.section); idx != h.tab {
			h.tab = idx
		}

		ui.Column(c).Margin(8, 12, 12, 12).Padding(10).Gap(4).Radius(10).
			Background(pal.CardBg).Children(func() {
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
				monText := "剪贴板监听中"
				if !item.Checked {
					dot = pal.Warning
					monText = "剪贴板监听已暂停"
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
					ui.Text(c, monText).FontSize(11).FontWeight(500).TextColor(pal.TextSecondary)
				})
				return
			}
		}
	}
	var names []string
	for _, p := range plugins {
		names = append(names, p.Info().Name)
	}
	summary := "无启用插件"
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
	ui.Column(c).Grow(1).FillHeight().Background(pal.Bg).Children(func() {
		ui.Column(c).Padding(14, 22, 10, 22).Gap(2).Children(func() {
			ui.Text(c, sec.title).FontSize(19).FontWeight(700).TextColor(pal.Text)
			ui.Text(c, sec.sub).FontSize(12).TextColor(pal.TextMuted)
		})
		ui.Box(c).Height(1).FillWidth().Background(pal.Border)

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(14, 22).Gap(12).Children(func() {
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
		ui.Row(c).Grow(1).Gap(6).AlignItems(ui.Center).Children(func() {
			switch {
			case h.recordingID != "":
				if h.recordingErr != "" {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.Danger)
					ui.Text(c, h.recordingErr).FontSize(11).FontWeight(500).TextColor(pal.Danger)
				} else {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.Accent)
					ui.Text(c, "录制中…请按下组合键（按 Esc 取消）").FontSize(11).FontWeight(500).TextColor(pal.Accent)
				}
			case dirty:
				if strings.HasPrefix(h.status, "已录制:") {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.Accent)
					ui.Text(c, h.status).FontSize(11).FontWeight(500).TextColor(pal.Accent)
				} else {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.Warning)
					ui.Text(c, "有未保存的更改").FontSize(11).FontWeight(500).TextColor(pal.Warning)
				}
			case h.status != "":
				ui.Text(c, h.status).FontSize(11).TextColor(pal.TextMuted)
			default:
				ui.Text(c, "配置保存后立即生效").FontSize(11).TextColor(pal.TextMuted)
			}
		})
		ui.Button(c, "放弃更改").Disabled(!dirty).OnClick(func() {
			h.stopRecorder()
			h.loadFromHost()
			for i := range h.sections {
				if h.sections[i].tab != nil && h.sections[i].tab.NewView != nil {
					h.sections[i].pluginView = h.sections[i].tab.NewView(h.app)
				}
			}
			h.status = "已放弃未保存的更改"
			h.invalidate()
		}).Border(0, ui.Transparent)
		ui.Button(c, "关闭").OnClick(func() { h.close() }).Border(0, ui.Transparent)
		ui.PrimaryButton(c, "保存").OnClick(func() { h.save(c) })
	})
}

func (h *settingsView) general(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, "外观与显示", "调整应用全局色彩主题（浅色、深色或跟随 Windows）", pal, func() {
		pureui.SettingRow(c, "主题模式", "支持跟随 Windows 深浅色或手动指定", pal, func() {
			seg := ui.Segmented(c, &h.themeIdx, "跟随系统", "浅色", "深色").Label("主题")
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				h.hostDraft.Appearance.Theme = themeKeyFromIndex(h.themeIdx)
			}
		})
	})

	pureui.SettingCard(c, "全局热键", "触发冲突或无效热键会在对应行下方显示原因，保存后立即生效", pal, func() {
		if len(h.hotkeys) == 0 {
			ui.Text(c, "当前没有启用提供全局热键的功能插件").FontSize(12).TextColor(pal.TextMuted)
			return
		}
		for i := range h.hotkeys {
			if i > 0 {
				ui.Divider(c)
			}
			h.hotkeyRow(c, i, pal)
		}
		ui.Divider(c)
		pureui.SettingRow(c, "接管系统 Win 组合键", "当 Win+V、Win+Shift+S、Win+Space 等被系统占用时，自动通过底层键盘钩子接管并屏蔽开始菜单（不支持 Win+L）", pal, func() {
			ui.Switch(c, &h.hostDraft.Hotkeys.TakeoverWinKeys).Label("开关：接管系统 Win 组合键")
		})
	})

	pureui.SettingCard(c, "快捷启动器", "按下全局热键（默认 Alt+Space）快速搜索插件能力并自动识别当前剪贴板或资源管理器选中项", pal, func() {
		pureui.SettingRow(c, "启用快捷启动器", "注册启动器全局快捷键并在系统托盘显示入口", pal, func() {
			ui.Switch(c, &h.hostDraft.Launcher.Enabled).Label("开关：启用快捷启动器")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "最多显示结果数", "控制启动器列表单次展示的最大匹配条数（4–30）", pal, func() {
			if ui.NumberInput(c, &h.launcherMax, 4, 30, 1).Label("数值：启动器最大结果数").Changed() {
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
			ui.Row(c).Grow(1).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, spec.Label).FontSize(13).FontWeight(500).TextColor(pal.Text)
				if spec.DefaultGesture != "" {
					ui.Text(c, "默认 "+spec.DefaultGesture).FontSize(10.5).TextColor(pal.TextMuted)
				}
			})
			ui.TextInput(c, &entry.draft).Width(168).Label("快捷键：" + spec.Label).Border(0, ui.Transparent).FocusRing(false)

			recBtn := ui.ButtonBase(c).Padding(5, 11).Radius(7).Gap(5).AlignItems(ui.Center).
				Label("录制" + spec.Label).Tooltip("按下组合键录制热键")
			recBg := pal.ControlBg
			recFg := pal.TextSecondary
			recText := "录制"
			if recording {
				recBg = pal.AccentSoft
				recFg = pal.Accent
				recText = "停止"
			} else if recBtn.Pressed() {
				recBg = pal.CardActive
			} else if recBtn.Hovered() {
				recBg = pal.CardHover
				recFg = pal.Text
			}
			recBtn.Background(recBg)
			recBtn.Children(func() {
				ui.Icon(c, pureui.IconRecord).Size(12, 12).TextColor(recFg)
				ui.Text(c, recText).FontSize(11).FontWeight(500).SingleLine().TextColor(recFg)
			})
			recBtn.OnClick(func() {
				if recording {
					h.stopRecorder()
					h.status = "已取消录制"
					h.invalidate()
				} else {
					h.startRecording(spec.ID)
				}
			})

			resetTooltip := "恢复默认热键 (" + spec.DefaultGesture + ")"
			if spec.DefaultGesture == "" {
				resetTooltip = "清空热键"
			}
			pureui.IconButton(c, pureui.IconRotateCcw, "恢复默认"+spec.Label, resetTooltip, false, false, pal).
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
		return "Win+L 是系统锁屏，不能占用"
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
	pureui.SettingCard(c, "系统与日志", "管理开机自启与诊断日志级别", pal, func() {
		pureui.SettingRow(c, "开机自动启动", "登录 Windows 时在系统托盘后台自动启动 PureUtils", pal, func() {
			ui.Switch(c, &h.hostDraft.Advanced.RunOnStartup).Label("开关：开机自动启动")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "记录详细日志", "写入更多调试上下文到每日日志文件以便排查问题", pal, func() {
			ui.Switch(c, &h.hostDraft.Advanced.VerboseLogging).Label("开关：记录详细日志")
		})
	})

	pureui.SettingCard(c, "存储与维护", "数据目录："+h.app.Paths().Home, pal, func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c, "打开数据目录").OnClick(func() {
				if err := mygo.Shell.OpenPath(h.app.Paths().Home); err != nil {
					h.status = "打开目录失败: " + err.Error()
				}
				h.invalidate()
			}).Border(0, ui.Transparent)
		})
	})

	pureui.SettingCard(c, "已安装模块", "可实时开启或关闭已安装的组件，保存后立即生效（无需重启）；精简构建未包含的组件无法开启", pal, func() {
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
			{"pureshot", "PureShot", core.ModuleCapture, "PureShot（截图、标注与悬浮贴图）", "区域/整屏截图、9 种标注与置顶悬浮贴图", h.app.compiled.Capture, &h.modDraft.Capture},
			{"pureclip", "PureClip", core.ModuleHistory, "PureClip（剪贴板历史）", "自动监听文本/富文本/图片/文件并持久化检索", h.app.compiled.History, &h.modDraft.History},
			{"pureimage", "PureImage", core.ModuleViewer, "PureImage（看图与空格预览）", "支持 JPEG Turbo/HEIC/AVIF/JXL/RAW、动图与资源管理器空格预览", h.app.compiled.Viewer, &h.modDraft.Viewer},
			{"purezip", "PureZip", core.ModuleZip, "PureZip（压缩与解压管理）", "浏览/智能解压/多线程压缩/加密/右键菜单与文件关联", h.app.compiled.Zip, &h.modDraft.Zip},
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
					pureui.PillBadge(c, "精简构建未包含", pal.TextMuted, pal.Bg)
				case h.app.custom:
					if active {
						pureui.PillBadge(c, "已启用", pal.Success, pal.SuccessSoft)
					} else {
						pureui.PillBadge(c, "未启用", pal.TextMuted, pal.Bg)
					}
				case !installed.Enabled(b.mod):
					pureui.PillBadge(c, "未安装", pal.TextMuted, pal.Bg)
				default:
					ui.Switch(c, b.draftPtr).Label("开关：启用 " + b.shortName)
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
				pureui.PillBadge(c, "已启用", pal.Success, pal.SuccessSoft)
			})
		}
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
		h.status = "无法录制: " + err.Error()
		h.invalidate()
		return
	}
	h.recorder = rec
	h.recordingID = id
	h.recordingErr = ""
	h.app.SetGlobalHookSuppressed("host:hotkey-recorder", true)
	h.status = "正在录制，请按下组合键（Esc 取消）"
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
		h.status = "已取消录制"
		h.recordingID = ""
		h.recordingErr = ""
	case res.Err != "":
		h.recordingErr = res.Err
		h.status = res.Err
	case !res.Gesture.Empty():
		for i := range h.hotkeys {
			if h.hotkeys[i].spec.ID == h.recordingID {
				h.hotkeys[i].draft = res.Gesture.String()
				break
			}
		}
		if core.RequiresTakeover(res.Gesture) && !h.hostDraft.Hotkeys.TakeoverWinKeys {
			h.hostDraft.Hotkeys.TakeoverWinKeys = true
			h.status = "已录制: " + res.Gesture.String() + "（已自动开启 Win 组合键接管，点击保存生效）"
		} else {
			h.status = "已录制: " + res.Gesture.String() + "（点击保存生效）"
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
		h.hostDraft.Hotkeys.TakeoverWinKeys != h.hostOrig.Hotkeys.TakeoverWinKeys ||
		h.hostDraft.Advanced.RunOnStartup != h.hostOrig.Advanced.RunOnStartup ||
		h.hostDraft.Advanced.VerboseLogging != h.hostOrig.Advanced.VerboseLogging ||
		h.hostDraft.Launcher.Enabled != h.hostOrig.Launcher.Enabled ||
		int(h.launcherMax) != h.hostOrig.Launcher.MaxResults ||
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
	h.stopRecorder()

	modulesChanged := !h.app.custom && h.modDraft != h.modOrig
	if modulesChanged && h.app.InstalledModules().Any() && !h.modDraft.Any() {
		h.status = "请至少保留一个已启用的功能组件"
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
			h.status = "热键无效: " + err.Error()
			h.invalidate()
			return
		}
		if g.Modifiers == core.ModWin && g.Key == "L" {
			h.status = "热键无效: Win+L 是系统锁屏，不能占用"
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
	nextHost.Hotkeys.TakeoverWinKeys = h.hostDraft.Hotkeys.TakeoverWinKeys
	if hasWinHotkey && !h.hostOrig.Hotkeys.TakeoverWinKeys && !h.hostDraft.Hotkeys.TakeoverWinKeys {
		nextHost.Hotkeys.TakeoverWinKeys = true
	}
	nextHost.Advanced.RunOnStartup = h.hostDraft.Advanced.RunOnStartup
	nextHost.Advanced.VerboseLogging = h.hostDraft.Advanced.VerboseLogging
	nextHost.Launcher.Enabled = h.hostDraft.Launcher.Enabled
	nextHost.Launcher.MaxResults = int(h.launcherMax)
	if !h.app.custom {
		installed := h.app.InstalledModules()
		for _, mod := range []core.ModuleID{core.ModuleCapture, core.ModuleHistory, core.ModuleViewer, core.ModuleZip} {
			if installed.Enabled(mod) {
				nextHost.SetModuleDisabled(mod, !h.modDraft.Enabled(mod))
			}
		}
	}
	if err := h.app.SaveHostConfig(nextHost); err != nil {
		h.status = "保存失败: " + err.Error()
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
				h.status = fmt.Sprintf("保存热键 %s 失败: %v", h.hotkeys[i].spec.Label, err)
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
				h.status = fmt.Sprintf("保存 %s 设置失败: %v", sec.title, err)
				h.invalidate()
				return
			}
		}
	}

	h.app.ReloadHotkeys()
	h.app.RebuildTray()
	if modulesChanged {
		prevSection := h.section
		h.buildSections()
		h.selectSection(prevSection)
	}
	h.loadFromHost()
	h.status = "设置已保存"
	if c != nil {
		c.Toast("设置已保存")
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
