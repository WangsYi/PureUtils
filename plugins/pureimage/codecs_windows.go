//go:build windows

package pureimage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/bodgit/sevenzip"
	"golang.org/x/sys/windows"

	"pureutils/pkg/purekit/i18n"
)

const (
	// CodecsAPIVersion matches PIC_API_VERSION in tools/build-pureimage-codecs/pureimage_codecs.h.
	CodecsAPIVersion uint32 = 0x00010000

	PicFmtAuto uint32 = 0
	PicFmtJPEG uint32 = 1
	PicFmtWebP uint32 = 2
	PicFmtAVIF uint32 = 3
	PicFmtHEIC uint32 = 4
	PicFmtJXL  uint32 = 5

	PicCapJPEG uint32 = 1 << 0
	PicCapWebP uint32 = 1 << 1
	PicCapAVIF uint32 = 1 << 2
	PicCapHEIC uint32 = 1 << 3
	PicCapJXL  uint32 = 1 << 4

	PicFlagAutoRotated uint32 = 1 << 0

	defaultCodecsTag   = "pureimage-codecs"
	defaultCodecsAsset = "pureimage-codecs-x64.7z"
	defaultCodecsAPI   = "https://api.github.com/repos/WangsYi/PureUtils/releases/tags/pureimage-codecs"
)

var (
	codecsMu        sync.Mutex
	codecsLoaded    bool
	codecsPluginDir string

	picAPIVersion       *syscall.LazyProc
	picCapabilities     *syscall.LazyProc
	picDecodeImage      *syscall.LazyProc
	picDecodeAnimation  *syscall.LazyProc
	picResultFrameCount *syscall.LazyProc
	picResultOrigWidth  *syscall.LazyProc
	picResultOrigHeight *syscall.LazyProc
	picResultFlags      *syscall.LazyProc
	picResultFrameWidth *syscall.LazyProc
	picResultFrameHt    *syscall.LazyProc
	picResultFrameDelay *syscall.LazyProc
	picResultFrameRGBA  *syscall.LazyProc
	picResultExif       *syscall.LazyProc
	picResultError      *syscall.LazyProc
	picResultFree       *syscall.LazyProc
)

// SetCodecsPluginDir records the plugin directory as an additional DLL search path.
func SetCodecsPluginDir(dir string) {
	codecsMu.Lock()
	codecsPluginDir = strings.TrimSpace(dir)
	codecsMu.Unlock()
}

// CodecsCandidates returns ordered candidate paths for pureimage_codecs.dll.
func CodecsCandidates(pluginDir string) []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		out = append(out,
			filepath.Join(dir, "pureimage_codecs.dll"),
			filepath.Join(dir, "libs", "codecs", "pureimage_codecs.dll"),
		)
	}
	out = append(out,
		filepath.Join("libs", "codecs", "pureimage_codecs.dll"),
		filepath.Join("..", "..", "libs", "codecs", "pureimage_codecs.dll"),
	)
	if strings.TrimSpace(pluginDir) != "" {
		out = append(out, filepath.Join(pluginDir, "pureimage_codecs.dll"))
	}
	out = append(out, "pureimage_codecs.dll")
	return out
}

// FindCodecsDLL locates the first existing pureimage_codecs.dll on disk.
func FindCodecsDLL(pluginDir string) (string, bool) {
	for _, cand := range CodecsCandidates(pluginDir) {
		if cand == "pureimage_codecs.dll" {
			if _, _, err := ProbeCodecs(cand); err == nil {
				return cand, true
			}
			continue
		}
		if info, err := os.Stat(cand); err == nil && !info.IsDir() && info.Size() > 0 {
			return cand, true
		}
	}
	return "", false
}

// ProbeCodecs temporarily loads dllPath and verifies its API version and capability bitmask.
func ProbeCodecs(dllPath string) (apiVer, caps uint32, err error) {
	h, err := windows.LoadLibrary(dllPath)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %w", i18n.F("无法加载 %s", "Failed to load %s", filepath.Base(dllPath)), err)
	}
	defer func() { _ = windows.FreeLibrary(h) }()

	verProc, err := windows.GetProcAddress(h, "pic_api_version")
	if err != nil || verProc == 0 {
		return 0, 0, fmt.Errorf("%s", i18n.F("%s 缺少 pic_api_version 导出", "%s is missing pic_api_version export", filepath.Base(dllPath)))
	}
	capsProc, err := windows.GetProcAddress(h, "pic_capabilities")
	if err != nil || capsProc == 0 {
		return 0, 0, fmt.Errorf("%s", i18n.F("%s 缺少 pic_capabilities 导出", "%s is missing pic_capabilities export", filepath.Base(dllPath)))
	}
	verRet, _, _ := syscall.SyscallN(verProc)
	if uint32(verRet) != CodecsAPIVersion {
		return uint32(verRet), 0, fmt.Errorf("%s (0x%08X)", i18n.F("%s API 版本不匹配", "%s API version mismatch", filepath.Base(dllPath)), uint32(verRet))
	}
	capsRet, _, _ := syscall.SyscallN(capsProc)
	return uint32(verRet), uint32(capsRet), nil
}

// LoadCodecs locates and binds pureimage_codecs.dll lazily without cgo.
func LoadCodecs() error {
	codecsMu.Lock()
	defer codecsMu.Unlock()
	if codecsLoaded {
		return nil
	}

	var lastErr error
	for _, cand := range CodecsCandidates(codecsPluginDir) {
		dll := syscall.NewLazyDLL(cand)
		pVer := dll.NewProc("pic_api_version")
		if err := pVer.Find(); err != nil {
			lastErr = err
			continue
		}
		procs := []*syscall.LazyProc{
			pVer,
			dll.NewProc("pic_capabilities"),
			dll.NewProc("pic_decode_image"),
			dll.NewProc("pic_decode_animation"),
			dll.NewProc("pic_result_frame_count"),
			dll.NewProc("pic_result_orig_width"),
			dll.NewProc("pic_result_orig_height"),
			dll.NewProc("pic_result_flags"),
			dll.NewProc("pic_result_frame_width"),
			dll.NewProc("pic_result_frame_height"),
			dll.NewProc("pic_result_frame_delay_ms"),
			dll.NewProc("pic_result_frame_rgba"),
			dll.NewProc("pic_result_exif"),
			dll.NewProc("pic_result_error"),
			dll.NewProc("pic_result_free"),
		}
		var missing error
		for _, p := range procs {
			if err := p.Find(); err != nil {
				missing = err
				break
			}
		}
		if missing != nil {
			lastErr = missing
			continue
		}
		verRet, _, _ := pVer.Call()
		if uint32(verRet) != CodecsAPIVersion {
			lastErr = fmt.Errorf("pureimage_codecs API mismatch: 0x%08X", uint32(verRet))
			continue
		}

		picAPIVersion = procs[0]
		picCapabilities = procs[1]
		picDecodeImage = procs[2]
		picDecodeAnimation = procs[3]
		picResultFrameCount = procs[4]
		picResultOrigWidth = procs[5]
		picResultOrigHeight = procs[6]
		picResultFlags = procs[7]
		picResultFrameWidth = procs[8]
		picResultFrameHt = procs[9]
		picResultFrameDelay = procs[10]
		picResultFrameRGBA = procs[11]
		picResultExif = procs[12]
		picResultError = procs[13]
		picResultFree = procs[14]
		codecsLoaded = true
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("pureimage_codecs.dll not found")
	}
	return lastErr
}

// CodecsAvailable reports whether native pureimage_codecs.dll is loaded and ready.
func CodecsAvailable() bool {
	return LoadCodecs() == nil
}

// CodecsCapabilities returns the codec capability bitmask exported by pureimage_codecs.dll.
func CodecsCapabilities() uint32 {
	if err := LoadCodecs(); err != nil {
		return 0
	}
	v, _, _ := picCapabilities.Call()
	return uint32(v)
}

func codecErrorFromResult(res uintptr, fallbackCode int32) error {
	if res != 0 {
		ptr, _, _ := picResultError.Call(res)
		if ptr != 0 {
			msg := strings.TrimSpace(windows.BytePtrToString((*byte)(unsafe.Pointer(ptr))))
			if msg != "" {
				return errors.New(msg)
			}
		}
	}
	return fmt.Errorf("pureimage_codecs error %d", fallbackCode)
}

func codecExifFromResult(res uintptr) ExifData {
	ex := ExifData{Orientation: 1}
	var exifLen uintptr
	exifPtr, _, _ := picResultExif.Call(res, uintptr(unsafe.Pointer(&exifLen)))
	if exifPtr != 0 && exifLen > 0 && exifLen <= 16*1024*1024 {
		raw := unsafe.Slice((*byte)(unsafe.Pointer(exifPtr)), int(exifLen))
		ex = ParseExifRaw(raw)
	}
	flagsRet, _, _ := picResultFlags.Call(res)
	if uint32(flagsRet)&PicFlagAutoRotated != 0 {
		ex.Orientation = 1
	}
	return ex
}

func codecFrameImage(res uintptr, idx int, opaque bool) (image.Image, int32, error) {
	fwRet, _, _ := picResultFrameWidth.Call(res, uintptr(idx))
	fhRet, _, _ := picResultFrameHt.Call(res, uintptr(idx))
	fw, fh := int(int32(fwRet)), int(int32(fhRet))
	delayRet, _, _ := picResultFrameDelay.Call(res, uintptr(idx))
	delayMs := int32(delayRet)

	var rgbaLen uintptr
	rgbaPtr, _, _ := picResultFrameRGBA.Call(res, uintptr(idx), uintptr(unsafe.Pointer(&rgbaLen)))
	wantLen := fw * fh * 4
	if fw <= 0 || fh <= 0 || rgbaPtr == 0 || int(rgbaLen) < wantLen {
		return nil, delayMs, fmt.Errorf("invalid codec frame %d (%dx%d len=%d)", idx, fw, fh, rgbaLen)
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(rgbaPtr)), wantLen)
	if opaque {
		img := image.NewRGBA(image.Rect(0, 0, fw, fh))
		copy(img.Pix, src)
		return img, delayMs, nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, fw, fh))
	copy(img.Pix, src)
	return img, delayMs, nil
}

// DecodeCodecImage decodes a still image using pureimage_codecs.dll.
// For JPEG, maxDim > 0 selects native DCT scaling <= 1x while origW/origH report the full dimensions.
func DecodeCodecImage(format uint32, data []byte, maxDim int) (image.Image, ExifData, int, int, error) {
	if len(data) == 0 {
		return nil, ExifData{Orientation: 1}, 0, 0, errors.New("empty image buffer")
	}
	if err := LoadCodecs(); err != nil {
		return nil, ExifData{Orientation: 1}, 0, 0, err
	}

	var res uintptr
	rc, _, _ := picDecodeImage.Call(
		uintptr(format),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(int32(maxDim)),
		uintptr(unsafe.Pointer(&res)),
	)
	runtime.KeepAlive(data)
	if res != 0 {
		defer picResultFree.Call(res)
	}
	if int32(rc) != 0 || res == 0 {
		return nil, ExifData{Orientation: 1}, 0, 0, codecErrorFromResult(res, int32(rc))
	}

	fcRet, _, _ := picResultFrameCount.Call(res)
	if int32(fcRet) < 1 {
		return nil, ExifData{Orientation: 1}, 0, 0, errors.New("codec returned zero frames")
	}
	ex := codecExifFromResult(res)
	img, _, err := codecFrameImage(res, 0, format == PicFmtJPEG)
	if err != nil {
		return nil, ex, 0, 0, err
	}
	owRet, _, _ := picResultOrigWidth.Call(res)
	ohRet, _, _ := picResultOrigHeight.Call(res)
	origW, origH := int(int32(owRet)), int(int32(ohRet))
	if origW <= 0 || origH <= 0 {
		b := img.Bounds()
		origW, origH = b.Dx(), b.Dy()
	}
	return img, ex, origW, origH, nil
}

// DecodeCodecAnimation decodes all animation frames via pureimage_codecs.dll.
func DecodeCodecAnimation(format uint32, data []byte) ([]image.Image, []time.Duration, ExifData, error) {
	if len(data) == 0 {
		return nil, nil, ExifData{Orientation: 1}, errors.New("empty animation buffer")
	}
	if err := LoadCodecs(); err != nil {
		return nil, nil, ExifData{Orientation: 1}, err
	}

	var res uintptr
	rc, _, _ := picDecodeAnimation.Call(
		uintptr(format),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&res)),
	)
	runtime.KeepAlive(data)
	if res != 0 {
		defer picResultFree.Call(res)
	}
	if int32(rc) != 0 || res == 0 {
		return nil, nil, ExifData{Orientation: 1}, codecErrorFromResult(res, int32(rc))
	}

	fcRet, _, _ := picResultFrameCount.Call(res)
	fc := int(int32(fcRet))
	if fc < 1 {
		return nil, nil, ExifData{Orientation: 1}, errors.New("codec animation returned zero frames")
	}
	ex := codecExifFromResult(res)

	frames := make([]image.Image, 0, fc)
	delays := make([]time.Duration, 0, fc)
	for i := 0; i < fc; i++ {
		img, delayMs, err := codecFrameImage(res, i, false)
		if err != nil {
			return nil, nil, ex, err
		}
		if delayMs <= 0 {
			delayMs = 100
		}
		frames = append(frames, img)
		delays = append(delays, time.Duration(delayMs)*time.Millisecond)
	}
	return frames, delays, ex, nil
}

// CodecsManager handles downloading, verifying, and extracting pureimage_codecs.dll.
type CodecsManager struct {
	targetDir      string
	httpClient     *http.Client
	releaseAPIURL  string
	fallbackURL    string
	mirrorPrefixes map[string]string
	probeFn        func(string) (uint32, uint32, error)
}

// NewCodecsManager creates a manager targeting outDir/pureimage_codecs.dll.
func NewCodecsManager(outDir string) *CodecsManager {
	return &CodecsManager{
		targetDir:     strings.TrimSpace(outDir),
		httpClient:    &http.Client{Timeout: 0},
		releaseAPIURL: defaultCodecsAPI,
		probeFn:       ProbeCodecs,
	}
}

// ManagedDLLPath returns outDir/pureimage_codecs.dll.
func (m *CodecsManager) ManagedDLLPath() string {
	if m.targetDir == "" {
		return "pureimage_codecs.dll"
	}
	return filepath.Join(m.targetDir, "pureimage_codecs.dll")
}

// Download fetches pureimage-codecs-x64.7z via mirrors, extracts pureimage_codecs.dll,
// validates PE and ABI exports, and installs it atomically.
func (m *CodecsManager) Download(ctx context.Context, source, customURL string) error {
	if m.targetDir == "" {
		return errors.New("codecs output directory is not configured")
	}
	if err := os.MkdirAll(m.targetDir, 0o755); err != nil {
		return err
	}
	rawGH := m.resolveAssetURL(ctx)
	cands := buildCodecsMirrorCandidates(source, customURL, rawGH, m.mirrorPrefixes)

	tmpDir, err := os.MkdirTemp(m.targetDir, "codecs-dl-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	var lastErr error
	for _, urlStr := range cands {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		arcPath := filepath.Join(tmpDir, defaultCodecsAsset)
		if err := m.downloadFile(ctx, urlStr, arcPath); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			lastErr = err
			continue
		}
		tmpDLL := filepath.Join(tmpDir, "pureimage_codecs.dll")
		if err := extractCodecsDLLFrom7z(arcPath, tmpDLL); err != nil {
			lastErr = err
			continue
		}
		if m.probeFn != nil {
			if _, _, err := m.probeFn(tmpDLL); err != nil {
				lastErr = err
				continue
			}
		}
		dst := m.ManagedDLLPath()
		_ = os.Remove(dst)
		if err := os.Rename(tmpDLL, dst); err != nil {
			return err
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("all pureimage-codecs download sources failed")
}

func (m *CodecsManager) resolveAssetURL(ctx context.Context) string {
	fallback := strings.TrimSpace(m.fallbackURL)
	if fallback == "" {
		fallback = fmt.Sprintf("https://github.com/WangsYi/PureUtils/releases/download/%s/%s", defaultCodecsTag, defaultCodecsAsset)
	}
	if strings.TrimSpace(m.releaseAPIURL) == "" {
		return fallback
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, m.releaseAPIURL, nil)
	if err != nil {
		return fallback
	}
	req.Header.Set("User-Agent", "PureUtils-PureImage")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fallback
	}
	var payload struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return fallback
	}
	for _, a := range payload.Assets {
		if strings.EqualFold(a.Name, defaultCodecsAsset) && strings.TrimSpace(a.BrowserDownloadURL) != "" {
			return strings.TrimSpace(a.BrowserDownloadURL)
		}
	}
	return fallback
}

func buildCodecsMirrorCandidates(source, customURL, rawGH string, overrides map[string]string) []string {
	ghProxy := "https://gh-proxy.org/"
	ghFast := "https://ghfast.top/"
	if overrides != nil {
		if v, ok := overrides["ghproxy"]; ok {
			ghProxy = v
		}
		if v, ok := overrides["ghfast"]; ok {
			ghFast = v
		}
	}
	var out []string
	if custom := strings.TrimSpace(customURL); custom != "" {
		if strings.Contains(custom, "{url}") {
			out = append(out, strings.ReplaceAll(custom, "{url}", rawGH))
		} else if strings.HasSuffix(strings.ToLower(custom), ".7z") {
			out = append(out, custom)
		} else {
			out = append(out, strings.TrimRight(custom, "/")+"/"+rawGH)
		}
	}
	var order []string
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "ghproxy":
		order = []string{"ghproxy", "ghfast", "github"}
	case "ghfast":
		order = []string{"ghfast", "ghproxy", "github"}
	case "github":
		order = []string{"github", "ghproxy", "ghfast"}
	default:
		order = []string{"ghproxy", "ghfast", "github"}
	}
	for _, id := range order {
		switch id {
		case "ghproxy":
			if ghProxy != "" {
				out = append(out, ghProxy+rawGH)
			}
		case "ghfast":
			if ghFast != "" {
				out = append(out, ghFast+rawGH)
			}
		case "github":
			out = append(out, rawGH)
		}
	}
	return out
}

func (m *CodecsManager) downloadFile(ctx context.Context, urlStr, dst string) error {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PureUtils-PureImage")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func extractCodecsDLLFrom7z(src7z, dstDLL string) error {
	r, err := sevenzip.OpenReader(src7z)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() || !strings.EqualFold(filepath.Base(f.Name), "pureimage_codecs.dll") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dstDLL)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, cpErr := io.Copy(out, rc)
		closeErr := out.Close()
		_ = rc.Close()
		if cpErr != nil {
			return cpErr
		}
		return closeErr
	}
	return errors.New("pureimage_codecs.dll not found in 7z archive")
}
