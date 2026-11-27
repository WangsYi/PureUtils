//go:build windows

package collector

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestLiveWindowsHardwareAndEngineSampling(t *testing.T) {
	// SMBIOS raw firmware table query should safely succeed or return empty struct without panicking.
	_ = ReadSMBIOS()

	inv := CollectInventory()
	if inv.OS.Name == "" {
		t.Fatal("expected non-empty OS.Name from CollectInventory")
	}
	if inv.CPU.LogicalCores < 1 || inv.CPU.PhysicalCores < 1 {
		t.Fatalf("expected valid CPU core topology, got %+v", inv.CPU)
	}
	if len(inv.Volumes) == 0 {
		t.Fatal("expected at least one mounted volume")
	}
	if FormatSpecSummary(inv) == "" {
		t.Fatal("expected non-empty FormatSpecSummary")
	}

	eng := NewEngine(t.TempDir(), true)
	defer eng.Close()

	eng.SetOptions("auto", true)
	snap1 := eng.Sample()
	time.Sleep(150 * time.Millisecond)
	snap2 := eng.Sample()

	if snap1.RAMTotalBytes == 0 || snap2.RAMTotalBytes == 0 {
		t.Fatalf("expected non-zero RAMTotalBytes, got %d / %d", snap1.RAMTotalBytes, snap2.RAMTotalBytes)
	}
	if snap2.RAMPct <= 0 || snap2.ProcessCount == 0 || snap2.Uptime <= 0 {
		t.Fatalf("unexpected live snapshot metrics: ramPct=%v procs=%d uptime=%v", snap2.RAMPct, snap2.ProcessCount, snap2.Uptime)
	}
	if len(snap2.Sensors) < 4 {
		t.Fatalf("expected >=4 live sensor entries, got %d", len(snap2.Sensors))
	}
	if eng.LastSnapshot().RAMTotalBytes != snap2.RAMTotalBytes {
		t.Fatal("LastSnapshot mismatch")
	}
	if len(eng.History().Samples("cpu")) != 2 {
		t.Fatalf("expected 2 history samples, got %d", len(eng.History().Samples("cpu")))
	}
	if eng.Inventory().OS.Name != inv.OS.Name {
		t.Fatal("expected cached Engine.Inventory()")
	}

	// Safe non-admin SMART disk temp probe (should not panic when unprivileged).
	_ = QueryDiskTemperatures(4)
}

func TestEngineBuildSensorsAndHelperMerge(t *testing.T) {
	eng := NewEngine(t.TempDir(), false)
	defer eng.Close()

	// Verify Stop cancels pending ConnectNamedPipe cleanly when no client ever connects.
	pendingSrv := NewHelperServer()
	if pendingSrv.PipeName() == "" {
		t.Fatal("expected non-empty PipeName")
	}
	if err := pendingSrv.StartListening(); err != nil {
		t.Fatalf("StartListening pending: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	pendingSrv.Stop()
	pendingSrv.Stop() // idempotent
	if err := pendingSrv.StartListening(); err == nil {
		t.Fatal("expected error starting already-stopped HelperServer")
	}

	// Verify real overlapped named-pipe streaming end-to-end.
	srv := NewHelperServer()
	if err := srv.StartListening(); err != nil {
		t.Fatalf("StartListening: %v", err)
	}
	defer srv.Stop()

	eng.SetHelper(srv)
	if eng.HelperServer() != srv {
		t.Fatal("HelperServer getter mismatch")
	}
	// Replace helper and verify old helper can be swapped back/cleanly stopped.
	srvSwap := NewHelperServer()
	if err := srvSwap.StartListening(); err != nil {
		t.Fatal(err)
	}
	defer srvSwap.Stop()
	eng.SetHelper(srvSwap)
	if !func() bool {
		srv.mu.RLock()
		defer srv.mu.RUnlock()
		return srv.stopped
	}() {
		t.Fatal("expected SetHelper replacement to stop previous server")
	}
	srv = srvSwap

	pPtr, err := windows.UTF16PtrFromString(srv.PipeName())
	if err != nil {
		t.Fatal(err)
	}
	var pipeH windows.Handle
	for i := 0; i < 20; i++ {
		h, cerr := windows.CreateFile(pPtr, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
		if cerr == nil {
			pipeH = h
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if pipeH == 0 {
		t.Fatal("failed to connect test client to HelperServer pipe")
	}
	f := os.NewFile(uintptr(pipeH), "test-pipe-client")
	_, _ = f.WriteString("{malformed-json\n")
	payload, _ := json.Marshal(HelperFrame{
		Timestamp: time.Now(),
		CPUTempC:  67.5,
		CPUPowerW: 85.0,
		PawnIO:    true,
		DiskTemps: []DiskTemperatureReading{{Index: 9, TempC: 44, WarnTempC: 70, CritTempC: 80}},
		PIDFPS:    []HelperPIDFPS{{PID: 1234, FPS: 120, AvgFPS: 118, Low1Pct: 95, Low01Pct: 80, FrameTimeMs: 8.3}},
	})
	_, _ = f.Write(append(payload, '\n'))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := srv.Latest(); ok && srv.Connected() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	hf, ok := srv.Latest()
	if !ok || hf.CPUTempC != 67.5 || !hf.PawnIO {
		t.Fatalf("HelperServer.Latest = (%+v, %v)", hf, ok)
	}

	// eng.Sample() should merge helper CPU temp/power/PawnIO/disk temperature.
	merged := eng.Sample()
	if !merged.HelperConnected || merged.CPUTempC != 67.5 || merged.CPUPowerW != 85.0 || !merged.PawnIOActive {
		t.Fatalf("helper merge into Engine.Sample failed: conn=%v cpuT=%v cpuW=%v pawn=%v",
			merged.HelperConnected, merged.CPUTempC, merged.CPUPowerW, merged.PawnIOActive)
	}
	foundDisk9 := false
	for _, d := range merged.Disks {
		if d.Index == 9 && d.TempC == 44 {
			foundDisk9 = true
		}
	}
	if !foundDisk9 {
		t.Fatalf("expected helper disk 9 temp merged into disks: %+v", merged.Disks)
	}

	// Test stale frame expiration in Latest().
	srv.mu.Lock()
	srv.lastAt = time.Now().Add(-10 * time.Second)
	srv.mu.Unlock()
	if _, ok := srv.Latest(); ok {
		t.Fatal("expected stale helper frame to return ok=false")
	}

	_ = f.Close()

	// Exercise all branches of buildSensors with rich synthetic GPU/Disk/CPU telemetry.
	sensors := eng.buildSensors(SystemSnapshot{
		CPUPercent:   50,
		CPUFreqMHz:   4800,
		CPUTempC:     62,
		CPUPowerW:    90,
		RAMPct:       45,
		RAMUsedBytes: 8 * 1024 * 1024 * 1024,
		GPUs: []GPULive{{
			Name: "TestGPU", Load3D: 75, LoadVideo: 12, VRAMUsedMB: 4096, TempC: 66,
			HotspotC: 79, PowerW: 180, FanPct: 55, FanRPM: 1650, CoreClockMHz: 2550, Source: "NVML",
		}},
		Disks: []DiskLive{{Index: 0, Name: "NVMe0", ReadBps: 1024 * 1024, WriteBps: 512 * 1024, TempC: 41}},
	})
	if len(sensors) < 16 {
		t.Fatalf("expected full buildSensors entries (>=16), got %d", len(sensors))
	}
}

func TestWatchParentProcessAndLaunchElevatedValidation(t *testing.T) {
	if err := LaunchElevatedExe("", "", 0); err == nil {
		t.Fatal("expected error for empty LaunchElevatedExe path")
	}
	if err := RunPawnIOSilentInstall(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty RunPawnIOSilentInstall path")
	}
	badExe := filepath.Join(t.TempDir(), "not-an-exe.exe")
	if err := os.WriteFile(badExe, []byte("invalid payload without MZ header"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunPawnIOSilentInstall(context.Background(), badExe); err == nil {
		t.Fatal("expected error for non-MZ RunPawnIOSilentInstall path")
	}
	// No-op guards
	WatchParentProcess(0, func() {})
	WatchParentProcess(uint32(os.Getpid()), nil)

	// Start a quick subprocess and verify WatchParentProcess fires when it exits.
	cmd := exec.Command("cmd.exe", "/c", "ping", "-n", "1", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child cmd: %v", err)
	}
	done := make(chan struct{})
	WatchParentProcess(uint32(cmd.Process.Pid), func() { close(done) })
	_ = cmd.Wait()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WatchParentProcess callback did not fire after child exit")
	}
}

func TestInventoryAndPDHParsers(t *testing.T) {
	if got := gpuVendorFromID(0x10DE, ""); got != "NVIDIA" {
		t.Fatalf("0x10DE -> %q", got)
	}
	if got := gpuVendorFromID(0x1002, ""); got != "AMD" {
		t.Fatalf("0x1002 -> %q", got)
	}
	if got := gpuVendorFromID(0x8086, ""); got != "Intel" {
		t.Fatalf("0x8086 -> %q", got)
	}
	if got := gpuVendorFromID(0, "NVIDIA GeForce RTX 4090"); got != "NVIDIA" {
		t.Fatalf("name RTX -> %q", got)
	}
	if got := gpuVendorFromID(0, "AMD Radeon RX 7900"); got != "AMD" {
		t.Fatalf("name Radeon -> %q", got)
	}
	if got := gpuVendorFromID(0, "Intel Arc A770"); got != "Intel" {
		t.Fatalf("name Arc -> %q", got)
	}
	if got := gpuVendorFromID(0, "Unknown Virtual"); got != "" {
		t.Fatalf("unknown -> %q", got)
	}

	if got := readCString([]byte{'A', 'B', 0, 'C'}, 0); got != "" {
		t.Fatalf("off=0 should be empty, got %q", got)
	}
	if got := readCString([]byte{'X', 'Y', 0}, 1); got != "Y" {
		t.Fatalf("readCString off=1 -> %q", got)
	}
	if got := readCString([]byte{'X', 'Y'}, 9); got != "" {
		t.Fatalf("OOB should be empty, got %q", got)
	}

	for code, want := range map[uint32]string{3: "ATA", 7: "USB", 8: "RAID", 11: "SATA", 12: "SD", 17: "NVMe", 18: "SCM", 99: "Storage"} {
		if got := storageBusTypeName(code); got != want {
			t.Fatalf("bus %d -> %q, want %q", code, got, want)
		}
	}
	if !isVirtualAdapterDesc("WAN Miniport (IP)") || !isVirtualAdapterDesc("Teredo Tunneling") || isVirtualAdapterDesc("Intel(R) Ethernet I226-V") {
		t.Fatal("isVirtualAdapterDesc mismatch")
	}
	if popCount64(0xF0F0) != 8 {
		t.Fatalf("popCount64(0xF0F0) = %d", popCount64(0xF0F0))
	}

	if idx, lbl, ok := parsePhysicalDiskInstance("0 C: D:"); !ok || idx != 0 || lbl != "0 C: D:" {
		t.Fatalf("parsePhysicalDiskInstance valid failed: %d %q %v", idx, lbl, ok)
	}
	if _, _, ok := parsePhysicalDiskInstance("_Total"); ok {
		t.Fatal("_Total should not parse as disk index")
	}
	if _, _, ok := parsePhysicalDiskInstance(""); ok {
		t.Fatal("empty instance should fail")
	}
	if pid, eng := parseGPUEngineInstance("pid_4321_luid_0x0_0x1_phys_0_eng_0_engtype_3D"); pid != 4321 || eng != "3D" {
		t.Fatalf("parseGPUEngineInstance = (%d, %q)", pid, eng)
	}
}

func TestPawnIOWindowsReaderErrorPaths(t *testing.T) {
	_ = IsPawnIODriverInstalled()

	emptyDir := t.TempDir()
	intel := NewPawnIOSensorReader(emptyDir, "GenuineIntel")
	if _, _, ok := intel.Sample(time.Now()); ok || intel.Active() || intel.LastError() == "" {
		t.Fatalf("expected missing module error, got active=%v err=%q", intel.Active(), intel.LastError())
	}
	intel.Close()

	// Create dummy >16-byte .bin files for both Intel and AMD and verify module discovery & safe failure without driver.
	modDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modDir, "IntelMSR.bin"), make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "AMDFamily17.bin"), make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	intelWithMod := NewPawnIOSensorReader(modDir, "GenuineIntel")
	if p := intelWithMod.findModuleBinary(); !strings.HasSuffix(p, "IntelMSR.bin") {
		t.Fatalf("findModuleBinary Intel = %q", p)
	}
	amdWithMod := NewPawnIOSensorReader(modDir, "AuthenticAMD")
	if p := amdWithMod.findModuleBinary(); !strings.HasSuffix(p, "AMDFamily17.bin") {
		t.Fatalf("findModuleBinary AMD = %q", p)
	}
	if _, err := intelWithMod.execFnLocked("ioctl_read_msr", nil); err == nil {
		t.Fatal("expected error calling execFnLocked before opening device")
	}
	_, _, _ = intelWithMod.Sample(time.Now())
	_, _, _ = amdWithMod.Sample(time.Now())
	intelWithMod.Close()
	amdWithMod.Close()
}

func TestADLVendorMatchingAndPMLogDecode(t *testing.T) {
	if !adlVendorMatches(1002) || !adlVendorMatches(0x1002) {
		t.Fatal("expected decimal 1002 and hex 0x1002 vendor IDs to match AMD")
	}
	if adlVendorMatches(0x10DE) || adlVendorMatches(0) {
		t.Fatal("unexpected non-AMD vendor match")
	}

	if got := normalizeODNTemp(0); got != 0 {
		t.Fatalf("normalizeODNTemp(0) = %v", got)
	}
	if got := normalizeODNTemp(58); got != 58 {
		t.Fatalf("normalizeODNTemp(58) = %v", got)
	}
	if got := normalizeODNTemp(58000); got != 58 {
		t.Fatalf("normalizeODNTemp(58000) = %v", got)
	}

	// Discrete GPU layout: TempEdge(8), ClkMem(2), FanPct(15), BoardPower(73).
	var pmDiscrete adlPMLogDataOutput
	pmDiscrete.Sensors[adlPMLogTempEdge] = adlSingleSensorData{Supported: 1, Value: 64}
	pmDiscrete.Sensors[adlPMLogTempHot] = adlSingleSensorData{Supported: 1, Value: 77}
	pmDiscrete.Sensors[adlPMLogClkGFX] = adlSingleSensorData{Supported: 1, Value: 2400}
	pmDiscrete.Sensors[adlPMLogClkMem] = adlSingleSensorData{Supported: 1, Value: 2250}
	pmDiscrete.Sensors[adlPMLogGfxAct] = adlSingleSensorData{Supported: 1, Value: 88}
	pmDiscrete.Sensors[adlPMLogFanPct] = adlSingleSensorData{Supported: 1, Value: 45}
	pmDiscrete.Sensors[adlPMLogFanRPM] = adlSingleSensorData{Supported: 1, Value: 1500}
	pmDiscrete.Sensors[adlPMLogBoardPower] = adlSingleSensorData{Supported: 1, Value: 195}

	var g1 GPULive
	ct1, cp1 := decodeADLPMLog(pmDiscrete, &g1)
	if g1.TempC != 64 || g1.HotspotC != 77 || g1.MemClockMHz != 2250 || g1.PowerW != 195 || g1.Load3D != 88 || g1.FanPct != 45 || g1.FanRPM != 1500 {
		t.Fatalf("discrete decode mismatch: %+v", g1)
	}
	if ct1 != 0 || cp1 != 0 {
		t.Fatalf("unexpected APU CPU values on discrete card: %v / %v", ct1, cp1)
	}

	// AMD APU layout (like Ryzen 8745H / Radeon 780M): TempGFX(28), ASICPower(23), TempCPU(32), CPUPower(33).
	var pmAPU adlPMLogDataOutput
	pmAPU.Sensors[adlPMLogTempGFX] = adlSingleSensorData{Supported: 1, Value: 44}
	pmAPU.Sensors[adlPMLogASICPower] = adlSingleSensorData{Supported: 1, Value: 21}
	pmAPU.Sensors[adlPMLogGfxPower] = adlSingleSensorData{Supported: 1, Value: 28}
	pmAPU.Sensors[adlPMLogTempCPU] = adlSingleSensorData{Supported: 1, Value: 54}
	pmAPU.Sensors[adlPMLogCPUPower] = adlSingleSensorData{Supported: 1, Value: 15}

	var g2 GPULive
	ct2, cp2 := decodeADLPMLog(pmAPU, &g2)
	if g2.TempC != 44 || g2.PowerW != 21 {
		t.Fatalf("APU GPU decode mismatch: %+v", g2)
	}
	if ct2 != 54 || cp2 != 15 {
		t.Fatalf("APU CPU decode mismatch: temp=%v power=%v", ct2, cp2)
	}
}

func TestSampleDetailedToggleTopProcessesAndScratchRelease(t *testing.T) {
	eng := NewEngine(t.TempDir(), false)
	defer eng.Close()

	snapFull := eng.SampleDetailed(true)
	if len(snapFull.TopCPU) == 0 || len(snapFull.TopRAM) == 0 {
		t.Fatalf("expected non-empty TopCPU/TopRAM when includeProcs=true, got %d / %d", len(snapFull.TopCPU), len(snapFull.TopRAM))
	}
	if len(eng.cpuSampler.procBuf) == 0 {
		t.Fatal("expected procBuf populated after SampleDetailed(true)")
	}

	snapIdle := eng.SampleDetailed(false)
	if len(snapIdle.TopCPU) != 0 || len(snapIdle.TopRAM) != 0 {
		t.Fatalf("expected nil TopCPU/TopRAM when includeProcs=false, got %d / %d", len(snapIdle.TopCPU), len(snapIdle.TopRAM))
	}
	if eng.cpuSampler.procBuf != nil || len(eng.cpuSampler.prevProcs) != 0 {
		t.Fatalf("expected idle scratch freed when includeProcs=false, procBuf=%d prevProcs=%d", len(eng.cpuSampler.procBuf), len(eng.cpuSampler.prevProcs))
	}
	if snapIdle.RAMTotalBytes == 0 || snapIdle.ProcessCount == 0 {
		t.Fatalf("expected basic telemetry preserved in idle mode: ram=%d procs=%d", snapIdle.RAMTotalBytes, snapIdle.ProcessCount)
	}
}
