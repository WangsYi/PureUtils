//go:build windows

package collector

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Engine orchestrates static hardware discovery and live Tier-1/Tier-2 telemetry sampling.
type Engine struct {
	mu           sync.RWMutex
	inventory    HardwareInventory
	invReady     bool
	lastSnapshot SystemSnapshot

	cpuSampler   *cpuProcSample
	netSampler   *netSampler
	pdhSampler   *pdhSampler
	nvmlSampler  *nvmlSampler
	adlSampler   *adlSampler
	igclSampler  *intelIGCLSampler
	pawnReader   *PawnIOSensorReader
	helperMu     sync.Mutex
	helper       *HelperServer
	sensorTrack  *SensorTracker
	history      *HistoryRing

	sensorDir     string
	pawnEnabled   bool
	selectedNIC   string
	lastDiskCheck time.Time
	cachedDiskT   []DiskTemperatureReading
}

// NewEngine creates a hardware & telemetry collector engine.
func NewEngine(sensorDir string, pawnIOEnabled bool) *Engine {
	return &Engine{
		cpuSampler:  newCPUSampler(),
		netSampler:  newNetSampler(),
		pdhSampler:  newPDHSampler(),
		nvmlSampler: newNVMLSampler(),
		adlSampler:  newADLSampler(),
		igclSampler: newIntelIGCLSampler(),
		sensorTrack: NewSensorTracker(),
		history:     NewHistoryRing(60),
		sensorDir:   sensorDir,
		pawnEnabled: pawnIOEnabled,
	}
}

// SetOptions updates runtime sampling preferences (adapter filter, PawnIO toggle).
func (e *Engine) SetOptions(selectedAdapter string, pawnIOEnabled bool) {
	e.mu.Lock()
	e.selectedNIC = strings.TrimSpace(selectedAdapter)
	e.pawnEnabled = pawnIOEnabled
	e.mu.Unlock()
}

// RefreshInventory rescans static hardware configuration and returns it.
func (e *Engine) RefreshInventory() HardwareInventory {
	inv := CollectInventory()
	e.mu.Lock()
	e.inventory = inv
	e.invReady = true
	if e.pawnReader == nil {
		e.pawnReader = NewPawnIOSensorReader(e.sensorDir, inv.CPU.Vendor)
	}
	e.mu.Unlock()
	return inv
}

// Inventory returns cached hardware inventory, collecting it on first call.
func (e *Engine) Inventory() HardwareInventory {
	e.mu.RLock()
	ready := e.invReady
	inv := e.inventory
	e.mu.RUnlock()
	if ready {
		return inv
	}
	return e.RefreshInventory()
}

// History returns the 60-sample sparkline ring buffer.
func (e *Engine) History() *HistoryRing {
	return e.history
}

// LastSnapshot returns the latest collected telemetry snapshot.
func (e *Engine) LastSnapshot() SystemSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastSnapshot
}

// PawnIOAccessDenied reports whether direct PawnIO kernel driver access returned ACCESS_DENIED.
func (e *Engine) PawnIOAccessDenied() bool {
	e.mu.RLock()
	pr := e.pawnReader
	e.mu.RUnlock()
	return pr != nil && pr.AccessDenied()
}

// SetHelper attaches an active elevated helper receiver.
func (e *Engine) SetHelper(h *HelperServer) {
	e.helperMu.Lock()
	if e.helper != nil && e.helper != h {
		e.helper.Stop()
	}
	e.helper = h
	e.helperMu.Unlock()
}

// HelperServer returns the current elevated helper server if any.
func (e *Engine) HelperServer() *HelperServer {
	e.helperMu.Lock()
	defer e.helperMu.Unlock()
	return e.helper
}

// Sample performs one full telemetry collection tick and updates history/sensors.
func (e *Engine) Sample() SystemSnapshot {
	return e.SampleDetailed(true)
}

// SampleDetailed performs a telemetry collection tick; includeProcs controls expensive per-process enumeration.
func (e *Engine) SampleDetailed(includeProcs bool) SystemSnapshot {
	now := time.Now()
	inv := e.Inventory()

	e.mu.RLock()
	selNIC := e.selectedNIC
	pawnEnabled := e.pawnEnabled
	pr := e.pawnReader
	e.mu.RUnlock()

	cpuPct, perCore, cpuFreq := e.cpuSampler.sampleCPU()
	mem := readMemoryStatus()
	commitUsed, commitTotal, procCnt, thrCnt, hndCnt := readPerformanceCounts()
	topCPU, topRAM := e.cpuSampler.sampleTopProcesses(now, includeProcs)
	downBps, upBps, totDown, totUp, activeNIC := e.netSampler.sample(selNIC, now)
	pdhRes := e.pdhSampler.sample()
	bat := readBatteryStatus()
	uptime := readSystemUptime()

	ramTotal := mem.TotalPhys
	ramUsed := uint64(0)
	if ramTotal >= mem.AvailPhys {
		ramUsed = ramTotal - mem.AvailPhys
	}
	ramPct := float64(mem.MemoryLoad)
	if ramTotal > 0 {
		ramPct = float64(ramUsed) / float64(ramTotal) * 100.0
	}

	snap := SystemSnapshot{
		Timestamp:        now,
		Uptime:           uptime,
		CPUPercent:       cpuPct,
		PerCorePercent:   perCore,
		CPUFreqMHz:       cpuFreq,
		RAMUsedBytes:     ramUsed,
		RAMTotalBytes:    ramTotal,
		RAMPct:           ramPct,
		CommitUsedBytes:  commitUsed,
		CommitTotalBytes: commitTotal,
		Disks:            pdhRes.Disks,
		DiskReadBps:      pdhRes.DiskReadBps,
		DiskWriteBps:     pdhRes.DiskWriteBps,
		NetDownBps:       downBps,
		NetUpBps:         upBps,
		NetTotalDown:     totDown,
		NetTotalUp:       totUp,
		ActiveNetAdapter: activeNIC,
		Battery:          bat,
		ProcessCount:     procCnt,
		ThreadCount:      thrCnt,
		HandleCount:      hndCnt,
		TopCPU:           topCPU,
		TopRAM:           topRAM,
		PerPIDGPU:        pdhRes.PerPIDGPU,
	}

	// Merge GPU metrics: vendor native (NVML / ADL) + PDH + DXGI VRAM inventory.
	var gpus []GPULive
	nvGpus := e.nvmlSampler.sample()
	amdGpus, adlCPUTemp, adlCPUPower := e.adlSampler.sample()
	gpus = append(gpus, nvGpus...)
	gpus = append(gpus, amdGpus...)

	// Enrich or append Intel/fallback GPU from DXGI + PDH.
	for _, hwGPU := range inv.GPUs {
		matched := false
		for i := range gpus {
			if strings.EqualFold(gpus[i].Vendor, hwGPU.Vendor) ||
				strings.Contains(strings.ToLower(hwGPU.Name), strings.ToLower(gpus[i].Name)) ||
				strings.Contains(strings.ToLower(gpus[i].Name), strings.ToLower(hwGPU.Name)) {
				matched = true
				if gpus[i].VRAMTotalMB == 0 {
					gpus[i].VRAMTotalMB = float64(hwGPU.VRAMMB)
				}
				if gpus[i].Load3D == 0 && pdhRes.GPU3DPct > 0 {
					gpus[i].Load3D = pdhRes.GPU3DPct
				}
				if gpus[i].LoadVideo == 0 {
					gpus[i].LoadVideo = pdhRes.GPUVideoPct
				}
				if gpus[i].LoadCopy == 0 {
					gpus[i].LoadCopy = pdhRes.GPUCopyPct
				}
				if gpus[i].VRAMUsedMB == 0 && pdhRes.VRAMUsedMB > 0 {
					gpus[i].VRAMUsedMB = pdhRes.VRAMUsedMB
				}
				break
			}
		}
		if !matched {
			src := "PDH"
			if hwGPU.Vendor == "Intel" && e.igclSampler.available() {
				src = "IGCL+PDH"
			}
			gpus = append(gpus, GPULive{
				Name:        hwGPU.Name,
				Vendor:      hwGPU.Vendor,
				Load3D:      pdhRes.GPU3DPct,
				LoadVideo:   pdhRes.GPUVideoPct,
				LoadCopy:    pdhRes.GPUCopyPct,
				VRAMUsedMB:  pdhRes.VRAMUsedMB,
				VRAMTotalMB: float64(hwGPU.VRAMMB),
				Source:      src,
			})
		}
	}
	if len(gpus) == 0 && (pdhRes.GPU3DPct > 0 || pdhRes.VRAMUsedMB > 0) {
		gpus = append(gpus, GPULive{
			Name:       "GPU",
			Load3D:     pdhRes.GPU3DPct,
			LoadVideo:  pdhRes.GPUVideoPct,
			LoadCopy:   pdhRes.GPUCopyPct,
			VRAMUsedMB: pdhRes.VRAMUsedMB,
			Source:     "PDH",
		})
	}
	snap.GPUs = gpus

	// Tier-2 PawnIO + SMART disk temp: try direct PawnIO first, else merge from elevated helper.
	if pawnEnabled && pr != nil {
		if t, p, ok := pr.Sample(now); ok {
			snap.CPUTempC = t
			snap.CPUPowerW = p
			snap.PawnIOActive = true
		}
	}

	// Try direct StorageDeviceTemperatureProperty every 5 seconds (succeeds if elevated).
	if now.Sub(e.lastDiskCheck) >= 5*time.Second {
		e.lastDiskCheck = now
		e.cachedDiskT = QueryDiskTemperatures(len(inv.Disks) + 4)
	}
	diskTemps := e.cachedDiskT

	// Merge helper frame if available.
	e.helperMu.Lock()
	srv := e.helper
	e.helperMu.Unlock()
	if srv != nil && srv.Connected() {
		snap.HelperConnected = true
		if hf, ok := srv.Latest(); ok {
			if snap.CPUTempC == 0 && hf.CPUTempC > 0 {
				snap.CPUTempC = hf.CPUTempC
			}
			if snap.CPUPowerW == 0 && hf.CPUPowerW > 0 {
				snap.CPUPowerW = hf.CPUPowerW
			}
			if hf.PawnIO {
				snap.PawnIOActive = true
			}
			if len(diskTemps) == 0 && len(hf.DiskTemps) > 0 {
				diskTemps = hf.DiskTemps
			}
		}
	}
	// Fall back to AMD ADL APU package temp/power when PawnIO or elevated helper is unavailable.
	if snap.CPUTempC <= 0 && adlCPUTemp > 0 {
		snap.CPUTempC = adlCPUTemp
	}
	if snap.CPUPowerW <= 0 && adlCPUPower > 0 {
		snap.CPUPowerW = adlCPUPower
	}

	for _, dt := range diskTemps {
		attached := false
		for i := range snap.Disks {
			if snap.Disks[i].Index == dt.Index {
				snap.Disks[i].TempC = dt.TempC
				snap.Disks[i].WarnTempC = dt.WarnTempC
				snap.Disks[i].CritTempC = dt.CritTempC
				attached = true
				break
			}
		}
		if !attached {
			name := fmt.Sprintf("Disk %d", dt.Index)
			for _, hwd := range inv.Disks {
				if hwd.Index == dt.Index && hwd.Model != "" {
					name = hwd.Model
					break
				}
			}
			snap.Disks = append(snap.Disks, DiskLive{
				Index:     dt.Index,
				Name:      name,
				TempC:     dt.TempC,
				WarnTempC: dt.WarnTempC,
				CritTempC: dt.CritTempC,
			})
		}
	}

	snap.Sensors = e.buildSensors(snap)
	e.sensorTrack.Track(snap.Sensors)
	e.history.RecordSnapshot(snap)

	e.mu.Lock()
	e.lastSnapshot = snap
	e.mu.Unlock()
	return snap
}

func (e *Engine) buildSensors(s SystemSnapshot) []SensorEntry {
	var out []SensorEntry
	out = append(out, SensorEntry{
		ID: "cpu.load", Category: "CPU", Name: "CPU Total Load", Value: s.CPUPercent, Unit: "%", Source: "NT",
	})
	if s.CPUFreqMHz > 0 {
		out = append(out, SensorEntry{
			ID: "cpu.freq", Category: "CPU", Name: "CPU Clock", Value: s.CPUFreqMHz, Unit: "MHz", Source: "PowrProf",
		})
	}
	cpuSrc := "PawnIO"
	if !s.PawnIOActive {
		cpuSrc = "ADL"
	}
	if s.CPUTempC > 0 {
		out = append(out, SensorEntry{
			ID: "cpu.temp", Category: "CPU", Name: "CPU Package Temp", Value: s.CPUTempC, Unit: "°C", Source: cpuSrc,
		})
	}
	if s.CPUPowerW > 0 {
		out = append(out, SensorEntry{
			ID: "cpu.power", Category: "CPU", Name: "CPU Package Power", Value: s.CPUPowerW, Unit: "W", Source: cpuSrc,
		})
	}

	out = append(out, SensorEntry{
		ID: "ram.pct", Category: "Memory", Name: "Physical Memory Load", Value: s.RAMPct, Unit: "%", Source: "Win32",
	})
	out = append(out, SensorEntry{
		ID: "ram.used", Category: "Memory", Name: "Memory Used", Value: float64(s.RAMUsedBytes) / (1024 * 1024), Unit: "MB", Source: "Win32",
	})

	for i, g := range s.GPUs {
		prefix := fmt.Sprintf("gpu.%d", i)
		out = append(out, SensorEntry{
			ID: prefix + ".load3d", Category: "GPU", Name: g.Name + " 3D Load", Value: g.Load3D, Unit: "%", Source: g.Source,
		})
		if g.LoadVideo > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".loadvid", Category: "GPU", Name: g.Name + " Video Engine", Value: g.LoadVideo, Unit: "%", Source: g.Source,
			})
		}
		if g.VRAMUsedMB > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".vram", Category: "GPU", Name: g.Name + " VRAM Used", Value: g.VRAMUsedMB, Unit: "MB", Source: g.Source,
			})
		}
		if g.TempC > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".temp", Category: "GPU", Name: g.Name + " Core Temp", Value: g.TempC, Unit: "°C", Source: g.Source,
			})
		}
		if g.HotspotC > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".hotspot", Category: "GPU", Name: g.Name + " Hotspot Temp", Value: g.HotspotC, Unit: "°C", Source: g.Source,
			})
		}
		if g.PowerW > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".power", Category: "GPU", Name: g.Name + " Power", Value: g.PowerW, Unit: "W", Source: g.Source,
			})
		}
		if g.FanPct > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".fanpct", Category: "GPU", Name: g.Name + " Fan", Value: g.FanPct, Unit: "%", Source: g.Source,
			})
		}
		if g.FanRPM > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".fanrpm", Category: "GPU", Name: g.Name + " Fan Speed", Value: float64(g.FanRPM), Unit: "RPM", Source: g.Source,
			})
		}
		if g.CoreClockMHz > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".clk", Category: "GPU", Name: g.Name + " Core Clock", Value: g.CoreClockMHz, Unit: "MHz", Source: g.Source,
			})
		}
	}

	for _, d := range s.Disks {
		prefix := fmt.Sprintf("disk.%d", d.Index)
		if d.ReadBps > 0 || d.WriteBps > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".read", Category: "Disk", Name: d.Name + " Read", Value: d.ReadBps / (1024 * 1024), Unit: "MB/s", Source: "PDH",
			})
			out = append(out, SensorEntry{
				ID: prefix + ".write", Category: "Disk", Name: d.Name + " Write", Value: d.WriteBps / (1024 * 1024), Unit: "MB/s", Source: "PDH",
			})
		}
		if d.TempC > 0 {
			out = append(out, SensorEntry{
				ID: prefix + ".temp", Category: "Disk", Name: d.Name + " Temperature", Value: d.TempC, Unit: "°C", Source: "SMART",
			})
		}
	}

	out = append(out, SensorEntry{
		ID: "net.down", Category: "System", Name: "Network Download", Value: s.NetDownBps / 1024.0, Unit: "KB/s", Source: "MIB2",
	})
	out = append(out, SensorEntry{
		ID: "net.up", Category: "System", Name: "Network Upload", Value: s.NetUpBps / 1024.0, Unit: "KB/s", Source: "MIB2",
	})
	return out
}

// Close releases all native DLLs, PDH queries, PawnIO handles, and helper pipes.
func (e *Engine) Close() {
	e.helperMu.Lock()
	h := e.helper
	e.helper = nil
	e.helperMu.Unlock()
	if h != nil {
		h.Stop()
	}
	e.mu.Lock()
	pr := e.pawnReader
	e.pawnReader = nil
	e.mu.Unlock()
	if pr != nil {
		pr.Close()
	}
	e.pdhSampler.close()
	e.nvmlSampler.close()
	e.adlSampler.close()
}
