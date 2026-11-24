package pureshot

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/storage"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/pureshot/annotate"
)

type fakeClipboardIO struct {
	mu      sync.Mutex
	content *core.ClipboardContent
}

func (f *fakeClipboardIO) Read() (*core.ClipboardContent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.content == nil {
		return nil, nil
	}
	c := *f.content
	c.Files = append([]string(nil), f.content.Files...)
	c.ImagePNG = append([]byte(nil), f.content.ImagePNG...)
	c.Formats = append([]string(nil), f.content.Formats...)
	return &c, nil
}

func (f *fakeClipboardIO) ReadImagePNG() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.content == nil {
		return nil, nil
	}
	return append([]byte(nil), f.content.ImagePNG...), nil
}

func (f *fakeClipboardIO) Write(c *core.ClipboardContent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c == nil {
		f.content = nil
		return nil
	}
	cp := *c
	cp.Files = append([]string(nil), c.Files...)
	cp.ImagePNG = append([]byte(nil), c.ImagePNG...)
	cp.Formats = append([]string(nil), c.Formats...)
	f.content = &cp
	return nil
}

type mockHost struct {
	paths      core.Paths
	cfgMgr     *config.Manager
	storeMgr   *storage.Manager
	bus        *purekit.Bus
	clip       *clipboard.Manager
	io         *fakeClipboardIO
	hostCfg    config.HostConfig
	notices    []string
	rebuilds   int
	suppressed map[string]bool
}

func newTestHost(t *testing.T) *mockHost {
	t.Helper()
	home := t.TempDir()
	paths := core.Paths{
		Home:     home,
		DB:       filepath.Join(home, "pureutils.db"),
		Settings: filepath.Join(home, "settings.json"),
		Images:   filepath.Join(home, "images"),
		Logs:     filepath.Join(home, "logs"),
	}
	cfgMgr, err := config.Open(paths.Settings)
	if err != nil {
		t.Fatal(err)
	}
	storeMgr, err := storage.NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	fio := &fakeClipboardIO{}
	return &mockHost{
		paths:      paths,
		cfgMgr:     cfgMgr,
		storeMgr:   storeMgr,
		bus:        purekit.NewBus(),
		io:         fio,
		clip:       clipboard.NewManager(fio, nil, func() string { return "" }, clipboard.ManagerOptions{Debounce: 20 * time.Millisecond}),
		hostCfg:    config.DefaultHostConfig(),
		suppressed: map[string]bool{},
	}
}

func (h *mockHost) Config(id string) config.Section            { return h.cfgMgr.Section(id) }
func (h *mockHost) HostConfig() config.HostConfig              { return h.hostCfg.Clone() }
func (h *mockHost) SaveHostConfig(c config.HostConfig) error { h.hostCfg = c.Clone(); return nil }
func (h *mockHost) Storage(id string) storage.PluginStorage    { return h.storeMgr.Plugin(id) }
func (h *mockHost) Clipboard() clipboard.Service               { return h.clip }
func (h *mockHost) Bus() *purekit.Bus                          { return h.bus }
func (h *mockHost) Log() *core.Logger                          { return nil }
func (h *mockHost) Notify(title, body string)                  { h.notices = append(h.notices, title+": "+body) }
func (h *mockHost) Headless() bool                             { return true }
func (h *mockHost) Paths() core.Paths                          { return h.paths }
func (h *mockHost) ModuleEnabled(id core.ModuleID) bool        { return true }
func (h *mockHost) ReloadHotkeys()                             {}
func (h *mockHost) RebuildTray()                               { h.rebuilds++ }
func (h *mockHost) OpenSettings(string)                        {}
func (h *mockHost) ExecuteCommand(core.Command, ...string)     {}
func (h *mockHost) HotkeyError(core.Command) string            { return "" }
func (h *mockHost) SetGlobalHookSuppressed(src string, on bool) {
	if on {
		h.suppressed[src] = true
	} else {
		delete(h.suppressed, src)
	}
}
func (h *mockHost) IsGlobalHookSuppressed() bool { return len(h.suppressed) > 0 }
func (h *mockHost) NotifyWindowClosed()          {}

func encodeTestPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPureShotCaptureAndEphemeralColorWithoutPureClip(t *testing.T) {
	h := newTestHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = p.Stop() }()

	var events []clipboard.Event
	unsub := h.clip.Subscribe(func(ev clipboard.Event) {
		events = append(events, ev)
	})
	defer unsub()

	frame := image.NewRGBA(image.Rect(0, 0, 100, 80))
	doc := annotate.NewDocument(40, 30)
	res, err := p.Capture().Complete(frame, image.Rect(10, 10, 50, 40), doc, "copy")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !res.Copied || res.Width != 40 || res.Height != 30 {
		t.Fatalf("unexpected ShotResult: %+v", res)
	}
	if len(events) != 1 || !events[0].SelfWrite || events[0].Source != PluginID || events[0].Ephemeral {
		t.Fatalf("expected pureshot self-write event, got %+v", events)
	}
	cur, err := h.clip.Read()
	if err != nil || cur == nil || !bytes.Equal(cur.ImagePNG, res.PNG) {
		t.Fatalf("clipboard did not receive screenshot PNG")
	}
	shots, err := p.Repo().RecentShots(10)
	if err != nil || len(shots) != 1 || shots[0].Width != 40 {
		t.Fatalf("expected 1 row in shots.db, got %+v (err=%v)", shots, err)
	}

	// Magnifier color copy uses WriteEphemeralText: writes clipboard without emitting history event.
	sess := &captureSession{plugin: p, magnifierColor: color.RGBA{R: 0x12, G: 0xAB, B: 0xEF, A: 0xFF}}
	sess.copyMagnifierColor()
	if len(events) != 1 {
		t.Fatalf("expected ephemeral magnifier copy NOT to emit subscriber event; got %d events", len(events))
	}
	cur, _ = h.clip.Read()
	if cur == nil || cur.Text != "#12ABEF" {
		t.Fatalf("expected #12ABEF in clipboard, got %+v", cur)
	}
}

func TestPureShotPinClipboardFallbackWithoutPureClip(t *testing.T) {
	h := newTestHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	if h.bus.Has(purekit.CapClipboardHistory) {
		t.Fatal("expected no CapClipboardHistory in standalone PureShot test")
	}

	_ = h.io.Write(&core.ClipboardContent{Text: "fallback pin text"})
	p.PinClipboardSync()
	if p.PinnedCount() != 1 || len(p.PinnedRecords()) != 1 || p.PinnedRecords()[0].Text != "fallback pin text" {
		t.Fatalf("F3 text fallback failed: count=%d recs=%+v notices=%v", p.PinnedCount(), p.PinnedRecords(), h.notices)
	}

	_ = h.io.Write(&core.ClipboardContent{ImagePNG: encodeTestPNG(t, 24, 18, color.RGBA{R: 200, A: 255})})
	p.PinClipboardSync()
	if p.PinnedCount() != 2 || !p.PinnedRecords()[1].IsImage || p.PinnedRecords()[1].Width != 24 {
		t.Fatalf("F3 image fallback failed: count=%d recs=%+v", p.PinnedCount(), p.PinnedRecords())
	}
}

func TestPureShotPinClipboardSequentialViaHistoryBus(t *testing.T) {
	h := newTestHost(t)

	var onChangedFn func()
	entries := []purekit.HistoryEntry{
		{ID: 20, Kind: core.KindText, Preview: "newest item"},
		{ID: 10, Kind: core.KindText, Preview: "older item"},
	}
	contents := map[int64]*core.ClipboardContent{
		20: {Text: "newest item"},
		10: {Text: "older item"},
	}
	_ = h.bus.Register(purekit.CapClipboardHistory, func(_ context.Context, raw any) (any, error) {
		req := raw.(purekit.ClipboardHistoryRequest)
		switch req.Action {
		case "subscribe_changed":
			onChangedFn = req.OnChanged
			return purekit.ClipboardHistoryResponse{Active: true, Monitoring: true, Unsubscribe: func() {}}, nil
		case "flush":
			return purekit.ClipboardHistoryResponse{Active: true, Monitoring: true}, nil
		case "list_recent":
			return purekit.ClipboardHistoryResponse{Active: true, Monitoring: true, Entries: entries}, nil
		case "load_content":
			return purekit.ClipboardHistoryResponse{Active: true, Monitoring: true, Content: contents[req.EntryID]}, nil
		default:
			return purekit.ClipboardHistoryResponse{Active: true, Monitoring: true}, nil
		}
	})

	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	// 1st F3 -> newest (cascade 0)
	p.PinClipboardSync()
	// 2nd F3 -> older (cascade 1)
	p.PinClipboardSync()
	// 3rd F3 -> exhausted notification
	p.PinClipboardSync()

	recs := p.PinnedRecords()
	if len(recs) != 2 || recs[0].Text != "newest item" || recs[0].Cascade != 0 || recs[1].Text != "older item" || recs[1].Cascade != 1 {
		t.Fatalf("unexpected sequential pins: %+v", recs)
	}
	if len(h.notices) == 0 {
		t.Fatal("expected exhaustion notice on 3rd F3")
	}

	// Simulate history change -> resets queue so next F3 starts from newest again.
	if onChangedFn == nil {
		t.Fatal("expected PureShot to subscribe to history changes")
	}
	onChangedFn()
	p.PinClipboardSync()
	recs = p.PinnedRecords()
	if len(recs) != 3 || recs[2].Text != "newest item" {
		t.Fatalf("expected reset pin sequence after history change: %+v", recs)
	}
}

func TestPureShotTrayCheckboxesAndCapImagePin(t *testing.T) {
	h := newTestHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	// 0 pins: both toggle checkboxes disabled & unchecked
	items := p.TrayItems()
	if len(items) != 5 {
		t.Fatalf("expected 5 tray items, got %d", len(items))
	}
	if items[3].Label != "显示贴图" || !items[3].Disabled || items[3].Checked {
		t.Fatalf("unexpected show_pins with 0 pins: %+v", items[3])
	}
	if items[4].Label != "贴图鼠标穿透" || !items[4].Disabled || items[4].Checked {
		t.Fatalf("unexpected passthrough_pins with 0 pins: %+v", items[4])
	}

	// Pin text & image via Bus
	if err := purekit.CallImagePinText(context.Background(), h.bus, "pin via bus", 0); err != nil {
		t.Fatalf("CallImagePinText: %v", err)
	}
	if err := purekit.CallImagePinImage(context.Background(), h.bus, image.NewRGBA(image.Rect(0, 0, 20, 12)), 1); err != nil {
		t.Fatalf("CallImagePinImage: %v", err)
	}

	items = p.TrayItems()
	if items[3].Label != "显示贴图 (2)" || items[3].Disabled || !items[3].Checked {
		t.Fatalf("unexpected show_pins with 2 pins: %+v", items[3])
	}
	if items[4].Disabled || items[4].Checked {
		t.Fatalf("unexpected passthrough_pins with 2 interactive pins: %+v", items[4])
	}

	p.SetPinsPassthrough(true)
	items = p.TrayItems()
	if !items[4].Checked {
		t.Fatalf("expected passthrough checkbox checked: %+v", items[4])
	}

	p.TogglePins()
	items = p.TrayItems()
	if items[3].Checked {
		t.Fatalf("expected show_pins unchecked after TogglePins: %+v", items[3])
	}
}

func TestPureShotSettingsTabPersistence(t *testing.T) {
	h := newTestHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	tab := p.SettingsTab()
	if tab == nil || tab.ID != PluginID {
		t.Fatalf("unexpected SettingsTab: %+v", tab)
	}
	sv := tab.NewView(h)
	st := ui.NewTester(func(c *ui.Context) {
		pal := pureui.ApplyTheme(c)
		ui.Column(c).Fill().Background(pal.Bg).Padding(16).Gap(12).Children(func() {
			sv.Render(c, pal)
		})
	}, 720, 1100)
	st.Frame()
	if !st.HasText("默认操作与文件保存") || !st.HasText("悬浮贴图设置") {
		t.Fatalf("PureShot settings tab missing cards: %v", st.Texts())
	}
	if err := st.Click("保存到文件"); err != nil {
		t.Fatalf("click default action segment: %v", err)
	}
	st.Frame()
	if !sv.Dirty() {
		t.Fatal("expected Dirty=true after changing default action")
	}
	if err := sv.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded := config.LoadPureShot(h.Config(PluginID))
	if loaded.Capture.DefaultAction != config.CaptureActionSave {
		t.Fatalf("DefaultAction = %q, want %q", loaded.Capture.DefaultAction, config.CaptureActionSave)
	}
	sv.ResetDefaults()
	if !sv.Dirty() {
		t.Fatal("expected Dirty=true after ResetDefaults")
	}
}
