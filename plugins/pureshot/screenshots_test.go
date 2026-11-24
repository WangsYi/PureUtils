package pureshot

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/platform"
	"pureutils/plugins/pureshot/annotate"
)

func fillRectRGBA(img *image.RGBA, x0, y0, w, h int, c color.RGBA) {
	b := img.Bounds()
	for y := max(y0, b.Min.Y); y < min(y0+h, b.Max.Y); y++ {
		for x := max(x0, b.Min.X); x < min(x0+w, b.Max.X); x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func makeShowcaseDesktopFrame(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8(16 + (x*24)/max(w, 1))
			g := uint8(24 + (y*28)/max(h, 1))
			b := uint8(42 + ((x+y)*44)/max(w+h, 1))
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	// Simulated application window inside the capture region
	fillRectRGBA(img, 176, 102, 568, 330, color.RGBA{R: 26, G: 32, B: 44, A: 255})
	fillRectRGBA(img, 176, 102, 568, 36, color.RGBA{R: 34, G: 42, B: 58, A: 255})
	fillRectRGBA(img, 198, 156, 224, 158, color.RGBA{R: 45, G: 85, B: 158, A: 255})
	fillRectRGBA(img, 444, 156, 276, 72, color.RGBA{R: 42, G: 50, B: 68, A: 255})
	fillRectRGBA(img, 444, 244, 276, 156, color.RGBA{R: 38, G: 118, B: 90, A: 255})
	return img
}

func TestGeneratePureShotScreenshots(t *testing.T) {
	if os.Getenv("PUREUTILS_GEN_SHOTS") != "1" {
		t.Skip("set PUREUTILS_GEN_SHOTS=1 to regenerate documentation screenshots")
	}

	h := newTestHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	const w, hPx = 920, 600
	frame := makeShowcaseDesktopFrame(w, hPx)
	doc := annotate.NewDocument(600, 360)
	doc.Add(&annotate.Item{
		Tool:      annotate.ToolRect,
		Color:     core.ARGB(0xFF3B82F6),
		Thickness: 4,
		Start:     core.Point{X: 36, Y: 48},
		End:       core.Point{X: 258, Y: 222},
	})
	doc.Add(&annotate.Item{
		Tool:       annotate.ToolArrow,
		Color:      core.DefaultStrokeColor,
		Thickness:  4,
		Start:      core.Point{X: 360, Y: 184},
		End:        core.Point{X: 268, Y: 136},
		HeadLength: 14,
	})
	doc.Add(&annotate.Item{
		Tool:      annotate.ToolBadge,
		Color:     core.DefaultStrokeColor,
		TextColor: core.DefaultBadgeTextColor,
		Center:    core.Point{X: 374, Y: 192},
		Number:    1,
		Radius:    14,
	})
	doc.Add(&annotate.Item{
		Tool:          annotate.ToolText,
		Color:         core.ARGB(0xFFF8FAFC),
		Origin:        core.Point{X: 306, Y: 72},
		Text:          "PureShot 标注预览",
		FontSize:      22,
		HasBackground: true,
		Background:    core.ARGB(0xE00E1118),
	})

	mag := image.NewRGBA(image.Rect(0, 0, 120, 120))
	sess := &captureSession{
		plugin:     p,
		frame:      frame,
		monitor:    platform.Monitor{Bounds: core.RectI{X: 0, Y: 0, W: w, H: hPx}},
		contentW:   w,
		contentH:   hPx,
		selection:  core.RectI{X: 160, Y: 84, W: 600, H: 360},
		doc:        doc,
		tool:       annotate.ToolArrow,
		color:      core.DefaultStrokeColor,
		thickness:  4,
		cursor:     core.Point{X: 590, Y: 168},
		shotBitmap: ui.NewBitmap(frame),
		magnifier:  ui.NewBitmap(mag),
	}
	sess.magnifier.Pixelated()
	sess.updateMagnifier(sess.cursor)

	tt := ui.NewTester(sess.view, w, hPx)
	tt.SetDark(true)
	tt.Frame()

	out := filepath.Join("..", "..", "docs", "screenshots", "overlay.png")
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
