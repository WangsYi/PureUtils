package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"pureutils/pkg/purekit/core"
)

func ForegroundWindow() HWND {
	h, _, _ := procGetForegroundWindow.Call()
	return HWND(h)
}

func ForegroundProcessName() string {
	h := ForegroundWindow()
	if h == 0 {
		return ""
	}
	var pid uint32
	procGetWindowThreadProcessID.Call(uintptr(h), uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	handle, _, _ := procOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer procCloseHandle.Call(handle)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	name := UTF16ToString(buf[:size])
	name = filepath.Base(name)
	name = strings.TrimSuffix(strings.ToLower(name), ".exe")
	return name
}

func FocusWindow(h HWND) error {
	if h == 0 {
		return fmt.Errorf("invalid window")
	}
	if iconic, _, _ := procIsIconic.Call(uintptr(h)); iconic != 0 {
		procShowWindow.Call(uintptr(h), SW_RESTORE)
	}
	r, _, _ := procSetForegroundWindow.Call(uintptr(h))
	if r != 0 {
		return nil
	}
	fg := ForegroundWindow()
	curThread, _, _ := procGetCurrentThreadID.Call()
	var fgThread, targetThread uintptr
	if fg != 0 {
		fgThread, _, _ = procGetWindowThreadProcessID.Call(uintptr(fg), 0)
	}
	targetThread, _, _ = procGetWindowThreadProcessID.Call(uintptr(h), 0)
	attachedFg := fgThread != 0 && fgThread != curThread
	attachedTarget := targetThread != 0 && targetThread != curThread && targetThread != fgThread
	if attachedFg {
		procAttachThreadInput.Call(curThread, fgThread, 1)
	}
	if attachedTarget {
		procAttachThreadInput.Call(curThread, targetThread, 1)
	}
	procBringWindowToTop.Call(uintptr(h))
	r, _, _ = procSetForegroundWindow.Call(uintptr(h))
	if attachedTarget {
		procAttachThreadInput.Call(curThread, targetThread, 0)
	}
	if attachedFg {
		procAttachThreadInput.Call(curThread, fgThread, 0)
	}
	if r == 0 {
		return fmt.Errorf("SetForegroundWindow failed")
	}
	return nil
}

func WindowBoundsAt(p core.Point) (core.RectI, bool) {
	packed := uintptr(uint32(int32(p.X))) | uintptr(uint32(int32(p.Y)))<<32
	h, _, _ := procWindowFromPoint.Call(packed)
	if h == 0 {
		return core.RectI{}, false
	}
	root, _, _ := procGetAncestor.Call(h, GA_ROOT)
	if root != 0 {
		h = root
	}
	bounds, ok := windowVisibleRect(h)
	if !ok {
		return core.RectI{}, false
	}
	if m, err := MonitorAtPoint(p); err == nil {
		bounds = bounds.Intersect(m.Bounds)
	}
	return bounds, true
}

func windowVisibleRect(hwnd uintptr) (core.RectI, bool) {
	var r Rect
	hr, _, _ := procDwmGetWindowAttribute.Call(hwnd, DWMWA_EXTENDED_FRAME_BOUNDS,
		uintptr(unsafe.Pointer(&r)), unsafe.Sizeof(r))
	if !hresultOK(hr) {
		r = Rect{}
		if v, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); v == 0 {
			return core.RectI{}, false
		}
	}
	rect := core.RectI{X: int(r.Left), Y: int(r.Top), W: int(r.Right - r.Left), H: int(r.Bottom - r.Top)}
	return rect, rect.W > 0 && rect.H > 0
}

func isCloakedWindow(hwnd uintptr) bool {
	var cloaked uint32
	hr, _, _ := procDwmGetWindowAttribute.Call(hwnd, DWMWA_CLOAKED,
		uintptr(unsafe.Pointer(&cloaked)), unsafe.Sizeof(cloaked))
	return hresultOK(hr) && cloaked != 0
}

func isIgnoredWindowClass(cn string) bool {
	switch strings.ToLower(strings.TrimSpace(cn)) {
	case "progman", "workerw", "shell_traywnd", "shell_secondarytraywnd",
		"windows.ui.core.corewindow", "edgeuiautohidewndclass",
		"shell_inputswitchtopwindow", "foregroundstaging",
		"multitaskingviewframe", "applicationmanager_desktopshellwindow":
		return true
	default:
		return false
	}
}

var (
	windowEnumMu       sync.Mutex
	windowEnumSink     func(hwnd uintptr) uintptr
	windowEnumCallback = syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if windowEnumSink != nil {
			return windowEnumSink(hwnd)
		}
		return 1
	})
)

func VisibleWindowRects(monitor core.RectI) []core.RectI {
	_ = EnsurePerMonitorV2()
	var out []core.RectI
	windowEnumMu.Lock()
	defer windowEnumMu.Unlock()
	windowEnumSink = func(hwnd uintptr) uintptr {
		if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		if v, _, _ := procIsIconic.Call(hwnd); v != 0 {
			return 1
		}
		if isCloakedWindow(hwnd) {
			return 1
		}
		ex, _, _ := procGetWindowLongW.Call(hwnd, GWL_EXSTYLE)
		if ex&WS_EX_TOOLWINDOW != 0 {
			return 1
		}
		if ex&(WS_EX_LAYERED|WS_EX_TRANSPARENT) == (WS_EX_LAYERED | WS_EX_TRANSPARENT) {
			return 1
		}
		if ex&WS_EX_LAYERED != 0 {
			var crKey uint32
			var alpha uint8
			var flags uint32
			if r, _, _ := procGetLayeredWindowAttributes.Call(hwnd,
				uintptr(unsafe.Pointer(&crKey)),
				uintptr(unsafe.Pointer(&alpha)),
				uintptr(unsafe.Pointer(&flags))); r != 0 && flags&LWA_ALPHA != 0 && alpha == 0 {
				return 1
			}
		}
		class := make([]uint16, 128)
		procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		if isIgnoredWindowClass(UTF16ToString(class)) {
			return 1
		}
		titleLen, _, _ := procGetWindowTextLengthW.Call(hwnd)
		if titleLen == 0 {
			return 1
		}
		rect, ok := windowVisibleRect(hwnd)
		if !ok {
			return 1
		}
		clipped := rect.Intersect(monitor)
		if clipped.W < 8 || clipped.H < 8 {
			return 1
		}
		out = append(out, clipped)
		return 1
	}
	procEnumWindows.Call(windowEnumCallback, 0)
	windowEnumSink = nil
	return out
}

func SetWindowBoundsPhysical(h HWND, r core.RectI, topmost bool) {
	if h == 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	insertAfter := uintptr(0)
	flags := uintptr(SWP_NOZORDER | SWP_NOACTIVATE)
	if topmost {
		insertAfter = HWND_TOPMOST
		flags = SWP_NOACTIVATE
	}
	procSetWindowPos.Call(uintptr(h), insertAfter,
		uintptr(int32(r.X)), uintptr(int32(r.Y)),
		uintptr(int32(r.W)), uintptr(int32(r.H)), flags)
}

func SendPaste() error {
	scanV, _, _ := procMapVirtualKeyW.Call('V', MAPVK_VK_TO_VSC)
	inputs := []Input{
		keyInput(VK_CONTROL, 0, 0),
		keyInput('V', uint16(scanV), 0),
		keyInput('V', uint16(scanV), KEYEVENTF_KEYUP),
		keyInput(VK_CONTROL, 0, KEYEVENTF_KEYUP),
	}
	r, _, err := procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	if r == 0 {
		return fmt.Errorf("SendInput: %w", err)
	}
	return nil
}

func keyInput(vk uint16, scan uint16, flags uint32) Input {
	return Input{Type: 1, Ki: KeybdInput{WVk: vk, WScan: scan, DwFlags: flags}}
}

func OpenPath(path string) error {
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(Utf16Ptr("open"))),
		uintptr(unsafe.Pointer(Utf16Ptr(path))), 0, 0, SW_SHOWNORMAL)
	if r <= 32 {
		return fmt.Errorf("ShellExecute failed (%d) for %s", r, path)
	}
	return nil
}

func RevealInFolder(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("path does not exist: %s", path)
	}
	params := `/select,"` + path + `"`
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(Utf16Ptr("open"))),
		uintptr(unsafe.Pointer(Utf16Ptr("explorer.exe"))),
		uintptr(unsafe.Pointer(Utf16Ptr(params))), 0, SW_SHOWNORMAL)
	if r <= 32 {
		return fmt.Errorf("ShellExecute failed (%d)", r)
	}
	return nil
}

func Beep() {
	procMessageBeep.Call(0)
}
