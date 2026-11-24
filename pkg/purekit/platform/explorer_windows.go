//go:build windows

package platform

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"

	"pureutils/pkg/purekit/core"
)

type guiThreadInfo struct {
	cbSize        uint32
	flags         uint32
	hwndActive    uintptr
	hwndFocus     uintptr
	hwndCapture   uintptr
	hwndMenuOwner uintptr
	hwndMoveSize  uintptr
	hwndCaret     uintptr
	rcCaret       Rect
}

type explorerReply struct {
	paths []string
	err   error
}

var (
	explorerSTAOnce sync.Once
	explorerSTAReq  chan chan explorerReply
)

func ensureExplorerSTA() {
	explorerSTAOnce.Do(func() {
		ch := make(chan chan explorerReply, 8)
		explorerSTAReq = ch
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			_ = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
			defer ole.CoUninitialize()
			for reply := range ch {
				paths, err := explorerSelectedPathsCOM(ForegroundWindow())
				reply <- explorerReply{paths: paths, err: err}
			}
		}()
	})
}

func WindowClassName(hwnd HWND) string {
	if hwnd == 0 {
		return ""
	}
	var buf [256]uint16
	n, _, _ := procGetClassNameW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func isExplorerClassName(class string) bool {
	c := strings.TrimSpace(class)
	return c == "CabinetWClass" || c == "ExploreWClass"
}

func isTextInputClassName(class string) bool {
	switch strings.TrimSpace(class) {
	case "Edit", "RICHEDIT50W", "RichEditD2DPT", "SearchEditBoxWrapperClass":
		return true
	default:
		return false
	}
}

// IsExplorerForeground reports whether a Windows File Explorer window is active.
func IsExplorerForeground() bool {
	return isExplorerClassName(WindowClassName(ForegroundWindow()))
}

// IsExplorerTextInputFocused reports whether the user is currently typing in
// an Explorer rename box, address bar, or search box.
func IsExplorerTextInputFocused() bool {
	fg := ForegroundWindow()
	if fg == 0 {
		return false
	}
	var tid uintptr
	tid, _, _ = procGetWindowThreadProcessID.Call(uintptr(fg), 0)
	if tid == 0 {
		return false
	}
	var gti guiThreadInfo
	gti.cbSize = uint32(unsafe.Sizeof(gti))
	ok, _, _ := procGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gti)))
	if ok == 0 || gti.hwndFocus == 0 {
		return false
	}
	return isTextInputClassName(WindowClassName(HWND(gti.hwndFocus)))
}

// ForegroundWindowRect returns the bounding rectangle of the current foreground
// window, useful for positioning contextual popups near Explorer.
func ForegroundWindowRect() (core.RectI, bool) {
	fg := ForegroundWindow()
	if fg == 0 {
		return core.RectI{}, false
	}
	return windowVisibleRect(uintptr(fg))
}

// ExplorerSelectedFiles returns all selected file/folder paths from the active
// Windows File Explorer window via STA COM Shell.Application, or nil if the
// foreground window is not Explorer or has no selection.
func ExplorerSelectedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 450*time.Millisecond)
	defer cancel()
	return ExplorerSelectedFilesContext(ctx)
}

// ExplorerSelectedFilesContext returns all selected paths in the foreground
// File Explorer window, respecting ctx cancellation.
func ExplorerSelectedFilesContext(ctx context.Context) ([]string, error) {
	if !IsExplorerForeground() {
		return nil, nil
	}
	ensureExplorerSTA()
	replyCh := make(chan explorerReply, 1)
	select {
	case explorerSTAReq <- replyCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case rep := <-replyCh:
		return rep.paths, rep.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func explorerSelectedPathsCOM(targetHWND HWND) ([]string, error) {
	unknown, err := oleutil.CreateObject("Shell.Application")
	if err != nil {
		return nil, fmt.Errorf("Shell.Application: %w", err)
	}
	defer unknown.Release()
	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, fmt.Errorf("IDispatch: %w", err)
	}
	defer disp.Release()

	wv, err := oleutil.CallMethod(disp, "Windows")
	if err != nil {
		return nil, fmt.Errorf("Shell.Windows: %w", err)
	}
	windows := wv.ToIDispatch()
	if windows == nil {
		return nil, nil
	}
	defer windows.Release()

	count := variantInt(oleutil.MustGetProperty(windows, "Count"))
	var rawPaths []string

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
		if targetHWND != 0 && bh != int(targetHWND) {
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
				rawPaths = append(rawPaths, pv.ToString())
			}
			item.Release()
		}
		items.Release()
		doc.Release()
		browser.Release()
		if len(rawPaths) > 0 {
			break
		}
	}
	return normalizeSelectedPaths(rawPaths), nil
}

func normalizeSelectedPaths(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		key := strings.ToLower(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
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
