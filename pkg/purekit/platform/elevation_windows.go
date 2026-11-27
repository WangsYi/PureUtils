//go:build windows

package platform

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// ElevatedStartupTaskName is the Task Scheduler task name for UAC-free admin logon startup.
	ElevatedStartupTaskName = "PureUtilsAutostart"

	seeMaskNoCloseProcess = 0x00000040
)

// ErrElevationCanceled is returned when the user declines or cancels the UAC prompt.
var ErrElevationCanceled = errors.New("administrator elevation canceled")

var procShellExecuteExW = shell32.NewProc("ShellExecuteExW")

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

// IsElevated reports whether the current process is running with elevated administrator privileges.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// CurrentUserSID returns the Windows security identifier (e.g. "S-1-5-21-...") for the current token user.
func CurrentUserSID() (string, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("get token user: %w", err)
	}
	if tu == nil || tu.User.Sid == nil {
		return "", errors.New("missing user SID in process token")
	}
	return tu.User.Sid.String(), nil
}

// WaitProcessExit waits up to timeout for the given PID to terminate (used for clean elevated relaunch handoff).
func WaitProcessExit(pid uint32, timeout time.Duration) {
	if pid == 0 {
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)

	ms := uint32(0)
	if timeout > 0 {
		ms = uint32(timeout.Milliseconds())
	}
	_, _ = windows.WaitForSingleObject(h, ms)
}

// RelaunchElevated starts a new elevated copy of the current executable with args via ShellExecuteW("runas").
func RelaunchElevated(args string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	verb := Utf16Ptr("runas")
	fileP := Utf16Ptr(exe)
	var paramsP *uint16
	if strings.TrimSpace(args) != "" {
		paramsP = Utf16Ptr(args)
	}
	r, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(fileP)),
		uintptr(unsafe.Pointer(paramsP)),
		0,
		uintptr(SW_SHOWNORMAL),
	)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(fileP)
	runtime.KeepAlive(paramsP)
	if r <= 32 {
		if callErr == windows.ERROR_CANCELLED || r == 5 {
			return fmt.Errorf("%w (%v)", ErrElevationCanceled, callErr)
		}
		return fmt.Errorf("ShellExecuteW(runas) failed (%d): %v", r, callErr)
	}
	return nil
}

func runElevatedCommandWait(file, params string, timeout time.Duration) error {
	verb := Utf16Ptr("runas")
	fileP := Utf16Ptr(file)
	paramsP := Utf16Ptr(params)
	sei := shellExecuteInfoW{
		CbSize:       uint32(unsafe.Sizeof(shellExecuteInfoW{})),
		FMask:        seeMaskNoCloseProcess,
		LpVerb:       verb,
		LpFile:       fileP,
		LpParameters: paramsP,
		NShow:        windows.SW_HIDE,
	}
	r, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	runtime.KeepAlive(verb)
	runtime.KeepAlive(fileP)
	runtime.KeepAlive(paramsP)
	if r == 0 {
		if callErr == windows.ERROR_CANCELLED {
			return fmt.Errorf("%w: %v", ErrElevationCanceled, callErr)
		}
		return fmt.Errorf("ShellExecuteExW(%s) failed: %v", file, callErr)
	}
	if sei.HProcess == 0 {
		return fmt.Errorf("ShellExecuteExW(%s) returned empty process handle", file)
	}
	h := windows.Handle(sei.HProcess)
	defer windows.CloseHandle(h)

	var ms uint32 = windows.INFINITE
	if timeout > 0 {
		ms = uint32(timeout.Milliseconds())
	}
	ev, werr := windows.WaitForSingleObject(h, ms)
	if ev != windows.WAIT_OBJECT_0 {
		if ev == uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("%s timed out after %v", file, timeout)
		}
		return fmt.Errorf("wait for %s: %w", file, werr)
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return fmt.Errorf("get exit code for %s: %w", file, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("%s exited with code %d", file, exitCode)
	}
	return nil
}

func runHiddenSchtasks(args ...string) error {
	cmd := exec.Command("schtasks.exe", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("schtasks %s: %w (%s)", strings.Join(args, " "), err, msg)
		}
		return fmt.Errorf("schtasks %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// ElevatedStartupTaskExists reports whether the Task Scheduler auto-start task is registered.
func ElevatedStartupTaskExists() bool {
	return runHiddenSchtasks("/Query", "/TN", ElevatedStartupTaskName) == nil
}

func escapeXMLText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// BuildElevatedStartupTaskXML constructs Task Scheduler 1.2 XML for UAC-free highest-privilege logon startup.
func BuildElevatedStartupTaskXML(userSID, exePath, args string) (string, error) {
	sid := strings.TrimSpace(userSID)
	exe := strings.TrimSpace(exePath)
	trimmedArgs := strings.TrimSpace(args)
	if sid == "" {
		return "", errors.New("empty user SID")
	}
	if exe == "" {
		return "", errors.New("empty executable path")
	}

	var argsElem string
	if trimmedArgs != "" {
		argsElem = fmt.Sprintf("\n      <Arguments>%s</Arguments>", escapeXMLText(trimmedArgs))
	}
	xmlText := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%s</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>false</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>%s
    </Exec>
  </Actions>
</Task>
`, escapeXMLText(sid), escapeXMLText(sid), escapeXMLText(exe), argsElem)
	return xmlText, nil
}

func encodeUTF16LEWithBOM(s string) []byte {
	codes := utf16.Encode([]rune(s))
	out := make([]byte, 2+len(codes)*2)
	out[0] = 0xFF
	out[1] = 0xFE
	for i, c := range codes {
		binary.LittleEndian.PutUint16(out[2+i*2:], c)
	}
	return out
}

// SetElevatedRunOnStartupWithArgs creates or removes the elevated Task Scheduler logon entry.
// When enabled, any HKCU Run entry is removed so PureUtils does not launch twice at logon.
// Non-interactive callers never trigger UAC prompts.
func SetElevatedRunOnStartupWithArgs(enabled bool, args string, interactive bool) error {
	if !enabled {
		if !ElevatedStartupTaskExists() {
			return nil
		}
		if IsElevated() {
			return runHiddenSchtasks("/Delete", "/TN", ElevatedStartupTaskName, "/F")
		}
		if !interactive {
			return nil
		}
		return runElevatedCommandWait("schtasks.exe", fmt.Sprintf(`/Delete /TN "%s" /F`, ElevatedStartupTaskName), 20*time.Second)
	}

	if !IsElevated() && !interactive {
		if ElevatedStartupTaskExists() {
			_ = SetRunOnStartupWithArgs(false, "")
			return nil
		}
		return errors.New("elevated startup task is not registered")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	sid, err := CurrentUserSID()
	if err != nil {
		return err
	}
	xmlText, err := BuildElevatedStartupTaskXML(sid, exe, args)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "pureutils-task-*.xml")
	if err != nil {
		return fmt.Errorf("create temp task xml: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, werr := tmp.Write(encodeUTF16LEWithBOM(xmlText)); werr != nil {
		_ = tmp.Close()
		return fmt.Errorf("write task xml: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		return fmt.Errorf("close task xml: %w", cerr)
	}

	if IsElevated() {
		if err := runHiddenSchtasks("/Create", "/TN", ElevatedStartupTaskName, "/XML", tmpPath, "/F"); err != nil {
			return err
		}
	} else {
		params := fmt.Sprintf(`/Create /TN "%s" /XML "%s" /F`, ElevatedStartupTaskName, tmpPath)
		if err := runElevatedCommandWait("schtasks.exe", params, 30*time.Second); err != nil {
			return err
		}
	}

	// Ensure standard HKCU Run key is removed so the app launches once cleanly via the scheduled task.
	_ = SetRunOnStartupWithArgs(false, "")
	return nil
}
