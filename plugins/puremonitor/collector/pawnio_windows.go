//go:build windows

package collector

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const pawnIODevicePath = `\\?\GLOBALROOT\Device\PawnIO`

// PawnIOSensorReader reads CPU temperature and package power via the optional PawnIO kernel driver.
type PawnIOSensorReader struct {
	mu           sync.Mutex
	sensorDir    string
	vendor       string // "intel" | "amd"
	handle       windows.Handle
	loadedModule string
	useDLL       bool
	dll          *windows.DLL
	pOpen        *windows.Proc
	pLoad        *windows.Proc
	pExec        *windows.Proc
	pClose       *windows.Proc
	tjMax        float64
	energyUnitJ  float64
	prevEnergy   uint64
	prevTime     time.Time
	accessDenied bool
	lastErr      string
}

// NewPawnIOSensorReader constructs a PawnIO reader reading signed .bin modules from sensorDir.
func NewPawnIOSensorReader(sensorDir, cpuVendor string) *PawnIOSensorReader {
	v := strings.ToLower(cpuVendor)
	if strings.Contains(v, "amd") {
		v = "amd"
	} else {
		v = "intel"
	}
	return &PawnIOSensorReader{
		sensorDir: sensorDir,
		vendor:    v,
		tjMax:     100,
	}
}

func pawnIOProgramDirs() []string {
	var dirs []string
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" {
			dirs = append(dirs, filepath.Join(pf, "PawnIO"), filepath.Join(pf, "PawnIO", "modules"))
		}
	}
	return dirs
}

func findPawnIOLibDLL() string {
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" {
			p := filepath.Join(pf, "PawnIO", "PawnIOLib.dll")
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p
			}
		}
	}
	return ""
}

// IsPawnIODriverInstalled reports whether the PawnIO kernel service is registered on this system.
func IsPawnIODriverInstalled() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\PawnIO`, registry.QUERY_VALUE)
	if err == nil {
		k.Close()
		return true
	}
	for _, dir := range pawnIOProgramDirs() {
		for _, name := range []string{"PawnIOLib.dll", "PawnIOUtil.exe", "PawnIO.sys"} {
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
				return true
			}
		}
	}
	return false
}

// Close releases any open PawnIO device handle.
func (r *PawnIOSensorReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeHandleLocked()
	if r.dll != nil {
		_ = r.dll.Release()
		r.dll = nil
		r.pOpen, r.pLoad, r.pExec, r.pClose = nil, nil, nil, nil
	}
}

func (r *PawnIOSensorReader) closeHandleLocked() {
	if r.handle != 0 {
		if r.useDLL && r.pClose != nil {
			_, _, _ = r.pClose.Call(uintptr(r.handle))
		} else {
			_ = windows.CloseHandle(r.handle)
		}
		r.handle = 0
		r.loadedModule = ""
		r.useDLL = false
	}
}

// AccessDenied reports whether opening PawnIO failed due to non-administrator permissions.
func (r *PawnIOSensorReader) AccessDenied() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.accessDenied
}

// Active reports whether PawnIO device communication succeeded recently.
func (r *PawnIOSensorReader) Active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.handle != 0 && r.loadedModule != ""
}

// LastError returns the latest error string if initialization failed.
func (r *PawnIOSensorReader) LastError() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr
}

// Sample reads CPU temperature (°C) and CPU package power (W).
func (r *PawnIOSensorReader) Sample(now time.Time) (tempC, powerW float64, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.ensureOpenLocked() {
		return 0, 0, false
	}

	switch r.vendor {
	case "amd":
		if smnVal, err := r.execFnLocked("ioctl_read_smn", []uint64{smnAMDZenCurTmp}); err == nil {
			if t, tok := DecodeAMDZenTemp(smnVal); tok {
				tempC = t
				ok = true
			}
		}
		if r.energyUnitJ == 0 {
			if u, err := r.execFnLocked("ioctl_read_msr", []uint64{msrAMDRAPLUnit}); err == nil {
				r.energyUnitJ = DecodeEnergyUnitJ(u)
			}
		}
		if r.energyUnitJ > 0 {
			if e, err := r.execFnLocked("ioctl_read_msr", []uint64{msrAMDPkgEnergy}); err == nil {
				if !r.prevTime.IsZero() {
					powerW = ComputeRAPLPowerW(r.prevEnergy, e, r.energyUnitJ, now.Sub(r.prevTime).Seconds())
				}
				r.prevEnergy = e
				r.prevTime = now
			}
		}
	default:
		if r.tjMax == 100 {
			if tjRaw, err := r.execFnLocked("ioctl_read_msr", []uint64{msrIntelTempTarget}); err == nil {
				r.tjMax = DecodeIntelTjMax(tjRaw)
			}
		}
		if msrVal, err := r.execFnLocked("ioctl_read_msr", []uint64{msrIntelPkgThermStat}); err == nil {
			if t, tok := DecodeIntelThermTemp(msrVal, r.tjMax); tok {
				tempC = t
				ok = true
			}
		}
		if !ok {
			if msrVal, err := r.execFnLocked("ioctl_read_msr", []uint64{msrIntelThermStatus}); err == nil {
				if t, tok := DecodeIntelThermTemp(msrVal, r.tjMax); tok {
					tempC = t
					ok = true
				}
			}
		}
		if r.energyUnitJ == 0 {
			if u, err := r.execFnLocked("ioctl_read_msr", []uint64{msrIntelRAPLUnit}); err == nil {
				r.energyUnitJ = DecodeEnergyUnitJ(u)
			}
		}
		if r.energyUnitJ > 0 {
			if e, err := r.execFnLocked("ioctl_read_msr", []uint64{msrIntelPkgEnergy}); err == nil {
				if !r.prevTime.IsZero() {
					powerW = ComputeRAPLPowerW(r.prevEnergy, e, r.energyUnitJ, now.Sub(r.prevTime).Seconds())
				}
				r.prevEnergy = e
				r.prevTime = now
			}
		}
	}
	return tempC, powerW, ok
}

func (r *PawnIOSensorReader) findModuleBinary() string {
	candidates := []string{"IntelMSR.bin", "IntelMSR.amx"}
	if r.vendor == "amd" {
		candidates = []string{"AMDFamily17.bin", "RyzenSMU.bin", "AMDFamily17.amx"}
	}
	dirs := []string{r.sensorDir}
	dirs = append(dirs, pawnIOProgramDirs()...)
	for _, d := range dirs {
		if strings.TrimSpace(d) == "" {
			continue
		}
		for _, name := range candidates {
			p := filepath.Join(d, name)
			if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Size() > 16 {
				return p
			}
		}
	}
	return ""
}

func (r *PawnIOSensorReader) ensureDLLProcsLocked() bool {
	if r.pOpen != nil && r.pLoad != nil && r.pExec != nil && r.pClose != nil {
		return true
	}
	path := findPawnIOLibDLL()
	if path == "" {
		return false
	}
	dll, err := windows.LoadDLL(path)
	if err != nil {
		return false
	}
	pOpen, e1 := dll.FindProc("pawnio_open_win32")
	pLoad, e2 := dll.FindProc("pawnio_load_win32")
	pExec, e3 := dll.FindProc("pawnio_execute_win32")
	pClose, e4 := dll.FindProc("pawnio_close_win32")
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		_ = dll.Release()
		return false
	}
	r.dll = dll
	r.pOpen, r.pLoad, r.pExec, r.pClose = pOpen, pLoad, pExec, pClose
	return true
}

func (r *PawnIOSensorReader) ensureOpenLocked() bool {
	if r.handle != 0 && r.loadedModule != "" {
		return true
	}
	modPath := r.findModuleBinary()
	if modPath == "" {
		r.lastErr = "missing module binary"
		return false
	}
	modBytes, err := os.ReadFile(modPath)
	if err != nil || len(modBytes) == 0 {
		r.lastErr = "failed to read module binary"
		return false
	}

	// Prefer official PawnIOLib.dll when installed locally; fall back to direct device IOCTL.
	if r.ensureDLLProcsLocked() {
		var h uintptr
		okOpen, _, openErr := r.pOpen.Call(uintptr(unsafe.Pointer(&h)))
		if okOpen != 0 && h != 0 {
			okLoad, _, loadErr := r.pLoad.Call(h, uintptr(unsafe.Pointer(&modBytes[0])), uintptr(len(modBytes)))
			if okLoad != 0 {
				r.handle = windows.Handle(h)
				r.loadedModule = modPath
				r.useDLL = true
				r.accessDenied = false
				r.lastErr = ""
				return true
			}
			_, _, _ = r.pClose.Call(h)
			r.lastErr = loadErr.Error()
		} else {
			if errors.Is(openErr, windows.ERROR_ACCESS_DENIED) {
				r.accessDenied = true
				r.lastErr = "access denied (requires elevated helper)"
				return false
			}
			if openErr != nil {
				r.lastErr = openErr.Error()
			}
		}
	}

	if r.handle == 0 {
		p, _ := windows.UTF16PtrFromString(pawnIODevicePath)
		h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				r.accessDenied = true
				r.lastErr = "access denied (requires elevated helper)"
			} else {
				r.lastErr = err.Error()
			}
			return false
		}
		r.handle = h
		r.useDLL = false
	}
	var ret uint32
	if err := windows.DeviceIoControl(r.handle, ioctlPawnIOLoadBinary, &modBytes[0], uint32(len(modBytes)), nil, 0, &ret, nil); err != nil {
		r.lastErr = err.Error()
		_ = windows.CloseHandle(r.handle)
		r.handle = 0
		return false
	}
	r.loadedModule = modPath
	r.accessDenied = false
	r.lastErr = ""
	return true
}

func (r *PawnIOSensorReader) execFnLocked(fnName string, args []uint64) (uint64, error) {
	if r.handle == 0 {
		return 0, errors.New("pawnio not open")
	}
	if r.useDLL && r.pExec != nil {
		cName := append([]byte(strings.TrimSpace(fnName)), 0)
		var inPtr uintptr
		if len(args) > 0 {
			inPtr = uintptr(unsafe.Pointer(&args[0]))
		}
		var outVal [1]uint64
		var retCount uintptr
		ok, _, callErr := r.pExec.Call(
			uintptr(r.handle),
			uintptr(unsafe.Pointer(&cName[0])),
			inPtr,
			uintptr(len(args)),
			uintptr(unsafe.Pointer(&outVal[0])),
			1,
			uintptr(unsafe.Pointer(&retCount)),
		)
		if ok == 0 {
			return 0, callErr
		}
		if retCount < 1 {
			return 0, errors.New("short pawnio reply")
		}
		return outVal[0], nil
	}
	inBuf := BuildPawnIOExecuteInput(fnName, args)
	var outBuf [8]byte
	var ret uint32
	err := windows.DeviceIoControl(
		r.handle,
		ioctlPawnIOExecuteFn,
		&inBuf[0],
		uint32(len(inBuf)),
		&outBuf[0],
		uint32(len(outBuf)),
		&ret,
		nil,
	)
	if err != nil {
		return 0, err
	}
	if ret < 8 {
		return 0, errors.New("short pawnio reply")
	}
	return binary.LittleEndian.Uint64(outBuf[:8]), nil
}
