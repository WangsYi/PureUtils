//go:build windows

package purevideo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/storage"
)

type mockVideoBackend struct {
	kind     string
	cb       BackendCallbacks
	opened   []string
	openErr  error
	state    PlaybackState
	pos      time.Duration
	dur      time.Duration
	vol      float32
	muted    bool
	rate     float32
	steps    int
	closed   bool
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
func (m *mockVideoBackend) Position() time.Duration    { return m.pos }
func (m *mockVideoBackend) Duration() time.Duration    { return m.dur }
func (m *mockVideoBackend) NativeSize() (int, int)     { return 1920, 1080 }
func (m *mockVideoBackend) State() PlaybackState       { return m.state }
func (m *mockVideoBackend) Resize(_, _ int)            {}
func (m *mockVideoBackend) Repaint()                   {}
func (m *mockVideoBackend) Close()                     { m.closed = true }

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
