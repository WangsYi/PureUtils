package platform

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
)

// HotkeyResult is one recorder event.
type HotkeyResult struct {
	Gesture  core.Gesture
	Err      string
	Canceled bool
}

// HotkeyRecorder installs a low-level keyboard hook on its own thread and
// captures one gesture.
type HotkeyRecorder struct {
	results chan HotkeyResult
	done    chan struct{}
	thread  uint32
	stop    sync.Once
	ready   chan error

	stateMu           sync.Mutex
	pressed           map[uint32]bool
	draining          bool
	skipAsyncFallback bool
}

var (
	hotkeyHookCallback uintptr
	activeRecorder     *HotkeyRecorder
	activeMu           sync.Mutex
)

func init() {
	hotkeyHookCallback = syscall.NewCallback(func(nCode int32, wParam, lParam uintptr) uintptr {
		activeMu.Lock()
		r := activeRecorder
		activeMu.Unlock()
		if nCode < 0 || r == nil {
			v, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
			return v
		}
		return r.hook(wParam, lParam)
	})
}

// StartHotkeyRecorder starts capturing a global hotkey gesture.
func StartHotkeyRecorder() (*HotkeyRecorder, error) {
	r := &HotkeyRecorder{
		results: make(chan HotkeyResult, 4),
		done:    make(chan struct{}),
		ready:   make(chan error, 1),
		pressed: map[uint32]bool{},
	}
	activeMu.Lock()
	if activeRecorder != nil {
		activeMu.Unlock()
		return nil, fmt.Errorf("hotkey recorder is already running")
	}
	activeRecorder = r
	activeMu.Unlock()
	go r.run()
	if err := <-r.ready; err != nil {
		activeMu.Lock()
		activeRecorder = nil
		activeMu.Unlock()
		return nil, err
	}
	return r, nil
}

func (r *HotkeyRecorder) Results() <-chan HotkeyResult { return r.results }
func (r *HotkeyRecorder) Done() <-chan struct{}       { return r.done }

func (r *HotkeyRecorder) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() {
		activeMu.Lock()
		if activeRecorder == r {
			activeRecorder = nil
		}
		activeMu.Unlock()
		close(r.done)
		close(r.results)
	}()

	tid, _, _ := procGetCurrentThreadID.Call()
	r.thread = uint32(tid)

	// Ensure this thread's message queue exists before signalling readiness so
	// an immediate Close() via PostThreadMessageW(WM_QUIT) is never dropped.
	var msg Msg
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)

	hMod, _, _ := procGetModuleHandleW.Call(0)
	hook, _, err := procSetWindowsHookExW.Call(WH_KEYBOARD_LL, hotkeyHookCallback, hMod, 0)
	if hook == 0 {
		r.ready <- fmt.Errorf("SetWindowsHookEx(WH_KEYBOARD_LL): %w", err)
		return
	}
	defer procUnhookWindowsHookEx.Call(hook)

	r.ready <- nil

	timer := time.AfterFunc(60*time.Second, func() {
		r.postQuit()
	})
	defer timer.Stop()

	for {
		rc, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(rc) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (r *HotkeyRecorder) postQuit() {
	if tid := r.thread; tid != 0 {
		procPostThreadMessageW.Call(uintptr(tid), WM_QUIT, 0, 0)
	}
}

func (r *HotkeyRecorder) hook(wParam, lParam uintptr) uintptr {
	ks := (*KbdLlHookStruct)(unsafe.Pointer(lParam))
	if ks.Flags&LLKHF_INJECTED != 0 {
		v, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
		return v
	}
	return r.handleKeyEvent(uint32(wParam), ks.VKCode, ks.Flags)
}

func (r *HotkeyRecorder) handleKeyEvent(msg uint32, vk uint32, flags uint32) uintptr {
	down := msg == WM_KEYDOWN || msg == WM_SYSKEYDOWN

	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if down {
		r.pressed[vk] = true
	} else {
		delete(r.pressed, vk)
	}
	if r.draining {
		if len(r.pressed) == 0 {
			r.postQuit()
		}
		return 1
	}
	if !down {
		return 1
	}
	if core.IsModifierKey(uint16(vk)) {
		return 1
	}
	mods := r.activeModifiersLocked(flags)
	if uint16(vk) == VK_ESCAPE && mods == 0 {
		r.send(HotkeyResult{Canceled: true})
		r.draining = true
		r.startDrainTimer()
		return 1
	}
	key, ok := core.KeyFromVK(uint16(vk))
	if !ok {
		r.send(HotkeyResult{Err: i18n.F("不支持的按键（VK 0x%02X）", "Unsupported key (VK 0x%02X)", vk)})
		return 1
	}
	g, err := core.HotkeyCapture(mods, key)
	if err != nil {
		r.send(HotkeyResult{Err: err.Error()})
		return 1
	}
	r.send(HotkeyResult{Gesture: g})
	r.draining = true
	r.startDrainTimer()
	return 1
}

func (r *HotkeyRecorder) activeModifiersLocked(flags uint32) core.Modifiers {
	var m core.Modifiers
	if r.pressed[VK_CONTROL] || r.pressed[VK_LCONTROL] || r.pressed[VK_RCONTROL] {
		m |= core.ModControl
	}
	if r.pressed[VK_MENU] || r.pressed[VK_LMENU] || r.pressed[VK_RMENU] || flags&LLKHF_ALTDOWN != 0 {
		m |= core.ModAlt
	}
	if r.pressed[VK_SHIFT] || r.pressed[VK_LSHIFT] || r.pressed[VK_RSHIFT] {
		m |= core.ModShift
	}
	if r.pressed[VK_LWIN] || r.pressed[VK_RWIN] {
		m |= core.ModWin
	}
	if !r.skipAsyncFallback {
		m |= currentModifiers()
	}
	return m
}

func (r *HotkeyRecorder) send(res HotkeyResult) {
	select {
	case r.results <- res:
	default:
	}
}

func (r *HotkeyRecorder) startDrainTimer() {
	if r.thread == 0 {
		return
	}
	time.AfterFunc(3*time.Second, func() {
		r.postQuit()
	})
}

func (r *HotkeyRecorder) Close() {
	if r == nil {
		return
	}
	r.stop.Do(func() {
		r.postQuit()
	})
	<-r.done
}

func currentModifiers() core.Modifiers {
	var m core.Modifiers
	if keyDown(VK_CONTROL) {
		m |= core.ModControl
	}
	if keyDown(VK_MENU) {
		m |= core.ModAlt
	}
	if keyDown(VK_SHIFT) {
		m |= core.ModShift
	}
	if keyDown(VK_LWIN) || keyDown(VK_RWIN) {
		m |= core.ModWin
	}
	return m
}

func keyDown(vk uintptr) bool {
	r, _, _ := procGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}
