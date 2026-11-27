//go:build windows

package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"pureutils/pkg/purekit/i18n"
)

const (
	seeMaskNoCloseProcess   = 0x00000040
	pawnIOSilentInstallArgs = "-install -silent"
)

// ErrInstallCanceled is returned when the user declines the UAC prompt for PawnIO installation.
var ErrInstallCanceled = errors.New("administrator authorization canceled")

var (
	modShell32Mon      = windows.NewLazySystemDLL("shell32.dll")
	procShellExecMon   = modShell32Mon.NewProc("ShellExecuteW")
	procShellExecExMon = modShell32Mon.NewProc("ShellExecuteExW")
)

type shellExecuteInfoW struct {
	CbSize       uint32
	FMask        uint32
	Hwnd         uintptr
	LpVerb       *uint16
	LpFile       *uint16
	LpParameters *uint16
	LpDirectory  *uint16
	NShow        int32
	HInstApp     uintptr
	LpIDList     uintptr
	LpClass      *uint16
	HkeyClass    uintptr
	DwHotKey     uint32
	HIconMonitor uintptr
	HProcess     uintptr
}

// HelperPIDFPS transfers per-PID frame timing calculated in an elevated helper process.
type HelperPIDFPS struct {
	PID         uint32    `json:"pid"`
	FPS         float64   `json:"fps"`
	AvgFPS      float64   `json:"avg"`
	Low1Pct     float64   `json:"low1"`
	Low01Pct    float64   `json:"low01"`
	FrameTimeMs float64   `json:"ft"`
	RecentFT    []float32 `json:"rft,omitempty"`
}

// HelperFrame is streamed as NDJSON from `--monitor-helper` over a named pipe.
type HelperFrame struct {
	Timestamp time.Time                `json:"ts"`
	CPUTempC  float64                  `json:"cpuTempC,omitempty"`
	CPUPowerW float64                  `json:"cpuPowerW,omitempty"`
	PawnIO    bool                     `json:"pawnIo,omitempty"`
	DiskTemps []DiskTemperatureReading `json:"diskTemps,omitempty"`
	PIDFPS    []HelperPIDFPS           `json:"pidFps,omitempty"`
}

// LaunchElevatedExe executes exePath with UAC elevation (`runas`).
func LaunchElevatedExe(exePath, args string, showCmd int32) error {
	if strings.TrimSpace(exePath) == "" {
		return fmt.Errorf("empty executable path")
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exePath)
	var paramsPtr uintptr
	if strings.TrimSpace(args) != "" {
		p, _ := syscall.UTF16PtrFromString(args)
		paramsPtr = uintptr(unsafe.Pointer(p))
	}
	r, _, err := procShellExecMon.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		paramsPtr,
		0,
		uintptr(showCmd),
	)
	if r <= 32 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return err
		}
		return fmt.Errorf("ShellExecuteW(runas) failed: %d", r)
	}
	return nil
}

// RunPawnIOSilentInstall executes setupPath silently with `-install -silent` and waits for completion.
// When the current process is not elevated, Windows presents a single UAC prompt before hidden execution.
func RunPawnIOSilentInstall(ctx context.Context, setupPath string) error {
	setupPath = strings.TrimSpace(setupPath)
	if setupPath == "" {
		return errors.New(i18n.S("未找到 PawnIO 安装包", "PawnIO installer not found"))
	}
	if err := validateMZHeader(setupPath); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if windows.GetCurrentProcessToken().IsElevated() {
		runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(runCtx, setupPath, "-install", "-silent")
		cmd.Dir = filepath.Dir(setupPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		out, err := cmd.CombinedOutput()
		if err != nil {
			if errors.Is(runCtx.Err(), context.Canceled) {
				return context.Canceled
			}
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				return errors.New(i18n.S("PawnIO 静默安装超时", "PawnIO silent installation timed out"))
			}
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return fmt.Errorf("%s (%v: %s)", i18n.S("PawnIO 静默安装失败", "PawnIO silent installation failed"), err, msg)
			}
			return fmt.Errorf("%s (%v)", i18n.S("PawnIO 静默安装失败", "PawnIO silent installation failed"), err)
		}
		return nil
	}

	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(setupPath)
	params, _ := windows.UTF16PtrFromString(pawnIOSilentInstallArgs)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(setupPath))
	sei := shellExecuteInfoW{
		CbSize:       uint32(unsafe.Sizeof(shellExecuteInfoW{})),
		FMask:        seeMaskNoCloseProcess,
		LpVerb:       verb,
		LpFile:       file,
		LpParameters: params,
		LpDirectory:  dir,
		NShow:        windows.SW_HIDE,
	}
	r, _, callErr := procShellExecExMon.Call(uintptr(unsafe.Pointer(&sei)))
	runtime.KeepAlive(verb)
	runtime.KeepAlive(file)
	runtime.KeepAlive(params)
	runtime.KeepAlive(dir)
	if r == 0 {
		if callErr == windows.ERROR_CANCELLED {
			return ErrInstallCanceled
		}
		return fmt.Errorf("%s (%v)", i18n.S("启动 PawnIO 安装程序失败", "Failed to launch PawnIO installer"), callErr)
	}
	if sei.HProcess == 0 {
		return errors.New(i18n.S("未能获取 PawnIO 安装进程句柄", "Failed to obtain PawnIO installer process handle"))
	}
	h := windows.Handle(sei.HProcess)
	defer windows.CloseHandle(h)

	ev, werr := windows.WaitForSingleObject(h, uint32((2*time.Minute).Milliseconds()))
	if ev == uint32(windows.WAIT_TIMEOUT) {
		return errors.New(i18n.S("PawnIO 静默安装超时", "PawnIO silent installation timed out"))
	}
	if ev != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("%s (%w)", i18n.S("等待 PawnIO 安装完成失败", "Failed waiting for PawnIO installer"), werr)
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return fmt.Errorf("%s (%w)", i18n.S("获取 PawnIO 安装结果失败", "Failed to read PawnIO installer exit code"), err)
	}
	if exitCode != 0 {
		return fmt.Errorf("%s (%d)", i18n.S("PawnIO 安装程序退出码异常", "PawnIO installer exited with code"), exitCode)
	}
	return nil
}

// HelperServer listens on a local named pipe for an elevated `--monitor-helper` child process.
type HelperServer struct {
	mu        sync.RWMutex
	pipeName  string
	pipe      windows.Handle
	stopEvent windows.Handle
	connected bool
	lastFrame HelperFrame
	lastAt    time.Time
	stopped   bool
	stopCh    chan struct{}
}

var helperPipeSeq atomic.Uint64

// NewHelperServer creates a named-pipe receiver for the elevated helper process.
func NewHelperServer() *HelperServer {
	seq := helperPipeSeq.Add(1)
	return &HelperServer{
		pipeName: fmt.Sprintf(`\\.\pipe\PureUtils.MonitorHelper.%d.%d.%d`, os.Getpid(), seq, time.Now().UnixNano()%100000),
		stopCh:   make(chan struct{}),
	}
}

// PipeName returns the full Windows named pipe path.
func (s *HelperServer) PipeName() string {
	return s.pipeName
}

// StartListening creates the overlapped named pipe and starts the background pipe reader
// without launching a UAC-elevated child process.
func (s *HelperServer) StartListening() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return fmt.Errorf("helper server stopped")
	}
	if s.pipe != 0 {
		s.mu.Unlock()
		return nil
	}
	pipePath, _ := windows.UTF16PtrFromString(s.pipeName)
	h, err := windows.CreateNamedPipe(
		pipePath,
		windows.PIPE_ACCESS_INBOUND|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1,
		64*1024,
		64*1024,
		0,
		nil,
	)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	stopEv, evErr := windows.CreateEvent(nil, 1, 0, nil)
	if evErr != nil {
		_ = windows.CloseHandle(h)
		s.mu.Unlock()
		return evErr
	}
	s.pipe = h
	s.stopEvent = stopEv
	s.mu.Unlock()

	go s.servePipe(h, stopEv)
	return nil
}

// StartAndLaunch creates the named pipe, starts accepting in background, and spawns
// `PureUtils.exe --monitor-helper ...` with UAC elevation.
func (s *HelperServer) StartAndLaunch(mode, sensorDir, cpuVendor string) error {
	if err := s.StartListening(); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		s.Stop()
		return err
	}
	args := fmt.Sprintf(`--monitor-helper --pipe "%s" --mode %s --parent-pid %d --sensor-dir "%s" --cpu-vendor "%s"`,
		s.pipeName, mode, os.Getpid(), sensorDir, cpuVendor)
	if err := LaunchElevatedExe(exe, args, windows.SW_HIDE); err != nil {
		s.Stop()
		return err
	}
	return nil
}

func (s *HelperServer) servePipe(h, stopEv windows.Handle) {
	defer func() {
		s.mu.Lock()
		s.connected = false
		if s.pipe == h {
			s.pipe = 0
		}
		if s.stopEvent == stopEv {
			s.stopEvent = 0
		}
		s.mu.Unlock()
		_ = windows.CloseHandle(h)
		_ = windows.CloseHandle(stopEv)
	}()

	ioEv, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return
	}
	defer windows.CloseHandle(ioEv)

	var connectOv windows.Overlapped
	connectOv.HEvent = ioEv
	err = windows.ConnectNamedPipe(h, &connectOv)
	if err != nil && err != windows.ERROR_PIPE_CONNECTED {
		if err != windows.ERROR_IO_PENDING {
			return
		}
		ev, werr := windows.WaitForMultipleObjects([]windows.Handle{ioEv, stopEv}, false, windows.INFINITE)
		if werr != nil || ev != windows.WAIT_OBJECT_0 {
			_ = windows.CancelIoEx(h, &connectOv)
			var dummy uint32
			_ = windows.GetOverlappedResult(h, &connectOv, &dummy, true)
			return
		}
		var dummy uint32
		if gerr := windows.GetOverlappedResult(h, &connectOv, &dummy, false); gerr != nil {
			return
		}
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.connected = true
	s.mu.Unlock()

	reader := &overlappedPipeReader{
		pipe:   h,
		ioEv:   ioEv,
		stopEv: stopEv,
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 512*1024)
	for scanner.Scan() {
		select {
		case <-s.stopCh:
			return
		default:
		}
		var frame HelperFrame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}
		s.mu.Lock()
		s.lastFrame = frame
		s.lastAt = time.Now()
		s.mu.Unlock()
	}
}

type overlappedPipeReader struct {
	pipe   windows.Handle
	ioEv   windows.Handle
	stopEv windows.Handle
}

func (r *overlappedPipeReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	_ = windows.ResetEvent(r.ioEv)
	var ov windows.Overlapped
	ov.HEvent = r.ioEv
	var n uint32
	err := windows.ReadFile(r.pipe, p, &n, &ov)
	if err == nil {
		if n == 0 {
			return 0, io.EOF
		}
		return int(n), nil
	}
	if err == windows.ERROR_BROKEN_PIPE || err == windows.ERROR_HANDLE_EOF {
		return 0, io.EOF
	}
	if err != windows.ERROR_IO_PENDING {
		return 0, err
	}
	ev, werr := windows.WaitForMultipleObjects([]windows.Handle{r.ioEv, r.stopEv}, false, windows.INFINITE)
	if werr != nil || ev != windows.WAIT_OBJECT_0 {
		_ = windows.CancelIoEx(r.pipe, &ov)
		_ = windows.GetOverlappedResult(r.pipe, &ov, &n, true)
		return 0, io.EOF
	}
	if gerr := windows.GetOverlappedResult(r.pipe, &ov, &n, false); gerr != nil {
		if gerr == windows.ERROR_BROKEN_PIPE || gerr == windows.ERROR_HANDLE_EOF {
			return 0, io.EOF
		}
		return 0, gerr
	}
	if n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

// Connected reports whether an elevated helper is actively streaming frames.
func (s *HelperServer) Connected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected && !s.stopped && (s.lastAt.IsZero() || time.Since(s.lastAt) < 6*time.Second)
}

// Latest returns the most recent helper frame when fresh (< 5s).
func (s *HelperServer) Latest() (HelperFrame, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.connected || s.stopped || s.lastAt.IsZero() || time.Since(s.lastAt) > 5*time.Second {
		return HelperFrame{}, false
	}
	return s.lastFrame, true
}

// Stop signals the named pipe server to stop; handle cleanup is owned solely by servePipe.
func (s *HelperServer) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.stopCh)
	stopEv := s.stopEvent
	s.connected = false
	s.mu.Unlock()

	if stopEv != 0 {
		_ = windows.SetEvent(stopEv)
	}
}

// WatchParentProcess invokes onExit as soon as parentPID terminates.
func WatchParentProcess(parentPID uint32, onExit func()) {
	if parentPID == 0 || onExit == nil {
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, parentPID)
	if err != nil {
		return
	}
	go func() {
		defer windows.CloseHandle(h)
		_, _ = windows.WaitForSingleObject(h, windows.INFINITE)
		onExit()
	}()
}
