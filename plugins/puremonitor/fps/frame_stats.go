package fps

import (
	"math"
	"sort"
	"sync"
	"time"
)

// PIDStats holds computed real-time FPS and frametime metrics for a single process.
type PIDStats struct {
	PID         uint32    `json:"pid"`
	FPS         float64   `json:"fps"`
	AvgFPS      float64   `json:"avgFps"`
	Low1PctFPS  float64   `json:"low1PctFps"`
	Low01PctFPS float64   `json:"low01PctFps"`
	FrameTimeMs float64   `json:"frameTimeMs"`
	RecentFTMs  []float32 `json:"recentFtMs,omitempty"`
	SampleCount int       `json:"sampleCount"`
	LastEventAt time.Time `json:"lastEventAt"`
}

type pidRing struct {
	timestamps []int64 // monotonic nanoseconds
	lastSeen   time.Time
	lastDXGI   time.Time
}

// FrameStats aggregates real-time presentation timestamps per PID and computes
// FPS, Average FPS, 1% Low, 0.1% Low, and moving-average frametime.
type FrameStats struct {
	mu      sync.RWMutex
	byPID   map[uint32]*pidRing
	maxKeep int
}

// NewFrameStats constructs a thread-safe per-PID frame timing aggregator.
func NewFrameStats() *FrameStats {
	return &FrameStats{
		byPID:   make(map[uint32]*pidRing),
		maxKeep: 2048,
	}
}

// RecordFrame registers a frame present event for pid at monotonic timestamp ns.
// isPrimary is true for DXGI/D3D9 events; fallback DxgKrnl events are ignored if
// a primary DXGI/D3D9 event arrived for the same PID within the last 1 second.
func (f *FrameStats) RecordFrame(pid uint32, ns int64, isPrimary bool, wall time.Time) {
	if pid == 0 || ns <= 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.byPID[pid]
	if r == nil {
		r = &pidRing{timestamps: make([]int64, 0, 256)}
		f.byPID[pid] = r
	}
	if !isPrimary && !r.lastDXGI.IsZero() && wall.Sub(r.lastDXGI) < time.Second {
		return
	}
	if isPrimary {
		r.lastDXGI = wall
	}
	r.lastSeen = wall
	if len(r.timestamps) > 0 && ns <= r.timestamps[len(r.timestamps)-1] {
		// Guard against slight out-of-order ETW buffer delivery; keep non-decreasing.
		return
	}
	r.timestamps = append(r.timestamps, ns)
	if len(r.timestamps) > f.maxKeep {
		drop := len(r.timestamps) - f.maxKeep
		r.timestamps = append([]int64(nil), r.timestamps[drop:]...)
	}
}

// StatsForPID computes live FPS metrics for pid at monotonic time nowNS.
func (f *FrameStats) StatsForPID(pid uint32, nowNS int64) (PIDStats, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	r := f.byPID[pid]
	if r == nil || len(r.timestamps) < 2 {
		return PIDStats{}, false
	}
	return computeRingStats(pid, r, nowNS)
}

// AllActive returns computed stats for all PIDs with at least 2 recent frames.
func (f *FrameStats) AllActive(nowNS int64) []PIDStats {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]PIDStats, 0, len(f.byPID))
	for pid, r := range f.byPID {
		if st, ok := computeRingStats(pid, r, nowNS); ok {
			out = append(out, st)
		}
	}
	return out
}

// Prune removes PIDs that haven't presented frames within maxAge.
func (f *FrameStats) Prune(now time.Time, maxAge time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for pid, r := range f.byPID {
		if now.Sub(r.lastSeen) > maxAge {
			delete(f.byPID, pid)
		}
	}
}

func computeRingStats(pid uint32, r *pidRing, nowNS int64) (PIDStats, bool) {
	ts := r.timestamps
	if len(ts) < 2 {
		return PIDStats{}, false
	}
	// If no frame in the last 2 seconds, return inactive.
	if nowNS-ts[len(ts)-1] > int64(2*time.Second) {
		return PIDStats{}, false
	}

	const win1s = int64(1000 * time.Millisecond)
	const win5s = int64(5000 * time.Millisecond)

	// Collect frametime intervals within last 5s (and count frames in last 1s).
	var intervalsMs []float64
	var win1Frames int
	var win1First, win1Last int64
	var win5First, win5Last int64
	var win5Frames int

	for i := 0; i < len(ts); i++ {
		t := ts[i]
		age := nowNS - t
		if age <= win5s {
			if win5First == 0 {
				win5First = t
			}
			win5Last = t
			win5Frames++
		}
		if age <= win1s {
			if win1First == 0 {
				win1First = t
			}
			win1Last = t
			win1Frames++
		}
		if i > 0 && age <= win5s {
			dtMs := float64(t-ts[i-1]) / 1e6
			if dtMs > 0.1 && dtMs < 2000 {
				intervalsMs = append(intervalsMs, dtMs)
			}
		}
	}

	if len(intervalsMs) == 0 {
		return PIDStats{}, false
	}

	var fps float64
	if win1Frames >= 2 && win1Last > win1First {
		fps = float64(win1Frames-1) / (float64(win1Last-win1First) / 1e9)
	} else {
		// Fall back to recent frame intervals if framerate is < 2 FPS.
		lastDt := intervalsMs[len(intervalsMs)-1]
		if lastDt > 0 {
			fps = 1000.0 / lastDt
		}
	}

	var avgFPS float64
	if win5Frames >= 2 && win5Last > win5First {
		avgFPS = float64(win5Frames-1) / (float64(win5Last-win5First) / 1e9)
	} else {
		avgFPS = fps
	}

	// Moving average frametime over up to last 30 frames.
	recentStart := 0
	if len(intervalsMs) > 30 {
		recentStart = len(intervalsMs) - 30
	}
	var sumFt float64
	for _, dt := range intervalsMs[recentStart:] {
		sumFt += dt
	}
	ftMs := sumFt / float64(len(intervalsMs[recentStart:]))

	// Keep up to 60 samples in chronological order for frametime sparkline.
	sparkStart := 0
	if len(intervalsMs) > 60 {
		sparkStart = len(intervalsMs) - 60
	}
	spark := make([]float32, len(intervalsMs[sparkStart:]))
	for i, dt := range intervalsMs[sparkStart:] {
		spark[i] = float32(dt)
	}

	// Compute 1% Low and 0.1% Low from longest frametimes (99th and 99.9th percentiles).
	sorted := append([]float64(nil), intervalsMs...)
	sort.Float64s(sorted)
	low1Ft := percentileMax(sorted, 0.99)
	low01Ft := percentileMax(sorted, 0.999)

	low1FPS := 0.0
	if low1Ft > 0 {
		low1FPS = 1000.0 / low1Ft
	}
	low01FPS := 0.0
	if low01Ft > 0 {
		low01FPS = 1000.0 / low01Ft
	}

	return PIDStats{
		PID:         pid,
		FPS:         round1(fps),
		AvgFPS:      round1(avgFPS),
		Low1PctFPS:  round1(low1FPS),
		Low01PctFPS: round1(low01FPS),
		FrameTimeMs: round1(ftMs),
		RecentFTMs:  spark,
		SampleCount: len(intervalsMs),
		LastEventAt: r.lastSeen,
	}, true
}

func percentileMax(sortedAsc []float64, p float64) float64 {
	if len(sortedAsc) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sortedAsc)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sortedAsc) {
		idx = len(sortedAsc) - 1
	}
	return sortedAsc[idx]
}

func round1(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*10) / 10
}
