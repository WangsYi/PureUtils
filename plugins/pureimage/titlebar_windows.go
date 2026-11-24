//go:build windows

package pureimage

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"pureutils/pkg/purekit/platform"
)

const (
	dwmwaUseImmersiveDarkMode = 20
	dwmwaBorderColor          = 34
	dwmwaCaptionColor         = 35
	dwmwaTextColor            = 36
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
)

func applyDarkTitleBar(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	proc := dwmapi.NewProc("DwmSetWindowAttribute")
	one := int32(1)
	_, _, _ = proc.Call(hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&one)), 4)
	black := uint32(0x00111216)
	_, _, _ = proc.Call(hwnd, dwmwaCaptionColor, uintptr(unsafe.Pointer(&black)), 4)
	_, _, _ = proc.Call(hwnd, dwmwaBorderColor, uintptr(unsafe.Pointer(&black)), 4)
	white := uint32(0x00F3F4F6)
	_, _, _ = proc.Call(hwnd, dwmwaTextColor, uintptr(unsafe.Pointer(&white)), 4)
}

func applyViewerTaskbarIdentity(hwnd uintptr, iconPNG []byte) {
	if hwnd == 0 {
		return
	}
	ico := platform.EnsureICOFromPNG(iconPNG, "image.ico", filepath.Join(os.TempDir(), "pureutils-icons", "image.ico"))
	iconRes := ico
	if iconRes != "" {
		iconRes += ",0"
	}
	relaunch := ""
	if exe, err := os.Executable(); err == nil {
		if abs, err := filepath.Abs(exe); err == nil {
			exe = abs
		}
		relaunch = `"` + exe + `" --view`
	}
	_ = platform.SetWindowTaskbarIdentity(platform.HWND(hwnd), "PureUtils.PureImage", "PureImage", relaunch, iconRes)
}
