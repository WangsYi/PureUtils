package platform

import (
	"fmt"
	"sort"
	"sync"
	"syscall"
	"unsafe"

	"pureutils/pkg/purekit/core"
)

// Monitor describes a physical display.
type Monitor struct {
	Handle  HMONITOR
	Device  string
	Bounds  core.RectI
	Work    core.RectI
	DPI     uint32
	Scale   float64
	Primary bool
}

var (
	monitorCallback uintptr
	monitorEnumMu   sync.Mutex
	monitorSink     func(HMONITOR)
)

func init() {
	monitorCallback = syscall.NewCallback(func(hmon, _, _, _ uintptr) uintptr {
		if monitorSink != nil {
			monitorSink(HMONITOR(hmon))
		}
		return 1
	})
}

func Monitors() ([]Monitor, error) {
	monitorEnumMu.Lock()
	defer monitorEnumMu.Unlock()
	var out []Monitor
	monitorSink = func(h HMONITOR) {
		var mi MonitorInfoEx
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		r, _, _ := procGetMonitorInfoW.Call(uintptr(h), uintptr(unsafe.Pointer(&mi)))
		if r == 0 {
			return
		}
		dpi := monitorDPI(h)
		out = append(out, Monitor{
			Handle: h,
			Device: UTF16ToString(mi.SzDevice[:]),
			Bounds: core.RectI{X: int(mi.RcMonitor.Left), Y: int(mi.RcMonitor.Top),
				W: int(mi.RcMonitor.Right - mi.RcMonitor.Left), H: int(mi.RcMonitor.Bottom - mi.RcMonitor.Top)},
			Work: core.RectI{X: int(mi.RcWork.Left), Y: int(mi.RcWork.Top),
				W: int(mi.RcWork.Right - mi.RcWork.Left), H: int(mi.RcWork.Bottom - mi.RcWork.Top)},
			DPI:     dpi,
			Scale:   float64(dpi) / 96,
			Primary: mi.DwFlags&MONITORINFOF_PRIMARY != 0,
		})
	}
	r, _, err := procEnumDisplayMonitors.Call(0, 0, monitorCallback, 0)
	monitorSink = nil
	if r == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Primary != out[j].Primary {
			return out[i].Primary
		}
		if out[i].Bounds.X != out[j].Bounds.X {
			return out[i].Bounds.X < out[j].Bounds.X
		}
		return out[i].Bounds.Y < out[j].Bounds.Y
	})
	return out, nil
}

func PrimaryMonitor() (Monitor, error) {
	ms, err := Monitors()
	if err != nil {
		return Monitor{}, err
	}
	if len(ms) == 0 {
		return Monitor{}, fmt.Errorf("no displays found")
	}
	for _, m := range ms {
		if m.Primary {
			return m, nil
		}
	}
	return ms[0], nil
}

func MonitorAtPoint(p core.Point) (Monitor, error) {
	packed := uintptr(uint32(int32(p.X))) | uintptr(uint32(int32(p.Y)))<<32
	h, _, _ := procMonitorFromPoint.Call(packed, MONITOR_DEFAULTTONEAREST)
	if h == 0 {
		return Monitor{}, fmt.Errorf("MonitorFromPoint returned 0")
	}
	return monitorByHandle(HMONITOR(h))
}

func MonitorAtCursor() (Monitor, error) {
	p, err := CursorPosition()
	if err != nil {
		return PrimaryMonitor()
	}
	return MonitorAtPoint(p)
}

func MonitorForRect(r core.RectI) (Monitor, error) {
	rc := Rect{Left: int32(r.X), Top: int32(r.Y), Right: int32(r.X + r.W), Bottom: int32(r.Y + r.H)}
	h, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&rc)), MONITOR_DEFAULTTONEAREST)
	if h == 0 {
		return Monitor{}, fmt.Errorf("MonitorFromRect returned 0")
	}
	return monitorByHandle(HMONITOR(h))
}

func monitorByHandle(h HMONITOR) (Monitor, error) {
	var mi MonitorInfoEx
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	r, _, _ := procGetMonitorInfoW.Call(uintptr(h), uintptr(unsafe.Pointer(&mi)))
	if r == 0 {
		return Monitor{}, fmt.Errorf("GetMonitorInfo failed")
	}
	dpi := monitorDPI(h)
	return Monitor{
		Handle: h,
		Device: UTF16ToString(mi.SzDevice[:]),
		Bounds: core.RectI{X: int(mi.RcMonitor.Left), Y: int(mi.RcMonitor.Top),
			W: int(mi.RcMonitor.Right - mi.RcMonitor.Left), H: int(mi.RcMonitor.Bottom - mi.RcMonitor.Top)},
		Work: core.RectI{X: int(mi.RcWork.Left), Y: int(mi.RcWork.Top),
			W: int(mi.RcWork.Right - mi.RcWork.Left), H: int(mi.RcWork.Bottom - mi.RcWork.Top)},
		DPI:     dpi,
		Scale:   float64(dpi) / 96,
		Primary: mi.DwFlags&MONITORINFOF_PRIMARY != 0,
	}, nil
}

func monitorDPI(h HMONITOR) uint32 {
	var x, y uint32
	r, _, _ := procGetDpiForMonitor.Call(uintptr(h), MDT_EFFECTIVE_DPI, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
	if hresultOK(r) && x > 0 {
		return x
	}
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi > 0 {
		return uint32(dpi)
	}
	return 96
}

func VirtualBounds() core.RectI {
	get := func(index int) int {
		v, _, _ := procGetSystemMetrics.Call(uintptr(index))
		return int(int32(v))
	}
	return core.RectI{X: get(SM_XVIRTUALSCREEN), Y: get(SM_YVIRTUALSCREEN),
		W: get(SM_CXVIRTUALSCREEN), H: get(SM_CYVIRTUALSCREEN)}
}

func CursorPosition() (core.Point, error) {
	var p Point
	r, _, err := procGetPhysicalCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		r, _, err = procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	}
	if r == 0 {
		return core.Point{}, err
	}
	return core.Point{X: float64(p.X), Y: float64(p.Y)}, nil
}

func EnsurePerMonitorV2() error {
	r, _, err := procSetProcessDpiAwarenessContext.Call(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)
	if r == 0 {
		return err
	}
	return nil
}

func MonitorBoundsAtCursor() (core.RectI, error) {
	m, err := MonitorAtCursor()
	if err != nil {
		return core.RectI{}, err
	}
	return m.Bounds, nil
}
