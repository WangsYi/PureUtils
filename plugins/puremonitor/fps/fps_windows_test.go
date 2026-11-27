//go:build windows

package fps

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
	"unsafe"
)

func TestETWRecordDispatchAndHelpers(t *testing.T) {
	t1 := MonotonicNowNS()
	t2 := MonotonicNowNS()
	if t1 <= 0 || t2 < t1 {
		t.Fatalf("MonotonicNowNS invalid: t1=%d t2=%d", t1, t2)
	}
	if got := qpcToNS(0, 1000); got <= 0 {
		t.Fatalf("qpcToNS fallback invalid: %d", got)
	}
	if got := qpcToNS(100, 0); got <= 0 {
		t.Fatalf("qpcToNS zero-freq fallback invalid: %d", got)
	}
	if got := qpcToNS(10_000_000, 10_000_000); got != 1_000_000_000 {
		t.Fatalf("qpcToNS(1s) = %d, want 1e9", got)
	}

	props := newPropsBuf("PureUtils-Test")
	if props.Props.Wnode.BufferSize != uint32(unsafe.Sizeof(*props)) || props.Props.Wnode.ClientContext != 1 {
		t.Fatalf("unexpected props header: %+v", props.Props)
	}

	eng := NewETWEngine(nil)
	if eng.Stats() == nil || eng.Running() || eng.LastError() != nil {
		t.Fatalf("unexpected initial ETWEngine state")
	}
	eng.Stop() // no-op when stopped

	// Feed synthetic ETW records across DXGI, D3D9, DxgKrnl, ignored IDs, and PID=0.
	baseQPC := int64(100 * eng.qpcFreq)
	sendFrames := func(pid uint32, prov [16]byte, id uint16, count int) {
		for i := 0; i < count; i++ {
			var rec etwEventRecord
			rec.EventHeader.ProcessID = pid
			rec.EventHeader.EventDescriptor.ID = id
			rec.EventHeader.TimeStamp = baseQPC + int64(i)*eng.qpcFreq/60
			copy((*[16]byte)(unsafe.Pointer(&rec.EventHeader.ProviderID))[:], prov[:])
			eng.handleRecord(&rec)
		}
	}
	dxgiBytes := *(*[16]byte)(unsafe.Pointer(&guidDXGI))
	d3d9Bytes := *(*[16]byte)(unsafe.Pointer(&guidD3D9))
	dxgkBytes := *(*[16]byte)(unsafe.Pointer(&guidDxgKrnl))

	sendFrames(0, dxgiBytes, 42, 5)
	sendFrames(1001, dxgiBytes, 42, 20)
	sendFrames(1001, dxgiBytes, 99, 5)  // non-present DXGI event ignored
	sendFrames(1002, dxgiBytes, 55, 20) // PresentMultiplaneOverlay_Start
	sendFrames(1003, d3d9Bytes, 1, 20)
	sendFrames(1003, d3d9Bytes, 2, 5)
	sendFrames(1004, dxgkBytes, 184, 20)
	sendFrames(1005, dxgkBytes, 171, 20)
	sendFrames(1006, dxgkBytes, 259, 20)
	sendFrames(1007, dxgkBytes, 999, 10) // unrecognized ID
	sendFrames(1008, [16]byte{1, 2, 3}, 42, 10)

	nowNS := qpcToNS(baseQPC+eng.qpcFreq/2, eng.qpcFreq)
	for _, pid := range []uint32{1001, 1002, 1003, 1004, 1005, 1006} {
		st, ok := eng.Stats().StatsForPID(pid, nowNS)
		if !ok || st.FPS < 30 || st.FPS > 90 {
			t.Fatalf("PID %d stats = (%+v, %v)", pid, st, ok)
		}
	}
	for _, pid := range []uint32{1007, 1008} {
		if _, ok := eng.Stats().StatsForPID(pid, nowNS); ok {
			t.Fatalf("PID %d should have been ignored", pid)
		}
	}
}

func TestETWStartStopLifecycle(t *testing.T) {
	eng := NewETWEngine(NewFrameStats())
	eng.sessionName = fmt.Sprintf("PureUtils-UnitTest-%d-%d", os.Getpid(), time.Now().UnixNano()%10000)

	err := eng.Start()
	defer eng.Stop()
	if err == nil {
		if !eng.Running() || eng.LastError() != nil {
			t.Fatalf("expected Running=true after clean Start")
		}
		// Calling Start again while running should be a clean no-op.
		if err2 := eng.Start(); err2 != nil {
			t.Fatalf("second Start = %v", err2)
		}
		eng.Stop()
		if eng.Running() {
			t.Fatal("expected Running=false after Stop")
		}
	} else {
		if eng.Running() || eng.LastError() == nil {
			t.Fatalf("expected Running=false and non-nil LastError on Start failure")
		}
		// Non-admin systems return ErrETWAccessDenied.
		if !errors.Is(err, ErrETWAccessDenied) {
			t.Logf("ETW start error (non-access): %v", err)
		}
	}
}

func TestPermissionsAndForegroundProbe(t *testing.T) {
	perm := CheckFPSPermission()
	if perm.UserName == "" {
		t.Fatal("expected non-empty current Windows username")
	}
	if perm.CanStartETW != (perm.IsElevated || perm.InPerfLogUsersGroup) {
		t.Fatalf("inconsistent CanStartETW state: %+v", perm)
	}

	fg, _, _, _, _ := Win32ForegroundProbe{}.Probe()
	_ = fg
	_ = IsAnyFullscreenWindow()
}
