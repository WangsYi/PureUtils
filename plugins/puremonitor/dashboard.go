//go:build windows

package puremonitor

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/puremonitor/collector"
	"pureutils/plugins/puremonitor/fps"
)

// Dashboard is the MyGo GPU-rendered hardware specs, live telemetry, sensors, and game FPS window.
type Dashboard struct {
	plugin *Plugin
	host   purekit.Host

	mu        sync.Mutex
	win       *mygo.Window
	open      bool
	tabIdx    int // 0=overview, 1=hardware, 2=sensors, 3=games
	sensorCat int // 0=All, 1=CPU, 2=GPU, 3=Memory, 4=Disk, 5=System
	status    string
}

// NewDashboard constructs a Dashboard controller bound to plugin p.
func NewDashboard(p *Plugin, host purekit.Host) *Dashboard {
	return &Dashboard{
		plugin: p,
		host:   host,
	}
}

// IsOpen reports whether the Dashboard window is currently open.
func (d *Dashboard) IsOpen() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.open
}

// Open shows or focuses the Dashboard window (no-op window in headless mode).
func (d *Dashboard) Open() {
	d.mu.Lock()
	if d.win != nil {
		w := d.win
		d.open = true
		d.mu.Unlock()
		w.Show()
		w.Focus()
		w.Invalidate()
		return
	}
	d.open = true
	if d.host == nil || d.host.Headless() {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     i18n.S("PureMonitor — 硬件信息与系统监控", "PureMonitor — Hardware & System Monitor"),
		Width:     920,
		Height:    660,
		MinWidth:  720,
		MinHeight: 520,
		StateKey:  "puremonitor-dashboard",
		Content:   ui.View(d.View),
	})
	_ = win.SetIcon(monitorIconPNG)
	win.OnClose(func(_ *mygo.CloseEvent) {
		d.mu.Lock()
		if d.win == win {
			d.win = nil
			d.open = false
		}
		d.mu.Unlock()
		if d.host != nil {
			d.host.NotifyWindowClosed()
		}
	})
	d.mu.Lock()
	d.win = win
	d.mu.Unlock()
	win.Focus()
}

// Close closes the Dashboard window if open.
func (d *Dashboard) Close() {
	d.mu.Lock()
	w := d.win
	d.win = nil
	d.open = false
	d.mu.Unlock()
	if w != nil {
		mygo.RunOnMain(func() { w.Close() })
	}
}

// Refresh requests a UI redraw when live telemetry ticks.
func (d *Dashboard) Refresh() {
	d.mu.Lock()
	w := d.win
	d.mu.Unlock()
	if w != nil {
		mygo.RunOnMain(func() { w.Invalidate() })
	}
}

// SetTab switches the active dashboard sub-tab (0..3).
func (d *Dashboard) SetTab(idx int) {
	if idx < 0 || idx > 3 {
		return
	}
	d.mu.Lock()
	d.tabIdx = idx
	d.mu.Unlock()
}

// View renders the full MyGo Dashboard interface.
func (d *Dashboard) View(c *ui.Context) {
	pal := pureui.ApplyTheme(c)
	cfg := d.plugin.Config()
	snap := d.plugin.Snapshot()
	inv := d.plugin.Inventory()

	ui.Column(c).Fill().Background(pal.Bg).Children(func() {
		// Top navigation header
		ui.Row(c).Padding(12, 18).Gap(10).AlignItems(ui.Center).Background(pal.BgElevated).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Grow(1).MinWidth(0).Children(func() {
				ui.Box(c).Size(32, 32).Radius(9).Gradient(pal.Accent, pal.AccentEnd, 135).Center().Shrink(0).Children(func() {
					ui.Icon(c, pureui.IconActivity).Size(17, 17).TextColor(pal.AccentText)
				})
				ui.Column(c).Gap(1).MinWidth(0).Children(func() {
					ui.Text(c, "PureMonitor").FontSize(14).FontWeight(700).SingleLine().TextColor(pal.Text)
					ui.Text(c, invSubtitle(inv, snap)).FontSize(11).SingleLine().TextColor(pal.TextMuted)
				})
			})
			ui.Segmented(c, &d.tabIdx,
				i18n.S("实时概览", "Overview"),
				i18n.S("硬件配置", "Hardware"),
				i18n.S("传感器", "Sensors"),
				i18n.S("进程与游戏", "Processes & FPS"),
			).Label(i18n.S("监控视图切换", "Monitor section")).Shrink(0)
			pureui.IconButton(c, pureui.IconSettings, i18n.S("设置", "Settings"), i18n.S("打开 PureMonitor 高级设置", "Open PureMonitor Settings"), false, false, pal).OnClick(func() {
				if d.host != nil {
					d.host.OpenSettings(PluginID)
				}
			})
		})
		ui.Divider(c)

		// Main scrollable area
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16, 18).Gap(12).FillWidth().Children(func() {
				switch d.tabIdx {
				case 0:
					d.renderOverview(c, pal, cfg, snap, inv)
				case 1:
					d.renderHardware(c, pal, inv)
				case 2:
					d.renderSensors(c, pal, cfg, snap)
				case 3:
					d.renderGamesAndProcs(c, pal, cfg, snap)
				}
			})
		})
	})
}

func invSubtitle(inv collector.HardwareInventory, snap collector.SystemSnapshot) string {
	parts := []string{}
	if inv.CPU.Name != "" {
		parts = append(parts, inv.CPU.Name)
	}
	if snap.RAMTotalBytes > 0 {
		parts = append(parts, fmt.Sprintf("%.0f GB RAM", float64(snap.RAMTotalBytes)/(1024*1024*1024)))
	}
	if len(parts) == 0 {
		return i18n.S("系统状态与硬件监控", "System & hardware monitor")
	}
	return strings.Join(parts, " · ")
}

func (d *Dashboard) renderOverview(c *ui.Context, pal pureui.Palette, cfg config.PureMonitorConfig, snap collector.SystemSnapshot, inv collector.HardwareInventory) {
	// Quick Overlay Controls bar
	pureui.SettingCard(c, i18n.S("快速显示开关", "Quick Display Toggles"), "", pal, func() {
		ui.Row(c).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
			tb := cfg.ShowTaskbar
			flt := cfg.ShowFloating
			hud := cfg.GameFPSEnabled
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				sw := ui.Switch(c, &tb).Label(i18n.S("任务栏监控", "Taskbar Monitor"))
				if sw.Changed() {
					next := cfg.Clone()
					next.ShowTaskbar = tb
					_ = d.plugin.ApplyConfig(next)
				}
				ui.Text(c, i18n.S("任务栏监控", "Taskbar")).FontSize(12.5).FontWeight(500).TextColor(pal.Text)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				sw := ui.Switch(c, &flt).Label(i18n.S("桌面悬浮窗", "Floating Window"))
				if sw.Changed() {
					next := cfg.Clone()
					next.ShowFloating = flt
					_ = d.plugin.ApplyConfig(next)
				}
				ui.Text(c, i18n.S("桌面悬浮窗", "Floating Pill")).FontSize(12.5).FontWeight(500).TextColor(pal.Text)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				sw := ui.Switch(c, &hud).Label(i18n.S("游戏帧率 HUD", "Game FPS HUD"))
				if sw.Changed() {
					next := cfg.Clone()
					next.GameFPSEnabled = hud
					_ = d.plugin.ApplyConfig(next)
				}
				ui.Text(c, i18n.S("游戏帧率 HUD", "Game FPS HUD")).FontSize(12.5).FontWeight(500).TextColor(pal.Text)
			})
		})
	})

	// 2x2 Summary Cards: Row 1 (CPU + RAM)
	ui.Row(c).Gap(12).FillWidth().Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			cpuTempStr := collector.FormatTemp(snap.CPUTempC, cfg.TempUnit)
			cpuSub := i18n.F("%.2f GHz · 温度 %s · 功耗 %.0fW", "%.2f GHz · Temp %s · Power %.0fW",
				snap.CPUFreqMHz/1000.0, cpuTempStr, snap.CPUPowerW)
			if snap.CPUTempC <= 0 && snap.CPUPowerW <= 0 {
				cpuSub = i18n.F("%.2f GHz · 核心 %d/%d", "%.2f GHz · %dC/%dT", snap.CPUFreqMHz/1000.0, inv.CPU.PhysicalCores, inv.CPU.LogicalCores)
			}
			d.renderMetricSummaryCard(c, pal, pureui.IconCpu, i18n.S("处理器 (CPU)", "Processor (CPU)"),
				fmt.Sprintf("%.1f%%", snap.CPUPercent), cpuSub, snap.CPUPercent/100.0, pal.Accent, d.plugin.HistorySamples("cpu"), 100)
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			usedGB := float64(snap.RAMUsedBytes) / (1024 * 1024 * 1024)
			totGB := float64(snap.RAMTotalBytes) / (1024 * 1024 * 1024)
			commitGB := float64(snap.CommitUsedBytes) / (1024 * 1024 * 1024)
			ramSub := i18n.F("已用 %.1f / %.1f GB · 提交 %.1f GB", "%.1f / %.1f GB used · Commit %.1f GB", usedGB, totGB, commitGB)
			d.renderMetricSummaryCard(c, pal, pureui.IconLayers, i18n.S("内存 (RAM)", "Memory (RAM)"),
				fmt.Sprintf("%.1f%%", snap.RAMPct), ramSub, snap.RAMPct/100.0, pal.KindRich, d.plugin.HistorySamples("ram"), 100)
		})
	})

	// Row 2 (GPU + Network/Disk)
	ui.Row(c).Gap(12).FillWidth().Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			gpu, hasGPU := snap.PrimaryGPU()
			gpuVal := "--"
			gpuSub := i18n.S("未检测到活跃显卡", "No active GPU telemetry")
			pct := 0.0
			if hasGPU {
				gpuVal = fmt.Sprintf("%.0f%%", gpu.Load3D)
				pct = gpu.Load3D / 100.0
				gpuSub = i18n.F("%s · 温度 %s · 显存 %.1f/%.1f GB", "%s · Temp %s · VRAM %.1f/%.1f GB",
					truncateShort(gpu.Name, 22), collector.FormatTemp(gpu.TempC, cfg.TempUnit), gpu.VRAMUsedMB/1024.0, gpu.VRAMTotalMB/1024.0)
			}
			d.renderMetricSummaryCard(c, pal, pureui.IconMonitor, i18n.S("显卡 (GPU)", "Graphics (GPU)"),
				gpuVal, gpuSub, pct, pal.Success, d.plugin.HistorySamples("gpu"), 100)
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			netHead := fmt.Sprintf("↓ %s  ↑ %s",
				collector.FormatBytesPerSec(snap.NetDownBps, cfg.UnitSpeed),
				collector.FormatBytesPerSec(snap.NetUpBps, cfg.UnitSpeed))
			netSub := i18n.F("磁盘读写 %s / %s · 最高盘温 %s", "Disk R/W %s / %s · Max Temp %s",
				collector.FormatBytesPerSec(snap.DiskReadBps, "auto"),
				collector.FormatBytesPerSec(snap.DiskWriteBps, "auto"),
				collector.FormatTemp(snap.MaxDiskTempC(), cfg.TempUnit))
			d.renderMetricSummaryCard(c, pal, pureui.IconHardDrive, i18n.S("网络与存储", "Network & Disk"),
				netHead, netSub, 0, pal.KindImage, d.plugin.HistorySamples("net_down"), 0)
		})
	})

	// System status footer card
	pureui.SettingCard(c, i18n.S("运行状态与游戏帧率", "System Uptime & Game FPS"), "", pal, func() {
		ui.Row(c).Gap(16).Wrap().AlignItems(ui.Center).Children(func() {
			upHours := int(snap.Uptime.Hours())
			upMins := int(snap.Uptime.Minutes()) % 60
			pureui.PillBadge(c, i18n.F("运行时间 %d小时%d分", "Uptime %dh %dm", upHours, upMins), pal.TextSecondary, pal.ControlBg)
			pureui.PillBadge(c, i18n.F("进程 %d · 线程 %d · 句柄 %d", "Procs %d · Threads %d · Handles %d",
				snap.ProcessCount, snap.ThreadCount, snap.HandleCount), pal.TextSecondary, pal.ControlBg)
			if snap.Battery.Present {
				batLbl := i18n.F("电池 %d%%", "Battery %d%%", snap.Battery.Percent)
				if snap.Battery.Charging {
					batLbl = i18n.F("充电中 %d%%", "Charging %d%%", snap.Battery.Percent)
				}
				pureui.PillBadge(c, batLbl, pal.Success, pal.SuccessSoft)
			}
			if snap.PawnIOActive {
				pureui.PillBadge(c, i18n.S("PawnIO 传感器已连接", "PawnIO Active"), pal.Success, pal.SuccessSoft)
			}
			if snap.FPSActive && snap.GameName != "" {
				pureui.PillBadge(c, fmt.Sprintf("%s: %.0f FPS (1%% %.0f)", snap.GameName, snap.FPS, snap.FPS1Low), pal.Accent, pal.AccentSoft)
			}
		})
	})
}

func (d *Dashboard) renderMetricSummaryCard(c *ui.Context, pal pureui.Palette, icon *ui.SVG, title, value, subtitle string, progress float64, accent ui.Color, samples []float64, fixedMax float64) {
	card := ui.Column(c).Padding(14).Gap(8).Radius(12).Background(pal.CardBg).Border(1, pal.Border)
	if !pal.Dark {
		card.Shadow(0, 1, 4, 0, ui.RGBA(15, 23, 42, 0.045))
	}
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Box(c).Size(24, 24).Radius(6).Background(accent.Alpha(0.14)).Center().Shrink(0).Children(func() {
				ui.Icon(c, icon).Size(14, 14).TextColor(accent)
			})
			ui.Text(c, title).FontSize(12.5).FontWeight(600).TextColor(pal.Text).Grow(1).MinWidth(0)
			ui.Text(c, value).FontSize(15).FontWeight(700).SingleLine().TextColor(accent).Shrink(0)
		})
		ui.Text(c, subtitle).FontSize(11).TextColor(pal.TextMuted)
		if progress > 0 {
			ui.Progress(c, math.Min(1, math.Max(0, progress))).FillWidth().Height(5).Radius(3)
		}
		renderSparklineBox(c, pal, accent, samples, fixedMax)
	})
}

func renderSparklineBox(c *ui.Context, pal pureui.Palette, lineCol ui.Color, samples []float64, fixedMax float64) {
	ui.Box(c).Height(38).FillWidth().Radius(7).Background(pal.BgInset).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		if len(samples) < 2 || r.W <= 4 || r.H <= 4 {
			return
		}
		maxV := fixedMax
		if maxV <= 0 {
			maxV = 1
			for _, v := range samples {
				if v > maxV {
					maxV = v
				}
			}
		}
		pts := make([][2]float32, len(samples))
		pad := float32(4)
		for i, v := range samples {
			x := r.X + pad + float32(i)/float32(len(samples)-1)*(r.W-2*pad)
			norm := float32(math.Min(1, math.Max(0, v/maxV)))
			y := r.Y + r.H - pad - norm*(r.H-2*pad)
			pts[i] = [2]float32{x, y}
		}
		p.StrokePolyline(pts, 1.8, lineCol)
	})
}

func (d *Dashboard) renderHardware(c *ui.Context, pal pureui.Palette, inv collector.HardwareInventory) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
		ui.PrimaryButton(c, i18n.S("复制硬件配置单", "Copy Hardware Spec")).OnClick(func() {
			summary := collector.FormatSpecSummary(inv)
			if d.host != nil && d.host.Clipboard() != nil {
				_ = d.host.Clipboard().WriteText(summary, PluginID)
			}
			d.status = i18n.S("已复制完整硬件配置单到剪贴板", "Hardware specification copied to clipboard")
			c.Toast(d.status)
		})
		ui.Button(c, i18n.S("重新扫描硬件", "Rescan Hardware")).OnClick(func() {
			if d.plugin.engine != nil && d.host != nil && !d.host.Headless() {
				go func() {
					d.plugin.engine.RefreshInventory()
					d.Refresh()
				}()
			}
			d.status = i18n.S("已刷新硬件信息", "Hardware inventory refreshed")
		}).Border(0, ui.Transparent)
		if d.status != "" {
			ui.Text(c, d.status).FontSize(11.5).TextColor(pal.Success)
		}
	})

	pureui.SettingCard(c, i18n.S("系统与主板", "System & Motherboard"), "", pal, func() {
		kvRow(c, pal, i18n.S("操作系统", "Operating System"), fmt.Sprintf("%s %s (Build %s, %s)", inv.OS.Name, inv.OS.Version, inv.OS.Build, inv.OS.Arch))
		if inv.System.Manufacturer != "" || inv.System.Model != "" {
			ui.Divider(c)
			kvRow(c, pal, i18n.S("整机型号", "System Model"), strings.TrimSpace(inv.System.Manufacturer+" "+inv.System.Model))
		}
		if inv.Motherboard.Manufacturer != "" || inv.Motherboard.Product != "" {
			ui.Divider(c)
			kvRow(c, pal, i18n.S("主板型号", "Motherboard"), strings.TrimSpace(inv.Motherboard.Manufacturer+" "+inv.Motherboard.Product))
		}
		if inv.Motherboard.BIOSVersion != "" {
			ui.Divider(c)
			kvRow(c, pal, "BIOS", strings.TrimSpace(inv.Motherboard.BIOSVendor+" "+inv.Motherboard.BIOSVersion+" ("+inv.Motherboard.BIOSDate+")"))
		}
	})

	pureui.SettingCard(c, i18n.S("处理器 (CPU)", "Processor (CPU)"), "", pal, func() {
		kvRow(c, pal, i18n.S("处理器型号", "CPU Model"), inv.CPU.Name)
		ui.Divider(c)
		topo := i18n.F("%d 核 %d 线程", "%d Cores / %d Threads", inv.CPU.PhysicalCores, inv.CPU.LogicalCores)
		if inv.CPU.PCores > 0 && inv.CPU.ECores > 0 {
			topo = i18n.F("%d 核 %d 线程 (%d P + %d E)", "%d Cores / %d Threads (%dP + %dE)",
				inv.CPU.PhysicalCores, inv.CPU.LogicalCores, inv.CPU.PCores, inv.CPU.ECores)
		}
		kvRow(c, pal, i18n.S("核心规格", "Topology"), topo)
		if inv.CPU.Socket != "" || inv.CPU.MaxClockMHz > 0 {
			ui.Divider(c)
			kvRow(c, pal, i18n.S("插槽与频率", "Socket & Clock"), fmt.Sprintf("%s · Base %d MHz / Max %d MHz", inv.CPU.Socket, inv.CPU.BaseClockMHz, inv.CPU.MaxClockMHz))
		}
		if inv.CPU.L2CacheKB > 0 || inv.CPU.L3CacheKB > 0 {
			ui.Divider(c)
			kvRow(c, pal, i18n.S("各级缓存", "Caches"), fmt.Sprintf("L1 %d KB · L2 %d KB · L3 %.1f MB", inv.CPU.L1CacheKB, inv.CPU.L2CacheKB, float64(inv.CPU.L3CacheKB)/1024.0))
		}
	})

	pureui.SettingCard(c, i18n.S("显卡 (GPU)", "Graphics Adapters"), "", pal, func() {
		if len(inv.GPUs) == 0 {
			ui.Text(c, i18n.S("未检测到独立或集成显卡", "No GPUs detected")).FontSize(12).TextColor(pal.TextMuted)
			return
		}
		for i, g := range inv.GPUs {
			if i > 0 {
				ui.Divider(c)
			}
			val := fmt.Sprintf("%s · VRAM %d MB", g.Vendor, g.VRAMMB)
			if g.DriverVersion != "" {
				val += fmt.Sprintf(" · Driver %s", g.DriverVersion)
			}
			kvRow(c, pal, g.Name, val)
		}
	})

	pureui.SettingCard(c, i18n.S("内存模组 (RAM)", "Memory Modules"), i18n.F("总物理内存 %.1f GB", "Total Physical RAM %.1f GB", float64(inv.TotalRAMMB)/1024.0), pal, func() {
		if len(inv.RAMModules) == 0 {
			ui.Text(c, i18n.S("SMBIOS 未提供单条内存明细", "SMBIOS did not expose per-slot DIMM details")).FontSize(12).TextColor(pal.TextMuted)
			return
		}
		for i, m := range inv.RAMModules {
			if i > 0 {
				ui.Divider(c)
			}
			slotLbl := m.Slot
			if slotLbl == "" {
				slotLbl = fmt.Sprintf("DIMM %d", i+1)
			}
			detail := fmt.Sprintf("%d GB %s-%d", m.CapacityMB/1024, m.Type, m.SpeedMTs)
			if m.Manufacturer != "" || m.PartNumber != "" {
				detail += " · " + strings.TrimSpace(m.Manufacturer+" "+m.PartNumber)
			}
			kvRow(c, pal, slotLbl, detail)
		}
	})

	pureui.SettingCard(c, i18n.S("存储设备与分区", "Storage & Volumes"), "", pal, func() {
		for i, dk := range inv.Disks {
			if i > 0 {
				ui.Divider(c)
			}
			kvRow(c, pal, dk.Model, fmt.Sprintf("%s · %s · %s", dk.BusType, dk.MediaType, collector.FormatBytes(dk.SizeBytes)))
		}
		for _, vol := range inv.Volumes {
			ui.Divider(c)
			used := vol.TotalBytes - vol.FreeBytes
			pct := 0.0
			if vol.TotalBytes > 0 {
				pct = float64(used) / float64(vol.TotalBytes)
			}
			lbl := vol.Drive
			if vol.Label != "" {
				lbl += " (" + vol.Label + ")"
			}
			ui.Column(c).Gap(4).FillWidth().Children(func() {
				kvRow(c, pal, lbl, fmt.Sprintf("%s / %s (%s)", collector.FormatBytes(used), collector.FormatBytes(vol.TotalBytes), vol.FileSystem))
				ui.Progress(c, pct).FillWidth().Height(5).Radius(3)
			})
		}
	})

	pureui.SettingCard(c, i18n.S("网卡与显示器", "Network & Displays"), "", pal, func() {
		for i, nic := range inv.NetAdapters {
			if i > 0 {
				ui.Divider(c)
			}
			state := i18n.S("未连接", "Disconnected")
			if nic.Connected {
				state = i18n.F("已连接 %d Mbps", "Connected %d Mbps", nic.SpeedMbps)
			}
			if nic.IPv4 != "" {
				state += " · " + nic.IPv4
			}
			kvRow(c, pal, nic.Name, state)
		}
		for _, disp := range inv.Displays {
			ui.Divider(c)
			tag := ""
			if disp.Primary {
				tag = i18n.S(" [主屏]", " [Primary]")
			}
			kvRow(c, pal, disp.Name+tag, fmt.Sprintf("%s @ %d Hz (%s)", disp.Resolution, disp.RefreshHz, disp.Adapter))
		}
	})
}

func (d *Dashboard) renderSensors(c *ui.Context, pal pureui.Palette, cfg config.PureMonitorConfig, snap collector.SystemSnapshot) {
	if snap.CPUTempC <= 0 && !snap.PawnIOActive {
		var st collector.SensorExtStatus
		if mgr := d.plugin.SensorManager(); mgr != nil {
			st = mgr.Status()
		}
		if st.DriverInstalled && st.ModulesReady {
			pureui.SettingCardTip(c, i18n.S("CPU 与硬盘温度需提权采集", "Elevated Helper Needed for CPU & Disk Temps"),
				i18n.S("已检测到本机 PawnIO 驱动与传感器模块。受 Windows 内核访问限制，读取 CPU 封装温度与硬盘 SMART 温度需启动一次便携提权采集子进程。",
					"Local PawnIO driver and modules detected. Windows requires elevated access to read CPU package & disk SMART thermal sensors."),
				pal, func() {
					ui.Row(c).Gap(8).Wrap().Children(func() {
						ui.PrimaryButton(c, i18n.S("提权启动传感器采集", "Start Elevated Sensor Helper")).OnClick(func() {
							if d.host != nil && !d.host.Headless() {
								if err := d.plugin.StartElevatedHelper("both"); err != nil {
									d.status = err.Error()
								} else {
									d.status = i18n.S("已启动便携采集子进程读取 CPU 与硬盘温度", "Elevated helper started for CPU & disk sensors")
								}
							}
						})
						ui.Button(c, i18n.S("打开设置", "Settings")).OnClick(func() {
							if d.host != nil {
								d.host.OpenSettings(PluginID)
							}
						}).Border(0, ui.Transparent)
					})
				})
		} else {
			pureui.SettingCardTip(c, i18n.S("CPU 与硬盘温度扩展未启用", "CPU & Disk Temperature Extension Not Active"),
				i18n.S("Windows 默认限制非驱动读取 CPU 封装温度与硬盘 SMART 温度；可在设置中按需下载安装签名 PawnIO 扩展或启动提权采集。",
					"Windows restricts CPU package & disk SMART thermal sensors without a signed driver; install the optional PawnIO extension or start the elevated helper in Settings."),
				pal, func() {
					ui.Row(c).Gap(8).Wrap().Children(func() {
						ui.PrimaryButton(c, i18n.S("前往配置 PawnIO", "Configure PawnIO")).OnClick(func() {
							if d.host != nil {
								d.host.OpenSettings(PluginID)
							}
						})
					})
				})
		}
	}

	ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
		ui.Segmented(c, &d.sensorCat,
			i18n.S("全部", "All"),
			"CPU",
			"GPU",
			i18n.S("内存", "Memory"),
			i18n.S("硬盘", "Disk"),
			i18n.S("系统", "System"),
		).Label(i18n.S("传感器分类过滤", "Sensor category filter"))
	})

	catNames := []string{"", "CPU", "GPU", "Memory", "Disk", "System"}
	wantCat := ""
	if d.sensorCat > 0 && d.sensorCat < len(catNames) {
		wantCat = catNames[d.sensorCat]
	}

	pureui.SettingCardTip(c, i18n.S("实时硬件传感器读数", "Live Hardware Sensors"), i18n.S("右侧依次显示当前读数与本次会话最小 / 最大峰值", "Columns on the right show current reading and session min / max peaks"), pal, func() {
		count := 0
		for _, s := range snap.Sensors {
			if wantCat != "" && !strings.EqualFold(s.Category, wantCat) {
				continue
			}
			if count > 0 {
				ui.Divider(c)
			}
			count++
			curStr := formatSensorReading(s.Value, s.Unit, cfg.TempUnit)
			minStr := formatSensorReading(s.Min, s.Unit, cfg.TempUnit)
			maxStr := formatSensorReading(s.Max, s.Unit, cfg.TempUnit)
			ui.Row(c).AlignItems(ui.Center).Gap(8).FillWidth().Children(func() {
				ui.Text(c, s.Name).FontSize(12.5).FontWeight(500).TextColor(pal.Text).Grow(1).MinWidth(0)
				pureui.PillBadge(c, s.Source, pal.TextMuted, pal.ControlBg)
				ui.Text(c, curStr).FontSize(12.5).FontWeight(700).SingleLine().TextColor(pal.Accent).Width(78).TextAlign(ui.End).Shrink(0)
				ui.Text(c, minStr+" / "+maxStr).FontSize(11).SingleLine().TextColor(pal.TextMuted).Width(120).TextAlign(ui.End).Shrink(0)
			})
		}
		if count == 0 {
			ui.Text(c, i18n.S("当前分类下暂无传感器读数", "No sensor readings available in this category")).FontSize(12).TextColor(pal.TextMuted)
		}
	})
}

func formatSensorReading(val float64, unit, tempUnit string) string {
	if unit == "°C" {
		return collector.FormatTemp(val, tempUnit)
	}
	if unit == "%" || unit == "MHz" || unit == "RPM" {
		return fmt.Sprintf("%.0f %s", val, unit)
	}
	return fmt.Sprintf("%.1f %s", val, unit)
}

func (d *Dashboard) renderGamesAndProcs(c *ui.Context, pal pureui.Palette, cfg config.PureMonitorConfig, snap collector.SystemSnapshot) {
	perm, etwRun, helperConn, etwErr := d.plugin.PermissionStatus()
	gState := d.plugin.GameState()

	pureui.SettingCard(c, i18n.S("游戏帧率与检测状态", "Game FPS & Auto-Detection Status"), "", pal, func() {
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			switch {
			case etwRun:
				pureui.PillBadge(c, i18n.S("ETW 帧率引擎运行中（免注入）", "ETW Engine Active (Zero-Injection)"), pal.Success, pal.SuccessSoft)
			case helperConn:
				pureui.PillBadge(c, i18n.S("便携提权采集服务已连接", "Elevated Helper Connected"), pal.Success, pal.SuccessSoft)
			case perm.CanStartETW:
				pureui.PillBadge(c, i18n.S("已授权（Performance Log Users）", "Authorized (Perf Log Users)"), pal.Accent, pal.AccentSoft)
			default:
				pureui.PillBadge(c, i18n.S("需要权限以捕获其他进程 FPS", "Permission Required for ETW FPS"), pal.Warning, pal.WarningSoft)
			}
			if gState.Active {
				pureui.PillBadge(c, fmt.Sprintf("%s (%s)", gState.ExeName, gState.Reason), pal.Accent, pal.AccentSoft)
			}
		})
		if etwErr != "" && !etwRun && !helperConn {
			ui.Text(c, etwErr).FontSize(11).TextColor(pal.Warning)
		}
		if snap.FPSActive {
			ui.Row(c).Gap(16).Wrap().Children(func() {
				pureui.PillBadge(c, fmt.Sprintf("FPS: %.0f", snap.FPS), pal.Success, pal.SuccessSoft)
				pureui.PillBadge(c, fmt.Sprintf("Avg: %.0f", snap.FPSAvg), pal.TextSecondary, pal.ControlBg)
				pureui.PillBadge(c, fmt.Sprintf("1%% Low: %.0f", snap.FPS1Low), pal.TextSecondary, pal.ControlBg)
				pureui.PillBadge(c, fmt.Sprintf("0.1%% Low: %.0f", snap.FPS01Low), pal.TextSecondary, pal.ControlBg)
				pureui.PillBadge(c, fmt.Sprintf("%.1f ms", snap.FrameTimeMs), pal.TextSecondary, pal.ControlBg)
			})
		}
		if !etwRun && !helperConn && !perm.CanStartETW {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				ui.PrimaryButton(c, i18n.S("一键加入性能日志用户组", "Grant Perf Log Users Access")).OnClick(func() {
					if err := fps.GrantPerfLogUsersPermission(); err != nil {
						d.status = err.Error()
					} else {
						d.status = i18n.S("已申请授权，注销并重新登录后生效", "Permission requested; sign out and back in to activate")
					}
				})
				ui.Button(c, i18n.S("立即启动临时提权采集", "Start Temporary Elevated Helper")).OnClick(func() {
					if err := d.plugin.StartElevatedHelper("both"); err != nil {
						d.status = err.Error()
					} else {
						d.status = i18n.S("已启动便携采集子进程", "Elevated helper started")
					}
				}).Border(0, ui.Transparent)
			})
		}
	})

	// Recent Foreground Apps for 1-click Whitelist / Blacklist
	recent := d.plugin.RecentApps()
	pureui.SettingCardTip(c, i18n.S("最近前台应用", "Recent Foreground Apps"), i18n.S("可将最近切换过的应用一键加入游戏白名单（必显 HUD）或黑名单（强制屏蔽）", "Add recent apps to the game whitelist (always show HUD) or blacklist (never show)"), pal, func() {
		if len(recent) == 0 {
			ui.Text(c, i18n.S("暂无最近记录，切换到其他应用或游戏后将自动列出", "No recent foreground apps captured yet")).FontSize(12).TextColor(pal.TextMuted)
			return
		}
		for i, app := range recent {
			if i > 0 {
				ui.Divider(c)
			}
			exe := app.ExeName
			ui.Row(c).AlignItems(ui.Center).Gap(8).FillWidth().Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, exe+".exe").FontSize(12.5).FontWeight(600).TextColor(pal.Text)
					if app.Title != "" {
						ui.Text(c, truncateShort(app.Title, 44)).FontSize(11).TextColor(pal.TextMuted)
					}
				})
				ui.Button(c, i18n.S("+ 白名单", "+ Whitelist")).Shrink(0).OnClick(func() {
					_ = d.plugin.AddGameRule(exe, true)
					d.status = i18n.F("已将 %s 加入游戏白名单", "Added %s to game whitelist", exe)
				}).Border(0, ui.Transparent)
				ui.Button(c, i18n.S("+ 黑名单", "+ Blacklist")).Shrink(0).OnClick(func() {
					_ = d.plugin.AddGameRule(exe, false)
					d.status = i18n.F("已将 %s 加入游戏黑名单", "Added %s to game blacklist", exe)
				}).Border(0, ui.Transparent)
			})
		}
	})

	// Top Processes
	ui.Row(c).Gap(12).FillWidth().Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			pureui.SettingCard(c, i18n.S("CPU 占用 Top 进程", "Top CPU Processes"), "", pal, func() {
				if len(snap.TopCPU) == 0 {
					ui.Text(c, i18n.S("等待采样…", "Sampling…")).FontSize(12).TextColor(pal.TextMuted)
					return
				}
				for i, pr := range snap.TopCPU {
					if i > 0 {
						ui.Divider(c)
					}
					ui.Row(c).AlignItems(ui.Center).Gap(8).FillWidth().Children(func() {
						ui.Text(c, pr.Name).FontSize(12).TextColor(pal.Text).Grow(1).MinWidth(0)
						ui.Text(c, fmt.Sprintf("%.1f%%", pr.CPUPct)).FontSize(12).FontWeight(600).SingleLine().TextColor(pal.Accent).Shrink(0)
					})
				}
			})
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			pureui.SettingCard(c, i18n.S("内存占用 Top 进程", "Top Memory Processes"), "", pal, func() {
				if len(snap.TopRAM) == 0 {
					ui.Text(c, i18n.S("等待采样…", "Sampling…")).FontSize(12).TextColor(pal.TextMuted)
					return
				}
				for i, pr := range snap.TopRAM {
					if i > 0 {
						ui.Divider(c)
					}
					ui.Row(c).AlignItems(ui.Center).Gap(8).FillWidth().Children(func() {
						ui.Text(c, pr.Name).FontSize(12).TextColor(pal.Text).Grow(1).MinWidth(0)
						ui.Text(c, collector.FormatBytes(pr.MemoryB)).FontSize(12).FontWeight(600).SingleLine().TextColor(pal.KindRich).Shrink(0)
					})
				}
			})
		})
	})

	_ = cfg
	_ = time.Second
}

func kvRow(c *ui.Context, pal pureui.Palette, k, v string) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).FillWidth().Children(func() {
		ui.Text(c, k).FontSize(12).FontWeight(500).TextColor(pal.TextMuted).Width(150).Shrink(0)
		ui.Text(c, v).FontSize(12.5).TextColor(pal.Text).Grow(1).MinWidth(0)
	})
}

func truncateShort(s string, maxRunes int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxRunes || maxRunes <= 1 {
		return string(r)
	}
	return string(r[:maxRunes-1]) + "…"
}
