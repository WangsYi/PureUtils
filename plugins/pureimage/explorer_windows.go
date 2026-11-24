//go:build windows

package pureimage

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

const (
	whKeyboardLL = 13
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	vkSpace      = 0x20
	vkEscape     = 0x1B
	vkShift      = 0x10
	vkMenu       = 0x12
	vkLWin       = 0x5B
	vkRWin       = 0x5C
	vkLeft       = 0x25
	vkUp         = 0x26
	vkRight      = 0x27
	vkDown       = 0x28
)

type kbdllhookstruct struct {
	vkCode      uint32
	scanCode    uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

type guiThreadInfo struct {
	cbSize        uint32
	flags         uint32
	hwndActive    uintptr
	hwndFocus     uintptr
	hwndCapture   uintptr
	hwndMenuOwner uintptr
	hwndMoveSize  uintptr
	hwndCaret     uintptr
	rcCaret       winRect
}

var (
	procGetForegroundWindow     = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcess  = user32.NewProc("GetWindowThreadProcessId")
	procGetGUIThreadInfo        = user32.NewProc("GetGUIThreadInfo")
	procGetClassName            = user32.NewProc("GetClassNameW")
	procGetSystemMetrics        = user32.NewProc("GetSystemMetrics")
	procGetAsyncKeyState        = user32.NewProc("GetAsyncKeyState")

	activeSpaceMgr    atomic.Pointer[spaceHookManager]
	spaceCallbackOnce sync.Once
	spaceHookCB       uintptr
)

func initSpaceHookCallback() {
	spaceCallbackOnce.Do(func() {
		spaceHookCB = syscall.NewCallback(func(nCode, wParam, lParam uintptr) uintptr {
			mgr := activeSpaceMgr.Load()
			if mgr == nil || nCode != 0 {
				ret, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
				return ret
			}
			return mgr.handleHook(wParam, lParam)
		})
	})
}

type spaceHookManager struct {
	mu      sync.Mutex
	hook    uintptr
	enabled atomic.Bool

	spaceDown atomic.Bool
	spaceOpen atomic.Bool

	active  atomic.Bool
	curPath string
	bmp     *ui.Bitmap
	srcW    int
	srcH    int
	win     *mygo.Window

	seq atomic.Int64

	shouldIgnore func() bool
	headless     bool
	logf         func(string, ...any)

	staOnce sync.Once
	staReq  chan chan string
	staStop chan struct{}
	closed  atomic.Bool
}

func newSpaceHookManager(headless bool, shouldIgnore func() bool, logf func(string, ...any)) *spaceHookManager {
	return &spaceHookManager{
		headless:     headless,
		shouldIgnore: shouldIgnore,
		logf:         logf,
	}
}

func (m *spaceHookManager) ensureSTAWorker() {
	m.staOnce.Do(func() {
		reqCh := make(chan chan string, 4)
		stopCh := make(chan struct{})
		m.staReq = reqCh
		m.staStop = stopCh
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			_ = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
			defer ole.CoUninitialize()
			for {
				select {
				case <-stopCh:
					return
				case reply := <-reqCh:
					reply <- explorerSelectedPathCOM()
				}
			}
		}()
	})
}

// Install attaches WH_KEYBOARD_LL and starts the STA Explorer lookup worker.
func (m *spaceHookManager) Install() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed.Load() {
		return
	}
	m.enabled.Store(true)
	if m.headless || m.hook != 0 {
		return
	}
	m.ensureSTAWorker()
	initSpaceHookCallback()
	activeSpaceMgr.Store(m)
	h, _, err := procSetWindowsHookEx.Call(whKeyboardLL, spaceHookCB, 0, 0)
	if h == 0 {
		if m.logf != nil {
			m.logf("viewer space hook install failed: %v", err)
		}
		return
	}
	m.hook = h
}

// Uninstall removes WH_KEYBOARD_LL and closes any open preview window.
func (m *spaceHookManager) Uninstall() {
	m.mu.Lock()
	m.enabled.Store(false)
	if m.hook != 0 {
		_, _, _ = procUnhookWindowsEx.Call(m.hook)
		m.hook = 0
	}
	if activeSpaceMgr.Load() == m {
		activeSpaceMgr.Store(nil)
	}
	m.mu.Unlock()
	m.spaceOpen.Store(false)
	m.spaceDown.Store(false)
	if !m.headless {
		mygo.RunOnMain(m.Hide)
	} else {
		m.Hide()
	}
}

// Shutdown removes the hook, closes preview resources, and exits the STA COM worker.
func (m *spaceHookManager) Shutdown() {
	m.closed.Store(true)
	m.Uninstall()
	m.mu.Lock()
	stop := m.staStop
	m.staStop = nil
	m.mu.Unlock()
	if stop != nil {
		close(stop)
	}
}

func (m *spaceHookManager) IsEnabled() bool {
	return m.enabled.Load()
}

func (m *spaceHookManager) handleHook(wParam, lParam uintptr) uintptr {
	if !m.enabled.Load() || m.closed.Load() {
		ret, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
		return ret
	}
	kb := (*kbdllhookstruct)(unsafe.Pointer(lParam))
	if kb == nil {
		ret, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
		return ret
	}

	if kb.vkCode == vkEscape && wParam == wmKeyDown && m.spaceOpen.Load() {
		m.spaceOpen.Store(false)
		mygo.RunOnMain(m.Hide)
		return 1
	}

	if m.spaceOpen.Load() && wParam == wmKeyDown &&
		(kb.vkCode == vkLeft || kb.vkCode == vkUp || kb.vkCode == vkRight || kb.vkCode == vkDown) &&
		isExplorerForeground() {
		go m.followArrowNavigation()
		ret, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
		return ret
	}

	if kb.vkCode == vkSpace {
		switch wParam {
		case wmKeyDown:
			if m.shouldIgnore != nil && m.shouldIgnore() {
				ret, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
				return ret
			}
			if hasAnyModifierDown() {
				ret, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
				return ret
			}
			if m.spaceDown.Load() {
				if m.spaceOpen.Load() {
					return 1
				}
				ret, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
				return ret
			}
			m.spaceDown.Store(true)
			if m.spaceOpen.Load() {
				m.spaceOpen.Store(false)
				mygo.RunOnMain(m.Hide)
				return 1
			}
			if isExplorerForeground() && !isExplorerTextInputFocused() {
				if path := m.selectedExplorerPath(); path != "" {
					m.spaceOpen.Store(true)
					mygo.RunOnMain(func() { m.Show(path) })
					return 1
				}
			}
		case wmKeyUp:
			m.spaceDown.Store(false)
			if m.spaceOpen.Load() {
				return 1
			}
		}
	}

	ret, _, _ := procCallNextHookEx.Call(0, 0, wParam, lParam)
	return ret
}

func (m *spaceHookManager) followArrowNavigation() {
	time.Sleep(55 * time.Millisecond)
	if !m.enabled.Load() || !m.spaceOpen.Load() || !isExplorerForeground() {
		return
	}
	path := m.selectedExplorerPath()
	if path == "" || strings.EqualFold(path, m.curPath) {
		return
	}
	mygo.RunOnMain(func() {
		if m.spaceOpen.Load() {
			m.Show(path)
		}
	})
}

func (m *spaceHookManager) selectedExplorerPath() string {
	reqCh := m.staReq
	if reqCh == nil {
		return ""
	}
	reply := make(chan string, 1)
	select {
	case reqCh <- reply:
	case <-time.After(15 * time.Millisecond):
		return ""
	}
	select {
	case p := <-reply:
		return p
	case <-time.After(80 * time.Millisecond):
		if m.logf != nil {
			m.logf("viewer space preview selection lookup timed out")
		}
		return ""
	}
}

func (m *spaceHookManager) Show(path string) {
	if path == "" {
		return
	}
	m.active.Store(true)
	m.curPath = path
	seq := m.seq.Add(1)
	go func() {
		bmp, _, sw, sh, _, _, err := DecodePreview(path, 1400, 0, false)
		if m.headless || m.closed.Load() {
			if bmp != nil {
				bmp.ReleasePixels()
			}
			return
		}
		mygo.RunOnMain(func() {
			if !m.active.Load() || m.seq.Load() != seq || m.curPath != path {
				if bmp != nil {
					bmp.ReleasePixels()
				}
				return
			}
			if err != nil || bmp == nil {
				m.Hide()
				return
			}
			if m.bmp != nil {
				m.bmp.ReleasePixels()
			}
			m.bmp = bmp
			m.srcW = sw
			m.srcH = sh
			m.showWindow()
		})
	}()
}

func (m *spaceHookManager) Hide() {
	m.seq.Add(1)
	m.active.Store(false)
	m.spaceOpen.Store(false)
	m.curPath = ""
	if m.win != nil {
		win := m.win
		m.win = nil
		win.Close()
	}
	if m.bmp != nil {
		m.bmp.ReleasePixels()
		m.bmp = nil
	}
}

func (m *spaceHookManager) showWindow() {
	if m.bmp == nil || m.headless {
		return
	}
	if m.win == nil {
		w, h := 720, 540
		if m.srcW > 0 && m.srcH > 0 {
			if m.srcH > m.srcW {
				w, h = 480, 640
			}
		}
		x, y := spacePreviewPosition(w, h)
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:        "PureImage 预览",
			Width:        w,
			Height:       h,
			X:            x,
			Y:            y,
			Frameless:    true,
			AlwaysOnTop:  true,
			SkipTaskbar:  true,
			CornerRadius: 10,
			Content:      ui.View(m.view),
		})
		_ = win.SetIcon(viewerIconPNG)
		win.OnClosed(func() {
			m.win = nil
			m.spaceOpen.Store(false)
			m.active.Store(false)
		})
		m.win = win
		win.ShowInactive()
	}
	m.win.Invalidate()
}

func (m *spaceHookManager) view(c *ui.Context) {
	c.SetTheme(ui.DarkTheme())
	if c.Shortcut(0, ui.KeyEscape) {
		m.Hide()
		return
	}
	ui.Box(c).Fill().Background(ui.Hex("#16181D")).Border(1, ui.RGBA(255, 255, 255, 0.14)).Radius(10).Clip().Children(func() {
		if m.bmp != nil {
			ui.Image(c, m.bmp).Fit(ui.Contain).Fill()
		}
	})
	if m.curPath != "" {
		ui.Overlay(c, func() {
			ui.Box(c).
				Absolute().Left(12).Top(12).
				Padding(6, 10).
				Background(ui.RGBA(18, 20, 26, 0.84)).
				Border(1, ui.RGBA(255, 255, 255, 0.14)).
				Radius(6).
				Children(func() {
					label := fmt.Sprintf("%s · %d×%d", filepath.Base(m.curPath), m.srcW, m.srcH)
					ui.Text(c, label).FontSize(12).TextColor(ui.RGB(243, 244, 246))
				})
		})
	}
}

func hasAnyModifierDown() bool {
	for _, vk := range []uintptr{vkControl, vkShift, vkMenu, vkLWin, vkRWin} {
		state, _, _ := procGetAsyncKeyState.Call(vk)
		if uint16(state)&0x8000 != 0 {
			return true
		}
	}
	return false
}

func isExplorerForeground() bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	class := windowClassName(hwnd)
	return class == "CabinetWClass" || class == "ExploreWClass"
}

func isExplorerTextInputFocused() bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	tid, _, _ := procGetWindowThreadProcess.Call(hwnd, 0)
	if tid == 0 {
		return false
	}
	var gti guiThreadInfo
	gti.cbSize = uint32(unsafe.Sizeof(gti))
	ok, _, _ := procGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gti)))
	if ok == 0 || gti.hwndFocus == 0 {
		return false
	}
	switch windowClassName(gti.hwndFocus) {
	case "Edit", "RICHEDIT50W", "RichEditD2DPT", "SearchEditBoxWrapperClass":
		return true
	}
	return false
}

func windowClassName(hwnd uintptr) string {
	var buf [256]uint16
	n, _, _ := procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func explorerSelectedPathCOM() string {
	unknown, err := oleutil.CreateObject("Shell.Application")
	if err != nil {
		return ""
	}
	defer unknown.Release()
	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return ""
	}
	defer disp.Release()

	wv, err := oleutil.CallMethod(disp, "Windows")
	if err != nil {
		return ""
	}
	windows := wv.ToIDispatch()
	if windows == nil {
		return ""
	}
	defer windows.Release()

	count := variantInt(oleutil.MustGetProperty(windows, "Count"))
	hwnd, _, _ := procGetForegroundWindow.Call()

	for i := 0; i < count; i++ {
		iv, err := oleutil.CallMethod(windows, "Item", i)
		if err != nil {
			continue
		}
		browser := iv.ToIDispatch()
		if browser == nil {
			continue
		}
		bh := variantInt(oleutil.MustGetProperty(browser, "HWND"))
		if hwnd != 0 && bh != int(hwnd) {
			browser.Release()
			continue
		}
		dv, err := oleutil.GetProperty(browser, "Document")
		if err != nil {
			browser.Release()
			continue
		}
		doc := dv.ToIDispatch()
		if doc == nil {
			browser.Release()
			continue
		}
		sv, err := oleutil.CallMethod(doc, "SelectedItems")
		if err != nil {
			doc.Release()
			browser.Release()
			continue
		}
		items := sv.ToIDispatch()
		if items == nil {
			doc.Release()
			browser.Release()
			continue
		}
		n := variantInt(oleutil.MustGetProperty(items, "Count"))
		for j := 0; j < n; j++ {
			fv, err := oleutil.CallMethod(items, "Item", j)
			if err != nil {
				continue
			}
			item := fv.ToIDispatch()
			if item == nil {
				continue
			}
			pv, err := oleutil.GetProperty(item, "Path")
			if err == nil {
				p := pv.ToString()
				if IsImagePath(p) {
					item.Release()
					items.Release()
					doc.Release()
					browser.Release()
					return p
				}
			}
			item.Release()
		}
		items.Release()
		doc.Release()
		browser.Release()
	}
	return ""
}

func variantInt(v *ole.VARIANT) int {
	if v == nil {
		return 0
	}
	switch x := v.Value().(type) {
	case int32:
		return int(x)
	case int64:
		return int(x)
	case int:
		return x
	case uint32:
		return int(x)
	}
	return 0
}

func spacePreviewPosition(w, h int) (int, int) {
	fg, _, _ := procGetForegroundWindow.Call()
	var r winRect
	if fg != 0 {
		_, _, _ = procGetWindowRect.Call(fg, uintptr(unsafe.Pointer(&r)))
	}
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	x := int(r.rightRect()) + 16
	if x+w > int(sw) {
		x = int(r.Left) - w - 16
	}
	if x < 0 {
		x = int(sw) - w - 24
	}
	y := int(r.Top) + 80
	if y+h > int(sh) {
		y = int(sh) - h - 24
	}
	if y < 0 {
		y = 24
	}
	return x, y
}

func (r winRect) rightRect() int32 { return r.Right }
