//go:build windows

package overlay

import (
	"math"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"pureutils/pkg/purekit/config"
	"pureutils/plugins/puremonitor/collector"
)

type gameHUDOverlay struct {
	mu       sync.Mutex
	cfg      config.PureMonitorConfig
	snap     collector.SystemSnapshot
	active   bool
	monX     int
	monY     int
	monW     int
	monH     int

	hwnd     uintptr
	threadID uint32
	dib      dibSurface
	visible  bool
	readyCh  chan struct{}
	stopOnce sync.Once
}

func newGameHUDOverlay(cfg config.PureMonitorConfig) *gameHUDOverlay {
	g := &gameHUDOverlay{
		cfg:     cfg,
		readyCh: make(chan struct{}),
	}
	go g.runThread()
	<-g.readyCh
	return g
}

func (g *gameHUDOverlay) Update(snap collector.SystemSnapshot, cfg config.PureMonitorConfig, active bool, monX, monY, monW, monH int) {
	g.mu.Lock()
	g.snap = snap
	g.cfg = cfg
	g.active = active
	g.monX = monX
	g.monY = monY
	g.monW = monW
	g.monH = monH
	hwnd := g.hwnd
	g.mu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmAppRender, 0, 0)
	}
}

func (g *gameHUDOverlay) Stop() {
	g.stopOnce.Do(func() {
		g.mu.Lock()
		hwnd := g.hwnd
		tid := g.threadID
		g.mu.Unlock()
		if hwnd != 0 {
			procPostMessageW.Call(hwnd, wmAppClose, 0, 0)
		} else if tid != 0 {
			procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0)
		}
	})
}

func (g *gameHUDOverlay) runThread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := procGetCurrentThreadID.Call()
	g.mu.Lock()
	g.threadID = uint32(tid)
	g.mu.Unlock()

	if err := ensureOverlayWindowClass(); err != nil {
		close(g.readyCh)
		return
	}

	hInst, _, _ := procGetModuleHandleW.Call(0)
	clsName, _ := syscall.UTF16PtrFromString("PureUtilsMonitorOverlay")
	title, _ := syscall.UTF16PtrFromString("PureMonitor Game HUD")

	exStyle := uintptr(wsExTopmost | wsExToolWindow | wsExLayered | wsExNoActivate | wsExTransparent)
	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		16, 16, 200, 120,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		close(g.readyCh)
		return
	}

	overlayWndMap.Store(hwnd, g.wndProc)
	g.mu.Lock()
	g.hwnd = hwnd
	g.mu.Unlock()
	close(g.readyCh)

	var msg winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	overlayWndMap.Delete(hwnd)
	g.dib.free()
}

func (g *gameHUDOverlay) wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmAppRender, wmAppConfig:
		g.render()
		return 0
	case wmAppClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostThreadMessageW.Call(uintptr(g.threadID), wmQuit, 0, 0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func (g *gameHUDOverlay) render() {
	g.mu.Lock()
	hwnd := g.hwnd
	cfg := g.cfg
	snap := g.snap
	active := g.active
	mx, my, mw, mh := g.monX, g.monY, g.monW, g.monH
	g.mu.Unlock()
	if hwnd == 0 {
		return
	}

	if !cfg.Enabled || !cfg.GameFPSEnabled || !active {
		if g.visible {
			procShowWindow.Call(hwnd, swHide)
			g.visible = false
		}
		return
	}

	if mw <= 0 || mh <= 0 {
		mon := monitorBoundsAt(0, 0)
		mx, my = int(mon.Left), int(mon.Top)
		mw, mh = mon.Width(), mon.Height()
	}

	scale := windowScale(hwnd)
	img := RenderGameHUDImage(snap, cfg, scale)
	if img == nil {
		return
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	x, y := computeGameHUDCoords(mx, my, mw, mh, w, h, cfg.GameHUDPosition, scale)

	g.dib.commitRGBA(hwnd, img, x, y, true)
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoActivate)
	if !g.visible {
		procShowWindow.Call(hwnd, swShowNA)
		g.visible = true
	}
}

func computeGameHUDCoords(mx, my, mw, mh, w, h int, pos string, scale float64) (x, y int) {
	if scale <= 0 {
		scale = 1
	}
	margin := int(math.Round(14 * scale))
	switch strings.ToLower(strings.TrimSpace(pos)) {
	case "top_right":
		return mx + mw - w - margin, my + margin
	case "bottom_left":
		return mx + margin, my + mh - h - margin
	case "bottom_right":
		return mx + mw - w - margin, my + mh - h - margin
	default: // "top_left"
		return mx + margin, my + margin
	}
}
