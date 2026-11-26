//go:build windows

package purevideo

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bodgit/sevenzip"
)

const (
	SourceAuto        = "auto"
	SourceGHProxy     = "ghproxy"
	SourceGHFast      = "ghfast"
	SourceSourceForge = "sourceforge"
	SourceGitHub      = "github"

	defaultMPVReleaseAPI = "https://api.github.com/repos/shinchiro/mpv-winbuild-cmake/releases/latest"
	defaultMPVTag        = "20261111"
	defaultMPVAsset      = "mpv-dev-x86_64-20261111-git-bcde6b93cc.7z"
)

// DownloadSourceInfo describes one selectable download mirror for libmpv.
type DownloadSourceInfo struct {
	ID    string
	Label string
	Hint  string
}

// DownloadSources lists download sources in preferred display order.
var DownloadSources = []DownloadSourceInfo{
	{SourceAuto, "自动优选（推荐）", "按顺序尝试国内 GitHub 镜像、SourceForge 与 GitHub 官方源"},
	{SourceGHProxy, "GitHub 加速 (gh-proxy.org)", "国内代理加速 GitHub Release"},
	{SourceGHFast, "GitHub 加速 (ghfast.top)", "备用国内 GitHub Release 镜像"},
	{SourceSourceForge, "SourceForge 官方镜像", "直连 SourceForge 全球 CDN"},
	{SourceGitHub, "GitHub 官方直连", "直连 github.com/shinchiro/mpv-winbuild-cmake"},
}

// DownloadSourceLabel returns the Chinese UI label for source id.
func DownloadSourceLabel(id string) string {
	for _, s := range DownloadSources {
		if s.ID == id {
			return s.Label
		}
	}
	return DownloadSources[0].Label
}

// DownloadSourceIDFromLabel resolves source id from UI label.
func DownloadSourceIDFromLabel(label string) string {
	for _, s := range DownloadSources {
		if s.Label == label {
			return s.ID
		}
	}
	return SourceAuto
}

// DownloadProgress reports real-time libmpv download/extraction status.
type DownloadProgress struct {
	Active      bool
	SourceID    string
	SourceLabel string
	Stage       string
	DoneBytes   int64
	TotalBytes  int64
	Err         string
	Completed   bool
}

type releaseRef struct {
	Tag       string
	Asset     string
	DirectURL string
}

type mirrorCandidate struct {
	SourceID string
	Label    string
	URL      string
}

// MPVManager handles locating, downloading, importing, and removing libmpv-2.dll.
type MPVManager struct {
	pluginDir      string
	httpClient     *http.Client
	releaseAPIURL  string
	mirrorPrefixes map[string]string
	probeFn        func(string) (uint32, error)

	mu       sync.RWMutex
	progress DownloadProgress
	cancel   context.CancelFunc
	onChange func()
}

// NewMPVManager manages libmpv-2.dll inside pluginDir.
func NewMPVManager(pluginDir string) *MPVManager {
	return &MPVManager{
		pluginDir: pluginDir,
		httpClient: &http.Client{
			Timeout: 0, // governed per-request by context
		},
		releaseAPIURL: defaultMPVReleaseAPI,
		probeFn:       ProbeLibMPV,
	}
}

// SetOnChange sets an optional callback fired whenever download progress changes.
func (m *MPVManager) SetOnChange(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

// ManagedDLLPath returns the plugin-scoped target path for libmpv-2.dll.
func (m *MPVManager) ManagedDLLPath() string {
	if strings.TrimSpace(m.pluginDir) == "" {
		return "libmpv-2.dll"
	}
	return filepath.Join(m.pluginDir, "libmpv-2.dll")
}

// Status reports whether libmpv-2.dll is available, its path, and file size.
func (m *MPVManager) Status() (installed bool, path string, size int64) {
	p, ok := FindLibMPV(m.pluginDir)
	if !ok {
		return false, m.ManagedDLLPath(), 0
	}
	if info, err := os.Stat(p); err == nil {
		return true, p, info.Size()
	}
	return true, p, 0
}

// HasManagedOverride reports whether a downloaded/imported libmpv-2.dll override exists in pluginDir.
func (m *MPVManager) HasManagedOverride() bool {
	if strings.TrimSpace(m.pluginDir) == "" {
		return false
	}
	info, err := os.Stat(m.ManagedDLLPath())
	return err == nil && !info.IsDir() && info.Size() > 0
}

// Progress returns the latest download progress snapshot.
func (m *MPVManager) Progress() DownloadProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.progress
}

func (m *MPVManager) updateProgress(fn func(*DownloadProgress)) {
	m.mu.Lock()
	fn(&m.progress)
	cb := m.onChange
	m.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// StartDownload begins asynchronous download and installation of libmpv-2.dll.
func (m *MPVManager) StartDownload(source, customURL string) {
	m.mu.Lock()
	if m.progress.Active {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.progress = DownloadProgress{
		Active:      true,
		SourceID:    source,
		SourceLabel: DownloadSourceLabel(source),
		Stage:       "正在解析最新版本…",
	}
	cb := m.onChange
	m.mu.Unlock()
	if cb != nil {
		cb()
	}

	go func() {
		err := m.Download(ctx, source, customURL)
		m.mu.Lock()
		m.cancel = nil
		m.progress.Active = false
		if err != nil {
			if errors.Is(err, context.Canceled) {
				m.progress.Err = "已取消下载"
				m.progress.Stage = "下载已取消"
			} else {
				m.progress.Err = err.Error()
				m.progress.Stage = "下载失败"
			}
			m.progress.Completed = false
		} else {
			m.progress.Err = ""
			m.progress.Stage = "libmpv 已就绪"
			m.progress.Completed = true
		}
		cb := m.onChange
		m.mu.Unlock()
		if cb != nil {
			cb()
		}
	}()
}

// CancelDownload aborts any active download.
func (m *MPVManager) CancelDownload() {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Download synchronously downloads, extracts, validates, and installs libmpv-2.dll.
func (m *MPVManager) Download(ctx context.Context, source, customURL string) error {
	if strings.TrimSpace(m.pluginDir) == "" {
		return fmt.Errorf("未配置插件数据目录")
	}
	if err := os.MkdirAll(m.pluginDir, 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	rel := m.resolveLatestRelease(ctx)
	candidates := buildMirrorCandidates(source, customURL, rel, m.mirrorPrefixes)
	if len(candidates) == 0 {
		return fmt.Errorf("未找到可用的下载源")
	}

	tmpDir, err := os.MkdirTemp(m.pluginDir, "mpv-dl-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	var lastErr error
	for _, cand := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.updateProgress(func(p *DownloadProgress) {
			p.SourceID = cand.SourceID
			p.SourceLabel = cand.Label
			p.Stage = "正在连接 " + cand.Label + "…"
			p.DoneBytes = 0
			p.TotalBytes = 0
		})
		archivePath := filepath.Join(tmpDir, "download.pkg")
		if strings.HasSuffix(strings.ToLower(cand.URL), ".dll") {
			archivePath = filepath.Join(tmpDir, "libmpv-2.dll")
		} else if strings.HasSuffix(strings.ToLower(cand.URL), ".zip") {
			archivePath = filepath.Join(tmpDir, "mpv.zip")
		} else if strings.HasSuffix(strings.ToLower(cand.URL), ".7z") || strings.Contains(strings.ToLower(cand.URL), ".7z/") {
			archivePath = filepath.Join(tmpDir, "mpv.7z")
		}
		if err := m.downloadFile(ctx, cand, archivePath); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			lastErr = fmt.Errorf("%s: %w", cand.Label, err)
			continue
		}
		m.updateProgress(func(p *DownloadProgress) {
			p.Stage = "正在解压并校验 libmpv-2.dll…"
		})
		if err := m.installPackage(archivePath); err != nil {
			lastErr = fmt.Errorf("%s 校验失败: %w", cand.Label, err)
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("所有下载源均失败")
}

func (m *MPVManager) resolveLatestRelease(ctx context.Context) releaseRef {
	def := releaseRef{Tag: defaultMPVTag, Asset: defaultMPVAsset}
	apiURL := strings.TrimSpace(m.releaseAPIURL)
	if apiURL == "" {
		return def
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return def
	}
	req.Header.Set("User-Agent", "PureUtils-PureVideo")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return def
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return def
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return def
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return def
	}
	for _, a := range payload.Assets {
		lower := strings.ToLower(a.Name)
		if strings.HasPrefix(lower, "mpv-dev-x86_64-") &&
			!strings.Contains(lower, "v3") &&
			(strings.HasSuffix(lower, ".7z") || strings.HasSuffix(lower, ".zip")) {
			return releaseRef{Tag: payload.TagName, Asset: a.Name, DirectURL: strings.TrimSpace(a.BrowserDownloadURL)}
		}
	}
	return def
}

func buildMirrorCandidates(source, customURL string, rel releaseRef, prefixOverrides ...map[string]string) []mirrorCandidate {
	rawGH := strings.TrimSpace(rel.DirectURL)
	if rawGH == "" {
		rawGH = fmt.Sprintf("https://github.com/shinchiro/mpv-winbuild-cmake/releases/download/%s/%s", rel.Tag, rel.Asset)
	}
	sfURL := fmt.Sprintf("https://sourceforge.net/projects/mpv-player-windows/files/libmpv/%s/download", rel.Asset)

	ghProxyPrefix := "https://gh-proxy.org/"
	ghFastPrefix := "https://ghfast.top/"
	disableSF := false
	if len(prefixOverrides) > 0 && prefixOverrides[0] != nil {
		ov := prefixOverrides[0]
		if v, ok := ov[SourceGHProxy]; ok {
			ghProxyPrefix = v
		}
		if v, ok := ov[SourceGHFast]; ok {
			ghFastPrefix = v
		}
		if v, ok := ov[SourceSourceForge]; ok {
			if v == "" {
				disableSF = true
			} else {
				sfURL = v
			}
		}
	}

	makeCand := func(id string) mirrorCandidate {
		switch id {
		case SourceGHProxy:
			if ghProxyPrefix == "" {
				return mirrorCandidate{}
			}
			return mirrorCandidate{SourceGHProxy, DownloadSourceLabel(SourceGHProxy), ghProxyPrefix + rawGH}
		case SourceGHFast:
			if ghFastPrefix == "" {
				return mirrorCandidate{}
			}
			return mirrorCandidate{SourceGHFast, DownloadSourceLabel(SourceGHFast), ghFastPrefix + rawGH}
		case SourceSourceForge:
			if disableSF {
				return mirrorCandidate{}
			}
			return mirrorCandidate{SourceSourceForge, DownloadSourceLabel(SourceSourceForge), sfURL}
		case SourceGitHub:
			return mirrorCandidate{SourceGitHub, DownloadSourceLabel(SourceGitHub), rawGH}
		default:
			return mirrorCandidate{}
		}
	}

	var out []mirrorCandidate
	if custom := strings.TrimSpace(customURL); custom != "" {
		resolved := custom
		if strings.Contains(custom, "{url}") {
			resolved = strings.ReplaceAll(custom, "{url}", rawGH)
		} else {
			lc := strings.ToLower(custom)
			if !strings.HasSuffix(lc, ".7z") && !strings.HasSuffix(lc, ".zip") && !strings.HasSuffix(lc, ".dll") && !strings.HasSuffix(lc, "/download") {
				resolved = strings.TrimRight(custom, "/") + "/" + rawGH
			}
		}
		out = append(out, mirrorCandidate{
			SourceID: "custom",
			Label:    "自定义下载源",
			URL:      resolved,
		})
	}

	var order []string
	switch strings.ToLower(strings.TrimSpace(source)) {
	case SourceGHProxy:
		order = []string{SourceGHProxy, SourceGHFast, SourceSourceForge, SourceGitHub}
	case SourceGHFast:
		order = []string{SourceGHFast, SourceGHProxy, SourceSourceForge, SourceGitHub}
	case SourceSourceForge:
		order = []string{SourceSourceForge, SourceGHProxy, SourceGHFast, SourceGitHub}
	case SourceGitHub:
		order = []string{SourceGitHub, SourceSourceForge, SourceGHProxy, SourceGHFast}
	default:
		order = []string{SourceGHProxy, SourceGHFast, SourceSourceForge, SourceGitHub}
	}
	for _, id := range order {
		if c := makeCand(id); c.URL != "" {
			out = append(out, c)
		}
	}
	return out
}

func (m *MPVManager) downloadFile(ctx context.Context, cand mirrorCandidate, dst string) error {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, cand.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PureUtils-PureVideo")

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
	total := resp.ContentLength
	m.updateProgress(func(p *DownloadProgress) {
		p.Stage = "正在下载 " + cand.Label
		p.TotalBytes = total
		p.DoneBytes = 0
	})

	buf := make([]byte, 64*1024)
	var done int64
	lastTick := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				_ = out.Close()
				return werr
			}
			done += int64(n)
			if time.Since(lastTick) >= 120*time.Millisecond {
				d := done
				m.updateProgress(func(p *DownloadProgress) {
					p.DoneBytes = d
				})
				lastTick = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = out.Close()
			return readErr
		}
	}
	m.updateProgress(func(p *DownloadProgress) {
		p.DoneBytes = done
		if p.TotalBytes <= 0 {
			p.TotalBytes = done
		}
	})
	return out.Close()
}

// ImportLocal imports libmpv-2.dll from a user-supplied .dll, .zip, or .7z file.
func (m *MPVManager) ImportLocal(srcPath string) error {
	if strings.TrimSpace(m.pluginDir) == "" {
		return fmt.Errorf("未配置插件数据目录")
	}
	if err := os.MkdirAll(m.pluginDir, 0o755); err != nil {
		return err
	}
	return m.installPackage(srcPath)
}

func (m *MPVManager) installPackage(pkgPath string) error {
	tmpDLL := filepath.Join(m.pluginDir, fmt.Sprintf("libmpv-2.dll.tmp.%d", time.Now().UnixNano()))
	defer func() { _ = os.Remove(tmpDLL) }()

	kind, err := detectPackageKind(pkgPath)
	if err != nil {
		return err
	}
	switch kind {
	case "pe":
		if err := copyFileTo(pkgPath, tmpDLL); err != nil {
			return err
		}
	case "zip":
		if err := extractDLLFromZip(pkgPath, tmpDLL); err != nil {
			return err
		}
	case "7z":
		if err := extractDLLFrom7z(pkgPath, tmpDLL); err != nil {
			return err
		}
	default:
		return fmt.Errorf("不支持的文件格式（需为 libmpv-2.dll、.7z 或 .zip）")
	}

	if err := validatePEHeader(tmpDLL); err != nil {
		return err
	}
	if m.probeFn != nil {
		if _, err := m.probeFn(tmpDLL); err != nil {
			return err
		}
	}

	target := m.ManagedDLLPath()
	_ = os.Remove(target)
	if err := os.Rename(tmpDLL, target); err != nil {
		return fmt.Errorf("替换 %s 失败（请先关闭正在播放的视频窗口）: %w", filepath.Base(target), err)
	}
	return nil
}

// Remove deletes the managed libmpv-2.dll from pluginDir.
func (m *MPVManager) Remove() error {
	target := m.ManagedDLLPath()
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return nil
	}
	if err := os.Remove(target); err != nil {
		return fmt.Errorf("删除 %s 失败（请先关闭正在播放的视频窗口）: %w", filepath.Base(target), err)
	}
	return nil
}

func detectPackageKind(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var hdr [6]byte
	n, err := io.ReadFull(f, hdr[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	if n >= 2 && hdr[0] == 'M' && hdr[1] == 'Z' {
		return "pe", nil
	}
	if n >= 4 && hdr[0] == 'P' && hdr[1] == 'K' && (hdr[2] == 0x03 || hdr[2] == 0x05 || hdr[2] == 0x07) {
		return "zip", nil
	}
	if n == 6 && hdr[0] == '7' && hdr[1] == 'z' && hdr[2] == 0xBC && hdr[3] == 0xAF && hdr[4] == 0x27 && hdr[5] == 0x1C {
		return "7z", nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".7z":
		return "7z", nil
	case ".zip":
		return "zip", nil
	case ".dll":
		return "pe", nil
	default:
		return "", fmt.Errorf("文件内容不是有效的 .dll / .zip / .7z 安装包")
	}
}

func validatePEHeader(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var buf [64]byte
	n, err := io.ReadFull(f, buf[:])
	if err != nil || n < 64 || buf[0] != 'M' || buf[1] != 'Z' {
		return fmt.Errorf("提取的文件不是有效的 Windows DLL")
	}
	return nil
}

func copyFileTo(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func extractDLLFromZip(srcZip, dstDLL string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() || !strings.EqualFold(filepath.Base(f.Name), "libmpv-2.dll") {
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
	return fmt.Errorf("ZIP 压缩包中未找到 libmpv-2.dll")
}

func extractDLLFrom7z(src7z, dstDLL string) error {
	r, err := sevenzip.OpenReader(src7z)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() || !strings.EqualFold(filepath.Base(f.Name), "libmpv-2.dll") {
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
	return fmt.Errorf("7Z 压缩包中未找到 libmpv-2.dll")
}
