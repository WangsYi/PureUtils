package platform

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestClipboardTextRoundTripAndRelease(t *testing.T) {
	mw, err := NewMessageWindow("clipboard-test")
	if err != nil {
		t.Skipf("message window unavailable: %v", err)
	}
	defer mw.Close()
	cb := NewClipboard(mw.HWND)
	if err := cb.WriteText("PureUtils clipboard test 你好"); err != nil {
		t.Fatalf("write text: %v", err)
	}
	content, err := cb.Read()
	if err != nil {
		t.Fatalf("read text: %v", err)
	}
	if content == nil || content.Text != "PureUtils clipboard test 你好" {
		t.Fatalf("content = %+v", content)
	}
	if _, err := cb.Read(); err != nil {
		t.Fatalf("second read: %v", err)
	}
}

func TestClipboardImageRoundTrip(t *testing.T) {
	mw, err := NewMessageWindow("clipboard-image-test")
	if err != nil {
		t.Skipf("message window unavailable: %v", err)
	}
	defer mw.Close()
	cb := NewClipboard(mw.HWND)
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 40), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := cb.WriteImagePNG(buf.Bytes()); err != nil {
		t.Fatalf("write image: %v", err)
	}
	got, err := cb.ReadImagePNG()
	if err != nil {
		t.Fatalf("read image: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("empty image")
	}
	if _, err := png.Decode(bytes.NewReader(got)); err != nil {
		t.Fatalf("returned bytes are not PNG: %v", err)
	}
}

func TestDIBRoundTrip(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(10 + x*10), G: uint8(20 + y*10), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	dib, err := pngToDIB(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got, err := dibToRGBA(dib)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != img.Bounds() {
		t.Fatalf("bounds = %v", got.Bounds())
	}
	r1, g1, b1, _ := got.At(2, 1).RGBA()
	r2, g2, b2, _ := img.At(2, 1).RGBA()
	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf("pixel = %d,%d,%d want %d,%d,%d", r1, g1, b1, r2, g2, b2)
	}
}

func TestBuildDropFiles(t *testing.T) {
	data := buildDropFiles([]string{`C:\A\one.txt`, `C:\A\two.txt`})
	if len(data) < int(unsafe.Sizeof(DropFiles{})) {
		t.Fatalf("short drop payload: %d", len(data))
	}
	if data[0] == 0 {
		t.Error("missing pFiles header")
	}
}

func TestFocusWindowPreservesMaximized(t *testing.T) {
	const wsOverlappedWindow = 0x00CF0000
	inst, _, _ := procGetModuleHandleW.Call(0)
	hwndPtr, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(Utf16Ptr("STATIC"))),
		uintptr(unsafe.Pointer(Utf16Ptr("pureutils-focus-test"))),
		wsOverlappedWindow,
		0, 0, 320, 240,
		0, 0, inst, 0,
	)
	if hwndPtr == 0 {
		t.Skip("window station unavailable in test environment")
	}
	defer procDestroyWindow.Call(hwndPtr)

	procShowWindow.Call(hwndPtr, SW_SHOWMAXIMIZED)
	zoomedBefore, _, _ := procIsZoomed.Call(hwndPtr)
	if zoomedBefore == 0 {
		t.Skip("could not maximize test window")
	}

	_ = FocusWindow(HWND(hwndPtr))
	zoomedAfter, _, _ := procIsZoomed.Call(hwndPtr)
	if zoomedAfter == 0 {
		t.Fatal("FocusWindow un-maximized an already maximized window")
	}
}

func TestDIBFormatsAndHTMLImageFallbacks(t *testing.T) {
	dib32 := make([]byte, 40+12+16)
	binary.LittleEndian.PutUint32(dib32[0:4], 40)
	binary.LittleEndian.PutUint32(dib32[4:8], 2)
	binary.LittleEndian.PutUint32(dib32[8:12], 0xFFFFFFFE)
	binary.LittleEndian.PutUint16(dib32[12:14], 1)
	binary.LittleEndian.PutUint16(dib32[14:16], 32)
	binary.LittleEndian.PutUint32(dib32[16:20], BI_BITFIELDS)
	binary.LittleEndian.PutUint32(dib32[40:44], 0x00FF0000)
	binary.LittleEndian.PutUint32(dib32[44:48], 0x0000FF00)
	binary.LittleEndian.PutUint32(dib32[48:52], 0x000000FF)
	binary.LittleEndian.PutUint32(dib32[52:56], 0x800A141E)
	img32, err := dibToRGBA(dib32)
	if err != nil {
		t.Fatalf("32bpp bitfields: %v", err)
	}
	px := img32.RGBAAt(0, 0)
	if px.R != 10 || px.G != 20 || px.B != 30 || px.A != 255 {
		t.Fatalf("32bpp pixel = %+v, want (10,20,30,255)", px)
	}

	v5 := make([]byte, 124+4)
	binary.LittleEndian.PutUint32(v5[0:4], 124)
	binary.LittleEndian.PutUint32(v5[4:8], 1)
	binary.LittleEndian.PutUint32(v5[8:12], 0xFFFFFFFF)
	binary.LittleEndian.PutUint16(v5[12:14], 1)
	binary.LittleEndian.PutUint16(v5[14:16], 32)
	binary.LittleEndian.PutUint32(v5[16:20], BI_BITFIELDS)
	binary.LittleEndian.PutUint32(v5[40:44], 0x000000FF)
	binary.LittleEndian.PutUint32(v5[44:48], 0x0000FF00)
	binary.LittleEndian.PutUint32(v5[48:52], 0x00FF0000)
	binary.LittleEndian.PutUint32(v5[52:56], 0xFF000000)
	binary.LittleEndian.PutUint32(v5[124:128], 0x80332211)
	imgV5, err := dibToRGBA(v5)
	if err != nil {
		t.Fatalf("V5 DIB: %v", err)
	}
	if px := imgV5.RGBAAt(0, 0); px != (color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0x80}) {
		t.Fatalf("V5 pixel = %+v", px)
	}

	sample := image.NewRGBA(image.Rect(0, 0, 3, 2))
	sample.SetRGBA(1, 0, color.RGBA{R: 12, G: 200, B: 34, A: 255})
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, sample); err != nil {
		t.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString(pngBuf.Bytes())
	gotData := extractHTMLImagePNG(`<!--StartFragment--><img src="data:image/png;base64,` + b64 + `"><!--EndFragment-->`)
	if len(gotData) == 0 {
		t.Fatal("extractHTMLImagePNG failed for data URI")
	}
	imgFile := filepath.Join(t.TempDir(), "clip.png")
	if err := os.WriteFile(imgFile, pngBuf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	fileURI := "file:///" + strings.ReplaceAll(imgFile, `\`, "/")
	gotFile := extractHTMLImagePNG(`<img src="` + fileURI + `">`)
	if len(gotFile) == 0 {
		t.Fatal("extractHTMLImagePNG failed for file URI")
	}

	mw, err := NewMessageWindow("cf-bitmap-test")
	if err == nil {
		defer mw.Close()
		cb := NewClipboard(mw.HWND)
		screenDC, _, _ := procGetDC.Call(0)
		if screenDC != 0 {
			hbmp, _, _ := procCreateCompatibleBitmap.Call(screenDC, 6, 4)
			procReleaseDC.Call(0, screenDC)
			if hbmp != 0 {
				if cb.open() {
					procEmptyClipboard.Call()
					r, _, _ := procSetClipboardData.Call(CF_BITMAP, hbmp)
					cb.close()
					if r == 0 {
						procDeleteObject.Call(hbmp)
					} else {
						gotBMP, err := cb.ReadImagePNG()
						if err != nil || len(gotBMP) == 0 {
							t.Fatalf("CF_BITMAP ReadImagePNG failed: len=%d err=%v", len(gotBMP), err)
						}
						dec, err := png.Decode(bytes.NewReader(gotBMP))
						if err != nil || dec.Bounds().Dx() != 6 || dec.Bounds().Dy() != 4 {
							t.Fatalf("CF_BITMAP decoded invalid PNG: %v", err)
						}
					}
				} else {
					procDeleteObject.Call(hbmp)
				}
			}
		}
	}
}
