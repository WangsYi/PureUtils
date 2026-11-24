package platform

import (
	"fmt"
	"image"
	"runtime"
	"unsafe"

	"pureutils/pkg/purekit/core"
)

// CaptureRect copies a physical screen rectangle with GDI BitBlt.
func CaptureRect(region core.RectI, includeCursor bool) (*image.RGBA, error) {
	if err := EnsurePerMonitorV2(); err != nil {
		_ = err
	}
	bounds := region.Intersect(VirtualBounds())
	if bounds.Empty() {
		return nil, fmt.Errorf("capture region %v is outside the virtual screen", region)
	}
	return capture(bounds, includeCursor)
}

// CaptureMonitor copies one display.
func CaptureMonitor(m Monitor, includeCursor bool) (*image.RGBA, error) {
	return CaptureRect(m.Bounds, includeCursor)
}

// CaptureVirtual copies the whole virtual desktop.
func CaptureVirtual(includeCursor bool) (*image.RGBA, error) {
	return CaptureRect(VirtualBounds(), includeCursor)
}

func capture(bounds core.RectI, includeCursor bool) (*image.RGBA, error) {
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)

	bitmap, _, _ := procCreateCompatibleBitmap.Call(screenDC, uintptr(bounds.W), uintptr(bounds.H))
	if bitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bitmap)

	old, _, _ := procSelectObject.Call(memDC, bitmap)
	if old == 0 {
		return nil, fmt.Errorf("SelectObject failed")
	}
	defer procSelectObject.Call(memDC, old)

	blit := func(rop uintptr) bool {
		r, _, _ := procBitBlt.Call(memDC, 0, 0, uintptr(bounds.W), uintptr(bounds.H),
			screenDC, uintptr(int32(bounds.X)), uintptr(int32(bounds.Y)), rop)
		return r != 0
	}

	ok := blit(SRCCOPY | CAPTUREBLT)
	if !ok {
		ok = blit(SRCCOPY)
	}
	if !ok {
		return nil, fmt.Errorf("BitBlt failed for %v", bounds)
	}

	if includeCursor {
		drawCursor(HDC(memDC), bounds)
	}

	var bmi BitmapInfo
	bmi.Header.BiSize = uint32(unsafe.Sizeof(bmi.Header))
	bmi.Header.BiWidth = int32(bounds.W)
	bmi.Header.BiHeight = -int32(bounds.H)
	bmi.Header.BiPlanes = 1
	bmi.Header.BiBitCount = 32
	bmi.Header.BiCompression = BI_RGB
	bmi.Header.BiSizeImage = uint32(bounds.W * bounds.H * 4)

	bits := make([]byte, bounds.W*bounds.H*4)
	r, _, _ := procGetDIBits.Call(memDC, bitmap, 0, uintptr(bounds.H),
		ptr(unsafe.Pointer(&bits[0])), ptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS)
	runtime.KeepAlive(bits)
	if r == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}

	img := image.NewRGBA(image.Rect(0, 0, bounds.W, bounds.H))
	for i := 0; i < bounds.W*bounds.H; i++ {
		b, g, rr := bits[i*4], bits[i*4+1], bits[i*4+2]
		img.Pix[i*4+0] = rr
		img.Pix[i*4+1] = g
		img.Pix[i*4+2] = b
		img.Pix[i*4+3] = 255
	}
	return img, nil
}

func drawCursor(dc HDC, bounds core.RectI) {
	var ci CursorInfo
	ci.CbSize = uint32(unsafe.Sizeof(ci))
	r, _, _ := procGetCursorInfo.Call(uintptr(unsafe.Pointer(&ci)))
	if r == 0 || ci.HCursor == 0 || ci.Flags == 0 {
		return
	}
	cx, cy := int(ci.PtScreenPos.X)-bounds.X, int(ci.PtScreenPos.Y)-bounds.Y
	if cx < 0 || cy < 0 || cx >= bounds.W || cy >= bounds.H {
		return
	}
	cw, _, _ := procGetSystemMetrics.Call(SM_CXCURSOR)
	ch, _, _ := procGetSystemMetrics.Call(SM_CYCURSOR)
	procDrawIconEx.Call(uintptr(dc), uintptr(cx), uintptr(cy), uintptr(ci.HCursor),
		cw, ch, 0, 0, DI_NORMAL)
}

// Crop copies a rectangle out of an image.
func Crop(src *image.RGBA, r image.Rectangle) *image.RGBA {
	r = r.Intersect(src.Bounds())
	if r.Empty() {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(dst.Pix[y*dst.Stride:(y+1)*dst.Stride], src.Pix[(r.Min.Y+y)*src.Stride+r.Min.X*4:(r.Min.Y+y)*src.Stride+r.Max.X*4])
	}
	return dst
}
