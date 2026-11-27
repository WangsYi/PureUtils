//go:build windows

package collector

import (
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modPDH                        = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQueryW             = modPDH.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounterW     = modPDH.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData       = modPDH.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterArr = modPDH.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery             = modPDH.NewProc("PdhCloseQuery")
)

const (
	pdhFmtDouble  = 0x00000200
	pdhMoreData   = 0x800007D2
	pdhCStatusOK  = 0
	pdhCStatusNew = 1
)

type pdhFmtCounterValueItemW struct {
	SzName      *uint16
	CStatus     uint32
	_           uint32
	DoubleValue float64
}

type pdhSampler struct {
	query        uintptr
	hDiskRead    uintptr
	hDiskWrite   uintptr
	hDiskTime    uintptr
	hGPUEngine   uintptr
	hGPUMemDed   uintptr
	primed       bool
	initAttempt  bool
	rawBuf       []byte
}

type pdhResult struct {
	DiskReadBps  float64
	DiskWriteBps float64
	Disks        []DiskLive
	GPU3DPct     float64
	GPUVideoPct  float64
	GPUCopyPct   float64
	VRAMUsedMB   float64
	PerPIDGPU    map[uint32]PIDGPUUsage
}

func newPDHSampler() *pdhSampler {
	return &pdhSampler{}
}

func (p *pdhSampler) ensureInit() bool {
	if p.query != 0 {
		return true
	}
	if p.initAttempt {
		return false
	}
	p.initAttempt = true
	if procPdhOpenQueryW.Find() != nil || procPdhAddEnglishCounterW.Find() != nil {
		return false
	}
	var q uintptr
	r, _, _ := procPdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&q)))
	if r != 0 || q == 0 {
		return false
	}
	addCounter := func(path string) uintptr {
		ptr, _ := windows.UTF16PtrFromString(path)
		var h uintptr
		cr, _, _ := procPdhAddEnglishCounterW.Call(q, uintptr(unsafe.Pointer(ptr)), 0, uintptr(unsafe.Pointer(&h)))
		if cr != 0 {
			return 0
		}
		return h
	}
	p.query = q
	p.hDiskRead = addCounter(`\PhysicalDisk(*)\Disk Read Bytes/sec`)
	p.hDiskWrite = addCounter(`\PhysicalDisk(*)\Disk Write Bytes/sec`)
	p.hDiskTime = addCounter(`\PhysicalDisk(*)\% Disk Time`)
	p.hGPUEngine = addCounter(`\GPU Engine(*)\Utilization Percentage`)
	p.hGPUMemDed = addCounter(`\GPU Adapter Memory(*)\Dedicated Usage`)

	cr, _, _ := procPdhCollectQueryData.Call(q)
	if cr == 0 {
		p.primed = false // rate counters need a second collection
	}
	return true
}

func (p *pdhSampler) close() {
	if p.query != 0 {
		procPdhCloseQuery.Call(p.query)
		p.query = 0
	}
}

func (p *pdhSampler) sample() pdhResult {
	var out pdhResult
	if !p.ensureInit() {
		return out
	}
	r, _, _ := procPdhCollectQueryData.Call(p.query)
	if r != 0 {
		return out
	}
	if !p.primed {
		p.primed = true
		return out
	}

	diskMap := map[int]*DiskLive{}
	getDisk := func(idx int, name string) *DiskLive {
		if d, ok := diskMap[idx]; ok {
			return d
		}
		d := &DiskLive{Index: idx, Name: name}
		diskMap[idx] = d
		return d
	}

	p.forEachDoubleArray(p.hDiskRead, false, func(name string, val float64) {
		if strings.EqualFold(name, "_Total") {
			out.DiskReadBps = val
			return
		}
		if idx, label, ok := parsePhysicalDiskInstance(name); ok {
			getDisk(idx, label).ReadBps = val
		}
	})
	p.forEachDoubleArray(p.hDiskWrite, false, func(name string, val float64) {
		if strings.EqualFold(name, "_Total") {
			out.DiskWriteBps = val
			return
		}
		if idx, label, ok := parsePhysicalDiskInstance(name); ok {
			getDisk(idx, label).WriteBps = val
		}
	})
	p.forEachDoubleArray(p.hDiskTime, false, func(name string, val float64) {
		if strings.EqualFold(name, "_Total") {
			return
		}
		if idx, label, ok := parsePhysicalDiskInstance(name); ok {
			v := val
			if v > 100 {
				v = 100
			}
			getDisk(idx, label).ActivePct = v
		}
	})

	if len(diskMap) > 0 {
		for i := 0; i < 16; i++ {
			if d, ok := diskMap[i]; ok {
				out.Disks = append(out.Disks, *d)
			}
		}
	}

	var perPID map[uint32]PIDGPUUsage
	p.forEachDoubleArray(p.hGPUEngine, true, func(name string, val float64) {
		pid, engType := parseGPUEngineInstance(name)
		switch engType {
		case "3D":
			out.GPU3DPct += val
			if pid > 0 {
				if perPID == nil {
					perPID = make(map[uint32]PIDGPUUsage, 8)
				}
				u := perPID[pid]
				u.Util3D += val
				perPID[pid] = u
			}
		case "VideoDecode", "VideoEncode", "VideoProcessing":
			out.GPUVideoPct += val
			if pid > 0 {
				if perPID == nil {
					perPID = make(map[uint32]PIDGPUUsage, 8)
				}
				u := perPID[pid]
				u.UtilVideo += val
				perPID[pid] = u
			}
		case "Copy":
			out.GPUCopyPct += val
		}
	})
	if out.GPU3DPct > 100 {
		out.GPU3DPct = 100
	}
	if out.GPUVideoPct > 100 {
		out.GPUVideoPct = 100
	}
	if out.GPUCopyPct > 100 {
		out.GPUCopyPct = 100
	}
	if len(perPID) > 0 {
		out.PerPIDGPU = perPID
	}

	p.forEachDoubleArray(p.hGPUMemDed, true, func(_ string, val float64) {
		mb := val / (1024 * 1024)
		if mb > out.VRAMUsedMB {
			out.VRAMUsedMB = mb
		}
	})
	return out
}

func (p *pdhSampler) forEachDoubleArray(counter uintptr, positiveOnly bool, fn func(name string, val float64)) {
	if counter == 0 {
		return
	}
	for attempt := 0; attempt < 2; attempt++ {
		var bufSize, itemCount uint32
		r, _, _ := procPdhGetFormattedCounterArr.Call(
			counter,
			pdhFmtDouble,
			uintptr(unsafe.Pointer(&bufSize)),
			uintptr(unsafe.Pointer(&itemCount)),
			0,
		)
		if r != pdhMoreData || bufSize == 0 {
			return
		}
		if uint32(cap(p.rawBuf)) < bufSize {
			p.rawBuf = make([]byte, bufSize)
		} else {
			p.rawBuf = p.rawBuf[:bufSize]
		}
		r, _, _ = procPdhGetFormattedCounterArr.Call(
			counter,
			pdhFmtDouble,
			uintptr(unsafe.Pointer(&bufSize)),
			uintptr(unsafe.Pointer(&itemCount)),
			uintptr(unsafe.Pointer(&p.rawBuf[0])),
		)
		if r == pdhMoreData {
			continue
		}
		if r != 0 || itemCount == 0 {
			return
		}
		elemSize := unsafe.Sizeof(pdhFmtCounterValueItemW{})
		if uintptr(itemCount)*elemSize > uintptr(len(p.rawBuf)) {
			return
		}
		items := unsafe.Slice((*pdhFmtCounterValueItemW)(unsafe.Pointer(&p.rawBuf[0])), int(itemCount))
		for _, it := range items {
			if it.CStatus != pdhCStatusOK && it.CStatus != pdhCStatusNew {
				continue
			}
			if it.SzName == nil {
				continue
			}
			v := it.DoubleValue
			if v < 0 {
				v = 0
			}
			if positiveOnly && v <= 0 {
				continue
			}
			fn(windows.UTF16PtrToString(it.SzName), v)
		}
		return
	}
}

func parsePhysicalDiskInstance(inst string) (int, string, bool) {
	inst = strings.TrimSpace(inst)
	if inst == "" {
		return 0, "", false
	}
	fields := strings.SplitN(inst, " ", 2)
	idx, err := strconv.Atoi(fields[0])
	if err != nil || idx < 0 {
		return 0, "", false
	}
	return idx, inst, true
}

// parseGPUEngineInstance parses PDH instances like "pid_1234_luid_0x0_0x1234_phys_0_eng_0_engtype_3D".
func parseGPUEngineInstance(inst string) (pid uint32, engType string) {
	if rest, ok := strings.CutPrefix(inst, "pid_"); ok {
		if us := strings.IndexByte(rest, '_'); us > 0 {
			if n, err := strconv.ParseUint(rest[:us], 10, 32); err == nil {
				pid = uint32(n)
			}
		}
	}
	if _, after, ok := strings.Cut(inst, "_engtype_"); ok {
		engType = after
	}
	return pid, engType
}
