package pureimage

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/storage"
	pureui "pureutils/pkg/purekit/ui"
)

type mockHost struct {
	paths     core.Paths
	cfgMgr    *config.Manager
	storeMgr  *storage.Manager
	bus       *purekit.Bus
	log       *core.Logger
	hostCfg   config.HostConfig
	clip      clipboard.Service
	closedNotified int
	reloadCalls    int
	rebuildCalls   int
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
	hc := config.DefaultHostConfig()
	return &mockHost{
		paths:    paths,
		cfgMgr:   cfgMgr,
		storeMgr: storeMgr,
		bus:      purekit.NewBus(),
		hostCfg:  hc,
	}
}

func (h *mockHost) Config(id string) config.Section    { return h.cfgMgr.Section(id) }
func (h *mockHost) HostConfig() config.HostConfig      { return h.hostCfg.Clone() }
func (h *mockHost) SaveHostConfig(c config.HostConfig) error { h.hostCfg = c.Clone(); return nil }
func (h *mockHost) Storage(id string) storage.PluginStorage { return h.storeMgr.Plugin(id) }
func (h *mockHost) Clipboard() clipboard.Service       { return h.clip }
func (h *mockHost) Bus() *purekit.Bus                  { return h.bus }
func (h *mockHost) Log() *core.Logger                  { return h.log }
func (h *mockHost) Notify(string, string)              {}
func (h *mockHost) Headless() bool                     { return true }
func (h *mockHost) Paths() core.Paths                  { return h.paths }
func (h *mockHost) ModuleEnabled(id core.ModuleID) bool { return true }
func (h *mockHost) ReloadHotkeys()                     { h.reloadCalls++ }
func (h *mockHost) RebuildTray()                       { h.rebuildCalls++ }
func (h *mockHost) OpenSettings(string)                {}
func (h *mockHost) ExecuteCommand(core.Command, ...string) {}
func (h *mockHost) HotkeyError(core.Command) string    { return "" }
func (h *mockHost) SetGlobalHookSuppressed(string, bool) {}
func (h *mockHost) IsGlobalHookSuppressed() bool       { return false }
func (h *mockHost) NotifyWindowClosed()                { h.closedNotified++ }

func TestPluginLifecycleCapabilitiesAndPinBus(t *testing.T) {
	h := newTestHost(t)
	p := NewPlugin()
	if p.ID() != PluginID || p.Info().ModuleID != core.ModuleViewer {
		t.Fatalf("unexpected plugin identity: %+v", p.Info())
	}
	if err := p.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !h.bus.Has(purekit.CapImagePreview) {
		t.Fatal("expected image.preview capability registered")
	}

	ctx := context.Background()
	exts, err := purekit.CallImagePreviewSupportedExts(ctx, h.bus)
	if err != nil || !slices.Contains(exts, ".png") || !slices.Contains(exts, ".heic") {
		t.Fatalf("CallImagePreviewSupportedExts = %v, err=%v", exts, err)
	}

	dir := t.TempDir()
	imgPath := filepath.Join(dir, "sample.png")
	writeTestPNG(t, imgPath, 40, 24, color.RGBA{R: 255, A: 255})
	if err := purekit.CallImagePreviewPath(ctx, h.bus, imgPath); err != nil {
		t.Fatalf("CallImagePreviewPath: %v", err)
	}
	if p.Viewer().path != imgPath {
		t.Fatalf("viewer path = %q, want %q", p.Viewer().path, imgPath)
	}

	memImg := image.NewRGBA(image.Rect(0, 0, 16, 12))
	if err := purekit.CallImagePreviewImage(ctx, h.bus, "bus-mem", memImg); err != nil {
		t.Fatalf("CallImagePreviewImage: %v", err)
	}
	if p.Viewer().displayName != "bus-mem" || p.Viewer().srcW != 16 {
		t.Fatalf("OpenImage via bus failed: name=%q w=%d", p.Viewer().displayName, p.Viewer().srcW)
	}

	if err := purekit.CallImagePreviewToggleQuickPreview(ctx, h.bus); err != nil {
		t.Fatalf("CallImagePreviewToggleQuickPreview: %v", err)
	}

	// Dynamic CapImagePin discovery even when registered after PureImage.Init().
	var pinned image.Image
	if err := h.bus.Register(purekit.CapImagePin, func(_ context.Context, raw any) (any, error) {
		req := raw.(purekit.ImagePinRequest)
		pinned = req.Image
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(p.Viewer().View, 1000, 680)
	tt.Frame()
	if err := tt.Click("贴到屏幕"); err != nil {
		t.Fatalf("click pin button: %v", err)
	}
	if pinned == nil || pinned.Bounds().Dx() != 16 {
		t.Fatalf("expected pinned image via Bus, got %v", pinned)
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if h.bus.Has(purekit.CapImagePreview) {
		t.Fatal("expected image.preview unregistered after Stop")
	}
}

func TestPluginMatchersTrayAndSettingsTab(t *testing.T) {
	h := newTestHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	if p.ShouldStayInTray() {
		t.Fatal("expected ShouldStayInTray=false by default")
	}

	// Verify matchers for selected image file + clipboard image.
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "clip_shot.png")
	writeTestPNG(t, imgPath, 20, 20, color.RGBA{G: 200, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 10, 10)))

	ms := p.Matchers()
	if len(ms) == 0 {
		t.Fatal("expected image matcher")
	}
	res := ms[0].Match(purekit.MatchContext{
		Query:         "",
		SelectedFiles: []string{imgPath, filepath.Join(dir, "readme.txt")},
		Clipboard:     &core.ClipboardContent{ImagePNG: buf.Bytes()},
	})
	if len(res) != 2 {
		t.Fatalf("expected 2 match results (file + clipboard image), got %d: %+v", len(res), res)
	}
	if err := res[1].Execute(context.Background()); err != nil {
		t.Fatalf("execute clipboard image match: %v", err)
	}
	if p.Viewer().displayName != "剪贴板图片" {
		t.Fatalf("expected OpenImage from matcher, got %q", p.Viewer().displayName)
	}

	// Render self-registered SettingsTab, toggle SpacePreview, Save and verify config + tray.
	tab := p.SettingsTab()
	if tab == nil || tab.ID != PluginID {
		t.Fatalf("unexpected SettingsTab: %+v", tab)
	}
	view := tab.NewView(h)
	st := ui.NewTester(func(c *ui.Context) {
		pal := pureui.ApplyTheme(c)
		ui.Column(c).Fill().Background(pal.Bg).Padding(16).Gap(12).Children(func() {
			view.Render(c, pal)
		})
	}, 720, 900)
	st.Frame()
	if !st.HasText("浏览与渲染") || !st.HasText("资源管理器空格预览") {
		t.Fatalf("settings tab missing expected cards: %v", st.Texts())
	}
	if err := st.Click("开关：资源管理器空格预览"); err != nil {
		t.Fatalf("click space preview switch: %v", err)
	}
	st.Frame()
	if !view.Dirty() {
		t.Fatal("expected Dirty=true after toggling space preview")
	}
	if err := view.Save(); err != nil {
		t.Fatalf("Save settings tab: %v", err)
	}
	if view.Dirty() {
		t.Fatal("expected Dirty=false after Save")
	}
	if !p.ShouldStayInTray() {
		t.Fatal("expected ShouldStayInTray=true when SpacePreview enabled")
	}
	if h.reloadCalls == 0 || h.rebuildCalls == 0 {
		t.Fatalf("expected ReloadHotkeys & RebuildTray on SpacePreview change, got reload=%d rebuild=%d", h.reloadCalls, h.rebuildCalls)
	}
	loaded := config.LoadPureImage(h.Config(PluginID))
	if !loaded.SpacePreview {
		t.Fatalf("expected SpacePreview persisted, got %+v", loaded)
	}

	items := p.TrayItems()
	if len(items) != 2 || items[1].Kind != purekit.TrayItemCheckbox || !items[1].Checked || items[1].Label != "资源管理器空格预览" {
		t.Fatalf("unexpected TrayItems: %+v", items)
	}

	view.ResetDefaults()
	if !view.Dirty() {
		t.Fatal("expected Dirty=true after ResetDefaults from non-default state")
	}
}
