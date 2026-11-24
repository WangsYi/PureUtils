//go:build windows

package pureimage

import (
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

const (
	wmMouseWheel   = 0x020A
	wmPointerWheel = 0x024E
	vkControl      = 0x11
	whMouseLL      = 14
	gaRoot         = 2
)

var gwlWndProc = ^uintptr(3)

var (
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProc   = user32.NewProc("CallWindowProcW")
	procGetKeyState      = user32.NewProc("GetKeyState")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procScreenToClient   = user32.NewProc("ScreenToClient")
	procGetDpiForWindow  = user32.NewProc("GetDpiForWindow")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	procWindowFromPoint  = user32.NewProc("WindowFromPoint")
	procGetAncestor      = user32.NewProc("GetAncestor")
	procSetWindowsHookEx = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx   = user32.NewProc("CallNextHookEx")
	procUnhookWindowsEx  = user32.NewProc("UnhookWindowsHookEx")
)

var (
	activeWheel       atomic.Pointer[wheelHook]
	wheelCallbackOnce sync.Once
	wheelWndProcCB    uintptr
	wheelMouseHookCB  uintptr
)

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type winPoint struct {
	X int32
	Y int32
}

type msllHookStruct struct {
	Pt        winPoint
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

func initWheelCallbacks() {
	wheelCallbackOnce.Do(func() {
		wheelWndProcCB = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
			h := activeWheel.Load()
			if h == nil {
				return 0
			}
			oldProc := h.oldProc.Load()
			if (msg == wmMouseWheel || msg == wmPointerWheel) && !h.closed.Load() {
				delta := int16(uint16(wparam >> 16))
				if delta != 0 && h.onWheel != nil {
					state, _, _ := procGetKeyState.Call(vkControl)
					ctrl := uint16(state)&0x8000 != 0
					onWheel := h.onWheel
					mygo.RunOnMain(func() { onWheel(int(delta), ctrl) })
				}
				return 0
			}
			if oldProc == 0 {
				return 0
			}
			ret, _, _ := procCallWindowProc.Call(oldProc, hwnd, msg, wparam, lparam)
			return ret
		})

		wheelMouseHookCB = syscall.NewCallback(func(nCode, wparam, lparam uintptr) uintptr {
			if nCode == 0 && wparam == wmMouseWheel {
				h := activeWheel.Load()
				if h != nil && !h.closed.Load() {
					hwnd := h.hwnd.Load()
					if hwnd != 0 && isWindowVisible(hwnd) {
						ms := (*msllHookStruct)(unsafe.Pointer(lparam))
						delta := int16(uint16(ms.MouseData >> 16))
						var r winRect
						ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
						if ok != 0 && delta != 0 &&
							ms.Pt.X >= r.Left && ms.Pt.X < r.Right &&
							ms.Pt.Y >= r.Top && ms.Pt.Y < r.Bottom &&
							rootWindowUnderPoint(ms.Pt) == hwnd {
							if h.onWheel != nil {
								state, _, _ := procGetKeyState.Call(vkControl)
								ctrl := uint16(state)&0x8000 != 0
								onWheel := h.onWheel
								mygo.RunOnMain(func() { onWheel(int(delta), ctrl) })
							}
							return 1
						}
					}
				}
			}
			ret, _, _ := procCallNextHookEx.Call(0, nCode, wparam, lparam)
			return ret
		})
	})
}

type wheelHook struct {
	mu        sync.Mutex
	hwnd      atomic.Uintptr
	oldProc   atomic.Uintptr
	mouseHook uintptr
	onWheel   func(delta int, ctrl bool)
	logf      func(string, ...any)
	closed    atomic.Bool
}

func (h *wheelHook) Install(hwnd uintptr, onWheel func(delta int, ctrl bool), logf func(string, ...any)) {
	if hwnd == 0 {
		return
	}
	initWheelCallbacks()
	h.Uninstall()

	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed.Store(false)
	h.onWheel = onWheel
	h.logf = logf
	h.hwnd.Store(hwnd)
	activeWheel.Store(h)

	old, _, err := procSetWindowLongPtr.Call(hwnd, gwlWndProc, wheelWndProcCB)
	if old == 0 && err != nil {
		if logf != nil {
			logf("viewer wheel wndproc hook: %v", err)
		}
	} else {
		h.oldProc.Store(old)
	}

	hook, _, err := procSetWindowsHookEx.Call(whMouseLL, wheelMouseHookCB, 0, 0)
	if hook == 0 {
		if logf != nil {
			logf("viewer low-level wheel hook: %v", err)
		}
		return
	}
	h.mouseHook = hook
}

func (h *wheelHook) Uninstall() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed.Store(true)
	if h.mouseHook != 0 {
		_, _, _ = procUnhookWindowsEx.Call(h.mouseHook)
		h.mouseHook = 0
	}
	hwnd := h.hwnd.Swap(0)
	oldProc := h.oldProc.Swap(0)
	if hwnd != 0 && oldProc != 0 {
		_, _, _ = procSetWindowLongPtr.Call(hwnd, gwlWndProc, oldProc)
	}
	if activeWheel.Load() == h {
		activeWheel.Store(nil)
	}
}

func isWindowVisible(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	v, _, _ := procIsWindowVisible.Call(hwnd)
	return v != 0
}

func rootWindowUnderPoint(pt winPoint) uintptr {
	packed := uintptr(uint32(pt.X)) | (uintptr(uint32(pt.Y)) << 32)
	target, _, _ := procWindowFromPoint.Call(packed)
	if target == 0 {
		return 0
	}
	root, _, _ := procGetAncestor.Call(target, gaRoot)
	if root != 0 {
		return root
	}
	return target
}

func cursorClient(hwnd uintptr) (float32, float32, bool) {
	if hwnd == 0 {
		return 0, 0, false
	}
	var p winPoint
	ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if ok == 0 {
		return 0, 0, false
	}
	ok, _, _ = procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&p)))
	if ok == 0 {
		return 0, 0, false
	}
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	scale := float32(1)
	if dpi > 0 {
		scale = float32(dpi) / 96
	}
	return float32(p.X) / scale, float32(p.Y) / scale, true
}

func mouseDown() bool {
	state, _, _ := procGetKeyState.Call(0x01)
	return uint16(state)&0x8000 != 0
}
