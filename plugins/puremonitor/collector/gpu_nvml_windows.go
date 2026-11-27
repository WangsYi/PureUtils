//go:build windows

package collector

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nvmlUtilization struct {
	GPU    uint32
	Memory uint32
}

type nvmlMemory struct {
	Total uint64
	Free  uint64
	Used  uint64
}

type nvmlSampler struct {
	mu           sync.Mutex
	tried        bool
	available    bool
	dll          *windows.DLL
	pInit        *windows.Proc
	pShutdown    *windows.Proc
	pCount       *windows.Proc
	pByIndex     *windows.Proc
	pGetName     *windows.Proc
	pGetTemp     *windows.Proc
	pGetUtil     *windows.Proc
	pGetMem      *windows.Proc
	pGetPower    *windows.Proc
	pGetFan      *windows.Proc
	pGetClock    *windows.Proc
	handles      []uintptr
	names        []string
	failStreak   int
}

func newNVMLSampler() *nvmlSampler {
	return &nvmlSampler{}
}

func findNVMLDLL() string {
	candidates := []string{}
	if sys := os.Getenv("WINDIR"); sys != "" {
		candidates = append(candidates, filepath.Join(sys, "System32", "nvml.dll"))
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		candidates = append(candidates, filepath.Join(pf, "NVIDIA Corporation", "NVSMI", "nvml.dll"))
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

func (n *nvmlSampler) ensureInit() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.tried {
		return n.available
	}
	n.tried = true
	path := findNVMLDLL()
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
	n.dll = dll
	n.pInit = find("nvmlInit_v2")
	if n.pInit == nil {
		n.pInit = find("nvmlInit")
	}
	n.pShutdown = find("nvmlShutdown")
	n.pCount = find("nvmlDeviceGetCount_v2")
	if n.pCount == nil {
		n.pCount = find("nvmlDeviceGetCount")
	}
	n.pByIndex = find("nvmlDeviceGetHandleByIndex_v2")
	if n.pByIndex == nil {
		n.pByIndex = find("nvmlDeviceGetHandleByIndex")
	}
	n.pGetName = find("nvmlDeviceGetName")
	n.pGetTemp = find("nvmlDeviceGetTemperature")
	n.pGetUtil = find("nvmlDeviceGetUtilizationRates")
	n.pGetMem = find("nvmlDeviceGetMemoryInfo")
	n.pGetPower = find("nvmlDeviceGetPowerUsage")
	n.pGetFan = find("nvmlDeviceGetFanSpeed")
	n.pGetClock = find("nvmlDeviceGetClockInfo")

	if n.pInit == nil || n.pCount == nil || n.pByIndex == nil {
		_ = dll.Release()
		n.dll = nil
		return false
	}
	if r, _, _ := n.pInit.Call(); r != 0 {
		_ = dll.Release()
		n.dll = nil
		return false
	}
	var count uint32
	if r, _, _ := n.pCount.Call(uintptr(unsafe.Pointer(&count))); r != 0 || count == 0 || count > 8 {
		n.shutdownLocked()
		return false
	}
	for i := uint32(0); i < count; i++ {
		var h uintptr
		if r, _, _ := n.pByIndex.Call(uintptr(i), uintptr(unsafe.Pointer(&h))); r != 0 || h == 0 {
			continue
		}
		name := "NVIDIA GPU"
		if n.pGetName != nil {
			var nameBuf [96]byte
			if r2, _, _ := n.pGetName.Call(h, uintptr(unsafe.Pointer(&nameBuf[0])), uintptr(len(nameBuf))); r2 == 0 {
				if s := strings.TrimRight(string(nameBuf[:]), "\x00 "); s != "" {
					name = s
				}
			}
		}
		n.handles = append(n.handles, h)
		n.names = append(n.names, name)
	}
	n.available = len(n.handles) > 0
	if !n.available {
		n.shutdownLocked()
	}
	return n.available
}

func (n *nvmlSampler) shutdownLocked() {
	if n.pShutdown != nil {
		n.pShutdown.Call()
	}
	if n.dll != nil {
		_ = n.dll.Release()
		n.dll = nil
	}
	n.handles = nil
	n.names = nil
	n.available = false
}

func (n *nvmlSampler) close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.available || n.dll != nil {
		n.shutdownLocked()
	}
}

func (n *nvmlSampler) sample() []GPULive {
	if !n.ensureInit() {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.available {
		return nil
	}
	const (
		nvmlTempGPU   = 0
		nvmlClockGFX  = 0
		nvmlClockMem  = 2
	)
	out := make([]GPULive, 0, len(n.handles))
	anyOK := false
	for i, h := range n.handles {
		g := GPULive{
			Name:   n.names[i],
			Vendor: "NVIDIA",
			Source: "NVML",
		}
		if n.pGetTemp != nil {
			var temp uint32
			if r, _, _ := n.pGetTemp.Call(h, nvmlTempGPU, uintptr(unsafe.Pointer(&temp))); r == 0 && temp > 0 && temp < 140 {
				g.TempC = float64(temp)
				anyOK = true
			}
		}
		if n.pGetUtil != nil {
			var util nvmlUtilization
			if r, _, _ := n.pGetUtil.Call(h, uintptr(unsafe.Pointer(&util))); r == 0 {
				g.Load3D = float64(util.GPU)
				anyOK = true
			}
		}
		if n.pGetMem != nil {
			var mem nvmlMemory
			if r, _, _ := n.pGetMem.Call(h, uintptr(unsafe.Pointer(&mem))); r == 0 && mem.Total > 0 {
				g.VRAMUsedMB = float64(mem.Used) / (1024 * 1024)
				g.VRAMTotalMB = float64(mem.Total) / (1024 * 1024)
				anyOK = true
			}
		}
		if n.pGetPower != nil {
			var mw uint32
			if r, _, _ := n.pGetPower.Call(h, uintptr(unsafe.Pointer(&mw))); r == 0 && mw > 0 {
				g.PowerW = float64(mw) / 1000.0
				anyOK = true
			}
		}
		if n.pGetFan != nil {
			var fan uint32
			if r, _, _ := n.pGetFan.Call(h, uintptr(unsafe.Pointer(&fan))); r == 0 && fan <= 100 {
				g.FanPct = float64(fan)
			}
		}
		if n.pGetClock != nil {
			var gfxClk, memClk uint32
			if r, _, _ := n.pGetClock.Call(h, nvmlClockGFX, uintptr(unsafe.Pointer(&gfxClk))); r == 0 {
				g.CoreClockMHz = float64(gfxClk)
			}
			if r, _, _ := n.pGetClock.Call(h, nvmlClockMem, uintptr(unsafe.Pointer(&memClk))); r == 0 {
				g.MemClockMHz = float64(memClk)
			}
		}
		out = append(out, g)
	}
	if !anyOK {
		n.failStreak++
	} else {
		n.failStreak = 0
	}
	return out
}
