//go:build windows

package platform

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

const (
	seeMaskInvokeIDList = 0x0000000C
	swShow              = 5

	spiSetDeskWallpaper = 0x0014
	spifUpdateIniFile   = 0x01
	spifSendChange      = 0x02
)

var procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")

// ShowFileProperties opens the Windows Shell file properties dialog for path.
func ShowFileProperties(hwnd HWND, path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return errors.New("platform: empty path for file properties")
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		trimmed = abs
	}
	verb := Utf16Ptr("properties")
	fileP := Utf16Ptr(trimmed)
	sei := shellExecuteInfoW{
		CbSize: uint32(unsafe.Sizeof(shellExecuteInfoW{})),
		FMask:  seeMaskInvokeIDList,
		Hwnd:   uintptr(hwnd),
		LpVerb: verb,
		LpFile: fileP,
		NShow:  swShow,
	}
	r, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	runtime.KeepAlive(verb)
	runtime.KeepAlive(fileP)
	if r == 0 {
		return fmt.Errorf("ShellExecuteExW(properties) failed: %v", callErr)
	}
	return nil
}

// SetDesktopWallpaper sets the Windows desktop wallpaper to path.
func SetDesktopWallpaper(path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return errors.New("platform: empty path for desktop wallpaper")
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		trimmed = abs
	}
	pathP := Utf16Ptr(trimmed)
	r, _, callErr := procSystemParametersInfoW.Call(
		spiSetDeskWallpaper,
		0,
		uintptr(unsafe.Pointer(pathP)),
		spifUpdateIniFile|spifSendChange,
	)
	runtime.KeepAlive(pathP)
	if r == 0 {
		return fmt.Errorf("SystemParametersInfoW(SPI_SETDESKWALLPAPER) failed: %v", callErr)
	}
	return nil
}
