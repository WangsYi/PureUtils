//go:build windows

package zipshell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssocExtensionsAndIconValue(t *testing.T) {
	exts := AssocExtensions()
	want := []string{"zip", "7z", "rar", "tar", "gz", "tgz", "bz2", "tbz", "tbz2", "xz", "txz", "zst", "tzst"}
	if len(exts) != len(want) {
		t.Fatalf("AssocExtensions len=%d (%v), want %d", len(exts), exts, len(want))
	}
	for i := range want {
		if exts[i] != want[i] {
			t.Fatalf("AssocExtensions[%d]=%q, want %q", i, exts[i], want[i])
		}
	}
	if !strings.Contains(DefaultAppsURI, "registeredAppUser=PureZip") {
		t.Fatalf("DefaultAppsURI=%q", DefaultAppsURI)
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "PureUtils.exe")
	if got := iconValue(exe); got != exe+",0" {
		t.Fatalf("fallback iconValue=%q", got)
	}
	res := filepath.Join(dir, "resources")
	if err := os.MkdirAll(res, 0o755); err != nil {
		t.Fatal(err)
	}
	ico := filepath.Join(res, "zip.ico")
	if err := os.WriteFile(ico, []byte("ico"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := iconValue(exe); got != ico+",0" {
		t.Fatalf("ico iconValue=%q", got)
	}
}

func TestInstallerScriptCoversZipComponentAndExtensions(t *testing.T) {
	iss := filepath.Join("..", "..", "..", "installer", "pureutils.iss")
	raw, err := os.ReadFile(iss)
	if err != nil {
		t.Fatalf("read %s: %v", iss, err)
	}
	src := string(raw)
	for _, needle := range []string{
		`Name: "zip";`,
		`Name: "zip\contextmenu";`,
		`Name: "zip\assoc";`,
		`RegisterZipContextMenu`,
		`CleanupOldZipAssociations`,
	} {
		if !strings.Contains(src, needle) {
			t.Errorf("pureutils.iss missing %q", needle)
		}
	}
	for _, ext := range AssocExtensions() {
		dot := "." + ext
		for _, want := range []string{
			`PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: "` + dot + `"`,
			`Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: "` + dot + `"`,
			`Software\Classes\` + dot + `\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"`,
		} {
			if !strings.Contains(src, want) {
				t.Errorf("pureutils.iss missing %s entry %s", dot, want)
			}
		}
	}
}
