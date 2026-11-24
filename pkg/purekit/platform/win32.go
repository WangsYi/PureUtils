// Package platform contains pure-Go Windows interop primitives using
// syscalls and COM without cgo.
package platform

import (
	"syscall"
	"unsafe"
)

type (
	HWND      uintptr
	HDC       uintptr
	HBITMAP   uintptr
	HMONITOR  uintptr
	HICON     uintptr
	HANDLE    uintptr
	HHOOK     uintptr
	HMENU     uintptr
	HINSTANCE uintptr
	ATOM      uint16
	LRESULT   uintptr
	WPARAM    uintptr
	LPARAM    uintptr
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")

	procGetDC                         = user32.NewProc("GetDC")
	procReleaseDC                     = user32.NewProc("ReleaseDC")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procGetPhysicalCursorPos          = user32.NewProc("GetPhysicalCursorPos")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procGetWindowThreadProcessID      = user32.NewProc("GetWindowThreadProcessId")
	procGetGUIThreadInfo              = user32.NewProc("GetGUIThreadInfo")
	procWindowFromPoint               = user32.NewProc("WindowFromPoint")
	procGetAncestor                   = user32.NewProc("GetAncestor")
	procGetWindowRect                 = user32.NewProc("GetWindowRect")
	procIsWindowVisible               = user32.NewProc("IsWindowVisible")
	procIsIconic                      = user32.NewProc("IsIconic")
	procIsZoomed                      = user32.NewProc("IsZoomed")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procBringWindowToTop              = user32.NewProc("BringWindowToTop")
	procEnumWindows                   = user32.NewProc("EnumWindows")
	procGetCurrentProcessID           = kernel32.NewProc("GetCurrentProcessId")
	procAttachThreadInput             = user32.NewProc("AttachThreadInput")
	procGetWindowTextW                = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW          = user32.NewProc("GetWindowTextLengthW")
	procGetClassNameW                 = user32.NewProc("GetClassNameW")
	procGetCursorInfo                 = user32.NewProc("GetCursorInfo")
	procDrawIconEx                    = user32.NewProc("DrawIconEx")
	procEnumDisplayMonitors           = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	procMonitorFromPoint              = user32.NewProc("MonitorFromPoint")
	procMonitorFromRect               = user32.NewProc("MonitorFromRect")
	procMonitorFromWindow             = user32.NewProc("MonitorFromWindow")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procPeekMessageW                  = user32.NewProc("PeekMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW            = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadID            = kernel32.NewProc("GetCurrentThreadId")
	procGetModuleHandleW              = kernel32.NewProc("GetModuleHandleW")
	procSetWindowsHookExW             = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx           = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx                = user32.NewProc("CallNextHookEx")
	procGetAsyncKeyState              = user32.NewProc("GetAsyncKeyState")
	procGetKeyState                   = user32.NewProc("GetKeyState")
	procSendInput                     = user32.NewProc("SendInput")
	procMapVirtualKeyW                = user32.NewProc("MapVirtualKeyW")
	procMessageBeep                   = user32.NewProc("MessageBeep")
	procShellExecuteW                 = shell32.NewProc("ShellExecuteW")
	procOpenClipboard                 = user32.NewProc("OpenClipboard")
	procCloseClipboard                = user32.NewProc("CloseClipboard")
	procEmptyClipboard                = user32.NewProc("EmptyClipboard")
	procSetClipboardData              = user32.NewProc("SetClipboardData")
	procGetClipboardData              = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable    = user32.NewProc("IsClipboardFormatAvailable")
	procEnumClipboardFormats          = user32.NewProc("EnumClipboardFormats")
	procGetClipboardFormatNameW       = user32.NewProc("GetClipboardFormatNameW")
	procRegisterClipboardFormatW      = user32.NewProc("RegisterClipboardFormatW")
	procAddClipboardFormatListener    = user32.NewProc("AddClipboardFormatListener")
	procRemoveClipboardFormatListener = user32.NewProc("RemoveClipboardFormatListener")
	procDragQueryFileW                = shell32.NewProc("DragQueryFileW")
	procGlobalAlloc                   = kernel32.NewProc("GlobalAlloc")
	procGlobalFree                    = kernel32.NewProc("GlobalFree")
	procGlobalLock                    = kernel32.NewProc("GlobalLock")
	procGlobalUnlock                  = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                    = kernel32.NewProc("GlobalSize")
	procOpenProcess                   = kernel32.NewProc("OpenProcess")
	procCloseHandle                   = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImageNameW    = kernel32.NewProc("QueryFullProcessImageNameW")
	procCreateCompatibleDC            = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap        = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject                  = gdi32.NewProc("SelectObject")
	procDeleteDC                      = gdi32.NewProc("DeleteDC")
	procDeleteObject                  = gdi32.NewProc("DeleteObject")
	procGetDIBits                     = gdi32.NewProc("GetDIBits")
	procGetObjectW                    = gdi32.NewProc("GetObjectW")
	procBitBlt                        = gdi32.NewProc("BitBlt")
	procCreateDCW                     = gdi32.NewProc("CreateDCW")
	procGetDpiForMonitor              = shcore.NewProc("GetDpiForMonitor")
	procDwmGetWindowAttribute         = dwmapi.NewProc("DwmGetWindowAttribute")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procGetWindowLongW                = user32.NewProc("GetWindowLongW")
	procGetLayeredWindowAttributes    = user32.NewProc("GetLayeredWindowAttributes")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procSHGetPropertyStoreForWindow   = shell32.NewProc("SHGetPropertyStoreForWindow")
)

type Point struct {
	X int32
	Y int32
}

type Rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type Size struct {
	CX int32
	CY int32
}

type MonitorInfoEx struct {
	CbSize    uint32
	RcMonitor Rect
	RcWork    Rect
	DwFlags   uint32
	SzDevice  [32]uint16
}

type CursorInfo struct {
	CbSize      uint32
	Flags       uint32
	HCursor     HICON
	PtScreenPos Point
}

type WndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     HINSTANCE
	HIcon         HICON
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       HICON
}

type Msg struct {
	Hwnd     HWND
	Message  uint32
	WParam   WPARAM
	LParam   LPARAM
	Time     uint32
	Pt       Point
	LPrivate uint32
}

type BitmapInfoHeader struct {
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

type RGBQuad struct {
	Blue     uint8
	Green    uint8
	Red      uint8
	Reserved uint8
}

type BitmapInfo struct {
	Header BitmapInfoHeader
	Colors [1]RGBQuad
}

type Bitmap struct {
	BmType       int32
	BmWidth      int32
	BmHeight     int32
	BmWidthBytes int32
	BmPlanes     uint16
	BmBitsPixel  uint16
	BmBits       uintptr
}

type DropFiles struct {
	PFiles uint32
	Pt     Point
	FNC    int32
	FWide  int32
}

type KeybdInput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type Input struct {
	Type uint32
	_    uint32
	Ki   KeybdInput
	_    [8]byte
}

type KbdLlHookStruct struct {
	VKCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

const (
	WM_NULL            = 0x0000
	WM_DESTROY         = 0x0002
	WM_CLOSE           = 0x0010
	WM_QUIT            = 0x0012
	WM_HOTKEY          = 0x0312
	WM_CLIPBOARDUPDATE = 0x031D
	WM_APP             = 0x8000

	WM_KEYDOWN     = 0x0100
	WM_KEYUP       = 0x0101
	WM_SYSKEYDOWN  = 0x0104
	WM_SYSKEYUP    = 0x0105
	WM_LBUTTONDOWN = 0x0201
	WM_RBUTTONDOWN = 0x0204
)

const (
	HWND_MESSAGE                      = ^HWND(2)
	SW_SHOWNORMAL                     = 1
	SW_SHOWMAXIMIZED                  = 3
	SW_RESTORE                        = 9
	SRCCOPY                           = 0x00CC0020
	CAPTUREBLT                        = 0x40000000
	BI_RGB                            = 0
	BI_BITFIELDS                      = 3
	BI_JPEG                           = 4
	BI_PNG                            = 5
	BI_ALPHABITFIELDS                 = 6
	DIB_RGB_COLORS                    = 0
	SM_XVIRTUALSCREEN                 = 76
	SM_YVIRTUALSCREEN                 = 77
	SM_CXVIRTUALSCREEN                = 78
	SM_CYVIRTUALSCREEN                = 79
	SM_CXCURSOR                       = 13
	SM_CYCURSOR                       = 14
	MONITOR_DEFAULTTONEAREST          = 2
	MONITOR_DEFAULTTONULL             = 0
	MONITOR_DEFAULTTOPRIMARY          = 1
	MONITORINFOF_PRIMARY              = 1
	GA_ROOT                           = 2
	DWMWA_EXTENDED_FRAME_BOUNDS       = 9
	DWMWA_CLOAKED                     = 14
	GWL_EXSTYLE                       = ^uintptr(19)
	WS_EX_TRANSPARENT                 = 0x00000020
	WS_EX_TOOLWINDOW                  = 0x00000080
	WS_EX_LAYERED                     = 0x00080000
	LWA_ALPHA                         = 0x00000002
	HWND_TOPMOST                      = ^uintptr(0)
	SWP_NOZORDER                      = 0x0004
	SWP_NOACTIVATE                    = 0x0010
	DI_NORMAL                         = 0x0003
	WH_KEYBOARD_LL                    = 13
	LLKHF_INJECTED                    = 0x10
	LLKHF_ALTDOWN                     = 0x20
	VK_SHIFT                          = 0x10
	VK_CONTROL                        = 0x11
	VK_MENU                           = 0x12
	VK_LSHIFT                         = 0xA0
	VK_RSHIFT                         = 0xA1
	VK_LCONTROL                       = 0xA2
	VK_RCONTROL                       = 0xA3
	VK_LMENU                          = 0xA4
	VK_RMENU                          = 0xA5
	VK_LWIN                           = 0x5B
	VK_RWIN                           = 0x5C
	VK_ESCAPE                         = 0x1B
	VK_RETURN                         = 0x0D
	VK_SPACE                          = 0x20
	VK_TAB                            = 0x09
	VK_BACK                           = 0x08
	VK_DELETE                         = 0x2E
	KEYEVENTF_KEYUP                   = 0x0002
	KEYEVENTF_EXTENDEDKEY             = 0x0001
	MAPVK_VK_TO_VSC                   = 0
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
)

const (
	CF_TEXT        = 1
	CF_BITMAP      = 2
	CF_DIB         = 8
	CF_UNICODETEXT = 13
	CF_HDROP       = 15
	CF_DIBV5       = 17
	GMEM_MOVEABLE  = 0x0002
	GMEM_ZEROINIT  = 0x0040
)

const (
	DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = ^uintptr(3)
	MDT_EFFECTIVE_DPI                          = 0
)

func Utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

func UTF16ToString(buf []uint16) string {
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return syscall.UTF16ToString(buf[:n])
}

func boolOf(r uintptr) bool { return r != 0 }

func hresultOK(r uintptr) bool { return int32(r) >= 0 }

func loword(v uintptr) uint16 { return uint16(v & 0xffff) }

func ptr(p unsafe.Pointer) uintptr { return uintptr(p) }
