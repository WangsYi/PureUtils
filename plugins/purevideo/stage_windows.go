//go:build windows

package purevideo

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/platform"
)

const (
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsClipsiblings = 0x04000000
	csDblClks      = 0x0008

	swHide   = 0
	swShowNA = 8

	swpNoActivate = 0x0010

	wmDestroy     = 0x0002
	wmSize        = 0x0005
	wmPaint       = 0x000F
	wmEraseBkgnd  = 0x0014
	wmContextMenu = 0x007B
	wmKeyDown     = 0x0100
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmLButtonDbl  = 0x0203
	wmRButtonDown = 0x0204
	wmRButtonUp   = 0x0205
	wmMouseWheel  = 0x020A

	idcArrow      = 32512
	dwmDarkMode   = 20
	dwmBorderCol  = 34
	dwmCaptionCol = 35
	dwmTextCol    = 36

	stageBGColorRef = 0x00120D0B // #0B0D12 in BGR
)

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
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

type winPaintStruct struct {
	HDC       uintptr
	FErase    int32
	RcPaint   winRect
	FRestore  int32
	FIncUpd   int32
	RgbReserv [32]byte
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")

	procRegisterClassExW   = user32.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procShowWindow         = user32.NewProc("ShowWindow")
	procSetWindowPos       = user32.NewProc("SetWindowPos")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procGetDpiForWindow    = user32.NewProc("GetDpiForWindow")
	procGetDpiForSystem    = user32.NewProc("GetDpiForSystem")
	procLoadCursorW        = user32.NewProc("LoadCursorW")
	procBeginPaint         = user32.NewProc("BeginPaint")
	procEndPaint           = user32.NewProc("EndPaint")
	procFillRect           = user32.NewProc("FillRect")
	procSetFocus           = user32.NewProc("SetFocus")
	procGetKeyState        = user32.NewProc("GetKeyState")
	procGetDoubleClickTime = user32.NewProc("GetDoubleClickTime")
	procCreateSolidBrush   = gdi32.NewProc("CreateSolidBrush")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procDwmSetWindowAttr   = dwmapi.NewProc("DwmSetWindowAttribute")

	stageClassOnce sync.Once
	stageClassErr  error
	stageBgBrush   uintptr
	stageWndProcCB = syscall.NewCallback(stageWndProc)
	stageInstances sync.Map // hwnd(uintptr) -> *winStage
)

type stageCallbacks struct {
	ParentHWND    uintptr
	OnRepaint     func()
	OnResize      func(w, h int)
	OnPlayPause   func()
	OnFullScreen  func()
	OnContextMenu func()
	OnWheel       func(steps int)
	OnKey         func(key ui.Key, mods ui.Modifiers)
}

type winStage struct {
	mu       sync.Mutex
	hwnd     uintptr
	parent   uintptr
	cb       stageCallbacks
	clickTmr *time.Timer
	visible  bool
	lastX    int
	lastY    int
}

func registerStageClass() error {
	stageClassOnce.Do(func() {
		stageBgBrush, _, _ = procCreateSolidBrush.Call(stageBGColorRef)
		hInst, _, _ := procGetModuleHandleW.Call(0)
		hCur, _, _ := procLoadCursorW.Call(0, idcArrow)
		clsName, err := syscall.UTF16PtrFromString("PureVideoStage")
		if err != nil {
			stageClassErr = err
			return
		}
		wc := wndClassExW{
			CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			Style:         csDblClks,
			LpfnWndProc:   stageWndProcCB,
			HInstance:     hInst,
			HCursor:       hCur,
			HbrBackground: stageBgBrush,
			LpszClassName: clsName,
		}
		atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if atom == 0 {
			stageClassErr = callErr
		}
	})
	return stageClassErr
}

func newWinStage(cb stageCallbacks) (*winStage, error) {
	if cb.ParentHWND == 0 {
		return nil, syscall.EINVAL
	}
	if err := registerStageClass(); err != nil {
		return nil, err
	}
	hInst, _, _ := procGetModuleHandleW.Call(0)
	clsName, _ := syscall.UTF16PtrFromString("PureVideoStage")
	title, _ := syscall.UTF16PtrFromString("")
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(title)),
		wsChild|wsClipsiblings,
		0, 0, 1, 1,
		cb.ParentHWND,
		0,
		hInst,
		0,
	)
	if hwnd == 0 {
		return nil, err
	}
	st := &winStage{
		hwnd:   hwnd,
		parent: cb.ParentHWND,
		cb:     cb,
	}
	stageInstances.Store(hwnd, st)
	return st, nil
}

func (s *winStage) HWND() uintptr {
	if s == nil {
		return 0
	}
	return s.hwnd
}

func (s *winStage) SyncBounds(barDIP float64, show bool) {
	if s == nil || s.hwnd == 0 || s.parent == 0 {
		return
	}
	if !show {
		if s.visible {
			_, _, _ = procShowWindow.Call(s.hwnd, swHide)
			s.visible = false
		}
		return
	}
	var rc winRect
	_, _, _ = procGetClientRect.Call(s.parent, uintptr(unsafe.Pointer(&rc)))
	cw := int(rc.Right - rc.Left)
	ch := int(rc.Bottom - rc.Top)
	if cw < 1 {
		cw = 1
	}
	dpi := windowDPI(s.parent)
	barPhys := int(barDIP*float64(dpi)/96.0 + 0.5)
	sh := ch - barPhys
	if sh < 0 {
		sh = 0
	}
	if !s.visible {
		_, _, _ = procShowWindow.Call(s.hwnd, swShowNA)
		s.visible = true
	}
	_, _, _ = procSetWindowPos.Call(
		s.hwnd,
		0, // HWND_TOP
		0, 0, uintptr(cw), uintptr(sh),
		swpNoActivate,
	)
	if s.lastX != cw || s.lastY != sh {
		s.lastX, s.lastY = cw, sh
		if s.cb.OnResize != nil {
			s.cb.OnResize(cw, sh)
		}
	}
}

func (s *winStage) Destroy() {
	if s == nil || s.hwnd == 0 {
		return
	}
	s.mu.Lock()
	if s.clickTmr != nil {
		s.clickTmr.Stop()
		s.clickTmr = nil
	}
	hwnd := s.hwnd
	s.hwnd = 0
	s.mu.Unlock()
	stageInstances.Delete(hwnd)
	_, _, _ = procDestroyWindow.Call(hwnd)
}

func windowDPI(hwnd uintptr) uint32 {
	if err := procGetDpiForWindow.Find(); err == nil && hwnd != 0 {
		if d, _, _ := procGetDpiForWindow.Call(hwnd); d > 0 {
			return uint32(d)
		}
	}
	if err := procGetDpiForSystem.Find(); err == nil {
		if d, _, _ := procGetDpiForSystem.Call(); d > 0 {
			return uint32(d)
		}
	}
	return 96
}

func stageWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	raw, ok := stageInstances.Load(hwnd)
	if !ok {
		r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
		return r
	}
	st := raw.(*winStage)

	switch msg {
	case wmEraseBkgnd:
		if stageBgBrush != 0 && wParam != 0 {
			var rc winRect
			_, _, _ = procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
			_, _, _ = procFillRect.Call(wParam, uintptr(unsafe.Pointer(&rc)), stageBgBrush)
			return 1
		}
	case wmPaint:
		var ps winPaintStruct
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if stageBgBrush != 0 && hdc != 0 {
			_, _, _ = procFillRect.Call(hdc, uintptr(unsafe.Pointer(&ps.RcPaint)), stageBgBrush)
		}
		_, _, _ = procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if st.cb.OnRepaint != nil {
			st.cb.OnRepaint()
		}
		return 0
	case wmSize:
		w := int(uint16(lParam & 0xFFFF))
		h := int(uint16((lParam >> 16) & 0xFFFF))
		if st.cb.OnResize != nil {
			st.cb.OnResize(w, h)
		}
		return 0
	case wmLButtonDown:
		if st.parent != 0 {
			_, _, _ = procSetFocus.Call(st.parent)
		}
		return 0
	case wmLButtonUp:
		dblMs := 220 * time.Millisecond
		if ms, _, _ := procGetDoubleClickTime.Call(); ms > 0 && ms < 350 {
			dblMs = time.Duration(ms) * time.Millisecond
		}
		st.mu.Lock()
		if st.clickTmr != nil {
			st.clickTmr.Stop()
		}
		st.clickTmr = time.AfterFunc(dblMs, func() {
			if st.cb.OnPlayPause != nil {
				mygo.RunOnMain(st.cb.OnPlayPause)
			}
		})
		st.mu.Unlock()
		return 0
	case wmLButtonDbl:
		st.mu.Lock()
		if st.clickTmr != nil {
			st.clickTmr.Stop()
			st.clickTmr = nil
		}
		st.mu.Unlock()
		if st.cb.OnFullScreen != nil {
			mygo.RunOnMain(st.cb.OnFullScreen)
		}
		return 0
	case wmRButtonDown:
		if st.parent != 0 {
			_, _, _ = procSetFocus.Call(st.parent)
		}
		return 0
	case wmRButtonUp:
		st.mu.Lock()
		if st.clickTmr != nil {
			st.clickTmr.Stop()
			st.clickTmr = nil
		}
		st.mu.Unlock()
		if st.cb.OnContextMenu != nil {
			go func() { mygo.RunOnMain(st.cb.OnContextMenu) }()
		}
		return 0
	case wmContextMenu:
		if lParam == ^uintptr(0) && st.cb.OnContextMenu != nil {
			go func() { mygo.RunOnMain(st.cb.OnContextMenu) }()
			return 0
		}
	case wmMouseWheel:
		delta := int16(uint16((wParam >> 16) & 0xFFFF))
		steps := int(delta) / 120
		if steps == 0 && delta != 0 {
			if delta > 0 {
				steps = 1
			} else {
				steps = -1
			}
		}
		if steps != 0 && st.cb.OnWheel != nil {
			mygo.RunOnMain(func() { st.cb.OnWheel(steps) })
		}
		return 0
	case wmKeyDown:
		if key, mods, ok := mapVirtualKey(uint32(wParam)); ok && st.cb.OnKey != nil {
			mygo.RunOnMain(func() { st.cb.OnKey(key, mods) })
			return 0
		}
	case wmDestroy:
		st.mu.Lock()
		if st.clickTmr != nil {
			st.clickTmr.Stop()
			st.clickTmr = nil
		}
		st.mu.Unlock()
	}

	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func isKeyDown(vk uintptr) bool {
	st, _, _ := procGetKeyState.Call(vk)
	return (uint16(st) & 0x8000) != 0
}

func mapVirtualKey(vk uint32) (ui.Key, ui.Modifiers, bool) {
	var mods ui.Modifiers
	if isKeyDown(0x10) {
		mods |= ui.Shift
	}
	if isKeyDown(0x11) {
		mods |= ui.Ctrl
	}
	if isKeyDown(0x12) {
		mods |= ui.Alt
	}
	switch vk {
	case 0x08:
		return ui.KeyBackspace, mods, true
	case 0x0D:
		return ui.KeyEnter, mods, true
	case 0x20:
		return ui.KeySpace, mods, true
	case 0x1B:
		return ui.KeyEscape, mods, true
	case 0x23:
		return ui.KeyEnd, mods, true
	case 0x24:
		return ui.KeyHome, mods, true
	case 0x25:
		return ui.KeyLeft, mods, true
	case 0x27:
		return ui.KeyRight, mods, true
	case 0x26:
		return ui.KeyUp, mods, true
	case 0x28:
		return ui.KeyDown, mods, true
	case 0x21:
		return ui.KeyPageUp, mods, true
	case 0x22:
		return ui.KeyPageDown, mods, true
	case 0x2E:
		return ui.KeyDelete, mods, true
	case 0x7A:
		return ui.KeyF11, mods, true
	case 'A':
		return ui.KeyA, mods, true
	case 'B':
		return ui.KeyB, mods, true
	case 'C':
		return ui.KeyC, mods, true
	case 'E':
		return ui.KeyE, mods, true
	case 'F':
		return ui.KeyF, mods, true
	case 'I':
		return ui.KeyI, mods, true
	case 'L':
		return ui.KeyL, mods, true
	case 'M':
		return ui.KeyM, mods, true
	case 'O':
		return ui.KeyO, mods, true
	case 'R':
		return ui.KeyR, mods, true
	case 'S':
		return ui.KeyS, mods, true
	case 'T':
		return ui.KeyT, mods, true
	case 'V':
		return ui.KeyV, mods, true
	case '0':
		return ui.Key0, mods, true
	case 0xBB: // '=' / '+'
		return ui.KeyEqual, mods, true
	case 0xBC: // ','
		return ui.KeyComma, mods, true
	case 0xBD: // '-'
		return ui.KeyMinus, mods, true
	case 0xBE: // '.'
		return ui.KeyPeriod, mods, true
	case 0xDB: // '['
		return ui.KeyBracketLeft, mods, true
	case 0xDD: // ']'
		return ui.KeyBracketRight, mods, true
	default:
		return 0, 0, false
	}
}

func applyDarkTitleBar(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	one := int32(1)
	_, _, _ = procDwmSetWindowAttr.Call(hwnd, dwmDarkMode, uintptr(unsafe.Pointer(&one)), 4)
	dark := uint32(0x00120D0B)
	_, _, _ = procDwmSetWindowAttr.Call(hwnd, dwmBorderCol, uintptr(unsafe.Pointer(&dark)), 4)
	_, _, _ = procDwmSetWindowAttr.Call(hwnd, dwmCaptionCol, uintptr(unsafe.Pointer(&dark)), 4)
	white := uint32(0x00F3F4F6)
	_, _, _ = procDwmSetWindowAttr.Call(hwnd, dwmTextCol, uintptr(unsafe.Pointer(&white)), 4)
}

func applyVideoTaskbarIdentity(hwnd uintptr, iconPNG []byte) {
	if hwnd == 0 {
		return
	}
	ico := platform.EnsureICOFromPNG(iconPNG, "video.ico", filepath.Join(os.TempDir(), "pureutils-icons", "video.ico"))
	iconRes := ico
	if iconRes != "" {
		iconRes += ",0"
	}
	relaunch := ""
	if exe, err := os.Executable(); err == nil {
		if abs, err := filepath.Abs(exe); err == nil {
			exe = abs
		}
		relaunch = `"` + exe + `" --video`
	}
	_ = platform.SetWindowTaskbarIdentity(platform.HWND(hwnd), "PureUtils.PureVideo", "PureVideo", relaunch, iconRes)
}
