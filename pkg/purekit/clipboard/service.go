package clipboard

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/platform"
)

// IO abstracts low-level clipboard reads and writes so Manager can run with
// Win32 handles in production and pure mocks in unit tests.
type IO interface {
	Read() (*core.ClipboardContent, error)
	ReadImagePNG() ([]byte, error)
	Write(content *core.ClipboardContent) error
}

// Watcher abstracts OS clipboard change notifications.
type Watcher interface {
	OnUpdate(fn func())
	Start() error
	Stop()
	Close()
}

// WriteOptions controls source attribution and self-write handling for Write.
type WriteOptions struct {
	Source           string
	Ephemeral        bool
	SuppressDuration time.Duration
}

// Event describes a clipboard change observed by Service, either ingested from
// the OS clipboard or broadcast directly from an in-process plugin write.
type Event struct {
	Content       *core.ClipboardContent
	Source        string
	SourceProcess string
	Ephemeral     bool
	SelfWrite     bool
	Timestamp     time.Time
}

// Service provides shared thread-safe clipboard I/O, source-tagged self-write
// suppression, debounced monitoring, and change subscriptions.
type Service interface {
	Read() (*core.ClipboardContent, error)
	ReadImagePNG() ([]byte, error)
	Write(content *core.ClipboardContent, opts WriteOptions) error
	WriteImage(png []byte, source string) error
	WriteText(text string, source string) error
	WriteEphemeralText(text string, source string) error
	SuppressSelfWrite(source string, duration time.Duration)
	Subscribe(handler func(Event)) (unsubscribe func())
	SetTimings(debounce, suppress time.Duration)
	FlushPending()
	Start() error
	Stop()
	Monitoring() bool
	Close()
}

// ManagerOptions configures Manager timings and logging.
type ManagerOptions struct {
	Debounce          time.Duration
	SelfWriteSuppress time.Duration
	DelayedImageRetry time.Duration
	Log               *core.Logger
}

type subscriberEntry struct {
	id int64
	fn func(Event)
}

// Manager implements Service over pluggable IO and Watcher backends.
type Manager struct {
	io         IO
	watcher    Watcher
	foreground func() string
	log        *core.Logger

	mu             sync.Mutex
	debounce       time.Duration
	suppress       time.Duration
	delayedRetry   time.Duration
	monitoring     bool
	busy           bool
	pendingIngest  bool
	suppressUntil  time.Time
	suppressSource string
	timer          *time.Timer
	closed         bool

	subMu  sync.RWMutex
	nextID atomic.Int64
	subs   []subscriberEntry
}

// NewManager creates a source-tagged clipboard Manager.
func NewManager(io IO, watcher Watcher, foreground func() string, opts ManagerOptions) *Manager {
	if opts.Debounce <= 0 {
		opts.Debounce = 160 * time.Millisecond
	}
	if opts.SelfWriteSuppress < 0 {
		opts.SelfWriteSuppress = 0
	} else if opts.SelfWriteSuppress == 0 {
		opts.SelfWriteSuppress = 400 * time.Millisecond
	}
	if opts.DelayedImageRetry <= 0 {
		opts.DelayedImageRetry = 150 * time.Millisecond
	}
	m := &Manager{
		io:           io,
		watcher:      watcher,
		foreground:   foreground,
		log:          opts.Log,
		debounce:     opts.Debounce,
		suppress:     opts.SelfWriteSuppress,
		delayedRetry: opts.DelayedImageRetry,
	}
	if watcher != nil {
		watcher.OnUpdate(m.onClipboardUpdate)
	}
	return m
}

// NewWin32Manager creates a Manager backed by Win32 Clipboard and ClipboardListener.
func NewWin32Manager(opts ManagerOptions) (*Manager, error) {
	listener, err := platform.NewClipboardListener()
	if err != nil {
		return nil, err
	}
	clip := platform.NewClipboard(listener.Handle())
	return NewManager(clip, listener, platform.ForegroundProcessName, opts), nil
}

func (m *Manager) Read() (*core.ClipboardContent, error) {
	if m == nil || m.io == nil {
		return nil, nil
	}
	return m.io.Read()
}

func (m *Manager) ReadImagePNG() ([]byte, error) {
	if m == nil || m.io == nil {
		return nil, nil
	}
	return m.io.ReadImagePNG()
}

func (m *Manager) SetTimings(debounce, suppress time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if debounce >= 0 {
		m.debounce = debounce
	}
	if suppress >= 0 {
		m.suppress = suppress
	}
}

func (m *Manager) SuppressSelfWrite(source string, duration time.Duration) {
	m.mu.Lock()
	dur := duration
	if dur <= 0 {
		dur = m.suppress
	}
	until := time.Now().Add(dur)
	if until.After(m.suppressUntil) {
		m.suppressUntil = until
	}
	m.suppressSource = source
	m.mu.Unlock()
}

func (m *Manager) Write(content *core.ClipboardContent, opts WriteOptions) error {
	if m == nil || m.io == nil {
		return nil
	}
	m.SuppressSelfWrite(opts.Source, opts.SuppressDuration)
	if err := m.io.Write(content); err != nil {
		return err
	}
	if !opts.Ephemeral {
		m.emit(Event{
			Content:       cloneContent(content),
			Source:        opts.Source,
			SourceProcess: opts.Source,
			Ephemeral:     false,
			SelfWrite:     true,
			Timestamp:     time.Now(),
		})
	}
	return nil
}

func (m *Manager) WriteImage(png []byte, source string) error {
	return m.Write(&core.ClipboardContent{ImagePNG: png}, WriteOptions{Source: source})
}

func (m *Manager) WriteText(text string, source string) error {
	return m.Write(&core.ClipboardContent{Text: text}, WriteOptions{Source: source})
}

func (m *Manager) WriteEphemeralText(text string, source string) error {
	return m.Write(&core.ClipboardContent{Text: text}, WriteOptions{Source: source, Ephemeral: true})
}

func (m *Manager) Subscribe(handler func(Event)) func() {
	if handler == nil {
		return func() {}
	}
	id := m.nextID.Add(1)
	m.subMu.Lock()
	m.subs = append(m.subs, subscriberEntry{id: id, fn: handler})
	m.subMu.Unlock()
	return func() {
		m.subMu.Lock()
		for i, sub := range m.subs {
			if sub.id == id {
				m.subs = append(m.subs[:i], m.subs[i+1:]...)
				break
			}
		}
		m.subMu.Unlock()
	}
}

func (m *Manager) emit(ev Event) {
	m.subMu.RLock()
	subs := append([]subscriberEntry(nil), m.subs...)
	m.subMu.RUnlock()
	for _, sub := range subs {
		sub.fn(ev)
	}
}

func (m *Manager) Start() error {
	if m.watcher != nil {
		if err := m.watcher.Start(); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.monitoring = true
	m.mu.Unlock()
	return nil
}

func (m *Manager) Stop() {
	if m.watcher != nil {
		m.watcher.Stop()
	}
	m.mu.Lock()
	m.monitoring = false
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.mu.Unlock()
}

func (m *Manager) Monitoring() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.monitoring
}

func (m *Manager) Close() {
	m.Stop()
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	if m.watcher != nil {
		m.watcher.Close()
	}
}

func (m *Manager) FlushPending() {
	if m == nil || m.io == nil {
		return
	}
	m.mu.Lock()
	hadTimer := m.timer != nil
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	canFlush := m.monitoring && !m.closed && !time.Now().Before(m.suppressUntil)
	m.mu.Unlock()
	if hadTimer && canFlush {
		m.ingest()
	}
}

func (m *Manager) onClipboardUpdate() {
	m.mu.Lock()
	if m.closed || !m.monitoring || time.Now().Before(m.suppressUntil) {
		m.mu.Unlock()
		return
	}
	if m.busy {
		m.pendingIngest = true
	}
	if m.timer != nil {
		m.timer.Stop()
	}
	delay := m.debounce
	if delay < 20*time.Millisecond {
		delay = 20 * time.Millisecond
	}
	m.timer = time.AfterFunc(delay, func() {
		go m.ingest()
	})
	m.mu.Unlock()
}

func (m *Manager) ingest() {
	m.mu.Lock()
	if m.closed || m.io == nil {
		m.mu.Unlock()
		return
	}
	if m.busy {
		m.pendingIngest = true
		m.mu.Unlock()
		return
	}
	if time.Now().Before(m.suppressUntil) {
		m.mu.Unlock()
		return
	}
	m.busy = true
	retryDelay := m.delayedRetry
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.busy = false
		rerun := m.pendingIngest && !m.closed && (m.monitoring || m.watcher == nil)
		m.pendingIngest = false
		debounce := m.debounce
		if debounce < 20*time.Millisecond {
			debounce = 20 * time.Millisecond
		}
		m.mu.Unlock()
		if rerun {
			time.AfterFunc(debounce, func() {
				go m.ingest()
			})
		}
	}()

	content, err := m.io.Read()
	if err != nil {
		if m.log != nil {
			m.log.Warnf("clipboard read failed: %v", err)
		}
		return
	}
	if content != nil && content.IsEmpty() {
		if hasImageFormat(content.Formats) {
			time.Sleep(retryDelay)
			if retryContent, rerr := m.io.Read(); rerr == nil && retryContent != nil && !retryContent.IsEmpty() {
				content = retryContent
			}
		}
		if content == nil || content.IsEmpty() {
			return
		}
	}
	if content == nil || content.IsEmpty() {
		return
	}
	proc := ""
	if m.foreground != nil {
		proc = m.foreground()
	}
	m.emit(Event{
		Content:       content,
		Source:        "",
		SourceProcess: proc,
		Ephemeral:     false,
		SelfWrite:     false,
		Timestamp:     time.Now(),
	})
}

func hasImageFormat(formats []string) bool {
	for _, f := range formats {
		up := strings.ToUpper(f)
		switch up {
		case "PNG", "IMAGE/PNG", "CF_DIB", "CF_DIBV5", "CF_BITMAP", "IMAGE/BMP", "JFIF", "IMAGE/JPEG", "GIF", "IMAGE/GIF", "HTML FORMAT":
			return true
		}
	}
	return false
}

func cloneContent(c *core.ClipboardContent) *core.ClipboardContent {
	if c == nil {
		return nil
	}
	out := *c
	out.Files = slices.Clone(c.Files)
	out.ImagePNG = slices.Clone(c.ImagePNG)
	out.Formats = slices.Clone(c.Formats)
	return &out
}
