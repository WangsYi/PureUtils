//go:build windows

package puremonitor

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/puremonitor/collector"
	"pureutils/plugins/puremonitor/fps"
	"pureutils/plugins/puremonitor/overlay"
)

//go:embed monitor.png
var monitorIconPNG []byte

// detectPawnIOInstalled is overridable in tests.
var detectPawnIOInstalled = collector.IsPawnIODriverInstalled

// PluginID is the stable identifier of the PureMonitor plugin.
const PluginID = config.PluginPureMonitor

// Plugin integrates hardware inventory, system status overlays, and game FPS HUD into PureUtils.
type Plugin struct {
	mu       sync.RWMutex
	host     purekit.Host
	cfg      config.PureMonitorConfig
	engine   *collector.Engine
	sensorMg *collector.SensorExtManager
	fpsStats *fps.FrameStats
	etw      *fps.ETWEngine
	detector *fps.Detector
	probe    fps.Win32ForegroundProbe
	overlays *overlay.Manager
	dash     *Dashboard

	started        bool
	hudUserVisible bool
	gameState      fps.GameState
	fpsPerm        fps.PermissionStatus
	etwErr         string

	stopCh   chan struct{}
	loopDone chan struct{}

	synthMu   sync.Mutex
	synthSnap *collector.SystemSnapshot
	synthInv  *collector.HardwareInventory
}

// NewPlugin constructs an uninitialized PureMonitor plugin.
func NewPlugin() *Plugin {
	return &Plugin{
		hudUserVisible: true,
	}
}

func (p *Plugin) ID() string { return PluginID }

func (p *Plugin) Info() purekit.PluginInfo {
	return purekit.PluginInfo{
		ID:          PluginID,
		Name:        "PureMonitor",
		Description: i18n.S("任务栏与可收起悬浮系统监控、完整硬件配置与游戏帧率 HUD", "Taskbar & collapsible floating system monitor, hardware specs & game FPS HUD"),
		Version:     core.AppVersion,
		ModuleID:    core.ModuleMonitor,
		IconPNG:     monitorIconPNG,
	}
}

func (p *Plugin) Init(host purekit.Host) error {
	if host == nil {
		return fmt.Errorf("puremonitor: nil host")
	}
	sec := host.Config(PluginID)
	cfg := config.LoadPureMonitor(sec)
	if !sec.Exists() && detectPawnIOInstalled() {
		cfg.PawnIOEnabled = true
	}
	pluginDir := host.Paths().PluginDir(PluginID)
	sensorDir := filepath.Join(pluginDir, "sensors")

	eng := collector.NewEngine(sensorDir, cfg.PawnIOEnabled)
	eng.SetOptions(cfg.SelectedNetAdapter, cfg.PawnIOEnabled)
	sMgr := collector.NewSensorExtManager(pluginDir)
	fStats := fps.NewFrameStats()
	det := fps.NewDetector()
	det.Configure(cfg.GameAutoDetect, cfg.GameWhitelist, cfg.GameBlacklist)
	etwEng := fps.NewETWEngine(fStats)

	ov := overlay.NewManager(host.Headless(), cfg, overlay.Callbacks{
		OnOpenDashboard: p.OpenDashboard,
		OnOpenSettings: func() {
			mygo.RunOnMain(func() { host.OpenSettings(PluginID) })
		},
		OnToggleTaskbar: func() { p.toggleTaskbarOverlay() },
		OnToggleFloat:   func() { p.toggleFloatingOverlay() },
		OnFloatMoved:    p.saveFloatState,
	})

	p.mu.Lock()
	p.host = host
	p.cfg = cfg
	p.engine = eng
	p.sensorMg = sMgr
	p.fpsStats = fStats
	p.etw = etwEng
	p.detector = det
	p.overlays = ov
	p.fpsPerm = fps.CheckFPSPermission()
	p.mu.Unlock()

	return nil
}

func (p *Plugin) Start() error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = true
	host := p.host
	cfg := p.cfg
	ov := p.overlays
	p.stopCh = make(chan struct{})
	p.loopDone = make(chan struct{})
	stopCh := p.stopCh
	doneCh := p.loopDone
	p.mu.Unlock()

	if host != nil && !host.Headless() && os.Getenv("PUREUTILS_INSTANCE_ID") == "" {
		// Prime inventory asynchronously so startup stays instant.
		go func() {
			p.engine.RefreshInventory()
		}()
		if ov != nil {
			ov.ApplyConfig(cfg)
		}
		p.reconcileETW()
		go p.runSamplerLoop(stopCh, doneCh)
	} else {
		close(doneCh)
	}
	return nil
}

func (p *Plugin) Stop() error {
	p.mu.Lock()
	if !p.started && p.engine == nil && p.overlays == nil {
		p.mu.Unlock()
		return nil
	}
	p.started = false
	stopCh := p.stopCh
	doneCh := p.loopDone
	p.stopCh = nil
	p.loopDone = nil
	ov := p.overlays
	etw := p.etw
	eng := p.engine
	sMgr := p.sensorMg
	dash := p.dash
	p.dash = nil
	p.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	if doneCh != nil {
		select {
		case <-doneCh:
		case <-time.After(2 * time.Second):
		}
	}
	if sMgr != nil {
		sMgr.CancelDownload()
	}
	if etw != nil {
		etw.Stop()
	}
	if ov != nil {
		ov.Stop()
	}
	if eng != nil {
		eng.Close()
	}
	if dash != nil {
		dash.Close()
	}
	return nil
}

func (p *Plugin) runSamplerLoop(stopCh, doneCh chan struct{}) {
	defer close(doneCh)
	p.sampleTick()

	for {
		p.mu.RLock()
		interval := time.Duration(p.cfg.RefreshIntervalMs) * time.Millisecond
		p.mu.RUnlock()
		if interval < 500*time.Millisecond {
			interval = 500 * time.Millisecond
		}

		tmr := time.NewTimer(interval)
		select {
		case <-stopCh:
			tmr.Stop()
			return
		case <-tmr.C:
			p.sampleTick()
		}
	}
}

func (p *Plugin) sampleTick() {
	now := time.Now()
	p.mu.RLock()
	cfg := p.cfg
	eng := p.engine
	det := p.detector
	fStats := p.fpsStats
	etw := p.etw
	ov := p.overlays
	dash := p.dash
	hudUser := p.hudUserVisible
	p.mu.RUnlock()

	if eng == nil {
		return
	}
	snap := eng.SampleDetailed(dash != nil && dash.IsOpen())

	// Merge helper FPS if ETW runs in elevated helper instead of main process.
	var helperFPS []collector.HelperPIDFPS
	if srv := eng.HelperServer(); srv != nil {
		if hf, ok := srv.Latest(); ok {
			helperFPS = hf.PIDFPS
		}
	}

	var gState fps.GameState
	var monX, monY, monW, monH int
	if cfg.Enabled && cfg.GameFPSEnabled && det != nil {
		fg, mx, my, mw, mh := p.probe.Probe()
		monX, monY, monW, monH = mx, my, mw, mh
		if u, ok := snap.PerPIDGPU[fg.PID]; ok {
			fg.GPU3DPct = u.Util3D
			fg.GPUVideoPct = u.UtilVideo
		}
		if fStats != nil {
			fStats.Prune(now, 5*time.Second)
			if st, ok := fStats.StatsForPID(fg.PID, fps.MonotonicNowNS()); ok {
				fg.FPS = st.FPS
				snap.FPS = st.FPS
				snap.FPSAvg = st.AvgFPS
				snap.FPS1Low = st.Low1PctFPS
				snap.FPS01Low = st.Low01PctFPS
				snap.FrameTimeMs = st.FrameTimeMs
				snap.RecentFrameTimes = st.RecentFTMs
				snap.FPSActive = true
			}
		}
		if !snap.FPSActive && len(helperFPS) > 0 {
			for _, hf := range helperFPS {
				if hf.PID == fg.PID {
					fg.FPS = hf.FPS
					snap.FPS = hf.FPS
					snap.FPSAvg = hf.AvgFPS
					snap.FPS1Low = hf.Low1Pct
					snap.FPS01Low = hf.Low01Pct
					snap.FrameTimeMs = hf.FrameTimeMs
					snap.RecentFrameTimes = hf.RecentFT
					snap.FPSActive = true
					break
				}
			}
		}
		gState = det.Evaluate(fg, now)
		gState.MonitorX, gState.MonitorY, gState.MonitorW, gState.MonitorH = mx, my, mw, mh
		if gState.Active {
			snap.GameName = gState.ExeName
			snap.GamePID = gState.PID
			if snap.FPS <= 0 {
				snap.FPSActive = false
			}
		}
	}

	_ = etw
	p.mu.Lock()
	p.gameState = gState
	p.mu.Unlock()

	if ov != nil {
		hudShow := cfg.Enabled && cfg.GameFPSEnabled && hudUser && gState.Active
		ov.Update(snap, hudShow, monX, monY, monW, monH)
	}
	if dash != nil && dash.IsOpen() {
		dash.Refresh()
	}
}

func (p *Plugin) reconcileETW() {
	p.mu.RLock()
	cfg := p.cfg
	etw := p.etw
	host := p.host
	p.mu.RUnlock()
	if etw == nil || host == nil || host.Headless() || os.Getenv("PUREUTILS_INSTANCE_ID") != "" {
		return
	}
	if cfg.Enabled && cfg.GameFPSEnabled {
		if !etw.Running() {
			if err := etw.Start(); err != nil {
				p.mu.Lock()
				p.etwErr = err.Error()
				p.fpsPerm = fps.CheckFPSPermission()
				p.mu.Unlock()
				if l := host.Log(); l != nil {
					l.Warnf("puremonitor: ETW FPS start restricted (%v); grant Perf Log Users or launch elevated helper in Settings", err)
				}
			} else {
				p.mu.Lock()
				p.etwErr = ""
				p.fpsPerm = fps.CheckFPSPermission()
				p.mu.Unlock()
			}
		}
	} else {
		etw.Stop()
	}
}

// Config returns a deep copy of the current PureMonitor configuration.
func (p *Plugin) Config() config.PureMonitorConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg.Clone()
}

// ApplyConfig validates, persists, and live-applies PureMonitor configuration.
func (p *Plugin) ApplyConfig(cfg config.PureMonitorConfig) error {
	cfg.Normalize()
	p.mu.Lock()
	host := p.host
	eng := p.engine
	det := p.detector
	ov := p.overlays
	p.cfg = cfg.Clone()
	p.mu.Unlock()

	if host != nil {
		if err := config.SavePureMonitor(host.Config(PluginID), cfg); err != nil {
			return err
		}
		host.RebuildTray()
		if inv, ok := host.(purekit.SettingsInvalidator); ok {
			inv.InvalidateSettings()
		}
	}
	if eng != nil {
		eng.SetOptions(cfg.SelectedNetAdapter, cfg.PawnIOEnabled)
	}
	if det != nil {
		det.Configure(cfg.GameAutoDetect, cfg.GameWhitelist, cfg.GameBlacklist)
	}
	if ov != nil {
		ov.ApplyConfig(cfg)
	}
	p.reconcileETW()
	return nil
}

// Snapshot returns the active telemetry snapshot (or synthetic test snapshot when set).
func (p *Plugin) Snapshot() collector.SystemSnapshot {
	p.synthMu.Lock()
	if p.synthSnap != nil {
		cp := *p.synthSnap
		p.synthMu.Unlock()
		return cp
	}
	p.synthMu.Unlock()
	if p.engine != nil {
		return p.engine.LastSnapshot()
	}
	return collector.SystemSnapshot{}
}

// Inventory returns the static hardware inventory (or synthetic test inventory when set).
func (p *Plugin) Inventory() collector.HardwareInventory {
	p.synthMu.Lock()
	if p.synthInv != nil {
		cp := *p.synthInv
		p.synthMu.Unlock()
		return cp
	}
	p.synthMu.Unlock()
	if p.engine != nil {
		return p.engine.Inventory()
	}
	return collector.HardwareInventory{}
}

// SetSyntheticData injects deterministic telemetry & inventory for headless/layout tests.
func (p *Plugin) SetSyntheticData(inv collector.HardwareInventory, snap collector.SystemSnapshot) {
	p.synthMu.Lock()
	p.synthInv = &inv
	p.synthSnap = &snap
	p.synthMu.Unlock()
	if p.engine != nil {
		p.engine.History().RecordSnapshot(snap)
	}
}

// HistorySamples returns recorded sparkline values for key ("cpu", "ram", "gpu", "net_down", "net_up").
func (p *Plugin) HistorySamples(key string) []float64 {
	if p.engine == nil {
		return nil
	}
	return p.engine.History().Samples(key)
}

// SensorManager returns the optional PawnIO downloader/installer manager.
func (p *Plugin) SensorManager() *collector.SensorExtManager {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sensorMg
}

// PermissionStatus reports current ETW/PerfLogUsers/Helper status.
func (p *Plugin) PermissionStatus() (perm fps.PermissionStatus, etwRunning bool, helperConn bool, etwErr string) {
	p.mu.RLock()
	perm = fps.CheckFPSPermission()
	etwRunning = p.etw != nil && p.etw.Running()
	etwErr = p.etwErr
	eng := p.engine
	p.mu.RUnlock()
	if eng != nil {
		if srv := eng.HelperServer(); srv != nil {
			helperConn = srv.Connected()
		}
	}
	return perm, etwRunning, helperConn, etwErr
}

// PawnIOStatus reports whether PawnIO telemetry is active and whether direct access was denied (gle=5).
func (p *Plugin) PawnIOStatus() (active bool, accessDenied bool) {
	snap := p.Snapshot()
	p.mu.RLock()
	eng := p.engine
	p.mu.RUnlock()
	if eng != nil {
		accessDenied = eng.PawnIOAccessDenied()
	}
	return snap.PawnIOActive, accessDenied
}

// StartElevatedHelper launches `PureUtils.exe --monitor-helper` with UAC for portable FPS/CPU/SMART telemetry.
func (p *Plugin) StartElevatedHelper(mode string) error {
	p.mu.RLock()
	eng := p.engine
	host := p.host
	p.mu.RUnlock()
	if eng == nil || (host != nil && host.Headless()) {
		return nil
	}
	inv := eng.Inventory()
	srv := collector.NewHelperServer()
	if err := srv.StartAndLaunch(mode, host.Paths().PluginDir(PluginID)+`\sensors`, inv.CPU.Vendor); err != nil {
		return err
	}
	eng.SetHelper(srv)
	return nil
}

// RecentApps returns recently seen foreground processes from the game detector.
func (p *Plugin) RecentApps() []fps.DetectedApp {
	if p.detector == nil {
		return nil
	}
	return p.detector.RecentApps()
}

// GameState returns the latest game detection state.
func (p *Plugin) GameState() fps.GameState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.gameState
}

// AddGameRule adds exe to whitelist or blacklist and persists config.
func (p *Plugin) AddGameRule(exe string, whitelist bool) error {
	exe = normalizeRuleName(exe)
	if exe == "" {
		return nil
	}
	cfg := p.Config()
	if whitelist {
		cfg.GameWhitelist = appendUnique(removeFromList(cfg.GameWhitelist, exe), exe)
		cfg.GameBlacklist = removeFromList(cfg.GameBlacklist, exe)
	} else {
		cfg.GameBlacklist = appendUnique(removeFromList(cfg.GameBlacklist, exe), exe)
		cfg.GameWhitelist = removeFromList(cfg.GameWhitelist, exe)
	}
	return p.ApplyConfig(cfg)
}

func (p *Plugin) toggleTaskbarOverlay() {
	cfg := p.Config()
	cfg.ShowTaskbar = !cfg.ShowTaskbar
	_ = p.ApplyConfig(cfg)
}

func (p *Plugin) toggleFloatingOverlay() {
	cfg := p.Config()
	cfg.ShowFloating = !cfg.ShowFloating
	_ = p.ApplyConfig(cfg)
}

func (p *Plugin) toggleOverlaysCommand() {
	cfg := p.Config()
	if cfg.ShowTaskbar || cfg.ShowFloating {
		cfg.ShowTaskbar = false
		cfg.ShowFloating = false
	} else {
		cfg.ShowTaskbar = true
	}
	_ = p.ApplyConfig(cfg)
}

func (p *Plugin) toggleGameHUD() {
	p.mu.Lock()
	p.hudUserVisible = !p.hudUserVisible
	vis := p.hudUserVisible
	p.mu.Unlock()
	cfg := p.Config()
	if !cfg.GameFPSEnabled && vis {
		cfg.GameFPSEnabled = true
		_ = p.ApplyConfig(cfg)
	}
}

func (p *Plugin) saveFloatState(x, y int, collapsed, locked, through bool) {
	cfg := p.Config()
	cfg.FloatX = x
	cfg.FloatY = y
	cfg.FloatCollapsed = collapsed
	cfg.FloatLocked = locked
	cfg.FloatClickThrough = through
	_ = p.ApplyConfig(cfg)
}

// OpenDashboard opens or focuses the PureMonitor Hardware & System Dashboard window.
func (p *Plugin) OpenDashboard() {
	open := func() {
		p.mu.Lock()
		if p.dash == nil {
			p.dash = NewDashboard(p, p.host)
		}
		dash := p.dash
		p.mu.Unlock()
		dash.Open()
	}
	if p.host != nil && p.host.Headless() {
		open()
		return
	}
	mygo.RunOnMain(open)
}

// Dashboard returns the dashboard controller (creating it lazily for tests).
func (p *Plugin) Dashboard() *Dashboard {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dash == nil {
		p.dash = NewDashboard(p, p.host)
	}
	return p.dash
}

func (p *Plugin) HasOpenWindows() bool {
	p.mu.RLock()
	dash := p.dash
	p.mu.RUnlock()
	return dash != nil && dash.IsOpen()
}

func (p *Plugin) ShouldStayInTray() bool {
	cfg := p.Config()
	return cfg.Enabled && (cfg.ShowTaskbar || cfg.ShowFloating || cfg.GameFPSEnabled)
}

func (p *Plugin) IsBusyForGlobalHooks() bool { return false }

func (p *Plugin) Features() []purekit.Feature {
	var openShortcut, hudShortcut string
	if p.host != nil {
		hk := p.host.HostConfig().Hotkeys
		openShortcut = hk.OpenMonitor
		hudShortcut = hk.ToggleGameHUD
	}
	return []purekit.Feature{
		{
			ID:       "puremonitor.open",
			PluginID: PluginID,
			Title:    i18n.S("打开 PureMonitor 硬件与状态监控", "Open PureMonitor Hardware & System Monitor"),
			Subtitle: i18n.S("查看完整硬件配置、实时传感器曲线、进程占用与游戏帧率状态", "Inspect full hardware specs, live sensors, top processes & game FPS status"),
			Keywords: []string{"monitor", "hardware", "cpu", "gpu", "ram", "fps", "temp", "鲁大师", "硬件", "温度", "帧率", "监控", "puremonitor"},
			Command:  core.CmdOpenMonitor,
			Shortcut: openShortcut,
			Badge:    "PureMonitor",
			Icon:     pureui.IconActivity,
			Run: func(context.Context, purekit.ActionInput) error {
				p.OpenDashboard()
				return nil
			},
		},
		{
			ID:       "puremonitor.toggle_overlay",
			PluginID: PluginID,
			Title:    i18n.S("显示/隐藏任务栏与悬浮监控", "Toggle Taskbar & Floating Monitor"),
			Subtitle: i18n.S("一键开启或隐藏桌面悬浮窗与任务栏实时状态条", "Show or hide desktop floating pill and taskbar telemetry strip"),
			Keywords: []string{"taskbar", "floating", "overlay", "任务栏", "悬浮窗", "网速", "监控"},
			Command:  core.CmdToggleMonitorOverlay,
			Badge:    "PureMonitor",
			Icon:     pureui.IconMonitor,
			Run: func(context.Context, purekit.ActionInput) error {
				p.toggleOverlaysCommand()
				return nil
			},
		},
		{
			ID:       "puremonitor.toggle_hud",
			PluginID: PluginID,
			Title:    i18n.S("显示/隐藏游戏帧率 HUD", "Toggle Game FPS HUD"),
			Subtitle: i18n.S("切换游戏中屏幕角标帧率与帧时间监控显隐", "Show or hide in-game FPS & frametime corner HUD"),
			Keywords: []string{"fps", "hud", "game", "frametime", "帧率", "游戏", "性能"},
			Command:  core.CmdToggleGameHUD,
			Shortcut: hudShortcut,
			Badge:    "PureMonitor",
			Icon:     pureui.IconGamepad,
			Run: func(context.Context, purekit.ActionInput) error {
				p.toggleGameHUD()
				return nil
			},
		},
	}
}

func (p *Plugin) Matchers() []purekit.Matcher { return nil }

func (p *Plugin) Hotkeys() []purekit.HotkeySpec {
	return []purekit.HotkeySpec{
		{
			ID:             "puremonitor.open",
			Command:        core.CmdOpenMonitor,
			Label:          i18n.S("打开 PureMonitor 硬件监控", "Open PureMonitor Dashboard"),
			DefaultGesture: "",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.OpenMonitor
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.OpenMonitor = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: p.OpenDashboard,
		},
		{
			ID:             "puremonitor.toggle_hud",
			Command:        core.CmdToggleGameHUD,
			Label:          i18n.S("显隐游戏帧率 HUD", "Toggle Game FPS HUD"),
			DefaultGesture: "",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.ToggleGameHUD
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.ToggleGameHUD = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: p.toggleGameHUD,
		},
	}
}

func (p *Plugin) TrayItems() []purekit.TrayItemSpec {
	cfg := p.Config()
	return []purekit.TrayItemSpec{
		{
			ID:      "puremonitor.open",
			Section: purekit.TraySectionPrimary,
			Kind:    purekit.TrayItemAction,
			Label:   i18n.S("硬件与系统监控", "System & Hardware Monitor"),
			Command: core.CmdOpenMonitor,
			OnClick: p.OpenDashboard,
		},
		{
			ID:      "puremonitor.taskbar",
			Section: purekit.TraySectionToggle,
			Kind:    purekit.TrayItemCheckbox,
			Label:   i18n.S("任务栏系统监控", "Taskbar Monitor"),
			Checked: cfg.ShowTaskbar,
			OnClick: p.toggleTaskbarOverlay,
		},
		{
			ID:      "puremonitor.floating",
			Section: purekit.TraySectionToggle,
			Kind:    purekit.TrayItemCheckbox,
			Label:   i18n.S("桌面悬浮监控窗", "Floating Monitor"),
			Checked: cfg.ShowFloating,
			OnClick: p.toggleFloatingOverlay,
		},
		{
			ID:      "puremonitor.gamehud",
			Section: purekit.TraySectionToggle,
			Kind:    purekit.TrayItemCheckbox,
			Label:   i18n.S("游戏帧率监控", "Game FPS HUD"),
			Checked: cfg.GameFPSEnabled,
			OnClick: func() {
				c := p.Config()
				c.GameFPSEnabled = !c.GameFPSEnabled
				_ = p.ApplyConfig(c)
			},
		},
	}
}

func (p *Plugin) CLICommands() []purekit.CLICommandSpec {
	return []purekit.CLICommandSpec{
		{
			Command: core.CmdOpenMonitor,
			Execute: func([]string) { p.OpenDashboard() },
		},
		{
			Command: core.CmdToggleMonitorOverlay,
			Execute: func([]string) { p.toggleOverlaysCommand() },
		},
		{
			Command: core.CmdToggleGameHUD,
			Execute: func([]string) { p.toggleGameHUD() },
		},
	}
}

func (p *Plugin) SettingsTab() *purekit.SettingsTab {
	return &purekit.SettingsTab{
		ID:       PluginID,
		Title:    i18n.S("PureMonitor · 硬件监控", "PureMonitor · Monitor"),
		Subtitle: "",
		Icon:     pureui.IconActivity,
		Order:    60,
		NewView: func(h purekit.Host) purekit.SettingsTabView {
			return newSettingsTabView(p, h)
		},
	}
}

func normalizeRuleName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	if strings.ContainsAny(s, `\/`) {
		s = filepath.Base(s)
	}
	s = strings.TrimSuffix(s, ".exe")
	return strings.TrimSpace(s)
}

func appendUnique(list []string, item string) []string {
	for _, x := range list {
		if x == item {
			return list
		}
	}
	return append(list, item)
}

func removeFromList(list []string, item string) []string {
	var out []string
	for _, x := range list {
		if x != item {
			out = append(out, x)
		}
	}
	return out
}
