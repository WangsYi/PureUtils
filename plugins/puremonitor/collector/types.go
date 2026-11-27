package collector

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"pureutils/pkg/purekit/i18n"
)

// MetricID identifies one displayable live metric across taskbar, floating, and HUD views.
type MetricID string

const (
	MetricCPU       MetricID = "cpu"
	MetricCPUFreq   MetricID = "cpu_freq"
	MetricCPUTemp   MetricID = "cpu_temp"
	MetricCPUPower  MetricID = "cpu_power"
	MetricRAM       MetricID = "ram"
	MetricRAMGB     MetricID = "ram_gb"
	MetricGPU       MetricID = "gpu"
	MetricGPUTemp   MetricID = "gpu_temp"
	MetricGPUPower  MetricID = "gpu_power"
	MetricGPUFan    MetricID = "gpu_fan"
	MetricVRAM      MetricID = "vram"
	MetricNetDown   MetricID = "net_down"
	MetricNetUp     MetricID = "net_up"
	MetricDiskRead  MetricID = "disk_read"
	MetricDiskWrite MetricID = "disk_write"
	MetricDiskTemp  MetricID = "disk_temp"
	MetricBattery   MetricID = "battery"
	MetricFPS       MetricID = "fps"
	MetricFPSAvg    MetricID = "fps_avg"
	MetricFPS1Low   MetricID = "fps_1low"
	MetricFPS01Low  MetricID = "fps_01low"
	MetricFrametime MetricID = "frametime"
)

// MetricMeta describes a metric's identifier, short tag, localized name, and category.
type MetricMeta struct {
	ID       MetricID
	Short    string
	Label    string
	Category string // "system" | "game"
}

// AllMetrics returns metadata for every supported display metric in canonical order.
func AllMetrics() []MetricMeta {
	return []MetricMeta{
		{MetricCPU, "CPU", i18n.S("CPU 占用", "CPU Load"), "system"},
		{MetricCPUFreq, "CLK", i18n.S("CPU 频率", "CPU Clock"), "system"},
		{MetricCPUTemp, "CTMP", i18n.S("CPU 温度", "CPU Temp"), "system"},
		{MetricCPUPower, "CPWR", i18n.S("CPU 功耗", "CPU Power"), "system"},
		{MetricRAM, "RAM", i18n.S("内存占用", "RAM Load"), "system"},
		{MetricRAMGB, "MEM", i18n.S("已用内存", "RAM Used"), "system"},
		{MetricGPU, "GPU", i18n.S("GPU 占用", "GPU Load"), "system"},
		{MetricGPUTemp, "GTMP", i18n.S("GPU 温度", "GPU Temp"), "system"},
		{MetricGPUPower, "GPWR", i18n.S("GPU 功耗", "GPU Power"), "system"},
		{MetricGPUFan, "GFAN", i18n.S("GPU 风扇", "GPU Fan"), "system"},
		{MetricVRAM, "VRAM", i18n.S("显存占用", "VRAM Used"), "system"},
		{MetricNetDown, "↓", i18n.S("下行网速", "Net Down"), "system"},
		{MetricNetUp, "↑", i18n.S("上行网速", "Net Up"), "system"},
		{MetricDiskRead, "R", i18n.S("磁盘读取", "Disk Read"), "system"},
		{MetricDiskWrite, "W", i18n.S("磁盘写入", "Disk Write"), "system"},
		{MetricDiskTemp, "DTMP", i18n.S("硬盘温度", "Disk Temp"), "system"},
		{MetricBattery, "BAT", i18n.S("电池电量", "Battery"), "system"},
		{MetricFPS, "FPS", i18n.S("实时帧率", "FPS"), "game"},
		{MetricFPSAvg, "AVG", i18n.S("平均帧率", "Avg FPS"), "game"},
		{MetricFPS1Low, "1%L", i18n.S("1% Low 帧", "1% Low FPS"), "game"},
		{MetricFPS01Low, ".1%L", i18n.S("0.1% Low 帧", "0.1% Low FPS"), "game"},
		{MetricFrametime, "FT", i18n.S("帧时间", "Frame Time"), "game"},
	}
}

// MetricShortLabel returns a compact display prefix for metric id.
func MetricShortLabel(id MetricID) string {
	for _, m := range AllMetrics() {
		if m.ID == id {
			return m.Short
		}
	}
	return strings.ToUpper(string(id))
}

// MetricLocalizedLabel returns a localized UI label for metric id.
func MetricLocalizedLabel(id MetricID) string {
	for _, m := range AllMetrics() {
		if m.ID == id {
			return m.Label
		}
	}
	return string(id)
}

// OSInfo describes the host Windows edition and build.
type OSInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Build   string `json:"build"`
	Arch    string `json:"arch"`
}

// SystemInfo describes chassis/OEM info from SMBIOS Type 1.
type SystemInfo struct {
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
}

// MotherboardInfo describes baseboard and BIOS metadata from SMBIOS Type 0/2.
type MotherboardInfo struct {
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
	BIOSVendor   string `json:"biosVendor"`
	BIOSVersion  string `json:"biosVersion"`
	BIOSDate     string `json:"biosDate"`
}

// CPUHardwareInfo describes static CPU topology and cache capacities.
type CPUHardwareInfo struct {
	Name          string `json:"name"`
	Vendor        string `json:"vendor"`
	Socket        string `json:"socket"`
	PhysicalCores int    `json:"physicalCores"`
	LogicalCores  int    `json:"logicalCores"`
	PCores        int    `json:"pCores"`
	ECores        int    `json:"eCores"`
	BaseClockMHz  int    `json:"baseClockMhz"`
	MaxClockMHz   int    `json:"maxClockMhz"`
	L1CacheKB     int    `json:"l1CacheKb"`
	L2CacheKB     int    `json:"l2CacheKb"`
	L3CacheKB     int    `json:"l3CacheKb"`
}

// GPUHardwareInfo describes one graphics adapter reported by DXGI/Registry.
type GPUHardwareInfo struct {
	Name          string `json:"name"`
	Vendor        string `json:"vendor"`
	VendorID      uint32 `json:"vendorId"`
	DeviceID      uint32 `json:"deviceId"`
	VRAMMB        uint64 `json:"vramMb"`
	DriverVersion string `json:"driverVersion"`
}

// RAMModuleInfo describes one physical DIMM slot from SMBIOS Type 17.
type RAMModuleInfo struct {
	Slot         string `json:"slot"`
	Bank         string `json:"bank"`
	Manufacturer string `json:"manufacturer"`
	PartNumber   string `json:"partNumber"`
	Type         string `json:"type"`
	CapacityMB   uint64 `json:"capacityMb"`
	SpeedMTs     int    `json:"speedMts"`
}

// DiskHardwareInfo describes one physical disk drive.
type DiskHardwareInfo struct {
	Index     int    `json:"index"`
	Model     string `json:"model"`
	Serial    string `json:"serial"`
	BusType   string `json:"busType"`
	MediaType string `json:"mediaType"` // "SSD" | "HDD" | "Removable"
	SizeBytes uint64 `json:"sizeBytes"`
}

// VolumeInfo describes one mounted logical drive volume.
type VolumeInfo struct {
	Drive      string `json:"drive"`
	Label      string `json:"label"`
	FileSystem string `json:"fileSystem"`
	TotalBytes uint64 `json:"totalBytes"`
	FreeBytes  uint64 `json:"freeBytes"`
}

// NetAdapterInfo describes one network interface.
type NetAdapterInfo struct {
	Index       uint32 `json:"index"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MAC         string `json:"mac"`
	SpeedMbps   uint64 `json:"speedMbps"`
	IPv4        string `json:"ipv4"`
	Connected   bool   `json:"connected"`
	Physical    bool   `json:"physical"`
}

// DisplayHardwareInfo describes one attached monitor/display mode.
type DisplayHardwareInfo struct {
	Name       string `json:"name"`
	Adapter    string `json:"adapter"`
	Resolution string `json:"resolution"`
	RefreshHz  int    `json:"refreshHz"`
	Primary    bool   `json:"primary"`
}

// HardwareInventory aggregates full static system & hardware specifications.
type HardwareInventory struct {
	OS          OSInfo                `json:"os"`
	System      SystemInfo            `json:"system"`
	Motherboard MotherboardInfo       `json:"motherboard"`
	CPU         CPUHardwareInfo       `json:"cpu"`
	GPUs        []GPUHardwareInfo     `json:"gpus"`
	RAMModules  []RAMModuleInfo       `json:"ramModules"`
	TotalRAMMB  uint64                `json:"totalRamMb"`
	Disks       []DiskHardwareInfo    `json:"disks"`
	Volumes     []VolumeInfo          `json:"volumes"`
	NetAdapters []NetAdapterInfo      `json:"netAdapters"`
	Displays    []DisplayHardwareInfo `json:"displays"`
	CollectedAt time.Time             `json:"collectedAt"`
}

// GPULive holds live utilization and thermal/power telemetry for one GPU.
type GPULive struct {
	Name         string  `json:"name"`
	Vendor       string  `json:"vendor"`
	Load3D       float64 `json:"load3d"`
	LoadVideo    float64 `json:"loadVideo"`
	LoadCopy     float64 `json:"loadCopy"`
	VRAMUsedMB   float64 `json:"vramUsedMb"`
	VRAMTotalMB  float64 `json:"vramTotalMb"`
	TempC        float64 `json:"tempC"`
	HotspotC     float64 `json:"hotspotC"`
	PowerW       float64 `json:"powerW"`
	FanPct       float64 `json:"fanPct"`
	FanRPM       int     `json:"fanRpm"`
	CoreClockMHz float64 `json:"coreClockMhz"`
	MemClockMHz  float64 `json:"memClockMhz"`
	Source       string  `json:"source"` // "PDH" | "NVML" | "ADL" | "IGCL"
}

// DiskLive holds live throughput and optional SMART temperature for one disk.
type DiskLive struct {
	Index     int     `json:"index"`
	Name      string  `json:"name"`
	ReadBps   float64 `json:"readBps"`
	WriteBps  float64 `json:"writeBps"`
	ActivePct float64 `json:"activePct"`
	TempC     float64 `json:"tempC"`
	WarnTempC float64 `json:"warnTempC"`
	CritTempC float64 `json:"critTempC"`
}

// BatteryLive holds laptop battery status.
type BatteryLive struct {
	Present   bool `json:"present"`
	Percent   int  `json:"percent"`
	Charging  bool `json:"charging"`
	ACOnline  bool `json:"acOnline"`
	RemainMin int  `json:"remainMin"`
}

// ProcessUsage summarizes CPU/RAM consumption for one running process.
type ProcessUsage struct {
	PID     uint32  `json:"pid"`
	Name    string  `json:"name"`
	CPUPct  float64 `json:"cpuPct"`
	MemoryB uint64  `json:"memoryB"`
}

// PIDGPUUsage stores per-process GPU engine usage parsed from PDH GPU Engine counters.
type PIDGPUUsage struct {
	Util3D    float64 `json:"util3d"`
	UtilVideo float64 `json:"utilVideo"`
}

// SensorEntry represents one row in the Sensors monitoring view with session min/max.
type SensorEntry struct {
	ID       string  `json:"id"`
	Category string  `json:"category"` // "CPU" | "GPU" | "Memory" | "Disk" | "System"
	Name     string  `json:"name"`
	Value    float64 `json:"value"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Unit     string  `json:"unit"` // "%" | "°C" | "MHz" | "W" | "RPM" | "MB" | "MB/s"
	Source   string  `json:"source"`
}

// SystemSnapshot holds one full telemetry sample.
type SystemSnapshot struct {
	Timestamp time.Time     `json:"timestamp"`
	Uptime    time.Duration `json:"uptime"`

	CPUPercent     float64   `json:"cpuPercent"`
	PerCorePercent []float64 `json:"perCorePercent"`
	CPUFreqMHz     float64   `json:"cpuFreqMhz"`
	CPUTempC       float64   `json:"cpuTempC"`
	CPUPowerW      float64   `json:"cpuPowerW"`

	RAMUsedBytes     uint64  `json:"ramUsedBytes"`
	RAMTotalBytes    uint64  `json:"ramTotalBytes"`
	RAMPct           float64 `json:"ramPct"`
	CommitUsedBytes  uint64  `json:"commitUsedBytes"`
	CommitTotalBytes uint64  `json:"commitTotalBytes"`

	GPUs []GPULive `json:"gpus"`

	Disks        []DiskLive `json:"disks"`
	DiskReadBps  float64    `json:"diskReadBps"`
	DiskWriteBps float64    `json:"diskWriteBps"`

	NetDownBps       float64 `json:"netDownBps"`
	NetUpBps         float64 `json:"netUpBps"`
	NetTotalDown     uint64  `json:"netTotalDown"`
	NetTotalUp       uint64  `json:"netTotalUp"`
	ActiveNetAdapter string  `json:"activeNetAdapter"`

	Battery BatteryLive `json:"battery"`

	ProcessCount uint32         `json:"processCount"`
	ThreadCount  uint32         `json:"threadCount"`
	HandleCount  uint32         `json:"handleCount"`
	TopCPU       []ProcessUsage `json:"topCpu"`
	TopRAM       []ProcessUsage `json:"topRam"`

	PerPIDGPU map[uint32]PIDGPUUsage `json:"perPidGpu,omitempty"`

	FPSActive        bool      `json:"fpsActive"`
	GameName         string    `json:"gameName,omitempty"`
	GamePID          uint32    `json:"gamePid,omitempty"`
	FPS              float64   `json:"fps,omitempty"`
	FPSAvg           float64   `json:"fpsAvg,omitempty"`
	FPS1Low          float64   `json:"fps1Low,omitempty"`
	FPS01Low         float64   `json:"fps01Low,omitempty"`
	FrameTimeMs      float64   `json:"frameTimeMs,omitempty"`
	RecentFrameTimes []float32 `json:"recentFrameTimes,omitempty"`

	Sensors         []SensorEntry `json:"sensors"`
	PawnIOActive    bool          `json:"pawnIoActive"`
	HelperConnected bool          `json:"helperConnected"`
}

// PrimaryGPU returns the best primary GPU from snapshot (preferring discrete/higher load/temp).
func (s SystemSnapshot) PrimaryGPU() (GPULive, bool) {
	if len(s.GPUs) == 0 {
		return GPULive{}, false
	}
	best := s.GPUs[0]
	bestScore := gpuRankScore(best)
	for _, g := range s.GPUs[1:] {
		sc := gpuRankScore(g)
		if sc > bestScore {
			best = g
			bestScore = sc
		}
	}
	return best, true
}

func gpuRankScore(g GPULive) float64 {
	var score float64
	if g.TempC > 0 {
		score += 1000
	}
	if g.VRAMTotalMB >= 2048 {
		score += 500
	}
	if strings.EqualFold(g.Vendor, "NVIDIA") || strings.EqualFold(g.Vendor, "AMD") {
		score += 200
	}
	score += g.Load3D
	return score
}

// MaxDiskTempC returns the highest available disk temperature in Celsius, or 0 if unavailable.
func (s SystemSnapshot) MaxDiskTempC() float64 {
	var maxT float64
	for _, d := range s.Disks {
		if d.TempC > maxT {
			maxT = d.TempC
		}
	}
	return maxT
}

// FormatOptions controls unit rendering for metrics.
type FormatOptions struct {
	UnitSpeed string // "auto" | "mbps"
	TempUnit  string // "C" | "F"
}

// FormatTemp formats a Celsius temperature according to unit ("C" or "F").
func FormatTemp(tempC float64, unit string) string {
	if tempC <= 0 || math.IsNaN(tempC) {
		return "--"
	}
	if strings.EqualFold(strings.TrimSpace(unit), "F") {
		f := tempC*9.0/5.0 + 32.0
		return fmt.Sprintf("%.0f°F", f)
	}
	return fmt.Sprintf("%.0f°C", tempC)
}

// FormatBytesPerSec formats network or disk throughput in B/s or Mbps.
func FormatBytesPerSec(bps float64, mode string) string {
	if bps < 0 || math.IsNaN(bps) {
		bps = 0
	}
	if strings.EqualFold(strings.TrimSpace(mode), "mbps") {
		mbps := bps * 8.0 / 1_000_000.0
		switch {
		case mbps >= 100:
			return fmt.Sprintf("%.0fMb", mbps)
		case mbps >= 10:
			return fmt.Sprintf("%.1fMb", mbps)
		default:
			return fmt.Sprintf("%.2fMb", mbps)
		}
	}
	switch {
	case bps >= 1024*1024*1024:
		return fmt.Sprintf("%.1fG/s", bps/(1024*1024*1024))
	case bps >= 100*1024*1024:
		return fmt.Sprintf("%.0fM/s", bps/(1024*1024))
	case bps >= 1024*1024:
		return fmt.Sprintf("%.1fM/s", bps/(1024*1024))
	case bps >= 10*1024:
		return fmt.Sprintf("%.0fK/s", bps/1024)
	default:
		return fmt.Sprintf("%.1fK/s", bps/1024)
	}
}

// FormatBytes formats an absolute byte count into human-readable units.
func FormatBytes(b uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
		tb = 1024 * gb
	)
	switch {
	case b >= tb:
		return fmt.Sprintf("%.2f TB", float64(b)/float64(tb))
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.0f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.0f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// MetricStatusLevel represents severity ("normal" | "warn" | "crit") for threshold coloring.
type MetricStatusLevel int

const (
	StatusNormal MetricStatusLevel = iota
	StatusWarn
	StatusCrit
)

// MetricAlertThresholds controls when metrics transition to warning/critical colors.
type MetricAlertThresholds struct {
	CPUPct int
	RAMPct int
	TempC  int
}

// FormatMetricValue formats one metric from snap and returns its short label, formatted value, and alert level.
func FormatMetricValue(id MetricID, snap SystemSnapshot, opts FormatOptions, th MetricAlertThresholds) (label string, val string, lvl MetricStatusLevel) {
	label = MetricShortLabel(id)
	warnCPU := float64(th.CPUPct)
	if warnCPU <= 0 {
		warnCPU = 85
	}
	warnRAM := float64(th.RAMPct)
	if warnRAM <= 0 {
		warnRAM = 85
	}
	warnTemp := float64(th.TempC)
	if warnTemp <= 0 {
		warnTemp = 80
	}

	gpu, hasGPU := snap.PrimaryGPU()

	switch id {
	case MetricCPU:
		val = fmt.Sprintf("%.0f%%", snap.CPUPercent)
		lvl = pctLevel(snap.CPUPercent, warnCPU)
	case MetricCPUFreq:
		if snap.CPUFreqMHz <= 0 {
			val = "--"
		} else {
			val = fmt.Sprintf("%.2fG", snap.CPUFreqMHz/1000.0)
		}
	case MetricCPUTemp:
		val = FormatTemp(snap.CPUTempC, opts.TempUnit)
		lvl = tempLevel(snap.CPUTempC, warnTemp)
	case MetricCPUPower:
		if snap.CPUPowerW <= 0 {
			val = "--"
		} else {
			val = fmt.Sprintf("%.0fW", snap.CPUPowerW)
		}
	case MetricRAM:
		val = fmt.Sprintf("%.0f%%", snap.RAMPct)
		lvl = pctLevel(snap.RAMPct, warnRAM)
	case MetricRAMGB:
		usedGB := float64(snap.RAMUsedBytes) / (1024 * 1024 * 1024)
		val = fmt.Sprintf("%.1fG", usedGB)
		lvl = pctLevel(snap.RAMPct, warnRAM)
	case MetricGPU:
		if !hasGPU {
			val = "--"
		} else {
			val = fmt.Sprintf("%.0f%%", gpu.Load3D)
			lvl = pctLevel(gpu.Load3D, warnCPU)
		}
	case MetricGPUTemp:
		if !hasGPU {
			val = "--"
		} else {
			val = FormatTemp(gpu.TempC, opts.TempUnit)
			lvl = tempLevel(gpu.TempC, warnTemp)
		}
	case MetricGPUPower:
		if !hasGPU || gpu.PowerW <= 0 {
			val = "--"
		} else {
			val = fmt.Sprintf("%.0fW", gpu.PowerW)
		}
	case MetricGPUFan:
		if !hasGPU || gpu.FanPct <= 0 {
			val = "--"
		} else {
			val = fmt.Sprintf("%.0f%%", gpu.FanPct)
		}
	case MetricVRAM:
		if !hasGPU || (gpu.VRAMUsedMB <= 0 && gpu.VRAMTotalMB <= 0) {
			val = "--"
		} else {
			val = fmt.Sprintf("%.1fG", gpu.VRAMUsedMB/1024.0)
			if gpu.VRAMTotalMB > 0 {
				lvl = pctLevel(gpu.VRAMUsedMB/gpu.VRAMTotalMB*100.0, warnRAM)
			}
		}
	case MetricNetDown:
		val = FormatBytesPerSec(snap.NetDownBps, opts.UnitSpeed)
	case MetricNetUp:
		val = FormatBytesPerSec(snap.NetUpBps, opts.UnitSpeed)
	case MetricDiskRead:
		val = FormatBytesPerSec(snap.DiskReadBps, "auto")
	case MetricDiskWrite:
		val = FormatBytesPerSec(snap.DiskWriteBps, "auto")
	case MetricDiskTemp:
		t := snap.MaxDiskTempC()
		val = FormatTemp(t, opts.TempUnit)
		lvl = tempLevel(t, warnTemp-15)
	case MetricBattery:
		if !snap.Battery.Present {
			val = "--"
		} else {
			val = fmt.Sprintf("%d%%", snap.Battery.Percent)
			if !snap.Battery.Charging && snap.Battery.Percent <= 20 {
				lvl = StatusWarn
			}
		}
	case MetricFPS:
		if !snap.FPSActive || snap.FPS <= 0 {
			val = "--"
		} else {
			val = fmt.Sprintf("%.0f", snap.FPS)
			lvl = fpsLevel(snap.FPS)
		}
	case MetricFPSAvg:
		if !snap.FPSActive || snap.FPSAvg <= 0 {
			val = "--"
		} else {
			val = fmt.Sprintf("%.0f", snap.FPSAvg)
			lvl = fpsLevel(snap.FPSAvg)
		}
	case MetricFPS1Low:
		if !snap.FPSActive || snap.FPS1Low <= 0 {
			val = "--"
		} else {
			val = fmt.Sprintf("%.0f", snap.FPS1Low)
			lvl = fpsLevel(snap.FPS1Low)
		}
	case MetricFPS01Low:
		if !snap.FPSActive || snap.FPS01Low <= 0 {
			val = "--"
		} else {
			val = fmt.Sprintf("%.0f", snap.FPS01Low)
			lvl = fpsLevel(snap.FPS01Low)
		}
	case MetricFrametime:
		if !snap.FPSActive || snap.FrameTimeMs <= 0 {
			val = "--"
		} else {
			val = fmt.Sprintf("%.1fms", snap.FrameTimeMs)
		}
	default:
		val = "--"
	}
	return label, val, lvl
}

func pctLevel(v, warn float64) MetricStatusLevel {
	if v >= math.Min(95, warn+8) {
		return StatusCrit
	}
	if v >= warn {
		return StatusWarn
	}
	return StatusNormal
}

func tempLevel(v, warn float64) MetricStatusLevel {
	if v <= 0 {
		return StatusNormal
	}
	if v >= warn+10 {
		return StatusCrit
	}
	if v >= warn {
		return StatusWarn
	}
	return StatusNormal
}

func fpsLevel(fps float64) MetricStatusLevel {
	if fps <= 0 {
		return StatusNormal
	}
	if fps < 30 {
		return StatusCrit
	}
	if fps < 60 {
		return StatusWarn
	}
	return StatusNormal
}

// HistoryRing maintains fixed-length sample histories for sparklines.
type HistoryRing struct {
	mu       sync.RWMutex
	capacity int
	series   map[string][]float64
}

// NewHistoryRing creates a ring history store keeping up to cap samples per key.
func NewHistoryRing(cap int) *HistoryRing {
	if cap <= 0 {
		cap = 60
	}
	return &HistoryRing{
		capacity: cap,
		series:   make(map[string][]float64),
	}
}

// RecordSnapshot appends the latest snapshot's core series into the ring.
func (h *HistoryRing) RecordSnapshot(s SystemSnapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pushLocked("cpu", s.CPUPercent)
	h.pushLocked("ram", s.RAMPct)
	if g, ok := s.PrimaryGPU(); ok {
		h.pushLocked("gpu", g.Load3D)
		h.pushLocked("gpu_temp", g.TempC)
	} else {
		h.pushLocked("gpu", 0)
		h.pushLocked("gpu_temp", 0)
	}
	h.pushLocked("cpu_temp", s.CPUTempC)
	h.pushLocked("net_down", s.NetDownBps)
	h.pushLocked("net_up", s.NetUpBps)
}

func (h *HistoryRing) pushLocked(key string, v float64) {
	arr := append(h.series[key], v)
	if len(arr) > h.capacity {
		arr = arr[len(arr)-h.capacity:]
	}
	h.series[key] = arr
}

// Samples returns a copy of the stored series for key.
func (h *HistoryRing) Samples(key string) []float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	src := h.series[key]
	if len(src) == 0 {
		return nil
	}
	out := make([]float64, len(src))
	copy(out, src)
	return out
}

// FormatSpecSummary produces a localized, shareable hardware specification text block.
func FormatSpecSummary(inv HardwareInventory) string {
	var b strings.Builder
	b.WriteString(i18n.S("=== PureMonitor 硬件配置单 ===\n", "=== PureMonitor Hardware Specification ===\n"))
	if inv.OS.Name != "" {
		b.WriteString(fmt.Sprintf("%s: %s (%s Build %s)\n", i18n.S("操作系统", "OS"), inv.OS.Name, inv.OS.Arch, inv.OS.Build))
	}
	if inv.System.Manufacturer != "" || inv.System.Model != "" {
		b.WriteString(fmt.Sprintf("%s: %s %s\n", i18n.S("电脑型号", "System"), inv.System.Manufacturer, inv.System.Model))
	}
	if inv.CPU.Name != "" {
		b.WriteString(fmt.Sprintf("%s: %s (%dC/%dT, %d MHz)\n", i18n.S("处理器", "CPU"), inv.CPU.Name, inv.CPU.PhysicalCores, inv.CPU.LogicalCores, inv.CPU.MaxClockMHz))
	}
	if inv.Motherboard.Product != "" || inv.Motherboard.Manufacturer != "" {
		b.WriteString(fmt.Sprintf("%s: %s %s (BIOS %s)\n", i18n.S("主板", "Motherboard"), inv.Motherboard.Manufacturer, inv.Motherboard.Product, inv.Motherboard.BIOSVersion))
	}
	if inv.TotalRAMMB > 0 || len(inv.RAMModules) > 0 {
		b.WriteString(fmt.Sprintf("%s: %.1f GB", i18n.S("内存", "Memory"), float64(inv.TotalRAMMB)/1024.0))
		if len(inv.RAMModules) > 0 {
			m0 := inv.RAMModules[0]
			b.WriteString(fmt.Sprintf(" (%d × %s %d MT/s)", len(inv.RAMModules), m0.Type, m0.SpeedMTs))
		}
		b.WriteByte('\n')
	}
	for i, g := range inv.GPUs {
		b.WriteString(fmt.Sprintf("%s %d: %s", i18n.S("显卡", "GPU"), i+1, g.Name))
		if g.VRAMMB > 0 {
			b.WriteString(fmt.Sprintf(" (%d MB)", g.VRAMMB))
		}
		if g.DriverVersion != "" {
			b.WriteString(fmt.Sprintf(" [%s]", g.DriverVersion))
		}
		b.WriteByte('\n')
	}
	for _, d := range inv.Disks {
		b.WriteString(fmt.Sprintf("%s %d: %s (%s %s, %s)\n", i18n.S("硬盘", "Disk"), d.Index, d.Model, d.BusType, d.MediaType, FormatBytes(d.SizeBytes)))
	}
	for _, mon := range inv.Displays {
		b.WriteString(fmt.Sprintf("%s: %s %s @ %dHz\n", i18n.S("显示器", "Display"), mon.Name, mon.Resolution, mon.RefreshHz))
	}
	return strings.TrimSpace(b.String())
}

// SensorTracker maintains session min/max values across polled SensorEntry slices.
type SensorTracker struct {
	mu    sync.Mutex
	stats map[string][2]float64 // id -> [min, max]
}

// NewSensorTracker creates a session min/max sensor tracker.
func NewSensorTracker() *SensorTracker {
	return &SensorTracker{stats: make(map[string][2]float64)}
}

// Track updates min/max values on entries in-place.
func (st *SensorTracker) Track(entries []SensorEntry) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i := range entries {
		id := entries[i].ID
		val := entries[i].Value
		if math.IsNaN(val) {
			continue
		}
		prev, ok := st.stats[id]
		if !ok {
			prev = [2]float64{val, val}
		} else {
			if val < prev[0] {
				prev[0] = val
			}
			if val > prev[1] {
				prev[1] = val
			}
		}
		st.stats[id] = prev
		entries[i].Min = prev[0]
		entries[i].Max = prev[1]
	}
}
