//go:build windows

package purevideo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVideoPath(t *testing.T) {
	valid := []string{"demo.mp4", "CLIP.MKV", "movie.MOV", "stream.flv", "legacy.rmvb", `D:\a\b\clip.webm`}
	for _, p := range valid {
		if !IsVideoPath(p) {
			t.Errorf("IsVideoPath(%q) = false, want true", p)
		}
	}
	invalid := []string{"photo.png", "archive.zip", "notes.txt", "", ".mp4dir"}
	for _, p := range invalid {
		if IsVideoPath(p) && p != ".mp4dir" {
			t.Errorf("IsVideoPath(%q) = true, want false", p)
		}
	}
	if IsVideoPath(".mp4dir") {
		t.Error("expected .mp4dir not treated as video")
	}
}

func TestScanVideoDirSorting(t *testing.T) {
	dir := t.TempDir()
	files := []string{"ep10.mp4", "ep2.mkv", "ep1.mov", "readme.txt"}
	for i, name := range files {
		p := filepath.Join(dir, name)
		data := make([]byte, (i+1)*10)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		ts := time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC)
		_ = os.Chtimes(p, ts, ts)
	}

	byName := ScanVideoDir(dir, "name", false)
	if len(byName) != 3 {
		t.Fatalf("ScanVideoDir len = %d, want 3: %v", len(byName), byName)
	}
	if filepath.Base(byName[0]) != "ep1.mov" || filepath.Base(byName[1]) != "ep2.mkv" || filepath.Base(byName[2]) != "ep10.mp4" {
		t.Fatalf("unexpected natural sort order: %v", byName)
	}

	bySizeDesc := ScanVideoDir(dir, "size", true)
	if filepath.Base(bySizeDesc[0]) != "ep1.mov" {
		t.Fatalf("expected largest video first on size desc: %v", bySizeDesc)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{-time.Second, "00:00"},
		{65 * time.Second, "01:05"},
		{3661 * time.Second, "1:01:01"},
	}
	for _, tc := range cases {
		if got := FormatDuration(tc.d); got != tc.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
