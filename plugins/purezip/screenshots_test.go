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

	"github.com/egoist/mygo/ui"

	"pureutils/plugins/purezip/archive"
)

func saveZipShowcasePNG(t *testing.T, img *image.RGBA, relPath string) {
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

func TestGeneratePureZipScreenshots(t *testing.T) {
	if os.Getenv("PUREUTILS_GEN_SHOTS") != "1" {
		t.Skip("set PUREUTILS_GEN_SHOTS=1 to regenerate documentation screenshots")
	}

	p, _ := newTestPlugin(t)
	tmp := t.TempDir()
	stageRoot := filepath.Join(tmp, "PureUtils-v1.0-release")
	_ = os.MkdirAll(filepath.Join(stageRoot, "docs", "screenshots"), 0o755)
	_ = os.MkdirAll(filepath.Join(stageRoot, "plugins", "pureclip"), 0o755)
	_ = os.MkdirAll(filepath.Join(stageRoot, "plugins", "pureshot"), 0o755)
	_ = os.MkdirAll(filepath.Join(stageRoot, "resources"), 0o755)

	_ = os.WriteFile(filepath.Join(stageRoot, "README.md"), []byte("# PureUtils\nModular native Windows utility suite.\n"), 0o644)
	_ = os.WriteFile(filepath.Join(stageRoot, "LICENSE"), []byte("GNU AFFERO GENERAL PUBLIC LICENSE v3.0\n"), 0o644)
	_ = os.WriteFile(filepath.Join(stageRoot, "go.mod"), []byte("module pureutils\ngo 1.27.3\n"), 0o644)
	_ = os.WriteFile(filepath.Join(stageRoot, "docs", "ARCHITECTURE.md"), []byte("# Architecture\n"), 0o644)
	_ = os.WriteFile(filepath.Join(stageRoot, "plugins", "pureclip", "repo.go"), []byte("package pureclip\n"), 0o644)
	_ = os.WriteFile(filepath.Join(stageRoot, "plugins", "pureshot", "plugin.go"), []byte("package pureshot\n"), 0o644)
	_ = os.WriteFile(filepath.Join(stageRoot, "resources", "icon.png"), make([]byte, 4096), 0o644)

	arcPath := filepath.Join(tmp, "PureUtils-v1.0-bundle.zip")
	if err := archive.Create(context.Background(), arcPath, []string{stageRoot}, archive.CreateOptions{
		Format: archive.FormatZip,
	}); err != nil {
		t.Fatal(err)
	}

	v := newZipView(p)
	t.Cleanup(func() { v.closed = true; v.closeArchive() })
	tt := ui.NewTester(v.view, 920, 580)
	v.load(arcPath, "")
	tt.SetDark(true)
	tt.Frame()

	v.navigate("PureUtils-v1.0-release")
	tt.Frame()
	tt.Advance(500 * time.Millisecond)

	saveZipShowcasePNG(t, tt.Image(), filepath.Join("..", "..", "docs", "screenshots", "zip.png"))
}
