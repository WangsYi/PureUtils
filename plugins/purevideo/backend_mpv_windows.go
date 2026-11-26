//go:build windows

package purevideo

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"pureutils/pkg/purekit/i18n"
)

const (
	mpvFormatFlag   int32 = 3
	mpvFormatInt64  int32 = 4
	mpvFormatDouble int32 = 5

	mpvEventShutdown   int32 = 1
	mpvEventEndFile    int32 = 7
	mpvEventFileLoaded int32 = 8

	mpvEndReasonEOF   int32 = 0
	mpvEndReasonError int32 = 4
)

type mpvEventRaw struct {
	EventID       int32
	Error         int32
	ReplyUserdata uint64
	Data          uintptr
}

type mpvEndFileRaw struct {
	Reason int32
	Error  int32
}

// LibMPVCandidates returns ordered search paths for libmpv-2.dll.
func LibMPVCandidates(pluginDir string) []string {
	var out []string
	if strings.TrimSpace(pluginDir) != "" {
		out = append(out, filepath.Join(pluginDir, "libmpv-2.dll"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		out = append(out,
			filepath.Join(dir, "libmpv-2.dll"),
			filepath.Join(dir, "libs", "mpv", "libmpv-2.dll"),
		)
	}
	out = append(out,
		filepath.Join("libs", "mpv", "libmpv-2.dll"),
		"libmpv-2.dll",
	)
	return out
}

// FindLibMPV locates the first valid libmpv-2.dll candidate on disk.
func FindLibMPV(pluginDir string) (string, bool) {
	for _, cand := range LibMPVCandidates(pluginDir) {
		if cand == "libmpv-2.dll" {
			if _, err := ProbeLibMPV(cand); err == nil {
				return cand, true
			}
			continue
		}
		if info, err := os.Stat(cand); err == nil && !info.IsDir() && info.Size() > 0 {
			return cand, true
		}
	}
	return "", false
}

// ProbeLibMPV temporarily loads dllPath, invokes mpv_client_api_version, and
// frees the DLL handle immediately so files on disk do not remain locked.
func ProbeLibMPV(dllPath string) (uint32, error) {
	h, err := windows.LoadLibrary(dllPath)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", i18n.F("无法加载 %s", "Failed to load %s", filepath.Base(dllPath)), err)
	}
	defer func() { _ = windows.FreeLibrary(h) }()

	proc, err := windows.GetProcAddress(h, "mpv_client_api_version")
	if err != nil || proc == 0 {
		return 0, fmt.Errorf("%s", i18n.F("%s 缺少 mpv_client_api_version 导出", "%s is missing mpv_client_api_version export", filepath.Base(dllPath)))
	}
	ver, _, _ := syscall.SyscallN(proc)
	if ver == 0 {
		return 0, fmt.Errorf("%s", i18n.F("%s 返回无效的 API 版本", "%s returned an invalid API version", filepath.Base(dllPath)))
	}
	return uint32(ver), nil
}

type mpvProcs struct {
	create           uintptr
	initialize       uintptr
	terminateDestroy uintptr
	setOptionString  uintptr
	command          uintptr
	setProperty      uintptr
	getProperty      uintptr
	waitEvent        uintptr
	wakeup           uintptr
	errorString      uintptr
}

type mpvBackend struct {
	mu       sync.Mutex
	dll      windows.Handle
	ctx      uintptr
	procs    mpvProcs
	hwnd     uintptr
	cb       BackendCallbacks
	state    PlaybackState
	closed   atomic.Bool
	doneCh   chan struct{}
	autoPlay bool
	duration time.Duration
	natW     int
	natH     int
}

func resolveMPVProcs(h windows.Handle) (mpvProcs, error) {
	var p mpvProcs
	need := []struct {
		name string
		dst  *uintptr
	}{
		{"mpv_create", &p.create},
		{"mpv_initialize", &p.initialize},
		{"mpv_terminate_destroy", &p.terminateDestroy},
		{"mpv_set_option_string", &p.setOptionString},
		{"mpv_command", &p.command},
		{"mpv_set_property", &p.setProperty},
		{"mpv_get_property", &p.getProperty},
		{"mpv_wait_event", &p.waitEvent},
		{"mpv_wakeup", &p.wakeup},
		{"mpv_error_string", &p.errorString},
	}
	for _, n := range need {
		addr, err := windows.GetProcAddress(h, n.name)
		if err != nil || addr == 0 {
			return mpvProcs{}, fmt.Errorf("%s", i18n.F("libmpv-2.dll 缺少导出符号 %s", "libmpv-2.dll is missing exported symbol %s", n.name))
		}
		*n.dst = addr
	}
	return p, nil
}

func cstr(s string) ([]byte, uintptr) {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return b, uintptr(unsafe.Pointer(&b[0]))
}

func goStringFromCStr(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var buf [512]byte
	n := 0
	for n < len(buf) {
		ch := *(*byte)(unsafe.Pointer(ptr + uintptr(n)))
		if ch == 0 {
			break
		}
		buf[n] = ch
		n++
	}
	return string(buf[:n])
}

func newMPVBackend(dllPath string, hwnd uintptr, cb BackendCallbacks) (*mpvBackend, error) {
	h, err := windows.LoadLibrary(dllPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.S("加载 libmpv-2.dll 失败", "Failed to load libmpv-2.dll"), err)
	}
	procs, err := resolveMPVProcs(h)
	if err != nil {
		_ = windows.FreeLibrary(h)
		return nil, err
	}
	mpvCtx, _, _ := syscall.SyscallN(procs.create)
	if mpvCtx == 0 {
		_ = windows.FreeLibrary(h)
		return nil, fmt.Errorf("%s", i18n.S("mpv_create 返回空句柄", "mpv_create returned a null handle"))
	}

	b := &mpvBackend{
		dll:    h,
		ctx:    mpvCtx,
		procs:  procs,
		hwnd:   hwnd,
		cb:     cb,
		state:  StateEmpty,
		doneCh: make(chan struct{}),
	}

	opts := [][2]string{
		{"wid", strconv.FormatInt(int64(hwnd), 10)},
		{"hwdec", "auto-safe"},
		{"keep-open", "yes"},
		{"osc", "no"},
		{"osd-level", "0"},
		{"input-default-bindings", "no"},
		{"input-vo-keyboard", "no"},
		{"background-color", "#0B0D12"},
		{"audio-pitch-correction", "yes"},
		{"sub-auto", "fuzzy"},
		{"terminal", "no"},
	}
	for _, kv := range opts {
		kb, kp := cstr(kv[0])
		vb, vp := cstr(kv[1])
		_, _, _ = syscall.SyscallN(procs.setOptionString, mpvCtx, kp, vp)
		runtime.KeepAlive(kb)
		runtime.KeepAlive(vb)
	}

	rc, _, _ := syscall.SyscallN(procs.initialize, mpvCtx)
	if int32(rc) < 0 {
		_, _, _ = syscall.SyscallN(procs.terminateDestroy, mpvCtx)
		_ = windows.FreeLibrary(h)
		return nil, fmt.Errorf("%s", i18n.F("mpv_initialize 失败 (%d)", "mpv_initialize failed (%d)", int32(rc)))
	}

	go b.eventLoop()
	return b, nil
}

func (b *mpvBackend) Kind() string { return EngineMPV }

func (b *mpvBackend) eventLoop() {
	defer close(b.doneCh)
	for {
		if b.closed.Load() {
			return
		}
		b.mu.Lock()
		ctx := b.ctx
		waitFn := b.procs.waitEvent
		b.mu.Unlock()
		if ctx == 0 {
			return
		}
		evPtr, _, _ := syscall.SyscallN(waitFn, ctx, uintptr(math.Float64bits(0.25)))
		if evPtr == 0 {
			continue
		}
		ev := *(*mpvEventRaw)(unsafe.Pointer(evPtr))
		switch ev.EventID {
		case mpvEventShutdown:
			return
		case mpvEventFileLoaded:
			dur := b.queryDuration()
			nw, nh := b.querySize()
			b.mu.Lock()
			b.duration = dur
			b.natW, b.natH = nw, nh
			autoPlay := b.autoPlay
			if autoPlay {
				b.state = StatePlaying
			} else {
				b.state = StatePaused
			}
			b.mu.Unlock()
			_ = b.setBoolProp("pause", !autoPlay)
			if b.cb.OnReady != nil {
				b.cb.OnReady()
			}
			if b.cb.OnState != nil {
				b.cb.OnState(b.State())
			}
		case mpvEventEndFile:
			if ev.Data != 0 {
				ef := *(*mpvEndFileRaw)(unsafe.Pointer(ev.Data))
				switch ef.Reason {
				case mpvEndReasonEOF:
					b.setState(StatePaused)
					if b.cb.OnEnded != nil {
						b.cb.OnEnded()
					}
				case mpvEndReasonError:
					msg := b.errStr(ef.Error)
					b.setState(StateError)
					if b.cb.OnError != nil {
						b.cb.OnError(uint32(ef.Error), msg)
					}
				}
			}
		}
	}
}

func (b *mpvBackend) errStr(code int32) string {
	b.mu.Lock()
	fn := b.procs.errorString
	b.mu.Unlock()
	if fn == 0 {
		return i18n.F("libmpv 错误 (%d)", "libmpv error (%d)", code)
	}
	ptr, _, _ := syscall.SyscallN(fn, uintptr(code))
	msg := goStringFromCStr(ptr)
	if msg == "" {
		return i18n.F("libmpv 错误 (%d)", "libmpv error (%d)", code)
	}
	return i18n.S("libmpv 无法播放此视频: ", "libmpv cannot play this video: ") + msg
}

func (b *mpvBackend) execCommand(args ...string) error {
	if b.closed.Load() {
		return fmt.Errorf("%s", i18n.S("播放器已关闭", "Player is closed"))
	}
	b.mu.Lock()
	ctx, cmdFn := b.ctx, b.procs.command
	b.mu.Unlock()
	if ctx == 0 {
		return fmt.Errorf("%s", i18n.S("播放器未初始化", "Player is not initialized"))
	}
	bufs := make([][]byte, len(args))
	ptrs := make([]uintptr, len(args)+1)
	for i, a := range args {
		bufs[i], ptrs[i] = cstr(a)
	}
	rc, _, _ := syscall.SyscallN(cmdFn, ctx, uintptr(unsafe.Pointer(&ptrs[0])))
	runtime.KeepAlive(bufs)
	runtime.KeepAlive(ptrs)
	if int32(rc) < 0 {
		return fmt.Errorf("%s", b.errStr(int32(rc)))
	}
	return nil
}

func (b *mpvBackend) setBoolProp(name string, v bool) error {
	if b.closed.Load() {
		return nil
	}
	b.mu.Lock()
	ctx, fn := b.ctx, b.procs.setProperty
	b.mu.Unlock()
	if ctx == 0 {
		return nil
	}
	var flag int32
	if v {
		flag = 1
	}
	kb, kp := cstr(name)
	rc, _, _ := syscall.SyscallN(fn, ctx, kp, uintptr(mpvFormatFlag), uintptr(unsafe.Pointer(&flag)))
	runtime.KeepAlive(kb)
	if int32(rc) < 0 {
		return fmt.Errorf("%s", b.errStr(int32(rc)))
	}
	return nil
}

func (b *mpvBackend) setDoubleProp(name string, v float64) error {
	if b.closed.Load() {
		return nil
	}
	b.mu.Lock()
	ctx, fn := b.ctx, b.procs.setProperty
	b.mu.Unlock()
	if ctx == 0 {
		return nil
	}
	kb, kp := cstr(name)
	val := v
	rc, _, _ := syscall.SyscallN(fn, ctx, kp, uintptr(mpvFormatDouble), uintptr(unsafe.Pointer(&val)))
	runtime.KeepAlive(kb)
	if int32(rc) < 0 {
		return fmt.Errorf("%s", b.errStr(int32(rc)))
	}
	return nil
}

func (b *mpvBackend) getDoubleProp(name string) (float64, bool) {
	if b.closed.Load() {
		return 0, false
	}
	b.mu.Lock()
	ctx, fn := b.ctx, b.procs.getProperty
	b.mu.Unlock()
	if ctx == 0 {
		return 0, false
	}
	kb, kp := cstr(name)
	var val float64
	rc, _, _ := syscall.SyscallN(fn, ctx, kp, uintptr(mpvFormatDouble), uintptr(unsafe.Pointer(&val)))
	runtime.KeepAlive(kb)
	return val, int32(rc) >= 0
}

func (b *mpvBackend) getInt64Prop(name string) (int64, bool) {
	if b.closed.Load() {
		return 0, false
	}
	b.mu.Lock()
	ctx, fn := b.ctx, b.procs.getProperty
	b.mu.Unlock()
	if ctx == 0 {
		return 0, false
	}
	kb, kp := cstr(name)
	var val int64
	rc, _, _ := syscall.SyscallN(fn, ctx, kp, uintptr(mpvFormatInt64), uintptr(unsafe.Pointer(&val)))
	runtime.KeepAlive(kb)
	return val, int32(rc) >= 0
}

func (b *mpvBackend) queryDuration() time.Duration {
	if d, ok := b.getDoubleProp("duration"); ok && d > 0 && !math.IsNaN(d) && !math.IsInf(d, 0) {
		return time.Duration(d * float64(time.Second))
	}
	return 0
}

func (b *mpvBackend) querySize() (int, int) {
	w, okW := b.getInt64Prop("video-params/w")
	h, okH := b.getInt64Prop("video-params/h")
	if okW && okH && w > 0 && h > 0 {
		return int(w), int(h)
	}
	return 0, 0
}

func (b *mpvBackend) setState(st PlaybackState) {
	b.mu.Lock()
	if b.closed.Load() {
		b.mu.Unlock()
		return
	}
	b.state = st
	b.mu.Unlock()
	if b.cb.OnState != nil {
		b.cb.OnState(st)
	}
}

func (b *mpvBackend) Open(path string, autoPlay bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("%s", i18n.S("视频路径为空", "Video path is empty"))
	}
	b.mu.Lock()
	b.autoPlay = autoPlay
	b.duration = 0
	b.natW, b.natH = 0, 0
	b.state = StateLoading
	b.mu.Unlock()
	if b.cb.OnState != nil {
		b.cb.OnState(StateLoading)
	}
	_ = b.setBoolProp("pause", !autoPlay)
	if err := b.execCommand("loadfile", path, "replace"); err != nil {
		b.setState(StateError)
		if b.cb.OnError != nil {
			b.cb.OnError(0, err.Error())
		}
		return err
	}
	return nil
}

func (b *mpvBackend) Play() error {
	if err := b.setBoolProp("pause", false); err != nil {
		return err
	}
	b.setState(StatePlaying)
	return nil
}

func (b *mpvBackend) Pause() error {
	if err := b.setBoolProp("pause", true); err != nil {
		return err
	}
	b.setState(StatePaused)
	return nil
}

func (b *mpvBackend) Stop() error {
	if err := b.execCommand("stop"); err != nil {
		return err
	}
	b.setState(StateStopped)
	return nil
}

func (b *mpvBackend) Seek(pos time.Duration) error {
	if pos < 0 {
		pos = 0
	}
	return b.execCommand("seek", fmt.Sprintf("%.3f", pos.Seconds()), "absolute")
}

func (b *mpvBackend) FrameStep() error {
	if err := b.execCommand("frame-step"); err != nil {
		return err
	}
	b.setState(StatePaused)
	return nil
}

func (b *mpvBackend) SetVolume(vol float32) error {
	if vol < 0 {
		vol = 0
	}
	if vol > 1 {
		vol = 1
	}
	return b.setDoubleProp("volume", float64(vol)*100)
}

func (b *mpvBackend) SetMute(mute bool) error {
	return b.setBoolProp("mute", mute)
}

func (b *mpvBackend) SetRate(rate float32) error {
	if rate <= 0 {
		rate = 1
	}
	return b.setDoubleProp("speed", float64(rate))
}

func (b *mpvBackend) Position() time.Duration {
	if pos, ok := b.getDoubleProp("time-pos"); ok && pos >= 0 && !math.IsNaN(pos) && !math.IsInf(pos, 0) {
		return time.Duration(pos * float64(time.Second))
	}
	return 0
}

func (b *mpvBackend) Duration() time.Duration {
	b.mu.Lock()
	d := b.duration
	b.mu.Unlock()
	if d > 0 {
		return d
	}
	d = b.queryDuration()
	if d > 0 {
		b.mu.Lock()
		b.duration = d
		b.mu.Unlock()
	}
	return d
}

func (b *mpvBackend) NativeSize() (int, int) {
	b.mu.Lock()
	w, h := b.natW, b.natH
	b.mu.Unlock()
	if w > 0 && h > 0 {
		return w, h
	}
	nw, nh := b.querySize()
	if nw > 0 && nh > 0 {
		b.mu.Lock()
		b.natW, b.natH = nw, nh
		b.mu.Unlock()
	}
	return nw, nh
}

func (b *mpvBackend) State() PlaybackState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *mpvBackend) Resize(_, _ int) {}
func (b *mpvBackend) Repaint()         {}

func (b *mpvBackend) Close() {
	if b.closed.Swap(true) {
		return
	}
	b.mu.Lock()
	ctx := b.ctx
	dll := b.dll
	wakeupFn := b.procs.wakeup
	destroyFn := b.procs.terminateDestroy
	b.ctx = 0
	b.dll = 0
	b.state = StateEmpty
	b.mu.Unlock()

	if ctx != 0 {
		if wakeupFn != 0 {
			_, _, _ = syscall.SyscallN(wakeupFn, ctx)
		}
		<-b.doneCh
		_, _, _ = syscall.SyscallN(destroyFn, ctx)
	}
	if dll != 0 {
		_ = windows.FreeLibrary(dll)
	}
}
