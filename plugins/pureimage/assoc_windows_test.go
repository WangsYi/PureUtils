//go:build windows

package pureimage

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewerEmbeddedIconPNG(t *testing.T) {
	if len(viewerIconPNG) == 0 {
		t.Fatal("expected viewerIconPNG to be embedded")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(viewerIconPNG))
	if err != nil {
		t.Fatalf("decode viewerIconPNG: %v", err)
	}
	if cfg.Width != 256 || cfg.Height != 256 {
		t.Fatalf("viewerIconPNG dimensions = %dx%d, want 256x256", cfg.Width, cfg.Height)
	}
}

func TestAssocIconValueAndDefaultAppsURI(t *testing.T) {
	if !strings.Contains(DefaultAppsURI, "registeredAppUser=PureImage") {
		t.Fatalf("DefaultAppsURI = %q, expected registeredAppUser=PureImage", DefaultAppsURI)
	}

	dir := t.TempDir()
	resDir := filepath.Join(dir, "resources")
	if err := os.MkdirAll(resDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "PureUtils.exe")
	fallback := assocIconValue(exe)
	if fallback != exe+",0" || strings.Contains(fallback, `"`) {
		t.Fatalf("fallback assocIconValue = %q, want unquoted %q", fallback, exe+",0")
	}

	ico := filepath.Join(resDir, "image.ico")
	if err := os.WriteFile(ico, []byte("ico"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := assocIconValue(exe)
	if got != ico+",0" || strings.Contains(got, `"`) {
		t.Fatalf("ico assocIconValue = %q, want unquoted %q", got, ico+",0")
	}
}

func TestInstallerScriptCoversAllImageExtensions(t *testing.T) {
	issPath := filepath.Join("..", "..", "installer", "pureutils.iss")
	raw, err := os.ReadFile(issPath)
	if err != nil {
		t.Fatalf("read %s: %v", issPath, err)
	}
	src := string(raw)

	for _, needle := range []string{
		"ChangesAssociations=yes",
		`Software\Classes\Applications\PureUtils.exe`,
	} {
		if !strings.Contains(src, needle) {
			t.Errorf("pureutils.iss missing %q", needle)
		}
	}

	for _, ext := range ImageExtList {
		dotExt := "." + ext
		checks := []string{
			`Capabilities\FileAssociations"; ValueType: string; ValueName: "` + dotExt + `"`,
			`Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: "` + dotExt + `"`,
			`Software\Classes\` + dotExt + `\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"`,
		}
		for _, want := range checks {
			if !strings.Contains(src, want) {
				t.Errorf("pureutils.iss missing registry entry for %s: %s", dotExt, want)
			}
		}
	}
}
