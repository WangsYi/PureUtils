//go:build windows

package pureimage

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

const (
	tjpfBGRA           = 8
	tjflagFastUpsample = 256
)

var (
	turboOnce sync.Once
	turboErr  error

	tjInitDecompress   *syscall.LazyProc
	tjDestroy          *syscall.LazyProc
	tjDecompressHeader *syscall.LazyProc
	tjDecompress2      *syscall.LazyProc
)

// LoadTurboJPEG locates and initializes native JPEG scaling via pureimage_codecs.dll
// (or legacy turbojpeg.dll as fallback) without cgo.
func LoadTurboJPEG() error {
	if LoadCodecs() == nil {
		return nil
	}
	turboOnce.Do(func() {
		var last error
		for _, p := range turboCandidates() {
			dll := syscall.NewLazyDLL(p)
			init := dll.NewProc("tjInitDecompress")
			if err := init.Find(); err != nil {
				last = err
				continue
			}
			tjInitDecompress = init
			tjDestroy = dll.NewProc("tjDestroy")
			tjDecompressHeader = dll.NewProc("tjDecompressHeader")
			tjDecompress2 = dll.NewProc("tjDecompress2")
			return
		}
		turboErr = last
	})
	return turboErr
}

// TurboAvailable reports whether native JPEG acceleration is available.
func TurboAvailable() bool {
	return LoadTurboJPEG() == nil
}

func turboCandidates() []string {
	exe, err := os.Executable()
	if err != nil {
		return []string{filepath.Join("libs", "turbojpeg", "turbojpeg.dll"), "turbojpeg.dll"}
	}
	dir := filepath.Dir(exe)
	return []string{
		filepath.Join(dir, "turbojpeg.dll"),
		filepath.Join(dir, "libs", "turbojpeg", "turbojpeg.dll"),
		filepath.Join("libs", "turbojpeg", "turbojpeg.dll"),
		"turbojpeg.dll",
	}
}

// DecodeTurboJPEG decodes a JPEG at the largest native DCT scale (<= 1/1) that stays near maxDim.
func DecodeTurboJPEG(data []byte, maxDim int) (image.Image, int, int, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, 0, 0, fmt.Errorf("not a JPEG")
	}
	if LoadCodecs() == nil {
		img, _, origW, origH, err := DecodeCodecImage(PicFmtJPEG, data, maxDim)
		if err == nil {
			return img, origW, origH, nil
		}
	}
	turboOnce.Do(func() {
		_ = LoadTurboJPEG()
	})
	if tjInitDecompress == nil {
		if err := LoadTurboJPEG(); err != nil {
			return nil, 0, 0, err
		}
	}
	if tjInitDecompress == nil {
		return nil, 0, 0, fmt.Errorf("turbojpeg not loaded")
	}

	h, _, _ := tjInitDecompress.Call()
	if h == 0 {
		return nil, 0, 0, fmt.Errorf("tjInitDecompress failed")
	}
	defer tjDestroy.Call(h)

	var w, hgt int32
	jpegPtr := uintptr(unsafe.Pointer(&data[0]))
	rc, _, _ := tjDecompressHeader.Call(
		h, jpegPtr, uintptr(len(data)),
		uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&hgt)),
	)
	if rc != 0 {
		return nil, 0, 0, fmt.Errorf("tjDecompressHeader failed: %d", rc)
	}
	if w <= 0 || hgt <= 0 {
		return nil, 0, 0, fmt.Errorf("empty JPEG")
	}

	limit := maxDim * 13 / 10
	m := 1
	for _, cand := range []int{8, 4, 2, 1} {
		dw := (int(w)*cand + 7) / 8
		dh := (int(hgt)*cand + 7) / 8
		if max(dw, dh) <= limit {
			m = cand
			break
		}
	}
	dw := (int(w)*m + 7) / 8
	dh := (int(hgt)*m + 7) / 8

	img := image.NewRGBA(image.Rect(0, 0, dw, dh))
	rc, _, _ = tjDecompress2.Call(
		h, jpegPtr, uintptr(len(data)),
		uintptr(unsafe.Pointer(&img.Pix[0])),
		uintptr(dw), uintptr(dw*4), uintptr(dh),
		uintptr(tjpfBGRA), uintptr(tjflagFastUpsample),
	)
	if rc != 0 {
		return nil, 0, 0, fmt.Errorf("tjDecompress2 failed: %d", rc)
	}

	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i]
	}
	return img, int(w), int(hgt), nil
}
