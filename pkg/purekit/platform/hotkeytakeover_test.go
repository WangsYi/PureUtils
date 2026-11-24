package platform

import (
	"testing"

	"pureutils/pkg/purekit/core"
)

func TestHotkeyTakeoverMatchesAndMasksStartMenu(t *testing.T) {
	var fired []string
	var maskCount int

	to := NewHotkeyTakeover(nil)
	to.skipAsyncSync = true
	to.maskFn = func() { maskCount++ }
	to.dispatchFn = func(fn func()) { fn() }

	bindings := []HotkeyBinding{
		{
			ID:       "pureclip.history",
			Gesture:  core.Gesture{Modifiers: core.ModWin, Key: "V"},
			Callback: func() { fired = append(fired, "Win+V") },
		},
		{
			ID:       "pureshot.region",
			Gesture:  core.Gesture{Modifiers: core.ModWin | core.ModShift, Key: "S"},
			Callback: func() { fired = append(fired, "Win+Shift+S") },
		},
		{
			ID:       "host.launcher",
			Gesture:  core.Gesture{Modifiers: core.ModWin, Key: "SPACE"},
			Callback: func() { fired = append(fired, "Win+Space") },
		},
	}

	next := make(map[takeoverKey]HotkeyBinding, len(bindings))
	for _, b := range bindings {
		vk, ok := core.VirtualKey(b.Gesture.Key)
		if !ok {
			t.Fatalf("missing vk for %+v", b.Gesture)
		}
		next[takeoverKey{mods: b.Gesture.Modifiers, vk: uint32(vk)}] = b
	}
	to.bindings = next

	// 1. Win+V triggers once, suppresses auto-repeat, swallows matching keyup, and leaves Win modifier unswallowed.
	if r := to.handleKeyEvent(WM_KEYDOWN, VK_LWIN, 0); r != 0 {
		t.Fatalf("VK_LWIN down return = %d, want 0", r)
	}
	if r := to.handleKeyEvent(WM_KEYDOWN, 'V', 0); r != 1 {
		t.Fatalf("Win+V down return = %d, want 1", r)
	}
	if r := to.handleKeyEvent(WM_KEYDOWN, 'V', 0); r != 1 {
		t.Fatalf("Win+V repeat return = %d, want 1", r)
	}
	if len(fired) != 1 || fired[0] != "Win+V" || maskCount != 1 {
		t.Fatalf("after Win+V repeat: fired=%v maskCount=%d", fired, maskCount)
	}
	if r := to.handleKeyEvent(WM_KEYUP, 'V', 0); r != 1 {
		t.Fatalf("Win+V keyup return = %d, want 1", r)
	}
	if r := to.handleKeyEvent(WM_KEYUP, VK_LWIN, 0); r != 0 {
		t.Fatalf("VK_LWIN up return = %d, want 0", r)
	}

	// 2. Win+Shift+S triggers and suppresses.
	if r := to.handleKeyEvent(WM_KEYDOWN, VK_LWIN, 0); r != 0 {
		t.Fatalf("VK_LWIN down = %d", r)
	}
	if r := to.handleKeyEvent(WM_KEYDOWN, VK_LSHIFT, 0); r != 0 {
		t.Fatalf("VK_LSHIFT down = %d", r)
	}
	if r := to.handleKeyEvent(WM_KEYDOWN, 'S', 0); r != 1 {
		t.Fatalf("Win+Shift+S down = %d, want 1", r)
	}
	if len(fired) != 2 || fired[1] != "Win+Shift+S" || maskCount != 2 {
		t.Fatalf("after Win+Shift+S: fired=%v maskCount=%d", fired, maskCount)
	}
	to.handleKeyEvent(WM_KEYUP, 'S', 0)
	to.handleKeyEvent(WM_KEYUP, VK_LSHIFT, 0)
	to.handleKeyEvent(WM_KEYUP, VK_LWIN, 0)

	// 3. Right Win + Space triggers Win+Space.
	if r := to.handleKeyEvent(WM_KEYDOWN, VK_RWIN, 0); r != 0 {
		t.Fatalf("VK_RWIN down = %d", r)
	}
	if r := to.handleKeyEvent(WM_KEYDOWN, VK_SPACE, 0); r != 1 {
		t.Fatalf("Win+Space down = %d, want 1", r)
	}
	if len(fired) != 3 || fired[2] != "Win+Space" || maskCount != 3 {
		t.Fatalf("after Win+Space: fired=%v maskCount=%d", fired, maskCount)
	}
	to.handleKeyEvent(WM_KEYUP, VK_SPACE, 0)
	to.handleKeyEvent(WM_KEYUP, VK_RWIN, 0)

	// 4. Unbound Win+D passes through untouched without masking.
	if r := to.handleKeyEvent(WM_KEYDOWN, VK_LWIN, 0); r != 0 {
		t.Fatalf("VK_LWIN down = %d", r)
	}
	if r := to.handleKeyEvent(WM_KEYDOWN, 'D', 0); r != 0 {
		t.Fatalf("unbound Win+D down = %d, want 0", r)
	}
	if r := to.handleKeyEvent(WM_KEYUP, 'D', 0); r != 0 {
		t.Fatalf("unbound Win+D up = %d, want 0", r)
	}
	to.handleKeyEvent(WM_KEYUP, VK_LWIN, 0)
	if len(fired) != 3 || maskCount != 3 {
		t.Fatalf("unbound Win+D changed state: fired=%v maskCount=%d", fired, maskCount)
	}

	// 5. Missed keyup recovery: release LWIN without V keyup, then press plain V -> must clear stale state and pass through.
	to.handleKeyEvent(WM_KEYDOWN, VK_LWIN, 0)
	if r := to.handleKeyEvent(WM_KEYDOWN, 'V', 0); r != 1 {
		t.Fatalf("second Win+V down = %d, want 1", r)
	}
	to.handleKeyEvent(WM_KEYUP, VK_LWIN, 0)
	if r := to.handleKeyEvent(WM_KEYDOWN, 'V', 0); r != 0 {
		t.Fatalf("plain V after missed keyup returned %d, want 0", r)
	}
	if r := to.handleKeyEvent(WM_KEYUP, 'V', 0); r != 0 {
		t.Fatalf("plain V keyup returned %d, want 0", r)
	}
	if len(fired) != 4 || maskCount != 4 {
		t.Fatalf("missed-keyup recovery unexpected count: fired=%v maskCount=%d", fired, maskCount)
	}
}

func TestHotkeyTakeoverHonorsShouldIgnore(t *testing.T) {
	fired := 0
	maskCount := 0
	ignored := true

	to := NewHotkeyTakeover(func() bool { return ignored })
	to.skipAsyncSync = true
	to.maskFn = func() { maskCount++ }
	to.dispatchFn = func(fn func()) { fn() }

	vkV, _ := core.VirtualKey("V")
	to.bindings[takeoverKey{mods: core.ModWin, vk: uint32(vkV)}] = HotkeyBinding{
		ID:       "pureclip.history",
		Gesture:  core.Gesture{Modifiers: core.ModWin, Key: "V"},
		Callback: func() { fired++ },
	}

	if r := to.handleKeyEvent(WM_KEYDOWN, VK_LWIN, 0); r != 0 {
		t.Fatalf("VK_LWIN down = %d, want 0", r)
	}
	if r := to.handleKeyEvent(WM_KEYDOWN, 'V', 0); r != 0 {
		t.Fatalf("ignored Win+V down = %d, want 0", r)
	}
	if fired != 0 || maskCount != 0 {
		t.Fatalf("ignored hotkey still fired (%d) or masked (%d)", fired, maskCount)
	}
}
