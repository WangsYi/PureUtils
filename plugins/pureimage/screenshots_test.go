package pureimage

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/config"
)

func renderShowcaseLandscape(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	horizon := float64(h) * 0.62

	for y := 0; y < h; y++ {
		ty := float64(y) / horizon
		for x := 0; x < w; x++ {
			tx := float64(x) / float64(w)
			var r, g, b float64
			if float64(y) < horizon {
				// Deep twilight indigo sky warming toward golden dusk at horizon
				glow := math.Exp(-math.Pow((tx-0.66)/0.28, 2)) * math.Pow(ty, 1.8)
				r = 16 + 78*math.Pow(ty, 2.2) + 150*glow
				g = 22 + 54*math.Pow(ty, 1.9) + 92*glow
				b = 48 + 72*ty - 16*math.Pow(ty, 3)
			} else {
				// Foreground lake reflection
				ry := (float64(y) - horizon) / (float64(h) - horizon)
				ripple := math.Sin(float64(y)*0.32+float64(x)*0.03) * 3.0
				glow := math.Exp(-math.Pow((tx-0.66)/0.24, 2)) * (1 - ry*0.65)
				r = 22 + 24*(1-ry) + 88*glow + ripple
				g = 28 + 20*(1-ry) + 52*glow + ripple
				b = 48 + 26*(1-ry) + ripple*0.5
			}

			// Layered mountain silhouettes
			m1 := horizon - 96 - 46*math.Sin(tx*4.2+0.4) - 24*math.Cos(tx*11.0)
			m2 := horizon - 46 - 28*math.Sin(tx*5.8+1.9) - 14*math.Sin(tx*15.0)
			if float64(y) >= m1 && float64(y) < horizon {
				r, g, b = 26, 32, 52
			}
			if float64(y) >= m2 && float64(y) < horizon+8 {
				r, g, b = 15, 19, 32
			}

			clampU8 := func(v float64) uint8 {
				if v < 0 {
					return 0
				}
				if v > 255 {
					return 255
				}
				return uint8(v)
			}
			img.SetRGBA(x, y, color.RGBA{R: clampU8(r), G: clampU8(g), B: clampU8(b), A: 255})
		}
	}
	return img
}

func encodeJPEGWithExif(t *testing.T, img image.Image) []byte {
	t.Helper()
	var jbuf bytes.Buffer
	if err := jpeg.Encode(&jbuf, img, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}
	raw := jbuf.Bytes()

	order := binary.LittleEndian
	tiff := make([]byte, 192)
	copy(tiff[0:2], "II")
	order.PutUint16(tiff[2:4], 42)
	order.PutUint32(tiff[4:8], 8)

	ifd0 := 8
	order.PutUint16(tiff[ifd0:ifd0+2], 4)
	makeOff := 62
	modelOff := 72
	exifOff := 84

	writeEntry := func(base, idx int, tag, typ uint16, count, valOrOff uint32, inline bool) {
		p := base + 2 + idx*12
		order.PutUint16(tiff[p:p+2], tag)
		order.PutUint16(tiff[p+2:p+4], typ)
		order.PutUint32(tiff[p+4:p+8], count)
		if inline && typ == 3 {
			order.PutUint16(tiff[p+8:p+10], uint16(valOrOff))
		} else {
			order.PutUint32(tiff[p+8:p+12], valOrOff)
		}
	}

	writeEntry(ifd0, 0, 0x0112, 3, 1, 1, true)
	writeEntry(ifd0, 1, 0x010F, 2, 6, uint32(makeOff), false)
	writeEntry(ifd0, 2, 0x0110, 2, 7, uint32(modelOff), false)
	writeEntry(ifd0, 3, 0x8769, 4, 1, uint32(exifOff), false)

	copy(tiff[makeOff:], "Canon\x00")
	copy(tiff[modelOff:], "EOS R5\x00")

	order.PutUint16(tiff[exifOff:exifOff+2], 4)
	expOff := exifOff + 2 + 4*12 + 4 // 138
	fnumOff := 146
	focalOff := 154

	writeEntry(exifOff, 0, 0x8827, 3, 1, 100, true)             // ISO 100
	writeEntry(exifOff, 1, 0x829A, 5, 1, uint32(expOff), false) // 1/250s
	writeEntry(exifOff, 2, 0x829D, 5, 1, uint32(fnumOff), false)
	writeEntry(exifOff, 3, 0x920A, 5, 1, uint32(focalOff), false)

	order.PutUint32(tiff[expOff:expOff+4], 1)
	order.PutUint32(tiff[expOff+4:expOff+8], 250)
	order.PutUint32(tiff[fnumOff:fnumOff+4], 28)
	order.PutUint32(tiff[fnumOff+4:fnumOff+8], 10)
	order.PutUint32(tiff[focalOff:focalOff+4], 35)
	order.PutUint32(tiff[focalOff+4:focalOff+8], 1)

	tiff = tiff[:164]
	var app1 bytes.Buffer
	app1.Write([]byte{0xFF, 0xD8, 0xFF, 0xE1})
	segLen := uint16(2 + 6 + len(tiff))
	_ = binary.Write(&app1, binary.BigEndian, segLen)
	app1.WriteString("Exif\x00\x00")
	app1.Write(tiff)
	app1.Write(raw[2:])
	return app1.Bytes()
}

func TestGeneratePureImageScreenshots(t *testing.T) {
	if os.Getenv("PUREUTILS_GEN_SHOTS") != "1" {
		t.Skip("set PUREUTILS_GEN_SHOTS=1 to regenerate documentation screenshots")
	}

	dir := t.TempDir()
	jpgPath := filepath.Join(dir, "dusk-ridge.jpg")
	if err := os.WriteFile(jpgPath, encodeJPEGWithExif(t, renderShowcaseLandscape(1280, 800)), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestPNG(t, filepath.Join(dir, "palette-sample.png"), 640, 400, color.RGBA{R: 64, G: 112, B: 224, A: 255})

	prefs := config.DefaultPureImageConfig()
	prefs.ShowDetails = true
	v := New(prefs, Callbacks{
		Headless: true,
		CanPin:   func() bool { return true },
	})
	defer v.Shutdown()

	tt := ui.NewTester(v.View, 920, 580)
	v.Open(jpgPath)
	tt.SetDark(true)
	tt.Frame()

	out := filepath.Join("..", "..", "docs", "screenshots", "viewer.png")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, tt.Image()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
