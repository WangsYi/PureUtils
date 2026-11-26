//go:build windows

package purevideo

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/config"
	pureui "pureutils/pkg/purekit/ui"
)

const transportBarDIP = 68

var speedPresets = []float64{0.5, 0.75, 1.0, 1.25, 1.5, 2.0}

// PlayerCallbacks connects Player events to the PureVideo Plugin host wrapper.
type PlayerCallbacks struct {
	PluginDir      string
	OnPrefsChange  func(config.PureVideoConfig)
	OnOpenSettings func()
	OnAllClosed    func()
	Logf           func(format string, args ...any)
	Headless       bool
	BackendFactory func(engine, pluginDir string, hwnd uintptr, cb BackendCallbacks) (PlaybackBackend, string, error)
}

// Player implements the PureVideo fast video browser window.
type Player struct {
	mu    sync.Mutex
	prefs config.PureVideoConfig
	cb    PlayerCallbacks

	win   *mygo.Window
	stage *winStage

	backend      PlaybackBackend
	activeEngine string
	backendSeq   uint64
	pendingSeek  time.Duration

	dir         string
	items       []string
	idx         int
	path        string
	displayName string

	state  PlaybackState
	pos    time.Duration
	dur    time.Duration
	natW   int
	natH   int
	volume int
	muted  bool
	loop   bool
	speed  float64

	errText   string
	toast     string
	toastTill time.Time

	fullscreen   bool
	scrubbing    bool
	scrubFrac    float64
	hoverScrub   bool
	hoverFrac    float64
	volScrubbing bool
	progressBarW float32
	volumeBarW   float32

	tickStop chan struct{}
	closed   atomic.Bool
}

// NewPlayer creates a PureVideo Player instance.
func NewPlayer(prefs config.PureVideoConfig, cb PlayerCallbacks) *Player {
	prefs.Normalize()
	if cb.BackendFactory == nil {
		cb.BackendFactory = defaultBackendFactory
	}
	return &Player{
		prefs:  prefs,
		cb:     cb,
		volume: prefs.Volume,
		muted:  prefs.Muted,
		loop:   prefs.Loop,
		speed:  prefs.DefaultSpeed,
	}
}

func defaultBackendFactory(_, pluginDir string, hwnd uintptr, cb BackendCallbacks) (PlaybackBackend, string, error) {
	dllPath, ok := FindLibMPV(pluginDir)
	if !ok {
		return nil, "", fmt.Errorf("未找到 libmpv-2.dll，请在「设置 → PureVideo」中下载或从本地导入")
	}
	b, err := newMPVBackend(dllPath, hwnd, cb)
	if err != nil {
		return nil, "", err
	}
	return b, EngineMPV, nil
}

// Open opens path (a video file or folder) or focuses an empty window when path is "".
func (p *Player) Open(path string) {
	if p.closed.Load() {
		return
	}
	trimmed := strings.TrimSpace(strings.Trim(path, `"`))
	if !p.cb.Headless {
		p.ensureWindow()
	}
	if trimmed == "" {
		p.invalidate()
		return
	}
	p.OpenPath(trimmed)
}

// OpenPath loads a file or scans a directory and opens its first video.
func (p *Player) OpenPath(path string) {
	if p.closed.Load() {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	info, err := os.Stat(path)
	if err != nil {
		p.setError("无法访问路径: " + err.Error())
		return
	}
	if info.IsDir() {
		items := ScanVideoDir(path, p.prefs.Sort, p.prefs.SortDesc)
		p.dir = path
		p.items = items
		p.idx = 0
		if len(items) == 0 {
			p.path = ""
			p.displayName = ""
			p.showToast("该文件夹中未找到支持的视频文件", 2500*time.Millisecond)
			p.syncStage()
			p.invalidate()
			return
		}
		p.loadItem(items[0])
		return
	}

	dir := filepath.Dir(path)
	items := ScanVideoDir(dir, p.prefs.Sort, p.prefs.SortDesc)
	idx := -1
	for i, it := range items {
		if strings.EqualFold(it, path) {
			idx = i
			break
		}
	}
	if idx < 0 {
		items = append([]string{path}, items...)
		idx = 0
	}
	p.dir = dir
	p.items = items
	p.idx = idx
	p.loadItem(path)
}

func (p *Player) loadItem(path string) {
	p.path = path
	p.displayName = filepath.Base(path)
	p.errText = ""
	p.pos = 0
	p.dur = 0
	p.natW, p.natH = 0, 0
	p.pendingSeek = 0
	p.state = StateLoading
	p.updateTitle()

	if err := p.ensureBackend(); err != nil {
		p.setError(err.Error())
		return
	}

	_ = p.backend.SetVolume(float32(p.volume) / 100.0)
	_ = p.backend.SetMute(p.muted)
	_ = p.backend.SetRate(float32(p.speed))

	p.syncStage()
	p.invalidate()

	if err := p.backend.Open(path, p.prefs.AutoPlay); err != nil {
		p.setError(err.Error())
		return
	}
	if p.prefs.AutoPlay {
		p.startTicker()
	}
}

func (p *Player) ensureWindow() {
	if p.win != nil {
		if p.win.IsMinimized() {
			p.win.Restore()
		}
		p.win.Show()
		p.win.Focus()
		p.syncStage()
		return
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "PureVideo",
		Width:           1040,
		Height:          680,
		MinWidth:        560,
		MinHeight:       380,
		BackgroundColor: "#0B0D12",
		Content:         ui.View(p.View),
	})
	_ = win.SetIcon(videoIconPNG)
	hwnd := win.NativeHandle()
	applyDarkTitleBar(hwnd)
	applyVideoTaskbarIdentity(hwnd, videoIconPNG)

	st, err := newWinStage(stageCallbacks{
		ParentHWND: hwnd,
		OnRepaint: func() {
			if p.backend != nil {
				p.backend.Repaint()
			}
		},
		OnResize: func(w, h int) {
			if p.backend != nil {
				p.backend.Resize(w, h)
			}
		},
		OnPlayPause:  p.TogglePlayPause,
		OnFullScreen: p.ToggleFullScreen,
		OnWheel: func(steps int) {
			p.SetVolume(p.volume + steps*5)
			p.showToast(fmt.Sprintf("音量 %d%%", p.volume), 1200*time.Millisecond)
		},
		OnKey: func(key ui.Key, mods ui.Modifiers) {
			p.handleKey(key, mods)
		},
	})
	if err == nil {
		p.stage = st
	} else if p.cb.Logf != nil {
		p.cb.Logf("创建视频舞台窗口失败: %v", err)
	}

	win.OnResize(func() { p.syncStage() })
	win.OnEnterFullScreen(func() {
		p.fullscreen = true
		p.syncStage()
		p.invalidate()
	})
	win.OnLeaveFullScreen(func() {
		p.fullscreen = false
		p.syncStage()
		p.invalidate()
	})
	win.OnFileDrop(func(e *mygo.FileDropEvent) {
		if e != nil && len(e.Paths) > 0 {
			p.OpenPath(e.Paths[0])
		}
	})
	win.OnClosed(func() {
		p.stopTicker()
		p.closeBackend()
		if p.stage != nil {
			p.stage.Destroy()
			p.stage = nil
		}
		p.win = nil
		p.fullscreen = false
		p.state = StateEmpty
		p.path = ""
		p.errText = ""
		if p.cb.OnAllClosed != nil {
			p.cb.OnAllClosed()
		}
	})

	p.win = win
	win.Show()
	win.Focus()
	p.syncStage()
}

func (p *Player) ensureBackend() error {
	if p.backend != nil {
		return nil
	}
	return p.recreateBackend(EngineMPV)
}

func (p *Player) recreateBackend(requested string) error {
	p.closeBackend()
	p.backendSeq++
	seq := p.backendSeq

	var stageHWND uintptr
	if p.stage != nil {
		stageHWND = p.stage.HWND()
	}

	cb := BackendCallbacks{
		OnReady: func() {
			p.runUI(func() { p.onBackendReady(seq) })
		},
		OnState: func(st PlaybackState) {
			p.runUI(func() { p.onBackendState(seq, st) })
		},
		OnEnded: func() {
			p.runUI(func() { p.onBackendEnded(seq) })
		},
		OnError: func(code uint32, msg string) {
			p.runUI(func() { p.onBackendError(seq, code, msg) })
		},
	}

	b, actual, err := p.cb.BackendFactory(requested, p.cb.PluginDir, stageHWND, cb)
	if err != nil {
		return err
	}
	p.backend = b
	p.activeEngine = actual
	if p.cb.Logf != nil {
		p.cb.Logf("初始化播放内核: requested=%s actual=%s", requested, actual)
	}
	return nil
}

func (p *Player) runUI(fn func()) {
	if p.closed.Load() {
		return
	}
	if p.cb.Headless {
		fn()
		return
	}
	mygo.RunOnMain(fn)
}

func (p *Player) onBackendReady(seq uint64) {
	if seq != p.backendSeq || p.backend == nil {
		return
	}
	p.dur = p.backend.Duration()
	p.natW, p.natH = p.backend.NativeSize()
	p.errText = ""
	if p.pendingSeek > 0 {
		target := p.pendingSeek
		p.pendingSeek = 0
		_ = p.backend.Seek(target)
		p.pos = target
	}
	if p.prefs.AutoPlay || p.state == StatePlaying {
		p.state = StatePlaying
		p.startTicker()
	} else {
		p.state = StatePaused
	}
	p.syncStage()
	p.invalidate()
}

func (p *Player) onBackendState(seq uint64, st PlaybackState) {
	if seq != p.backendSeq || p.backend == nil {
		return
	}
	p.state = st
	if st == StatePlaying {
		p.startTicker()
	} else if st == StatePaused || st == StateStopped || st == StateError {
		p.stopTicker()
		if st == StatePaused || st == StateStopped {
			p.pos = p.backend.Position()
		}
	}
	p.syncStage()
	p.invalidate()
}

func (p *Player) onBackendEnded(seq uint64) {
	if seq != p.backendSeq || p.backend == nil {
		return
	}
	if p.loop {
		_ = p.backend.Seek(0)
		_ = p.backend.Play()
		p.pos = 0
		p.state = StatePlaying
		p.startTicker()
		p.invalidate()
		return
	}
	p.stopTicker()
	p.state = StatePaused
	if d := p.backend.Duration(); d > 0 {
		p.dur = d
		p.pos = d
	}
	p.invalidate()
}

func (p *Player) onBackendError(seq uint64, _ uint32, msg string) {
	if seq != p.backendSeq {
		return
	}
	p.setError(msg)
}

func (p *Player) setError(msg string) {
	p.stopTicker()
	p.state = StateError
	p.errText = msg
	p.syncStage()
	p.invalidate()
}

func (p *Player) closeBackend() {
	if p.backend != nil {
		b := p.backend
		p.backend = nil
		p.activeEngine = ""
		b.Close()
	}
}

func (p *Player) syncStage() {
	if p.stage == nil || p.win == nil {
		return
	}
	show := p.path != "" && p.errText == "" && p.state != StateError
	p.stage.SyncBounds(transportBarDIP, show)
}

func (p *Player) updateTitle() {
	if p.win == nil {
		return
	}
	if p.displayName == "" {
		p.win.SetTitle("PureVideo")
	} else {
		p.win.SetTitle(p.displayName + " - PureVideo")
	}
}

func (p *Player) startTicker() {
	if p.cb.Headless || p.tickStop != nil {
		return
	}
	stop := make(chan struct{})
	p.tickStop = stop
	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if p.closed.Load() {
					return
				}
				mygo.RunOnMain(func() {
					if p.backend != nil && p.state == StatePlaying && !p.scrubbing {
						p.pos = p.backend.Position()
						if p.dur <= 0 {
							p.dur = p.backend.Duration()
						}
						if p.natW == 0 {
							p.natW, p.natH = p.backend.NativeSize()
						}
						p.invalidate()
					}
				})
			}
		}
	}()
}

func (p *Player) stopTicker() {
	if p.tickStop != nil {
		close(p.tickStop)
		p.tickStop = nil
	}
}

// TogglePlayPause switches between Playing and Paused.
func (p *Player) TogglePlayPause() {
	if p.backend == nil || p.path == "" || p.errText != "" {
		return
	}
	if p.state == StatePlaying {
		_ = p.backend.Pause()
		p.state = StatePaused
		p.stopTicker()
	} else {
		if p.dur > 0 && p.pos >= p.dur-200*time.Millisecond {
			_ = p.backend.Seek(0)
			p.pos = 0
		}
		_ = p.backend.Play()
		p.state = StatePlaying
		p.startTicker()
	}
	p.invalidate()
}

// SeekBy steps relative to current position clamped inside [0, dur].
func (p *Player) SeekBy(delta time.Duration) {
	if p.backend == nil || p.path == "" {
		return
	}
	p.SeekTo(p.pos + delta)
}

// SeekTo jumps playback to target clamped to [0, dur].
func (p *Player) SeekTo(target time.Duration) {
	if p.backend == nil || p.path == "" {
		return
	}
	if target < 0 {
		target = 0
	}
	if p.dur > 0 && target > p.dur {
		target = p.dur
	}
	p.pos = target
	_ = p.backend.Seek(target)
	p.invalidate()
}

// FrameStep steps forward a single frame and pauses.
func (p *Player) FrameStep() {
	if p.backend == nil || p.path == "" {
		return
	}
	p.stopTicker()
	if err := p.backend.FrameStep(); err == nil {
		p.state = StatePaused
		p.pos = p.backend.Position()
		p.invalidate()
	}
}

// Next moves to the next video in the active folder.
func (p *Player) Next() {
	if len(p.items) <= 1 {
		return
	}
	p.idx = (p.idx + 1) % len(p.items)
	p.loadItem(p.items[p.idx])
}

// Prev moves to the previous video in the active folder.
func (p *Player) Prev() {
	if len(p.items) <= 1 {
		return
	}
	p.idx = (p.idx - 1 + len(p.items)) % len(p.items)
	p.loadItem(p.items[p.idx])
}

// SetVolume updates playback volume [0..100] and persists when RememberVolume is set.
func (p *Player) SetVolume(vol int) {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	p.volume = vol
	if vol > 0 && p.muted {
		p.muted = false
		if p.backend != nil {
			_ = p.backend.SetMute(false)
		}
	}
	if p.backend != nil {
		_ = p.backend.SetVolume(float32(vol) / 100.0)
	}
	p.persistPlaybackPrefs()
	p.invalidate()
}

// ToggleMute toggles audio mute state.
func (p *Player) ToggleMute() {
	p.muted = !p.muted
	if p.backend != nil {
		_ = p.backend.SetMute(p.muted)
	}
	p.persistPlaybackPrefs()
	p.invalidate()
}

// ToggleLoop toggles single-file repeat.
func (p *Player) ToggleLoop() {
	p.loop = !p.loop
	p.prefs.Loop = p.loop
	if p.cb.OnPrefsChange != nil {
		p.cb.OnPrefsChange(p.prefs)
	}
	if p.loop {
		p.showToast("已开启循环播放", 1200*time.Millisecond)
	} else {
		p.showToast("已关闭循环播放", 1200*time.Millisecond)
	}
	p.invalidate()
}

// SetSpeed changes playback speed multiplier.
func (p *Player) SetSpeed(rate float64) {
	if rate < 0.25 {
		rate = 0.25
	}
	if rate > 4.0 {
		rate = 4.0
	}
	p.speed = rate
	if p.backend != nil {
		_ = p.backend.SetRate(float32(rate))
	}
	p.invalidate()
}

// CycleSpeed cycles through speedPresets.
func (p *Player) CycleSpeed() {
	next := speedPresets[0]
	for i, s := range speedPresets {
		if math.Abs(s-p.speed) < 0.05 {
			next = speedPresets[(i+1)%len(speedPresets)]
			break
		}
		if s > p.speed {
			next = s
			break
		}
	}
	p.SetSpeed(next)
}

func (p *Player) persistPlaybackPrefs() {
	if !p.prefs.RememberVolume {
		return
	}
	p.prefs.Volume = p.volume
	p.prefs.Muted = p.muted
	if p.cb.OnPrefsChange != nil {
		p.cb.OnPrefsChange(p.prefs)
	}
}

// ReleaseBackend stops playback and frees the active libmpv instance so libmpv-2.dll can be replaced or removed.
func (p *Player) ReleaseBackend() {
	p.stopTicker()
	p.closeBackend()
	if p.state == StatePlaying {
		p.state = StatePaused
	}
	p.invalidate()
}

// Reload recreates the libmpv backend and reopens the active video at the current position.
func (p *Player) Reload() {
	if p.path == "" {
		p.closeBackend()
		p.invalidate()
		return
	}
	resumeAt := p.pos
	wasPlaying := p.state == StatePlaying || p.prefs.AutoPlay
	if err := p.recreateBackend(EngineMPV); err != nil {
		p.setError(err.Error())
		return
	}
	p.pendingSeek = resumeAt
	p.errText = ""
	p.state = StateLoading
	_ = p.backend.SetVolume(float32(p.volume) / 100.0)
	_ = p.backend.SetMute(p.muted)
	_ = p.backend.SetRate(float32(p.speed))
	p.syncStage()
	p.invalidate()
	if err := p.backend.Open(p.path, wasPlaying); err != nil {
		p.setError(err.Error())
		return
	}
	p.showToast("已重新加载 libmpv 内核", 1500*time.Millisecond)
}

// ApplySettings synchronizes settings updated from the PureUtils Settings window.
func (p *Player) ApplySettings(next config.PureVideoConfig) {
	next.Normalize()
	before := p.prefs
	p.prefs = next
	p.loop = next.Loop
	if before.DefaultSpeed != next.DefaultSpeed {
		p.SetSpeed(next.DefaultSpeed)
	}
	if before.Sort != next.Sort || before.SortDesc != next.SortDesc {
		if p.dir != "" {
			p.rescanDir()
		}
	}
	p.invalidate()
}

func (p *Player) rescanDir() {
	if p.dir == "" {
		return
	}
	items := ScanVideoDir(p.dir, p.prefs.Sort, p.prefs.SortDesc)
	p.items = items
	for i, it := range items {
		if strings.EqualFold(it, p.path) {
			p.idx = i
			break
		}
	}
}

// ToggleFullScreen switches borderless fullscreen on the player window.
func (p *Player) ToggleFullScreen() {
	if p.win == nil || p.cb.Headless {
		return
	}
	p.win.ToggleFullScreen()
}

func (p *Player) pickFile() {
	if p.cb.Headless || p.win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  p.win,
			Title:   "选择视频文件",
			Filters: VideoFileFilters(),
		})
		if err != nil || len(paths) == 0 {
			return
		}
		mygo.RunOnMain(func() { p.OpenPath(paths[0]) })
	}()
}

func (p *Player) pickFolder() {
	if p.cb.Headless || p.win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    p.win,
			Title:     "选择视频文件夹",
			Directory: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		mygo.RunOnMain(func() { p.OpenPath(paths[0]) })
	}()
}

func (p *Player) showToast(msg string, d time.Duration) {
	p.toast = msg
	p.toastTill = time.Now().Add(d)
	p.invalidate()
}

func (p *Player) invalidate() {
	if p.win != nil && !p.cb.Headless {
		p.win.Invalidate()
	}
}

// IsOpen reports whether the PureVideo player window is currently open.
func (p *Player) IsOpen() bool {
	return p.win != nil
}

// Shutdown releases playback backends, child stage HWND, ticker, and window.
func (p *Player) Shutdown() {
	if p.closed.Swap(true) {
		return
	}
	p.stopTicker()
	p.closeBackend()
	if p.stage != nil {
		p.stage.Destroy()
		p.stage = nil
	}
	if p.win != nil && !p.cb.Headless {
		w := p.win
		p.win = nil
		w.Close()
	}
}

func (p *Player) handleKey(key ui.Key, mods ui.Modifiers) {
	switch key {
	case ui.KeySpace:
		p.TogglePlayPause()
	case ui.KeyLeft:
		if mods&ui.Shift != 0 {
			p.SeekBy(-30 * time.Second)
		} else {
			p.SeekBy(-5 * time.Second)
		}
	case ui.KeyRight:
		if mods&ui.Shift != 0 {
			p.SeekBy(30 * time.Second)
		} else {
			p.SeekBy(5 * time.Second)
		}
	case ui.KeyUp:
		p.SetVolume(p.volume + 5)
		p.showToast(fmt.Sprintf("音量 %d%%", p.volume), 1200*time.Millisecond)
	case ui.KeyDown:
		p.SetVolume(p.volume - 5)
		p.showToast(fmt.Sprintf("音量 %d%%", p.volume), 1200*time.Millisecond)
	case ui.KeyPageUp, ui.KeyBracketLeft:
		p.Prev()
	case ui.KeyPageDown, ui.KeyBracketRight:
		p.Next()
	case ui.KeyF, ui.KeyF11:
		p.ToggleFullScreen()
	case ui.KeyM:
		p.ToggleMute()
	case ui.KeyL:
		p.ToggleLoop()
	case ui.KeyO:
		p.pickFile()
	case ui.KeyPeriod:
		p.FrameStep()
	case ui.KeyEscape:
		if p.fullscreen {
			p.ToggleFullScreen()
		} else if p.win != nil && !p.cb.Headless {
			p.win.Close()
		}
	}
}

func (p *Player) handleShortcuts(c *ui.Context) {
	switch {
	case c.Shortcut(0, ui.KeySpace):
		p.TogglePlayPause()
	case c.Shortcut(ui.Shift, ui.KeyLeft):
		p.SeekBy(-30 * time.Second)
	case c.Shortcut(ui.Shift, ui.KeyRight):
		p.SeekBy(30 * time.Second)
	case c.Shortcut(0, ui.KeyLeft):
		p.SeekBy(-5 * time.Second)
	case c.Shortcut(0, ui.KeyRight):
		p.SeekBy(5 * time.Second)
	case c.Shortcut(0, ui.KeyUp):
		p.SetVolume(p.volume + 5)
		p.showToast(fmt.Sprintf("音量 %d%%", p.volume), 1200*time.Millisecond)
	case c.Shortcut(0, ui.KeyDown):
		p.SetVolume(p.volume - 5)
		p.showToast(fmt.Sprintf("音量 %d%%", p.volume), 1200*time.Millisecond)
	case c.Shortcut(0, ui.KeyPageUp) || c.Shortcut(0, ui.KeyBracketLeft):
		p.Prev()
	case c.Shortcut(0, ui.KeyPageDown) || c.Shortcut(0, ui.KeyBracketRight):
		p.Next()
	case c.Shortcut(0, ui.KeyF) || c.Shortcut(0, ui.KeyF11):
		p.ToggleFullScreen()
	case c.Shortcut(0, ui.KeyM):
		p.ToggleMute()
	case c.Shortcut(0, ui.KeyL):
		p.ToggleLoop()
	case c.Shortcut(0, ui.KeyO):
		p.pickFile()
	case c.Shortcut(0, ui.KeyPeriod):
		p.FrameStep()
	case c.Shortcut(0, ui.KeyEscape):
		if p.fullscreen {
			p.ToggleFullScreen()
		} else if p.win != nil && !p.cb.Headless {
			p.win.Close()
		}
	}
}

func applyVideoTheme(c *ui.Context) pureui.Palette {
	pal := pureui.PaletteFor(true)
	base := ui.DarkTheme()
	t := *base
	t.Dark = true
	t.Background = pal.BgInset
	t.Surface = pal.ControlBg
	t.SurfaceHover = pal.CardHover
	t.SurfacePressed = pal.CardActive
	t.Border = pal.Border
	t.Text = pal.Text
	t.TextMuted = pal.TextMuted
	t.Accent = pal.Accent
	t.AccentHover = pal.AccentHover
	t.AccentPressed = pal.AccentPressed
	t.AccentText = pal.AccentText
	t.Radius = 8
	c.SetTheme(&t)
	return pal
}

// View renders the PureVideo player UI (empty state, error recovery state, and bottom transport bar).
func (p *Player) View(c *ui.Context) {
	pal := applyVideoTheme(c)
	p.handleShortcuts(c)

	ui.Column(c).Fill().Background(pal.BgInset).Children(func() {
		switch {
		case p.errText != "":
			p.renderErrorState(c, pal)
		case p.path == "":
			p.renderEmptyState(c, pal)
		default:
			ui.Column(c).Grow(1).MinHeight(0).Center().Padding(24).Gap(8).Children(func() {
				if p.state == StateLoading {
					ui.Text(c, "正在加载 "+p.displayName+"…").FontSize(13).TextColor(pal.TextMuted)
				} else {
					ui.Text(c, p.displayName).FontSize(14).FontWeight(500).TextColor(pal.TextSecondary)
					if p.natW > 0 && p.natH > 0 {
						ui.Text(c, fmt.Sprintf("%d × %d · %s", p.natW, p.natH, p.engineLabel())).
							FontSize(11.5).TextColor(pal.TextMuted)
					}
				}
			})
			p.renderTransportBar(c, pal)
		}
	})

	if p.toast != "" && time.Now().Before(p.toastTill) {
		ui.Overlay(c, func() {
			ui.Row(c).Absolute().Left(0).Right(0).Bottom(transportBarDIP+14).Justify(ui.Center).PassThrough().Children(func() {
				ui.Box(c).Padding(6, 12).Radius(999).Background(ui.RGBA(20, 24, 34, 0.92)).Children(func() {
					ui.Text(c, p.toast).FontSize(11.5).FontWeight(500).TextColor(pal.HudText)
				})
			})
		})
	}
}

func (p *Player) renderEmptyState(c *ui.Context, pal pureui.Palette) {
	engineHint := "未检测到 libmpv-2.dll，请前往设置页下载或导入内核"
	if _, ok := FindLibMPV(p.cb.PluginDir); ok {
		engineHint = "内置 libmpv 硬件解码内核（支持 MP4 / MKV / MOV / HEVC / FLV / 外挂字幕）"
	}

	ui.Column(c).Fill().Center().Padding(28).Gap(14).Children(func() {
		ui.Box(c).Size(68, 68).Radius(34).Background(pal.AccentSoft).Center().Children(func() {
			ui.Box(c).Size(48, 48).Radius(24).Gradient(pal.Accent, pal.AccentEnd, 135).
				Shadow(0, 6, 18, 0, pal.Accent.Alpha(0.30)).Center().Children(func() {
				ui.Icon(c, pureui.IconFilm).Size(22, 22).TextColor(pal.AccentText)
			})
		})
		ui.Column(c).AlignItems(ui.Center).Gap(5).Children(func() {
			ui.Text(c, "PureVideo 极速视频浏览").FontSize(16).FontWeight(600).TextColor(pal.Text)
			ui.Text(c, "拖拽视频或文件夹到此窗口即可秒开 · "+engineHint).
				FontSize(11.5).TextColor(pal.TextMuted).TextAlign(ui.Center).MaxWidth(460)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.PrimaryButton(c, "打开视频文件…").OnClick(p.pickFile)
			ui.Button(c, "打开文件夹…").OnClick(p.pickFolder).Border(0, ui.Transparent)
			if p.cb.OnOpenSettings != nil {
				ui.Button(c, "播放与内核设置").OnClick(p.cb.OnOpenSettings).Border(0, ui.Transparent)
			}
		})
		ui.Row(c).Gap(8).Margin(4, 0, 0, 0).AlignItems(ui.Center).Children(func() {
			pureui.Keycap(c, "Space", pal)
			ui.Text(c, "播放/暂停").FontSize(11).TextColor(pal.TextMuted)
			pureui.Keycap(c, "←/→", pal)
			ui.Text(c, "快进退").FontSize(11).TextColor(pal.TextMuted)
			pureui.Keycap(c, "PgUp/PgDn", pal)
			ui.Text(c, "切换视频").FontSize(11).TextColor(pal.TextMuted)
			pureui.Keycap(c, "F", pal)
			ui.Text(c, "全屏").FontSize(11).TextColor(pal.TextMuted)
		})
	})
}

func (p *Player) renderErrorState(c *ui.Context, pal pureui.Palette) {
	_, mpvInstalled := FindLibMPV(p.cb.PluginDir)
	ui.Column(c).Grow(1).MinHeight(0).Center().Padding(28).Gap(14).Children(func() {
		ui.Box(c).Size(56, 56).Radius(28).Background(pal.DangerSoft).Center().Children(func() {
			ui.Icon(c, pureui.IconAlertCircle).Size(24, 24).TextColor(pal.Danger)
		})
		ui.Column(c).AlignItems(ui.Center).Gap(6).MaxWidth(480).Children(func() {
			ui.Text(c, "无法播放 "+p.displayName).FontSize(15).FontWeight(600).TextColor(pal.Text)
			ui.Text(c, p.errText).FontSize(12).LineHeight(1.45).TextColor(pal.TextSecondary).TextAlign(ui.Center)
		})
		ui.Row(c).Gap(8).Wrap().Justify(ui.Center).Children(func() {
			if !mpvInstalled && p.cb.OnOpenSettings != nil {
				ui.PrimaryButton(c, "前往下载 / 导入 libmpv 内核").OnClick(p.cb.OnOpenSettings)
			} else if p.path != "" {
				ui.PrimaryButton(c, "重新加载").OnClick(p.Reload)
			}
			if p.path != "" && !p.cb.Headless {
				ui.Button(c, "用系统默认程序打开").OnClick(func() {
					_ = mygo.Shell.OpenPath(p.path)
				}).Border(0, ui.Transparent)
			}
			ui.Button(c, "打开其他视频…").OnClick(p.pickFile).Border(0, ui.Transparent)
		})
	})
	p.renderTransportBar(c, pal)
}

func (p *Player) engineLabel() string {
	return "libmpv"
}

func (p *Player) renderTransportBar(c *ui.Context, pal pureui.Palette) {
	ui.Column(c).Height(transportBarDIP).FillWidth().Background(pal.SidebarBg).Children(func() {
		ui.Box(c).Height(1).FillWidth().Background(ui.RGBA(255, 255, 255, 0.06))
		ui.Column(c).Fill().Padding(4, 14, 6, 14).Gap(4).Children(func() {
			p.renderProgressBar(c, pal)
			p.renderControlsRow(c, pal)
		})
	})
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func (p *Player) renderProgressBar(c *ui.Context, pal pureui.Palette) {
	frac := 0.0
	if p.scrubbing {
		frac = p.scrubFrac
	} else if p.dur > 0 {
		frac = float64(p.pos) / float64(p.dur)
	}
	frac = clamp01(frac)

	bar := ui.Box(c).Height(16).FillWidth().Cursor(ui.CursorPointer).Label("播放进度条")
	if bw := bar.Bounds().W; bw > 0 {
		p.progressBarW = bw
	}
	bar.HandleInput(func(ev ui.InputEvent) bool {
		w := float64(p.progressBarW)
		if w <= 0 {
			return false
		}
		calc := func(x float32) float64 {
			return clamp01(float64(x) / w)
		}
		switch ev.Kind {
		case ui.InputPointerMove:
			p.hoverScrub = true
			p.hoverFrac = calc(ev.X)
			if p.scrubbing {
				p.scrubFrac = p.hoverFrac
				p.invalidate()
				return true
			}
			return false
		case ui.InputPointerDown:
			if ev.Button == 0 && p.dur > 0 {
				p.scrubbing = true
				p.scrubFrac = calc(ev.X)
				p.SeekTo(time.Duration(p.scrubFrac * float64(p.dur)))
				return true
			}
		case ui.InputPointerUp:
			if ev.Button == 0 && p.scrubbing {
				p.scrubFrac = calc(ev.X)
				p.scrubbing = false
				if p.dur > 0 {
					p.SeekTo(time.Duration(p.scrubFrac * float64(p.dur)))
				}
				return true
			}
		}
		return false
	})

	hovered := bar.Hovered()
	if !hovered && p.hoverScrub {
		p.hoverScrub = false
	}
	bar.Draw(func(painter *ui.Painter, r ui.Rect) {
		if r.W > 0 {
			p.progressBarW = r.W
		}
		trackH := float32(4)
		if hovered || p.scrubbing {
			trackH = 6
		}
		ty := r.Y + (r.H-trackH)/2
		painter.Fill(ui.Rect{X: r.X, Y: ty, W: r.W, H: trackH}, ui.RGBA(255, 255, 255, 0.14), trackH/2)
		fillW := r.W * float32(frac)
		if fillW > 0 {
			painter.Fill(ui.Rect{X: r.X, Y: ty, W: fillW, H: trackH}, pal.Accent, trackH/2)
		}
		if hovered || p.scrubbing {
			thumb := float32(12)
			cx := r.X + fillW - thumb/2
			if cx < r.X {
				cx = r.X
			}
			if cx > r.X+r.W-thumb {
				cx = r.X + r.W - thumb
			}
			cy := r.Y + (r.H-thumb)/2
			painter.Fill(ui.Rect{X: cx, Y: cy, W: thumb, H: thumb}, ui.RGB(255, 255, 255), thumb/2)
		}
	})
}

func (p *Player) renderControlsRow(c *ui.Context, pal pureui.Palette) {
	ui.Row(c).FillWidth().Grow(1).AlignItems(ui.Center).Gap(6).Children(func() {
		pureui.IconButton(c, pureui.IconSkipBack, "上一个", "上一个视频 (PgUp / [)", false, false, pal).
			Disabled(len(p.items) <= 1).OnClick(p.Prev)

		playIcon := pureui.IconPlay
		playLabel := "播放"
		if p.state == StatePlaying {
			playIcon = pureui.IconPause
			playLabel = "暂停"
		}
		playBtn := ui.ButtonBase(c).Size(32, 32).Radius(9).Center().Label(playLabel).Tooltip(playLabel + " (Space)")
		bg := pal.Accent
		if playBtn.Pressed() {
			bg = pal.AccentPressed
		} else if playBtn.Hovered() {
			bg = pal.AccentHover
		}
		playBtn.Background(bg).Children(func() {
			ui.Icon(c, playIcon).Size(15, 15).TextColor(pal.AccentText)
		})
		playBtn.OnClick(p.TogglePlayPause)

		pureui.IconButton(c, pureui.IconSkipForward, "下一个", "下一个视频 (PgDn / ])", false, false, pal).
			Disabled(len(p.items) <= 1).OnClick(p.Next)
		pureui.IconButton(c, pureui.IconStepForward, "逐帧步进", "前进一帧 (.)", false, false, pal).
			OnClick(p.FrameStep)

		timeText := FormatDuration(p.pos) + " / " + FormatDuration(p.dur)
		ui.Text(c, timeText).FontSize(12).FontWeight(500).SingleLine().TextColor(pal.TextSecondary).Margin(0, 4, 0, 6)

		var metaParts []string
		if p.natW > 0 && p.natH > 0 {
			metaParts = append(metaParts, fmt.Sprintf("%d×%d", p.natW, p.natH))
		}
		if len(p.items) > 1 {
			metaParts = append(metaParts, fmt.Sprintf("%d/%d", p.idx+1, len(p.items)))
		}
		metaParts = append(metaParts, p.engineLabel())
		pureui.PillBadge(c, strings.Join(metaParts, " · "), pal.TextSecondary, pal.ControlBg)

		ui.Spacer(c)

		pureui.IconButton(c, pureui.IconRepeat, "循环播放", "单视频循环 (L)", p.loop, false, pal).
			OnClick(p.ToggleLoop)

		spdBtn := ui.ButtonBase(c).Height(26).Padding(0, 9).Radius(7).Center().
			Label("播放倍速").Tooltip("点击切换倍速，或右键/下拉选择")
		spdBg := pal.ControlBg
		if spdBtn.Hovered() {
			spdBg = pal.CardHover
		}
		spdBtn.Background(spdBg).Children(func() {
			ui.Text(c, formatSpeed(p.speed)).FontSize(11).FontWeight(600).SingleLine().TextColor(pal.TextSecondary)
		})
		spdBtn.OnClick(p.CycleSpeed)
		spdBtn.ContextMenu(func(m *ui.Menu) {
			for _, s := range speedPresets {
				if m.Item(formatSpeed(s)).Checked(math.Abs(p.speed-s) < 0.05).Chosen() {
					p.SetSpeed(s)
				}
			}
		})

		volIcon := pureui.IconVolume2
		if p.muted || p.volume == 0 {
			volIcon = pureui.IconVolumeX
		}
		pureui.IconButton(c, volIcon, "静音", "静音/取消静音 (M)", p.muted, false, pal).
			OnClick(p.ToggleMute)
		p.renderVolumeBar(c, pal)

		pureui.IconButton(c, pureui.IconFolder, "打开视频", "打开视频文件 (O)", false, false, pal).
			OnClick(p.pickFile)
		if p.cb.OnOpenSettings != nil {
			pureui.IconButton(c, pureui.IconSettings, "PureVideo 设置", "打开解码内核与播放设置", false, false, pal).
				OnClick(p.cb.OnOpenSettings)
		}
		fsIcon := pureui.IconMaximize
		if p.fullscreen {
			fsIcon = pureui.IconMinimize
		}
		pureui.IconButton(c, fsIcon, "全屏", "切换全屏 (F)", p.fullscreen, false, pal).
			OnClick(p.ToggleFullScreen)
	})
}

func (p *Player) renderVolumeBar(c *ui.Context, pal pureui.Palette) {
	eff := float32(p.volume) / 100.0
	if p.muted {
		eff = 0
	}
	vbar := ui.Box(c).Size(68, 16).Cursor(ui.CursorPointer).Label("音量滑块")
	if bw := vbar.Bounds().W; bw > 0 {
		p.volumeBarW = bw
	}
	vbar.HandleInput(func(ev ui.InputEvent) bool {
		w := float64(p.volumeBarW)
		if w <= 0 {
			w = 68
		}
		apply := func(x float32) {
			f := clamp01(float64(x) / w)
			p.SetVolume(int(math.Round(f * 100)))
		}
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button == 0 {
				p.volScrubbing = true
				apply(ev.X)
				return true
			}
		case ui.InputPointerMove:
			if p.volScrubbing {
				apply(ev.X)
				return true
			}
		case ui.InputPointerUp:
			if ev.Button == 0 && p.volScrubbing {
				p.volScrubbing = false
				apply(ev.X)
				return true
			}
		}
		return false
	})
	vbar.Draw(func(painter *ui.Painter, r ui.Rect) {
		if r.W > 0 {
			p.volumeBarW = r.W
		}
		ty := r.Y + (r.H-4)/2
		painter.Fill(ui.Rect{X: r.X, Y: ty, W: r.W, H: 4}, ui.RGBA(255, 255, 255, 0.14), 2)
		if eff > 0 {
			painter.Fill(ui.Rect{X: r.X, Y: ty, W: r.W * eff, H: 4}, pal.Accent, 2)
		}
	})
}

func formatSpeed(s float64) string {
	switch {
	case math.Abs(s-math.Round(s)) < 0.01:
		return fmt.Sprintf("%.0f×", s)
	case math.Abs(s*10-math.Round(s*10)) < 0.01:
		return fmt.Sprintf("%.1f×", s)
	default:
		return fmt.Sprintf("%.2f×", s)
	}
}
