//go:build windows

package fps

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrETWAccessDenied indicates StartTraceW failed due to lack of Performance Log Users / Admin rights.
var ErrETWAccessDenied = errors.New("etw: access denied (requires Performance Log Users or elevated helper)")

var (
	modAdvapi32ETW    = windows.NewLazySystemDLL("advapi32.dll")
	modKernel32ETW    = windows.NewLazySystemDLL("kernel32.dll")
	procStartTraceW   = modAdvapi32ETW.NewProc("StartTraceW")
	procControlTraceW = modAdvapi32ETW.NewProc("ControlTraceW")
	procEnableTraceEx = modAdvapi32ETW.NewProc("EnableTraceEx2")
	procOpenTraceW    = modAdvapi32ETW.NewProc("OpenTraceW")
	procProcessTrace  = modAdvapi32ETW.NewProc("ProcessTrace")
	procCloseTrace    = modAdvapi32ETW.NewProc("CloseTrace")
	procQPC           = modKernel32ETW.NewProc("QueryPerformanceCounter")
	procQPF           = modKernel32ETW.NewProc("QueryPerformanceFrequency")
)

const (
	etwSessionName            = "PureUtils-FPS-ETW"
	etwRealTimeMode           = 0x00000100
	etwProcessTraceEventRec   = 0x10000000
	etwWnodeFlagTracedGUID    = 0x00020000
	etwControlStop            = 1
	etwEnableProvider         = 1
	etwTraceLevelInfo         = 4
	etwErrorAccessDenied      = 5
	etwInvalidTraceHandle     = ^uintptr(0)
)

var (
	// Microsoft-Windows-DXGI {CA11C036-0102-4A2D-A6AD-F03CFED5D3C9}
	guidDXGI = windows.GUID{
		Data1: 0xCA11C036,
		Data2: 0x0102,
		Data3: 0x4A2D,
		Data4: [8]byte{0xA6, 0xAD, 0xF0, 0x3C, 0xFE, 0xD5, 0xD3, 0xC9},
	}
	// Microsoft-Windows-D3D9 {783ACA0A-790E-4D7F-8451-AA850511C6B9}
	guidD3D9 = windows.GUID{
		Data1: 0x783ACA0A,
		Data2: 0x790E,
		Data3: 0x4D7F,
		Data4: [8]byte{0x84, 0x51, 0xAA, 0x85, 0x05, 0x11, 0xC6, 0xB9},
	}
	// Microsoft-Windows-DxgKrnl {802EC45A-1E99-4B83-9920-87C98277BA9D}
	guidDxgKrnl = windows.GUID{
		Data1: 0x802EC45A,
		Data2: 0x1E99,
		Data3: 0x4B83,
		Data4: [8]byte{0x99, 0x20, 0x87, 0xC9, 0x82, 0x77, 0xBA, 0x9D},
	}
)

type wnodeHeader struct {
	BufferSize        uint32
	ProviderID        uint32
	HistoricalContext uint64
	TimeStamp         int64
	GUID              windows.GUID
	ClientContext     uint32
	Flags             uint32
}

type eventTraceProperties struct {
	Wnode               wnodeHeader
	BufferSize          uint32
	MinimumBuffers      uint32
	MaximumBuffers      uint32
	MaximumFileSize     uint32
	LogFileMode         uint32
	FlushTimer          uint32
	EnableFlags         uint32
	AgeLimit            int32
	NumberOfBuffers     uint32
	FreeBuffers         uint32
	EventsLost          uint32
	BuffersWritten      uint32
	LogBuffersLost      uint32
	RealTimeBuffersLost uint32
	LoggerThreadID      uintptr
	LogFileNameOffset   uint32
	LoggerNameOffset    uint32
}

type eventTracePropsBuf struct {
	Props      eventTraceProperties
	LoggerName [256]uint16
}

type etwEventDescriptor struct {
	ID      uint16
	Version byte
	Channel byte
	Level   byte
	Opcode  byte
	Task    uint16
	Keyword uint64
}

type etwEventHeader struct {
	Size            uint16
	HeaderType      uint16
	Flags           uint16
	EventProperty   uint16
	ThreadID        uint32
	ProcessID       uint32
	TimeStamp       int64
	ProviderID      windows.GUID
	EventDescriptor etwEventDescriptor
	ProcessorTime   uint64
	ActivityID      windows.GUID
}

type etwBufferContext struct {
	ProcessorNumber byte
	Alignment       byte
	LoggerID        uint16
}

type etwEventRecord struct {
	EventHeader       etwEventHeader
	BufferContext     etwBufferContext
	ExtendedDataCount uint16
	UserDataLength    uint16
	ExtendedData      uintptr
	UserData          uintptr
	UserContext       uintptr
}

type etwTraceHeader struct {
	Size           uint16
	FieldTypeFlags uint16
	Version        uint32
	ThreadID       uint32
	ProcessID      uint32
	TimeStamp      int64
	GUID           windows.GUID
	ClientContext  uint32
	Flags          uint32
}

type etwEventTrace struct {
	Header           etwTraceHeader
	InstanceID       uint32
	ParentInstanceID uint32
	ParentGUID       windows.GUID
	MofData          uintptr
	MofLength        uint32
	BufferContext    uint32
}

type etwTimeZoneInfo struct {
	Bias         int32
	StandardName [32]uint16
	StandardDate [8]uint16
	StandardBias int32
	DaylightName [32]uint16
	DaylightDate [8]uint16
	DaylightBias int32
}

type etwTraceLogfileHeader struct {
	BufferSize         uint32
	Version            uint32
	ProviderVersion    uint32
	NumberOfProcessors uint32
	EndTime            int64
	TimerResolution    uint32
	MaximumFileSize    uint32
	LogFileMode        uint32
	BuffersWritten     uint32
	LogInstanceGUID    windows.GUID
	LoggerName         *uint16
	LogFileName        *uint16
	TimeZone           etwTimeZoneInfo
	BootTime           int64
	PerfFreq           int64
	StartTime          int64
	ReservedFlags      uint32
	BuffersLost        uint32
}

type etwTraceLogfileW struct {
	LogFileName         *uint16
	LoggerName          *uint16
	CurrentTime         int64
	BuffersRead         uint32
	ProcessTraceMode    uint32
	CurrentEvent        etwEventTrace
	LogfileHeader       etwTraceLogfileHeader
	BufferCallback      uintptr
	BufferSize          uint32
	Filled              uint32
	EventsLost          uint32
	EventRecordCallback uintptr
	IsKernelTrace       uint32
	Context             uintptr
}

var (
	etwActiveEngine atomic.Pointer[ETWEngine]
	etwRecordCB     = syscall.NewCallback(func(recPtr uintptr) uintptr {
		eng := etwActiveEngine.Load()
		if eng != nil && recPtr != 0 {
			rec := (*etwEventRecord)(unsafe.Pointer(recPtr))
			eng.handleRecord(rec)
		}
		return 0
	})
)

// MonotonicNowNS returns current QPC monotonic time in nanoseconds, matching ETW event timestamps.
func MonotonicNowNS() int64 {
	var cnt, freq int64
	procQPC.Call(uintptr(unsafe.Pointer(&cnt)))
	procQPF.Call(uintptr(unsafe.Pointer(&freq)))
	return qpcToNS(cnt, freq)
}

func qpcToNS(cnt, freq int64) int64 {
	if cnt <= 0 || freq <= 0 {
		return time.Now().UnixNano()
	}
	secs := cnt / freq
	rem := cnt % freq
	return secs*1_000_000_000 + (rem*1_000_000_000)/freq
}

// ETWEngine runs an out-of-process, zero-injection ETW real-time trace session
// listening to DXGI, D3D9, and DxgKrnl present/flip events.
type ETWEngine struct {
	mu          sync.Mutex
	stats       *FrameStats
	sessionName string
	qpcFreq     int64
	running     bool
	sessionH    uint64
	traceH      uintptr
	doneCh      chan struct{}
	lastErr     error
}

// NewETWEngine creates a new ETW real-time frame capture engine.
func NewETWEngine(stats *FrameStats) *ETWEngine {
	if stats == nil {
		stats = NewFrameStats()
	}
	var freq int64
	procQPF.Call(uintptr(unsafe.Pointer(&freq)))
	if freq <= 0 {
		freq = 10_000_000
	}
	return &ETWEngine{
		stats:       stats,
		sessionName: etwSessionName,
		qpcFreq:     freq,
	}
}

// Stats returns the underlying FrameStats store.
func (e *ETWEngine) Stats() *FrameStats {
	return e.stats
}

// Running reports whether the ETW session is currently active.
func (e *ETWEngine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// LastError returns the most recent ETW startup/runtime error, if any.
func (e *ETWEngine) LastError() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastErr
}

// Start begins real-time ETW capture on a dedicated OS thread.
func (e *ETWEngine) Start() error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil
	}

	// Ensure no stale session from a previous crash blocks StartTraceW.
	e.stopExistingSessionLocked()

	props := newPropsBuf(e.sessionName)
	var sessionH uint64
	sessPtr, _ := syscall.UTF16PtrFromString(e.sessionName)
	r, _, _ := procStartTraceW.Call(
		uintptr(unsafe.Pointer(&sessionH)),
		uintptr(unsafe.Pointer(sessPtr)),
		uintptr(unsafe.Pointer(&props.Props)),
	)
	if r != 0 {
		var err error = syscall.Errno(r)
		if r == etwErrorAccessDenied {
			err = ErrETWAccessDenied
		}
		e.lastErr = err
		e.mu.Unlock()
		return err
	}

	// Enable DXGI (Present_Start=42, PresentMultiplaneOverlay_Start=55), D3D9 (Present_Start=1),
	// and DxgKrnl (MatchAnyKeyword=0x1: PresentHistory=184, Flip=171, MMIOFlip=259).
	enable := func(guid windows.GUID, kw uint64) error {
		r2, _, _ := procEnableTraceEx.Call(
			uintptr(sessionH),
			uintptr(unsafe.Pointer(&guid)),
			etwEnableProvider,
			etwTraceLevelInfo,
			uintptr(kw),
			0,
			0,
			0,
		)
		if r2 != 0 {
			return fmt.Errorf("EnableTraceEx2(%v): %w", guid, syscall.Errno(r2))
		}
		return nil
	}
	if err := enable(guidDXGI, 0); err != nil {
		e.stopSessionHandle(sessionH)
		e.lastErr = err
		e.mu.Unlock()
		return err
	}
	_ = enable(guidD3D9, 0)
	_ = enable(guidDxgKrnl, 0x1)

	logf := &etwTraceLogfileW{
		LoggerName:          sessPtr,
		ProcessTraceMode:    etwRealTimeMode | etwProcessTraceEventRec,
		EventRecordCallback: etwRecordCB,
	}
	th, _, callErr := procOpenTraceW.Call(uintptr(unsafe.Pointer(logf)))
	if th == etwInvalidTraceHandle || th == 0 {
		e.stopSessionHandle(sessionH)
		err := fmt.Errorf("OpenTraceW: %w", callErr)
		e.lastErr = err
		e.mu.Unlock()
		return err
	}

	etwActiveEngine.Store(e)
	e.sessionH = sessionH
	e.traceH = th
	e.running = true
	e.lastErr = nil
	done := make(chan struct{})
	e.doneCh = done
	e.mu.Unlock()

	go e.runLoop(th, sessPtr, logf, done)
	return nil
}

func (e *ETWEngine) runLoop(th uintptr, sessPtr *uint16, logf *etwTraceLogfileW, done chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)
	defer func() {
		runtime.KeepAlive(sessPtr)
		runtime.KeepAlive(logf)
	}()

	handles := [1]uintptr{th}
	_, _, _ = procProcessTrace.Call(uintptr(unsafe.Pointer(&handles[0])), 1, 0, 0)

	e.mu.Lock()
	e.running = false
	e.mu.Unlock()
}

// Stop terminates the ETW session and waits briefly for ProcessTrace to unwind.
func (e *ETWEngine) Stop() {
	e.mu.Lock()
	if !e.running && e.traceH == 0 && e.sessionH == 0 {
		e.mu.Unlock()
		return
	}
	sh := e.sessionH
	th := e.traceH
	done := e.doneCh
	e.sessionH = 0
	e.traceH = 0
	e.running = false
	e.mu.Unlock()

	if etwActiveEngine.Load() == e {
		etwActiveEngine.Store(nil)
	}
	if sh != 0 {
		e.stopSessionHandle(sh)
	}
	if th != 0 {
		procCloseTrace.Call(th)
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

func (e *ETWEngine) stopExistingSessionLocked() {
	props := newPropsBuf(e.sessionName)
	sessPtr, _ := syscall.UTF16PtrFromString(e.sessionName)
	procControlTraceW.Call(0, uintptr(unsafe.Pointer(sessPtr)), uintptr(unsafe.Pointer(&props.Props)), etwControlStop)
}

func (e *ETWEngine) stopSessionHandle(sh uint64) {
	props := newPropsBuf(e.sessionName)
	procControlTraceW.Call(uintptr(sh), 0, uintptr(unsafe.Pointer(&props.Props)), etwControlStop)
}

func (e *ETWEngine) handleRecord(rec *etwEventRecord) {
	pid := rec.EventHeader.ProcessID
	if pid == 0 {
		return
	}
	id := rec.EventHeader.EventDescriptor.ID
	prov := rec.EventHeader.ProviderID

	var isPrimary, match bool
	switch prov {
	case guidDXGI:
		if id == 42 || id == 55 {
			match = true
			isPrimary = true
		}
	case guidD3D9:
		if id == 1 {
			match = true
			isPrimary = true
		}
	case guidDxgKrnl:
		if id == 184 || id == 171 || id == 259 {
			match = true
			isPrimary = false
		}
	}
	if !match {
		return
	}

	tsNS := qpcToNS(rec.EventHeader.TimeStamp, e.qpcFreq)
	e.stats.RecordFrame(pid, tsNS, isPrimary, time.Now())
}

func newPropsBuf(sessionName string) *eventTracePropsBuf {
	var buf eventTracePropsBuf
	buf.Props.Wnode.BufferSize = uint32(unsafe.Sizeof(buf))
	buf.Props.Wnode.ClientContext = 1 // QPC high-res clock
	buf.Props.Wnode.Flags = etwWnodeFlagTracedGUID
	buf.Props.BufferSize = 64
	buf.Props.MinimumBuffers = 4
	buf.Props.MaximumBuffers = 64
	buf.Props.LogFileMode = etwRealTimeMode
	buf.Props.FlushTimer = 1
	buf.Props.LoggerNameOffset = uint32(unsafe.Offsetof(buf.LoggerName))
	if u16, err := syscall.UTF16FromString(sessionName); err == nil {
		copy(buf.LoggerName[:], u16)
	}
	return &buf
}
