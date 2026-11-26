package purezip

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/purezip/archive"
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

func checkEnglishZipView(t *testing.T, tt *ui.Tester, winW float32, shotName string) {
	t.Helper()
	checkLayoutBounds(t, tt, winW, shotName, true)
}

func setupZipLayoutSample(t *testing.T) (*Plugin, string, string, string) {
	t.Helper()
	p, _ := newTestPlugin(t)
	tmp := t.TempDir()
	stageRoot := filepath.Join(tmp, "PureUtils-v1.0-release")
	_ = os.MkdirAll(filepath.Join(stageRoot, "docs"), 0o755)
	_ = os.MkdirAll(filepath.Join(stageRoot, "src"), 0o755)

	readmePath := filepath.Join(stageRoot, "README.md")
	_ = os.WriteFile(readmePath, []byte("# PureUtils\nEnglish release bundle notes.\n"), 0o644)
	_ = os.WriteFile(filepath.Join(stageRoot, "docs", "architecture.md"), []byte("# Architecture\n"), 0o644)
	_ = os.WriteFile(filepath.Join(stageRoot, "src", "main.go"), []byte("package main\n"), 0o644)

	arcPath := filepath.Join(tmp, "PureUtils-v1.0-bundle.zip")
	if err := archive.Create(context.Background(), arcPath, []string{stageRoot}, archive.CreateOptions{
		Format: archive.FormatZip,
	}); err != nil {
		t.Fatal(err)
	}
	return p, tmp, readmePath, arcPath
}

func TestEnglishPureZipLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	p, tmp, readmePath, arcPath := setupZipLayoutSample(t)

	v := newZipView(p)
	t.Cleanup(func() { v.closed = true; v.closeArchive() })

	// 1. Start page (1000x660)
	vt := ui.NewTester(v.view, 1000, 660)
	vt.SetDark(true)
	vt.Frame()
	checkEnglishZipView(t, vt, 1000, "purezip_start")
	vt.SetDark(false)
	vt.Frame()
	checkEnglishZipView(t, vt, 1000, "")

	// 2. Loaded archive (1000x660)
	v.load(arcPath, "")
	v.navigate("PureUtils-v1.0-release")
	vt.SetDark(true)
	vt.Frame()
	checkEnglishZipView(t, vt, 1000, "purezip_loaded_1000")
	vt.SetDark(false)
	vt.Frame()
	checkEnglishZipView(t, vt, 1000, "")

	// 3. Info modal (1000x660)
	v.infoOpen = true
	vt.SetDark(true)
	vt.Frame()
	checkEnglishZipView(t, vt, 1000, "purezip_info_modal")
	vt.SetDark(false)
	vt.Frame()
	checkEnglishZipView(t, vt, 1000, "")
	v.infoOpen = false

	// 4. Password prompt modal (1000x660)
	v.requestPassword(true, nil)
	vt.SetDark(true)
	vt.Frame()
	checkEnglishZipView(t, vt, 1000, "purezip_pw_modal")
	vt.SetDark(false)
	vt.Frame()
	checkEnglishZipView(t, vt, 1000, "")
	v.pwPrompt = false
	v.pwRetry = nil

	// 5. Loaded archive at compact min width (720x460)
	vt.SetSize(720, 460)
	vt.SetDark(true)
	vt.Frame()
	checkEnglishZipView(t, vt, 720, "purezip_loaded_720")
	vt.SetDark(false)
	vt.Frame()
	checkEnglishZipView(t, vt, 720, "")

	// 6. Compress dialog (600x640)
	cd := newCompressDialog(p, []string{readmePath}, filepath.Join(tmp, "out.7z"))
	cdt := ui.NewTester(cd.view, 600, 640)
	cdt.SetDark(true)
	cdt.Frame()
	checkEnglishZipView(t, cdt, 600, "purezip_compress_dialog")
	cdt.SetDark(false)
	cdt.Frame()
	checkEnglishZipView(t, cdt, 600, "")

	// 7. Extract dialog (560x470)
	ed := newExtractDialog(p, []string{arcPath}, nil, "", "")
	edt := ui.NewTester(ed.view, 560, 470)
	edt.SetDark(true)
	edt.Frame()
	checkEnglishZipView(t, edt, 560, "purezip_extract_dialog")
	edt.SetDark(false)
	edt.Frame()
	checkEnglishZipView(t, edt, 560, "")

	// 8. Job progress & conflict dialog (480x268)
	j := newZipJob(p, "compress", "release-bundle.7z")
	j.report(archive.Progress{Current: "docs/architecture.md", DoneBytes: 512, TotalBytes: 1024, DoneFiles: 3, TotalFiles: 6})
	jt := ui.NewTester(j.view, 480, 268)
	jt.SetDark(true)
	jt.Frame()
	checkEnglishZipView(t, jt, 480, "purezip_job_progress")
	jt.SetDark(false)
	jt.Frame()
	checkEnglishZipView(t, jt, 480, "")

	fi, _ := os.Stat(readmePath)
	j.prompt = &zipPrompt{
		kind:     zipPromptConflict,
		title:    i18n.S("文件已存在", "File Already Exists"),
		message:  readmePath,
		existing: fi,
		incoming: archive.Entry{Name: "README.md", Size: 2048, Modified: time.Now()},
	}
	jt.SetDark(true)
	jt.Frame()
	checkEnglishZipView(t, jt, 480, "purezip_job_conflict")
	jt.SetDark(false)
	jt.Frame()
	checkEnglishZipView(t, jt, 480, "")
}

func TestChinesePureZipLayout(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	p, tmp, readmePath, arcPath := setupZipLayoutSample(t)

	v := newZipView(p)
	t.Cleanup(func() { v.closed = true; v.closeArchive() })

	vt := ui.NewTester(v.view, 1000, 660)
	v.load(arcPath, "")
	v.navigate("PureUtils-v1.0-release")
	vt.SetDark(true)
	vt.Frame()
	checkLayoutBounds(t, vt, 1000, "purezip_loaded_1000_zh", false)
	vt.SetDark(false)
	vt.Frame()
	checkLayoutBounds(t, vt, 1000, "", false)

	vt.SetSize(720, 460)
	vt.SetDark(true)
	vt.Frame()
	checkLayoutBounds(t, vt, 720, "purezip_loaded_720_zh", false)
	vt.SetDark(false)
	vt.Frame()
	checkLayoutBounds(t, vt, 720, "", false)

	cd := newCompressDialog(p, []string{readmePath}, filepath.Join(tmp, "out.7z"))
	cdt := ui.NewTester(cd.view, 600, 640)
	cdt.SetDark(true)
	cdt.Frame()
	checkLayoutBounds(t, cdt, 600, "purezip_compress_dialog_zh", false)
	cdt.SetDark(false)
	cdt.Frame()
	checkLayoutBounds(t, cdt, 600, "", false)

	ed := newExtractDialog(p, []string{arcPath}, nil, "", "")
	edt := ui.NewTester(ed.view, 560, 470)
	edt.SetDark(true)
	edt.Frame()
	checkLayoutBounds(t, edt, 560, "purezip_extract_dialog_zh", false)
	edt.SetDark(false)
	edt.Frame()
	checkLayoutBounds(t, edt, 560, "", false)
}
