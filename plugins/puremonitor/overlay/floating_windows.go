//go:build windows

package overlay

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/puremonitor/collector"
	"pureutils/plugins/puremonitor/fps"
)

const (
	fltTimerCollapse = 10
	fltTimerAltPoll  = 11

	fltMenuDashboard  = 201
	fltMenuCollapse   = 202
	fltMenuLock       = 203
	fltMenuThrough    = 204
	fltMenuSettings   = 205
	fltMenuHide       = 206
)

type floatingOverlay struct {
	mu           sync.Mutex
	cfg          config.PureMonitorConfig
	snap         collector.SystemSnapshot
	onDashboard  func()
	onSettings   func()
	onHide       func()
	onStateSave  func(x, y int, collapsed, locked, through bool)

	hwnd        uintptr
	threadID    uint32
	dib         dibSurface
	visible     bool
	x, y        int
	w, h        int
	dockEdge    string // "" | "left" | "right" | "top"
	collapsed   bool
	hovered     bool
	dragging    bool
	dragOffsetX int32
	dragOffsetY int32
	tracking    bool
	altTempOpen bool
	menuOpen    bool

	readyCh  chan struct{}
	stopOnce sync.Once
}

func newFloatingOverlay(
	cfg config.PureMonitorConfig,
	onDashboard, onSettings, onHide func(),
	onStateSave func(x, y int, collapsed, locked, through bool),
) *floatingOverlay {
	f := &floatingOverlay{
		cfg:         cfg,
		x:           cfg.FloatX,
		y:           cfg.FloatY,
		collapsed:   cfg.FloatCollapsed,
		onDashboard: onDashboard,
		onSettings:  onSettings,
		onHide:      onHide,
		onStateSave: onStateSave,
		readyCh:     make(chan struct{}),
	}
	go f.runThread()
	<-f.readyCh
	return f
}

func (f *floatingOverlay) Update(snap collector.SystemSnapshot, cfg config.PureMonitorConfig) {
	f.mu.Lock()
	f.snap = snap
	prevCfg := f.cfg
	f.cfg = cfg
	if cfg.FloatX >= 0 && cfg.FloatY >= 0 && (cfg.FloatX != prevCfg.FloatX || cfg.FloatY != prevCfg.FloatY) && !f.dragging {
		f.x = cfg.FloatX
		f.y = cfg.FloatY
	}
	if cfg.FloatCollapsed != prevCfg.FloatCollapsed {
		f.collapsed = cfg.FloatCollapsed
	}
	hwnd := f.hwnd
	f.mu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmAppRender, 0, 0)
	}
}

func (f *floatingOverlay) UpdateConfig(cfg config.PureMonitorConfig) {
	f.mu.Lock()
	prevCfg := f.cfg
	f.cfg = cfg
	if cfg.FloatX >= 0 && cfg.FloatY >= 0 && (cfg.FloatX != prevCfg.FloatX || cfg.FloatY != prevCfg.FloatY) && !f.dragging {
		f.x = cfg.FloatX
		f.y = cfg.FloatY
	}
	if cfg.FloatCollapsed != prevCfg.FloatCollapsed {
		f.collapsed = cfg.FloatCollapsed
	}
	hwnd := f.hwnd
	f.mu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmAppConfig, 0, 0)
	}
}

func (f *floatingOverlay) Stop() {
	f.stopOnce.Do(func() {
		f.mu.Lock()
		hwnd := f.hwnd
		tid := f.threadID
		f.mu.Unlock()
		if hwnd != 0 {
			procPostMessageW.Call(hwnd, wmAppClose, 0, 0)
		} else if tid != 0 {
			procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0)
		}
	})
}

func (f *floatingOverlay) runThread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tid, _, _ := procGetCurrentThreadID.Call()
	f.mu.Lock()
	f.threadID = uint32(tid)
	f.mu.Unlock()

	if err := ensureOverlayWindowClass(); err != nil {
		close(f.readyCh)
		return
	}

	hInst, _, _ := procGetModuleHandleW.Call(0)
	clsName, _ := syscall.UTF16PtrFromString("PureUtilsMonitorOverlay")
	title, _ := syscall.UTF16PtrFromString("PureMonitor Floating")

	exStyle := uintptr(wsExTopmost | wsExToolWindow | wsExLayered | wsExNoActivate)
	if f.cfg.FloatClickThrough {
		exStyle |= wsExTransparent
	}
	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		100, 100, 186, 150,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		close(f.readyCh)
		return
	}

	overlayWndMap.Store(hwnd, f.wndProc)
	f.mu.Lock()
	f.hwnd = hwnd
	f.mu.Unlock()
	close(f.readyCh)

	procSetTimer.Call(hwnd, fltTimerAltPoll, 150, 0)
	f.render()

	var msg winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	overlayWndMap.Delete(hwnd)
	f.dib.free()
}

func (f *floatingOverlay) wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmAppRender, wmAppConfig:
		f.render()
		return 0
	case wmDisplayChg, wmSettingChg, wmDpiChanged:
		f.render()
		return 0
	case wmTimer:
		switch wParam {
		case fltTimerCollapse:
			procKillTimer.Call(hwnd, fltTimerCollapse)
			f.mu.Lock()
			canCollapse := f.cfg.FloatAutoDock && f.dockEdge != "" && !f.dragging && !f.hovered
			if canCollapse {
				f.collapsed = true
			}
			f.mu.Unlock()
			if canCollapse {
				f.render()
			}
		case fltTimerAltPoll:
			f.syncClickThroughAlt(hwnd)
		}
		return 0
	case wmMouseMove:
		if !f.tracking {
			tme := trackMouseEventStruct{
				CbSize:    uint32(unsafe.Sizeof(trackMouseEventStruct{})),
				DwFlags:   tmeLeave,
				HwndTrack: hwnd,
			}
			procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			f.tracking = true
		}
		var pt winPoint
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

		f.mu.Lock()
		f.hovered = true
		needRender := false
		if f.collapsed && f.cfg.FloatAutoDock {
			f.collapsed = false
			needRender = true
		}
		if f.dragging && !f.cfg.FloatLocked {
			f.x = int(pt.X - f.dragOffsetX)
			f.y = int(pt.Y - f.dragOffsetY)
			f.clampToMonitorLocked()
			needRender = true
		}
		f.mu.Unlock()

		procKillTimer.Call(hwnd, fltTimerCollapse)
		if needRender {
			f.render()
		}
		return 0
	case wmMouseLeave:
		f.tracking = false
		f.mu.Lock()
		f.hovered = false
		shouldTimer := f.cfg.FloatAutoDock && f.dockEdge != "" && !f.dragging
		f.mu.Unlock()
		if shouldTimer {
			procSetTimer.Call(hwnd, fltTimerCollapse, 750, 0)
		}
		return 0
	case wmLButtonDown:
		f.mu.Lock()
		locked := f.cfg.FloatLocked
		if !locked {
			var pt winPoint
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			f.dragging = true
			f.dragOffsetX = pt.X - int32(f.x)
			f.dragOffsetY = pt.Y - int32(f.y)
			procSetCapture.Call(hwnd)
		}
		f.mu.Unlock()
		return 0
	case wmLButtonUp:
		var moved bool
		var nx, ny int
		var col, lck, thr bool
		f.mu.Lock()
		if f.dragging {
			f.dragging = false
			procReleaseCapture.Call()
			f.snapToEdgeLocked()
			nx, ny = f.x, f.y
			col, lck, thr = f.collapsed, f.cfg.FloatLocked, f.cfg.FloatClickThrough
			moved = true
		}
		f.mu.Unlock()
		if moved {
			f.render()
			if f.onStateSave != nil {
				go f.onStateSave(nx, ny, col, lck, thr)
			}
		}
		return 0
	case wmLButtonDbl:
		if f.onDashboard != nil {
			go f.onDashboard()
		}
		return 0
	case wmRButtonUp:
		f.showMenu(hwnd)
		return 0
	case wmAppClose:
		procKillTimer.Call(hwnd, fltTimerCollapse)
		procKillTimer.Call(hwnd, fltTimerAltPoll)
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostThreadMessageW.Call(uintptr(f.threadID), wmQuit, 0, 0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func (f *floatingOverlay) syncClickThroughAlt(hwnd uintptr) {
	f.mu.Lock()
	wantThrough := f.cfg.FloatClickThrough
	f.mu.Unlock()
	if !wantThrough {
		if f.altTempOpen {
			f.altTempOpen = false
			f.setTransparentStyle(hwnd, false)
		}
		return
	}
	const vkMenu = 0x12 // Alt
	st, _, _ := procGetAsyncKeyState.Call(vkMenu)
	altDown := (uint16(st) & 0x8000) != 0
	if altDown {
		if !f.altTempOpen {
			f.altTempOpen = true
			f.setTransparentStyle(hwnd, false)
		}
	} else {
		f.altTempOpen = false
		f.setTransparentStyle(hwnd, true)
	}
}

func (f *floatingOverlay) setTransparentStyle(hwnd uintptr, through bool) {
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExStyle)
	has := (ex & wsExTransparent) != 0
	if has == through {
		return
	}
	if through {
		ex |= wsExTransparent
	} else {
		ex &^= wsExTransparent
	}
	procSetWindowLongPtrW.Call(hwnd, gwlExStyle, ex)
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
}

func (f *floatingOverlay) showMenu(hwnd uintptr) {
	f.mu.Lock()
	col := f.collapsed
	lck := f.cfg.FloatLocked
	thr := f.cfg.FloatClickThrough
	x, y := f.x, f.y
	f.mu.Unlock()

	collapseLbl := i18n.S("收起为边缘胶囊", "Collapse to Edge Pill")
	if col {
		collapseLbl = i18n.S("展开悬浮窗", "Expand Floating Monitor")
	}

	f.menuOpen = true
	cmd := showPopupMenu(hwnd, []popupMenuEntry{
		{ID: fltMenuDashboard, Label: i18n.S("打开硬件监控面板", "Open Hardware Dashboard")},
		{ID: fltMenuCollapse, Label: collapseLbl},
		{ID: fltMenuLock, Label: i18n.S("锁定位置", "Lock Position"), Checked: lck},
		{ID: fltMenuThrough, Label: i18n.S("鼠标穿透（按住 Alt 可拖动）", "Click-Through (Hold Alt to Drag)"), Checked: thr},
		{ID: fltMenuSettings, Label: i18n.S("监控设置…", "Monitor Settings…")},
		{Separator: true},
		{ID: fltMenuHide, Label: i18n.S("隐藏悬浮监控窗", "Hide Floating Monitor")},
	})
	f.menuOpen = false
	switch cmd {
	case fltMenuDashboard:
		if f.onDashboard != nil {
			go f.onDashboard()
		}
	case fltMenuCollapse:
		f.mu.Lock()
		f.collapsed = !f.collapsed
		newCol := f.collapsed
		f.mu.Unlock()
		f.render()
		if f.onStateSave != nil {
			go f.onStateSave(x, y, newCol, lck, thr)
		}
	case fltMenuLock:
		f.mu.Lock()
		f.cfg.FloatLocked = !f.cfg.FloatLocked
		newLck := f.cfg.FloatLocked
		f.mu.Unlock()
		f.render()
		if f.onStateSave != nil {
			go f.onStateSave(x, y, col, newLck, thr)
		}
	case fltMenuThrough:
		f.mu.Lock()
		f.cfg.FloatClickThrough = !f.cfg.FloatClickThrough
		newThr := f.cfg.FloatClickThrough
		f.mu.Unlock()
		f.setTransparentStyle(hwnd, newThr)
		f.render()
		if f.onStateSave != nil {
			go f.onStateSave(x, y, col, lck, newThr)
		}
	case fltMenuSettings:
		if f.onSettings != nil {
			go f.onSettings()
		}
	case fltMenuHide:
		if f.onHide != nil {
			go f.onHide()
		}
	}
}

func clampRectToWorkArea(x, y, w, h int, wa winRect) (int, int) {
	if x < int(wa.Left) {
		x = int(wa.Left)
	}
	if y < int(wa.Top) {
		y = int(wa.Top)
	}
	if w > 0 && x+w > int(wa.Right) {
		x = int(wa.Right) - w
	}
	if h > 0 && y+h > int(wa.Bottom) {
		y = int(wa.Bottom) - h
	}
	return x, y
}

func snapRectToEdge(x, y, w, h int, wa winRect, autoDock bool) (nx, ny int, edge string) {
	if !autoDock {
		return x, y, ""
	}
	const threshold = 18
	switch {
	case x-int(wa.Left) <= threshold:
		return int(wa.Left), y, "left"
	case int(wa.Right)-(x+w) <= threshold:
		return int(wa.Right) - w, y, "right"
	case y-int(wa.Top) <= threshold:
		return x, int(wa.Top), "top"
	default:
		return x, y, ""
	}
}

func (f *floatingOverlay) clampToMonitorLocked() {
	wa := monitorWorkRectAt(int32(f.x+f.w/2), int32(f.y+f.h/2))
	f.x, f.y = clampRectToWorkArea(f.x, f.y, f.w, f.h, wa)
}

func (f *floatingOverlay) snapToEdgeLocked() {
	if !f.cfg.FloatAutoDock {
		f.dockEdge = ""
		return
	}
	wa := monitorWorkRectAt(int32(f.x+f.w/2), int32(f.y+f.h/2))
	f.x, f.y, f.dockEdge = snapRectToEdge(f.x, f.y, f.w, f.h, wa, f.cfg.FloatAutoDock)
}

func (f *floatingOverlay) render() {
	f.mu.Lock()
	hwnd := f.hwnd
	cfg := f.cfg
	snap := f.snap
	collapsed := f.collapsed
	edge := f.dockEdge
	prevW := f.w
	if f.x < 0 && edge == "" {
		edge = "right"
	}
	f.mu.Unlock()
	if hwnd == 0 {
		return
	}

	if !cfg.Enabled || !cfg.ShowFloating || (cfg.FloatHideOnFullscreen && fps.IsAnyFullscreenWindow()) {
		if f.visible {
			procShowWindow.Call(hwnd, swHide)
			f.visible = false
		}
		return
	}

	f.setTransparentStyle(hwnd, cfg.FloatClickThrough && !f.altTempOpen)

	scale := windowScale(hwnd)
	dark := isSystemDarkTheme(cfg.Theme)
	img := RenderFloatingImageAt(snap, cfg, dark, scale, collapsed, edge)
	if img == nil {
		return
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	f.mu.Lock()
	f.w, f.h = w, h
	if f.x < 0 || f.y < 0 {
		wa := monitorWorkRectAt(0, 0)
		f.dockEdge = "right"
		if collapsed {
			f.x = int(wa.Right) - w
		} else {
			f.x = int(wa.Right) - w - 14
		}
		f.y = int(wa.Top) + (wa.Height()-h)/2
	} else {
		wa := monitorWorkRectAt(int32(f.x+prevW/2), int32(f.y+h/2))
		switch f.dockEdge {
		case "right":
			f.x = int(wa.Right) - w
			f.clampToMonitorLocked()
		case "left":
			f.x = int(wa.Left)
			f.clampToMonitorLocked()
		case "top":
			if prevW > 0 && prevW != w {
				f.x = f.x + (prevW-w)/2
			}
			f.y = int(wa.Top)
			f.clampToMonitorLocked()
		default:
			f.clampToMonitorLocked()
		}
	}
	px, py := f.x, f.y
	f.mu.Unlock()

	f.dib.commitRGBA(hwnd, img, px, py, true)
	if !f.visible {
		if !f.menuOpen {
			procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		}
		procShowWindow.Call(hwnd, swShowNA)
		f.visible = true
	}
}
