package pureimage

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/config"
)

func TestNaturalLessAndFormatPredicates(t *testing.T) {
	if !NaturalLess("img2.png", "img10.png") {
		t.Error("expected img2.png < img10.png")
	}
	if !NaturalLess("IMG01.jpg", "img1.jpg") {
		// NaturalLess sorts "1" before "01" (fewer leading zeros) — opposite should be false
	}
	if !NaturalLess("img1.jpg", "img01.jpg") {
		t.Error("expected img1.jpg < img01.jpg")
	}
	if NaturalLess("img20.png", "img3.png") {
		t.Error("did not expect img20.png < img3.png")
	}

	if !IsImagePath("a/b/Photo.HEIC") || !IsImagePath("raw.cr3") || IsImagePath("notes.txt") {
		t.Error("IsImagePath returned unexpected result")
	}
	if !IsJPEGExt(".JPG") || IsJPEGExt(".png") {
		t.Error("IsJPEGExt returned unexpected result")
	}
	if !IsRawExt(".NEF") || !IsSpecialExt(".avif") || !IsAnimatedExt("clip.webp") {
		t.Error("special/raw/animated format predicate failed")
	}
}

func buildMinimalTestJPEGWithExif(order binary.ByteOrder) []byte {
	// Encode 2x2 JPEG first, then inject APP1 EXIF segment right after SOI.
	var jbuf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	_ = jpeg.Encode(&jbuf, img, nil)
	raw := jbuf.Bytes()

	// Build minimal TIFF payload:
	// [0..8) Header, [8..) IFD0 with 4 entries (Orientation, Make, Model, ExifIFD),
	// followed by Make string, Model string, and ExifIFD with ISO & Exposure & FNumber & Focal.
	tiff := make([]byte, 256)
	if order == binary.LittleEndian {
		copy(tiff[0:2], "II")
	} else {
		copy(tiff[0:2], "MM")
	}
	order.PutUint16(tiff[2:4], 42)
	order.PutUint32(tiff[4:8], 8) // IFD0 offset

	ifd0 := 8
	order.PutUint16(tiff[ifd0:ifd0+2], 4)
	makeOff := 8 + 2 + 4*12 + 4 // 62
	modelOff := 70
	exifOff := 80

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

	writeEntry(ifd0, 0, 0x0112, 3, 1, 6, true)              // Orientation = 6
	writeEntry(ifd0, 1, 0x010F, 2, 6, uint32(makeOff), false)  // Make = "Canon\x00"
	writeEntry(ifd0, 2, 0x0110, 2, 7, uint32(modelOff), false) // Model = "EOS R5\x00"
	writeEntry(ifd0, 3, 0x8769, 4, 1, uint32(exifOff), false)  // ExifIFD

	copy(tiff[makeOff:], "Canon\x00")
	copy(tiff[modelOff:], "EOS R5\x00")

	order.PutUint16(tiff[exifOff:exifOff+2], 4)
	expOff := exifOff + 2 + 4*12 + 4 // 134
	fnumOff := 142
	focalOff := 150

	writeEntry(exifOff, 0, 0x8827, 3, 1, 400, true)             // ISO 400
	writeEntry(exifOff, 1, 0x829A, 5, 1, uint32(expOff), false) // 1/125
	writeEntry(exifOff, 2, 0x829D, 5, 1, uint32(fnumOff), false)
	writeEntry(exifOff, 3, 0x920A, 5, 1, uint32(focalOff), false)

	order.PutUint32(tiff[expOff:expOff+4], 1)
	order.PutUint32(tiff[expOff+4:expOff+8], 125)
	order.PutUint32(tiff[fnumOff:fnumOff+4], 28)
	order.PutUint32(tiff[fnumOff+4:fnumOff+8], 10)
	order.PutUint32(tiff[focalOff:focalOff+4], 50)
	order.PutUint32(tiff[focalOff+4:focalOff+8], 1)

	tiff = tiff[:160]
	var app1 bytes.Buffer
	app1.Write([]byte{0xFF, 0xD8, 0xFF, 0xE1})
	segLen := uint16(2 + 6 + len(tiff))
	_ = binary.Write(&app1, binary.BigEndian, segLen)
	app1.WriteString("Exif\x00\x00")
	app1.Write(tiff)
	app1.Write(raw[2:])
	return app1.Bytes()
}

func TestParseExifLittleAndBigEndian(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order binary.ByteOrder
	}{
		{"little-endian", binary.LittleEndian},
		{"big-endian", binary.BigEndian},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := ParseExif(buildMinimalTestJPEGWithExif(tc.order))
			if ex.Orientation != 6 {
				t.Errorf("Orientation = %d, want 6", ex.Orientation)
			}
			if ex.Camera() != "Canon EOS R5" {
				t.Errorf("Camera = %q, want Canon EOS R5", ex.Camera())
			}
			wantExp := "1/125s · f/2.8 · ISO 400 · 50.0mm"
			if ex.ExposureText() != wantExp {
				t.Errorf("ExposureText = %q, want %q", ex.ExposureText(), wantExp)
			}
		})
	}
}

func TestTransformAndFitWithin(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})
	rot := TransformImage(src, 1, 90, false)
	if rot.Bounds().Dx() != 2 || rot.Bounds().Dy() != 4 {
		t.Fatalf("rotated bounds = %v, want 2x4", rot.Bounds())
	}
	w, h := TransformedDims(400, 200, 6, 90)
	if w != 400 || h != 200 {
		t.Errorf("TransformedDims(400,200,orient=6,rot=90) = %dx%d, want 400x200", w, h)
	}
	fw, fh := FitWithin(4000, 2000, 1600)
	if fw != 1600 || fh != 800 {
		t.Errorf("FitWithin = %dx%d, want 1600x800", fw, fh)
	}
}

func writeTestPNG(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()
}

func TestScanDirSortsAndRecursion(t *testing.T) {
	dir := t.TempDir()
	p2 := filepath.Join(dir, "img2.png")
	p10 := filepath.Join(dir, "img10.png")
	pSub := filepath.Join(dir, "nested", "img1.png")
	writeTestPNG(t, p2, 10, 10, color.RGBA{R: 10, A: 255})
	writeTestPNG(t, p10, 40, 40, color.RGBA{G: 20, A: 255})
	writeTestPNG(t, pSub, 4, 4, color.RGBA{B: 30, A: 255})

	v := New(config.DefaultPureImageConfig(), Callbacks{Headless: true})
	got := v.ScanDir(dir)
	if len(got) != 2 || filepath.Base(got[0]) != "img2.png" || filepath.Base(got[1]) != "img10.png" {
		t.Fatalf("natural sort non-recursive = %v", got)
	}

	v.prefs.Recursive = true
	v.prefs.Sort = "name"
	v.prefs.SortDesc = true
	got = v.ScanDir(dir)
	if len(got) != 3 || filepath.Base(got[0]) != "img10.png" {
		t.Fatalf("recursive desc = %v", got)
	}
}

func TestDecodeStaticAndAnimatedImages(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "alpha.png")
	writeTestPNG(t, pngPath, 32, 16, color.RGBA{R: 10, G: 20, B: 30, A: 128})
	if IsAPNGPath(pngPath) {
		t.Fatal("plain PNG flagged as APNG")
	}
	bmp, raw, sw, sh, _, alpha, err := DecodePreview(pngPath, 1600, 0, false)
	if err != nil || bmp == nil || raw == nil || sw != 32 || sh != 16 || !alpha {
		t.Fatalf("DecodePreview PNG = sw=%d sh=%d alpha=%v err=%v", sw, sh, alpha, err)
	}
	bmp.ReleasePixels()

	gifPath := filepath.Join(dir, "anim.gif")
	pal := color.Palette{color.Black, color.White}
	f1 := image.NewPaletted(image.Rect(0, 0, 8, 8), pal)
	f2 := image.NewPaletted(image.Rect(0, 0, 8, 8), pal)
	f2.SetColorIndex(1, 1, 1)
	var gbuf bytes.Buffer
	_ = gif.EncodeAll(&gbuf, &gif.GIF{
		Image:    []*image.Paletted{f1, f2},
		Delay:    []int{5, 10},
		Disposal: []byte{gif.DisposalNone, gif.DisposalNone},
		Config:   image.Config{ColorModel: pal, Width: 8, Height: 8},
	})
	if err := os.WriteFile(gifPath, gbuf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	frames, first, delays, gw, gh, _, _, err := DecodeAnimation(gifPath, 1000, 0, false)
	if err != nil || len(frames) != 2 || len(delays) != 2 || first == nil || gw != 8 || gh != 8 {
		t.Fatalf("DecodeAnimation GIF = frames=%d gw=%d gh=%d err=%v", len(frames), gw, gh, err)
	}
	for _, fr := range frames {
		fr.ReleasePixels()
	}
}

func TestViewerHeadlessEmptyMenusToolbarAndPin(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "photo1.png")
	p2 := filepath.Join(dir, "photo2.png")
	writeTestPNG(t, p1, 120, 80, color.RGBA{R: 255, A: 255})
	writeTestPNG(t, p2, 80, 120, color.RGBA{G: 255, A: 255})

	var changedPrefs []config.PureImageConfig
	var pinnedImg image.Image
	v := New(config.DefaultPureImageConfig(), Callbacks{
		Headless: true,
		CanPin:   func() bool { return true },
		OnPrefsChange: func(vs config.PureImageConfig) {
			changedPrefs = append(changedPrefs, vs)
		},
		OnPinImage: func(img image.Image) {
			pinnedImg = img
		},
	})
	defer v.Shutdown()

	tt := ui.NewTester(v.View, 1100, 720)
	tt.Frame()
	if !tt.HasText("PureImage") || !tt.HasText("打开图片") {
		t.Fatalf("empty state missing copy: %v", tt.Texts())
	}

	if err := tt.Click("设置"); err != nil {
		t.Fatalf("click settings in empty state: %v", err)
	}
	tt.Frame()
	if !v.settingsOpen || !tt.HasText("滚轮缩放（关闭则滚轮切换）") {
		t.Fatalf("settings flyout not open: %v", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	// Load photo1.png in directory
	v.Open(p1)
	tt.Frame()
	if filepath.Base(v.path) != "photo1.png" || !tt.HasText("photo1.png") || !tt.HasText("1/2") {
		t.Fatalf("loaded HUD missing photo1 info: path=%q info=%q texts=%v", v.path, v.info, tt.Texts())
	}

	// Next button -> photo2.png
	if err := tt.Click("下一张"); err != nil {
		t.Fatalf("click next: %v", err)
	}
	tt.Frame()
	if filepath.Base(v.path) != "photo2.png" || !tt.HasText("2/2") {
		t.Fatalf("expected photo2.png after next, got %q (%v)", v.path, tt.Texts())
	}

	// Zoom in -> leaves fit mode
	if err := tt.Click("放大"); err != nil {
		t.Fatalf("click zoomIn: %v", err)
	}
	tt.Frame()
	if v.fit {
		t.Fatal("expected fit=false after zoom in")
	}
	if err := tt.Click("适配窗口"); err != nil {
		t.Fatalf("click fit: %v", err)
	}
	tt.Frame()
	if !v.fit {
		t.Fatal("expected fit=true after fit")
	}

	// Toggle details HUD
	if err := tt.Click("详细信息"); err != nil {
		t.Fatalf("click details: %v", err)
	}
	tt.Frame()
	if !v.prefs.ShowDetails || len(changedPrefs) == 0 {
		t.Fatalf("details toggle did not persist/notify")
	}

	// Sort menu
	if err := tt.Click("排序"); err != nil {
		t.Fatalf("click sort: %v", err)
	}
	tt.Frame()
	if !v.sortOpen || !tt.HasText("名称降序") {
		t.Fatalf("sort menu missing: %v", tt.Texts())
	}
	if err := tt.Click("名称降序"); err != nil {
		t.Fatalf("click sortNameDesc: %v", err)
	}
	tt.Frame()
	if !v.prefs.SortDesc {
		t.Fatal("expected SortDesc=true after menu selection")
	}

	// Pin to screen callback
	if err := tt.Click("贴到屏幕"); err != nil {
		t.Fatalf("click pin: %v", err)
	}
	tt.Frame()
	if pinnedImg == nil {
		t.Fatal("OnPinImage callback was not invoked")
	}

	// In-memory image viewer
	memImg := image.NewRGBA(image.Rect(0, 0, 64, 32))
	v.OpenImage("内存贴图", memImg)
	tt.Frame()
	if !tt.HasText("内存贴图") || !tt.HasText("64×32") {
		t.Fatalf("OpenImage HUD mismatch: %v", tt.Texts())
	}
	if err := tt.Click("旋转"); err != nil {
		t.Fatalf("click rotate on in-memory image: %v", err)
	}
	tt.Frame()
	if v.srcW != 32 || v.srcH != 64 {
		t.Fatalf("rotated in-memory dims = %dx%d, want 32x64", v.srcW, v.srcH)
	}
}
