//go:build windows

package fps

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32Det    = windows.NewLazySystemDLL("user32.dll")
	modShell32Det   = windows.NewLazySystemDLL("shell32.dll")
	modKernel32Det  = windows.NewLazySystemDLL("kernel32.dll")

	procGetFgWnd          = modUser32Det.NewProc("GetForegroundWindow")
	procGetWndThreadPID   = modUser32Det.NewProc("GetWindowThreadProcessId")
	procGetWndRect        = modUser32Det.NewProc("GetWindowRect")
	procGetWndTextW       = modUser32Det.NewProc("GetWindowTextW")
	procIsWindowVisible   = modUser32Det.NewProc("IsWindowVisible")
	procIsIconic          = modUser32Det.NewProc("IsIconic")
	procMonitorFromWnd    = modUser32Det.NewProc("MonitorFromWindow")
	procGetMonitorInfoW   = modUser32Det.NewProc("GetMonitorInfoW")
	procQueryUserNotif    = modShell32Det.NewProc("SHQueryUserNotificationState")
	procQueryFullProcName = modKernel32Det.NewProc("QueryFullProcessImageNameW")
)

type winRect struct {
	Left, Top, Right, Bottom int32
}

type winMonitorInfo struct {
	CbSize    uint32
	RcMonitor winRect
	RcWork    winRect
	DwFlags   uint32
}

// Win32ForegroundProbe collects live foreground window process, fullscreen, and monitor bounds.
type Win32ForegroundProbe struct{}

// Probe inspects the foreground window and returns ForegroundInfo plus its monitor rectangle.
func (Win32ForegroundProbe) Probe() (fg ForegroundInfo, monX, monY, monW, monH int) {
	hwnd, _, _ := procGetFgWnd.Call()
	if hwnd == 0 {
		return ForegroundInfo{}, 0, 0, 0, 0
	}

	var pid uint32
	procGetWndThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ForegroundInfo{}, 0, 0, 0, 0
	}
	fg.PID = pid

	var titleBuf [256]uint16
	n, _, _ := procGetWndTextW.Call(hwnd, uintptr(unsafe.Pointer(&titleBuf[0])), uintptr(len(titleBuf)))
	if n > 0 {
		fg.WindowTitle = strings.TrimSpace(windows.UTF16ToString(titleBuf[:n]))
	}

	if hProc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid); err == nil {
		defer windows.CloseHandle(hProc)
		var pathBuf [1024]uint16
		sz := uint32(len(pathBuf))
		r, _, _ := procQueryFullProcName.Call(uintptr(hProc), 0, uintptr(unsafe.Pointer(&pathBuf[0])), uintptr(unsafe.Pointer(&sz)))
		if r != 0 && sz > 0 {
			fg.ExePath = windows.UTF16ToString(pathBuf[:sz])
			fg.ExeName = normalizeExeName(filepath.Base(fg.ExePath))
		}
	}

	var wRect winRect
	procGetWndRect.Call(hwnd, uintptr(unsafe.Pointer(&wRect)))
	const monitorDefaultToNearest = 2
	hMon, _, _ := procMonitorFromWnd.Call(hwnd, monitorDefaultToNearest)
	if hMon != 0 {
		var mi winMonitorInfo
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		r, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi)))
		if r != 0 {
			monX = int(mi.RcMonitor.Left)
			monY = int(mi.RcMonitor.Top)
			monW = int(mi.RcMonitor.Right - mi.RcMonitor.Left)
			monH = int(mi.RcMonitor.Bottom - mi.RcMonitor.Top)
			if vis, _, _ := procIsWindowVisible.Call(hwnd); vis != 0 {
				if min, _, _ := procIsIconic.Call(hwnd); min == 0 &&
					wRect.Left <= mi.RcMonitor.Left+2 &&
					wRect.Top <= mi.RcMonitor.Top+2 &&
					wRect.Right >= mi.RcMonitor.Right-2 &&
					wRect.Bottom >= mi.RcMonitor.Bottom-2 {
					fg.Fullscreen = true
				}
			}
		}
	}

	const qunsRunningD3DFullScreen = 3
	var state uint32
	if hr, _, _ := procQueryUserNotif.Call(uintptr(unsafe.Pointer(&state))); hr == 0 && state == qunsRunningD3DFullScreen {
		fg.D3DFullscreen = true
		fg.Fullscreen = true
	}
	return fg, monX, monY, monW, monH
}

// IsAnyFullscreenWindow reports whether a non-shell foreground window currently covers its monitor.
func IsAnyFullscreenWindow() bool {
	fg, _, _, monW, monH := Win32ForegroundProbe{}.Probe()
	if monW <= 0 || monH <= 0 || isShellProcess(fg.ExeName) {
		return false
	}
	return fg.Fullscreen || fg.D3DFullscreen
}
