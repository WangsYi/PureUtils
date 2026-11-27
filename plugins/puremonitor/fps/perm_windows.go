//go:build windows

package fps

import (
	"fmt"
	"os/user"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modShell32Perm    = windows.NewLazySystemDLL("shell32.dll")
	procShellExecPerm = modShell32Perm.NewProc("ShellExecuteW")
)

// PermissionStatus summarizes current process privileges for ETW FPS capture.
type PermissionStatus struct {
	IsElevated          bool
	InPerfLogUsersGroup bool
	CanStartETW         bool
	GroupName           string
	UserName            string
}

// CheckFPSPermission checks whether the current process is elevated or belongs to
// the built-in Performance Log Users group (well-known SID S-1-5-32-559).
func CheckFPSPermission() PermissionStatus {
	st := PermissionStatus{}
	tok := windows.GetCurrentProcessToken()
	st.IsElevated = tok.IsElevated()

	if sid, err := windows.CreateWellKnownSid(windows.WinBuiltinPerfLoggingUsersSid); err == nil {
		if ok, err := tok.IsMember(sid); err == nil {
			st.InPerfLogUsersGroup = ok
		}
		// Resolve localized group name via LookupAccountSid (works on Chinese/English/any Windows).
		if acct, _, _, err := sid.LookupAccount(""); err == nil && acct != "" {
			st.GroupName = acct
		}
	}
	if u, err := user.Current(); err == nil {
		st.UserName = u.Username
	}
	st.CanStartETW = st.IsElevated || st.InPerfLogUsersGroup
	return st
}

// GrantPerfLogUsersPermission triggers a single UAC prompt to add the current user
// to the localized Performance Log Users group (S-1-5-32-559).
func GrantPerfLogUsersPermission() error {
	sid, err := windows.CreateWellKnownSid(windows.WinBuiltinPerfLoggingUsersSid)
	if err != nil {
		return err
	}
	groupName, _, _, err := sid.LookupAccount("")
	if err != nil || strings.TrimSpace(groupName) == "" {
		groupName = "Performance Log Users"
	}

	u, err := user.Current()
	if err != nil || strings.TrimSpace(u.Username) == "" {
		return fmt.Errorf("resolve current user: %w", err)
	}
	// `net localgroup "<group>" "<DOMAIN\User>" /add` handles local & MSA accounts cleanly.
	args := fmt.Sprintf(`localgroup "%s" "%s" /add`, groupName, u.Username)

	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString("net.exe")
	params, _ := syscall.UTF16PtrFromString(args)

	r, _, callErr := procShellExecPerm.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		uintptr(windows.SW_HIDE),
	)
	if r <= 32 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return callErr
		}
		return fmt.Errorf("ShellExecuteW(net localgroup) failed: %d", r)
	}
	return nil
}
