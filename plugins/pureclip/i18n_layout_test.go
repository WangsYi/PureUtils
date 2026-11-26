package pureclip

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

func checkEnglishClipView(t *testing.T, tt *ui.Tester, winW float32, shotName string) {
	t.Helper()
	checkLayoutBounds(t, tt, winW, shotName, true)
}

func seedClipLayoutEntries(t *testing.T, h *mockHost, p *Plugin) *historyView {
	t.Helper()
	_ = h.io.Write(&core.ClipboardContent{
		Text: "PureUtils Modular Desktop Suite Architecture Notes",
		HTML: "<b>PureUtils</b> Modular Desktop Suite Architecture Notes",
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

	_ = h.clip.WriteImage(makeShowcasePNG(t, 420, 240), "PureShot.exe")

	hv := newHistoryView(p)
	if len(hv.items) > 1 {
		_, _ = p.Service().ToggleFavorite(hv.items[len(hv.items)-1].ID)
		hv.Reload()
	}
	return hv
}

func TestEnglishPureClipLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	h := newTestHost(t, nil)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	hv := seedClipLayoutEntries(t, h, p)

	et := ui.NewTester(hv.view, historyWidth, historyHeight)
	et.SetDark(true)
	et.Frame()
	checkEnglishClipView(t, et, historyWidth, "pureclip_expanded")
	et.SetDark(false)
	et.Frame()
	checkEnglishClipView(t, et, historyWidth, "")

	hv.query = "nomatch_xyz"
	hv.applyFilter(false)
	et.SetDark(true)
	et.Frame()
	checkEnglishClipView(t, et, historyWidth, "pureclip_empty")
	et.SetDark(false)
	et.Frame()
	checkEnglishClipView(t, et, historyWidth, "")

	cv := newCompactHistoryView(p)
	ct := ui.NewTester(cv.view, compactHistoryWidth, compactHistoryHeight)
	ct.SetDark(true)
	ct.Frame()
	checkEnglishClipView(t, ct, compactHistoryWidth, "pureclip_compact")
	ct.SetDark(false)
	ct.Frame()
	checkEnglishClipView(t, ct, compactHistoryWidth, "")
}

func TestChinesePureClipLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	h := newTestHost(t, nil)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Stop() }()

	hv := seedClipLayoutEntries(t, h, p)

	et := ui.NewTester(hv.view, historyWidth, historyHeight)
	et.SetDark(true)
	et.Frame()
	checkLayoutBounds(t, et, historyWidth, "pureclip_expanded_zh", false)
	et.SetDark(false)
	et.Frame()
	checkLayoutBounds(t, et, historyWidth, "", false)

	cv := newCompactHistoryView(p)
	ct := ui.NewTester(cv.view, compactHistoryWidth, compactHistoryHeight)
	ct.SetDark(true)
	ct.Frame()
	checkLayoutBounds(t, ct, compactHistoryWidth, "pureclip_compact_zh", false)
	ct.SetDark(false)
	ct.Frame()
	checkLayoutBounds(t, ct, compactHistoryWidth, "", false)
}
