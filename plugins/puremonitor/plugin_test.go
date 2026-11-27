//go:build windows

package puremonitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/pkg/purekit/storage"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/puremonitor/collector"
	"pureutils/plugins/puremonitor/fps"
	"pureutils/plugins/puremonitor/overlay"
)

type mockMonitorHost struct {
	paths     core.Paths
	cfgMgr    *config.Manager
	storeMgr  *storage.Manager
	bus       *purekit.Bus
	log       *core.Logger
	openedTab string
	closedCnt int
	invCount  int
}

func newMockMonitorHost(t *testing.T) *mockMonitorHost {
	t.Helper()
	home := t.TempDir()
	paths := core.Paths{
		Home:     home,
		DB:       filepath.Join(home, "pureutils.db"),
		Settings: filepath.Join(home, "settings.json"),
		Images:   filepath.Join(home, "images"),
		Logs:     filepath.Join(home, "logs"),
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	sm, err := storage.NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	return &mockMonitorHost{
		paths:    paths,
		cfgMgr:   config.Load(paths.Settings),
		storeMgr: sm,
		bus:      purekit.NewBus(),
	}
}

func (m *mockMonitorHost) Config(id string) config.Section    { return m.cfgMgr.Section(id) }
func (m *mockMonitorHost) HostConfig() config.HostConfig      { return m.cfgMgr.Host() }
func (m *mockMonitorHost) SaveHostConfig(c config.HostConfig) error { return m.cfgMgr.SaveHost(c) }
func (m *mockMonitorHost) Storage(id string) storage.PluginStorage { return m.storeMgr.Plugin(id) }
func (m *mockMonitorHost) Clipboard() clipboard.Service       { return nil }
func (m *mockMonitorHost) Bus() *purekit.Bus                  { return m.bus }
func (m *mockMonitorHost) Log() *core.Logger                  { return m.log }
func (m *mockMonitorHost) Headless() bool                     { return true }
func (m *mockMonitorHost) Paths() core.Paths                  { return m.paths }
func (m *mockMonitorHost) ModuleEnabled(core.ModuleID) bool   { return true }
func (m *mockMonitorHost) Notify(_, _ string)                 {}
func (m *mockMonitorHost) OpenSettings(tabID string)          { m.openedTab = tabID }
func (m *mockMonitorHost) ExecuteCommand(_ core.Command, _ ...string) {}
func (m *mockMonitorHost) ReloadHotkeys()                     {}
func (m *mockMonitorHost) RebuildTray()                       {}
func (m *mockMonitorHost) HotkeyError(core.Command) string    { return "" }
func (m *mockMonitorHost) SetGlobalHookSuppressed(_ string, _ bool) {}
func (m *mockMonitorHost) IsGlobalHookSuppressed() bool       { return false }
func (m *mockMonitorHost) NotifyWindowClosed()                { m.closedCnt++ }
func (m *mockMonitorHost) InvalidateSettings()                { m.invCount++ }

func sampleTestInventory() collector.HardwareInventory {
	return collector.HardwareInventory{
		OS:          collector.OSInfo{Name: "Windows 11 Pro", Version: "24H2", Build: "26300", Arch: "x64"},
		System:      collector.SystemInfo{Manufacturer: "ASUS", Model: "ROG Workstation"},
		Motherboard: collector.MotherboardInfo{Manufacturer: "ASUSTeK", Product: "ROG STRIX Z790", BIOSVendor: "AMI", BIOSVersion: "2102", BIOSDate: "2026-01-10"},
		CPU:         collector.CPUHardwareInfo{Name: "Intel Core i7-14700K", Vendor: "GenuineIntel", Socket: "LGA1700", PhysicalCores: 20, LogicalCores: 28, PCores: 8, ECores: 12, BaseClockMHz: 3400, MaxClockMHz: 5600, L1CacheKB: 1280, L2CacheKB: 28672, L3CacheKB: 33792},
		GPUs:        []collector.GPUHardwareInfo{{Name: "NVIDIA GeForce RTX 4070 Ti", Vendor: "NVIDIA", VRAMMB: 12288, DriverVersion: "566.14"}},
		RAMModules:  []collector.RAMModuleInfo{{Slot: "DIMM_A2", Manufacturer: "G.Skill", Type: "DDR5", CapacityMB: 16384, SpeedMTs: 6000}, {Slot: "DIMM_B2", Manufacturer: "G.Skill", Type: "DDR5", CapacityMB: 16384, SpeedMTs: 6000}},
		TotalRAMMB:  32768,
		Disks:       []collector.DiskHardwareInfo{{Index: 0, Model: "Samsung SSD 990 PRO 2TB", BusType: "NVMe", MediaType: "SSD", SizeBytes: 2000_398_934_016}},
		Volumes:     []collector.VolumeInfo{{Drive: "C:", Label: "Windows", FileSystem: "NTFS", TotalBytes: 1000_000_000_000, FreeBytes: 420_000_000_000}},
		NetAdapters: []collector.NetAdapterInfo{{Name: "Ethernet", Description: "Intel I226-V", SpeedMbps: 2500, IPv4: "192.168.1.50", Connected: true}},
		Displays:    []collector.DisplayHardwareInfo{{Name: "ROG PG27AQDM", Adapter: "NVIDIA GeForce RTX 4070 Ti", Resolution: "2560x1440", RefreshHz: 240, Primary: true}},
		CollectedAt: time.Now(),
	}
}

func sampleTestSnapshot() collector.SystemSnapshot {
	return collector.SystemSnapshot{
		Timestamp:       time.Now(),
		Uptime:          4*time.Hour + 28*time.Minute,
		CPUPercent:      34.6,
		CPUFreqMHz:      5180,
		CPUTempC:        61,
		CPUPowerW:       78,
		RAMUsedBytes:    14_500_000_000,
		RAMTotalBytes:   34_359_738_368,
		RAMPct:          42.2,
		CommitUsedBytes: 18_200_000_000,
		GPUs: []collector.GPULive{
			{Name: "NVIDIA GeForce RTX 4070 Ti", Vendor: "NVIDIA", Load3D: 68, LoadVideo: 4, VRAMUsedMB: 5200, VRAMTotalMB: 12288, TempC: 64, PowerW: 192, FanPct: 45, CoreClockMHz: 2685, Source: "NVML"},
		},
		Disks: []collector.DiskLive{
			{Index: 0, Name: "0 C:", ReadBps: 42 * 1024 * 1024, WriteBps: 12 * 1024 * 1024, TempC: 44},
		},
		DiskReadBps:  42 * 1024 * 1024,
		DiskWriteBps: 12 * 1024 * 1024,
		NetDownBps:   8.4 * 1024 * 1024,
		NetUpBps:     1.2 * 1024 * 1024,
		Battery:      collector.BatteryLive{Present: true, Percent: 87, Charging: true},
		ProcessCount: 215,
		ThreadCount:  3100,
		HandleCount:  92000,
		TopCPU:       []collector.ProcessUsage{{PID: 1001, Name: "Cyberpunk2077.exe", CPUPct: 24.5, MemoryB: 6_200_000_000}},
		TopRAM:       []collector.ProcessUsage{{PID: 1001, Name: "Cyberpunk2077.exe", CPUPct: 24.5, MemoryB: 6_200_000_000}},
		FPSActive:    true,
		GameName:     "cyberpunk2077",
		FPS:          118,
		FPSAvg:       114,
		FPS1Low:      86,
		FPS01Low:     72,
		FrameTimeMs:  8.5,
		RecentFrameTimes: []float32{8.2, 8.4, 8.9, 8.1, 8.5, 9.2, 8.3, 8.4, 8.6, 8.5},
		Sensors: []collector.SensorEntry{
			{ID: "cpu.load", Category: "CPU", Name: "CPU Total Load", Value: 34.6, Min: 12.0, Max: 65.0, Unit: "%", Source: "NT"},
			{ID: "cpu.temp", Category: "CPU", Name: "CPU Package Temp", Value: 61, Min: 42, Max: 74, Unit: "°C", Source: "PawnIO"},
			{ID: "gpu.0.temp", Category: "GPU", Name: "RTX 4070 Ti Core Temp", Value: 64, Min: 39, Max: 68, Unit: "°C", Source: "NVML"},
			{ID: "disk.0.temp", Category: "Disk", Name: "Samsung 990 PRO Temp", Value: 44, Min: 38, Max: 46, Unit: "°C", Source: "SMART"},
		},
		PawnIOActive: true,
	}
}

func TestPureMonitorPluginContractAndSettingsUI(t *testing.T) {
	h := newMockMonitorHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()

	if p.ID() != PluginID || p.Info().ModuleID != core.ModuleMonitor {
		t.Fatalf("Info = %+v", p.Info())
	}
	if !p.ShouldStayInTray() || p.IsBusyForGlobalHooks() {
		t.Fatalf("unexpected tray/hook state: stay=%v busy=%v", p.ShouldStayInTray(), p.IsBusyForGlobalHooks())
	}
	if len(p.Features()) != 3 || len(p.TrayItems()) != 4 || len(p.CLICommands()) != 3 {
		t.Fatalf("unexpected metadata counts: features=%d tray=%d cli=%d", len(p.Features()), len(p.TrayItems()), len(p.CLICommands()))
	}

	// Verify open dashboard command in headless mode.
	for _, f := range p.Features() {
		if f.ID == "puremonitor.open" {
			if err := f.Run(context.Background(), purekit.ActionInput{}); err != nil {
				t.Fatalf("open run: %v", err)
			}
		}
	}
	if !p.HasOpenWindows() {
		t.Fatal("expected HasOpenWindows=true after opening Dashboard")
	}

	// Whitelist / blacklist rules persistence
	if err := p.AddGameRule("EldenRing.exe", true); err != nil {
		t.Fatal(err)
	}
	if got := p.Config().GameWhitelist; len(got) != 1 || got[0] != "eldenring" {
		t.Fatalf("GameWhitelist = %v", got)
	}
	if err := p.AddGameRule("EldenRing.exe", false); err != nil {
		t.Fatal(err)
	}
	if len(p.Config().GameWhitelist) != 0 || p.Config().GameBlacklist[0] != "eldenring" {
		t.Fatalf("expected move to blacklist, got wl=%v bl=%v", p.Config().GameWhitelist, p.Config().GameBlacklist)
	}

	// Settings tab Dirty / Save / ResetDefaults
	tab := p.SettingsTab()
	if tab == nil || tab.ID != PluginID {
		t.Fatalf("SettingsTab = %+v", tab)
	}
	view := tab.NewView(h)
	if view.Dirty() {
		t.Fatal("expected clean SettingsTabView initially")
	}
	p.SetSyntheticData(sampleTestInventory(), sampleTestSnapshot())
	tt := ui.NewTester(func(c *ui.Context) {
		pal := pureui.ApplyTheme(c)
		view.Render(c, pal)
	}, 860, 640)
	tt.Frame()
	if !tt.HasText("显示方式与外观") && !tt.HasText("Display Modes & Appearance") {
		t.Fatalf("SettingsTab did not render expected headings: %v", tt.Texts())
	}

	view.ResetDefaults()
	cfg := p.Config()
	cfg.ShowFloating = true
	cfg.FloatCollapsedMetrics = []string{"net_down", "net_up", "gpu"}
	if err := p.ApplyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	gotCfg := p.Config()
	if !gotCfg.ShowFloating {
		t.Fatal("expected ShowFloating persisted")
	}
	if len(gotCfg.FloatCollapsedMetrics) != 2 || gotCfg.FloatCollapsedMetrics[0] != "net_down" || gotCfg.FloatCollapsedMetrics[1] != "net_up" {
		t.Fatalf("expected collapsed metrics capped to 2 items, got %v", gotCfg.FloatCollapsedMetrics)
	}
}

func TestOverlayImageRenderingNonBlank(t *testing.T) {
	snap := sampleTestSnapshot()
	cfg := config.DefaultPureMonitorConfig()

	for _, img := range []struct {
		name string
		rgba interface{ Bounds() interface{ Dx() int; Dy() int } }
	}{} {
		_ = img
	}

	tb := overlay.RenderTaskbarImage(snap, cfg, true, 1.5)
	if tb.Bounds().Dx() < 80 || tb.Bounds().Dy() < 24 {
		t.Fatalf("unexpected taskbar size: %v", tb.Bounds())
	}
	fltExp := overlay.RenderFloatingImage(snap, cfg, false, 1.0, false)
	fltCol := overlay.RenderFloatingImage(snap, cfg, true, 1.0, true)
	if fltExp.Bounds().Dx() <= fltCol.Bounds().Dx() {
		t.Fatalf("expected expanded float wider than collapsed pill: exp=%v col=%v", fltExp.Bounds(), fltCol.Bounds())
	}
	cfg.GameHUDStyle = "graph"
	hud := overlay.RenderGameHUDImage(snap, cfg, 1.0)
	if hud.Bounds().Dx() < 150 || hud.Bounds().Dy() < 100 {
		t.Fatalf("unexpected graph HUD bounds: %v", hud.Bounds())
	}
}

func TestPluginHotkeysTrayCLITogglesAndSamplerTick(t *testing.T) {
	h := newMockMonitorHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	if p.Matchers() != nil || p.SensorManager() == nil {
		t.Fatal("unexpected Matchers or SensorManager")
	}
	if err := p.StartElevatedHelper("both"); err != nil {
		t.Fatalf("StartElevatedHelper in headless = %v", err)
	}

	// Test Hotkeys metadata & persistence
	hks := p.Hotkeys()
	if len(hks) != 2 {
		t.Fatalf("Hotkeys len = %d, want 2", len(hks))
	}
	if err := hks[0].SetGesture("Ctrl+Alt+M"); err != nil {
		t.Fatal(err)
	}
	if err := hks[1].SetGesture("Ctrl+Alt+G"); err != nil {
		t.Fatal(err)
	}
	if hks[0].CurrentGesture() != "Ctrl+Alt+M" || hks[1].CurrentGesture() != "Ctrl+Alt+G" {
		t.Fatalf("Hotkey gestures = (%q, %q)", hks[0].CurrentGesture(), hks[1].CurrentGesture())
	}
	hks[0].Callback()
	hks[1].Callback()

	// Test TrayItems click handlers
	before := p.Config()
	for _, ti := range p.TrayItems() {
		if ti.OnClick != nil {
			ti.OnClick()
		}
	}
	afterTray := p.Config()
	if afterTray.ShowTaskbar == before.ShowTaskbar || afterTray.ShowFloating == before.ShowFloating || afterTray.GameFPSEnabled == before.GameFPSEnabled {
		t.Fatalf("expected TrayItems to toggle settings: before=%+v after=%+v", before, afterTray)
	}

	// Test CLI commands & Features (including toggling overlays back and forth + GameHUD re-enable)
	for _, cli := range p.CLICommands() {
		cli.Execute(nil)
	}
	for _, f := range p.Features() {
		_ = f.Run(context.Background(), purekit.ActionInput{})
	}
	p.toggleOverlaysCommand()
	p.toggleOverlaysCommand()
	cfgNoHUD := p.Config()
	cfgNoHUD.GameFPSEnabled = false
	_ = p.ApplyConfig(cfgNoHUD)
	p.mu.Lock()
	p.hudUserVisible = false
	p.mu.Unlock()
	p.toggleGameHUD()
	if !p.Config().GameFPSEnabled {
		t.Fatal("toggleGameHUD should re-enable GameFPSEnabled when showing HUD")
	}

	p.saveFloatState(120, 240, true, true, true)
	fltCfg := p.Config()
	if fltCfg.FloatX != 120 || fltCfg.FloatY != 240 || !fltCfg.FloatCollapsed || !fltCfg.FloatLocked || !fltCfg.FloatClickThrough {
		t.Fatalf("saveFloatState did not persist: %+v", fltCfg)
	}

	// Non-synthetic live sampleTick + HistorySamples + Inventory + Snapshot + RecentApps
	p.sampleTick()
	if snap := p.Snapshot(); snap.RAMTotalBytes == 0 {
		t.Fatal("expected live Snapshot RAMTotalBytes > 0 after sampleTick")
	}
	if inv := p.Inventory(); inv.OS.Name == "" {
		t.Fatal("expected live Inventory OS.Name non-empty")
	}
	if len(p.HistorySamples("cpu")) == 0 {
		t.Fatal("expected live HistorySamples(cpu) populated")
	}
	_ = p.RecentApps()
	if perm, _, _, _ := p.PermissionStatus(); perm.UserName == "" {
		t.Fatal("expected non-empty PermissionStatus user")
	}

	// Utility helper edge cases
	if normalizeRuleName(`C:\Games\MyGame.EXE`) != "mygame" || normalizeRuleName("  ") != "" {
		t.Fatal("normalizeRuleName edge mismatch")
	}
	if got := appendUnique([]string{"a"}, "a"); len(got) != 1 {
		t.Fatalf("appendUnique duplicate failed: %v", got)
	}
	if got := removeFromList([]string{"a", "b"}, "c"); len(got) != 2 {
		t.Fatalf("removeFromList missing element failed: %v", got)
	}
}

func TestRunHelperCLIPipeStreaming(t *testing.T) {
	if handled, code := RunHelperCLI([]string{"--monitor"}); handled || code != 0 {
		t.Fatalf("non-helper args = (%v, %d), want (false, 0)", handled, code)
	}
	if handled, code := RunHelperCLI([]string{"--monitor-helper"}); !handled || code != 2 {
		t.Fatalf("missing pipe = (%v, %d), want (true, 2)", handled, code)
	}

	srv := collector.NewHelperServer()
	if err := srv.StartListening(); err != nil {
		t.Fatalf("StartListening: %v", err)
	}
	defer srv.Stop()

	type helperRes struct {
		handled bool
		code    int
	}
	resCh := make(chan helperRes, 1)
	go func() {
		h, c := RunHelperCLI([]string{
			"--monitor-helper",
			"--pipe", srv.PipeName(),
			"--mode", "sensors",
			"--sensor-dir", t.TempDir(),
			"--cpu-vendor", "Intel",
		})
		resCh <- helperRes{h, c}
	}()

	deadline := time.Now().Add(3 * time.Second)
	gotFrame := false
	for time.Now().Before(deadline) {
		if _, ok := srv.Latest(); ok {
			gotFrame = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !gotFrame {
		t.Fatal("RunHelperCLI did not stream a HelperFrame within 3s")
	}

	srv.Stop()
	select {
	case res := <-resCh:
		if !res.handled || res.code != 0 {
			t.Fatalf("RunHelperCLI exit = (%v, %d), want (true, 0)", res.handled, res.code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunHelperCLI did not exit after server stopped")
	}
}

func TestSettingsTabInteractiveSaveAndAllVariants(t *testing.T) {
	h := newMockMonitorHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	// Exercise syncIndices across non-default enum branches.
	customCfg := config.DefaultPureMonitorConfig()
	customCfg.TaskbarMode = "overlay"
	customCfg.TaskbarAlign = "left"
	customCfg.TaskbarLayout = "single_row"
	customCfg.RefreshIntervalMs = 500
	customCfg.Theme = "dark"
	customCfg.UnitSpeed = "mbps"
	customCfg.TempUnit = "F"
	customCfg.SelectedNetAdapter = "Ethernet"
	customCfg.GameHUDPosition = "bottom_right"
	customCfg.GameHUDStyle = "graph"
	customCfg.GameWhitelist = []string{"eldenring"}
	customCfg.GameBlacklist = []string{"chrome"}
	_ = p.ApplyConfig(customCfg)

	sv := newSettingsTabView(p, h)
	if sv.Dirty() || sv.Validate() != nil {
		t.Fatalf("initial SettingsTab state invalid: dirty=%v", sv.Dirty())
	}

	// Cycle through alternate indices and test Save() & ResetDefaults().
	sv.tbModeIdx = 2
	sv.tbAlignIdx = 0
	sv.tbLayoutIdx = 0
	sv.refreshIdx = 3
	sv.themeIdx = 2
	sv.speedUnitIdx = 0
	sv.tempUnitIdx = 0
	sv.hudPosIdx = 1
	sv.hudStyleIdx = 0
	sv.nicLabel = "自动聚合全部物理网卡"
	sv.sourceLabel = collector.SensorSourceLabel(collector.SensorSourceGHProxy)
	if !sv.Dirty() {
		t.Fatal("expected Dirty=true after editing SettingsTabView")
	}
	if err := sv.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if sv.Dirty() {
		t.Fatal("expected Dirty=false right after Save")
	}
	saved := p.Config()
	if saved.TaskbarMode != "embed" || saved.RefreshIntervalMs != 2000 || saved.Theme != "light" || saved.GameHUDPosition != "top_right" || saved.GameHUDStyle != "compact" || saved.SelectedNetAdapter != "auto" {
		t.Fatalf("saved config mismatch: %+v", saved)
	}

	sv.ResetDefaults()
	if err := sv.Save(); err != nil {
		t.Fatal(err)
	}
	sv.invalidate()
	if h.invCount == 0 {
		t.Fatal("expected InvalidateSettings callback count > 0")
	}

	// Render interactive pills & rule delete buttons via ui.Tester
	prevPref, prevLang := i18n.Preference(), i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()
	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	sv2 := newSettingsTabView(p, h)
	sv2.wlInput = "witcher3.exe"
	sv2.draft.GameWhitelist = []string{"doom"}
	sv2.status = "Status test message"
	tt := ui.NewTester(func(c *ui.Context) {
		pal := pureui.ApplyTheme(c)
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Padding(16).Gap(12).Children(func() {
				sv2.Render(c, pal)
			})
		})
	}, 860, 1800)
	tt.Frame()
	_ = tt.Click("CPU Temp")
	_ = tt.Click("doom ×")
	_ = tt.Click("Add")
	tt.Frame()
}

func TestDashboardInteractiveSparklinesAndActions(t *testing.T) {
	prevPref, prevLang := i18n.Preference(), i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()
	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	h := newMockMonitorHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	// Seed 5 history samples for real GPU/CPU/RAM/Net sparkline polylines + recent app for whitelist/blacklist buttons.
	inv := sampleTestInventory()
	snap := sampleTestSnapshot()
	snap.PawnIOActive = false
	snap.CPUTempC = 0
	for i := 0; i < 5; i++ {
		s := snap
		s.CPUPercent = float64(20 + i*10)
		s.NetDownBps = float64((i + 1) * 1024 * 1024)
		p.SetSyntheticData(inv, s)
	}
	p.detector.Evaluate(fps.ForegroundInfo{PID: 555, ExeName: "balatro.exe", WindowTitle: "Balatro"}, time.Now())

	dash := p.Dashboard()
	dash.SetTab(-1)
	dash.SetTab(99)
	dash.SetTab(0)

	tt := ui.NewTester(dash.View, 920, 900)
	tt.Frame()
	_ = tt.Click("Taskbar Monitor")
	tt.Frame()

	// Hardware tab: click Copy Hardware Spec button
	dash.SetTab(1)
	tt.Frame()
	if err := tt.Click("Copy Hardware Spec"); err != nil {
		t.Fatalf("click Copy Hardware Spec: %v", err)
	}
	tt.Frame()

	// Sensors tab: inactive PawnIO banner + cycle all 6 sensor categories (including empty Disk/System on custom snap)
	dash.SetTab(2)
	for cat := 0; cat < 6; cat++ {
		dash.sensorCat = cat
		tt.Frame()
	}

	// Processes & FPS tab: verify recent app 1-click whitelist & blacklist persistence
	dash.SetTab(3)
	tt.Frame()
	if err := tt.Click("+ Whitelist"); err != nil {
		t.Fatalf("click + Whitelist: %v", err)
	}
	tt.Frame()
	if len(p.Config().GameWhitelist) == 0 || p.Config().GameWhitelist[0] != "balatro" {
		t.Fatalf("expected balatro in GameWhitelist, got %v", p.Config().GameWhitelist)
	}
	if err := tt.Click("+ Blacklist"); err != nil {
		t.Fatalf("click + Blacklist: %v", err)
	}
	tt.Frame()
	if len(p.Config().GameBlacklist) == 0 || p.Config().GameBlacklist[0] != "balatro" {
		t.Fatalf("expected balatro moved to GameBlacklist, got %v", p.Config().GameBlacklist)
	}

	dash.Refresh()
	dash.Close()
	if p.HasOpenWindows() {
		t.Fatal("expected HasOpenWindows=false after dash.Close()")
	}
}

func TestPawnIOAutoEnableOnFreshInitAndInstalledCardUI(t *testing.T) {
	prevDetect := detectPawnIOInstalled
	detectPawnIOInstalled = func() bool { return true }
	defer func() { detectPawnIOInstalled = prevDetect }()

	h := newMockMonitorHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	if !p.Config().PawnIOEnabled {
		t.Fatal("expected PawnIOEnabled=true automatically on fresh init when local PawnIO is installed")
	}

	// Explicitly disable and verify Init respects persisted user choice on subsequent init.
	cfgOff := p.Config()
	cfgOff.PawnIOEnabled = false
	if err := p.ApplyConfig(cfgOff); err != nil {
		t.Fatal(err)
	}
	p2 := NewPlugin()
	if err := p2.Init(h); err != nil {
		t.Fatal(err)
	}
	defer p2.Stop()
	if p2.Config().PawnIOEnabled {
		t.Fatal("expected persisted PawnIOEnabled=false to be respected on subsequent Init")
	}

	// Render SettingsTab when DriverInstalled=true + ModulesReady=true and verify no "Install PawnIO Driver" button appears.
	prevPref, prevLang := i18n.Preference(), i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()
	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	cfgOn := p.Config()
	cfgOn.PawnIOEnabled = true
	_ = p.ApplyConfig(cfgOn)
	modDir := filepath.Join(h.Paths().PluginDir(PluginID), "sensors")
	_ = os.MkdirAll(modDir, 0o755)
	_ = os.WriteFile(filepath.Join(modDir, "AMDFamily17.bin"), make([]byte, 64), 0o600)

	sv := newSettingsTabView(p, h)
	sv.sensorMgr.SetDriverCheckFn(func() bool { return true })
	sv.sensorMgr.Status()
	tt := ui.NewTester(func(c *ui.Context) {
		pal := pureui.ApplyTheme(c)
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Padding(16).Gap(12).Children(func() {
				sv.Render(c, pal)
			})
		})
	}, 720, 1800)
	tt.Frame()
	if tt.HasText("Install PawnIO Driver") || tt.HasText("Download & Install PawnIO") {
		t.Fatal("should not offer Install/Download PawnIO when driver is already installed locally")
	}
	if !tt.HasText("Start Elevated Sensor Helper") || !tt.HasText("Update Modules") {
		t.Fatalf("expected elevated helper and update modules actions when driver+modules ready, texts=%v", tt.Texts())
	}
}

func TestPawnIOUninstalledCardShowsDownloadAndInstallButton(t *testing.T) {
	prevDetect := detectPawnIOInstalled
	detectPawnIOInstalled = func() bool { return false }
	defer func() { detectPawnIOInstalled = prevDetect }()

	prevPref, prevLang := i18n.Preference(), i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()
	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	h := newMockMonitorHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	sv := newSettingsTabView(p, h)
	var installed bool
	sv.sensorMgr.SetDriverCheckFn(func() bool { return installed })
	sv.sensorMgr.SetInstallFn(func(_ context.Context, _ string) error {
		installed = true
		return nil
	})

	renderTab := func() *ui.Tester {
		tt := ui.NewTester(func(c *ui.Context) {
			pal := pureui.ApplyTheme(c)
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).Padding(16).Gap(12).Children(func() {
					sv.Render(c, pal)
				})
			})
		}, 720, 1800)
		tt.Frame()
		return tt
	}

	// 1. Fresh uninstalled state shows primary one-click "Download & Install PawnIO".
	tt1 := renderTab()
	if !tt1.HasText("Download & Install PawnIO") {
		t.Fatalf("expected Download & Install PawnIO button when nothing downloaded, texts=%v", tt1.Texts())
	}
	if tt1.HasText("Install PawnIO Driver") {
		t.Fatal("did not expect Install PawnIO Driver before PawnIO_setup.exe is downloaded")
	}

	// 2. When PawnIO_setup.exe + modules exist locally (e.g. UAC canceled earlier), offer silent install retry + redownload.
	modDir := filepath.Join(h.Paths().PluginDir(PluginID), "sensors")
	_ = os.MkdirAll(modDir, 0o755)
	exe := make([]byte, 2048)
	exe[0], exe[1] = 'M', 'Z'
	if err := os.WriteFile(filepath.Join(modDir, "PawnIO_setup.exe"), exe, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "IntelMSR.bin"), make([]byte, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	tt2 := renderTab()
	if !tt2.HasText("Install PawnIO Driver") || !tt2.HasText("Redownload & Install") {
		t.Fatalf("expected Install PawnIO Driver and Redownload & Install buttons when setup+modules ready, texts=%v", tt2.Texts())
	}

	// 3. Running silent install auto-enables PawnIOEnabled once driver+modules are ready.
	doneCh := make(chan struct{})
	sv.sensorMgr.SetOnChange(func() {
		sv.onSensorProgressChanged()
		if !sv.sensorMgr.Progress().Active {
			select {
			case <-doneCh:
			default:
				close(doneCh)
			}
		}
	})
	sv.sensorMgr.StartSilentInstall()
	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatalf("StartSilentInstall did not complete in UI test: %+v", sv.sensorMgr.Progress())
	}
	if !sv.draft.PawnIOEnabled || !p.Config().PawnIOEnabled {
		t.Fatalf("expected PawnIOEnabled auto-enabled after silent install, draft=%v saved=%v", sv.draft.PawnIOEnabled, p.Config().PawnIOEnabled)
	}
}

func TestExternalToggleSyncsSettingsTab(t *testing.T) {
	prevPref, prevLang := i18n.Preference(), i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()
	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	h := newMockMonitorHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	sv := newSettingsTabView(p, h)
	if sv.draft.ShowFloating {
		t.Fatal("expected ShowFloating initially false")
	}

	beforeInv := h.invCount
	p.toggleFloatingOverlay()
	if h.invCount <= beforeInv {
		t.Fatalf("expected external toggleFloatingOverlay to trigger InvalidateSettings (before=%d after=%d)", beforeInv, h.invCount)
	}

	tt := ui.NewTester(func(c *ui.Context) {
		pal := pureui.ApplyTheme(c)
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Padding(16).Gap(12).Children(func() {
				sv.Render(c, pal)
			})
		})
	}, 860, 640)
	tt.Frame()

	if !sv.draft.ShowFloating {
		t.Fatal("expected SettingsTab ShowFloating switch state synced to true after external toggle")
	}
	if sv.Dirty() {
		t.Fatal("expected SettingsTab Dirty() == false after clean external sync")
	}

	// Preserve unrelated unsaved edits while syncing another external toggle.
	sv.fontSize = 14
	if !sv.Dirty() {
		t.Fatal("expected Dirty=true with unsaved fontSize change")
	}
	prevTB := sv.draft.ShowTaskbar
	p.toggleTaskbarOverlay()
	tt.Frame()
	if sv.draft.ShowTaskbar == prevTB {
		t.Fatal("expected external ShowTaskbar toggle merged into draft")
	}
	if sv.fontSize != 14 || !sv.Dirty() {
		t.Fatalf("expected unsaved fontSize edit preserved (fontSize=%v dirty=%v)", sv.fontSize, sv.Dirty())
	}
}
