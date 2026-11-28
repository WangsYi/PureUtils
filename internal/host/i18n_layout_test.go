package host

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
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

func checkLayoutBounds(t *testing.T, tt *ui.Tester, winW float32, shotName string, forbidHan bool, allowTexts ...string) {
	t.Helper()
	texts := tt.Texts()
	if len(texts) == 0 {
		t.Errorf("%s: rendered view has no text elements", shotName)
	}
	img := tt.Image()
	assertNonBlankFrame(t, img, shotName)

	for _, txt := range texts {
		if forbidHan && !slices.Contains(allowTexts, txt) {
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
		saveHostTestPNG(t, img, dir, shotName)
	}
}

func checkEnglishHostView(t *testing.T, tt *ui.Tester, winW float32, shotName string) {
	t.Helper()
	checkLayoutBounds(t, tt, winW, shotName, true, "简体中文")
}

func saveHostTestPNG(t *testing.T, img *image.RGBA, dir, shotName string) {
	t.Helper()
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

func TestEnglishSettingsLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	a := testHostWithModules(t, CompiledModules(), nil, nil)
	a.Start([]string{"--background"})
	a.paths.Home = `%LOCALAPPDATA%\PureUtils`

	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	sv := newSettingsView(a, "general")
	for _, size := range []struct {
		w, h int
		tag  string
	}{
		{860, 640, "860"},
		{720, 540, "720"},
	} {
		tt := ui.NewTester(sv.view, size.w, size.h)
		for _, sec := range sv.sections {
			sv.section = sec.id
			name := fmt.Sprintf("settings_%s_%s", sec.id, size.tag)
			t.Run(name, func(t *testing.T) {
				tt.SetDark(true)
				tt.Frame()
				checkEnglishHostView(t, tt, float32(size.w), name)

				tt.SetDark(false)
				tt.Frame()
				checkEnglishHostView(t, tt, float32(size.w), "")
			})
			if size.tag == "720" && (sec.id == "general" || sec.id == "advanced") {
				tt.SetDark(true)
				tt.Scroll(450, 300, 0, 520)
				tt.Frame()
				bName := fmt.Sprintf("settings_%s_bottom_720", sec.id)
				t.Run(bName, func(t *testing.T) {
					checkEnglishHostView(t, tt, float32(size.w), bName)
					tt.SetDark(false)
					tt.Frame()
					checkEnglishHostView(t, tt, float32(size.w), "")
				})
				tt.SetDark(true)
				tt.Scroll(450, 300, 0, -520)
				tt.Frame()
			}
		}
	}
}

func TestChineseSettingsLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	a := testHostWithModules(t, CompiledModules(), nil, nil)
	a.Start([]string{"--background"})
	a.paths.Home = `%LOCALAPPDATA%\PureUtils`

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	sv := newSettingsView(a, "general")
	for _, size := range []struct {
		w, h int
		tag  string
	}{
		{860, 640, "860"},
		{720, 540, "720"},
	} {
		tt := ui.NewTester(sv.view, size.w, size.h)
		for _, sec := range sv.sections {
			sv.section = sec.id
			name := fmt.Sprintf("settings_%s_%s_zh", sec.id, size.tag)
			t.Run(name, func(t *testing.T) {
				tt.SetDark(true)
				tt.Frame()
				checkLayoutBounds(t, tt, float32(size.w), name, false)

				tt.SetDark(false)
				tt.Frame()
				checkLayoutBounds(t, tt, float32(size.w), "", false)
			})
			if size.tag == "720" && (sec.id == "general" || sec.id == "advanced") {
				tt.SetDark(true)
				tt.Scroll(450, 300, 0, 520)
				tt.Frame()
				bName := fmt.Sprintf("settings_%s_bottom_720_zh", sec.id)
				t.Run(bName, func(t *testing.T) {
					checkLayoutBounds(t, tt, float32(size.w), bName, false)
					tt.SetDark(false)
					tt.Frame()
					checkLayoutBounds(t, tt, float32(size.w), "", false)
				})
				tt.SetDark(true)
				tt.Scroll(450, 300, 0, -520)
				tt.Frame()
			}
		}
	}
}

func TestSettingsLiveLanguageSwitch(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	a := testHostWithModules(t, CompiledModules(), nil, nil)
	a.Start([]string{"--background"})
	hc := a.HostConfig()
	hc.Appearance.Language = i18n.PrefAuto
	_ = a.SaveHostConfig(hc)
	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	sv := newSettingsView(a, "general")
	tt := ui.NewTester(sv.view, 860, 640)
	tt.SetDark(true)
	tt.Frame()

	if !tt.HasText("常规与外观") {
		t.Fatalf("expected Chinese heading before switch, got %v", tt.Texts())
	}

	if err := tt.Click("English"); err != nil {
		t.Fatalf("click English option: %v", err)
	}
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("click Save (ZH): %v", err)
	}
	tt.Frame()

	if got := i18n.Current(); got != i18n.LangEN {
		t.Fatalf("i18n.Current() = %q, want %q", got, i18n.LangEN)
	}
	if !tt.HasText("General & Appearance") {
		t.Fatalf("expected English heading after live switch, got %v", tt.Texts())
	}

	if err := tt.Click("简体中文"); err != nil {
		t.Fatalf("click 简体中文 option: %v", err)
	}
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatalf("click Save (EN): %v", err)
	}
	tt.Frame()

	if got := i18n.Current(); got != i18n.LangZH {
		t.Fatalf("i18n.Current() = %q, want %q", got, i18n.LangZH)
	}
	if !tt.HasText("常规与外观") {
		t.Fatalf("expected Chinese heading restored, got %v", tt.Texts())
	}
}

func TestEnglishLauncherLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	a := testHostWithModules(t, CompiledModules(), nil, nil)
	a.Start([]string{"--background"})

	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	lv := newLauncherView(a, purekit.MatchContext{
		SelectedFiles: []string{`D:\Workspaces\VeryLongProjectFolderName\PureUtils-Release-2026-x64-Bundle.zip`},
		Clipboard:     &core.ClipboardContent{ImagePNG: encodeTinyPNG(t)},
	})
	lt := ui.NewTester(lv.view, launcherWidth, launcherHeight)
	lt.SetDark(true)
	lt.Frame()
	checkEnglishHostView(t, lt, launcherWidth, "launcher_en")

	lt.SetDark(false)
	lt.Frame()
	checkEnglishHostView(t, lt, launcherWidth, "")
}

func TestChineseLauncherLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	a := testHostWithModules(t, CompiledModules(), nil, nil)
	a.Start([]string{"--background"})

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	lv := newLauncherView(a, purekit.MatchContext{
		SelectedFiles: []string{`D:\Workspaces\VeryLongProjectFolderName\PureUtils-Release-2026-x64-Bundle.zip`},
		Clipboard:     &core.ClipboardContent{ImagePNG: encodeTinyPNG(t)},
	})
	lt := ui.NewTester(lv.view, launcherWidth, launcherHeight)
	lt.SetDark(true)
	lt.Frame()
	checkLayoutBounds(t, lt, launcherWidth, "launcher_zh", false)

	lt.SetDark(false)
	lt.Frame()
	checkLayoutBounds(t, lt, launcherWidth, "", false)
}

func TestFluentVibrancyAndTitleBarLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	a := testHostWithModules(t, CompiledModules(), nil, nil)
	a.Start([]string{"--background"})
	a.paths.Home = `%LOCALAPPDATA%\PureUtils`

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	sv := newSettingsView(a, "general")
	for _, size := range []struct {
		w, h int
	}{
		{860, 640},
		{720, 540},
	} {
		tt := ui.NewTester(sv.view, size.w, size.h)
		tt.SetVibrancy(true)
		tt.SetTitleBar(ui.TitleBar{Height: 40, Right: 138})
		for _, dark := range []bool{true, false} {
			tt.SetDark(dark)
			tt.Frame()
			checkLayoutBounds(t, tt, float32(size.w), "", false)

			if err := tt.Click("高级"); err != nil {
				t.Fatalf("%dx%d dark=%v click 高级: %v", size.w, size.h, dark, err)
			}
			tt.Frame()
			if !tt.HasText("高级与维护") {
				t.Fatalf("%dx%d dark=%v expected 高级与维护 heading, got %v", size.w, size.h, dark, tt.Texts())
			}
			checkLayoutBounds(t, tt, float32(size.w), "", false)

			if err := tt.Click("常规"); err != nil {
				t.Fatalf("%dx%d dark=%v click 常规: %v", size.w, size.h, dark, err)
			}
			tt.Frame()
			checkLayoutBounds(t, tt, float32(size.w), "", false)
		}
		if r, ok := tt.Find("PureUtils 设置"); !ok || r.X+r.W > float32(size.w-138) {
			t.Fatalf("%dx%d title overlapped caption buttons: ok=%v rect=%+v", size.w, size.h, ok, r)
		}
	}

	lv := newLauncherView(a, purekit.MatchContext{
		SelectedFiles: []string{`D:\Workspaces\Project\PureUtils-Bundle.zip`},
		Clipboard:     &core.ClipboardContent{ImagePNG: encodeTinyPNG(t)},
	})
	lt := ui.NewTester(lv.view, launcherWidth, launcherHeight)
	lt.SetVibrancy(true)
	for _, dark := range []bool{true, false} {
		lt.SetDark(dark)
		lt.Frame()
		lt.Key(0, ui.KeyDown)
		lt.Frame()
		checkLayoutBounds(t, lt, launcherWidth, "", false)
	}
	if err := lt.Click("清除上下文"); err != nil {
		t.Fatalf("click 清除上下文: %v", err)
	}
	lt.Frame()
	if lv.hasContext() {
		t.Fatal("expected launcher context cleared")
	}
	checkLayoutBounds(t, lt, launcherWidth, "", false)
}
