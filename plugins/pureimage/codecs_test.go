//go:build windows

package pureimage

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseExifRawVariants(t *testing.T) {
	jpg := encodeJPEGWithExif(t, renderShowcaseLandscape(64, 48))
	// Extract APP1 Exif payload after "Exif\x00\x00" and verify all container wrappers work.
	idx := bytes.Index(jpg, []byte("Exif\x00\x00"))
	if idx < 0 {
		t.Fatal("missing EXIF segment in test JPEG")
	}
	tiffPayload := jpg[idx+6:]

	ex1 := ParseExifRaw(tiffPayload)
	if ex1.Make != "Canon" || ex1.Model != "EOS R5" {
		t.Fatalf("ParseExifRaw(TIFF) got Make=%q Model=%q", ex1.Make, ex1.Model)
	}

	// ISOBMFF HEIF style: 4-byte BE offset + "Exif\x00\x00" + TIFF
	heifStyle := append([]byte{0, 0, 0, 6}, jpg[idx:]...)
	ex2 := ParseExifRaw(heifStyle)
	if ex2.Make != "Canon" || ex2.Model != "EOS R5" {
		t.Fatalf("ParseExifRaw(HEIF prefix) got Make=%q Model=%q", ex2.Make, ex2.Model)
	}

	if empty := ParseExifRaw([]byte{1, 2, 3}); empty.Orientation != 1 || empty.Make != "" {
		t.Fatalf("unexpected non-empty EXIF for short input: %+v", empty)
	}
}

func TestCodecsMirrorCandidatesAndCandidatesList(t *testing.T) {
	cands := CodecsCandidates(`C:\fake\plugin`)
	if len(cands) < 4 {
		t.Fatalf("expected >=4 codec candidates, got %v", cands)
	}
	if !strings.HasSuffix(cands[0], "pureimage_codecs.dll") {
		t.Fatalf("unexpected candidate 0: %q", cands[0])
	}

	raw := "https://example.com/pureimage-codecs-x64.7z"
	urls := buildCodecsMirrorCandidates("github", "https://custom.local/mirror/{url}", raw, nil)
	if len(urls) < 4 || urls[0] != "https://custom.local/mirror/"+raw || urls[1] != raw {
		t.Fatalf("unexpected mirror URLs: %v", urls)
	}
}

func TestProbeCodecsAndErrorPaths(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "not-a-dll.dll")
	if err := os.WriteFile(bad, []byte("not pe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ProbeCodecs(bad); err == nil {
		t.Fatal("expected ProbeCodecs error for invalid DLL")
	}
	if _, _, _, _, err := DecodeCodecImage(PicFmtAuto, nil, 0); err == nil {
		t.Fatal("expected error for empty image buffer")
	}
	if _, _, _, err := DecodeCodecAnimation(PicFmtWebP, nil); err == nil {
		t.Fatal("expected error for empty animation buffer")
	}
}

func TestNativeCodecsRealDecodeAndDownloadFallback(t *testing.T) {
	if !CodecsAvailable() {
		t.Skip("pureimage_codecs.dll not staged locally; run go run ./tools/fetchcodecs -out libs/codecs")
	}
	dllPath, ok := FindCodecsDLL("")
	if !ok {
		t.Fatal("expected FindCodecsDLL to locate pureimage_codecs.dll when CodecsAvailable=true")
	}
	ver, caps, err := ProbeCodecs(dllPath)
	if err != nil {
		t.Fatalf("ProbeCodecs(%q): %v", dllPath, err)
	}
	wantCaps := PicCapJPEG | PicCapWebP | PicCapAVIF | PicCapHEIC | PicCapJXL
	if ver != CodecsAPIVersion || (caps&wantCaps) != wantCaps {
		t.Fatalf("unexpected ProbeCodecs ver=0x%08X caps=0x%02X", ver, caps)
	}

	// Small JPEG must NOT be upscaled when maxDim > image dimensions.
	smallJPG := encodeJPEGWithExif(t, renderShowcaseLandscape(120, 80))
	imgSmall, _, ow, oh, err := DecodeCodecImage(PicFmtJPEG, smallJPG, 1600)
	if err != nil {
		t.Fatalf("DecodeCodecImage small JPEG: %v", err)
	}
	if ow != 120 || oh != 80 || imgSmall.Bounds().Dx() != 120 || imgSmall.Bounds().Dy() != 80 {
		t.Fatalf("expected 120x80 without upscaling, got orig=%dx%d decoded=%v", ow, oh, imgSmall.Bounds())
	}

	// Large JPEG must downscale cleanly via libjpeg-turbo DCT scaling while reporting original dimensions.
	largeSrc := image.NewRGBA(image.Rect(0, 0, 1280, 800))
	for y := 0; y < 800; y += 8 {
		for x := 0; x < 1280; x += 8 {
			largeSrc.SetRGBA(x, y, color.RGBA{R: 50, G: 140, B: 220, A: 255})
		}
	}
	largeJPG := encodeJPEGWithExif(t, largeSrc)
	imgScaled, _, owL, ohL, err := DecodeCodecImage(PicFmtJPEG, largeJPG, 320)
	if err != nil {
		t.Fatalf("DecodeCodecImage large JPEG: %v", err)
	}
	if owL != 1280 || ohL != 800 {
		t.Fatalf("expected orig=1280x800, got %dx%d", owL, ohL)
	}
	if imgScaled.Bounds().Dx() >= 1280 || imgScaled.Bounds().Dx() == 0 {
		t.Fatalf("expected scaled width < 1280, got %v", imgScaled.Bounds())
	}

	// Corrupt buffers must return clean errors from the native DLL without crashing.
	for _, tc := range []struct {
		name string
		fmt  uint32
	}{
		{"auto", PicFmtAuto},
		{"webp", PicFmtWebP},
		{"avif", PicFmtAVIF},
		{"heic", PicFmtHEIC},
		{"jxl", PicFmtJXL},
	} {
		if _, _, _, _, err := DecodeCodecImage(tc.fmt, []byte("invalid-image-bytes-1234"), 0); err == nil {
			t.Errorf("%s: expected error on corrupt payload", tc.name)
		}
		if _, _, _, err := DecodeCodecAnimation(tc.fmt, []byte("invalid-anim-bytes-1234")); err == nil {
			t.Errorf("%s: expected animation error on corrupt payload", tc.name)
		}
	}

	// CodecsManager error handling on hermetic local failing mirror server (zero external network).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	defer srv.Close()
	mgr := NewCodecsManager(t.TempDir())
	mgr.releaseAPIURL = ""
	mgr.fallbackURL = srv.URL + "/pureimage-codecs-x64.7z"
	mgr.mirrorPrefixes = map[string]string{"ghproxy": "", "ghfast": ""}
	if err := mgr.Download(context.Background(), "github", srv.URL+"/custom/pureimage-codecs-x64.7z"); err == nil {
		t.Fatal("expected error when mirror returns HTTP 502")
	}
}
