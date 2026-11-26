//go:build windows

package purevideo

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/config"
	pureui "pureutils/pkg/purekit/ui"
)

func uiPaletteForTest() pureui.Palette {
	return pureui.PaletteFor(true)
}

func TestGeneratePureVideoScreenshots(t *testing.T) {
	if os.Getenv("PUREUTILS_GEN_SHOTS") != "1" {
		t.Skip("set PUREUTILS_GEN_SHOTS=1 to regenerate documentation screenshots")
	}

	dir := t.TempDir()
	sample := filepath.Join(dir, "PureUtils-Showreel-4K.mp4")
	if err := os.WriteFile(sample, []byte("ftypmp42"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "02-Clip.mkv"), []byte("mkv"), 0o600)

	p := NewPlayer(config.DefaultPureVideoConfig(), PlayerCallbacks{
		Headless: true,
		BackendFactory: func(engine, _ string, _ uintptr, cb BackendCallbacks) (PlaybackBackend, string, error) {
			mb := &mockVideoBackend{
				kind: engine,
				cb:   cb,
				dur:  3*time.Minute + 45*time.Second,
				pos:  1*time.Minute + 12*time.Second,
			}
			return mb, engine, nil
		},
	})
	defer p.Shutdown()

	p.Open(sample)
	p.pos = 1*time.Minute + 12*time.Second
	p.dur = 3*time.Minute + 45*time.Second
	p.natW, p.natH = 3840, 2160

	tt := ui.NewTester(p.View, 920, 560)
	tt.SetDark(true)
	tt.Frame()

	out := filepath.Join("..", "..", "docs", "screenshots", "video.png")
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
