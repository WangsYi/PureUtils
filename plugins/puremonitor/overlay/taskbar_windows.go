//go:build windows

package overlay

import (
	"image"
	"math"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/puremonitor/collector"
	"pureutils/plugins/puremonitor/fps"
)

const (
	tbTimerRefresh     = 1
	tbTimerZOrderRetry = 2

	tbMenuDashboard = 101
	tbMenuFloat     = 102
	tbMenuSettings  = 103
	tbMenuHide      = 104
)

type taskbarOverlay struct {
	mu           sync.Mutex
	cfg          config.PureMonitorConfig
	snap         collector.SystemSnapshot
	onDashboard  func()
	onSettings   func()
	onToggleFlt  func()
	onHide       func()

	hwnd           uintptr
	threadID       uint32
	dib            dibSurface
	embedded       bool
	visible        bool
	menuOpen       bool
	zRetryLeft     int
	shellPID       uint32
	fgHook         uintptr
	menuHook       uintptr
	shellCapHook   uintptr
	shellReordHook uintptr
	lastTrayRect   winRect
	readyCh        chan struct{}
	stopOnce       sync.Once
}

var shellTrayClassPtr, _ = syscall.UTF16PtrFromString("Shell_TrayWnd")

func newTaskbarOverlay(cfg config.PureMonitorConfig, onDashboard, onSettings, onToggleFlt, onHide func()) *taskbarOverlay {
	t := &taskbarOverlay{
		cfg:         cfg,
		onDashboard: onDashboard,
		onSettings:  onSettings,
		onToggleFlt: onToggleFlt,
		onHide:      onHide,
		readyCh:     make(chan struct{}),
	}
	go t.runThread()
	<-t.readyCh
	return t
}

func (t *taskbarOverlay) Update(snap collector.SystemSnapshot, cfg config.PureMonitorConfig) {
	t.mu.Lock()
	t.snap = snap
	t.cfg = cfg
	hwnd := t.hwnd
	t.mu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmAppRender, 0, 0)
	}
}

func (t *taskbarOverlay) UpdateConfig(cfg config.PureMonitorConfig) {
	t.mu.Lock()
	t.cfg = cfg
	hwnd := t.hwnd
	t.mu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmAppConfig, 0, 0)
	}
}

func (t *taskbarOverlay) Stop() {
	t.stopOnce.Do(func() {
		t.mu.Lock()
		hwnd := t.hwnd
		tid := t.threadID
		t.mu.Unlock()
		if hwnd != 0 {
			procPostMessageW.Call(hwnd, wmAppClose, 0, 0)
		} else if tid != 0 {
			procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0)
		}
	})
}

func (t *taskbarOverlay) runThread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := procGetCurrentThreadID.Call()
	t.mu.Lock()
	t.threadID = uint32(tid)
	t.mu.Unlock()

	if err := ensureOverlayWindowClass(); err != nil {
		close(t.readyCh)
		return
	}

	hInst, _, _ := procGetModuleHandleW.Call(0)
	clsName, _ := syscall.UTF16PtrFromString("PureUtilsMonitorOverlay")
	title, _ := syscall.UTF16PtrFromString("PureMonitor Taskbar")

	exStyle := uintptr(wsExTopmost | wsExToolWindow | wsExLayered | wsExNoActivate)
	style := uintptr(wsPopup)
	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(title)),
		style,
		0, 0, 120, 36,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		close(t.readyCh)
		return
	}

	overlayWndMap.Store(hwnd, t.wndProc)
	winEventThreadMap.Store(uint32(tid), t.onWinEvent)
	t.installGlobalWinEventHooks()
	t.mu.Lock()
	t.hwnd = hwnd
	t.mu.Unlock()
	close(t.readyCh)

	// Periodic position/z-order guard every 1500ms so taskbar overlay never drops behind Shell_TrayWnd.
	procSetTimer.Call(hwnd, tbTimerRefresh, 1500, 0)
	t.renderAndPosition()

	var msg winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	t.uninstallWinEventHooks()
	winEventThreadMap.Delete(uint32(tid))
	overlayWndMap.Delete(hwnd)
	t.dib.free()
}

func (t *taskbarOverlay) installGlobalWinEventHooks() {
	hFg, _, _ := procSetWinEventHook.Call(
		eventSystemForeground, eventSystemForeground,
		0, winEventHookCB, 0, 0,
		wineventOutOfContext|wineventSkipOwnProcess,
	)
	t.fgHook = hFg
	hMenu, _, _ := procSetWinEventHook.Call(
		eventSystemMenuEnd, eventSystemMenuEnd,
		0, winEventHookCB, 0, 0,
		wineventOutOfContext|wineventSkipOwnProcess,
	)
	t.menuHook = hMenu
}

func (t *taskbarOverlay) syncShellHooks(shellTray uintptr) {
	if shellTray == 0 {
		return
	}
	var pid uint32
	procGetWindowThreadPID.Call(shellTray, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 || pid == t.shellPID {
		return
	}
	t.unhookShellHooks()
	t.shellPID = pid
	hCap, _, _ := procSetWinEventHook.Call(
		eventSystemCaptureStart, eventSystemCaptureEnd,
		0, winEventHookCB, uintptr(pid), 0,
		wineventOutOfContext,
	)
	t.shellCapHook = hCap
	hReord, _, _ := procSetWinEventHook.Call(
		eventObjectReorder, eventObjectReorder,
		0, winEventHookCB, uintptr(pid), 0,
		wineventOutOfContext,
	)
	t.shellReordHook = hReord
}

func (t *taskbarOverlay) unhookShellHooks() {
	if t.shellCapHook != 0 {
		procUnhookWinEvent.Call(t.shellCapHook)
		t.shellCapHook = 0
	}
	if t.shellReordHook != 0 {
		procUnhookWinEvent.Call(t.shellReordHook)
		t.shellReordHook = 0
	}
	t.shellPID = 0
}

func (t *taskbarOverlay) uninstallWinEventHooks() {
	t.unhookShellHooks()
	if t.fgHook != 0 {
		procUnhookWinEvent.Call(t.fgHook)
		t.fgHook = 0
	}
	if t.menuHook != 0 {
		procUnhookWinEvent.Call(t.menuHook)
		t.menuHook = 0
	}
}

func (t *taskbarOverlay) onWinEvent(event, evHwnd uintptr) {
	if !t.visible || t.embedded || t.menuOpen || t.hwnd == 0 {
		return
	}
	t.ensureZOrderFast()
	t.zRetryLeft = 3
	procSetTimer.Call(t.hwnd, tbTimerZOrderRetry, 40, 0)
}

func (t *taskbarOverlay) wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmAppRender, wmAppConfig:
		t.renderAndPosition()
		return 0
	case wmTimer:
		switch wParam {
		case tbTimerRefresh:
			t.guardPositionAndVisibility()
		case tbTimerZOrderRetry:
			t.ensureZOrderFast()
			t.zRetryLeft--
			if t.zRetryLeft <= 0 {
				t.zRetryLeft = 0
				procKillTimer.Call(hwnd, tbTimerZOrderRetry)
			}
		}
		return 0
	case wmDisplayChg, wmSettingChg, wmDpiChanged:
		t.renderAndPosition()
		return 0
	case wmTaskbarCreated:
		t.mu.Lock()
		t.embedded = false
		t.mu.Unlock()
		t.renderAndPosition()
		return 0
	case wmLButtonUp:
		if t.onDashboard != nil {
			go t.onDashboard()
		}
		return 0
	case wmRButtonUp:
		t.showMenu(hwnd)
		return 0
	case wmAppClose:
		procKillTimer.Call(hwnd, tbTimerRefresh)
		procKillTimer.Call(hwnd, tbTimerZOrderRetry)
		t.uninstallWinEventHooks()
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostThreadMessageW.Call(uintptr(t.threadID), wmQuit, 0, 0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func (t *taskbarOverlay) showMenu(hwnd uintptr) {
	t.mu.Lock()
	fltOn := t.cfg.ShowFloating
	t.mu.Unlock()

	t.menuOpen = true
	cmd := showPopupMenu(hwnd, []popupMenuEntry{
		{ID: tbMenuDashboard, Label: i18n.S("打开硬件监控面板", "Open Hardware Dashboard")},
		{ID: tbMenuFloat, Label: i18n.S("显示悬浮监控窗", "Show Floating Monitor"), Checked: fltOn},
		{ID: tbMenuSettings, Label: i18n.S("监控设置…", "Monitor Settings…")},
		{Separator: true},
		{ID: tbMenuHide, Label: i18n.S("隐藏任务栏监控", "Hide Taskbar Monitor")},
	})
	t.menuOpen = false
	t.ensureZOrderFast()
	switch cmd {
	case tbMenuDashboard:
		if t.onDashboard != nil {
			go t.onDashboard()
		}
	case tbMenuFloat:
		if t.onToggleFlt != nil {
			go t.onToggleFlt()
		}
	case tbMenuSettings:
		if t.onSettings != nil {
			go t.onSettings()
		}
	case tbMenuHide:
		if t.onHide != nil {
			go t.onHide()
		}
	}
}

func (t *taskbarOverlay) renderAndPosition() {
	t.mu.Lock()
	hwnd := t.hwnd
	cfg := t.cfg
	snap := t.snap
	t.mu.Unlock()
	if hwnd == 0 {
		return
	}

	if !cfg.Enabled || !cfg.ShowTaskbar || (cfg.TaskbarHideOnFullscreen && fps.IsAnyFullscreenWindow()) {
		if t.visible {
			procShowWindow.Call(hwnd, swHide)
			t.visible = false
		}
		return
	}

	shellTray, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(shellTrayClassPtr)), 0)
	if shellTray == 0 {
		if t.visible {
			procShowWindow.Call(hwnd, swHide)
			t.visible = false
		}
		return
	}

	var trayRect winRect
	procGetWindowRect.Call(shellTray, uintptr(unsafe.Pointer(&trayRect)))
	if trayRect.Width() <= 0 || trayRect.Height() <= 0 {
		return
	}
	t.lastTrayRect = trayRect

	// Hide when Windows taskbar auto-hide has slid off-screen.
	mon := monitorBoundsAt(trayRect.Left+10, trayRect.Top+10)
	if isTaskbarAutoHide() && trayRect.Top >= mon.Bottom-6 {
		if t.visible {
			procShowWindow.Call(hwnd, swHide)
			t.visible = false
		}
		return
	}

	scale := windowScale(shellTray)
	dark := isSystemDarkTheme(cfg.Theme)
	img := RenderTaskbarImage(snap, cfg, dark, scale)
	if img == nil {
		return
	}

	wantEmbed := strings.EqualFold(cfg.TaskbarMode, "embed") ||
		(strings.EqualFold(cfg.TaskbarMode, "auto") && !isWindows11OrNewer())
	if wantEmbed && isWindows11OrNewer() && strings.EqualFold(cfg.TaskbarMode, "auto") {
		wantEmbed = false
	}

	t.syncShellHooks(shellTray)
	t.syncParentMode(hwnd, shellTray, wantEmbed)

	x, y := computeTaskbarCoords(shellTray, trayRect, img, cfg, scale)
	if wantEmbed {
		cx := x - int(trayRect.Left)
		cy := y - int(trayRect.Top)
		procSetWindowPos.Call(hwnd, 0, uintptr(cx), uintptr(cy), uintptr(img.Bounds().Dx()), uintptr(img.Bounds().Dy()), swpNoZOrder|swpNoActivate)
		t.dib.commitRGBA(hwnd, img, 0, 0, false)
	} else {
		t.dib.commitRGBA(hwnd, img, x, y, true)
		if !t.menuOpen {
			ensureAboveTaskbar(hwnd, shellTray, true)
		}
	}

	if !t.visible {
		procShowWindow.Call(hwnd, swShowNA)
		t.visible = true
		if !wantEmbed && !t.menuOpen {
			ensureAboveTaskbar(hwnd, shellTray, true)
		}
	}
}

func (t *taskbarOverlay) ensureZOrderFast() {
	if !t.visible || t.embedded || t.menuOpen || t.hwnd == 0 {
		return
	}
	shellTray, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(shellTrayClassPtr)), 0)
	if shellTray == 0 {
		return
	}
	t.syncShellHooks(shellTray)
	ensureAboveTaskbar(t.hwnd, shellTray, true)
}

func (t *taskbarOverlay) guardPositionAndVisibility() {
	t.mu.Lock()
	hwnd := t.hwnd
	cfg := t.cfg
	t.mu.Unlock()
	if hwnd == 0 || t.menuOpen {
		return
	}

	if !cfg.Enabled || !cfg.ShowTaskbar || (cfg.TaskbarHideOnFullscreen && fps.IsAnyFullscreenWindow()) {
		if t.visible {
			procShowWindow.Call(hwnd, swHide)
			t.visible = false
		}
		return
	}

	shellTray, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(shellTrayClassPtr)), 0)
	if shellTray == 0 {
		if t.visible {
			procShowWindow.Call(hwnd, swHide)
			t.visible = false
		}
		return
	}

	var trayRect winRect
	procGetWindowRect.Call(shellTray, uintptr(unsafe.Pointer(&trayRect)))
	if trayRect.Width() <= 0 || trayRect.Height() <= 0 {
		return
	}

	mon := monitorBoundsAt(trayRect.Left+10, trayRect.Top+10)
	if isTaskbarAutoHide() && trayRect.Top >= mon.Bottom-6 {
		if t.visible {
			procShowWindow.Call(hwnd, swHide)
			t.visible = false
		}
		return
	}

	t.syncShellHooks(shellTray)

	wantEmbed := strings.EqualFold(cfg.TaskbarMode, "embed") ||
		(strings.EqualFold(cfg.TaskbarMode, "auto") && !isWindows11OrNewer())
	if wantEmbed && isWindows11OrNewer() && strings.EqualFold(cfg.TaskbarMode, "auto") {
		wantEmbed = false
	}

	if !t.visible || wantEmbed != t.embedded || trayRect != t.lastTrayRect {
		t.renderAndPosition()
		return
	}

	if !wantEmbed {
		ensureAboveTaskbar(hwnd, shellTray, true)
	}
}

func (t *taskbarOverlay) syncParentMode(hwnd, shellTray uintptr, wantEmbed bool) {
	if wantEmbed == t.embedded {
		return
	}
	if wantEmbed {
		procSetParent.Call(hwnd, shellTray)
		st, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlStyle)
		st = (st &^ wsPopup) | wsChild | wsClipsiblings
		procSetWindowLongPtrW.Call(hwnd, gwlStyle, st)
		t.embedded = true
	} else {
		procSetParent.Call(hwnd, 0)
		st, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlStyle)
		st = (st &^ (wsChild | wsClipsiblings)) | wsPopup
		procSetWindowLongPtrW.Call(hwnd, gwlStyle, st)
		t.embedded = false
	}
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
}

func computeTaskbarCoords(shellTray uintptr, trayRect winRect, img *image.RGBA, cfg config.PureMonitorConfig, scale float64) (x, y int) {
	var notifyRect winRect
	var hasNotify bool
	if shellTray != 0 && !strings.EqualFold(cfg.TaskbarAlign, "left") {
		clsNotify, _ := syscall.UTF16PtrFromString("TrayNotifyWnd")
		notifyWnd, _, _ := procFindWindowExW.Call(shellTray, 0, uintptr(unsafe.Pointer(clsNotify)), 0)
		if notifyWnd != 0 {
			procGetWindowRect.Call(notifyWnd, uintptr(unsafe.Pointer(&notifyRect)))
			hasNotify = true
		}
	}
	return computeTaskbarCoordsPure(trayRect, notifyRect, hasNotify, img.Bounds().Dx(), img.Bounds().Dy(), cfg, scale)
}

func computeTaskbarCoordsPure(trayRect, notifyRect winRect, hasNotify bool, winW, winH int, cfg config.PureMonitorConfig, scale float64) (x, y int) {
	if scale <= 0 {
		scale = 1
	}
	trayW := trayRect.Width()
	trayH := trayRect.Height()
	offsetPx := int(math.Round(float64(cfg.TaskbarOffsetX) * scale))

	y = int(trayRect.Top) + (trayH-winH)/2
	if strings.EqualFold(cfg.TaskbarAlign, "left") {
		x = int(trayRect.Left) + int(math.Round(16*scale)) + offsetPx
	} else {
		// Guard against the Win11 24H2 bug where TrayNotifyWnd reports Left=0 or degenerate width.
		validNotify := hasNotify &&
			notifyRect.Width() >= 40 &&
			notifyRect.Right <= trayRect.Right+4 &&
			notifyRect.Left > trayRect.Left+int32(trayW/2)
		if validNotify {
			x = int(notifyRect.Left) - winW - int(math.Round(8*scale)) + offsetPx
		} else {
			x = int(trayRect.Right) - int(math.Round(240*scale)) - winW + offsetPx
		}
	}

	if x < int(trayRect.Left)+4 {
		x = int(trayRect.Left) + 4
	}
	if x+winW > int(trayRect.Right)-4 {
		x = int(trayRect.Right) - winW - 4
	}
	return x, y
}
