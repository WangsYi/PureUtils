package clipboard

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"pureutils/pkg/purekit/core"
)

type mockIO struct {
	mu      sync.Mutex
	seq     int
	content *core.ClipboardContent
	reads   []*core.ClipboardContent
}

func (m *mockIO) Read() (*core.ClipboardContent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.reads) > 0 {
		c := m.reads[m.seq%len(m.reads)]
		if m.seq < len(m.reads)-1 {
			m.seq++
		}
		return cloneContent(c), nil
	}
	return cloneContent(m.content), nil
}

func (m *mockIO) ReadImagePNG() ([]byte, error) {
	c, err := m.Read()
	if err != nil || c == nil {
		return nil, err
	}
	return c.ImagePNG, nil
}

func (m *mockIO) Write(c *core.ClipboardContent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.content = cloneContent(c)
	return nil
}

type mockWatcher struct {
	mu       sync.Mutex
	started  bool
	onUpdate func()
}

func (w *mockWatcher) OnUpdate(fn func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onUpdate = fn
}

func (w *mockWatcher) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.started = true
	return nil
}

func (w *mockWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.started = false
}

func (w *mockWatcher) Close() {}

func (w *mockWatcher) fire() {
	w.mu.Lock()
	fn := w.onUpdate
	w.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func TestWriteSourceTagAndSuppressOSEcho(t *testing.T) {
	io := &mockIO{}
	w := &mockWatcher{}
	mgr := NewManager(io, w, func() string { return "notepad" }, ManagerOptions{
		Debounce:          25 * time.Millisecond,
		SelfWriteSuppress: 250 * time.Millisecond,
	})
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	var events []Event
	var mu sync.Mutex
	unsub := mgr.Subscribe(func(ev Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})
	defer unsub()

	pngBytes := []byte("shot-png-payload")
	if err := mgr.WriteImage(pngBytes, "pureshot"); err != nil {
		t.Fatal(err)
	}
	// Simulated OS WM_CLIPBOARDUPDATE echo inside suppression window must be dropped.
	w.fire()
	time.Sleep(60 * time.Millisecond)

	mu.Lock()
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1 (echo must be suppressed)", len(events))
	}
	if events[0].Source != "pureshot" || !events[0].SelfWrite || !bytes.Equal(events[0].Content.ImagePNG, pngBytes) {
		t.Fatalf("unexpected source-tagged event: %+v", events[0])
	}
	mu.Unlock()

	// Ephemeral write (e.g. magnifier hex color) suppresses OS echo and does not emit event.
	if err := mgr.WriteEphemeralText("#3B82F6", "pureshot"); err != nil {
		t.Fatal(err)
	}
	w.fire()
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	if len(events) != 1 {
		t.Fatalf("ephemeral write leaked event: %d", len(events))
	}
	mu.Unlock()
}

func TestExternalUpdateDebounceFlushUnsubscribeAndDelayedImageRetry(t *testing.T) {
	io := &mockIO{
		reads: []*core.ClipboardContent{
			{Formats: []string{"CF_DIB"}},
			{Formats: []string{"CF_DIB"}, ImagePNG: []byte("delayed-png")},
		},
	}
	w := &mockWatcher{}
	mgr := NewManager(io, w, func() string { return "chrome" }, ManagerOptions{
		Debounce:          40 * time.Millisecond,
		SelfWriteSuppress: 200 * time.Millisecond,
		DelayedImageRetry: 15 * time.Millisecond,
	})
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	ch := make(chan Event, 4)
	unsub := mgr.Subscribe(func(ev Event) { ch <- ev })

	w.fire()
	w.fire()
	mgr.FlushPending()

	select {
	case ev := <-ch:
		if ev.SelfWrite || ev.SourceProcess != "chrome" || string(ev.Content.ImagePNG) != "delayed-png" {
			t.Fatalf("unexpected external event: %+v", ev)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for delayed-retry clipboard event")
	}

	unsub()
	io.reads = []*core.ClipboardContent{{Text: "after-unsub"}}
	io.seq = 0
	w.fire()
	time.Sleep(80 * time.Millisecond)
	select {
	case ev := <-ch:
		t.Fatalf("expected no events after unsubscribe, got %+v", ev)
	default:
	}
}
