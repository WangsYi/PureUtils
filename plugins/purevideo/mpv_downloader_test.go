//go:build windows

package purevideo

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildFakeLibMPVDLLBytes(size int) []byte {
	if size < 1024 {
		size = 1024
	}
	b := make([]byte, size)
	b[0] = 'M'
	b[1] = 'Z'
	return b
}

func buildFakeLibMPVZipBytes(t *testing.T, dllBytes []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("x86_64/libmpv-2.dll")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(dllBytes); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMPVManagerDownloadFallbackImportAndRemove(t *testing.T) {
	dllPayload := buildFakeLibMPVDLLBytes(8192)
	zipPayload := buildFakeLibMPVZipBytes(t, dllPayload)

	var srvURL string
	var ghproxyHits, ghfastHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/latest":
			fmt.Fprintf(w, `{"tag_name":"20261111","assets":[{"name":"mpv-dev-x86_64-20261111-git-test.zip","browser_download_url":"%s/dl/mpv-dev-x86_64-20261111-git-test.zip"}]}`, srvURL)
		case "/ghproxy/" + srvURL + "/dl/mpv-dev-x86_64-20261111-git-test.zip":
			ghproxyHits++
			http.Error(w, "mirror unavailable", http.StatusBadGateway)
		case "/ghfast/" + srvURL + "/dl/mpv-dev-x86_64-20261111-git-test.zip":
			ghfastHits++
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipPayload)
		case "/dl/mpv-dev-x86_64-20261111-git-test.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	pluginDir := t.TempDir()
	mgr := NewMPVManager(pluginDir)
	mgr.releaseAPIURL = srvURL + "/api/latest"
	mgr.mirrorPrefixes = map[string]string{
		SourceGHProxy:     srvURL + "/ghproxy/",
		SourceGHFast:      srvURL + "/ghfast/",
		SourceSourceForge: "",
	}
	mgr.probeFn = func(p string) (uint32, error) { return 0x00020000, nil }

	// Offline multi-mirror fallback: ghproxy fails 502 -> ghfast succeeds with zip extraction.
	if err := mgr.Download(context.Background(), SourceAuto, ""); err != nil {
		t.Fatalf("Download(SourceAuto): %v", err)
	}
	if ghproxyHits != 1 || ghfastHits != 1 {
		t.Fatalf("expected mirror fallback hits (1, 1), got (%d, %d)", ghproxyHits, ghfastHits)
	}
	installed, path, size := mgr.Status()
	if !installed || size != int64(len(dllPayload)) || !strings.HasSuffix(strings.ToLower(path), "libmpv-2.dll") {
		t.Fatalf("Status after zip download = (%v, %q, %d)", installed, path, size)
	}

	if err := mgr.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if installed, _, _ := mgr.Status(); installed {
		t.Fatal("expected libmpv removed")
	}

	// Reject corrupted PE file on local import.
	badFile := filepath.Join(t.TempDir(), "not-a-dll.dll")
	_ = os.WriteFile(badFile, []byte("hello"), 0o600)
	if err := mgr.ImportLocal(badFile); err == nil {
		t.Fatal("expected ImportLocal to reject invalid PE dll")
	}

	// Local .zip import succeeds.
	goodZip := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(goodZip, zipPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ImportLocal(goodZip); err != nil {
		t.Fatalf("ImportLocal(.zip): %v", err)
	}
	if installed, _, _ := mgr.Status(); !installed {
		t.Fatal("expected libmpv installed after ImportLocal")
	}

	// Verify Download cancels cleanly.
	_ = mgr.Remove()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mgr.Download(ctx, SourceGitHub, ""); err == nil {
		t.Fatal("expected canceled Download to return error")
	}
}
