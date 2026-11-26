//go:build windows

package purevideo

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
)

func assertNonBlankFrame(t *testing.T, img *image.RGBA, shotName string) {
	t.Helper()
	if img == nil {
		t.Fatalf("%s: rendered frame image is nil", shotName)
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Fatalf("%s: invalid frame bounds %v", shotName, b)
	}
	first := img.RGBAAt(b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
			if img.RGBAAt(x, y) != first {
				return
			}
		}
	}
	t.Errorf("%s: rendered frame appears completely blank/uniform (%+v)", shotName, first)
}

func checkLayoutBounds(t *testing.T, tt *ui.Tester, winW float32, shotName string, forbidHan bool) {
	t.Helper()
	texts := tt.Texts()
	if len(texts) == 0 {
		t.Errorf("%s: rendered view has no text elements", shotName)
	}
	img := tt.Image()
	assertNonBlankFrame(t, img, shotName)

	for _, txt := range texts {
		if forbidHan {
			for _, r := range txt {
				if unicode.Is(unicode.Han, r) {
					t.Errorf("%s: unexpected Han text %q in English UI", shotName, txt)
					break
				}
			}
		}
		if rect, ok := tt.Find(txt); ok && rect.W > 0 && rect.H > 0 {
			if rect.X < -2 || rect.X+rect.W > winW+2 {
				t.Errorf("%s: text %q horizontally out of bounds: rect=%+v winW=%.0f", shotName, txt, rect, winW)
			}
			if txt != "Divider" && rect.W > 4 && rect.W < 45 && rect.H > 55 {
				t.Errorf("%s: text %q crushed narrow column: rect=%+v", shotName, txt, rect)
			}
		}
	}
	envVar := "PUREUTILS_ZH_SHOT_DIR"
	if forbidHan {
		envVar = "PUREUTILS_EN_SHOT_DIR"
	}
	if dir := os.Getenv(envVar); dir != "" && shotName != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, shotName+".png"), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func checkEnglishVideoView(t *testing.T, tt *ui.Tester, winW float32, shotName string) {
	t.Helper()
	checkLayoutBounds(t, tt, winW, shotName, true)
}

func TestEnglishPureVideoLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	dir := t.TempDir()
	sample := filepath.Join(dir, "PureUtils-Showreel-4K.mp4")
	if err := os.WriteFile(sample, []byte("ftypmp42"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "02-Clip.mkv"), []byte("mkv"), 0o600)

	p := NewPlayer(config.DefaultPureVideoConfig(), PlayerCallbacks{
		Headless:       true,
		OnOpenSettings: func() {},
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

	const winW, winH = 960, 600
	tt := ui.NewTester(p.View, winW, winH)
	tt.SetDark(true)
	tt.Frame()
	checkEnglishVideoView(t, tt, winW, "purevideo_empty")
	tt.SetDark(false)
	tt.Frame()
	checkEnglishVideoView(t, tt, winW, "")

	p.Open(sample)
	p.pos = 1*time.Minute + 12*time.Second
	p.dur = 3*time.Minute + 45*time.Second
	p.natW, p.natH = 3840, 2160
	tt.SetDark(true)
	tt.Frame()
	checkEnglishVideoView(t, tt, winW, "purevideo_playing")
	tt.SetDark(false)
	tt.Frame()
	checkEnglishVideoView(t, tt, winW, "")

	p.setError("Unsupported codec or missing libmpv-2.dll")
	tt.SetDark(true)
	tt.Frame()
	checkEnglishVideoView(t, tt, winW, "purevideo_error")
	tt.SetDark(false)
	tt.Frame()
	checkEnglishVideoView(t, tt, winW, "")
}

func TestChinesePureVideoLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	dir := t.TempDir()
	sample := filepath.Join(dir, "PureUtils-Showreel-4K.mp4")
	if err := os.WriteFile(sample, []byte("ftypmp42"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "02-Clip.mkv"), []byte("mkv"), 0o600)

	p := NewPlayer(config.DefaultPureVideoConfig(), PlayerCallbacks{
		Headless:       true,
		OnOpenSettings: func() {},
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

	const winW, winH = 960, 600
	tt := ui.NewTester(p.View, winW, winH)
	tt.SetDark(true)
	tt.Frame()
	checkLayoutBounds(t, tt, winW, "purevideo_empty_zh", false)
	tt.SetDark(false)
	tt.Frame()
	checkLayoutBounds(t, tt, winW, "", false)

	p.Open(sample)
	p.pos = 1*time.Minute + 12*time.Second
	p.dur = 3*time.Minute + 45*time.Second
	p.natW, p.natH = 3840, 2160
	tt.SetDark(true)
	tt.Frame()
	checkLayoutBounds(t, tt, winW, "purevideo_playing_zh", false)
	tt.SetDark(false)
	tt.Frame()
	checkLayoutBounds(t, tt, winW, "", false)
}
