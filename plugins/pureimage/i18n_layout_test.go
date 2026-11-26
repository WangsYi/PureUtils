package pureimage

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
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

func checkEnglishImageView(t *testing.T, tt *ui.Tester, winW float32, shotName string) {
	t.Helper()
	checkLayoutBounds(t, tt, winW, shotName, true)
}

func TestEnglishPureImageLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	dir := t.TempDir()
	jpgPath := filepath.Join(dir, "dusk-ridge.jpg")
	if err := os.WriteFile(jpgPath, encodeJPEGWithExif(t, renderShowcaseLandscape(1280, 800)), 0o600); err != nil {
		t.Fatal(err)
	}

	prefs := config.DefaultPureImageConfig()
	prefs.ShowDetails = true
	v := New(prefs, Callbacks{
		Headless: true,
		CanPin:   func() bool { return true },
	})
	defer v.Shutdown()

	const winW, winH = 1000, 680
	tt := ui.NewTester(v.View, winW, winH)
	tt.SetDark(true)
	tt.Frame()
	checkEnglishImageView(t, tt, winW, "pureimage_empty")
	tt.SetDark(false)
	tt.Frame()
	checkEnglishImageView(t, tt, winW, "")

	v.Open(jpgPath)
	v.showHUD()
	tt.SetDark(true)
	tt.Frame()
	checkEnglishImageView(t, tt, winW, "pureimage_loaded_hud")
	tt.SetDark(false)
	tt.Frame()
	checkEnglishImageView(t, tt, winW, "")

	v.settingsOpen = true
	v.sortOpen = false
	tt.SetDark(true)
	tt.Frame()
	checkEnglishImageView(t, tt, winW, "pureimage_settings_popover")
	tt.SetDark(false)
	tt.Frame()
	checkEnglishImageView(t, tt, winW, "")

	v.settingsOpen = false
	v.sortOpen = true
	tt.SetDark(true)
	tt.Frame()
	checkEnglishImageView(t, tt, winW, "pureimage_sort_popover")
	tt.SetDark(false)
	tt.Frame()
	checkEnglishImageView(t, tt, winW, "")
}

func TestChinesePureImageLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	dir := t.TempDir()
	jpgPath := filepath.Join(dir, "dusk-ridge.jpg")
	if err := os.WriteFile(jpgPath, encodeJPEGWithExif(t, renderShowcaseLandscape(1280, 800)), 0o600); err != nil {
		t.Fatal(err)
	}

	prefs := config.DefaultPureImageConfig()
	prefs.ShowDetails = true
	v := New(prefs, Callbacks{
		Headless: true,
		CanPin:   func() bool { return true },
	})
	defer v.Shutdown()

	const winW, winH = 1000, 680
	tt := ui.NewTester(v.View, winW, winH)
	v.Open(jpgPath)
	v.showHUD()
	tt.SetDark(true)
	tt.Frame()
	checkLayoutBounds(t, tt, winW, "pureimage_loaded_hud_zh", false)
	tt.SetDark(false)
	tt.Frame()
	checkLayoutBounds(t, tt, winW, "", false)

	v.settingsOpen = true
	v.sortOpen = false
	tt.SetDark(true)
	tt.Frame()
	checkLayoutBounds(t, tt, winW, "pureimage_settings_popover_zh", false)
	tt.SetDark(false)
	tt.Frame()
	checkLayoutBounds(t, tt, winW, "", false)
}
