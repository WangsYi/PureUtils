package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
)

func buildFakeSetupEXE(size int, fill byte) ([]byte, string) {
	if size < 4096 {
		size = 4096
	}
	b := make([]byte, size)
	b[0] = 'M'
	b[1] = 'Z'
	for i := 2; i < len(b); i++ {
		b[i] = fill
	}
	sum := sha256.Sum256(b)
	return b, hex.EncodeToString(sum[:])
}

func TestUpdaterAPICheckDownloadInstallAndMirrors(t *testing.T) {
	setupBytes, setupSHA := buildFakeSetupEXE(8192, 0x5A)

	var srvURL string
	var mirrorHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/latest":
			fmt.Fprintf(w, `{
				"tag_name": "v1.2.0",
				"name": "PureUtils v1.2.0",
				"body": "新增 PureVideo 极速视频与自动更新功能",
				"html_url": "%s/releases/tag/v1.2.0",
				"assets": [
					{
						"name": "PureUtils-1.2.0-setup.exe",
						"size": %d,
						"digest": "sha256:%s",
						"browser_download_url": "%s/asset/PureUtils-1.2.0-setup.exe"
					}
				]
			}`, srvURL, len(setupBytes), setupSHA, srvURL)
		case "/ghproxy/asset/PureUtils-1.2.0-setup.exe":
			mirrorHits++
			// Simulate mirror outage so SourceAuto seamlessly falls back to ghfast/direct.
			http.Error(w, "bad gateway", http.StatusBadGateway)
		case "/ghfast/" + srvURL + "/asset/PureUtils-1.2.0-setup.exe",
			"/ghproxy/" + srvURL + "/asset/PureUtils-1.2.0-setup.exe":
			mirrorHits++
			if strings.HasPrefix(r.URL.Path, "/ghproxy/") {
				http.Error(w, "mirror down", http.StatusBadGateway)
				return
			}
			_, _ = w.Write(setupBytes)
		case "/asset/PureUtils-1.2.0-setup.exe":
			_, _ = w.Write(setupBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	var mu sync.Mutex
	var notified []string
	var installedPkg, installedExe string
	var quitCalled bool

	dataDir := filepath.Join(t.TempDir(), "updates")
	fakeExe := filepath.Join(t.TempDir(), "PureUtils.exe")
	u := NewUpdater(UpdaterOptions{
		CurrentVersion: "1.0.0",
		DataDir:        dataDir,
		ExePath:        fakeExe,
		APIURL:         srvURL + "/api/latest",
		MirrorPrefixes: map[string]string{
			UpdateSourceGHProxy: srvURL + "/ghproxy/",
			UpdateSourceGHFast:  srvURL + "/ghfast/",
		},
		MinAssetBytes: 1024,
		LaunchInstallerFn: func(installerPath, currentExe string) error {
			installedPkg = installerPath
			installedExe = currentExe
			return nil
		},
		OnNotify: func(title, body string) {
			mu.Lock()
			notified = append(notified, title+" | "+body)
			mu.Unlock()
		},
		OnQuit: func() {
			quitCalled = true
		},
	})

	cfg := config.DefaultHostConfig().Update
	snap, err := u.Check(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if snap.Status != UpdateAvailable || snap.Release == nil || snap.Release.TagName != "v1.2.0" {
		t.Fatalf("unexpected snapshot after Check: %+v", snap)
	}
	if snap.Release.AssetSHA256 != setupSHA {
		t.Fatalf("AssetSHA256 = %q, want %q", snap.Release.AssetSHA256, setupSHA)
	}
	if u.ShouldAutoCheck(24*time.Hour, time.Now()) {
		t.Fatal("ShouldAutoCheck should be false immediately after check")
	}

	mu.Lock()
	if len(notified) != 1 {
		t.Fatalf("expected 1 background availability notification, got %v", notified)
	}
	mu.Unlock()

	if err := u.Download(context.Background(), UpdateSourceAuto); err != nil {
		t.Fatalf("Download(SourceAuto) failed: %v", err)
	}
	if mirrorHits == 0 {
		t.Fatal("expected SourceAuto to hit configured mirror prefixes")
	}
	snap = u.Snapshot()
	if snap.Status != UpdateReady || snap.DownloadedPath == "" {
		t.Fatalf("expected UpdateReady, got %+v", snap)
	}

	if err := u.InstallAndRestart(); err != nil {
		t.Fatalf("InstallAndRestart: %v", err)
	}
	if !quitCalled || installedPkg != snap.DownloadedPath || installedExe != fakeExe {
		t.Fatalf("InstallAndRestart state: quit=%v pkg=%q exe=%q", quitCalled, installedPkg, installedExe)
	}
}

func TestUpdaterRedirectFallbackSHA256RejectAndCleanup(t *testing.T) {
	setupBytes, setupSHA := buildFakeSetupEXE(4096, 0x33)

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api-forbidden":
			http.Error(w, "rate limit exceeded", http.StatusForbidden)
		case "/releases/latest":
			w.Header().Set("Location", srvURL+"/WangsYi/PureUtils/releases/tag/v1.3.0")
			w.WriteHeader(http.StatusFound)
		case "/bad-sha":
			bad := make([]byte, 4096)
			bad[0], bad[1] = 'M', 'Z'
			_, _ = w.Write(bad)
		case "/not-pe":
			_, _ = w.Write([]byte("html rate limit page instead of exe payload"))
		case "/good":
			_, _ = w.Write(setupBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	dataDir := filepath.Join(t.TempDir(), "updates")
	u := NewUpdater(UpdaterOptions{
		CurrentVersion: "1.2.0",
		DataDir:        dataDir,
		APIURL:         srvURL + "/api-forbidden",
		ReleasePageURL: srvURL + "/releases/latest",
		MirrorPrefixes: map[string]string{
			UpdateSourceGHProxy: "",
			UpdateSourceGHFast:  "",
		},
		MinAssetBytes: 1024,
	})

	snap, err := u.Check(context.Background(), config.UpdateConfig{DownloadSource: UpdateSourceGitHub}, true)
	if err != nil {
		t.Fatalf("Check with HTML redirect fallback failed: %v", err)
	}
	if snap.Status != UpdateAvailable || snap.Release == nil || snap.Release.TagName != "v1.3.0" {
		t.Fatalf("redirect fallback snapshot = %+v", snap)
	}
	if snap.Release.AssetName != "PureUtils-1.3.0-setup.exe" {
		t.Fatalf("AssetName = %q", snap.Release.AssetName)
	}

	// Reject corrupted SHA-256 download.
	u.mu.Lock()
	u.snap.Release.AssetURL = srvURL + "/bad-sha"
	u.snap.Release.AssetSHA256 = setupSHA
	u.mu.Unlock()
	if err := u.Download(context.Background(), UpdateSourceGitHub); err == nil {
		t.Fatal("expected SHA-256 mismatch error")
	}

	// Reject non-PE HTML error payload.
	u.mu.Lock()
	u.snap.Release.AssetURL = srvURL + "/not-pe"
	u.snap.Release.AssetSHA256 = ""
	u.mu.Unlock()
	if err := u.Download(context.Background(), UpdateSourceGitHub); err == nil {
		t.Fatal("expected invalid PE/size rejection")
	}

	// Succeed on valid download, then test CleanupOldDownloads removes old installers & .part files.
	u.mu.Lock()
	u.snap.Release.AssetURL = srvURL + "/good"
	u.snap.Release.AssetSHA256 = setupSHA
	u.mu.Unlock()
	if err := u.Download(context.Background(), UpdateSourceGitHub); err != nil {
		t.Fatalf("valid download failed: %v", err)
	}
	oldFile := filepath.Join(dataDir, "PureUtils-1.0.0-setup.exe")
	partFile := filepath.Join(dataDir, "PureUtils-1.3.0-setup.exe.part")
	_ = os.WriteFile(oldFile, []byte("MZold"), 0o600)
	_ = os.WriteFile(partFile, []byte("part"), 0o600)
	u.CleanupOldDownloads()
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("expected obsolete installer cleaned up")
	}
	if _, err := os.Stat(partFile); !os.IsNotExist(err) {
		t.Fatal("expected .part file cleaned up")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "PureUtils-1.3.0-setup.exe")); err != nil {
		t.Fatalf("expected newer v1.3.0 installer preserved: %v", err)
	}
}

func TestUpdaterUpToDateAndSettingsCardUI(t *testing.T) {
	setupBytes, setupSHA := buildFakeSetupEXE(4096, 0x11)
	curVer := core.ShortVersion(core.AppVersion)
	const newerVer = "99.0.0"
	const newerTag = "v99.0.0"
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/same":
			fmt.Fprintf(w, `{"tag_name":"v%s","assets":[{"name":"PureUtils-%s-setup.exe","size":%d,"browser_download_url":"%s/setup.exe"}]}`, curVer, curVer, len(setupBytes), srvURL)
		case "/api/newer":
			fmt.Fprintf(w, `{"tag_name":"%s","body":"Fast update notes","assets":[{"name":"PureUtils-%s-setup.exe","size":%d,"digest":"sha256:%s","browser_download_url":"%s/setup.exe"}]}`, newerTag, newerVer, len(setupBytes), setupSHA, srvURL)
		case "/setup.exe":
			_, _ = w.Write(setupBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	a := testHostWithModules(t, CompiledModules(), nil, nil)
	a.updater.apiURL = srvURL + "/api/same"
	a.updater.minAssetBytes = 1024

	snap, err := a.updater.Check(context.Background(), a.HostConfig().Update, true)
	if err != nil || snap.Status != UpdateUpToDate {
		t.Fatalf("same-version Check = (%+v, %v), want UpdateUpToDate", snap, err)
	}

	a.updater.apiURL = srvURL + "/api/newer"
	snap, err = a.updater.Check(context.Background(), a.HostConfig().Update, true)
	if err != nil || snap.Status != UpdateAvailable {
		t.Fatalf("newer Check = (%+v, %v), want UpdateAvailable", snap, err)
	}

	v := newSettingsView(a, "advanced")
	tt := ui.NewTester(v.view, 860, 640)
	tt.Frame()
	if !tt.HasText("软件更新") || !tt.HasText("发现新版本 "+newerTag) {
		t.Fatalf("expected Software Update card in Settings > Advanced, texts=%v", tt.Texts())
	}
	if err := tt.Click("忽略此版本"); err != nil {
		t.Fatalf("click skip version: %v", err)
	}
	tt.Frame()
	if got := a.HostConfig().Update.SkippedVersion; got != newerTag {
		t.Fatalf("SkippedVersion = %q, want %s", got, newerTag)
	}

	if err := tt.Click("开关：自动下载更新"); err != nil {
		t.Fatalf("toggle AutoDownload: %v", err)
	}
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	tt.Frame()
	if !a.HostConfig().Update.AutoDownload {
		t.Fatal("expected Update.AutoDownload=true persisted")
	}
}
