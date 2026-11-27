package fps

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ForegroundInfo captures metadata about the current foreground window and its process.
type ForegroundInfo struct {
	PID          uint32
	ExeName      string // lower-case base name without .exe, e.g. "cyberpunk2077"
	ExePath      string // full executable path
	WindowTitle  string
	Fullscreen   bool
	D3DFullscreen bool
	GPU3DPct     float64
	GPUVideoPct  float64
	FPS          float64
}

// DetectedApp records a recently observed foreground process for quick whitelist/blacklist actions.
type DetectedApp struct {
	ExeName string
	Title   string
	SeenAt  time.Time
}

// DetectorProbe provides OS foreground & GPU/FPS telemetry (abstracted for tests).
type DetectorProbe interface {
	ProbeForeground() ForegroundInfo
}

// GameState represents the current output of the automatic game detector.
type GameState struct {
	Active   bool
	PID      uint32
	ExeName  string
	Title    string
	Reason   string // "whitelist" | "auto" | ""
	MonitorX int
	MonitorY int
	MonitorW int
	MonitorH int
}

var defaultIgnoredProcesses = map[string]bool{
	"explorer":            true,
	"searchhost":          true,
	"shellexperiencehost": true,
	"startmenuexperiencehost": true,
	"applicationframehost": true,
	"lockapp":             true,
	"dwm":                 true,
	"taskmgr":             true,
	"pureutils":           true,
	"chrome":              true,
	"msedge":              true,
	"firefox":             true,
	"opera":               true,
	"brave":               true,
	"vivaldi":             true,
	"vlc":                 true,
	"potplayermini64":     true,
	"potplayermini":       true,
	"mpv":                 true,
	"mpc-hc64":            true,
	"mpc-be64":            true,
	"wmplayer":            true,
	"iina":                true,
	"code":                true,
	"devenv":              true,
	"goland64":            true,
	"idea64":              true,
	"pycharm64":           true,
	"clion64":             true,
	"rider64":             true,
	"wechat":              true,
	"weixin":              true,
	"qq":                  true,
	"tim":                 true,
	"dingtalk":            true,
	"feishu":              true,
	"lark":                true,
	"wemeetapp":           true,
	"zoom":                true,
	"teams":               true,
	"discord":             true,
	"slack":               true,
	"obs64":               true,
	"wallpaper64":         true,
	"wallpaper32":         true,
	"powerpnt":            true,
	"winword":             true,
	"excel":               true,
	"wps":                 true,
	"notepad":             true,
	"windowsterminal":     true,
	"cmd":                 true,
	"powershell":          true,
	"pwsh":                true,
}

var knownGamePathHints = []string{
	`\steamapps\common\`,
	`\epic games\`,
	`\wegameapps\`,
	`\xboxgames\`,
	`\gog galaxy\games\`,
	`\ubisoft game launcher\games\`,
	`\riot games\`,
	`\battle.net\`,
	`\ea games\`,
	`\hoyoverse\`,
	`\mihoyo\`,
}

// Detector evaluates foreground apps against whitelist/blacklist and multi-signal heuristics.
type Detector struct {
	mu           sync.Mutex
	autoDetect   bool
	whitelist    map[string]bool
	blacklist    map[string]bool
	recent       []DetectedApp
	activeState  GameState
	lastPositive time.Time
	gracePeriod  time.Duration
}

// NewDetector creates a game detector with default 3-second Alt-Tab exit hysteresis.
func NewDetector() *Detector {
	return &Detector{
		autoDetect:  true,
		whitelist:   make(map[string]bool),
		blacklist:   make(map[string]bool),
		gracePeriod: 3 * time.Second,
	}
}

// Configure updates detector auto-detect mode and user whitelist/blacklist entries.
func (d *Detector) Configure(autoDetect bool, whitelist, blacklist []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.autoDetect = autoDetect
	d.whitelist = normalizeExeSet(whitelist)
	d.blacklist = normalizeExeSet(blacklist)
}

// RecentApps returns up to the 10 most recently observed non-system foreground processes.
func (d *Detector) RecentApps() []DetectedApp {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]DetectedApp, len(d.recent))
	copy(out, d.recent)
	return out
}

// Evaluate updates and returns the current GameState given foreground info and time now.
func (d *Detector) Evaluate(fg ForegroundInfo, now time.Time) GameState {
	d.mu.Lock()
	defer d.mu.Unlock()

	exe := normalizeExeName(fg.ExeName)
	if exe == "" && fg.ExePath != "" {
		exe = normalizeExeName(filepath.Base(fg.ExePath))
	}

	if exe != "" && !isShellProcess(exe) {
		d.recordRecentLocked(exe, fg.WindowTitle, now)
	}

	// 1. User blacklist (and PureUtils itself) always suppresses.
	if exe == "pureutils" || d.blacklist[exe] {
		d.activeState = GameState{}
		d.lastPositive = time.Time{}
		return d.activeState
	}

	// 2. User whitelist always triggers immediately.
	if exe != "" && d.whitelist[exe] {
		st := GameState{
			Active:  true,
			PID:     fg.PID,
			ExeName: exe,
			Title:   fg.WindowTitle,
			Reason:  "whitelist",
		}
		d.activeState = st
		d.lastPositive = now
		return st
	}

	// 3. Auto-detection off => only whitelist triggers.
	if !d.autoDetect {
		d.activeState = GameState{}
		d.lastPositive = time.Time{}
		return d.activeState
	}

	// 4. Built-in non-game exclusions.
	if exe == "" || defaultIgnoredProcesses[exe] {
		return d.hysteresisOrClearLocked(now)
	}

	// 5. Multi-signal game heuristic.
	score := ScoreGameCandidate(fg)
	if score >= 55 {
		st := GameState{
			Active:  true,
			PID:     fg.PID,
			ExeName: exe,
			Title:   fg.WindowTitle,
			Reason:  "auto",
		}
		d.activeState = st
		d.lastPositive = now
		return st
	}

	return d.hysteresisOrClearLocked(now)
}

func (d *Detector) hysteresisOrClearLocked(now time.Time) GameState {
	if d.activeState.Active && !d.lastPositive.IsZero() && now.Sub(d.lastPositive) <= d.gracePeriod {
		return d.activeState
	}
	d.activeState = GameState{}
	return d.activeState
}

func (d *Detector) recordRecentLocked(exe, title string, now time.Time) {
	item := DetectedApp{ExeName: exe, Title: title, SeenAt: now}
	filtered := make([]DetectedApp, 0, 10)
	filtered = append(filtered, item)
	for _, r := range d.recent {
		if r.ExeName == exe {
			continue
		}
		filtered = append(filtered, r)
		if len(filtered) >= 10 {
			break
		}
	}
	d.recent = filtered
}

// ScoreGameCandidate computes a confidence score [0..120] indicating whether fg is a running game.
func ScoreGameCandidate(fg ForegroundInfo) int {
	// Reject pure video playback (high video decode, low 3D).
	if fg.GPUVideoPct > 12 && fg.GPU3DPct < 15 && !fg.D3DFullscreen {
		return 0
	}

	score := 0
	if fg.D3DFullscreen {
		score += 50
	} else if fg.Fullscreen {
		score += 35
	}

	switch {
	case fg.GPU3DPct >= 25:
		score += 45
	case fg.GPU3DPct >= 10:
		score += 30
	case fg.GPU3DPct >= 3:
		score += 15
	}

	if fg.FPS >= 15 {
		score += 30
	} else if fg.FPS >= 5 {
		score += 15
	}

	if fg.ExePath != "" {
		lp := strings.ToLower(fg.ExePath)
		for _, hint := range knownGamePathHints {
			if strings.Contains(lp, hint) {
				score += 25
				break
			}
		}
	}
	return score
}

func normalizeExeSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, raw := range list {
		if n := normalizeExeName(raw); n != "" {
			out[n] = true
		}
	}
	return out
}

func normalizeExeName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	if strings.ContainsAny(s, `\/`) {
		s = filepath.Base(s)
	}
	s = strings.TrimSuffix(s, ".exe")
	return strings.TrimSpace(s)
}

func isShellProcess(exe string) bool {
	switch exe {
	case "explorer", "searchhost", "shellexperiencehost", "startmenuexperiencehost", "lockapp", "dwm":
		return true
	default:
		return false
	}
}
