package platform

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// MessageWindow is a hidden message-only window with its own message loop.
type MessageWindow struct {
	HWND      HWND
	thread    uint32
	ready     chan error
	done      chan struct{}
	closeOnce sync.Once

	mu       sync.Mutex
	handlers map[uint32]func(wParam, lParam uintptr) uintptr
}

var (
	messageWindows  sync.Map
	wndProcCallback uintptr
	classMu         sync.Mutex
	registered      = map[string]bool{}
)

func init() {
	wndProcCallback = syscall.NewCallback(func(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
		if v, ok := messageWindows.Load(hwnd); ok {
			return v.(*MessageWindow).handle(msg, wparam, lparam)
		}
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
		return r
	})
}

// NewMessageWindow creates and starts a message-only window.
func NewMessageWindow(name string) (*MessageWindow, error) {
	w := &MessageWindow{
		ready:    make(chan error, 1),
		done:     make(chan struct{}),
		handlers: map[uint32]func(wParam, lParam uintptr) uintptr{},
	}
	go w.run(name)
	if err := <-w.ready; err != nil {
		return nil, err
	}
	return w, nil
}

func (w *MessageWindow) run(name string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	className := "PureUtilsMsg_" + name
	if err := registerClass(className); err != nil {
		w.ready <- err
		return
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(Utf16Ptr(className))),
		uintptr(unsafe.Pointer(Utf16Ptr(""))), 0, 0, 0, 0, 0,
		uintptr(HWND_MESSAGE), 0, hInstance, 0)
	if hwnd == 0 {
		w.ready <- fmt.Errorf("CreateWindowEx(%s): %w", className, err)
		return
	}
	w.HWND = HWND(hwnd)
	tid, _, _ := procGetCurrentThreadID.Call()
	w.thread = uint32(tid)
	messageWindows.Store(hwnd, w)
	w.ready <- nil

	var msg Msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	messageWindows.Delete(hwnd)
	close(w.done)
}

func registerClass(name string) error {
	classMu.Lock()
	defer classMu.Unlock()
	if registered[name] {
		return nil
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	wc := WndClassEx{
		CbSize:        uint32(unsafe.Sizeof(WndClassEx{})),
		LpfnWndProc:   wndProcCallback,
		HInstance:     HINSTANCE(hInstance),
		LpszClassName: Utf16Ptr(name),
	}
	r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		if errno, ok := err.(syscall.Errno); ok && errno == 1410 {
			registered[name] = true
			return nil
		}
		return fmt.Errorf("RegisterClassEx(%s): %w", name, err)
	}
	registered[name] = true
	return nil
}

func (w *MessageWindow) handle(msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CLOSE:
		procDestroyWindow.Call(uintptr(w.HWND))
		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}
	w.mu.Lock()
	fn := w.handlers[msg]
	w.mu.Unlock()
	if fn != nil {
		return fn(wParam, lParam)
	}
	r, _, _ := procDefWindowProcW.Call(uintptr(w.HWND), uintptr(msg), wParam, lParam)
	return r
}

func (w *MessageWindow) OnMessage(msg uint32, fn func(wParam, lParam uintptr) uintptr) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[msg] = fn
}

func (w *MessageWindow) Post(msg uint32, wParam, lParam uintptr) bool {
	r, _, _ := procPostMessageW.Call(uintptr(w.HWND), uintptr(msg), wParam, lParam)
	return r != 0
}

func (w *MessageWindow) Close() {
	if w == nil || w.HWND == 0 {
		return
	}
	w.closeOnce.Do(func() {
		w.Post(WM_CLOSE, 0, 0)
	})
	<-w.done
}
