//go:build windows

package collector

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	adlMaxPath         = 256
	adlMaxSensors      = 256
	adlPMLogClkGFX     = 1
	adlPMLogClkMem     = 2
	adlPMLogTempEdge   = 8
	adlPMLogTempMem    = 9
	adlPMLogTempHot    = 13
	adlPMLogFanRPM     = 14
	adlPMLogFanPct     = 15
	adlPMLogGfxAct     = 19
	adlPMLogASICPower  = 23
	adlPMLogTempGFX    = 28
	adlPMLogTempSoc    = 29
	adlPMLogGfxPower   = 30
	adlPMLogTempCPU    = 32
	adlPMLogCPUPower   = 33
	adlPMLogBoardPower = 73
)

func adlVendorMatches(vendorID int32) bool {
	// AMD ADL returns 1002 in decimal (0x3EA) on some drivers and 0x1002 in hex on others.
	return vendorID == 1002 || vendorID == 0x1002
}

func decodeADLPMLog(pm adlPMLogDataOutput, g *GPULive) (cpuTempC, cpuPowerW float64) {
	readPos := func(idx int) (float64, bool) {
		if idx < 0 || idx >= len(pm.Sensors) {
			return 0, false
		}
		s := pm.Sensors[idx]
		if s.Supported == 0 || s.Value <= 0 {
			return 0, false
		}
		return float64(s.Value), true
	}
	readNonNeg := func(idx int) (float64, bool) {
		if idx < 0 || idx >= len(pm.Sensors) {
			return 0, false
		}
		s := pm.Sensors[idx]
		if s.Supported == 0 || s.Value < 0 {
			return 0, false
		}
		return float64(s.Value), true
	}

	for _, idx := range []int{adlPMLogTempEdge, adlPMLogTempGFX, adlPMLogTempHot, adlPMLogTempSoc} {
		if v, ok := readPos(idx); ok {
			g.TempC = v
			break
		}
	}
	if v, ok := readPos(adlPMLogTempHot); ok {
		g.HotspotC = v
	}
	if v, ok := readNonNeg(adlPMLogGfxAct); ok {
		g.Load3D = v
	}
	if v, ok := readPos(adlPMLogClkGFX); ok {
		g.CoreClockMHz = v
	}
	if v, ok := readPos(adlPMLogClkMem); ok {
		g.MemClockMHz = v
	}
	if v, ok := readNonNeg(adlPMLogFanPct); ok {
		g.FanPct = v
	}
	if v, ok := readNonNeg(adlPMLogFanRPM); ok {
		g.FanRPM = int(v)
	}
	for _, idx := range []int{adlPMLogBoardPower, adlPMLogASICPower, adlPMLogGfxPower} {
		if v, ok := readPos(idx); ok {
			g.PowerW = v
			break
		}
	}
	if v, ok := readPos(adlPMLogTempCPU); ok {
		cpuTempC = v
	}
	if v, ok := readPos(adlPMLogCPUPower); ok {
		cpuPowerW = v
	}
	return cpuTempC, cpuPowerW
}

func normalizeODNTemp(raw int32) float64 {
	if raw <= 0 {
		return 0
	}
	if raw > 200 {
		return float64(raw) / 1000.0
	}
	return float64(raw)
}

var (
	procGetProcessHeap = modKernel32.NewProc("GetProcessHeap")
	procHeapAlloc      = modKernel32.NewProc("HeapAlloc")

	// Use HeapAlloc for ADL_MAIN_MALLOC_CALLBACK so C runtime never holds a Go pointer.
	adlMallocCB = syscall.NewCallback(func(size int32) uintptr {
		if size <= 0 {
			return 0
		}
		heap, _, _ := procGetProcessHeap.Call()
		if heap == 0 {
			return 0
		}
		ptr, _, _ := procHeapAlloc.Call(heap, 0x00000008, uintptr(size)) // HEAP_ZERO_MEMORY
		return ptr
	})
)

type adlAdapterInfo struct {
	Size          int32
	AdapterIndex  int32
	UDID          [adlMaxPath]byte
	BusNumber     int32
	DeviceNumber  int32
	FunctionNum   int32
	VendorID      int32
	AdapterName   [adlMaxPath]byte
	DisplayName   [adlMaxPath]byte
	Present       int32
	Exist         int32
	DriverPath    [adlMaxPath]byte
	DriverPathExt [adlMaxPath]byte
	PNPString     [adlMaxPath]byte
	OSDisplayIdx  int32
}

type adlSingleSensorData struct {
	Supported int32
	Value     int32
}

type adlPMLogDataOutput struct {
	Size    int32
	Sensors [adlMaxSensors]adlSingleSensorData
}

type adlTemperature struct {
	Size        int32
	Temperature int32 // millidegrees Celsius
}

type adlSampler struct {
	mu           sync.Mutex
	tried        bool
	available    bool
	dll          *windows.DLL
	ctx          uintptr
	pCreate      *windows.Proc
	pDestroy     *windows.Proc
	pNumAdapters *windows.Proc
	pAdapterInfo *windows.Proc
	pPMLog       *windows.Proc
	pODNTemp     *windows.Proc
	pOD5Temp     *windows.Proc
	adapters     []adlAdapterInfo
}

func newADLSampler() *adlSampler {
	return &adlSampler{}
}

func findADLDLL() string {
	if sys := os.Getenv("WINDIR"); sys != "" {
		for _, name := range []string{"atiadlxx.dll", "atiadlxy.dll"} {
			p := filepath.Join(sys, "System32", name)
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p
			}
		}
	}
	return ""
}

func (a *adlSampler) ensureInit() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tried {
		return a.available
	}
	a.tried = true
	path := findADLDLL()
	if path == "" {
		return false
	}
	dll, err := windows.LoadDLL(path)
	if err != nil {
		return false
	}
	find := func(name string) *windows.Proc {
		p, _ := dll.FindProc(name)
		return p
	}
	a.dll = dll
	a.pCreate = find("ADL2_Main_Control_Create")
	a.pDestroy = find("ADL2_Main_Control_Destroy")
	a.pNumAdapters = find("ADL2_Adapter_NumberOfAdapters_Get")
	a.pAdapterInfo = find("ADL2_Adapter_AdapterInfo_Get")
	a.pPMLog = find("ADL2_New_QueryPMLogData_Get")
	a.pODNTemp = find("ADL2_OverdriveN_Temperature_Get")
	a.pOD5Temp = find("ADL2_Overdrive5_Temperature_Get")

	if a.pCreate == nil || a.pDestroy == nil || a.pNumAdapters == nil || a.pAdapterInfo == nil {
		_ = dll.Release()
		a.dll = nil
		return false
	}
	var ctx uintptr
	r, _, _ := a.pCreate.Call(adlMallocCB, 1, uintptr(unsafe.Pointer(&ctx)))
	if r != 0 || ctx == 0 {
		_ = dll.Release()
		a.dll = nil
		return false
	}
	a.ctx = ctx

	var count int32
	if r, _, _ := a.pNumAdapters.Call(ctx, uintptr(unsafe.Pointer(&count))); r != 0 || count <= 0 || count > 64 {
		a.closeLocked()
		return false
	}
	raw := make([]adlAdapterInfo, count)
	sz := int32(count) * int32(unsafe.Sizeof(adlAdapterInfo{}))
	if r, _, _ := a.pAdapterInfo.Call(ctx, uintptr(unsafe.Pointer(&raw[0])), uintptr(sz)); r != 0 {
		a.closeLocked()
		return false
	}

	seenBus := map[int32]bool{}
	for _, ad := range raw {
		if !adlVendorMatches(ad.VendorID) || ad.Exist == 0 {
			continue
		}
		if seenBus[ad.BusNumber] {
			continue
		}
		seenBus[ad.BusNumber] = true
		a.adapters = append(a.adapters, ad)
	}
	a.available = len(a.adapters) > 0
	if !a.available {
		a.closeLocked()
	}
	return a.available
}

func (a *adlSampler) closeLocked() {
	if a.pDestroy != nil && a.ctx != 0 {
		a.pDestroy.Call(a.ctx)
		a.ctx = 0
	}
	if a.dll != nil {
		_ = a.dll.Release()
		a.dll = nil
	}
	a.adapters = nil
	a.available = false
}

func (a *adlSampler) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.available || a.dll != nil || a.ctx != 0 {
		a.closeLocked()
	}
}

func (a *adlSampler) sample() ([]GPULive, float64, float64) {
	if !a.ensureInit() {
		return nil, 0, 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.available {
		return nil, 0, 0
	}
	out := make([]GPULive, 0, len(a.adapters))
	var apuCPUTemp, apuCPUPower float64
	for _, ad := range a.adapters {
		name := strings.TrimRight(string(ad.AdapterName[:]), "\x00 ")
		if name == "" {
			name = "AMD Radeon GPU"
		}
		g := GPULive{
			Name:   name,
			Vendor: "AMD",
			Source: "ADL",
		}
		if a.pPMLog != nil {
			var pm adlPMLogDataOutput
			pm.Size = int32(unsafe.Sizeof(pm))
			if r, _, _ := a.pPMLog.Call(a.ctx, uintptr(ad.AdapterIndex), uintptr(unsafe.Pointer(&pm))); r == 0 {
				ct, cp := decodeADLPMLog(pm, &g)
				if apuCPUTemp <= 0 && ct > 0 {
					apuCPUTemp = ct
				}
				if apuCPUPower <= 0 && cp > 0 {
					apuCPUPower = cp
				}
			}
		}
		if g.TempC == 0 && a.pODNTemp != nil {
			var tempC int32
			if r, _, _ := a.pODNTemp.Call(a.ctx, uintptr(ad.AdapterIndex), 1, uintptr(unsafe.Pointer(&tempC))); r == 0 && tempC > 0 {
				g.TempC = normalizeODNTemp(tempC)
			}
		}
		if g.TempC == 0 && a.pOD5Temp != nil {
			var t adlTemperature
			t.Size = int32(unsafe.Sizeof(t))
			if r, _, _ := a.pOD5Temp.Call(a.ctx, uintptr(ad.AdapterIndex), 0, uintptr(unsafe.Pointer(&t))); r == 0 && t.Temperature > 0 {
				g.TempC = float64(t.Temperature) / 1000.0
			}
		}
		out = append(out, g)
	}
	return out, apuCPUTemp, apuCPUPower
}
