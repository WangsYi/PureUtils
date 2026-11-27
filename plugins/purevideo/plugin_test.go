//go:build windows

package purevideo

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/pkg/purekit/storage"
)

type mockVideoBackend struct {
	kind        string
	cb          BackendCallbacks
	opened      []string
	openErr     error
	state       PlaybackState
	pos         time.Duration
	dur         time.Duration
	vol         float32
	muted       bool
	rate        float32
	steps       int
	backSteps   int
	tracks      []MediaTrack
	audioTrack  int
	subTrack    int
	addedSub    []string
	subVisible  bool
	subVisSet   bool
	audioDelay  float64
	subDelay    float64
	aspectRatio string
	rotateDeg   int
	flipH       bool
	flipV       bool
	zoom        float64
	deinterlace bool
	abA         float64
	abB         float64
	hwdec       bool
	hwdecSet    bool
	frameImg    image.Image
	info        MediaInfo
	closed      bool
}

func (m *mockVideoBackend) Kind() string { return m.kind }
func (m *mockVideoBackend) Open(path string, autoPlay bool) error {
	m.opened = append(m.opened, path)
	if m.openErr != nil {
		m.state = StateError
		if m.cb.OnError != nil {
			m.cb.OnError(0xC00D36C4, m.openErr.Error())
		}
		return m.openErr
	}
	if autoPlay {
		m.state = StatePlaying
	} else {
		m.state = StatePaused
	}
	if m.cb.OnReady != nil {
		m.cb.OnReady()
	}
	if m.cb.OnState != nil {
		m.cb.OnState(m.state)
	}
	return nil
}
func (m *mockVideoBackend) Play() error {
	m.state = StatePlaying
	return nil
}
func (m *mockVideoBackend) Pause() error {
	m.state = StatePaused
	return nil
}
func (m *mockVideoBackend) Stop() error {
	m.state = StateStopped
	return nil
}
func (m *mockVideoBackend) Seek(pos time.Duration) error {
	m.pos = pos
	return nil
}
func (m *mockVideoBackend) FrameStep() error {
	m.steps++
	m.state = StatePaused
	return nil
}
func (m *mockVideoBackend) FrameBackStep() error {
	m.backSteps++
	m.state = StatePaused
	return nil
}
func (m *mockVideoBackend) SetVolume(vol float32) error {
	m.vol = vol
	return nil
}
func (m *mockVideoBackend) SetMute(mute bool) error {
	m.muted = mute
	return nil
}
func (m *mockVideoBackend) SetRate(rate float32) error {
	m.rate = rate
	return nil
}
func (m *mockVideoBackend) Position() time.Duration { return m.pos }
func (m *mockVideoBackend) Duration() time.Duration { return m.dur }
func (m *mockVideoBackend) NativeSize() (int, int)  { return 1920, 1080 }
func (m *mockVideoBackend) State() PlaybackState    { return m.state }
func (m *mockVideoBackend) Resize(_, _ int)         {}
func (m *mockVideoBackend) Repaint()                {}
func (m *mockVideoBackend) Close()                  { m.closed = true }
func (m *mockVideoBackend) Tracks() []MediaTrack    { return m.tracks }
func (m *mockVideoBackend) SetAudioTrack(id int) error {
	m.audioTrack = id
	return nil
}
func (m *mockVideoBackend) SetSubtitleTrack(id int) error {
	m.subTrack = id
	return nil
}
func (m *mockVideoBackend) AddSubtitle(path string) error {
	m.addedSub = append(m.addedSub, path)
	return nil
}
func (m *mockVideoBackend) SubtitleVisible() bool {
	if !m.subVisSet {
		return true
	}
	return m.subVisible
}
func (m *mockVideoBackend) SetSubtitleVisible(v bool) error {
	m.subVisSet = true
	m.subVisible = v
	return nil
}
func (m *mockVideoBackend) AudioDelay() float64 { return m.audioDelay }
func (m *mockVideoBackend) SetAudioDelay(sec float64) error {
	m.audioDelay = sec
	return nil
}
func (m *mockVideoBackend) SubtitleDelay() float64 { return m.subDelay }
func (m *mockVideoBackend) SetSubtitleDelay(sec float64) error {
	m.subDelay = sec
	return nil
}
func (m *mockVideoBackend) SetAspectRatio(ratio string) error {
	m.aspectRatio = ratio
	return nil
}
func (m *mockVideoBackend) SetRotate(deg int) error {
	m.rotateDeg = deg
	return nil
}
func (m *mockVideoBackend) SetVideoFlip(hflip, vflip bool) error {
	m.flipH = hflip
	m.flipV = vflip
	return nil
}
func (m *mockVideoBackend) SetZoom(zoom float64) error {
	m.zoom = zoom
	return nil
}
func (m *mockVideoBackend) SetDeinterlace(on bool) error {
	m.deinterlace = on
	return nil
}
func (m *mockVideoBackend) SetABLoop(a, b float64) error {
	m.abA = a
	m.abB = b
	return nil
}
func (m *mockVideoBackend) SetHWDec(enabled bool) error {
	m.hwdecSet = true
	m.hwdec = enabled
	return nil
}
func (m *mockVideoBackend) CaptureFrame() (image.Image, error) {
	if m.frameImg != nil {
		return m.frameImg, nil
	}
	return nil, errors.New("no frame")
}
func (m *mockVideoBackend) MediaInfo() MediaInfo { return m.info }

type mockVideoHost struct {
	paths      core.Paths
	cfgMgr     *config.Manager
	storeMgr   *storage.Manager
	bus        *purekit.Bus
	log        *core.Logger
	openedTab  string
	closedCnt  int
	invCount   int
}

func newMockVideoHost(t *testing.T) *mockVideoHost {
	t.Helper()
	home := t.TempDir()
	paths := core.Paths{
		Home:     home,
		DB:       filepath.Join(home, "pureutils.db"),
		Settings: filepath.Join(home, "settings.json"),
		Images:   filepath.Join(home, "images"),
		Logs:     filepath.Join(home, "logs"),
	}
	_ = paths.Ensure()
	storeMgr, err := storage.NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	return &mockVideoHost{
		paths:    paths,
		cfgMgr:   config.Load(paths.Settings),
		storeMgr: storeMgr,
		bus:      purekit.NewBus(),
	}
}

func (h *mockVideoHost) Config(pluginID string) config.Section       { return h.cfgMgr.Section(pluginID) }
func (h *mockVideoHost) HostConfig() config.HostConfig               { return h.cfgMgr.Host() }
func (h *mockVideoHost) SaveHostConfig(cfg config.HostConfig) error  { return h.cfgMgr.SaveHost(cfg) }
func (h *mockVideoHost) Storage(pluginID string) storage.PluginStorage { return h.storeMgr.Plugin(pluginID) }
func (h *mockVideoHost) Clipboard() clipboard.Service                { return nil }
func (h *mockVideoHost) Bus() *purekit.Bus                           { return h.bus }
func (h *mockVideoHost) Log() *core.Logger                           { return h.log }
func (h *mockVideoHost) Notify(string, string)                       {}
func (h *mockVideoHost) Headless() bool                              { return true }
func (h *mockVideoHost) Paths() core.Paths                           { return h.paths }
func (h *mockVideoHost) ModuleEnabled(core.ModuleID) bool            { return true }
func (h *mockVideoHost) ReloadHotkeys()                              {}
func (h *mockVideoHost) RebuildTray()                                {}
func (h *mockVideoHost) OpenSettings(tabID string)                   { h.openedTab = tabID }
func (h *mockVideoHost) ExecuteCommand(core.Command, ...string)      {}
func (h *mockVideoHost) HotkeyError(core.Command) string             { return "" }
func (h *mockVideoHost) SetGlobalHookSuppressed(string, bool)        {}
func (h *mockVideoHost) IsGlobalHookSuppressed() bool                { return false }
func (h *mockVideoHost) NotifyWindowClosed()                         { h.closedCnt++ }
func (h *mockVideoHost) InvalidateSettings()                         { h.invCount++ }

func TestPlayerStateMachineAndNavigation(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"01.mp4", "02.mkv", "03.mov"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("video"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var backends []*mockVideoBackend
	var savedPrefs config.PureVideoConfig
	p := NewPlayer(config.DefaultPureVideoConfig(), PlayerCallbacks{
		Headless: true,
		OnPrefsChange: func(cfg config.PureVideoConfig) {
			savedPrefs = cfg
		},
		BackendFactory: func(engine, _ string, _ uintptr, cb BackendCallbacks) (PlaybackBackend, string, error) {
			mb := &mockVideoBackend{kind: engine, cb: cb, dur: 90 * time.Second}
			backends = append(backends, mb)
			return mb, engine, nil
		},
	})
	defer p.Shutdown()

	p.Open(filepath.Join(dir, "01.mp4"))
	if len(backends) != 1 || p.idx != 0 || p.state != StatePlaying {
		t.Fatalf("after Open: backends=%d idx=%d state=%v", len(backends), p.idx, p.state)
	}

	p.TogglePlayPause()
	if p.state != StatePaused {
		t.Fatalf("expected paused after TogglePlayPause, got %v", p.state)
	}
	p.FrameStep()
	if backends[0].steps != 1 {
		t.Fatalf("expected FrameStep called once, got %d", backends[0].steps)
	}

	p.SeekTo(25 * time.Second)
	if backends[0].pos != 25*time.Second || p.pos != 25*time.Second {
		t.Fatalf("SeekTo mismatch: backend=%v player=%v", backends[0].pos, p.pos)
	}
	p.SetVolume(55)
	p.ToggleMute()
	p.CycleSpeed()
	p.ToggleLoop()
	if savedPrefs.Volume != 55 || !savedPrefs.Muted || p.speed != 1.25 || !p.loop {
		t.Fatalf("playback state mismatch: vol=%d muted=%v speed=%v loop=%v", savedPrefs.Volume, savedPrefs.Muted, p.speed, p.loop)
	}

	p.Next()
	if p.idx != 1 || filepath.Base(p.path) != "02.mkv" {
		t.Fatalf("Next -> idx=%d path=%q", p.idx, p.path)
	}
	p.Prev()
	if p.idx != 0 || filepath.Base(p.path) != "01.mp4" {
		t.Fatalf("Prev -> idx=%d path=%q", p.idx, p.path)
	}

	tt := ui.NewTester(p.View, 920, 560)
	tt.Frame()
	if !tt.HasText("libmpv") {
		t.Fatalf("expected transport bar engine label, texts=%v", tt.Texts())
	}
}

func TestPlayerErrorRecoveryAndReload(t *testing.T) {
	dir := t.TempDir()
	flv := filepath.Join(dir, "stream.flv")
	if err := os.WriteFile(flv, []byte("FLV"), 0o600); err != nil {
		t.Fatal(err)
	}

	failNext := true
	p := NewPlayer(config.DefaultPureVideoConfig(), PlayerCallbacks{
		Headless: true,
		BackendFactory: func(engine, _ string, _ uintptr, cb BackendCallbacks) (PlaybackBackend, string, error) {
			mb := &mockVideoBackend{kind: EngineMPV, cb: cb, dur: 40 * time.Second}
			if failNext {
				failNext = false
				mb.openErr = errors.New("模拟解码失败")
			}
			return mb, EngineMPV, nil
		},
	})
	defer p.Shutdown()

	p.Open(flv)
	if p.errText == "" {
		t.Fatal("expected initial open failure to surface error state")
	}

	tt := ui.NewTester(p.View, 920, 560)
	tt.Frame()
	if !tt.HasText("无法播放 stream.flv") {
		t.Fatalf("expected error state title, texts=%v", tt.Texts())
	}

	p.Reload()
	if p.errText != "" || p.activeEngine != EngineMPV || p.state != StatePlaying {
		t.Fatalf("expected Reload recovery: err=%q engine=%q state=%v", p.errText, p.activeEngine, p.state)
	}
}

func TestPureVideoPluginContractAndSettingsUI(t *testing.T) {
	h := newMockVideoHost(t)
	plg := NewPlugin()
	if err := plg.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer func() { _ = plg.Stop() }()

	if plg.ID() != PluginID || plg.Info().ModuleID != core.ModuleVideo {
		t.Fatalf("unexpected Info: %+v", plg.Info())
	}
	if !h.bus.Has(purekit.CapVideoPreview) {
		t.Fatal("expected CapVideoPreview registered")
	}
	exts, err := purekit.CallVideoPreviewSupportedExts(context.Background(), h.bus)
	if err != nil || len(exts) != len(VideoExtList) {
		t.Fatalf("CallVideoPreviewSupportedExts = (%d, %v)", len(exts), err)
	}

	matches := plg.Matchers()[0].Match(purekit.MatchContext{
		SelectedFiles: []string{`D:\Videos\clip.mp4`, `D:\Photos\a.png`},
	})
	if len(matches) != 1 || matches[0].FeatureID != "purevideo.open" {
		t.Fatalf("unexpected matcher results: %+v", matches)
	}

	tab := plg.SettingsTab()
	if tab == nil || tab.NewView == nil {
		t.Fatal("expected SettingsTab")
	}
	sv := tab.NewView(h)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
			sv.Render(c, uiPaletteForTest())
		})
	}, 640, 720)
	tt.Frame()
	if !tt.HasText("libmpv 解码内核") || tt.HasText("系统极速") {
		t.Fatalf("expected PureVideo libmpv-only settings card, texts=%v", tt.Texts())
	}
	if got := plg.Config().Engine; got != EngineMPV {
		t.Fatalf("initial Engine = %q, want %q", got, EngineMPV)
	}
	if err := tt.Click("开关：默认循环播放"); err != nil {
		t.Fatalf("click loop switch: %v", err)
	}
	tt.Frame()
	if !sv.Dirty() {
		t.Fatal("expected settings tab dirty after changing loop setting")
	}
	if err := sv.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !plg.Config().Loop {
		t.Fatal("expected saved Loop = true")
	}
}

func TestPlayerProgressAndVolumeBarPointerInput(t *testing.T) {
	dir := t.TempDir()
	mp4 := filepath.Join(dir, "demo.mp4")
	if err := os.WriteFile(mp4, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	var mb *mockVideoBackend
	p := NewPlayer(config.DefaultPureVideoConfig(), PlayerCallbacks{
		Headless: true,
		BackendFactory: func(engine, _ string, _ uintptr, cb BackendCallbacks) (PlaybackBackend, string, error) {
			mb = &mockVideoBackend{kind: engine, cb: cb, dur: 100 * time.Second}
			return mb, engine, nil
		},
	})
	defer p.Shutdown()

	p.Open(mp4)
	if mb == nil || p.dur != 100*time.Second {
		t.Fatalf("after Open: mb=%v dur=%v", mb, p.dur)
	}

	tt := ui.NewTester(p.View, 920, 560)
	tt.Frame()

	progRect, ok := tt.Find("播放进度条")
	if !ok || progRect.W <= 0 {
		t.Fatalf("expected progress bar rect, ok=%v rect=%+v", ok, progRect)
	}
	volRect, ok := tt.Find("音量滑块")
	if !ok || volRect.W <= 0 {
		t.Fatalf("expected volume bar rect, ok=%v rect=%+v", ok, volRect)
	}

	// Hover/move over progress bar (outside build pass) must not panic and must track local X.
	tt.Move(progRect.X+progRect.W*0.25, progRect.Y+progRect.H/2)
	if !p.hoverScrub || p.hoverFrac < 0.20 || p.hoverFrac > 0.30 {
		t.Fatalf("after hover: hoverScrub=%v hoverFrac=%.3f", p.hoverScrub, p.hoverFrac)
	}

	// Press, drag, and release on progress bar.
	tt.Press(progRect.X+progRect.W*0.50, progRect.Y+progRect.H/2)
	tt.Move(progRect.X+progRect.W*0.75, progRect.Y+progRect.H/2)
	tt.Release(progRect.X+progRect.W*0.75, progRect.Y+progRect.H/2)
	if p.scrubbing {
		t.Fatal("expected scrubbing=false after release")
	}
	if p.pos < 73*time.Second || p.pos > 77*time.Second || mb.pos != p.pos {
		t.Fatalf("expected pos ~75s after scrub, got player=%v backend=%v", p.pos, mb.pos)
	}

	// Hover, press, drag, and release on volume bar.
	tt.Move(volRect.X+volRect.W*0.50, volRect.Y+volRect.H/2)
	tt.Press(volRect.X+volRect.W*0.50, volRect.Y+volRect.H/2)
	if p.volume < 47 || p.volume > 53 {
		t.Fatalf("expected volume ~50 after press at 50%%, got %d", p.volume)
	}
	tt.Move(volRect.X+volRect.W*0.80, volRect.Y+volRect.H/2)
	tt.Release(volRect.X+volRect.W*0.80, volRect.Y+volRect.H/2)
	if p.volScrubbing {
		t.Fatal("expected volScrubbing=false after release")
	}
	if p.volume < 77 || p.volume > 83 {
		t.Fatalf("expected volume ~80 after drag to 80%%, got %d", p.volume)
	}

	tt.Frame()
}

func TestBGR0ToRGBAAndSaveVideoFrame(t *testing.T) {
	const w, h, stride = 3, 2, 16 // 3*4 bytes + 4 bytes stride padding
	src := make([]byte, stride*h)
	// Row 0, Col 0 -> B=10, G=20, R=200, X=0
	src[0], src[1], src[2], src[3] = 10, 20, 200, 0
	// Row 1, Col 2 -> B=220, G=150, R=30, X=0
	off := stride + 2*4
	src[off+0], src[off+1], src[off+2], src[off+3] = 220, 150, 30, 0

	rgba := bgr0ToRGBA(src, w, h, stride)
	if rgba.Bounds().Dx() != w || rgba.Bounds().Dy() != h {
		t.Fatalf("bounds = %v, want %dx%d", rgba.Bounds(), w, h)
	}
	if got := rgba.RGBAAt(0, 0); got != (color.RGBA{R: 200, G: 20, B: 10, A: 255}) {
		t.Fatalf("(0,0) = %+v", got)
	}
	if got := rgba.RGBAAt(2, 1); got != (color.RGBA{R: 30, G: 150, B: 220, A: 255}) {
		t.Fatalf("(2,1) = %+v", got)
	}

	dir := t.TempDir()
	pngOut := filepath.Join(dir, "frame.png")
	if err := saveVideoFrameToFile(pngOut, rgba); err != nil {
		t.Fatalf("saveVideoFrameToFile png: %v", err)
	}
	if raw, err := os.ReadFile(pngOut); err != nil {
		t.Fatal(err)
	} else if decoded, err := png.Decode(bytes.NewReader(raw)); err != nil || decoded.Bounds().Dx() != w {
		t.Fatalf("decode saved PNG = (%v, %v)", decoded, err)
	}
	jpgOut := filepath.Join(dir, "frame.jpg")
	if err := saveVideoFrameToFile(jpgOut, rgba); err != nil {
		t.Fatalf("saveVideoFrameToFile jpg: %v", err)
	}
}

func TestPlayerContextMenuAndNativeMenuParity(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	dir := t.TempDir()
	v1 := filepath.Join(dir, "01.mp4")
	v2 := filepath.Join(dir, "02.mkv")
	if err := os.WriteFile(v1, []byte("mp4data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v2, []byte("mkvdata"), 0o600); err != nil {
		t.Fatal(err)
	}

	frame := image.NewRGBA(image.Rect(0, 0, 8, 6))
	frame.SetRGBA(0, 0, color.RGBA{R: 120, G: 200, B: 80, A: 255})

	var mb *mockVideoBackend
	var copiedText string
	var copiedImg []byte
	var pinnedImg image.Image
	var settingsOpened bool

	p := NewPlayer(config.DefaultPureVideoConfig(), PlayerCallbacks{
		Headless:       true,
		CanPin:         func() bool { return true },
		OnPinImage:     func(img image.Image) { pinnedImg = img },
		OnCopyText:     func(s string) { copiedText = s },
		OnCopyImage:    func(b []byte) { copiedImg = b },
		OnOpenSettings: func() { settingsOpened = true },
		BackendFactory: func(engine, _ string, _ uintptr, cb BackendCallbacks) (PlaybackBackend, string, error) {
			mb = &mockVideoBackend{
				kind: engine,
				cb:   cb,
				dur:  120 * time.Second,
				tracks: []MediaTrack{
					{ID: 1, Type: "audio", Title: "Main", Lang: "eng", Codec: "aac", Selected: true},
					{ID: 2, Type: "audio", Title: "Director", Lang: "jpn", Codec: "ac3"},
					{ID: 3, Type: "sub", Title: "Subtitles", Lang: "chi", Codec: "ass"},
				},
				frameImg: frame,
				info: MediaInfo{
					VideoCodec:    "hevc",
					AudioCodec:    "aac",
					Width:         1920,
					Height:        1080,
					FPS:           23.976,
					VideoBitrate:  4500000,
					AudioRate:     48000,
					AudioChannels: 2,
				},
			}
			return mb, engine, nil
		},
	})
	defer p.Shutdown()

	tt := ui.NewTester(p.View, 920, 560)
	tt.Frame()

	// Empty state context menu: open enabled, playback disabled
	tt.RightClickAt(120, 120)
	emptyMenu := tt.Menu()
	if len(emptyMenu) < 5 || emptyMenu[0] != "打开视频文件…" {
		t.Fatalf("empty context menu unexpected: %v", emptyMenu)
	}
	if err := tt.ChooseMenuItem("播放"); err == nil {
		t.Fatal("expected disabled Play item in empty state")
	}
	tt.CloseMenu()

	// Load multi-video playlist
	p.Open(v1)
	tt.Frame()

	choose := func(path ...string) {
		t.Helper()
		tt.RightClickAt(460, 220)
		if err := tt.ChooseMenuItem(path...); err != nil {
			t.Fatalf("ChooseMenuItem(%v): %v (menu=%v)", path, err, tt.Menu())
		}
		tt.Frame()
	}

	// Transport + navigate
	choose("暂停")
	if p.state != StatePaused {
		t.Fatalf("expected StatePaused after Pause menu, got %v", p.state)
	}
	choose("播放")
	if p.state != StatePlaying {
		t.Fatalf("expected StatePlaying after Play menu, got %v", p.state)
	}
	choose("下一个视频")
	if p.idx != 1 || filepath.Base(p.path) != "02.mkv" {
		t.Fatalf("Next menu failed: idx=%d path=%q", p.idx, p.path)
	}
	choose("上一个视频")
	if p.idx != 0 || filepath.Base(p.path) != "01.mp4" {
		t.Fatalf("Prev menu failed: idx=%d path=%q", p.idx, p.path)
	}
	choose("跳转与逐帧", "前进 30 秒")
	if p.pos != 30*time.Second {
		t.Fatalf("Forward 30s pos=%v", p.pos)
	}
	choose("跳转与逐帧", "从头播放 (00:00)")
	if p.pos != 0 {
		t.Fatalf("Restart pos=%v", p.pos)
	}
	choose("跳转与逐帧", "前进一帧")
	choose("跳转与逐帧", "后退一帧")
	if mb.steps != 1 || mb.backSteps != 1 {
		t.Fatalf("step counts steps=%d backSteps=%d", mb.steps, mb.backSteps)
	}

	// Speed
	choose("播放速度", "1.5×")
	if p.speed != 1.5 {
		t.Fatalf("speed = %v, want 1.5", p.speed)
	}
	choose("播放速度", "恢复正常倍速 (1×)")
	if p.speed != 1.0 {
		t.Fatalf("reset speed = %v, want 1.0", p.speed)
	}

	// Loop & A-B loop
	choose("循环与 AB 复读", "单视频循环播放")
	if !p.loop {
		t.Fatal("expected loop enabled")
	}
	p.SeekTo(10 * time.Second)
	choose("循环与 AB 复读", "设置 A 点（当前位置）")
	p.SeekTo(25 * time.Second)
	choose("循环与 AB 复读", "设置 B 点（当前位置）")
	if mb.abA != 10 || mb.abB != 25 {
		t.Fatalf("A-B loop bounds = (%v, %v), want (10, 25)", mb.abA, mb.abB)
	}
	choose("循环与 AB 复读", "清除 A-B 循环")
	if mb.abA >= 0 || mb.abB >= 0 {
		t.Fatalf("expected A-B loop cleared, got (%v, %v)", mb.abA, mb.abB)
	}

	// Audio & Subtitles
	choose("音频", "静音")
	if !p.muted {
		t.Fatal("expected muted")
	}
	choose("音频", "恢复 100% 音量")
	if p.volume != 100 || p.muted {
		t.Fatalf("volume=%d muted=%v", p.volume, p.muted)
	}
	choose("音频", "音轨选择", "#2 · Director · [jpn] · AC3")
	if mb.audioTrack != 2 {
		t.Fatalf("audioTrack = %d, want 2", mb.audioTrack)
	}
	choose("音频", "音频同步延迟", "提前 0.1 秒 (+0.1s)")
	if mb.audioDelay <= 0 {
		t.Fatalf("expected positive audioDelay, got %v", mb.audioDelay)
	}
	choose("音频", "音频同步延迟", "重置音频延迟 (0.0s)")
	if mb.audioDelay != 0 {
		t.Fatalf("expected audioDelay=0, got %v", mb.audioDelay)
	}

	choose("字幕", "显示字幕")
	if p.subVisible || mb.subVisible {
		t.Fatalf("expected subVisible=false")
	}
	choose("字幕", "字幕轨道", "#3 · Subtitles · [chi] · ASS")
	if mb.subTrack != 3 || !p.subVisible {
		t.Fatalf("subTrack=%d subVisible=%v", mb.subTrack, p.subVisible)
	}
	choose("字幕", "字幕同步延迟", "延迟 -0.1 秒")
	if mb.subDelay >= 0 {
		t.Fatalf("expected negative subDelay, got %v", mb.subDelay)
	}
	choose("字幕", "字幕同步延迟", "重置字幕延迟 (0.0s)")
	if mb.subDelay != 0 {
		t.Fatalf("expected subDelay=0, got %v", mb.subDelay)
	}

	// Video transforms
	choose("画面与变换", "宽高比", "16:9")
	choose("画面与变换", "画面旋转", "90°")
	choose("画面与变换", "水平镜像翻转")
	choose("画面与变换", "垂直翻转")
	choose("画面与变换", "画面缩放", "放大画面 (+)")
	choose("画面与变换", "反交错 (Deinterlace)")
	choose("画面与变换", "硬件加速解码 (HWDec)")
	if mb.aspectRatio != "16:9" || mb.rotateDeg != 90 || !mb.flipH || !mb.flipV || mb.zoom <= 0 || !mb.deinterlace || mb.hwdec {
		t.Fatalf("video adjustments mismatch: aspect=%q rot=%d flip=(%v,%v) zoom=%v deint=%v hwdec=%v",
			mb.aspectRatio, mb.rotateDeg, mb.flipH, mb.flipV, mb.zoom, mb.deinterlace, mb.hwdec)
	}
	choose("画面与变换", "重置全部画面调整")
	if mb.aspectRatio != "" || mb.rotateDeg != 0 || mb.flipH || mb.flipV || mb.zoom != 0 || mb.deinterlace {
		t.Fatalf("expected reset video adjustments, got aspect=%q rot=%d", mb.aspectRatio, mb.rotateDeg)
	}

	// Capture & Pin
	choose("截图与采帧", "复制当前画面帧")
	if decoded, err := png.Decode(bytes.NewReader(copiedImg)); err != nil || decoded.Bounds().Dx() != 8 || decoded.Bounds().Dy() != 6 {
		t.Fatalf("CopyCurrentFrame PNG decode failed: err=%v len=%d", err, len(copiedImg))
	}
	choose("截图与采帧", "贴图当前帧到屏幕")
	if pinnedImg == nil || pinnedImg.Bounds().Dx() != 8 {
		t.Fatalf("PinCurrentFrame failed: %v", pinnedImg)
	}

	// Window & File & Sort & Reload & Settings
	choose("窗口置顶")
	if !p.alwaysOnTop {
		t.Fatal("expected alwaysOnTop=true")
	}
	choose("文件与属性", "复制文件路径")
	if copiedText != v1 {
		t.Fatalf("CopyPath = %q, want %q", copiedText, v1)
	}
	choose("文件与属性", "复制文件名")
	if copiedText != "01.mp4" {
		t.Fatalf("CopyFileName = %q, want 01.mp4", copiedText)
	}
	choose("文件与属性", "媒体与编码信息…")
	if copiedText == "" || !containsSubstring(copiedText, "HEVC") {
		t.Fatalf("ShowMediaInfo copied summary unexpected: %q", copiedText)
	}
	choose("文件夹排序", "修改时间降序")
	if p.prefs.Sort != "modified" || !p.prefs.SortDesc {
		t.Fatalf("Sort prefs = (%q, %v)", p.prefs.Sort, p.prefs.SortDesc)
	}
	choose("停止")
	if p.state != StateStopped {
		t.Fatalf("expected StateStopped after Stop, got %v", p.state)
	}
	choose("PureVideo 设置…")
	if !settingsOpened {
		t.Fatal("expected OnOpenSettings callback invoked")
	}
	choose("重新加载 libmpv 内核")
	if len(mb.opened) == 0 {
		t.Fatal("expected Reload to recreate & reopen video")
	}

	// Verify native menu parity (what Win32 stageWndProc shows on right-click over playing video)
	tt.RightClickAt(460, 220)
	uiLabels := tt.Menu()
	tt.CloseMenu()

	nativeItems := buildNativeVideoMenu(p.buildContextMenuEntries())
	if len(nativeItems) != len(uiLabels) {
		t.Fatalf("native menu len=%d != UI menu len=%d", len(nativeItems), len(uiLabels))
	}
	var topItem *ui.MenuItem
	_ = topItem
	for i, ni := range nativeItems {
		want := uiLabels[i]
		if want == "-" {
			if ni.Type != "separator" {
				t.Fatalf("item %d: expected separator, got %q", i, ni.Type)
			}
			continue
		}
		if ni.Label != want {
			t.Fatalf("item %d: native label %q != UI label %q", i, ni.Label, want)
		}
		if ni.Label == "窗口置顶" && ni.Click != nil {
			before := p.alwaysOnTop
			ni.Click(ni, nil)
			if p.alwaysOnTop == before {
				t.Fatal("native menu click did not toggle alwaysOnTop")
			}
		}
	}

	// Verify EN locale has zero Han runes across every label/submenu
	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)
	enNative := buildNativeVideoMenu(p.buildContextMenuEntries())
	var checkNoHan func(items []*mygo.MenuItem)
	checkNoHan = func(items []*mygo.MenuItem) {
		for _, it := range items {
			for _, r := range it.Label {
				if unicode.Is(unicode.Han, r) {
					t.Errorf("unexpected Han rune in English video context menu item %q", it.Label)
					break
				}
			}
			if len(it.Submenu) > 0 {
				checkNoHan(it.Submenu)
			}
		}
	}
	checkNoHan(enNative)
}

func containsSubstring(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && bytes.Contains([]byte(haystack), []byte(needle)))
}
