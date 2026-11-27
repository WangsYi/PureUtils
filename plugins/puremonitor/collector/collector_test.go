package collector

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pureutils/pkg/purekit/i18n"
)

func TestFormattingAndHistoryRing(t *testing.T) {
	if got := FormatTemp(68.4, "C"); got != "68°C" {
		t.Fatalf("FormatTemp C = %q", got)
	}
	if got := FormatTemp(100, "F"); got != "212°F" {
		t.Fatalf("FormatTemp F = %q", got)
	}
	if got := FormatTemp(0, "C"); got != "--" {
		t.Fatalf("FormatTemp 0 = %q", got)
	}

	if got := FormatBytesPerSec(1500*1024, "auto"); got != "1.5M/s" {
		t.Fatalf("FormatBytesPerSec auto = %q", got)
	}
	if got := FormatBytesPerSec(12_500_000, "mbps"); got != "100Mb" {
		t.Fatalf("FormatBytesPerSec mbps = %q", got)
	}

	snap := SystemSnapshot{
		CPUPercent: 88,
		CPUTempC:   76,
		RAMPct:     62,
		GPUs: []GPULive{
			{Name: "NVIDIA GeForce RTX 4070", Vendor: "NVIDIA", Load3D: 94, TempC: 83, VRAMUsedMB: 6144, VRAMTotalMB: 12288},
		},
		FPSActive: true,
		FPS:       144,
		FPS1Low:   98,
	}
	lbl, val, lvl := FormatMetricValue(MetricCPU, snap, FormatOptions{UnitSpeed: "auto", TempUnit: "C"}, MetricAlertThresholds{CPUPct: 85, RAMPct: 85, TempC: 80})
	if lbl != "CPU" || val != "88%" || lvl != StatusWarn {
		t.Fatalf("CPU format = (%q, %q, %v)", lbl, val, lvl)
	}
	_, gVal, gLvl := FormatMetricValue(MetricGPUTemp, snap, FormatOptions{TempUnit: "C"}, MetricAlertThresholds{TempC: 80})
	if gVal != "83°C" || gLvl != StatusWarn {
		t.Fatalf("GPU temp = (%q, %v)", gVal, gLvl)
	}

	ring := NewHistoryRing(3)
	for i := 1; i <= 5; i++ {
		ring.RecordSnapshot(SystemSnapshot{CPUPercent: float64(i * 10)})
	}
	samples := ring.Samples("cpu")
	if len(samples) != 3 || samples[0] != 30 || samples[2] != 50 {
		t.Fatalf("HistoryRing samples = %v", samples)
	}
}

func TestPawnIODecodingAndInputPacking(t *testing.T) {
	in := BuildPawnIOExecuteInput("ioctl_read_msr", []uint64{0x1B1})
	if len(in) != 40 {
		t.Fatalf("BuildPawnIOExecuteInput len = %d, want 40", len(in))
	}
	if got := string(in[:14]); got != "ioctl_read_msr" {
		t.Fatalf("fn name = %q", got)
	}
	if binary.LittleEndian.Uint64(in[32:40]) != 0x1B1 {
		t.Fatalf("arg[0] = %#x", binary.LittleEndian.Uint64(in[32:40]))
	}

	tj := DecodeIntelTjMax(uint64(105) << 16)
	if tj != 105 {
		t.Fatalf("TjMax = %v, want 105", tj)
	}
	msr := (uint64(1) << 31) | (uint64(32) << 16) // valid=1, delta=32 -> 105-32 = 73C
	temp, ok := DecodeIntelThermTemp(msr, tj)
	if !ok || temp != 73 {
		t.Fatalf("Intel temp = (%v, %v), want (73, true)", temp, ok)
	}

	// AMD Zen SMN: curTmp = 62.5 C (62.5 / 0.125 = 500) -> 500 << 21
	smn := uint64(500) << 21
	amdT, ok := DecodeAMDZenTemp(smn)
	if !ok || amdT != 62.5 {
		t.Fatalf("AMD temp = (%v, %v), want (62.5, true)", amdT, ok)
	}
	// With -49C offset bit 19: (111.5 / 0.125 = 892) -> 892<<21 | 1<<19 => 62.5
	smnOff := (uint64(892) << 21) | (1 << 19)
	amdOffT, ok := DecodeAMDZenTemp(smnOff)
	if !ok || amdOffT != 62.5 {
		t.Fatalf("AMD offset temp = (%v, %v), want (62.5, true)", amdOffT, ok)
	}

	// RAPL power: unit shift 14 => 1/16384 J/unit. Delta = 1638400 units over 1s => 100W.
	unit := DecodeEnergyUnitJ(uint64(14) << 8)
	watts := ComputeRAPLPowerW(0xFFFFFF00, 0xFFFFFF00+1638400, unit, 1.0)
	if watts < 99.9 || watts > 100.1 {
		t.Fatalf("RAPL wrap watts = %v, want 100", watts)
	}
}

func TestSensorDownloaderMirrorsAndLocalImport(t *testing.T) {
	cands := buildSensorMirrorCandidates(SensorSourceGHFast, "https://my.mirror/{url}", "https://example.com/a.exe")
	if len(cands) != 4 || cands[0].URL != "https://my.mirror/https://example.com/a.exe" || cands[1].SourceID != SensorSourceGHFast {
		t.Fatalf("mirror candidates = %+v", cands)
	}

	// Mock HTTP server serving a fake MZ executable and a zip containing IntelMSR.bin
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/setup.exe"):
			exe := make([]byte, 2048)
			exe[0], exe[1] = 'M', 'Z'
			_, _ = w.Write(exe)
		case strings.HasSuffix(r.URL.Path, "/mods.zip"):
			zw := zip.NewWriter(w)
			f, _ := zw.Create("IntelMSR.bin")
			_, _ = f.Write([]byte("PAWN_MOD_INTEL_MSR_TEST_DATA"))
			_ = zw.Close()
		case strings.HasSuffix(r.URL.Path, "/setup-api"):
			_, _ = w.Write([]byte(`{"assets":[{"name":"PawnIO_setup.exe","browser_download_url":"` + srvURL + `/setup.exe"}]}`))
		case strings.HasSuffix(r.URL.Path, "/mods-api"):
			_, _ = w.Write([]byte(`{"assets":[{"name":"PawnIO.Modules.zip","browser_download_url":"` + srvURL + `/mods.zip"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	pluginDir := t.TempDir()
	mgr := NewSensorExtManager(pluginDir)
	mgr.setupAPIURL = srvURL + "/setup-api"
	mgr.modulesAPIURL = srvURL + "/mods-api"
	mgr.mirrorPrefixes = map[string]string{
		SensorSourceGHProxy: "",
		SensorSourceGHFast:  "",
	}
	mgr.driverCheckFn = func() bool { return false }

	if err := mgr.Download(context.Background(), SensorSourceGitHub, ""); err != nil {
		t.Fatalf("Download: %v", err)
	}
	st := mgr.Status()
	if !st.SetupDownloaded || !st.ModulesReady || st.ModuleCount != 1 {
		t.Fatalf("Status after download = %+v", st)
	}

	// Local .bin import
	extraBin := filepath.Join(t.TempDir(), "AMDFamily17.bin")
	if err := os.WriteFile(extraBin, []byte("PAWN_MOD_AMD_FAM17_TEST"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ImportLocal(extraBin); err != nil {
		t.Fatalf("ImportLocal: %v", err)
	}
	if st2 := mgr.Status(); st2.ModuleCount != 2 {
		t.Fatalf("ModuleCount after ImportLocal = %d, want 2", st2.ModuleCount)
	}
	if err := mgr.RemoveLocalFiles(); err != nil {
		t.Fatal(err)
	}
	if st3 := mgr.Status(); st3.SetupDownloaded || st3.ModulesReady {
		t.Fatalf("expected clean state after RemoveLocalFiles, got %+v", st3)
	}
}

func TestFormatSpecSummaryContainsKeyFields(t *testing.T) {
	inv := HardwareInventory{
		OS:          OSInfo{Name: "Windows 11 Pro", Build: "26300", Arch: "x64"},
		System:      SystemInfo{Manufacturer: "ASUSTeK", Model: "ROG Hero"},
		CPU:         CPUHardwareInfo{Name: "AMD Ryzen 7 7800X3D", PhysicalCores: 8, LogicalCores: 16, MaxClockMHz: 5050},
		Motherboard: MotherboardInfo{Manufacturer: "MSI", Product: "B650 TOMAHAWK", BIOSVersion: "1.A0"},
		TotalRAMMB:  32768,
		RAMModules:  []RAMModuleInfo{{Type: "DDR5", SpeedMTs: 6000, CapacityMB: 16384}, {Type: "DDR5", SpeedMTs: 6000, CapacityMB: 16384}},
		GPUs:        []GPUHardwareInfo{{Name: "NVIDIA GeForce RTX 4080", VRAMMB: 16384, DriverVersion: "566.14"}},
		Disks:       []DiskHardwareInfo{{Index: 0, Model: "WD Black SN850X", BusType: "NVMe", MediaType: "SSD", SizeBytes: 2_000_000_000_000}},
		Displays:    []DisplayHardwareInfo{{Name: "LG OLED", Resolution: "3840x2160", RefreshHz: 120}},
		CollectedAt: time.Now(),
	}
	txt := FormatSpecSummary(inv)
	for _, sub := range []string{"Windows 11 Pro", "7800X3D", "B650 TOMAHAWK", "DDR5", "RTX 4080", "566.14", "SN850X", "3840x2160"} {
		if !strings.Contains(txt, sub) {
			t.Fatalf("FormatSpecSummary missing %q:\n%s", sub, txt)
		}
	}
}

func TestTypesFormattingAllMetricsAndHelpers(t *testing.T) {
	rich := SystemSnapshot{
		CPUPercent:   96, // crit
		CPUFreqMHz:   5200,
		CPUTempC:     92, // crit
		CPUPowerW:    140,
		RAMPct:       88, // warn
		RAMUsedBytes: 24 * 1024 * 1024 * 1024,
		GPUs: []GPULive{
			{Name: "Intel UHD", Vendor: "Intel", Load3D: 5},
			{Name: "NVIDIA RTX 4080", Vendor: "NVIDIA", Load3D: 90, LoadVideo: 10, VRAMUsedMB: 10240, VRAMTotalMB: 16384, TempC: 82, PowerW: 250, FanPct: 65},
		},
		Disks:        []DiskLive{{Index: 0, TempC: 39}, {Index: 1, TempC: 68}}, // warn
		DiskReadBps:  200 * 1024 * 1024,
		DiskWriteBps: 50 * 1024 * 1024,
		NetDownBps:   12_500_000,
		NetUpBps:     1_500_000,
		Battery:      BatteryLive{Present: true, Percent: 15, Charging: false},
		FPSActive:    true,
		FPS:          25,  // crit < 30
		FPSAvg:       48,  // warn < 60
		FPS1Low:      65,  // normal >= 60
		FPS01Low:     28,
		FrameTimeMs:  16.7,
	}
	empty := SystemSnapshot{}
	th := MetricAlertThresholds{CPUPct: 85, RAMPct: 85, TempC: 80}

	for _, meta := range AllMetrics() {
		lbl, val, _ := FormatMetricValue(meta.ID, rich, FormatOptions{UnitSpeed: "auto", TempUnit: "F"}, th)
		if lbl == "" || val == "" {
			t.Fatalf("rich FormatMetricValue(%s) empty: (%q, %q)", meta.ID, lbl, val)
		}
		_, valEmpty, _ := FormatMetricValue(meta.ID, empty, FormatOptions{UnitSpeed: "mbps", TempUnit: "C"}, MetricAlertThresholds{})
		if valEmpty == "" {
			t.Fatalf("empty FormatMetricValue(%s) empty", meta.ID)
		}
	}
	if _, val, _ := FormatMetricValue(MetricID("unknown_metric"), rich, FormatOptions{}, th); val != "--" {
		t.Fatalf("unknown metric val = %q, want --", val)
	}
	if MetricShortLabel("custom_id") != "CUSTOM_ID" {
		t.Fatalf("MetricShortLabel(custom_id) = %q", MetricShortLabel("custom_id"))
	}
	if MetricLocalizedLabel(MetricCPU) == "" || MetricLocalizedLabel("missing") != "missing" {
		t.Fatal("MetricLocalizedLabel mismatch")
	}
	if g, ok := rich.PrimaryGPU(); !ok || g.Vendor != "NVIDIA" {
		t.Fatalf("PrimaryGPU = (%+v, %v), want NVIDIA discrete", g, ok)
	}
	if _, ok := empty.PrimaryGPU(); ok {
		t.Fatal("empty PrimaryGPU should return false")
	}
	if got := rich.MaxDiskTempC(); got != 68 {
		t.Fatalf("MaxDiskTempC = %v, want 68", got)
	}
	if got := empty.MaxDiskTempC(); got != 0 {
		t.Fatalf("empty MaxDiskTempC = %v, want 0", got)
	}

	// FormatBytes and FormatBytesPerSec across all magnitude ranges
	for _, b := range []uint64{42, 4096, 8 * 1024 * 1024, 5 * 1024 * 1024 * 1024, 3 * 1024 * 1024 * 1024 * 1024} {
		if s := FormatBytes(b); s == "" {
			t.Fatalf("FormatBytes(%d) empty", b)
		}
	}
	for _, bps := range []float64{-1, math.NaN(), 512, 20 * 1024, 5 * 1024 * 1024, 200 * 1024 * 1024, 2 * 1024 * 1024 * 1024} {
		if s := FormatBytesPerSec(bps, "auto"); s == "" {
			t.Fatalf("FormatBytesPerSec auto(%v) empty", bps)
		}
		if s := FormatBytesPerSec(bps, "mbps"); s == "" {
			t.Fatalf("FormatBytesPerSec mbps(%v) empty", bps)
		}
	}
	if FormatTemp(math.NaN(), "C") != "--" {
		t.Fatal("NaN FormatTemp should be --")
	}
	if fpsLevel(0) != StatusNormal || fpsLevel(25) != StatusCrit || fpsLevel(50) != StatusWarn || fpsLevel(100) != StatusNormal {
		t.Fatal("fpsLevel mismatch")
	}

	// HistoryRing default cap and SensorTracker min/max + NaN guard
	defRing := NewHistoryRing(0)
	if defRing.capacity != 60 || defRing.Samples("none") != nil {
		t.Fatalf("HistoryRing defaults mismatch: cap=%d", defRing.capacity)
	}
	defRing.RecordSnapshot(empty)
	if len(defRing.Samples("gpu")) != 1 {
		t.Fatal("expected empty RecordSnapshot to push 0 for gpu")
	}

	tracker := NewSensorTracker()
	entries := []SensorEntry{{ID: "cpu.temp", Value: 50}, {ID: "bad", Value: math.NaN()}}
	tracker.Track(entries)
	entries[0].Value = 40
	tracker.Track(entries)
	entries[0].Value = 75
	tracker.Track(entries)
	if entries[0].Min != 40 || entries[0].Max != 75 {
		t.Fatalf("SensorTracker min/max = (%v, %v), want (40, 75)", entries[0].Min, entries[0].Max)
	}
}

func TestPawnIOEdgeCases(t *testing.T) {
	longFn := strings.Repeat("x", 45)
	buf := BuildPawnIOExecuteInput(longFn, nil)
	if len(buf) != 32 || buf[31] != 0 {
		t.Fatalf("truncated fn buf len=%d last=%d", len(buf), buf[31])
	}

	if DecodeIntelTjMax(uint64(10)<<16) != 100 || DecodeIntelTjMax(uint64(200)<<16) != 100 {
		t.Fatal("DecodeIntelTjMax out-of-range should default to 100")
	}
	if _, ok := DecodeIntelThermTemp(0, 100); ok {
		t.Fatal("bit 31 unset should return false")
	}
	if _, ok := DecodeIntelThermTemp((1<<31)|(120<<16), 0); ok {
		t.Fatal("negative derived temp should return false")
	}
	if _, ok := DecodeAMDZenTemp(0); ok {
		t.Fatal("0C AMD Zen reading should return false")
	}
	if _, ok := DecodeAMDZenTemp(uint64(2000) << 21); ok {
		t.Fatal(">130C AMD Zen reading should return false")
	}
	if DecodeEnergyUnitJ(uint64(31)<<8) != 0 {
		t.Fatal("shift>30 should return 0")
	}
	if ComputeRAPLPowerW(0, 100, 0, 1) != 0 || ComputeRAPLPowerW(0, 100, 0.1, 0.01) != 0 || ComputeRAPLPowerW(0, 10_000_000, 1.0, 1.0) != 0 {
		t.Fatal("ComputeRAPLPowerW bounds guard failed")
	}
}

func TestSensorDownloaderFullFlows(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)
	for _, s := range SensorDownloadSources {
		lbl := SensorSourceLabel(s.ID)
		if SensorSourceIDFromLabel(lbl) != s.ID {
			t.Fatalf("ZH roundtrip(%s) = %q -> %q", s.ID, lbl, SensorSourceIDFromLabel(lbl))
		}
	}
	if len(SensorSourceLabels()) != len(SensorDownloadSources) {
		t.Fatal("SensorSourceLabels len mismatch")
	}
	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)
	for _, s := range SensorDownloadSources {
		lbl := SensorSourceLabel(s.ID)
		if SensorSourceIDFromLabel(lbl) != s.ID {
			t.Fatalf("EN roundtrip(%s) = %q -> %q", s.ID, lbl, SensorSourceIDFromLabel(lbl))
		}
	}
	if SensorSourceIDFromLabel("unknown-label") != SensorSourceAuto {
		t.Fatal("unknown label should map to auto")
	}
	if !isDesiredModuleName("IntelMSR.bin") || !isDesiredModuleName("Custom.amx") || isDesiredModuleName("readme.txt") {
		t.Fatal("isDesiredModuleName mismatch")
	}

	// Empty pluginDir manager error guards
	unconfigured := NewSensorExtManager("")
	if err := unconfigured.Download(context.Background(), SensorSourceAuto, ""); err == nil {
		t.Fatal("expected error downloading without sensorDir")
	}
	if err := unconfigured.ImportLocal("foo.bin"); err == nil {
		t.Fatal("expected error importing without sensorDir")
	}
	_ = unconfigured.RemoveLocalFiles()

	// HTTP server testing fallback (404 API -> fallback fails), mirror failover, .bin direct download, and cancel
	var srvURL string
	blockCh := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/fail/"):
			http.Error(w, "mirror down", http.StatusBadGateway)
		case r.URL.Path == "/setup.exe":
			exe := make([]byte, 2048)
			exe[0], exe[1] = 'M', 'Z'
			_, _ = w.Write(exe)
		case r.URL.Path == "/setup-api-bin":
			_, _ = w.Write([]byte(`{"assets":[{"name":"PawnIO_setup.exe","browser_download_url":"` + srvURL + `/setup.exe"}]}`))
		case r.URL.Path == "/mods-api-bin":
			_, _ = w.Write([]byte(`{"assets":[{"name":"IntelMSR.bin","browser_download_url":"` + srvURL + `/IntelMSR.bin"}]}`))
		case r.URL.Path == "/IntelMSR.bin":
			_, _ = w.Write([]byte("INTEL_MSR_BIN_TEST_PAYLOAD_1234"))
		case r.URL.Path == "/slow-api":
			select {
			case <-r.Context().Done():
			case <-blockCh:
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer func() {
		close(blockCh)
		srv.Close()
	}()
	srvURL = srv.URL

	mgr := NewSensorExtManager(t.TempDir())
	if mgr.SensorDir() == "" || !strings.HasSuffix(mgr.SetupPath(), "PawnIO_setup.exe") {
		t.Fatalf("SensorDir=%q SetupPath=%q", mgr.SensorDir(), mgr.SetupPath())
	}
	var cbCount int
	mgr.SetOnChange(func() { cbCount++ })
	mgr.setupAPIURL = srvURL + "/setup-api-bin"
	mgr.modulesAPIURL = srvURL + "/mods-api-bin"
	mgr.mirrorPrefixes = map[string]string{
		SensorSourceGHProxy: "",
		SensorSourceGHFast:  "",
	}
	mgr.driverCheckFn = func() bool { return false }

	// Pass failing custom mirror URL; Download falls back to GitHub candidate (srvURL/setup.exe) + individual .bin download.
	if err := mgr.Download(context.Background(), SensorSourceAuto, srvURL+"/fail/{url}"); err != nil {
		t.Fatalf("Download with mirror failover + .bin path: %v", err)
	}
	if cbCount == 0 {
		t.Fatal("expected OnChange callbacks during Download")
	}

	// Verify that when PawnIO driver IS already installed locally, Download skips PawnIO_setup.exe entirely.
	mgrDriverReady := NewSensorExtManager(t.TempDir())
	mgrDriverReady.setupAPIURL = srvURL + "/fail/setup-api"
	mgrDriverReady.modulesAPIURL = srvURL + "/mods-api-bin"
	mgrDriverReady.mirrorPrefixes = map[string]string{
		SensorSourceGHProxy: "",
		SensorSourceGHFast:  "",
	}
	mgrDriverReady.driverCheckFn = func() bool { return true }
	if err := mgrDriverReady.Download(context.Background(), SensorSourceGitHub, ""); err != nil {
		t.Fatalf("Download with DriverInstalled=true failed: %v", err)
	}
	stReady := mgrDriverReady.Status()
	if !stReady.DriverInstalled || stReady.SetupDownloaded || !stReady.ModulesReady || stReady.ModuleCount != 1 {
		t.Fatalf("expected setup skipped & module downloaded when driver installed, got %+v", stReady)
	}

	// ImportLocal error cases (.txt, non-MZ .exe, invalid small .bin, empty .zip, valid .zip & valid .exe)
	tmp := t.TempDir()
	badTxt := filepath.Join(tmp, "note.txt")
	_ = os.WriteFile(badTxt, []byte("hello"), 0o600)
	if err := mgr.ImportLocal(badTxt); err == nil {
		t.Fatal("expected error importing .txt")
	}
	badExe := filepath.Join(tmp, "bad.exe")
	_ = os.WriteFile(badExe, make([]byte, 128), 0o600)
	if err := mgr.ImportLocal(badExe); err == nil {
		t.Fatal("expected error importing non-MZ .exe")
	}
	goodExe := filepath.Join(tmp, "good.exe")
	goodBytes := make([]byte, 2048)
	goodBytes[0], goodBytes[1] = 'M', 'Z'
	_ = os.WriteFile(goodExe, goodBytes, 0o600)
	if err := mgr.ImportLocal(goodExe); err != nil {
		t.Fatalf("ImportLocal valid exe: %v", err)
	}
	emptyZip := filepath.Join(tmp, "empty.zip")
	if f, err := os.Create(emptyZip); err == nil {
		zw := zip.NewWriter(f)
		entry, _ := zw.Create("README.md")
		_, _ = entry.Write([]byte("ignore"))
		_ = zw.Close()
		_ = f.Close()
	}
	if err := mgr.ImportLocal(emptyZip); err == nil {
		t.Fatal("expected error importing zip with no modules")
	}

	// StartDownload + CancelDownload
	mgr2 := NewSensorExtManager(t.TempDir())
	mgr2.setupAPIURL = srvURL + "/slow-api"
	mgr2.driverCheckFn = func() bool { return false }
	doneCh := make(chan struct{})
	mgr2.SetOnChange(func() {
		p := mgr2.Progress()
		if !p.Active && p.Err != "" {
			select {
			case <-doneCh:
			default:
				close(doneCh)
			}
		}
	})
	mgr2.StartDownload(SensorSourceGitHub, srvURL+"/slow-api/setup.exe")
	time.Sleep(40 * time.Millisecond)
	mgr2.CancelDownload()
	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatalf("StartDownload cancel did not settle: %+v", mgr2.Progress())
	}
}

func TestSensorExtManagerDownloadAndSilentInstall(t *testing.T) {
	prevPref, prevLang := i18n.Preference(), i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()
	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/setup-api":
			_, _ = w.Write([]byte(`{"assets":[{"name":"PawnIO_setup.exe","browser_download_url":"` + srvURL + `/PawnIO_setup.exe"}]}`))
		case "/mods-api":
			_, _ = w.Write([]byte(`{"assets":[{"name":"IntelMSR.bin","browser_download_url":"` + srvURL + `/IntelMSR.bin"}]}`))
		case "/PawnIO_setup.exe":
			exe := make([]byte, 2048)
			exe[0], exe[1] = 'M', 'Z'
			_, _ = w.Write(exe)
		case "/IntelMSR.bin":
			_, _ = w.Write([]byte("INTEL_MSR_BIN_TEST_PAYLOAD_1234"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	runAndWait := func(mgr *SensorExtManager, label string, trigger func()) SensorDownloadProgress {
		t.Helper()
		ch := make(chan SensorDownloadProgress, 1)
		mgr.SetOnChange(func() {
			if p := mgr.Progress(); !p.Active {
				select {
				case ch <- p:
				default:
				}
			}
		})
		trigger()
		select {
		case p := <-ch:
			return p
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not settle: %+v", label, mgr.Progress())
			return SensorDownloadProgress{}
		}
	}

	// 1. Download + automatic silent install succeeds end-to-end.
	mgr := NewSensorExtManager(t.TempDir())
	mgr.setupAPIURL = srvURL + "/setup-api"
	mgr.modulesAPIURL = srvURL + "/mods-api"
	mgr.mirrorPrefixes = map[string]string{SensorSourceGHProxy: "", SensorSourceGHFast: ""}
	var installed atomic.Bool
	mgr.SetDriverCheckFn(func() bool { return installed.Load() })
	mgr.SetInstallFn(func(_ context.Context, setupPath string) error {
		if !strings.HasSuffix(setupPath, "PawnIO_setup.exe") {
			t.Fatalf("unexpected setupPath passed to installFn: %q", setupPath)
		}
		if err := validateMZHeader(setupPath); err != nil {
			t.Fatalf("invalid installer MZ header: %v", err)
		}
		installed.Store(true)
		return nil
	})
	prog := runAndWait(mgr, "StartDownloadAndInstall", func() {
		mgr.StartDownloadAndInstall(SensorSourceGitHub, "")
	})
	if !prog.Completed || prog.Err != "" || prog.Installing {
		t.Fatalf("expected clean completed install, got %+v", prog)
	}
	st := mgr.Status()
	if !st.DriverInstalled || !st.ModulesReady || !st.SetupDownloaded {
		t.Fatalf("expected DriverInstalled & ModulesReady & SetupDownloaded, got %+v", st)
	}

	// 2. UAC cancellation during download+install preserves SetupDownloaded for one-click retry.
	mgrRetry := NewSensorExtManager(t.TempDir())
	mgrRetry.setupAPIURL = srvURL + "/setup-api"
	mgrRetry.modulesAPIURL = srvURL + "/mods-api"
	mgrRetry.mirrorPrefixes = map[string]string{SensorSourceGHProxy: "", SensorSourceGHFast: ""}
	var installedRetry atomic.Bool
	mgrRetry.SetDriverCheckFn(func() bool { return installedRetry.Load() })
	mgrRetry.SetInstallFn(func(_ context.Context, _ string) error {
		return ErrInstallCanceled
	})
	pCancel := runAndWait(mgrRetry, "StartDownloadAndInstall cancel", func() {
		mgrRetry.StartDownloadAndInstall(SensorSourceGitHub, "")
	})
	if pCancel.Completed || !strings.Contains(pCancel.Err, "Administrator authorization canceled") {
		t.Fatalf("expected UAC cancel error, got %+v", pCancel)
	}
	if stR := mgrRetry.Status(); !stR.SetupDownloaded || !stR.ModulesReady || stR.DriverInstalled {
		t.Fatalf("expected local setup+modules retained after cancel, got %+v", stR)
	}

	// 3. Installer returns 0 but driver stays undetected -> report explicit policy/detection error.
	mgrRetry.SetInstallFn(func(_ context.Context, _ string) error { return nil })
	pUndet := runAndWait(mgrRetry, "StartSilentInstall undetected", func() {
		mgrRetry.StartSilentInstall()
	})
	if pUndet.Completed || !strings.Contains(pUndet.Err, "PawnIO driver was not detected") {
		t.Fatalf("expected driver-not-detected error, got %+v", pUndet)
	}

	// 4. Retry StartSilentInstall with driver registration succeeding.
	mgrRetry.SetInstallFn(func(_ context.Context, _ string) error {
		installedRetry.Store(true)
		return nil
	})
	pRetry := runAndWait(mgrRetry, "StartSilentInstall retry", func() {
		mgrRetry.StartSilentInstall()
	})
	if !pRetry.Completed || pRetry.Err != "" || !mgrRetry.Status().DriverInstalled {
		t.Fatalf("expected silent install retry to succeed, prog=%+v st=%+v", pRetry, mgrRetry.Status())
	}
}
