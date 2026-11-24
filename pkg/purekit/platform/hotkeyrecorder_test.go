package platform

import (
	"syscall"
	"testing"
	"time"

	"pureutils/pkg/purekit/core"
)

func TestHotkeyRecorderInjectedKeys(t *testing.T) {
	rec, err := StartHotkeyRecorder()
	if err != nil {
		t.Skipf("hotkey recorder unavailable: %v", err)
	}
	defer rec.Close()

	keybd := syscall.NewLazyDLL("user32.dll").NewProc("keybd_event")
	key := func(vk uintptr, up bool) {
		flags := uintptr(0)
		if up {
			flags = 2
		}
		keybd.Call(vk, 0, flags, 0)
	}
	time.Sleep(150 * time.Millisecond)
	key(0x11, false)
	key(0x10, false)
	key('K', false)
	key('K', true)
	key(0x10, true)
	key(0x11, true)

	select {
	case result := <-rec.Results():
		if result.Canceled {
			t.Skip("recorder was canceled")
		}
		want := core.Gesture{Modifiers: core.ModControl | core.ModShift, Key: "K"}
		if result.Gesture != want {
			t.Errorf("gesture = %+v, want %+v", result.Gesture, want)
		}
	case <-time.After(3 * time.Second):
		t.Skip("injected keys were not observed in this session")
	}
}

func TestHotkeyRecorderModifierTracking(t *testing.T) {
	newRec := func() *HotkeyRecorder {
		return &HotkeyRecorder{
			results:           make(chan HotkeyResult, 8),
			done:              make(chan struct{}),
			pressed:           map[uint32]bool{},
			skipAsyncFallback: true,
		}
	}

	// 1. Ctrl+K tracked from swallowed VK_LCONTROL down
	r := newRec()
	r.handleKeyEvent(WM_KEYDOWN, VK_LCONTROL, 0)
	r.handleKeyEvent(WM_KEYDOWN, 'K', 0)
	r.handleKeyEvent(WM_KEYUP, 'K', 0)
	r.handleKeyEvent(WM_KEYUP, VK_LCONTROL, 0)
	res := <-r.Results()
	if res.Err != "" || res.Canceled || res.Gesture != (core.Gesture{Modifiers: core.ModControl, Key: "K"}) {
		t.Fatalf("Ctrl+K result = %+v", res)
	}

	// 2. Alt+Space via WM_SYSKEYDOWN + LLKHF_ALTDOWN
	r = newRec()
	r.handleKeyEvent(WM_SYSKEYDOWN, VK_LMENU, LLKHF_ALTDOWN)
	r.handleKeyEvent(WM_SYSKEYDOWN, VK_SPACE, LLKHF_ALTDOWN)
	res = <-r.Results()
	if res.Gesture != (core.Gesture{Modifiers: core.ModAlt, Key: "SPACE"}) {
		t.Fatalf("Alt+Space result = %+v", res)
	}

	// 3. Ctrl+Shift+V
	r = newRec()
	r.handleKeyEvent(WM_KEYDOWN, VK_LCONTROL, 0)
	r.handleKeyEvent(WM_KEYDOWN, VK_LSHIFT, 0)
	r.handleKeyEvent(WM_KEYDOWN, 'V', 0)
	res = <-r.Results()
	if res.Gesture != (core.Gesture{Modifiers: core.ModControl | core.ModShift, Key: "V"}) {
		t.Fatalf("Ctrl+Shift+V result = %+v", res)
	}

	// 4. Win+E
	r = newRec()
	r.handleKeyEvent(WM_KEYDOWN, VK_LWIN, 0)
	r.handleKeyEvent(WM_KEYDOWN, 'E', 0)
	res = <-r.Results()
	if res.Gesture != (core.Gesture{Modifiers: core.ModWin, Key: "E"}) {
		t.Fatalf("Win+E result = %+v", res)
	}

	// 5. Bare 'A' emits error without stopping recording, followed by Alt+Q
	r = newRec()
	r.handleKeyEvent(WM_KEYDOWN, 'A', 0)
	r.handleKeyEvent(WM_KEYUP, 'A', 0)
	errRes := <-r.Results()
	if errRes.Err == "" {
		t.Fatalf("expected error on bare A, got %+v", errRes)
	}
	if r.draining {
		t.Fatal("error result should keep recorder active, not draining")
	}
	r.handleKeyEvent(WM_SYSKEYDOWN, VK_LMENU, LLKHF_ALTDOWN)
	r.handleKeyEvent(WM_SYSKEYDOWN, 'Q', LLKHF_ALTDOWN)
	okRes := <-r.Results()
	if okRes.Gesture != (core.Gesture{Modifiers: core.ModAlt, Key: "Q"}) {
		t.Fatalf("Alt+Q after error = %+v", okRes)
	}

	// 6. Bare F2 accepted, bare Esc cancels
	r = newRec()
	r.handleKeyEvent(WM_KEYDOWN, 0x71, 0) // VK_F2
	if got := <-r.Results(); got.Gesture != (core.Gesture{Modifiers: core.ModNone, Key: "F2"}) {
		t.Fatalf("F2 result = %+v", got)
	}

	r = newRec()
	r.handleKeyEvent(WM_KEYDOWN, VK_ESCAPE, 0)
	if got := <-r.Results(); !got.Canceled {
		t.Fatalf("Esc result = %+v, want Canceled=true", got)
	}
}
