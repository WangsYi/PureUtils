//go:build windows

package overlay

import (
	"image"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"pureutils/pkg/purekit/config"
	"pureutils/plugins/puremonitor/collector"
)

func TestPureOverlayGeometryCalculations(t *testing.T) {
	tray := winRect{Left: 0, Top: 1032, Right: 1920, Bottom: 1080}
	cfg := config.DefaultPureMonitorConfig()

	// Left align + positive offset
	cfg.TaskbarAlign = "left"
	cfg.TaskbarOffsetX = 20
	lx, ly := computeTaskbarCoordsPure(tray, winRect{}, false, 240, 38, cfg, 1.0)
	if lx != 36 || ly != 1037 {
		t.Fatalf("left align = (%d, %d), want (36, 1037)", lx, ly)
	}

	// Clamping left & right bounds
	cfg.TaskbarOffsetX = -500
	cxLeft, _ := computeTaskbarCoordsPure(tray, winRect{}, false, 240, 38, cfg, 1.0)
	if cxLeft != 4 {
		t.Fatalf("clamped left = %d, want 4", cxLeft)
	}
	cfg.TaskbarOffsetX = 3000
	cxRight, _ := computeTaskbarCoordsPure(tray, winRect{}, false, 240, 38, cfg, 1.0)
	if cxRight != 1920-240-4 {
		t.Fatalf("clamped right = %d, want %d", cxRight, 1920-240-4)
	}

	// Right align with valid TrayNotifyWnd vs Win11 24H2 broken Left=0 TrayNotifyWnd
	cfg.TaskbarAlign = "right"
	cfg.TaskbarOffsetX = 0
	validNotify := winRect{Left: 1620, Top: 1032, Right: 1920, Bottom: 1080}
	rx, _ := computeTaskbarCoordsPure(tray, validNotify, true, 240, 38, cfg, 1.0)
	if rx != 1620-240-8 {
		t.Fatalf("valid notify rx = %d, want %d", rx, 1620-240-8)
	}
	brokenNotify := winRect{Left: 0, Top: 1032, Right: 20, Bottom: 1080}
	fx, _ := computeTaskbarCoordsPure(tray, brokenNotify, true, 240, 38, cfg, 0)
	if fx != 1920-240-240 {
		t.Fatalf("broken notify fallback = %d, want %d", fx, 1920-240-240)
	}

	// clampRectToWorkArea & snapRectToEdge
	wa := winRect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	if x, y := clampRectToWorkArea(-20, -10, 180, 140, wa); x != 0 || y != 0 {
		t.Fatalf("clamp top-left = (%d, %d)", x, y)
	}
	if x, y := clampRectToWorkArea(1850, 980, 180, 140, wa); x != 1740 || y != 900 {
		t.Fatalf("clamp bot-right = (%d, %d)", x, y)
	}

	if nx, _, edge := snapRectToEdge(12, 200, 180, 140, wa, true); edge != "left" || nx != 0 {
		t.Fatalf("snap left = (%d, %q)", nx, edge)
	}
	if nx, _, edge := snapRectToEdge(1730, 200, 180, 140, wa, true); edge != "right" || nx != 1740 {
		t.Fatalf("snap right = (%d, %q)", nx, edge)
	}
	if _, ny, edge := snapRectToEdge(500, 10, 180, 140, wa, true); edge != "top" || ny != 0 {
		t.Fatalf("snap top = (%d, %q)", ny, edge)
	}
	if _, _, edge := snapRectToEdge(500, 300, 180, 140, wa, true); edge != "" {
		t.Fatalf("middle edge = %q, want empty", edge)
	}
	if _, _, edge := snapRectToEdge(5, 5, 180, 140, wa, false); edge != "" {
		t.Fatalf("autoDock=false edge = %q, want empty", edge)
	}

	// computeGameHUDCoords corners
	if x, y := computeGameHUDCoords(0, 0, 1920, 1080, 200, 100, "top_left", 1.0); x != 14 || y != 14 {
		t.Fatalf("top_left = (%d, %d)", x, y)
	}
	if x, y := computeGameHUDCoords(0, 0, 1920, 1080, 200, 100, "top_right", 1.0); x != 1706 || y != 14 {
		t.Fatalf("top_right = (%d, %d)", x, y)
	}
	if x, y := computeGameHUDCoords(0, 0, 1920, 1080, 200, 100, "bottom_left", 1.0); x != 14 || y != 966 {
		t.Fatalf("bottom_left = (%d, %d)", x, y)
	}
	if x, y := computeGameHUDCoords(0, 0, 1920, 1080, 200, 100, "bottom_right", 0); x != 1706 || y != 966 {
		t.Fatalf("bottom_right = (%d, %d)", x, y)
	}
}

func TestWin32SurfaceHelpersAndDIB(t *testing.T) {
	if r := (winRect{Left: 10, Top: 20, Right: 110, Bottom: 70}); r.Width() != 100 || r.Height() != 50 {
		t.Fatalf("winRect methods mismatch: %+v", r)
	}

	var d dibSurface
	defer d.free()
	if d.ensure(0, 10) {
		t.Fatal("ensure(0, 10) should be false")
	}
	if !d.ensure(64, 32) {
		t.Fatal("ensure(64, 32) failed")
	}
	if !d.ensure(64, 32) || !d.ensure(96, 48) {
		t.Fatal("re-ensure / resize failed")
	}
	if d.commitRGBA(0, image.NewRGBA(image.Rect(0, 0, 10, 10)), 0, 0, false) {
		t.Fatal("commitRGBA with hwnd=0 should be false")
	}
	if d.commitRGBA(1, nil, 0, 0, false) {
		t.Fatal("commitRGBA with nil img should be false")
	}
	d.free()

	if err := ensureOverlayWindowClass(); err != nil {
		t.Fatalf("ensureOverlayWindowClass: %v", err)
	}
	_ = isWindows11OrNewer()
	if !isSystemDarkTheme("dark") || isSystemDarkTheme("light") {
		t.Fatal("isSystemDarkTheme explicit override mismatch")
	}
	_ = isSystemDarkTheme("system")
	if sc := windowScale(0); sc <= 0 {
		t.Fatalf("windowScale(0) = %v", sc)
	}
	if wa := monitorWorkRectAt(0, 0); wa.Width() <= 0 || wa.Height() <= 0 {
		t.Fatalf("invalid work rect: %+v", wa)
	}
	if mb := monitorBoundsAt(0, 0); mb.Width() <= 0 || mb.Height() <= 0 {
		t.Fatalf("invalid monitor bounds: %+v", mb)
	}
	_ = isTaskbarAutoHide()
}

func TestManagerHeadlessAndLiveWin32Lifecycle(t *testing.T) {
	cfg := config.DefaultPureMonitorConfig()
	cfg.ShowTaskbar = true
	cfg.ShowFloating = true
	cfg.GameFPSEnabled = true

	// Headless manager must remain inert.
	hm := NewManager(true, cfg, Callbacks{})
	hm.ApplyConfig(cfg)
	hm.Update(collector.SystemSnapshot{}, true, 0, 0, 1920, 1080)
	hm.Stop()
	if hm.taskbar != nil || hm.floating != nil || hm.gameHUD != nil {
		t.Fatal("headless manager must not allocate Win32 overlays")
	}

	// E2E isolated env must also force headless.
	t.Setenv("PUREUTILS_INSTANCE_ID", "iso-test")
	im := NewManager(false, cfg, Callbacks{})
	im.ApplyConfig(cfg)
	im.Stop()
	if !im.headless {
		t.Fatal("PUREUTILS_INSTANCE_ID must disable Win32 overlays")
	}

	// Live Win32 smoke using layered overlay mode only (never SetParent onto Explorer Shell_TrayWnd).
	t.Setenv("PUREUTILS_INSTANCE_ID", "")
	cfg.TaskbarMode = "overlay"
	cfg.TaskbarHideOnFullscreen = false
	cfg.FloatHideOnFullscreen = false
	cfg.FloatClickThrough = true

	var dashClicks atomic.Int32
	m := NewManager(false, cfg, Callbacks{
		OnOpenDashboard: func() { dashClicks.Add(1) },
		OnOpenSettings:  func() {},
		OnToggleTaskbar: func() {},
		OnToggleFloat:   func() {},
		OnFloatMoved:    func(x, y int, col, lck, thr bool) {},
	})
	defer m.Stop()

	m.ApplyConfig(cfg)
	if m.taskbar == nil || m.taskbar.hwnd == 0 || m.floating == nil || m.floating.hwnd == 0 || m.gameHUD == nil || m.gameHUD.hwnd == 0 {
		t.Fatalf("expected all 3 overlay HWNDs created: tb=%v flt=%v hud=%v", m.taskbar, m.floating, m.gameHUD)
	}

	snap := collector.SystemSnapshot{
		CPUPercent: 42,
		RAMPct:     55,
		FPSActive:  true,
		FPS:        120,
		FPS1Low:    95,
		FrameTimeMs: 8.3,
		RecentFrameTimes: []float32{8.1, 8.3, 8.5},
	}
	m.Update(snap, true, 0, 0, 1920, 1080)
	time.Sleep(80 * time.Millisecond)

	// Exercise UpdateConfig transitions and non-blocking messages across all 3 windows.
	cfg2 := cfg.Clone()
	cfg2.FloatClickThrough = false
	cfg2.FloatCollapsed = true
	cfg2.FloatX = 100
	cfg2.FloatY = 120
	cfg2.TaskbarLayout = "single_row"
	m.ApplyConfig(cfg2)

	tbHwnd := m.taskbar.hwnd
	fltHwnd := m.floating.hwnd
	hudHwnd := m.gameHUD.hwnd
	procPostMessageW.Call(tbHwnd, wmDisplayChg, 0, 0)
	procPostMessageW.Call(tbHwnd, wmTimer, tbTimerRefresh, 0)
	procPostMessageW.Call(tbHwnd, uintptr(wmTaskbarCreated), 0, 0)
	procPostMessageW.Call(tbHwnd, wmLButtonUp, 0, 0)

	procPostMessageW.Call(fltHwnd, wmSettingChg, 0, 0)
	procPostMessageW.Call(fltHwnd, wmTimer, fltTimerAltPoll, 0)
	procPostMessageW.Call(fltHwnd, wmTimer, fltTimerCollapse, 0)
	procPostMessageW.Call(fltHwnd, wmMouseLeave, 0, 0)
	procPostMessageW.Call(fltHwnd, wmLButtonDbl, 0, 0)

	procPostMessageW.Call(hudHwnd, wmAppConfig, 0, 0)
	time.Sleep(80 * time.Millisecond)
	_ = unsafe.Sizeof(winPoint{})

	if dashClicks.Load() < 1 {
		t.Logf("note: async OnOpenDashboard clicks = %d", dashClicks.Load())
	}

	// Disable overlays via ApplyConfig then Stop.
	cfgOff := cfg2.Clone()
	cfgOff.ShowTaskbar = false
	cfgOff.ShowFloating = false
	cfgOff.GameFPSEnabled = false
	m.ApplyConfig(cfgOff)
	m.Stop()
	time.Sleep(40 * time.Millisecond)
}

func TestEnsureAboveTaskbarPreservesPopupMenuOrder(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ensureOverlayWindowClass(); err != nil {
		t.Fatalf("ensureOverlayWindowClass: %v", err)
	}
	hInst, _, _ := procGetModuleHandleW.Call(0)
	clsName, _ := syscall.UTF16PtrFromString("PureUtilsMonitorOverlay")

	createTestTopmost := func(name string) uintptr {
		title, _ := syscall.UTF16PtrFromString(name)
		hwnd, _, _ := procCreateWindowExW.Call(
			uintptr(wsExTopmost|wsExToolWindow|wsExNoActivate),
			uintptr(unsafe.Pointer(clsName)),
			uintptr(unsafe.Pointer(title)),
			wsPopup,
			0, 0, 8, 8,
			0, 0, hInst, 0,
		)
		if hwnd == 0 {
			t.Fatalf("failed to create test window %s", name)
		}
		return hwnd
	}

	mockTray := createTestTopmost("MockTray")
	defer procDestroyWindow.Call(mockTray)
	mockOverlay := createTestTopmost("MockOverlay")
	defer procDestroyWindow.Call(mockOverlay)
	mockMenu := createTestTopmost("MockMenu")
	defer procDestroyWindow.Call(mockMenu)

	// Stack bottom-to-top: [mockMenu, mockOverlay, mockTray]
	procSetWindowPos.Call(mockTray, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	procSetWindowPos.Call(mockOverlay, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	procSetWindowPos.Call(mockMenu, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)

	if !isWindowAbove(mockOverlay, mockTray) || !isWindowAbove(mockMenu, mockOverlay) {
		t.Fatalf("initial Z-order setup failed: overlayAboveTray=%v menuAboveOverlay=%v",
			isWindowAbove(mockOverlay, mockTray), isWindowAbove(mockMenu, mockOverlay))
	}

	// Case 1: Already above tray -> must not jump above mockMenu.
	ensureAboveTaskbar(mockOverlay, mockTray, true)
	if !isWindowAbove(mockOverlay, mockTray) || !isWindowAbove(mockMenu, mockOverlay) {
		t.Fatalf("ensureAboveTaskbar disturbed menu above overlay: overlayAboveTray=%v menuAboveOverlay=%v",
			isWindowAbove(mockOverlay, mockTray), isWindowAbove(mockMenu, mockOverlay))
	}

	// Case 2: Tray raised immediately below mockMenu (above mockOverlay) -> overlay recovers above tray while staying below mockMenu.
	procSetWindowPos.Call(mockTray, mockMenu, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	if isWindowAbove(mockOverlay, mockTray) {
		t.Fatal("expected mockTray to be temporarily above mockOverlay before recovery")
	}
	ensureAboveTaskbar(mockOverlay, mockTray, true)
	if !isWindowAbove(mockOverlay, mockTray) {
		t.Fatal("expected mockOverlay restored above mockTray")
	}
	if !isWindowAbove(mockMenu, mockOverlay) {
		t.Fatal("expected mockMenu to remain strictly above mockOverlay after recovery")
	}

	// Case 3: Tray raised to very top -> overlay still recovers above tray.
	procSetWindowPos.Call(mockTray, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	ensureAboveTaskbar(mockOverlay, mockTray, true)
	if !isWindowAbove(mockOverlay, mockTray) {
		t.Fatal("expected mockOverlay recovered above topmost mockTray")
	}
}
