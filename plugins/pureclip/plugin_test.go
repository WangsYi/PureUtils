package pureclip

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	_ "modernc.org/sqlite"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/storage"
	pureui "pureutils/pkg/purekit/ui"
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

type fakeWatcher struct {
	mu      sync.Mutex
	onUpd   func()
	running bool
}

func (w *fakeWatcher) OnUpdate(fn func()) {
	w.mu.Lock()
	w.onUpd = fn
	w.mu.Unlock()
}
func (w *fakeWatcher) Start() error { w.running = true; return nil }
func (w *fakeWatcher) Stop()        { w.running = false }
func (w *fakeWatcher) Close()       { w.running = false }
func (w *fakeWatcher) Fire() {
	w.mu.Lock()
	fn := w.onUpd
	w.mu.Unlock()
	if fn != nil {
		fn()
	}
}

type mockHost struct {
	paths    core.Paths
	cfgMgr   *config.Manager
	storeMgr *storage.Manager
	bus      *purekit.Bus
	clip     *clipboard.Manager
	hostCfg  config.HostConfig
	procName string
	watcher  *fakeWatcher
	io       *fakeClipboardIO
	rebuilds int
}

func newTestHost(t *testing.T, preSeed func(home string)) *mockHost {
	t.Helper()
	home := t.TempDir()
	if preSeed != nil {
		preSeed(home)
	}
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
	fw := &fakeWatcher{}
	h := &mockHost{
		paths:    paths,
		cfgMgr:   cfgMgr,
		storeMgr: storeMgr,
		bus:      purekit.NewBus(),
		hostCfg:  config.DefaultHostConfig(),
		procName: "notepad",
		watcher:  fw,
		io:       fio,
	}
	h.clip = clipboard.NewManager(fio, fw, func() string { return h.procName }, clipboard.ManagerOptions{
		Debounce:          20 * time.Millisecond,
		SelfWriteSuppress: 300 * time.Millisecond,
	})
	return h
}

func (h *mockHost) Config(id string) config.Section            { return h.cfgMgr.Section(id) }
func (h *mockHost) HostConfig() config.HostConfig              { return h.hostCfg.Clone() }
func (h *mockHost) SaveHostConfig(c config.HostConfig) error { h.hostCfg = c.Clone(); return nil }
func (h *mockHost) Storage(id string) storage.PluginStorage    { return h.storeMgr.Plugin(id) }
func (h *mockHost) Clipboard() clipboard.Service               { return h.clip }
func (h *mockHost) Bus() *purekit.Bus                          { return h.bus }
func (h *mockHost) Log() *core.Logger                          { return nil }
func (h *mockHost) Notify(string, string)                      {}
func (h *mockHost) Headless() bool                             { return true }
func (h *mockHost) Paths() core.Paths                          { return h.paths }
func (h *mockHost) ModuleEnabled(id core.ModuleID) bool        { return true }
func (h *mockHost) ReloadHotkeys()                             {}
func (h *mockHost) RebuildTray()                               { h.rebuilds++ }
func (h *mockHost) OpenSettings(string)                        {}
func (h *mockHost) ExecuteCommand(core.Command, ...string)     {}
func (h *mockHost) HotkeyError(core.Command) string            { return "" }
func (h *mockHost) SetGlobalHookSuppressed(string, bool)       {}
func (h *mockHost) IsGlobalHookSuppressed() bool               { return false }
func (h *mockHost) NotifyWindowClosed()                        {}

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

func TestPureClipIngestionAndCapabilityBus(t *testing.T) {
	h := newTestHost(t, nil)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = p.Stop() }()

	if !p.Monitoring() {
		t.Fatal("expected monitoring enabled by default")
	}
	if !h.bus.Has(purekit.CapClipboardHistory) {
		t.Fatal("expected CapClipboardHistory registered")
	}

	var changed atomic.Int32
	unsub, err := purekit.CallClipboardHistorySubscribeChanged(context.Background(), h.bus, func() {
		changed.Add(1)
	})
	if err != nil {
		t.Fatalf("subscribe changed: %v", err)
	}
	defer unsub()

	// 1. External copy via OS watcher
	_ = h.io.Write(&core.ClipboardContent{Text: "hello pureclip"})
	h.watcher.Fire()
	h.clip.FlushPending()

	// 2. Ignored process skipped
	h.procName = "keepass"
	_ = h.io.Write(&core.ClipboardContent{Text: "secret-password"})
	h.watcher.Fire()
	h.clip.FlushPending()
	h.procName = "notepad"

	// 3. PureShot self-write recorded with PureShot source attribution
	pngBytes := encodeTestPNG(t, 12, 8, color.RGBA{R: 255, A: 255})
	if err := h.clip.WriteImage(pngBytes, "pureshot"); err != nil {
		t.Fatal(err)
	}

	// 4. PureClip self-write is ignored (no duplicate noise row)
	if err := h.clip.WriteText("replay-from-pureclip", PluginID); err != nil {
		t.Fatal(err)
	}

	// 5. Ephemeral write (magnifier color) does NOT pollute history
	if err := h.clip.WriteEphemeralText("#112233", "pureshot"); err != nil {
		t.Fatal(err)
	}

	listRes, err := purekit.CallClipboardHistoryListRecent(context.Background(), h.bus, 20)
	if err != nil {
		t.Fatalf("list_recent: %v", err)
	}
	if !listRes.Active || !listRes.Monitoring {
		t.Fatalf("expected active & monitoring, got %+v", listRes)
	}
	if len(listRes.Entries) != 2 {
		t.Fatalf("expected 2 entries (external text + pureshot image), got %d: %+v", len(listRes.Entries), listRes.Entries)
	}
	if listRes.Entries[0].SourceProcess != "PureShot" && listRes.Entries[1].SourceProcess != "PureShot" {
		t.Fatalf("expected PureShot attribution on screenshot entry: %+v", listRes.Entries)
	}
	for _, e := range listRes.Entries {
		if e.Preview == "secret-password" || e.Preview == "replay-from-pureclip" || e.Preview == "#112233" {
			t.Fatalf("unexpected forbidden entry recorded: %+v", e)
		}
	}
	if changed.Load() < 2 {
		t.Fatalf("expected >= 2 change notifications, got %d", changed.Load())
	}

	// Verify load_content reconstructs image bytes
	var imgEntryID int64
	for _, e := range listRes.Entries {
		if e.Kind == core.KindImage {
			imgEntryID = e.ID
		}
	}
	loaded, err := purekit.CallClipboardHistoryLoadContent(context.Background(), h.bus, imgEntryID)
	if err != nil || loaded == nil || !bytes.Equal(loaded.ImagePNG, pngBytes) {
		t.Fatalf("CallClipboardHistoryLoadContent mismatch: loaded=%v err=%v", loaded != nil, err)
	}

	// Stop unregisters Bus capability cleanly.
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if h.bus.Has(purekit.CapClipboardHistory) {
		t.Fatal("expected CapClipboardHistory unregistered on Stop")
	}
}

func TestPureClipLegacyDBReadOnlyFallback(t *testing.T) {
	legacyPNG := encodeTestPNG(t, 16, 16, color.RGBA{G: 220, A: 255})
	sum := sha1.Sum(legacyPNG)
	hexSum := hex.EncodeToString(sum[:])
	legacyRel := filepath.ToSlash(filepath.Join(hexSum[:2], hexSum+".png"))

	var legacyDBPath, legacyImgPath string
	var origDBBytes, origPNGBytes []byte

	h := newTestHost(t, func(home string) {
		legacyDBPath = filepath.Join(home, "shotclip.db")
		legacyImgPath = filepath.Join(home, "images", filepath.FromSlash(legacyRel))
		if err := os.MkdirAll(filepath.Dir(legacyImgPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacyImgPath, legacyPNG, 0o600); err != nil {
			t.Fatal(err)
		}

		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(legacyDBPath))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(clipSchema); err != nil {
			t.Fatal(err)
		}
		now := time.Now().Add(-24 * time.Hour).Unix()
		if _, err := db.Exec(`
			INSERT INTO clipboard_entry (id, kind, preview, body, hash, byte_size, created_utc, last_used_utc, source_process)
			VALUES (10, ?, 'legacy note', 'legacy body content', 'hash-legacy-txt', 19, ?, ?, 'oldapp')`,
			int(core.KindText), now, now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
			INSERT INTO clipboard_entry (id, kind, preview, hash, byte_size, width, height, image_path, created_utc, last_used_utc, source_process)
			VALUES (11, ?, '[图片]', 'hash-legacy-img', ?, 16, 16, ?, ?, ?, 'oldimg')`,
			int(core.KindImage), len(legacyPNG), legacyRel, now+10, now+10); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}

		origDBBytes, err = os.ReadFile(legacyDBPath)
		if err != nil {
			t.Fatal(err)
		}
		origPNGBytes, err = os.ReadFile(legacyImgPath)
		if err != nil {
			t.Fatal(err)
		}
	})

	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatalf("Init with legacy db: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Add a new writable entry in plugins/pureclip/clip.db
	_ = h.io.Write(&core.ClipboardContent{Text: "new pureclip note"})
	h.watcher.Fire()
	h.clip.FlushPending()

	rows, err := p.Service().Search("", nil, nil, 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 merged rows (1 new + 2 legacy), got %d: %+v", len(rows), rows)
	}

	var legacyImgID int64
	for _, r := range rows {
		if r.SourceProcess == "oldimg" {
			legacyImgID = r.ID
			if !r.IsLegacy() {
				t.Fatalf("expected negative legacy ID for oldimg, got %d", r.ID)
			}
		}
	}
	if legacyImgID == 0 {
		t.Fatalf("legacy image row missing from Search: %+v", rows)
	}

	loaded, err := p.Service().LoadContent(legacyImgID)
	if err != nil || loaded == nil || !bytes.Equal(loaded.ImagePNG, legacyPNG) {
		t.Fatalf("LoadContent legacy image mismatch: loaded=%v err=%v", loaded != nil, err)
	}

	out := p.Service().Restore(legacyImgID)
	if !out.Wrote {
		t.Fatal("expected Restore of legacy image to succeed")
	}

	if _, err := p.Service().ToggleFavorite(legacyImgID); !errors.Is(err, ErrLegacyReadOnly) {
		t.Fatalf("ToggleFavorite(legacy) err = %v, want ErrLegacyReadOnly", err)
	}
	if err := p.Service().Delete(legacyImgID); !errors.Is(err, ErrLegacyReadOnly) {
		t.Fatalf("Delete(legacy) err = %v, want ErrLegacyReadOnly", err)
	}

	// Clear, Prune, and CollectOrphans must only touch writable clip.db/blobs.
	if err := p.Service().Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := p.Service().CollectOrphans(); err != nil {
		t.Fatalf("CollectOrphans: %v", err)
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Verify ZERO byte changes and ZERO sidecar files on legacy assets!
	afterDBBytes, err := os.ReadFile(legacyDBPath)
	if err != nil || !bytes.Equal(origDBBytes, afterDBBytes) {
		t.Fatalf("legacy shotclip.db was modified on disk! err=%v", err)
	}
	afterPNGBytes, err := os.ReadFile(legacyImgPath)
	if err != nil || !bytes.Equal(origPNGBytes, afterPNGBytes) {
		t.Fatalf("legacy image file was modified or deleted! err=%v", err)
	}
	if _, err := os.Stat(legacyDBPath + "-wal"); err == nil {
		t.Fatal("unexpected WAL sidecar created for read-only legacy db")
	}
	if _, err := os.Stat(legacyDBPath + "-shm"); err == nil {
		t.Fatal("unexpected SHM sidecar created for read-only legacy db")
	}
}

func TestPureClipHistoryViewBusAndSettingsTab(t *testing.T) {
	h := newTestHost(t, nil)
	var pinnedText string
	var viewedImg image.Image
	var viewedPath string
	_ = h.bus.Register(purekit.CapImagePin, func(_ context.Context, raw any) (any, error) {
		req := raw.(purekit.ImagePinRequest)
		if req.Content != nil {
			pinnedText = req.Content.Text
		}
		return nil, nil
	})
	_ = h.bus.Register(purekit.CapImagePreview, func(_ context.Context, raw any) (any, error) {
		req := raw.(purekit.ImagePreviewRequest)
		viewedImg = req.Image
		viewedPath = req.Path
		return purekit.ImagePreviewResponse{}, nil
	})

	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	_ = h.io.Write(&core.ClipboardContent{Text: "bus pin item"})
	h.watcher.Fire()
	h.clip.FlushPending()
	_ = h.io.Write(&core.ClipboardContent{Text: "rich pin item", HTML: "<b>rich pin item</b>"})
	h.watcher.Fire()
	h.clip.FlushPending()
	_ = h.clip.WriteImage(encodeTestPNG(t, 10, 10, color.RGBA{B: 255, A: 255}), "pureshot")

	hv := newHistoryView(p)
	tt := ui.NewTester(hv.view, 840, 600)
	tt.Frame()
	for _, tabLabel := range []string{"全部", "文本", "图片", "文件", "收藏"} {
		if !tt.HasText(tabLabel) {
			t.Fatalf("historyView missing filter %q: %v", tabLabel, tt.Texts())
		}
	}
	if !tt.HasText("PNG 10×10 ·") || tt.HasText("[图片]") {
		t.Fatalf("expected clean PNG dimensions title without [图片], texts=%v", tt.Texts())
	}

	// "文本" tab merges plain text and rich text cleanly.
	if err := tt.Click("文本"); err != nil {
		t.Fatalf("click 文本 filter: %v", err)
	}
	tt.Frame()
	if len(hv.items) != 2 {
		t.Fatalf("文本 filter items = %d, want 2 (text + richtext)", len(hv.items))
	}
	if err := tt.Click("图片"); err != nil {
		t.Fatalf("click 图片 filter: %v", err)
	}
	tt.Frame()
	if len(hv.items) != 1 || hv.items[0].Kind != core.KindImage {
		t.Fatalf("图片 filter items = %+v, want 1 image item", hv.items)
	}
	_ = tt.Click("全部")
	tt.Frame()

	// Compact view auto-focuses search so typing immediately filters records (including bare digits).
	cv := newCompactHistoryView(p)
	ct := ui.NewTester(cv.view, compactHistoryWidth, compactHistoryHeight)
	ct.Frame()
	if !ct.HasText("全部") || !ct.HasText("PNG 10×10 ·") || !ct.HasText("Esc") {
		t.Fatalf("compactView missing expected minimal chrome: %v", ct.Texts())
	}

	ct.Type("rich")
	ct.Frame()
	if cv.query != "rich" || len(cv.items) != 1 || cv.items[0].Preview != "rich pin item" {
		t.Fatalf("typing immediately into compact view failed: query=%q items=%+v", cv.query, cv.items)
	}
	ct.Key(0, ui.KeyEscape)
	ct.Frame()
	if cv.query != "" || cv.closed {
		t.Fatalf("Esc should clear non-empty search first: query=%q closed=%v", cv.query, cv.closed)
	}
	ct.Key(0, ui.KeyDown)
	ct.Frame()
	if cv.selected != 1 {
		t.Fatalf("KeyDown while search focused selected = %d, want 1", cv.selected)
	}
	ct.Key(0, ui.KeyTab)
	ct.Frame()
	if cv.compact {
		t.Fatal("expected KeyTab to toggle compact view to expanded")
	}

	// Find text row and pin it via Bus
	var textID int64
	var imgID int64
	for _, it := range hv.items {
		if it.Kind == core.KindText {
			textID = it.ID
		}
		if it.Kind == core.KindImage {
			imgID = it.ID
		}
	}
	hv.pinRecord(textID)
	if pinnedText != "bus pin item" {
		t.Fatalf("pin via Bus failed: got %q", pinnedText)
	}
	hv.viewInFreeImage(imgID)
	if viewedImg == nil && viewedPath == "" {
		t.Fatal("view in PureImage via Bus was not invoked")
	}

	// Render self-registered SettingsTab
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
	}, 720, 900)
	st.Frame()
	if !st.HasText("监听与回放") || !st.HasText("历史维护") {
		t.Fatalf("PureClip settings tab missing cards: %v", st.Texts())
	}
	if err := st.Click("开关：记录图片"); err != nil {
		t.Fatalf("click captureImages toggle: %v", err)
	}
	st.Frame()
	if !sv.Dirty() {
		t.Fatal("expected Dirty=true after toggling switch")
	}
	if err := sv.Save(); err != nil {
		t.Fatalf("Save settings tab: %v", err)
	}
	loaded := config.LoadPureClip(h.Config(PluginID))
	if loaded.History.CaptureImages {
		t.Fatalf("expected CaptureImages=false persisted")
	}
	sv.ResetDefaults()
	if !sv.Dirty() {
		t.Fatal("expected Dirty=true after ResetDefaults")
	}
}

func TestPureClipInstantSearchAndCacheRetention(t *testing.T) {
	h := newTestHost(t, nil)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	_ = h.io.Write(&core.ClipboardContent{Text: "alpha beta gamma"})
	h.watcher.Fire()
	h.clip.FlushPending()
	_ = h.io.Write(&core.ClipboardContent{Text: "zeta delta"})
	h.watcher.Fire()
	h.clip.FlushPending()
	_ = h.clip.WriteImage(encodeTestPNG(t, 12, 10, color.RGBA{R: 255, A: 255}), "PureShot")

	hv := newHistoryView(p)
	tt := ui.NewTester(hv.view, 840, 600)
	tt.Frame()

	if len(hv.allItems) != 3 || len(hv.items) != 3 {
		t.Fatalf("expected 3 snapshot/visible items, got all=%d visible=%d", len(hv.allItems), len(hv.items))
	}
	var imgID int64
	for _, it := range hv.items {
		if it.Kind == core.KindImage {
			imgID = it.ID
			break
		}
	}
	if imgID == 0 {
		t.Fatal("missing image record")
	}
	thumbPtr := hv.thumbCache[imgID]
	if thumbPtr == nil {
		t.Fatal("expected image thumbnail to be decoded and cached after initial frame")
	}

	// Search by SourceProcess (case-insensitive) via instant in-memory filter.
	tt.Type("pureshot")
	tt.Frame()
	if len(hv.items) != 1 || hv.items[0].ID != imgID {
		t.Fatalf("SourceProcess search got %+v, want only image id %d", hv.items, imgID)
	}
	// Verify SQL fallback in repo.go also matches source_process.
	sqlHits, err := p.Service().Search("pureshot", nil, nil, 50)
	if err != nil || len(sqlHits) != 1 || sqlHits[0].ID != imgID {
		t.Fatalf("SQL Search(source_process) got %+v err=%v, want id %d", sqlHits, err, imgID)
	}

	// Clearing then entering a non-matching query must NOT evict decoded thumbnail/preview caches.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Type("no-such-match-xyz")
	tt.Frame()
	if len(hv.items) != 0 {
		t.Fatalf("expected 0 items for unmatched query, got %d", len(hv.items))
	}
	if hv.thumbCache[imgID] != thumbPtr {
		t.Fatal("expected thumbnail cache entry to stay retained across empty search results")
	}

	// Multi-token search must match regardless of middle words.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Type("alpha gamma")
	tt.Frame()
	if len(hv.items) != 1 || hv.items[0].Preview != "alpha beta gamma" {
		t.Fatalf("multi-token search got %+v, want alpha beta gamma", hv.items)
	}

	// Returning to full list reuses exact cached thumbnail pointer without re-decoding.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if len(hv.items) != 3 || hv.thumbCache[imgID] != thumbPtr {
		t.Fatalf("expected full list with reused thumbnail ptr %p, got items=%d ptr=%p", thumbPtr, len(hv.items), hv.thumbCache[imgID])
	}

	// Deleting the image record explicitly drops its cache entries.
	hv.pending, hv.pendingID = "delete", imgID
	tt.Frame()
	if _, ok := hv.thumbCache[imgID]; ok {
		t.Fatalf("expected deleted record %d to be removed from thumbCache", imgID)
	}
}

func TestPureClipVibrantRender(t *testing.T) {
	h := newTestHost(t, nil)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	_ = h.io.Write(&core.ClipboardContent{Text: "fluent acrylic item"})
	h.watcher.Fire()
	h.clip.FlushPending()
	_ = h.clip.WriteImage(encodeTestPNG(t, 10, 10, color.RGBA{G: 200, A: 255}), "pureshot")

	cv := newCompactHistoryView(p)
	ct := ui.NewTester(cv.view, compactHistoryWidth, compactHistoryHeight)
	ct.SetVibrancy(true)
	for _, dark := range []bool{true, false} {
		ct.SetDark(dark)
		ct.Frame()
		if !ct.HasText("PNG 10×10 ·") || !ct.HasText("fluent acrylic item") {
			t.Fatalf("compact vibrant dark=%v missing items: %v", dark, ct.Texts())
		}
	}

	hv := newHistoryView(p)
	ht := ui.NewTester(hv.view, 840, 600)
	ht.SetVibrancy(true)
	for _, dark := range []bool{true, false} {
		ht.SetDark(dark)
		ht.Frame()
		if !ht.HasText("粘贴 ↵") {
			t.Fatalf("expanded vibrant dark=%v missing action bar: %v", dark, ht.Texts())
		}
	}

	hv.selected = 0
	if rec := hv.selectedRecord(); rec != nil && rec.Kind == core.KindImage {
		hv.loadPreview(rec)
	}
	pt := ui.NewTester(hv.previewPopoverView, 360, 280)
	pt.SetVibrancy(true)
	for _, dark := range []bool{true, false} {
		pt.SetDark(dark)
		pt.Frame()
		if !pt.HasText("Tab Full View") && !pt.HasText("Tab 完整视图") {
			t.Fatalf("popover vibrant dark=%v missing footer: %v", dark, pt.Texts())
		}
	}
}
