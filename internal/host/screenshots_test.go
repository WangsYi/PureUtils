package host

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
)

func saveHostShowcasePNG(t *testing.T, img *image.RGBA, relPath string) {
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

func TestGenerateHostScreenshots(t *testing.T) {
	if os.Getenv("PUREUTILS_GEN_SHOTS") != "1" {
		t.Skip("set PUREUTILS_GEN_SHOTS=1 to regenerate documentation screenshots")
	}

	a := testHostWithModules(t, CompiledModules(), nil, nil)
	a.Start([]string{"--background"})

	// Sanitize displayed data path so screenshots never expose local usernames.
	a.paths.Home = `%LOCALAPPDATA%\PureUtils`

	sv := newSettingsView(a, "")
	st := ui.NewTester(sv.view, 920, 660)
	st.SetDark(true)
	st.Frame()
	st.Advance(500 * time.Millisecond)
	saveHostShowcasePNG(t, st.Image(), filepath.Join("..", "..", "docs", "screenshots", "settings.png"))

	lv := newLauncherView(a, purekit.MatchContext{
		SelectedFiles: []string{`D:\Releases\PureUtils-v1.0-bundle.zip`},
		Clipboard:     &core.ClipboardContent{ImagePNG: encodeTinyPNG(t)},
	})
	lt := ui.NewTester(lv.view, launcherWidth, launcherHeight)
	lt.SetDark(true)
	lt.Frame()
	lt.Advance(500 * time.Millisecond)
	saveHostShowcasePNG(t, lt.Image(), filepath.Join("..", "..", "docs", "screenshots", "launcher.png"))
}
