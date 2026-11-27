package platform

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestIsIgnoredWindowClass(t *testing.T) {
	for _, cls := range []string{
		"Progman",
		"WorkerW",
		"Shell_TrayWnd",
		"shell_secondarytraywnd",
		"Windows.UI.Core.CoreWindow",
		" ForegroundStaging ",
		"MultitaskingViewFrame",
		"ApplicationManager_DesktopShellWindow",
	} {
		if !isIgnoredWindowClass(cls) {
			t.Errorf("isIgnoredWindowClass(%q) = false, want true", cls)
		}
	}
	for _, cls := range []string{"Chrome_WidgetWin_1", "CabinetWClass", "Notepad", ""} {
		if isIgnoredWindowClass(cls) {
			t.Errorf("isIgnoredWindowClass(%q) = true, want false", cls)
		}
	}
}

func TestEnsureICOFromPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.SetRGBA(4, 4, color.RGBA{R: 255, G: 120, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	pngBytes := buf.Bytes()

	cachePath := filepath.Join(t.TempDir(), "icons", "subapp.ico")
	got := EnsureICOFromPNG(pngBytes, "", cachePath)
	if got != cachePath {
		t.Fatalf("EnsureICOFromPNG = %q, want %q", got, cachePath)
	}
	icoBytes, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read generated ico: %v", err)
	}
	if len(icoBytes) != 22+len(pngBytes) {
		t.Fatalf("ico len = %d, want %d", len(icoBytes), 22+len(pngBytes))
	}
	if binary.LittleEndian.Uint16(icoBytes[0:2]) != 0 ||
		binary.LittleEndian.Uint16(icoBytes[2:4]) != 1 ||
		binary.LittleEndian.Uint16(icoBytes[4:6]) != 1 {
		t.Fatalf("invalid ICONDIR header: %v", icoBytes[:6])
	}
	if !bytes.Equal(icoBytes[22:], pngBytes) {
		t.Fatal("ICO payload does not match source PNG")
	}
	if got2 := EnsureICOFromPNG(pngBytes, "", cachePath); got2 != cachePath {
		t.Fatalf("second EnsureICOFromPNG = %q, want %q", got2, cachePath)
	}
	if bad := EnsureICOFromPNG([]byte("not a png"), "", filepath.Join(t.TempDir(), "bad.ico")); bad != "" {
		t.Fatalf("expected empty path for invalid PNG, got %q", bad)
	}
}

func TestTrimWorkingSet(t *testing.T) {
	TrimWorkingSet()
}
