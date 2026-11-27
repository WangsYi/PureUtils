package fps

import (
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestFrameStatsFPSAndLowPercentiles(t *testing.T) {
	fs := NewFrameStats()
	wall := time.Now()
	const pid = 4242

	// Simulate 100 intervals over ~1s: 98 frames at 10ms (100 FPS) + 2 stutter frames at 25ms & 40ms.
	ts := int64(1_000_000_000)
	fs.RecordFrame(pid, ts, true, wall)
	for i := 0; i < 100; i++ {
		dtMs := int64(10)
		switch i {
		case 40:
			dtMs = 25
		case 80:
			dtMs = 40
		}
		ts += dtMs * 1_000_000
		fs.RecordFrame(pid, ts, true, wall)
	}
	// Verify DxgKrnl fallback is ignored when DXGI was seen within 1s.
	fs.RecordFrame(pid, ts+1_000_000, false, wall)

	st, ok := fs.StatsForPID(pid, ts+50_000_000)
	if !ok {
		t.Fatal("expected active stats")
	}
	if st.SampleCount != 100 {
		t.Fatalf("SampleCount = %d, want 100 (fallback frame should be suppressed)", st.SampleCount)
	}
	if st.FPS < 85 || st.FPS > 105 {
		t.Fatalf("FPS = %.1f, expected ~95", st.FPS)
	}
	// 99th percentile of 100 intervals is index 98 (25ms -> 40 FPS) or 99 (40ms -> 25 FPS).
	if st.Low1PctFPS < 24 || st.Low1PctFPS > 41 {
		t.Fatalf("Low1PctFPS = %.1f, expected 25..40", st.Low1PctFPS)
	}
	if len(st.RecentFTMs) != 60 {
		t.Fatalf("RecentFTMs len = %d, want 60", len(st.RecentFTMs))
	}

	// Stale pruning
	fs.Prune(wall.Add(10*time.Second), 5*time.Second)
	if active := fs.AllActive(ts + 50_000_000); len(active) != 0 {
		t.Fatalf("expected pruned empty, got %d", len(active))
	}
}

func TestDetectorWhitelistBlacklistAndHeuristics(t *testing.T) {
	det := NewDetector()
	det.Configure(true, []string{"myindiegame.exe"}, []string{"blockedgame"})
	now := time.Now()

	// 1. Whitelist triggers even in small window with 0 GPU load.
	st := det.Evaluate(ForegroundInfo{PID: 100, ExeName: "MyIndieGame", WindowTitle: "My Indie"}, now)
	if !st.Active || st.Reason != "whitelist" {
		t.Fatalf("whitelist failed: %+v", st)
	}

	// 2. Blacklist always suppresses even if D3D fullscreen + high 3D + high FPS.
	st = det.Evaluate(ForegroundInfo{
		PID: 101, ExeName: "blockedgame", D3DFullscreen: true, GPU3DPct: 95, FPS: 144,
	}, now.Add(time.Second))
	if st.Active {
		t.Fatalf("blacklist should suppress immediately: %+v", st)
	}

	// 3. Built-in browser/video player rejected even in fullscreen.
	st = det.Evaluate(ForegroundInfo{
		PID: 102, ExeName: "chrome", Fullscreen: true, GPU3DPct: 30, FPS: 60,
	}, now.Add(2*time.Second))
	if st.Active {
		t.Fatalf("chrome should be ignored: %+v", st)
	}

	// 4. Fullscreen video player using VideoDecode (not 3D game) rejected.
	st = det.Evaluate(ForegroundInfo{
		PID: 103, ExeName: "customplayer", Fullscreen: true, GPUVideoPct: 45, GPU3DPct: 5, FPS: 24,
	}, now.Add(3*time.Second))
	if st.Active {
		t.Fatalf("video decode app should not trigger game HUD: %+v", st)
	}

	// 5. Real game auto-detection: borderless fullscreen + GPU 3D + FPS + Steam path.
	gameAt := now.Add(4 * time.Second)
	st = det.Evaluate(ForegroundInfo{
		PID:        104,
		ExeName:    "eldenring",
		ExePath:    `D:\SteamLibrary\steamapps\common\ELDEN RING\Game\eldenring.exe`,
		Fullscreen: true,
		GPU3DPct:   82,
		FPS:        60,
	}, gameAt)
	if !st.Active || st.Reason != "auto" || st.ExeName != "eldenring" {
		t.Fatalf("expected auto game detection, got %+v", st)
	}

	// 6. Alt-Tab hysteresis: brief switch to notepad stays active for <= 3s, clears after > 3s.
	stAlt := det.Evaluate(ForegroundInfo{PID: 105, ExeName: "notepad"}, gameAt.Add(1500*time.Millisecond))
	if !stAlt.Active {
		t.Fatalf("expected 3s hysteresis to keep HUD alive briefly, got %+v", stAlt)
	}
	stAfter := det.Evaluate(ForegroundInfo{PID: 105, ExeName: "notepad"}, gameAt.Add(4*time.Second))
	if stAfter.Active {
		t.Fatalf("expected HUD cleared after 3s grace period, got %+v", stAfter)
	}

	if recent := det.RecentApps(); len(recent) == 0 {
		t.Fatal("expected recent foreground apps recorded")
	}
}

func TestETWStructLayoutAMD64(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("amd64 Windows struct layout check")
	}
	if got := unsafe.Sizeof(etwTraceLogfileW{}); got != 448 {
		t.Fatalf("sizeof(etwTraceLogfileW) = %d, want 448", got)
	}
	if got := unsafe.Offsetof(etwTraceLogfileW{}.EventRecordCallback); got != 424 {
		t.Fatalf("offsetof(etwTraceLogfileW.EventRecordCallback) = %d, want 424", got)
	}
	if got := unsafe.Sizeof(etwEventRecord{}); got != 112 {
		t.Fatalf("sizeof(etwEventRecord) = %d, want 112", got)
	}
}

func TestFrameStatsEdgeCases(t *testing.T) {
	fs := NewFrameStats()
	wall := time.Now()
	base := int64(10_000_000_000)

	// Invalid / non-monotonic inputs should be ignored safely.
	fs.RecordFrame(0, base, true, wall)
	fs.RecordFrame(100, 0, true, wall)
	fs.RecordFrame(100, base, true, wall)
	if _, ok := fs.StatsForPID(100, base+1_000_000); ok {
		t.Fatal("single sample should not yield FPS stats")
	}
	// Non-monotonic timestamp ignored.
	fs.RecordFrame(100, base-500, true, wall)
	if _, ok := fs.StatsForPID(100, base+1_000_000); ok {
		t.Fatal("non-monotonic timestamp should be ignored")
	}
	// Slow frame (<2 FPS): single 700ms interval -> falls back to interval-based FPS.
	fs.RecordFrame(100, base+700_000_000, true, wall)
	stSlow, ok := fs.StatsForPID(100, base+800_000_000)
	if !ok || stSlow.FPS < 1.0 || stSlow.FPS > 2.0 {
		t.Fatalf("slow interval FPS = (%+v, %v)", stSlow, ok)
	}

	// Fast PID 200 over 70 frames to exercise >60 sparkline truncation.
	ts := base
	for i := 0; i < 70; i++ {
		ts += 16_000_000
		fs.RecordFrame(200, ts, true, wall)
	}
	all := fs.AllActive(ts + 10_000_000)
	if len(all) < 1 {
		t.Fatalf("expected active PIDs, got %d", len(all))
	}
	stFast, ok := fs.StatsForPID(200, ts+10_000_000)
	if !ok || len(stFast.RecentFTMs) != 60 {
		t.Fatalf("expected 60 frametime samples, got %d (ok=%v)", len(stFast.RecentFTMs), ok)
	}
	// Inactive after >2s gap.
	if _, ok := fs.StatsForPID(200, ts+int64(3*time.Second)); ok {
		t.Fatal("expected inactive stats when last frame >2s old")
	}

	if got := percentileMax(nil, 0.99); got != 0 {
		t.Fatalf("empty percentileMax = %v, want 0", got)
	}
	if got := percentileMax([]float64{5, 10}, -1); got != 5 {
		t.Fatalf("p=-1 percentileMax = %v, want 5", got)
	}
	if got := percentileMax([]float64{5, 10}, 2); got != 10 {
		t.Fatalf("p=2 percentileMax = %v, want 10", got)
	}
}

func TestDetectorCompleteHeuristicsAndEdges(t *testing.T) {
	det := NewDetector()
	now := time.Now()

	// Empty process name yields inactive state.
	if st := det.Evaluate(ForegroundInfo{PID: 1}, now); st.Active {
		t.Fatalf("empty ExeName should not be active: %+v", st)
	}

	// Self process "pureutils" is always excluded even if on whitelist.
	det.Configure(true, []string{"pureutils.exe", "customgame.exe"}, nil)
	if st := det.Evaluate(ForegroundInfo{PID: 2, ExeName: "PureUtils.exe", D3DFullscreen: true, FPS: 120}, now); st.Active {
		t.Fatalf("pureutils itself must stay suppressed: %+v", st)
	}

	// ExeName resolved from ExePath when ExeName is empty + known game launcher path scoring.
	sc := ScoreGameCandidate(ForegroundInfo{
		Fullscreen: true,
		GPU3DPct:   12,
		FPS:        20,
		ExePath:    `C:\XboxGames\Halo Infinite\game.exe`,
	})
	if sc < 55 {
		t.Fatalf("ScoreGameCandidate = %d, want >= 55", sc)
	}
	scLow := ScoreGameCandidate(ForegroundInfo{GPU3DPct: 4, FPS: 8})
	if scLow >= 55 {
		t.Fatalf("low score unexpectedly high: %d", scLow)
	}

	// Trigger auto-detect via path resolution, then verify shell process keeps 3s hysteresis without polluting RecentApps.
	tGame := now.Add(time.Second)
	st := det.Evaluate(ForegroundInfo{
		PID:          300,
		ExePath:      `D:\Epic Games\GenshinImpact\Game.exe`,
		WindowTitle:  "Genshin Impact",
		D3DFullscreen: true,
		GPU3DPct:     70,
		FPS:          60,
	}, tGame)
	if !st.Active || st.ExeName != "game" {
		t.Fatalf("expected active via ExePath, got %+v", st)
	}
	stShell := det.Evaluate(ForegroundInfo{PID: 301, ExeName: "explorer.exe"}, tGame.Add(time.Second))
	if !stShell.Active {
		t.Fatalf("expected shell process within 3s grace period to keep state active: %+v", stShell)
	}

	// autoDetect=false allows whitelist only and clears immediately otherwise.
	det.Configure(false, []string{"whitelistonly"}, nil)
	if st := det.Evaluate(ForegroundInfo{PID: 302, ExeName: "nonwl", D3DFullscreen: true, GPU3DPct: 99, FPS: 144}, tGame.Add(2*time.Second)); st.Active {
		t.Fatalf("autoDetect=false should ignore non-whitelisted app: %+v", st)
	}
	if st := det.Evaluate(ForegroundInfo{PID: 303, ExeName: "whitelistonly"}, tGame.Add(2*time.Second)); !st.Active || st.Reason != "whitelist" {
		t.Fatalf("autoDetect=false should still honor whitelist: %+v", st)
	}

	// Record >12 unique apps to test 10-entry RecentApps cap and deduplication.
	for i := 0; i < 14; i++ {
		name := string(rune('a'+i)) + "_app.exe"
		det.Evaluate(ForegroundInfo{PID: uint32(400 + i), ExeName: name}, tGame.Add(time.Duration(i)*time.Millisecond))
	}
	det.Evaluate(ForegroundInfo{PID: 400, ExeName: "a_app.exe"}, tGame.Add(20*time.Millisecond))
	recent := det.RecentApps()
	if len(recent) != 10 || recent[0].ExeName != "a_app" {
		t.Fatalf("RecentApps len=%d first=%+v", len(recent), recent)
	}
}
