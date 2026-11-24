package pureclip

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
)

func makeShowcasePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8(24 + (x*72)/max(w, 1))
			g := uint8(34 + (y*48)/max(h, 1))
			b := uint8(66 + ((x+y)*120)/max(w+h, 1))
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	// Draw card accent bars inside the sample capture thumbnail.
	for y := 36; y < h-36; y++ {
		for x := 44; x < w-44; x++ {
			if y < 62 {
				img.SetRGBA(x, y, color.RGBA{R: 79, G: 70, B: 229, A: 255})
			} else if y > h-64 && x < w/2 {
				img.SetRGBA(x, y, color.RGBA{R: 16, G: 185, B: 129, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func saveShowcaseRGBA(t *testing.T, img *image.RGBA, relPath string) {
	t.Helper()
	out := filepath.Clean(relPath)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratePureClipScreenshots(t *testing.T) {
	if os.Getenv("PUREUTILS_GEN_SHOTS") != "1" {
		t.Skip("set PUREUTILS_GEN_SHOTS=1 to regenerate documentation screenshots")
	}

	h := newTestHost(t, nil)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	_ = h.io.Write(&core.ClipboardContent{
		Text: "PureUtils 原生插件化桌面平台架构设计",
		HTML: "<b>PureUtils</b> 原生插件化桌面平台架构设计",
	})
	h.watcher.Fire()
	h.clip.FlushPending()

	_ = h.io.Write(&core.ClipboardContent{
		Files: []string{`D:\Releases\PureUtils-1.0.0-setup.exe`, `D:\Releases\README.md`},
	})
	h.watcher.Fire()
	h.clip.FlushPending()

	_ = h.io.Write(&core.ClipboardContent{
		Text: "func ApplyTheme(c *ui.Context) pureui.Palette {\n\treturn pureui.PaletteFor(c.Dark())\n}",
	})
	h.watcher.Fire()
	h.clip.FlushPending()

	_ = h.clip.WriteImage(makeShowcasePNG(t, 420, 240), "PureShot")

	// Render full split history panel
	hv := newHistoryView(p)
	if len(hv.items) > 1 {
		_, _ = p.Service().ToggleFavorite(hv.items[len(hv.items)-1].ID)
		hv.Reload()
	}
	tt := ui.NewTester(hv.view, 840, 560)
	tt.SetDark(true)
	tt.Frame()
	saveShowcaseRGBA(t, tt.Image(), filepath.Join("..", "..", "docs", "screenshots", "history.png"))

	// Render compact floating Ditto-style history panel
	cv := newCompactHistoryView(p)
	ct := ui.NewTester(cv.view, compactHistoryWidth, compactHistoryHeight)
	ct.SetDark(true)
	ct.Frame()
	saveShowcaseRGBA(t, ct.Image(), filepath.Join("..", "..", "docs", "screenshots", "history-compact.png"))
}
