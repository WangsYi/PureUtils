//go:build windows

package overlay

import (
	"image"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modGdi32    = windows.NewLazySystemDLL("gdi32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")

	procRegisterClassExW     = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW      = modUser32.NewProc("CreateWindowExW")
	procDestroyWindow        = modUser32.NewProc("DestroyWindow")
	procDefWindowProcW       = modUser32.NewProc("DefWindowProcW")
	procShowWindow           = modUser32.NewProc("ShowWindow")
	procSetWindowPos         = modUser32.NewProc("SetWindowPos")
	procSetParent            = modUser32.NewProc("SetParent")
	procGetWindowRect        = modUser32.NewProc("GetWindowRect")
	procGetClientRect        = modUser32.NewProc("GetClientRect")
	procFindWindowW          = modUser32.NewProc("FindWindowW")
	procFindWindowExW        = modUser32.NewProc("FindWindowExW")
	procGetDpiForWindow      = modUser32.NewProc("GetDpiForWindow")
	procGetDpiForSystem      = modUser32.NewProc("GetDpiForSystem")
	procLoadCursorW          = modUser32.NewProc("LoadCursorW")
	procGetMessageW          = modUser32.NewProc("GetMessageW")
	procTranslateMessage     = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW     = modUser32.NewProc("DispatchMessageW")
	procPostMessageW         = modUser32.NewProc("PostMessageW")
	procPostThreadMessageW   = modUser32.NewProc("PostThreadMessageW")
	procRegisterWindowMsgW   = modUser32.NewProc("RegisterWindowMessageW")
	procUpdateLayeredWindow  = modUser32.NewProc("UpdateLayeredWindow")
	procSetCapture           = modUser32.NewProc("SetCapture")
	procReleaseCapture       = modUser32.NewProc("ReleaseCapture")
	procGetCursorPos         = modUser32.NewProc("GetCursorPos")
	procGetAsyncKeyState     = modUser32.NewProc("GetAsyncKeyState")
	procTrackMouseEvent      = modUser32.NewProc("TrackMouseEvent")
	procSetTimer             = modUser32.NewProc("SetTimer")
	procKillTimer            = modUser32.NewProc("KillTimer")
	procCreatePopupMenu      = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW          = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu       = modUser32.NewProc("TrackPopupMenu")
	procDestroyMenu          = modUser32.NewProc("DestroyMenu")
	procSetForegroundWindow  = modUser32.NewProc("SetForegroundWindow")
	procGetWindowLongPtrW    = modUser32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW    = modUser32.NewProc("SetWindowLongPtrW")
	procGetWindow            = modUser32.NewProc("GetWindow")
	procIsWindowVisible      = modUser32.NewProc("IsWindowVisible")
	procGetWindowThreadPID   = modUser32.NewProc("GetWindowThreadProcessId")
	procSetWinEventHook      = modUser32.NewProc("SetWinEventHook")
	procUnhookWinEvent       = modUser32.NewProc("UnhookWinEvent")
	procMonitorFromPoint     = modUser32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW      = modUser32.NewProc("GetMonitorInfoW")
	procGetDC                = modUser32.NewProc("GetDC")
	procReleaseDC            = modUser32.NewProc("ReleaseDC")

	procCreateCompatibleDC = modGdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = modGdi32.NewProc("CreateDIBSection")
	procSelectObject       = modGdi32.NewProc("SelectObject")
	procDeleteDC           = modGdi32.NewProc("DeleteDC")
	procDeleteObject       = modGdi32.NewProc("DeleteObject")

	procGetModuleHandleW     = modKernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID   = modKernel32.NewProc("GetCurrentThreadId")
	procSHAppBarMessage      = modShell32.NewProc("SHAppBarMessage")
)

const (
	wsPopup           = 0x80000000
	wsChild           = 0x40000000
	wsClipsiblings    = 0x04000000
	csDblClks         = 0x0008
	wsExTopmost       = 0x00000008
	wsExTransparent   = 0x00000020
	wsExToolWindow    = 0x00000080
	wsExLayered       = 0x00080000
	wsExNoActivate    = 0x08000000

	gwlStyle   = ^uintptr(15) // -16
	gwlExStyle = ^uintptr(19) // -20

	swHide   = 0
	swShowNA = 8

	gwHwndNext = 2
	gwHwndPrev = 3

	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
	hwndTopmost     = ^uintptr(0)

	wmNull        = 0x0000
	wmDestroy     = 0x0002
	wmQuit        = 0x0012
	wmTimer       = 0x0113
	wmDisplayChg  = 0x007E
	wmSettingChg  = 0x001A
	wmDpiChanged  = 0x02E0
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmLButtonDbl  = 0x0203
	wmRButtonUp   = 0x0205
	wmMouseLeave  = 0x02A3
	wmApp         = 0x8000
	wmAppRender   = wmApp + 1
	wmAppConfig   = wmApp + 2
	wmAppClose    = wmApp + 3

	ulwAlpha   = 0x00000002
	acSrcOver  = 0x00
	acSrcAlpha = 0x01
	tmeLeave   = 0x00000002
	idcArrow   = 32512

	mfString    = 0x0000
	mfSeparator = 0x0800
	mfChecked   = 0x0008
	tpmRightBtn = 0x0002
	tpmReturnCmd = 0x0100
	tpmNonotify  = 0x0080

	abmGetState = 0x00000004
	absAutohide = 0x00000001

	eventSystemForeground   = 0x0003
	eventSystemMenuEnd      = 0x0005
	eventSystemCaptureStart = 0x0008
	eventSystemCaptureEnd   = 0x0009
	eventObjectReorder      = 0x8004
	wineventOutOfContext    = 0x0000
	wineventSkipOwnProcess  = 0x0002
)

type winPoint struct {
	X, Y int32
}

type winSize struct {
	CX, CY int32
}

type winRect struct {
	Left, Top, Right, Bottom int32
}

func (r winRect) Width() int  { return int(r.Right - r.Left) }
func (r winRect) Height() int { return int(r.Bottom - r.Top) }

type winMonitorInfo struct {
	CbSize    uint32
	RcMonitor winRect
	RcWork    winRect
	DwFlags   uint32
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type winMsg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       winPoint
	LPrivate uint32
}

type trackMouseEventStruct struct {
	CbSize      uint32
	DwFlags     uint32
	HwndTrack   uintptr
	DwHoverTime uint32
}

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type appBarData struct {
	CbSize           uint32
	Hwnd             uintptr
	UCallbackMessage uint32
	UEdge            uint32
	Rc               winRect
	LParam           uintptr
}

// dibSurface caches a reusable 32-bit top-down BGRA DIBSection for crisp UpdateLayeredWindow updates.
type dibSurface struct {
	memDC  uintptr
	hbmp   uintptr
	oldObj uintptr
	bits   unsafe.Pointer
	w, h   int
}

func (d *dibSurface) ensure(w, h int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	if d.memDC != 0 && d.w == w && d.h == h && d.bits != nil {
		return true
	}
	d.free()
	screenDC, _, _ := procGetDC.Call(0)
	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if screenDC != 0 {
		procReleaseDC.Call(0, screenDC)
	}
	if memDC == 0 {
		return false
	}
	bi := bitmapInfo{
		Header: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(w),
			BiHeight:      -int32(h), // top-down DIB
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: 0, // BI_RGB
		},
	}
	var bits unsafe.Pointer
	hbmp, _, _ := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmp == 0 || bits == nil {
		procDeleteDC.Call(memDC)
		return false
	}
	old, _, _ := procSelectObject.Call(memDC, hbmp)
	d.memDC = memDC
	d.hbmp = hbmp
	d.oldObj = old
	d.bits = bits
	d.w = w
	d.h = h
	return true
}

func (d *dibSurface) free() {
	if d.memDC != 0 {
		if d.oldObj != 0 {
			procSelectObject.Call(d.memDC, d.oldObj)
		}
		if d.hbmp != 0 {
			procDeleteObject.Call(d.hbmp)
		}
		procDeleteDC.Call(d.memDC)
	}
	d.memDC = 0
	d.hbmp = 0
	d.oldObj = 0
	d.bits = nil
	d.w = 0
	d.h = 0
}

func (d *dibSurface) commitRGBA(hwnd uintptr, img *image.RGBA, x, y int, movePos bool) bool {
	if hwnd == 0 || img == nil {
		return false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if !d.ensure(w, h) {
		return false
	}
	dst := unsafe.Slice((*byte)(d.bits), w*h*4)
	src := img.Pix
	stride := img.Stride
	// image.RGBA is already premultiplied [R, G, B, A]; convert to BGRA [B, G, R, A].
	for row := 0; row < h; row++ {
		sRow := src[row*stride : row*stride+w*4]
		dRow := dst[row*w*4 : (row+1)*w*4]
		for col := 0; col < w*4; col += 4 {
			dRow[col] = sRow[col+2]   // B
			dRow[col+1] = sRow[col+1] // G
			dRow[col+2] = sRow[col]   // R
			dRow[col+3] = sRow[col+3] // A
		}
	}

	sz := winSize{CX: int32(w), CY: int32(h)}
	srcPt := winPoint{X: 0, Y: 0}
	bf := blendFunction{
		BlendOp:             acSrcOver,
		SourceConstantAlpha: 255,
		AlphaFormat:         acSrcAlpha,
	}
	var dstPtPtr uintptr
	if movePos {
		pt := winPoint{X: int32(x), Y: int32(y)}
		dstPtPtr = uintptr(unsafe.Pointer(&pt))
		defer runtime.KeepAlive(pt)
	}
	r, _, _ := procUpdateLayeredWindow.Call(
		hwnd,
		0,
		dstPtPtr,
		uintptr(unsafe.Pointer(&sz)),
		d.memDC,
		uintptr(unsafe.Pointer(&srcPt)),
		0,
		uintptr(unsafe.Pointer(&bf)),
		ulwAlpha,
	)
	return r != 0
}

var (
	overlayClassOnce sync.Once
	overlayClassErr  error
	overlayWndMap    sync.Map // hwnd(uintptr) -> func(hwnd, msg, wParam, lParam uintptr) uintptr
	overlayWndCB     = syscall.NewCallback(func(hwnd, msg, wParam, lParam uintptr) uintptr {
		if fn, ok := overlayWndMap.Load(hwnd); ok {
			if handler, ok := fn.(func(uintptr, uintptr, uintptr, uintptr) uintptr); ok {
				return handler(hwnd, msg, wParam, lParam)
			}
		}
		r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
		return r
	})
	wmTaskbarCreated uint32
)

func ensureOverlayWindowClass() error {
	overlayClassOnce.Do(func() {
		msgName, _ := syscall.UTF16PtrFromString("TaskbarCreated")
		m, _, _ := procRegisterWindowMsgW.Call(uintptr(unsafe.Pointer(msgName)))
		wmTaskbarCreated = uint32(m)

		hInst, _, _ := procGetModuleHandleW.Call(0)
		hCur, _, _ := procLoadCursorW.Call(0, idcArrow)
		clsName, _ := syscall.UTF16PtrFromString("PureUtilsMonitorOverlay")
		wc := wndClassExW{
			CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			Style:         csDblClks,
			LpfnWndProc:   overlayWndCB,
			HInstance:     hInst,
			HCursor:       hCur,
			LpszClassName: clsName,
		}
		atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if atom == 0 {
			overlayClassErr = err
		}
	})
	return overlayClassErr
}

func isWindows11OrNewer() bool {
	ver := windows.RtlGetVersion()
	return ver != nil && ver.BuildNumber >= 22000
}

func isSystemDarkTheme(pref string) bool {
	switch pref {
	case "dark":
		return true
	case "light":
		return false
	}
	// Query SystemUsesLightTheme (0 = dark taskbar/system, 1 = light).
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer k.Close()
	val, _, err := k.GetIntegerValue("SystemUsesLightTheme")
	if err != nil {
		return true
	}
	return val == 0
}

func windowScale(hwnd uintptr) float64 {
	var dpi uintptr
	if hwnd != 0 && procGetDpiForWindow.Find() == nil {
		dpi, _, _ = procGetDpiForWindow.Call(hwnd)
	}
	if dpi == 0 && procGetDpiForSystem.Find() == nil {
		dpi, _, _ = procGetDpiForSystem.Call()
	}
	if dpi == 0 {
		return 1.0
	}
	return float64(dpi) / 96.0
}

func monitorWorkRectAt(x, y int32) winRect {
	packed := uintptr(uint32(x)) | (uintptr(uint32(y)) << 32)
	const monitorDefaultToNearest = 2
	hMon, _, _ := procMonitorFromPoint.Call(packed, monitorDefaultToNearest)
	if hMon != 0 {
		var mi winMonitorInfo
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		if r, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); r != 0 {
			return mi.RcWork
		}
	}
	return winRect{0, 0, 1920, 1040}
}

func monitorBoundsAt(x, y int32) winRect {
	packed := uintptr(uint32(x)) | (uintptr(uint32(y)) << 32)
	const monitorDefaultToNearest = 2
	hMon, _, _ := procMonitorFromPoint.Call(packed, monitorDefaultToNearest)
	if hMon != 0 {
		var mi winMonitorInfo
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		if r, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); r != 0 {
			return mi.RcMonitor
		}
	}
	return winRect{0, 0, 1920, 1080}
}

func isTaskbarAutoHide() bool {
	var abd appBarData
	abd.CbSize = uint32(unsafe.Sizeof(abd))
	r, _, _ := procSHAppBarMessage.Call(abmGetState, uintptr(unsafe.Pointer(&abd)))
	return (uint32(r) & absAutohide) != 0
}

type popupMenuEntry struct {
	ID        uint32
	Label     string
	Checked   bool
	Separator bool
}

func showPopupMenu(hwnd uintptr, entries []popupMenuEntry) uint32 {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return 0
	}
	defer procDestroyMenu.Call(menu)

	for _, e := range entries {
		if e.Separator {
			procAppendMenuW.Call(menu, mfSeparator, 0, 0)
			continue
		}
		flags := uintptr(mfString)
		if e.Checked {
			flags |= mfChecked
		}
		txt, _ := syscall.UTF16PtrFromString(e.Label)
		procAppendMenuW.Call(menu, flags, uintptr(e.ID), uintptr(unsafe.Pointer(txt)))
	}
	var pt winPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightBtn|tpmReturnCmd|tpmNonotify,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		hwnd,
		0,
	)
	procPostMessageW.Call(hwnd, wmNull, 0, 0)
	return uint32(cmd)
}

var (
	popupMenuClassPtr, _ = syscall.UTF16PtrFromString("#32768")
	winEventThreadMap    sync.Map // threadID(uint32) -> func(event, hwnd uintptr)
	winEventHookCB       = syscall.NewCallback(func(hHook, event, hwnd, idObject, idChild, dwEventThread, dwmsEventTime uintptr) uintptr {
		tid, _, _ := procGetCurrentThreadID.Call()
		if fn, ok := winEventThreadMap.Load(uint32(tid)); ok {
			if handler, ok := fn.(func(uintptr, uintptr)); ok {
				handler(event, hwnd)
			}
		}
		return 0
	})
)

func hasVisiblePopupMenu() bool {
	menuWnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(popupMenuClassPtr)), 0)
	if menuWnd == 0 {
		return false
	}
	vis, _, _ := procIsWindowVisible.Call(menuWnd)
	return vis != 0
}

func isWindowAbove(hwnd, ref uintptr) bool {
	if hwnd == 0 || ref == 0 || hwnd == ref {
		return false
	}
	cur := ref
	for i := 0; i < 128; i++ {
		prev, _, _ := procGetWindow.Call(cur, gwHwndPrev)
		if prev == 0 {
			return false
		}
		if prev == hwnd {
			return true
		}
		cur = prev
	}
	return false
}

func ensureAboveTaskbar(hwnd, shellTray uintptr, allowTopmostFallback bool) {
	if hwnd == 0 || shellTray == 0 {
		return
	}
	if isWindowAbove(hwnd, shellTray) {
		return
	}
	above, _, _ := procGetWindow.Call(shellTray, gwHwndPrev)
	if above != 0 && above != hwnd {
		procSetWindowPos.Call(hwnd, above, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	}
	if !isWindowAbove(hwnd, shellTray) && allowTopmostFallback && !hasVisiblePopupMenu() {
		procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	}
}
