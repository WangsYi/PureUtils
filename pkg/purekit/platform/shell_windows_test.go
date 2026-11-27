//go:build windows

package platform

import (
	"testing"
	"unsafe"
)

func TestShellHelpersValidationAndABISize(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		if got := unsafe.Sizeof(shellExecuteInfoW{}); got != 112 {
			t.Fatalf("shellExecuteInfoW size = %d, want 112", got)
		}
	}
	if err := ShowFileProperties(0, ""); err == nil {
		t.Fatal("expected ShowFileProperties(\"\") to return an error")
	}
	if err := SetDesktopWallpaper("   "); err == nil {
		t.Fatal("expected SetDesktopWallpaper(\"\") to return an error")
	}
}
