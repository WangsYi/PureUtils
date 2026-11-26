package pureshot

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"unicode"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/pkg/purekit/platform"
	"pureutils/plugins/pureshot/annotate"
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

func checkEnglishShotView(t *testing.T, tt *ui.Tester, winW float32, shotName string) {
	t.Helper()
	checkLayoutBounds(t, tt, winW, shotName, true)
}

func newLayoutCaptureSession(p *Plugin, w, hPx int) *captureSession {
	frame := makeShowcaseDesktopFrame(w, hPx)
	doc := annotate.NewDocument(720, 420)
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
		Tool:          annotate.ToolText,
		Color:         core.ARGB(0xFFF8FAFC),
		Origin:        core.Point{X: 306, Y: 72},
		Text:          "PureShot Annotation Preview",
		FontSize:      22,
		HasBackground: true,
		Background:    core.ARGB(0xE00E1118),
	})

	mag := image.NewRGBA(image.Rect(0, 0, 120, 120))
	sess := &captureSession{
		plugin:     p,
		frame:      frame,
		monitor:    platform.Monitor{Bounds: core.RectI{X: 0, Y: 0, W: w, H: hPx}},
		contentW:   float64(w),
		contentH:   float64(hPx),
		selection:  core.RectI{X: 220, Y: 90, W: 720, H: 420},
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
	return sess
}

func TestEnglishPureShotLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	h := newTestHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	const w, hPx = 1280, 720
	sess := newLayoutCaptureSession(p, w, hPx)

	tt := ui.NewTester(sess.view, w, hPx)
	tt.SetDark(true)
	tt.Frame()
	checkEnglishShotView(t, tt, w, "pureshot_overlay")
	tt.SetDark(false)
	tt.Frame()
	checkEnglishShotView(t, tt, w, "")

	sess.tool = annotate.ToolText
	sess.textEditing = true
	sess.textOrigin = core.Point{X: 300, Y: 220}
	sess.textValue = "Sample annotation"
	tt.SetDark(true)
	tt.Frame()
	checkEnglishShotView(t, tt, w, "pureshot_overlay_text")
	tt.SetDark(false)
	tt.Frame()
	checkEnglishShotView(t, tt, w, "")

	pin := &pinWindow{
		plugin:      p,
		text:        "Pinned Release Checklist:\n- Verify EN layout bounds\n- Inspect rendered PNGs",
		baseW:       320,
		baseH:       180,
		zoom:        1.0,
		width:       320,
		height:      180,
		scale:       1.0,
		opacity:     0.9,
		alwaysOnTop: true,
		pinHovered:  true,
		hudHovered:  true,
	}
	pt := ui.NewTester(pin.view, 320, 220)
	pt.SetDark(true)
	pt.Frame()
	checkEnglishShotView(t, pt, 320, "pureshot_pin")
	pt.SetDark(false)
	pt.Frame()
	checkEnglishShotView(t, pt, 320, "")

	hudW, hudH := pin.hudSize()
	ht := ui.NewTester(pin.hudView, hudW, hudH)
	ht.SetDark(true)
	ht.Frame()
	checkEnglishShotView(t, ht, float32(hudW), "pureshot_pin_hud")
	ht.SetDark(false)
	ht.Frame()
	checkEnglishShotView(t, ht, float32(hudW), "")
}

func TestChinesePureShotLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	h := newTestHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	const w, hPx = 1280, 720
	sess := newLayoutCaptureSession(p, w, hPx)

	tt := ui.NewTester(sess.view, w, hPx)
	tt.SetDark(true)
	tt.Frame()
	checkLayoutBounds(t, tt, w, "pureshot_overlay_zh", false)
	tt.SetDark(false)
	tt.Frame()
	checkLayoutBounds(t, tt, w, "", false)

	pin := &pinWindow{
		plugin:      p,
		text:        "贴图验证清单：\n- 检查中文与英文边界\n- 验证深浅色渲染正常",
		baseW:       320,
		baseH:       180,
		zoom:        1.0,
		width:       320,
		height:      180,
		scale:       1.0,
		opacity:     0.9,
		alwaysOnTop: true,
		pinHovered:  true,
		hudHovered:  true,
	}
	pt := ui.NewTester(pin.view, 320, 220)
	pt.SetDark(true)
	pt.Frame()
	checkLayoutBounds(t, pt, 320, "pureshot_pin_zh", false)
	pt.SetDark(false)
	pt.Frame()
	checkLayoutBounds(t, pt, 320, "", false)
}
