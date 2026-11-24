package platform

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

type comGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type propertyKey struct {
	Format comGUID
	ID     uint32
}

type propVariant struct {
	VT         uint16
	WReserved1 uint16
	WReserved2 uint16
	WReserved3 uint16
	Val        uint64
	Extra      uint64
}

var (
	iidIPropertyStore = comGUID{
		Data1: 0x886d8eeb,
		Data2: 0x8cf2,
		Data3: 0x4446,
		Data4: [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99},
	}
	fmtIDAppUserModel = comGUID{
		Data1: 0x9f4c2855,
		Data2: 0x9f79,
		Data3: 0x4b39,
		Data4: [8]byte{0xa8, 0xd0, 0xe1, 0xd4, 0x2d, 0xe1, 0xd5, 0xf3},
	}
	pkeyAppUserModelID                  = propertyKey{Format: fmtIDAppUserModel, ID: 5}
	pkeyAppUserModelRelaunchDisplayName = propertyKey{Format: fmtIDAppUserModel, ID: 4}
	pkeyAppUserModelRelaunchIcon        = propertyKey{Format: fmtIDAppUserModel, ID: 3}
	pkeyAppUserModelRelaunchCommand     = propertyKey{Format: fmtIDAppUserModel, ID: 2}
)

const (
	vtLPWSTR      = 31
	ipropRelease  = 2
	ipropSetValue = 6
	ipropCommit   = 7
)

func comVtblCall(obj uintptr, slot int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	full := make([]uintptr, 0, len(args)+1)
	full = append(full, obj)
	full = append(full, args...)
	r, _, _ := syscall.SyscallN(fn, full...)
	return r
}

func EnsureICOFromPNG(pngData []byte, exeRelativeResource, cachePath string) string {
	if exeRelativeResource != "" {
		if exe, err := os.Executable(); err == nil {
			cand := filepath.Join(filepath.Dir(exe), "resources", exeRelativeResource)
			if _, err := os.Stat(cand); err == nil {
				return cand
			}
		}
	}
	if cachePath == "" || len(pngData) < 24 {
		return ""
	}
	ico := encodePNGAsICO(pngData)
	if len(ico) == 0 {
		return ""
	}
	if existing, err := os.ReadFile(cachePath); err == nil && bytes.Equal(existing, ico) {
		return cachePath
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return ""
	}
	tmp := cachePath + ".tmp"
	if err := os.WriteFile(tmp, ico, 0o644); err != nil {
		return ""
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	return cachePath
}

func encodePNGAsICO(pngData []byte) []byte {
	var pngSig = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if len(pngData) < 24 || !bytes.Equal(pngData[:8], pngSig) {
		return nil
	}
	w := binary.BigEndian.Uint32(pngData[16:20])
	h := binary.BigEndian.Uint32(pngData[20:24])
	var wb, hb byte
	if w > 0 && w < 256 {
		wb = byte(w)
	}
	if h > 0 && h < 256 {
		hb = byte(h)
	}
	buf := make([]byte, 22+len(pngData))
	binary.LittleEndian.PutUint16(buf[0:2], 0)
	binary.LittleEndian.PutUint16(buf[2:4], 1)
	binary.LittleEndian.PutUint16(buf[4:6], 1)
	buf[6] = wb
	buf[7] = hb
	buf[8] = 0
	buf[9] = 0
	binary.LittleEndian.PutUint16(buf[10:12], 1)
	binary.LittleEndian.PutUint16(buf[12:14], 32)
	binary.LittleEndian.PutUint32(buf[14:18], uint32(len(pngData)))
	binary.LittleEndian.PutUint32(buf[18:22], 22)
	copy(buf[22:], pngData)
	return buf
}

func SetWindowTaskbarIdentity(hwnd HWND, appID, displayName, relaunchCmd, iconResource string) error {
	if hwnd == 0 || procSHGetPropertyStoreForWindow.Find() != nil {
		return nil
	}
	var store uintptr
	hr, _, _ := procSHGetPropertyStoreForWindow.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&iidIPropertyStore)),
		uintptr(unsafe.Pointer(&store)),
	)
	if !hresultOK(hr) || store == 0 {
		return fmt.Errorf("SHGetPropertyStoreForWindow: 0x%x", uint32(hr))
	}
	defer comVtblCall(store, ipropRelease)

	setStr := func(k propertyKey, val string) error {
		if val == "" {
			return nil
		}
		u16, err := syscall.UTF16FromString(val)
		if err != nil {
			return err
		}
		pv := propVariant{
			VT:  vtLPWSTR,
			Val: uint64(uintptr(unsafe.Pointer(&u16[0]))),
		}
		r := comVtblCall(store, ipropSetValue, uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&pv)))
		runtime.KeepAlive(u16)
		if !hresultOK(r) {
			return fmt.Errorf("SetValue(%d): 0x%x", k.ID, uint32(r))
		}
		return nil
	}

	if err := setStr(pkeyAppUserModelID, appID); err != nil {
		return err
	}
	if err := setStr(pkeyAppUserModelRelaunchDisplayName, displayName); err != nil {
		return err
	}
	if err := setStr(pkeyAppUserModelRelaunchIcon, iconResource); err != nil {
		return err
	}
	if err := setStr(pkeyAppUserModelRelaunchCommand, relaunchCmd); err != nil {
		return err
	}
	if r := comVtblCall(store, ipropCommit); !hresultOK(r) {
		return fmt.Errorf("Commit: 0x%x", uint32(r))
	}
	return nil
}
