//go:build windows

package archive

import "syscall"

var procGetACP = syscall.NewLazyDLL("kernel32.dll").NewProc("GetACP")

func systemCodePage() int {
	r, _, _ := procGetACP.Call()
	return int(r)
}
