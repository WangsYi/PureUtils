//go:build windows

package collector

import (
	"runtime"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modPowrProf = windows.NewLazySystemDLL("powrprof.dll")
	modPsapi    = windows.NewLazySystemDLL("psapi.dll")

	procGlobalMemoryStatusEx  = modKernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemPowerStatus  = modKernel32.NewProc("GetSystemPowerStatus")
	procGetTickCount64        = modKernel32.NewProc("GetTickCount64")
	procCallNtPowerInfo       = modPowrProf.NewProc("CallNtPowerInformation")
	procGetPerformanceInfo    = modPsapi.NewProc("GetPerformanceInfo")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

type performanceInformation struct {
	Cb                uint32
	CommitTotal       uintptr
	CommitLimit       uintptr
	CommitPeak        uintptr
	PhysicalTotal     uintptr
	PhysicalAvailable uintptr
	SystemCache       uintptr
	KernelTotal       uintptr
	KernelPaged       uintptr
	KernelNonpaged    uintptr
	PageSize          uintptr
	HandleCount       uint32
	ProcessCount      uint32
	ThreadCount       uint32
}

type systemProcessorPerfInfo struct {
	IdleTime       int64
	KernelTime     int64
	UserTime       int64
	DpcTime        int64
	InterruptTime  int64
	InterruptCount uint32
	_              uint32
}

type processorPowerInformation struct {
	Number           uint32
	MaxMhz           uint32
	CurrentMhz       uint32
	MhzLimit         uint32
	MaxIdleState     uint32
	CurrentIdleState uint32
}

type cpuProcSample struct {
	coreBuf   []systemProcessorPerfInfo
	prevCores []systemProcessorPerfInfo
	powerBuf  []processorPowerInformation
	procBuf   []byte
	prevProcs map[uint32]int64
	nextProcs map[uint32]int64
	allProcs  []ProcessUsage
	prevTime  time.Time
	logical   int
}

func newCPUSampler() *cpuProcSample {
	n := runtime.NumCPU()
	if n <= 0 {
		n = 1
	}
	return &cpuProcSample{
		coreBuf:   make([]systemProcessorPerfInfo, 256),
		prevProcs: make(map[uint32]int64),
		nextProcs: make(map[uint32]int64),
		logical:   n,
	}
}

func (s *cpuProcSample) sampleCPU() (totalPct float64, perCore []float64, freqMHz float64) {
	const sysProcPerfInfoClass = 8
	buf := s.coreBuf
	if len(buf) == 0 {
		buf = make([]systemProcessorPerfInfo, 256)
		s.coreBuf = buf
	}
	var retLen uint32
	err := windows.NtQuerySystemInformation(sysProcPerfInfoClass, unsafe.Pointer(&buf[0]), uint32(len(buf)*int(unsafe.Sizeof(buf[0]))), &retLen)
	if err == nil && retLen > 0 {
		count := int(retLen) / int(unsafe.Sizeof(buf[0]))
		if count > 0 {
			s.logical = count
			cur := buf[:count]
			perCore = make([]float64, count)
			if len(s.prevCores) == count {
				var sumPct float64
				for i := 0; i < count; i++ {
					idleDelta := cur[i].IdleTime - s.prevCores[i].IdleTime
					kernDelta := cur[i].KernelTime - s.prevCores[i].KernelTime
					userDelta := cur[i].UserTime - s.prevCores[i].UserTime
					totalDelta := kernDelta + userDelta
					pct := 0.0
					if totalDelta > 0 {
						pct = float64(totalDelta-idleDelta) / float64(totalDelta) * 100.0
					}
					if pct < 0 {
						pct = 0
					}
					if pct > 100 {
						pct = 100
					}
					perCore[i] = pct
					sumPct += pct
				}
				totalPct = sumPct / float64(count)
			}
			if len(s.prevCores) != count {
				s.prevCores = make([]systemProcessorPerfInfo, count)
			}
			copy(s.prevCores, cur)
		}
	}

	// Query CPU frequency via CallNtPowerInformation(ProcessorInformation=11)
	if procCallNtPowerInfo.Find() == nil && s.logical > 0 {
		if len(s.powerBuf) < s.logical {
			s.powerBuf = make([]processorPowerInformation, s.logical)
		}
		pBuf := s.powerBuf[:s.logical]
		sz := uintptr(len(pBuf)) * unsafe.Sizeof(pBuf[0])
		r, _, _ := procCallNtPowerInfo.Call(11, 0, 0, uintptr(unsafe.Pointer(&pBuf[0])), sz)
		if r == 0 {
			var maxFreq float64
			for _, p := range pBuf {
				if float64(p.CurrentMhz) > maxFreq {
					maxFreq = float64(p.CurrentMhz)
				}
			}
			freqMHz = maxFreq
		}
	}
	return totalPct, perCore, freqMHz
}

func readMemoryStatus() memoryStatusEx {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	return m
}

func readPerformanceCounts() (commitUsed, commitTotal uint64, procCount, threadCount, handleCount uint32) {
	if procGetPerformanceInfo.Find() != nil {
		return 0, 0, 0, 0, 0
	}
	var pi performanceInformation
	pi.Cb = uint32(unsafe.Sizeof(pi))
	r, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&pi)), uintptr(pi.Cb))
	if r == 0 {
		return 0, 0, 0, 0, 0
	}
	ps := uint64(pi.PageSize)
	return uint64(pi.CommitTotal) * ps, uint64(pi.CommitLimit) * ps, pi.ProcessCount, pi.ThreadCount, pi.HandleCount
}

func readBatteryStatus() BatteryLive {
	var st systemPowerStatus
	r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&st)))
	if r == 0 {
		return BatteryLive{}
	}
	// BatteryFlag 128 = No system battery; 255 = Unknown
	if st.BatteryFlag&128 != 0 || st.BatteryLifePercent == 255 {
		return BatteryLive{}
	}
	remain := -1
	if st.BatteryLifeTime != 0xFFFFFFFF {
		remain = int(st.BatteryLifeTime / 60)
	}
	return BatteryLive{
		Present:   true,
		Percent:   int(st.BatteryLifePercent),
		Charging:  st.BatteryFlag&8 != 0,
		ACOnline:  st.ACLineStatus == 1,
		RemainMin: remain,
	}
}

func readSystemUptime() time.Duration {
	r, _, _ := procGetTickCount64.Call()
	return time.Duration(uint64(r)) * time.Millisecond
}

func (s *cpuProcSample) releaseProcessScratch() {
	s.procBuf = nil
	s.allProcs = nil
	if len(s.prevProcs) > 0 {
		s.prevProcs = make(map[uint32]int64)
	}
	if len(s.nextProcs) > 0 {
		s.nextProcs = make(map[uint32]int64)
	}
	s.prevTime = time.Time{}
}

func (s *cpuProcSample) sampleTopProcesses(now time.Time, active bool) (topCPU, topRAM []ProcessUsage) {
	if !active {
		if s.procBuf != nil || len(s.prevProcs) > 0 {
			s.releaseProcessScratch()
		}
		return nil, nil
	}
	const sysProcInfoClass = 5
	bufSize := uint32(512 * 1024)
	if uint32(cap(s.procBuf)) > bufSize {
		bufSize = uint32(cap(s.procBuf))
	}
	var buf []byte
	var retLen uint32
	for attempt := 0; attempt < 4; attempt++ {
		if uint32(cap(s.procBuf)) < bufSize {
			s.procBuf = make([]byte, bufSize)
		} else {
			s.procBuf = s.procBuf[:bufSize]
		}
		buf = s.procBuf
		err := windows.NtQuerySystemInformation(sysProcInfoClass, unsafe.Pointer(&buf[0]), bufSize, &retLen)
		if err == nil {
			break
		}
		if err == windows.STATUS_INFO_LENGTH_MISMATCH {
			bufSize = retLen + 64*1024
			if bufSize < 512*1024 {
				bufSize = 512 * 1024
			}
			continue
		}
		return nil, nil
	}

	dt := now.Sub(s.prevTime).Seconds()
	cores := float64(s.logical)
	if cores <= 0 {
		cores = 1
	}

	if s.nextProcs == nil {
		s.nextProcs = make(map[uint32]int64, 256)
	} else {
		clear(s.nextProcs)
	}
	all := s.allProcs[:0]
	for off := uint32(0); int(off)+int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{})) <= len(buf); {
		sp := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buf[off]))
		pid := uint32(sp.UniqueProcessID)
		if pid != 0 {
			name := strings.TrimSpace(sp.ImageName.String())
			if name == "" {
				name = "System"
			}
			totalCPU100ns := sp.KernelTime + sp.UserTime
			s.nextProcs[pid] = totalCPU100ns
			var pct float64
			if prev, ok := s.prevProcs[pid]; ok && dt > 0.05 && totalCPU100ns >= prev {
				// 100ns ticks -> seconds: / 1e7
				cpuSec := float64(totalCPU100ns-prev) / 1e7
				pct = cpuSec / (dt * cores) * 100.0
				if pct < 0 {
					pct = 0
				}
				if pct > 100 {
					pct = 100
				}
			}
			all = append(all, ProcessUsage{
				PID:     pid,
				Name:    name,
				CPUPct:  pct,
				MemoryB: uint64(sp.WorkingSetSize),
			})
		}
		if sp.NextEntryOffset == 0 {
			break
		}
		off += sp.NextEntryOffset
	}
	s.allProcs = all
	s.prevProcs, s.nextProcs = s.nextProcs, s.prevProcs
	s.prevTime = now

	if len(all) == 0 {
		return nil, nil
	}

	sort.Slice(all, func(i, j int) bool { return all[i].CPUPct > all[j].CPUPct })
	nCPU := min(8, len(all))
	topCPU = append([]ProcessUsage(nil), all[:nCPU]...)

	sort.Slice(all, func(i, j int) bool { return all[i].MemoryB > all[j].MemoryB })
	nRAM := min(8, len(all))
	topRAM = append([]ProcessUsage(nil), all[:nRAM]...)
	return topCPU, topRAM
}
