//go:build windows

package puremonitor

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/puremonitor/collector"
	"pureutils/plugins/puremonitor/fps"
)

type settingsTabTarget interface {
	Config() config.PureMonitorConfig
	ApplyConfig(config.PureMonitorConfig) error
	Inventory() collector.HardwareInventory
	Snapshot() collector.SystemSnapshot
	PawnIOStatus() (active bool, accessDenied bool)
	SensorManager() *collector.SensorExtManager
	PermissionStatus() (fps.PermissionStatus, bool, bool, string)
	StartElevatedHelper(mode string) error
	RecentApps() []fps.DetectedApp
	OpenDashboard()
}

type settingsTabView struct {
	plugin    settingsTabTarget
	host      purekit.Host
	sensorMgr *collector.SensorExtManager

	mu    sync.Mutex
	saved config.PureMonitorConfig
	draft config.PureMonitorConfig

	tbModeIdx     int
	tbAlignIdx    int
	tbLayoutIdx   int
	tbOffset      float64
	fltOpacity    float64
	refreshIdx    int
	themeIdx      int
	speedUnitIdx  int
	tempUnitIdx   int
	nicLabel      string
	hudPosIdx     int
	hudStyleIdx   int
	hudOpacity    float64
	fontSize      float64
	warnCPU       float64
	warnRAM       float64
	warnTemp      float64
	sourceLabel   string
	wlInput       string
	blInput       string
	status        string
}

func newSettingsTabView(p settingsTabTarget, host purekit.Host) *settingsTabView {
	sMgr := p.SensorManager()
	v := &settingsTabView{
		plugin:    p,
		host:      host,
		sensorMgr: sMgr,
	}
	v.syncFrom(p.Config())
	if sMgr != nil {
		sMgr.SetOnChange(v.onSensorProgressChanged)
	}
	return v
}

func (v *settingsTabView) headless() bool {
	return v.host == nil || v.host.Headless()
}

func (v *settingsTabView) invalidate() {
	if inv, ok := v.host.(purekit.SettingsInvalidator); ok {
		inv.InvalidateSettings()
	}
}

func (v *settingsTabView) onSensorProgressChanged() {
	if v.sensorMgr == nil {
		return
	}
	prog := v.sensorMgr.Progress()
	apply := func() {
		if prog.Completed {
			v.status = prog.Stage
			st := v.sensorMgr.Status()
			if st.DriverInstalled && st.ModulesReady {
				v.draft.PawnIOEnabled = true
				cfg := v.plugin.Config()
				if !cfg.PawnIOEnabled {
					cfg.PawnIOEnabled = true
					_ = v.plugin.ApplyConfig(cfg)
					v.saved.PawnIOEnabled = true
				}
			}
		} else if !prog.Active && prog.Err != "" {
			v.status = prog.Err
		}
		v.invalidate()
	}
	if v.headless() {
		apply()
	} else {
		mygo.RunOnMain(apply)
	}
}

func (v *settingsTabView) syncFrom(cfg config.PureMonitorConfig) {
	cfg.Normalize()
	v.saved = cfg.Clone()
	v.draft = cfg.Clone()
	v.syncIndices()
}

func (v *settingsTabView) syncIndices() {
	switch v.draft.TaskbarMode {
	case "overlay":
		v.tbModeIdx = 1
	case "embed":
		v.tbModeIdx = 2
	default:
		v.tbModeIdx = 0
	}
	if v.draft.TaskbarAlign == "left" {
		v.tbAlignIdx = 1
	} else {
		v.tbAlignIdx = 0
	}
	if v.draft.TaskbarLayout == "single_row" {
		v.tbLayoutIdx = 1
	} else {
		v.tbLayoutIdx = 0
	}
	v.tbOffset = float64(v.draft.TaskbarOffsetX)
	v.fltOpacity = v.draft.FloatOpacity

	switch v.draft.RefreshIntervalMs {
	case 500:
		v.refreshIdx = 0
	case 1500:
		v.refreshIdx = 2
	case 2000:
		v.refreshIdx = 3
	default:
		v.refreshIdx = 1
	}
	switch v.draft.Theme {
	case "dark":
		v.themeIdx = 1
	case "light":
		v.themeIdx = 2
	default:
		v.themeIdx = 0
	}
	if v.draft.UnitSpeed == "mbps" {
		v.speedUnitIdx = 1
	} else {
		v.speedUnitIdx = 0
	}
	if v.draft.TempUnit == "F" {
		v.tempUnitIdx = 1
	} else {
		v.tempUnitIdx = 0
	}
	v.nicLabel = v.adapterLabelFromConfig(v.draft.SelectedNetAdapter)

	switch v.draft.GameHUDPosition {
	case "top_right":
		v.hudPosIdx = 1
	case "bottom_left":
		v.hudPosIdx = 2
	case "bottom_right":
		v.hudPosIdx = 3
	default:
		v.hudPosIdx = 0
	}
	switch v.draft.GameHUDStyle {
	case "compact":
		v.hudStyleIdx = 0
	case "graph":
		v.hudStyleIdx = 2
	default:
		v.hudStyleIdx = 1
	}
	v.hudOpacity = v.draft.GameHUDOpacity
	v.fontSize = float64(v.draft.FontSize)
	v.warnCPU = float64(v.draft.WarnCPUPct)
	v.warnRAM = float64(v.draft.WarnRAMPct)
	v.warnTemp = float64(v.draft.WarnTempC)
	v.sourceLabel = collector.SensorSourceLabel(v.draft.SensorDownloadSource)
}

func (v *settingsTabView) candidate() config.PureMonitorConfig {
	cand := v.draft.Clone()
	switch v.tbModeIdx {
	case 1:
		cand.TaskbarMode = "overlay"
	case 2:
		cand.TaskbarMode = "embed"
	default:
		cand.TaskbarMode = "auto"
	}
	if v.tbAlignIdx == 1 {
		cand.TaskbarAlign = "left"
	} else {
		cand.TaskbarAlign = "right"
	}
	if v.tbLayoutIdx == 1 {
		cand.TaskbarLayout = "single_row"
	} else {
		cand.TaskbarLayout = "compact_2row"
	}
	cand.TaskbarOffsetX = int(v.tbOffset)
	cand.FloatOpacity = v.fltOpacity

	switch v.refreshIdx {
	case 0:
		cand.RefreshIntervalMs = 500
	case 2:
		cand.RefreshIntervalMs = 1500
	case 3:
		cand.RefreshIntervalMs = 2000
	default:
		cand.RefreshIntervalMs = 1000
	}
	switch v.themeIdx {
	case 1:
		cand.Theme = "dark"
	case 2:
		cand.Theme = "light"
	default:
		cand.Theme = "system"
	}
	if v.speedUnitIdx == 1 {
		cand.UnitSpeed = "mbps"
	} else {
		cand.UnitSpeed = "auto"
	}
	if v.tempUnitIdx == 1 {
		cand.TempUnit = "F"
	} else {
		cand.TempUnit = "C"
	}
	cand.SelectedNetAdapter = v.adapterConfigFromLabel(v.nicLabel)

	switch v.hudPosIdx {
	case 1:
		cand.GameHUDPosition = "top_right"
	case 2:
		cand.GameHUDPosition = "bottom_left"
	case 3:
		cand.GameHUDPosition = "bottom_right"
	default:
		cand.GameHUDPosition = "top_left"
	}
	switch v.hudStyleIdx {
	case 0:
		cand.GameHUDStyle = "compact"
	case 2:
		cand.GameHUDStyle = "graph"
	default:
		cand.GameHUDStyle = "detailed"
	}
	cand.GameHUDOpacity = v.hudOpacity
	cand.FontSize = int(v.fontSize)
	cand.WarnCPUPct = int(v.warnCPU)
	cand.WarnRAMPct = int(v.warnRAM)
	cand.WarnTempC = int(v.warnTemp)
	cand.SensorDownloadSource = collector.SensorSourceIDFromLabel(v.sourceLabel)
	cand.Normalize()
	return cand
}

func (v *settingsTabView) syncExternalChanges() {
	cur := v.plugin.Config()
	if cur.Equal(v.saved) {
		return
	}
	cand := v.candidate()
	if cand.Equal(v.saved) {
		v.syncFrom(cur)
		return
	}
	syncBool := func(draftField *bool, oldVal, newVal bool) {
		if newVal != oldVal && *draftField == oldVal {
			*draftField = newVal
		}
	}
	syncInt := func(draftField *int, oldVal, newVal int) {
		if newVal != oldVal && *draftField == oldVal {
			*draftField = newVal
		}
	}
	syncStrings := func(draftField *[]string, oldVal, newVal []string) {
		if !slices.Equal(newVal, oldVal) && slices.Equal(*draftField, oldVal) {
			*draftField = append([]string(nil), newVal...)
		}
	}
	syncBool(&v.draft.Enabled, v.saved.Enabled, cur.Enabled)
	syncBool(&v.draft.ShowTaskbar, v.saved.ShowTaskbar, cur.ShowTaskbar)
	syncBool(&v.draft.ShowFloating, v.saved.ShowFloating, cur.ShowFloating)
	syncInt(&v.draft.FloatX, v.saved.FloatX, cur.FloatX)
	syncInt(&v.draft.FloatY, v.saved.FloatY, cur.FloatY)
	syncBool(&v.draft.FloatCollapsed, v.saved.FloatCollapsed, cur.FloatCollapsed)
	syncBool(&v.draft.FloatLocked, v.saved.FloatLocked, cur.FloatLocked)
	syncBool(&v.draft.FloatClickThrough, v.saved.FloatClickThrough, cur.FloatClickThrough)
	syncStrings(&v.draft.FloatMetrics, v.saved.FloatMetrics, cur.FloatMetrics)
	syncStrings(&v.draft.FloatCollapsedMetrics, v.saved.FloatCollapsedMetrics, cur.FloatCollapsedMetrics)
	syncBool(&v.draft.GameFPSEnabled, v.saved.GameFPSEnabled, cur.GameFPSEnabled)
	syncStrings(&v.draft.GameWhitelist, v.saved.GameWhitelist, cur.GameWhitelist)
	syncStrings(&v.draft.GameBlacklist, v.saved.GameBlacklist, cur.GameBlacklist)
	syncBool(&v.draft.PawnIOEnabled, v.saved.PawnIOEnabled, cur.PawnIOEnabled)
	v.saved = cur.Clone()
}

func (v *settingsTabView) Dirty() bool {
	v.syncExternalChanges()
	return !v.candidate().Equal(v.saved)
}

func (v *settingsTabView) Validate() error { return nil }

func (v *settingsTabView) Save() error {
	cand := v.candidate()
	if err := v.plugin.ApplyConfig(cand); err != nil {
		return err
	}
	v.syncFrom(v.plugin.Config())
	return nil
}

func (v *settingsTabView) ResetDefaults() {
	def := config.DefaultPureMonitorConfig()
	if detectPawnIOInstalled() {
		def.PawnIOEnabled = true
	}
	v.draft = def
	v.syncIndices()
}

func (v *settingsTabView) adapterLabelFromConfig(raw string) string {
	if raw == "" || strings.EqualFold(raw, "auto") {
		return i18n.S("自动聚合全部物理网卡", "Auto (All Physical NICs)")
	}
	return raw
}

func (v *settingsTabView) adapterConfigFromLabel(lbl string) string {
	switch lbl {
	case "", "自动聚合全部物理网卡", "Auto (All Physical NICs)":
		return "auto"
	default:
		return lbl
	}
}

func (v *settingsTabView) adapterOptions() []string {
	opts := []string{i18n.S("自动聚合全部物理网卡", "Auto (All Physical NICs)")}
	if !v.headless() {
		inv := v.plugin.Inventory()
		for _, nic := range inv.NetAdapters {
			if nic.Name != "" && !slices.Contains(opts, nic.Name) {
				opts = append(opts, nic.Name)
			}
		}
	}
	return opts
}

func (v *settingsTabView) Render(c *ui.Context, pal pureui.Palette) {
	v.syncExternalChanges()
	v.renderDisplayCard(c, pal)
	v.renderMetricsAndThresholdsCard(c, pal)
	v.renderGameFPSCard(c, pal)
	v.renderPawnIOCard(c, pal)
}

func (v *settingsTabView) renderDisplayCard(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, i18n.S("显示方式与外观", "Display Modes & Appearance"), "", pal, func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.PrimaryButton(c, i18n.S("打开硬件监控面板", "Open Hardware Dashboard")).OnClick(func() {
				v.plugin.OpenDashboard()
			})
		})

		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("启用任务栏监控", "Show Taskbar Monitor"), i18n.S("单击任务栏监控条可打开硬件详情面板，右键可快捷开关", "Click the taskbar strip to open Hardware Dashboard; right-click for quick toggles"), pal, func() {
			ui.Switch(c, &v.draft.ShowTaskbar).Label(i18n.S("开关：启用任务栏监控", "Toggle: Taskbar monitor"))
		})
		ui.Divider(c)
		pureui.SettingBlockTip(c, i18n.S("任务栏显示模式", "Taskbar Integration Mode"), i18n.S("自动模式在 Win11 使用置顶分层锚定、Win10 使用原生子窗口嵌入", "Auto uses Topmost overlay anchoring on Win11 and child embedding on Win10"), pal, func() {
			ui.Segmented(c, &v.tbModeIdx,
				i18n.S("自动（推荐）", "Auto"),
				i18n.S("置顶浮层", "Overlay"),
				i18n.S("原生嵌入", "Embed"),
			).Label(i18n.S("任务栏显示模式", "Taskbar mode"))
		})
		ui.Divider(c)
		pureui.SettingBlock(c, i18n.S("任务栏位置与排版", "Taskbar Alignment & Layout"), "", pal, func() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				ui.Segmented(c, &v.tbAlignIdx, i18n.S("托盘左侧", "Right (Near Tray)"), i18n.S("任务栏左侧", "Left")).Label(i18n.S("任务栏对齐", "Taskbar alignment"))
				ui.Segmented(c, &v.tbLayoutIdx, i18n.S("双行紧凑", "2-Row Compact"), i18n.S("单行横排", "Single Row")).Label(i18n.S("任务栏排版", "Taskbar layout"))
			})
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("任务栏水平偏移微调", "Taskbar Horizontal Offset"), i18n.S("当 Win11 小组件或居中图标位置不同时，可微调水平像素偏移", "Fine-tune horizontal placement to avoid taskbar icon overlap"), pal, func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Slider(c, &v.tbOffset, -400, 400).Width(130).Label(i18n.S("任务栏水平偏移", "Taskbar offset"))
				pureui.PillBadge(c, fmt.Sprintf("%.0fpx", v.tbOffset), pal.Accent, pal.AccentSoft)
			})
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("全屏时自动隐藏任务栏监控", "Hide Taskbar Monitor in Fullscreen"), "", pal, func() {
			ui.Switch(c, &v.draft.TaskbarHideOnFullscreen).Label(i18n.S("开关：全屏隐藏任务栏监控", "Toggle: Hide taskbar in fullscreen"))
		})

		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("启用可收起悬浮监控窗", "Show Collapsible Floating Monitor"), i18n.S("拖至屏幕左/右/上边缘自动贴边，鼠标移开收起成胶囊，悬停展开", "Dock to left/right/top edges; collapses into a compact pill until hovered"), pal, func() {
			ui.Switch(c, &v.draft.ShowFloating).Label(i18n.S("开关：启用悬浮监控窗", "Toggle: Floating monitor"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("靠边自动贴边收起", "Auto-Dock & Edge Collapse"), "", pal, func() {
			ui.Switch(c, &v.draft.FloatAutoDock).Label(i18n.S("开关：自动贴边收起", "Toggle: Auto-dock collapse"))
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("悬浮窗鼠标穿透", "Click-Through Floating Window"), i18n.S("开启后鼠标穿透操作底层窗口；按住 Alt 键可临时恢复拖拽与右键菜单", "Mouse clicks pass through; hold Alt anytime to drag or open context menu"), pal, func() {
			ui.Switch(c, &v.draft.FloatClickThrough).Label(i18n.S("开关：悬浮窗穿透", "Toggle: Floating click-through"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("锁定悬浮窗位置", "Lock Floating Position"), "", pal, func() {
			ui.Switch(c, &v.draft.FloatLocked).Label(i18n.S("开关：锁定悬浮窗位置", "Toggle: Lock floating window"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("悬浮窗不透明度", "Floating Opacity"), "", pal, func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Slider(c, &v.fltOpacity, 0.35, 1.0).Width(130).Label(i18n.S("悬浮窗不透明度", "Floating opacity"))
				pureui.PillBadge(c, fmt.Sprintf("%.0f%%", v.fltOpacity*100), pal.Accent, pal.AccentSoft)
			})
		})

		ui.Divider(c)
		pureui.SettingBlock(c, i18n.S("采样刷新频率与外观主题", "Refresh Interval & Theme"), "", pal, func() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				ui.Segmented(c, &v.refreshIdx, "0.5s", "1.0s", "1.5s", "2.0s").Label(i18n.S("刷新间隔", "Refresh interval"))
				ui.Segmented(c, &v.themeIdx, i18n.S("跟随系统", "System"), i18n.S("深色", "Dark"), i18n.S("浅色", "Light")).Label(i18n.S("监控主题", "Monitor theme"))
			})
		})
		ui.Divider(c)
		pureui.SettingBlock(c, i18n.S("单位与字体大小", "Units & Font Size"), "", pal, func() {
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Segmented(c, &v.speedUnitIdx, "KB/s · MB/s", "Mbps").Label(i18n.S("网速单位", "Speed unit"))
				ui.Segmented(c, &v.tempUnitIdx, "°C", "°F").Label(i18n.S("温度单位", "Temp unit"))
				ui.NumberInput(c, &v.fontSize, 9, 16, 1).Label(i18n.S("字体大小", "Font size"))
			})
		})
	})
}

func (v *settingsTabView) renderMetricsAndThresholdsCard(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, i18n.S("显示指标与警戒阈值", "Displayed Metrics & Alert Thresholds"), "", pal, func() {
		pureui.SettingBlock(c, i18n.S("任务栏显示指标", "Taskbar Metrics"), "", pal, func() {
			v.renderMetricTogglePills(c, pal, &v.draft.TaskbarMetrics, "system", 0)
		})
		ui.Divider(c)
		pureui.SettingBlock(c, i18n.S("悬浮窗显示指标", "Floating Window Metrics"), "", pal, func() {
			v.renderMetricTogglePills(c, pal, &v.draft.FloatMetrics, "all", 0)
		})
		ui.Divider(c)
		pureui.SettingBlockTip(
			c,
			i18n.S("悬浮窗收起显示指标（最多 2 项）", "Collapsed Floating Pill Metrics (Max 2)"),
			i18n.S("悬浮窗贴边收起后在迷你胶囊上展示的 1~2 项核心指标", "Select 1 or 2 metrics shown on the collapsed edge pill"),
			pal,
			func() {
				v.renderMetricTogglePills(c, pal, &v.draft.FloatCollapsedMetrics, "all", 2)
			},
		)
		ui.Divider(c)
		v.nicLabel = v.adapterLabelFromConfig(v.draft.SelectedNetAdapter)
		pureui.SettingRow(c, i18n.S("监控网卡", "Network Adapter"), "", pal, func() {
			sel := ui.Select(c, &v.nicLabel, v.adapterOptions()).Width(210).Label(i18n.S("监控网卡", "Network adapter"))
			sel.Border(0, ui.Transparent)
			if sel.Changed() {
				v.draft.SelectedNetAdapter = v.adapterConfigFromLabel(v.nicLabel)
			}
		})
		ui.Divider(c)
		pureui.SettingBlock(c, i18n.S("高负载 / 高温变色警戒阈值", "Warning Thresholds (CPU% / RAM% / Temp°C)"), "", pal, func() {
			ui.Row(c).Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "CPU%").FontSize(12).TextColor(pal.TextMuted)
					ui.NumberInput(c, &v.warnCPU, 50, 99, 5).Label(i18n.S("CPU 警戒阈值", "CPU warning %"))
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "RAM%").FontSize(12).TextColor(pal.TextMuted)
					ui.NumberInput(c, &v.warnRAM, 50, 99, 5).Label(i18n.S("内存警戒阈值", "RAM warning %"))
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "°C").FontSize(12).TextColor(pal.TextMuted)
					ui.NumberInput(c, &v.warnTemp, 50, 105, 5).Label(i18n.S("温度警戒阈值", "Temp warning C"))
				})
			})
		})
	})
}

func (v *settingsTabView) renderMetricTogglePills(c *ui.Context, pal pureui.Palette, target *[]string, filterCat string, maxSelect int) {
	ui.Row(c).Gap(6).Wrap().Children(func() {
		for _, m := range collector.AllMetrics() {
			if filterCat == "system" && m.Category != "system" {
				continue
			}
			idStr := string(m.ID)
			active := slices.Contains(*target, idStr)
			btn := ui.ButtonBase(c).Padding(4, 9).Radius(999).Label(m.Label)
			if active {
				btn.Background(pal.AccentSoft).Border(1, pal.AccentBorder)
			} else {
				btn.Background(pal.ControlBg).Border(1, pal.Border)
			}
			btn.Children(func() {
				col := pal.TextMuted
				if active {
					col = pal.Accent
				}
				ui.Text(c, m.Label).FontSize(11).FontWeight(500).SingleLine().TextColor(col)
			})
			if btn.Clicked() {
				if active {
					if len(*target) > 1 {
						*target = removeFromList(*target, idStr)
					}
				} else {
					if maxSelect > 0 && len(*target) >= maxSelect {
						*target = append((*target)[len(*target)-maxSelect+1:], idStr)
					} else {
						*target = appendUnique(*target, idStr)
					}
				}
			}
		}
	})
}

func (v *settingsTabView) renderGameFPSCard(c *ui.Context, pal pureui.Palette) {
	perm, etwRun, helperConn, _ := v.plugin.PermissionStatus()
	pureui.SettingCardTip(c, i18n.S("游戏帧率监控 (FPS) 与黑白名单", "Game FPS Monitor & Lists"), i18n.S("基于 Windows 原生 ETW 帧管线采集，零 DLL 注入与 Hook，兼容主流反作弊", "Uses native Windows ETW frame telemetry with zero DLL injection or hooking (anti-cheat safe)"), pal, func() {
		pureui.SettingRow(c, i18n.S("启用游戏帧率监控", "Enable Game FPS Monitor"), "", pal, func() {
			ui.Switch(c, &v.draft.GameFPSEnabled).Label(i18n.S("开关：启用游戏帧率监控", "Toggle: Game FPS monitor"))
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("自动识别运行中的游戏", "Auto-Detect Running Games"), i18n.S("结合全屏/无边框、GPU 3D 引擎负载、ETW 帧流与游戏目录自动识别，自动排除浏览器与播放器", "Detects fullscreen/3D GPU load/ETW frame stream & game folders while excluding browsers/players"), pal, func() {
			ui.Switch(c, &v.draft.GameAutoDetect).Label(i18n.S("开关：自动识别游戏", "Toggle: Auto-detect games"))
		})
		ui.Divider(c)
		pureui.SettingBlock(c, i18n.S("游戏 HUD 角落与样式", "Game HUD Corner & Style"), "", pal, func() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				ui.Segmented(c, &v.hudPosIdx,
					i18n.S("左上", "Top-L"),
					i18n.S("右上", "Top-R"),
					i18n.S("左下", "Bot-L"),
					i18n.S("右下", "Bot-R"),
				).Label(i18n.S("游戏 HUD 位置", "Game HUD corner"))
				ui.Segmented(c, &v.hudStyleIdx,
					i18n.S("极简", "Compact"),
					i18n.S("详细", "Detailed"),
					i18n.S("帧时间曲线", "Graph"),
				).Label(i18n.S("游戏 HUD 样式", "Game HUD style"))
			})
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("HUD 背景不透明度", "HUD Opacity"), "", pal, func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Slider(c, &v.hudOpacity, 0.35, 1.0).Width(130).Label(i18n.S("HUD 不透明度", "HUD opacity"))
				pureui.PillBadge(c, fmt.Sprintf("%.0f%%", v.hudOpacity*100), pal.Accent, pal.AccentSoft)
			})
		})
		ui.Divider(c)
		pureui.SettingBlock(c, i18n.S("游戏 HUD 显示指标", "Game HUD Metrics"), "", pal, func() {
			v.renderMetricTogglePills(c, pal, &v.draft.GameHUDMetrics, "all", 0)
		})
		ui.Divider(c)

		// ETW Permission & Helper status
		pureui.SettingRowTip(c, i18n.S("ETW 帧率采集权限", "ETW Capture Permission"), i18n.S("可一次性将当前用户加入 Performance Log Users 组（永久免管理员），或临时启动提权采集子进程", "Grant Performance Log Users once (permanent non-admin), or spawn a temporary elevated helper"), pal, func() {
			switch {
			case etwRun:
				pureui.PillBadge(c, i18n.S("ETW 运行中", "ETW Active"), pal.Success, pal.SuccessSoft)
			case helperConn:
				pureui.PillBadge(c, i18n.S("提权子进程已连接", "Helper Active"), pal.Success, pal.SuccessSoft)
			case perm.CanStartETW:
				pureui.PillBadge(c, i18n.S("已授权", "Authorized"), pal.Accent, pal.AccentSoft)
			default:
				pureui.PillBadge(c, i18n.S("未授权", "Not Authorized"), pal.Warning, pal.WarningSoft)
			}
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c, i18n.S("一键配置性能日志权限", "Grant Perf Log Users Access")).OnClick(func() {
				if v.headless() {
					return
				}
				if err := fps.GrantPerfLogUsersPermission(); err != nil {
					v.status = err.Error()
				} else {
					v.status = i18n.S("已配置 Performance Log Users，注销或重启后主程序即可免管理员采集 FPS", "Added to Performance Log Users; sign out/restart to activate without admin")
				}
				v.invalidate()
			}).Border(0, ui.Transparent)
			ui.Button(c, i18n.S("临时提权启动采集", "Start Elevated Helper")).OnClick(func() {
				if v.headless() {
					return
				}
				if err := v.plugin.StartElevatedHelper("both"); err != nil {
					v.status = err.Error()
				} else {
					v.status = i18n.S("已启动临时采集子进程", "Elevated helper started")
				}
				v.invalidate()
			}).Border(0, ui.Transparent)
		})

		ui.Divider(c)
		v.renderRuleListBlock(c, pal, i18n.S("游戏白名单", "Game Whitelist"),
			i18n.S("命中白名单的进程强制显示游戏 HUD", "Processes in the whitelist always show the game HUD"),
			i18n.S("输入进程名如 eldenring.exe", "Process name, e.g. eldenring.exe"),
			&v.wlInput, &v.draft.GameWhitelist)
		ui.Divider(c)
		v.renderRuleListBlock(c, pal, i18n.S("游戏黑名单", "Game Blacklist"),
			i18n.S("命中黑名单的进程强制屏蔽游戏 HUD", "Processes in the blacklist never show the game HUD"),
			i18n.S("输入进程名如 blender.exe", "Process name, e.g. blender.exe"),
			&v.blInput, &v.draft.GameBlacklist)
	})
}

func (v *settingsTabView) renderRuleListBlock(c *ui.Context, pal pureui.Palette, title, tip, placeholder string, input *string, list *[]string) {
	ui.Column(c).Gap(6).FillWidth().Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(pal.Text).Shrink(1).MinWidth(0)
			pureui.InfoTip(c, tip, pal)
		})
		ui.Row(c).Gap(8).FillWidth().Children(func() {
			ui.TextInput(c, input).Placeholder(placeholder).Grow(1).MinWidth(0).Label(title).Border(0, ui.Transparent).FocusRing(false)
			ui.Button(c, i18n.S("添加", "Add")).Shrink(0).OnClick(func() {
				norm := normalizeRuleName(*input)
				if norm != "" {
					*list = appendUnique(*list, norm)
					*input = ""
				}
			}).Border(0, ui.Transparent)
		})
		if len(*list) > 0 {
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, item := range *list {
					proc := item
					b := ui.ButtonBase(c).Padding(3, 8).Radius(999).Background(pal.ControlBg).Border(1, pal.Border).Label(proc + " ×")
					b.Children(func() {
						ui.Text(c, proc+" ×").FontSize(11).SingleLine().TextColor(pal.TextSecondary)
					})
					if b.Clicked() {
						*list = removeFromList(*list, proc)
					}
				}
			})
		}
	})
}

func (v *settingsTabView) renderPawnIOCard(c *ui.Context, pal pureui.Palette) {
	var st collector.SensorExtStatus
	var prog collector.SensorDownloadProgress
	if v.sensorMgr != nil {
		st = v.sensorMgr.Status()
		prog = v.sensorMgr.Progress()
	}
	pawnActive, pawnDenied := v.plugin.PawnIOStatus()

	pureui.SettingCardTip(c, i18n.S("硬件温度传感器扩展（可选 PawnIO）", "Hardware Sensor Extension (Optional PawnIO)"), i18n.S("主程序默认零驱动解包即用（含显卡温度）；如需 CPU 封装温度、功耗与硬盘 SMART 温度，会自动复用本机已安装的 PawnIO 驱动并支持仅补充官方模块", "PureUtils ships driver-free (GPU temp included). Reuses locally installed PawnIO automatically for CPU package temp, power & disk SMART temp without re-downloading the driver."), pal, func() {
		pureui.SettingRow(c, i18n.S("启用 PawnIO CPU 传感器", "Enable PawnIO CPU Sensors"), "", pal, func() {
			ui.Switch(c, &v.draft.PawnIOEnabled).Label(i18n.S("开关：启用 PawnIO 传感器", "Toggle: PawnIO CPU sensors"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("扩展状态", "Extension Status"), st.SensorDir, pal, func() {
			ui.Row(c).Gap(6).Wrap().Children(func() {
				switch {
				case st.DriverInstalled && !st.ModulesReady:
					pureui.PillBadge(c, i18n.S("驱动已安装（免重下驱动）", "Driver Installed (Skip Setup)"), pal.Success, pal.SuccessSoft)
				case st.DriverInstalled:
					pureui.PillBadge(c, i18n.S("驱动已安装", "Driver Installed"), pal.Success, pal.SuccessSoft)
				case st.SetupDownloaded:
					pureui.PillBadge(c, i18n.S("安装包已就绪", "Installer Ready"), pal.Accent, pal.AccentSoft)
				default:
					pureui.PillBadge(c, i18n.S("未下载", "Not Downloaded"), pal.TextMuted, pal.ControlBg)
				}
				if st.ModulesReady {
					pureui.PillBadge(c, i18n.F("%d 个模块就绪", "%d Modules Ready", st.ModuleCount), pal.Success, pal.SuccessSoft)
				} else if st.DriverInstalled {
					pureui.PillBadge(c, i18n.S("缺少传感器模块", "Modules Missing"), pal.Warning, pal.WarningSoft)
				}
				if st.DriverInstalled && st.ModulesReady && v.draft.PawnIOEnabled {
					if pawnActive {
						pureui.PillBadge(c, i18n.S("采集运行中", "Sensors Active"), pal.Success, pal.SuccessSoft)
					} else if pawnDenied {
						pureui.PillBadge(c, i18n.S("需提权读取驱动", "Needs Elevated Helper"), pal.Warning, pal.WarningSoft)
					}
				}
			})
		})
		ui.Divider(c)
		v.sourceLabel = collector.SensorSourceLabel(v.draft.SensorDownloadSource)
		pureui.SettingRow(c, i18n.S("下载源选择", "Download Source"), "", pal, func() {
			sel := ui.Select(c, &v.sourceLabel, collector.SensorSourceLabels()).Width(210).Label(i18n.S("传感器扩展下载源", "Sensor extension download source"))
			sel.Border(0, ui.Transparent)
			if sel.Changed() {
				v.draft.SensorDownloadSource = collector.SensorSourceIDFromLabel(v.sourceLabel)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("自定义镜像地址（可选）", "Custom Mirror URL (Optional)"), "", pal, func() {
			ui.TextInput(c, &v.draft.SensorCustomMirrorURL).
				Placeholder(i18n.S("https://...（留空使用预设源）", "https://... (leave empty for preset)")).
				Width(210).
				Label(i18n.S("传感器扩展自定义镜像", "Custom mirror URL")).
				Border(0, ui.Transparent).FocusRing(false)
		})

		if prog.Active {
			ui.Divider(c)
			pct := 0.0
			if prog.Installing {
				pct = 1.0
			} else if prog.TotalBytes > 0 {
				pct = float64(prog.DoneBytes) / float64(prog.TotalBytes)
				if pct > 1 {
					pct = 1
				}
			}
			ui.Column(c).Gap(6).FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, prog.Stage).FontSize(12).FontWeight(500).TextColor(pal.Text).Grow(1).MinWidth(0)
					if !prog.Installing && prog.TotalBytes > 0 {
						ui.Text(c, fmt.Sprintf("%.1f / %.1f MB", float64(prog.DoneBytes)/(1024*1024), float64(prog.TotalBytes)/(1024*1024))).
							FontSize(11).SingleLine().TextColor(pal.TextSecondary).Shrink(0)
					}
				})
				ui.Progress(c, pct).FillWidth().Height(6).Radius(3)
			})
		}

		if !prog.Active || !prog.Installing {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if prog.Active {
					ui.Button(c, i18n.S("取消下载", "Cancel Download")).OnClick(func() {
						if v.sensorMgr != nil {
							v.sensorMgr.CancelDownload()
						}
					}).Border(0, ui.Transparent)
				} else {
					switch {
					case st.DriverInstalled && st.ModulesReady:
						if !pawnActive && v.draft.PawnIOEnabled {
							ui.PrimaryButton(c, i18n.S("提权启动传感器采集", "Start Elevated Sensor Helper")).OnClick(func() {
								if v.headless() {
									return
								}
								if err := v.plugin.StartElevatedHelper("both"); err != nil {
									v.status = err.Error()
								} else {
									v.status = i18n.S("已启动便携采集子进程读取 CPU 与硬盘温度", "Elevated helper started for CPU & disk sensors")
								}
								v.invalidate()
							})
						}
						ui.Button(c, i18n.S("更新传感器模块", "Update Modules")).OnClick(func() {
							if v.sensorMgr != nil {
								v.status = ""
								src := collector.SensorSourceIDFromLabel(v.sourceLabel)
								v.sensorMgr.StartDownload(src, v.draft.SensorCustomMirrorURL)
							}
						}).Border(0, ui.Transparent)
					case st.DriverInstalled && !st.ModulesReady:
						ui.PrimaryButton(c, i18n.S("仅下载传感器模块", "Download Modules Only")).OnClick(func() {
							if v.sensorMgr != nil {
								v.status = ""
								src := collector.SensorSourceIDFromLabel(v.sourceLabel)
								v.sensorMgr.StartDownload(src, v.draft.SensorCustomMirrorURL)
							}
						})
					case st.SetupDownloaded && st.ModulesReady:
						ui.PrimaryButton(c, i18n.S("静默安装 PawnIO 驱动", "Install PawnIO Driver")).OnClick(func() {
							if v.sensorMgr != nil {
								v.status = ""
								v.sensorMgr.StartSilentInstall()
							}
						})
						ui.Button(c, i18n.S("重新下载并安装", "Redownload & Install")).OnClick(func() {
							if v.sensorMgr != nil {
								v.status = ""
								src := collector.SensorSourceIDFromLabel(v.sourceLabel)
								v.sensorMgr.StartDownloadAndInstall(src, v.draft.SensorCustomMirrorURL)
							}
						}).Border(0, ui.Transparent)
					default:
						ui.PrimaryButton(c, i18n.S("下载并安装 PawnIO", "Download & Install PawnIO")).OnClick(func() {
							if v.sensorMgr != nil {
								v.status = ""
								src := collector.SensorSourceIDFromLabel(v.sourceLabel)
								v.sensorMgr.StartDownloadAndInstall(src, v.draft.SensorCustomMirrorURL)
							}
						})
						if st.SetupDownloaded {
							ui.Button(c, i18n.S("静默安装 PawnIO 驱动", "Install PawnIO Driver")).OnClick(func() {
								if v.sensorMgr != nil {
									v.status = ""
									v.sensorMgr.StartSilentInstall()
								}
							}).Border(0, ui.Transparent)
						}
					}
					ui.Button(c, i18n.S("从本地导入…", "Import Local…")).OnClick(func() {
						if !v.headless() && v.sensorMgr != nil {
							v.importLocalSensorFile(c)
						}
					}).Border(0, ui.Transparent)
					if st.SetupDownloaded || st.ModulesReady {
						ui.Button(c, i18n.S("移除本地文件", "Remove Local Files")).OnClick(func() {
							if v.sensorMgr != nil {
								_ = v.sensorMgr.RemoveLocalFiles()
								v.status = i18n.S("已移除本地传感器扩展文件", "Removed local sensor extension files")
								v.invalidate()
							}
						}).Border(0, ui.Transparent)
					}
				}
			})
		}

		msg := strings.TrimSpace(v.status)
		msgCol := pal.TextMuted
		if !prog.Active && strings.TrimSpace(prog.Err) != "" {
			msg = prog.Err
			msgCol = pal.Danger
		}
		if msg != "" {
			ui.Text(c, msg).FontSize(11.5).TextColor(msgCol)
		}
	})
}

func (v *settingsTabView) importLocalSensorFile(c *ui.Context) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: i18n.S("选择 PawnIO_setup.exe、模块包 (.zip) 或 .bin 模块", "Select PawnIO_setup.exe, Module Archive (.zip), or .bin"),
			Filters: []mygo.FileFilter{
				{Name: i18n.S("PawnIO 文件", "PawnIO Files"), Extensions: []string{"exe", "zip", "bin", "amx"}},
				{Name: i18n.S("所有文件", "All Files"), Extensions: []string{"*"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		impErr := v.sensorMgr.ImportLocal(paths[0])
		mygo.RunOnMain(func() {
			if impErr != nil {
				v.status = impErr.Error()
			} else {
				v.status = i18n.S("已成功导入本地 PawnIO 文件", "Imported local PawnIO asset")
				if c != nil {
					c.Toast(v.status)
				}
			}
			v.invalidate()
		})
	}()
}
