//go:build windows

package collector

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemFirmwareTbl  = modKernel32.NewProc("GetSystemFirmwareTable")
)

// ReadSMBIOS reads and parses the local machine's SMBIOS firmware table (non-admin).
func ReadSMBIOS() SMBIOSData {
	const rsmbSignature = 0x52534D42 // 'RSMB'
	needed, _, _ := procGetSystemFirmwareTbl.Call(rsmbSignature, 0, 0, 0)
	if needed == 0 || needed > 4*1024*1024 {
		return SMBIOSData{}
	}
	buf := make([]byte, needed)
	read, _, _ := procGetSystemFirmwareTbl.Call(rsmbSignature, 0, uintptr(unsafe.Pointer(&buf[0])), needed)
	if read == 0 {
		return SMBIOSData{}
	}
	return ParseRawSMBIOS(buf[:read])
}
