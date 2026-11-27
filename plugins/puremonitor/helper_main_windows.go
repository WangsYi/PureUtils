//go:build windows

package puremonitor

import (
	"context"
	"encoding/json"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"pureutils/plugins/puremonitor/collector"
	"pureutils/plugins/puremonitor/fps"
)

// RunHelperCLI executes the elevated telemetry/ETW helper subprocess when args contain
// `--monitor-helper`. It streams HelperFrame NDJSON lines to the caller's named pipe and
// terminates automatically as soon as the parent process exits.
func RunHelperCLI(args []string) (handled bool, exitCode int) {
	var isHelper bool
	var pipeName, mode, sensorDir, cpuVendor string
	var parentPID uint32

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--monitor-helper":
			isHelper = true
		case "--pipe":
			if i+1 < len(args) {
				pipeName = args[i+1]
				i++
			}
		case "--mode":
			if i+1 < len(args) {
				mode = strings.ToLower(strings.TrimSpace(args[i+1]))
				i++
			}
		case "--parent-pid":
			if i+1 < len(args) {
				if n, err := strconv.ParseUint(args[i+1], 10, 32); err == nil {
					parentPID = uint32(n)
				}
				i++
			}
		case "--sensor-dir":
			if i+1 < len(args) {
				sensorDir = args[i+1]
				i++
			}
		case "--cpu-vendor":
			if i+1 < len(args) {
				cpuVendor = args[i+1]
				i++
			}
		}
	}

	if !isHelper {
		return false, 0
	}
	debug.SetGCPercent(25)
	debug.SetMemoryLimit(24 << 20)
	if strings.TrimSpace(pipeName) == "" {
		return true, 2
	}
	if mode == "" {
		mode = "both"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if parentPID != 0 {
		collector.WatchParentProcess(parentPID, cancel)
	}

	pPtr, err := windows.UTF16PtrFromString(pipeName)
	if err != nil {
		return true, 3
	}
	var pipeH windows.Handle
	for attempt := 0; attempt < 25; attempt++ {
		h, cerr := windows.CreateFile(pPtr, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
		if cerr == nil {
			pipeH = h
			break
		}
		time.Sleep(120 * time.Millisecond)
	}
	if pipeH == 0 {
		return true, 4
	}
	pipeFile := os.NewFile(uintptr(pipeH), pipeName)
	defer pipeFile.Close()

	var etwEng *fps.ETWEngine
	if mode == "fps" || mode == "both" {
		etwEng = fps.NewETWEngine(fps.NewFrameStats())
		if err := etwEng.Start(); err != nil {
			etwEng = nil
		} else {
			defer etwEng.Stop()
		}
	}

	var pawnReader *collector.PawnIOSensorReader
	if mode == "sensors" || mode == "both" {
		pawnReader = collector.NewPawnIOSensorReader(sensorDir, cpuVendor)
		defer pawnReader.Close()
	}

	var cachedDisks []collector.DiskTemperatureReading
	var lastDiskAt time.Time

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return true, 0
		case now := <-ticker.C:
			frame := collector.HelperFrame{
				Timestamp: now,
			}
			if pawnReader != nil {
				if t, p, ok := pawnReader.Sample(now); ok {
					frame.CPUTempC = t
					frame.CPUPowerW = p
					frame.PawnIO = true
				}
			}
			if (mode == "sensors" || mode == "both") && now.Sub(lastDiskAt) >= 5*time.Second {
				lastDiskAt = now
				cachedDisks = collector.QueryDiskTemperatures(16)
			}
			frame.DiskTemps = cachedDisks

			if etwEng != nil {
				nowNS := fps.MonotonicNowNS()
				etwEng.Stats().Prune(now, 5*time.Second)
				for _, st := range etwEng.Stats().AllActive(nowNS) {
					frame.PIDFPS = append(frame.PIDFPS, collector.HelperPIDFPS{
						PID:         st.PID,
						FPS:         st.FPS,
						AvgFPS:      st.AvgFPS,
						Low1Pct:     st.Low1PctFPS,
						Low01Pct:    st.Low01PctFPS,
						FrameTimeMs: st.FrameTimeMs,
						RecentFT:    st.RecentFTMs,
					})
				}
			}

			line, merr := json.Marshal(frame)
			if merr != nil {
				continue
			}
			line = append(line, '\n')
			if _, werr := pipeFile.Write(line); werr != nil {
				return true, 0
			}
		}
	}
}
