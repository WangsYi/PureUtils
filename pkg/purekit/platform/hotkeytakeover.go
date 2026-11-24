package platform

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"pureutils/pkg/purekit/core"
)

// vkMenuMask is an unassigned virtual key injected via SendInput when a bound
// Win+... chord triggers. Windows suppresses the Start Menu pop-up on Win key
// release whenever another key event occurred while Win was held.
const vkMenuMask uint16 = 0xE8

// HotkeyBinding describes a global shortcut handled by HotkeyTakeover.
type HotkeyBinding struct {
	ID       string
	Gesture  core.Gesture
	Callback func()
}

type takeoverKey struct {
	mods core.Modifiers
	vk   uint32
}

// HotkeyTakeover installs a lazy low-level WH_KEYBOARD_LL hook on its own
// OS thread when one or more system-reserved Win+... bindings are active,
// suppressing matching key events and Start Menu activation.
type HotkeyTakeover struct {
	shouldIgnore func() bool

	opMu sync.Mutex

	mu            sync.Mutex
	bindings      map[takeoverKey]HotkeyBinding
	pressed       map[uint32]bool
	swallowed     map[uint32]bool
	maskFn        func()
	dispatchFn    func(func())
	skipAsyncSync bool

	running bool
	thread  uint32
	done    chan struct{}
	ready   chan error
}

var (
	takeoverHookCallback uintptr
	activeTakeover       *HotkeyTakeover
	activeTakeoverMu     sync.Mutex
)

func init() {
	takeoverHookCallback = syscall.NewCallback(func(nCode int32, wParam, lParam uintptr) uintptr {
		activeTakeoverMu.Lock()
		t := activeTakeover
		activeTakeoverMu.Unlock()
		if nCode < 0 || t == nil {
			v, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
			return v
		}
		return t.hook(wParam, lParam)
	})
}

// NewHotkeyTakeover creates a takeover manager. No OS hook or background thread
// is started until SetBindings receives at least one valid binding.
func NewHotkeyTakeover(shouldIgnore func() bool) *HotkeyTakeover {
	return &HotkeyTakeover{
		shouldIgnore: shouldIgnore,
		bindings:     map[takeoverKey]HotkeyBinding{},
		pressed:      map[uint32]bool{},
		swallowed:    map[uint32]bool{},
		maskFn:       sendMenuMaskKey,
		dispatchFn:   func(fn func()) { go fn() },
	}
}

// SetBindings replaces the active takeover bindings, starting or stopping the
// low-level keyboard hook thread on demand.
func (t *HotkeyTakeover) SetBindings(bindings []HotkeyBinding) error {
	if t == nil {
		return nil
	}
	next := make(map[takeoverKey]HotkeyBinding, len(bindings))
	for _, b := range bindings {
		if b.Gesture.Empty() || b.Callback == nil {
			continue
		}
		vk, ok := core.VirtualKey(b.Gesture.Key)
		if !ok {
			continue
		}
		next[takeoverKey{mods: b.Gesture.Modifiers, vk: uint32(vk)}] = b
	}

	t.opMu.Lock()
	defer t.opMu.Unlock()

	t.mu.Lock()
	t.bindings = next
	t.pressed = map[uint32]bool{}
	t.swallowed = map[uint32]bool{}
	t.mu.Unlock()

	if len(next) == 0 {
		t.stopLocked()
		return nil
	}
	return t.startLocked()
}

// Close clears bindings and uninstalls any active hook cleanly.
func (t *HotkeyTakeover) Close() {
	if t == nil {
		return
	}
	t.opMu.Lock()
	defer t.opMu.Unlock()

	t.mu.Lock()
	t.bindings = map[takeoverKey]HotkeyBinding{}
	t.pressed = map[uint32]bool{}
	t.swallowed = map[uint32]bool{}
	t.mu.Unlock()

	t.stopLocked()
}

func (t *HotkeyTakeover) startLocked() error {
	if t.running {
		return nil
	}
	done := make(chan struct{})
	ready := make(chan error, 1)
	t.done = done
	t.ready = ready
	t.running = true

	go t.run(done, ready)
	if err := <-ready; err != nil {
		t.running = false
		t.thread = 0
		t.done = nil
		t.ready = nil
		return err
	}
	return nil
}

func (t *HotkeyTakeover) stopLocked() {
	if !t.running {
		return
	}
	tid := t.thread
	done := t.done
	if tid != 0 {
		procPostThreadMessageW.Call(uintptr(tid), WM_QUIT, 0, 0)
	}
	if done != nil {
		<-done
	}
	t.running = false
	t.thread = 0
	t.done = nil
	t.ready = nil
}

func (t *HotkeyTakeover) run(done chan struct{}, ready chan error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := procGetCurrentThreadID.Call()
	t.thread = uint32(tid)

	// Materialize the thread message queue before posting WM_QUIT can happen.
	var msg Msg
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)

	activeTakeoverMu.Lock()
	activeTakeover = t
	activeTakeoverMu.Unlock()

	hMod, _, _ := procGetModuleHandleW.Call(0)
	hook, _, err := procSetWindowsHookExW.Call(WH_KEYBOARD_LL, takeoverHookCallback, hMod, 0)
	if hook == 0 {
		activeTakeoverMu.Lock()
		if activeTakeover == t {
			activeTakeover = nil
		}
		activeTakeoverMu.Unlock()
		ready <- fmt.Errorf("SetWindowsHookEx(WH_KEYBOARD_LL): %w", err)
		close(done)
		return
	}
	defer func() {
		procUnhookWindowsHookEx.Call(hook)
		activeTakeoverMu.Lock()
		if activeTakeover == t {
			activeTakeover = nil
		}
		activeTakeoverMu.Unlock()
		close(done)
	}()

	ready <- nil

	for {
		rc, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(rc) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (t *HotkeyTakeover) hook(wParam, lParam uintptr) uintptr {
	ks := (*KbdLlHookStruct)(unsafe.Pointer(lParam))
	if ks.Flags&LLKHF_INJECTED != 0 {
		v, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
		return v
	}
	if t.handleKeyEvent(uint32(wParam), ks.VKCode, ks.Flags) != 0 {
		return 1
	}
	v, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
	return v
}

func (t *HotkeyTakeover) handleKeyEvent(msg uint32, vk uint32, flags uint32) uintptr {
	var down bool
	switch msg {
	case WM_KEYDOWN, WM_SYSKEYDOWN:
		down = true
	case WM_KEYUP, WM_SYSKEYUP:
		down = false
	default:
		return 0
	}

	if !down {
		t.mu.Lock()
		delete(t.pressed, vk)
		swallowed := t.swallowed[vk]
		if swallowed {
			delete(t.swallowed, vk)
		}
		t.mu.Unlock()
		if swallowed {
			return 1
		}
		return 0
	}

	t.mu.Lock()
	t.pressed[vk] = true
	if core.IsModifierKey(uint16(vk)) {
		t.mu.Unlock()
		return 0
	}
	ignoreFn := t.shouldIgnore
	t.mu.Unlock()

	if ignoreFn != nil && ignoreFn() {
		return 0
	}

	t.mu.Lock()
	mods := t.activeModifiersLocked(flags)
	b, ok := t.bindings[takeoverKey{mods: mods, vk: vk}]
	if !ok {
		delete(t.swallowed, vk)
		t.mu.Unlock()
		return 0
	}
	if t.swallowed[vk] {
		t.mu.Unlock()
		return 1
	}
	t.swallowed[vk] = true
	mask := t.maskFn
	dispatch := t.dispatchFn
	cb := b.Callback
	t.mu.Unlock()

	if mask != nil {
		mask()
	}
	if cb != nil {
		if dispatch != nil {
			dispatch(cb)
		} else {
			go cb()
		}
	}
	return 1
}

func (t *HotkeyTakeover) activeModifiersLocked(flags uint32) core.Modifiers {
	if !t.skipAsyncSync {
		mods := currentModifiers()
		if flags&LLKHF_ALTDOWN != 0 {
			mods |= core.ModAlt
		}
		return mods
	}
	var m core.Modifiers
	if t.pressed[VK_CONTROL] || t.pressed[VK_LCONTROL] || t.pressed[VK_RCONTROL] {
		m |= core.ModControl
	}
	if t.pressed[VK_MENU] || t.pressed[VK_LMENU] || t.pressed[VK_RMENU] || flags&LLKHF_ALTDOWN != 0 {
		m |= core.ModAlt
	}
	if t.pressed[VK_SHIFT] || t.pressed[VK_LSHIFT] || t.pressed[VK_RSHIFT] {
		m |= core.ModShift
	}
	if t.pressed[VK_LWIN] || t.pressed[VK_RWIN] {
		m |= core.ModWin
	}
	return m
}

func sendMenuMaskKey() {
	inputs := []Input{
		keyInput(vkMenuMask, 0, 0),
		keyInput(vkMenuMask, 0, KEYEVENTF_KEYUP),
	}
	procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
}
