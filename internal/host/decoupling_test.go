package host

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
)

type memClipIO struct {
	mu      sync.Mutex
	current *core.ClipboardContent
}

func (m *memClipIO) Read() (*core.ClipboardContent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current, nil
}

func (m *memClipIO) ReadImagePNG() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return nil, nil
	}
	return m.current.ImagePNG, nil
}

func (m *memClipIO) Write(c *core.ClipboardContent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = c
	return nil
}

type memClipWatcher struct {
	mu sync.Mutex
	fn func()
}

func (w *memClipWatcher) OnUpdate(fn func()) {
	w.mu.Lock()
	w.fn = fn
	w.mu.Unlock()
}
func (w *memClipWatcher) Start() error { return nil }
func (w *memClipWatcher) Stop()        {}
func (w *memClipWatcher) Close()       {}
func (w *memClipWatcher) Fire() {
	w.mu.Lock()
	fn := w.fn
	w.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func newMockClipboard(t *testing.T) (*memClipIO, *memClipWatcher, clipboard.Service) {
	t.Helper()
	io := &memClipIO{}
	watcher := &memClipWatcher{}
	svc := clipboard.NewManager(io, watcher, func() string { return "notepad.exe" }, clipboard.ManagerOptions{
		Debounce:          5 * time.Millisecond,
		SelfWriteSuppress: 200 * time.Millisecond,
	})
	return io, watcher, svc
}

func encodeTinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 12, 12))
	img.SetRGBA(1, 1, color.RGBA{R: 120, G: 210, B: 80, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecouplingPureShotPlusPureClipWithoutPureImage(t *testing.T) {
	compiled := CompiledModules()
	if !compiled.Capture || !compiled.History {
		t.Skip("requires both PureShot and PureClip compiled")
	}

	_, _, clip := newMockClipboard(t)
	a := testHostWithModules(t, core.ModuleConfig{Capture: true, History: true, Viewer: false, Zip: false}, clip, nil)
	a.Start([]string{"--background"})

	if a.Bus().Has(purekit.CapImagePreview) {
		t.Fatal("CapImagePreview must not be registered when PureImage is disabled")
	}
	if !a.Bus().Has(purekit.CapImagePin) || !a.Bus().Has(purekit.CapClipboardHistory) {
		t.Fatalf("expected CapImagePin & CapClipboardHistory registered, got %v", a.Bus().List())
	}

	// Simulate PureShot writing an image via shared host.Clipboard() without direct PureClip import.
	if err := a.Clipboard().WriteImage(encodeTinyPNG(t), config.PluginPureShot); err != nil {
		t.Fatal(err)
	}
	if err := purekit.CallClipboardHistoryFlush(context.Background(), a.Bus()); err != nil {
		t.Fatal(err)
	}
	recent, err := purekit.CallClipboardHistoryListRecent(context.Background(), a.Bus(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Entries) != 1 {
		t.Fatalf("expected 1 history entry from PureShot write, got %d", len(recent.Entries))
	}
	if recent.Entries[0].Kind != core.KindImage || recent.Entries[0].SourceProcess != "PureShot" {
		t.Fatalf("entry = %+v, want Kind=Image SourceProcess=PureShot", recent.Entries[0])
	}
}

func TestDecouplingPureClipAlone(t *testing.T) {
	if !CompiledModules().History {
		t.Skip("requires PureClip compiled")
	}
	io, watcher, clip := newMockClipboard(t)
	a := testHostWithModules(t, core.ModuleConfig{History: true}, clip, nil)
	a.Start([]string{"--background"})

	if a.Bus().Has(purekit.CapImagePin) || a.Bus().Has(purekit.CapImagePreview) {
		t.Fatalf("unexpected sibling capabilities when running PureClip alone: %v", a.Bus().List())
	}

	_ = io.Write(&core.ClipboardContent{Text: "solo clipboard item"})
	watcher.Fire()
	_ = purekit.CallClipboardHistoryFlush(context.Background(), a.Bus())

	recent, err := purekit.CallClipboardHistoryListRecent(context.Background(), a.Bus(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Entries) != 1 || recent.Entries[0].Preview != "solo clipboard item" {
		t.Fatalf("solo PureClip entries = %+v", recent.Entries)
	}
}

func TestDecouplingPureImageAlone(t *testing.T) {
	if !CompiledModules().Viewer {
		t.Skip("requires PureImage compiled")
	}
	a := testHostWithModules(t, core.ModuleConfig{Viewer: true}, nil, nil)
	a.Start([]string{"--background"})

	if !a.Bus().Has(purekit.CapImagePreview) {
		t.Fatal("expected CapImagePreview registered when PureImage enabled")
	}
	if a.Bus().Has(purekit.CapImagePin) || a.Bus().Has(purekit.CapClipboardHistory) {
		t.Fatalf("unexpected non-PureImage capabilities: %v", a.Bus().List())
	}
	exts, err := purekit.CallImagePreviewSupportedExts(context.Background(), a.Bus())
	if err != nil || len(exts) == 0 {
		t.Fatalf("CallImagePreviewSupportedExts = (%v, %v)", exts, err)
	}
	if a.ShouldStayInTray() {
		t.Error("expected ShouldStayInTray=false by default for PureImage alone")
	}
}

func TestDecouplingPureZipAlone(t *testing.T) {
	if !CompiledModules().Zip {
		t.Skip("requires PureZip compiled")
	}
	a := testHostWithModules(t, core.ModuleConfig{Zip: true}, nil, nil)
	a.Start([]string{"--background"})

	if a.Bus().Has(purekit.CapImagePreview) || a.Bus().Has(purekit.CapImagePin) || a.Bus().Has(purekit.CapClipboardHistory) || a.Bus().Has(purekit.CapVideoPreview) {
		t.Fatalf("unexpected capabilities when running PureZip alone: %v", a.Bus().List())
	}
	sampleDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(sampleDir, "hello.txt"), []byte("zip-only"), 0o600)
	// Invoking CmdOpenZip with a nonexistent path should not panic cleanly in headless mode.
	a.ExecuteCommand(core.CmdOpenZip)
}

func TestDecouplingPureVideoAlone(t *testing.T) {
	if !CompiledModules().Video {
		t.Skip("requires PureVideo compiled")
	}
	a := testHostWithModules(t, core.ModuleConfig{Video: true}, nil, nil)
	a.Start([]string{"--background"})

	if !a.Bus().Has(purekit.CapVideoPreview) {
		t.Fatal("expected CapVideoPreview registered when PureVideo enabled")
	}
	if a.Bus().Has(purekit.CapImagePreview) || a.Bus().Has(purekit.CapImagePin) || a.Bus().Has(purekit.CapClipboardHistory) {
		t.Fatalf("unexpected sibling capabilities when running PureVideo alone: %v", a.Bus().List())
	}
	exts, err := purekit.CallVideoPreviewSupportedExts(context.Background(), a.Bus())
	if err != nil || len(exts) == 0 {
		t.Fatalf("CallVideoPreviewSupportedExts = (%v, %v)", exts, err)
	}
	if a.ShouldStayInTray() {
		t.Error("expected ShouldStayInTray=false for PureVideo alone")
	}
}

func TestDecouplingAllCompiledPluginsAndShutdown(t *testing.T) {
	compiled := CompiledModules()
	_, _, clip := newMockClipboard(t)
	a := testHostWithModules(t, compiled, clip, nil)
	a.Start([]string{"--background"})

	if compiled.Capture && !a.Bus().Has(purekit.CapImagePin) {
		t.Error("missing CapImagePin")
	}
	if compiled.History && !a.Bus().Has(purekit.CapClipboardHistory) {
		t.Error("missing CapClipboardHistory")
	}
	if compiled.Viewer && !a.Bus().Has(purekit.CapImagePreview) {
		t.Error("missing CapImagePreview")
	}
	if compiled.Video && !a.Bus().Has(purekit.CapVideoPreview) {
		t.Error("missing CapVideoPreview")
	}

	a.Shutdown()
	if caps := a.Bus().List(); len(caps) != 0 {
		t.Fatalf("expected Shutdown() to unregister all capabilities, still have: %v", caps)
	}
}
