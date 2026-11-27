package collector

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

	"pureutils/pkg/purekit/i18n"
)

const (
	SensorSourceAuto    = "auto"
	SensorSourceGHProxy = "ghproxy"
	SensorSourceGHFast  = "ghfast"
	SensorSourceGitHub  = "github"

	defaultPawnIOSetupAPI     = "https://api.github.com/repos/namazso/PawnIO.Setup/releases/latest"
	defaultPawnIOModulesAPI   = "https://api.github.com/repos/namazso/PawnIO.Modules/releases/latest"
	fallbackPawnIOSetupURL    = "https://github.com/namazso/PawnIO.Setup/releases/latest/download/PawnIO_setup.exe"
	fallbackPawnIOModulesZIP  = "https://github.com/namazso/PawnIO.Modules/releases/latest/download/PawnIO.Modules.zip"
	fallbackIntelMSRBinURL    = "https://github.com/namazso/PawnIO.Modules/releases/latest/download/IntelMSR.bin"
	fallbackAMDFamily17BinURL = "https://github.com/namazso/PawnIO.Modules/releases/latest/download/AMDFamily17.bin"
)

// SensorSourceInfo describes one selectable mirror source for PawnIO assets.
type SensorSourceInfo struct {
	ID    string
	Label string
}

// SensorDownloadSources lists selectable sources in UI order.
var SensorDownloadSources = []SensorSourceInfo{
	{SensorSourceAuto, "Auto (Recommended)"},
	{SensorSourceGHProxy, "GitHub Mirror (gh-proxy.org)"},
	{SensorSourceGHFast, "GitHub Mirror (ghfast.top)"},
	{SensorSourceGitHub, "GitHub Direct"},
}

// SensorSourceLabel returns the localized label for id.
func SensorSourceLabel(id string) string {
	switch id {
	case SensorSourceGHProxy:
		return i18n.S("GitHub 加速 (gh-proxy.org)", "GitHub Mirror (gh-proxy.org)")
	case SensorSourceGHFast:
		return i18n.S("GitHub 加速 (ghfast.top)", "GitHub Mirror (ghfast.top)")
	case SensorSourceGitHub:
		return i18n.S("GitHub 官方直连", "GitHub Direct")
	default:
		return i18n.S("自动优选（推荐）", "Auto (Recommended)")
	}
}

// SensorSourceLabels returns all localized download source labels.
func SensorSourceLabels() []string {
	out := make([]string, len(SensorDownloadSources))
	for i, s := range SensorDownloadSources {
		out[i] = SensorSourceLabel(s.ID)
	}
	return out
}

// SensorSourceIDFromLabel resolves a source ID from a localized UI label.
func SensorSourceIDFromLabel(label string) string {
	switch label {
	case "GitHub 加速 (gh-proxy.org)", "GitHub Mirror (gh-proxy.org)":
		return SensorSourceGHProxy
	case "GitHub 加速 (ghfast.top)", "GitHub Mirror (ghfast.top)":
		return SensorSourceGHFast
	case "GitHub 官方直连", "GitHub Direct":
		return SensorSourceGitHub
	case "自动优选（推荐）", "Auto (Recommended)":
		return SensorSourceAuto
	default:
		return SensorSourceAuto
	}
}

// SensorExtStatus summarizes local and driver status of the optional PawnIO extension.
type SensorExtStatus struct {
	DriverInstalled bool
	SetupDownloaded bool
	SetupPath       string
	ModulesReady    bool
	ModuleCount     int
	SensorDir       string
}

// SensorDownloadProgress reports live PawnIO extension download progress.
type SensorDownloadProgress struct {
	Active      bool
	SourceID    string
	SourceLabel string
	Stage       string
	DoneBytes   int64
	TotalBytes  int64
	Err         string
	Completed   bool
}

type mirrorCandidate struct {
	SourceID string
	Label    string
	URL      string
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// SensorExtManager handles downloading, importing, installing, and removing optional
// PawnIO driver setup and signed hardware sensor modules inside `<pluginDir>\sensors`.
type SensorExtManager struct {
	sensorDir       string
	httpClient      *http.Client
	setupAPIURL     string
	modulesAPIURL   string
	mirrorPrefixes  map[string]string
	driverCheckFn   func() bool

	mu       sync.RWMutex
	progress SensorDownloadProgress
	cancel   context.CancelFunc
	onChange func()
}

// NewSensorExtManager manages optional PawnIO sensor files under pluginDir/sensors.
func NewSensorExtManager(pluginDir string) *SensorExtManager {
	sDir := ""
	if strings.TrimSpace(pluginDir) != "" {
		sDir = filepath.Join(pluginDir, "sensors")
	}
	return &SensorExtManager{
		sensorDir:     sDir,
		httpClient:    &http.Client{Timeout: 0},
		setupAPIURL:   defaultPawnIOSetupAPI,
		modulesAPIURL: defaultPawnIOModulesAPI,
	}
}

// SetOnChange registers a callback triggered when download progress changes.
func (m *SensorExtManager) SetOnChange(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

// SensorDir returns the local managed folder for PawnIO setup & module binaries.
func (m *SensorExtManager) SensorDir() string {
	return m.sensorDir
}

// SetupPath returns the expected local path to PawnIO_setup.exe.
func (m *SensorExtManager) SetupPath() string {
	if m.sensorDir == "" {
		return "PawnIO_setup.exe"
	}
	return filepath.Join(m.sensorDir, "PawnIO_setup.exe")
}

func (m *SensorExtManager) isDriverInstalled() bool {
	if m.driverCheckFn != nil {
		return m.driverCheckFn()
	}
	return IsPawnIODriverInstalled()
}

// Status reports current driver installation and downloaded asset status.
func (m *SensorExtManager) Status() SensorExtStatus {
	st := SensorExtStatus{
		SetupPath:       m.SetupPath(),
		SensorDir:       m.sensorDir,
		DriverInstalled: m.isDriverInstalled(),
	}
	if info, err := os.Stat(st.SetupPath); err == nil && !info.IsDir() && info.Size() > 1024 {
		st.SetupDownloaded = true
	}
	if m.sensorDir != "" {
		if entries, err := os.ReadDir(m.sensorDir); err == nil {
			for _, ent := range entries {
				if ent.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(ent.Name()))
				if ext == ".bin" || ext == ".amx" {
					st.ModuleCount++
				}
			}
		}
	}
	// Also count modules installed in Program Files\PawnIO when driver is installed.
	if st.ModuleCount == 0 {
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			for _, dir := range []string{filepath.Join(pf, "PawnIO"), filepath.Join(pf, "PawnIO", "modules")} {
				if entries, err := os.ReadDir(dir); err == nil {
					for _, ent := range entries {
						if !ent.IsDir() && (strings.EqualFold(filepath.Ext(ent.Name()), ".bin") || strings.EqualFold(filepath.Ext(ent.Name()), ".amx")) {
							st.ModuleCount++
						}
					}
				}
			}
		}
	}
	st.ModulesReady = st.ModuleCount > 0
	return st
}

// Progress returns the latest download progress snapshot.
func (m *SensorExtManager) Progress() SensorDownloadProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.progress
}

func (m *SensorExtManager) updateProgress(fn func(*SensorDownloadProgress)) {
	m.mu.Lock()
	fn(&m.progress)
	cb := m.onChange
	m.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// StartDownload begins background download of PawnIO_setup.exe and signed sensor modules.
func (m *SensorExtManager) StartDownload(source, customURL string) {
	m.mu.Lock()
	if m.progress.Active {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.progress = SensorDownloadProgress{
		Active:      true,
		SourceID:    source,
		SourceLabel: SensorSourceLabel(source),
		Stage:       i18n.S("正在解析 PawnIO 最新版本…", "Resolving latest PawnIO release…"),
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
				m.progress.Err = i18n.S("已取消下载", "Download canceled")
				m.progress.Stage = i18n.S("下载已取消", "Download canceled")
			} else {
				m.progress.Err = err.Error()
				m.progress.Stage = i18n.S("下载失败", "Download failed")
			}
			m.progress.Completed = false
		} else {
			m.progress.Err = ""
			m.progress.Stage = i18n.S("PawnIO 传感器扩展已下载就绪", "PawnIO sensor extension ready")
			m.progress.Completed = true
		}
		cb := m.onChange
		m.mu.Unlock()
		if cb != nil {
			cb()
		}
	}()
}

// CancelDownload aborts an active download.
func (m *SensorExtManager) CancelDownload() {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Download synchronously downloads PawnIO_setup.exe and sensor modules into sensorDir.
func (m *SensorExtManager) Download(ctx context.Context, source, customURL string) error {
	if strings.TrimSpace(m.sensorDir) == "" {
		return errors.New(i18n.S("未配置传感器扩展目录", "Sensor extension directory is not configured"))
	}
	if err := os.MkdirAll(m.sensorDir, 0o755); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(m.sensorDir, "dl-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// 1. Download PawnIO_setup.exe only when the PawnIO driver is not already installed locally.
	if !m.isDriverInstalled() {
		setupURL := m.resolveSetupURL(ctx)
		setupTmp := filepath.Join(tmpDir, "PawnIO_setup.exe")
		if err := m.downloadWithMirrors(ctx, source, customURL, setupURL, setupTmp, i18n.S("PawnIO 安装包", "PawnIO Installer")); err != nil {
			return err
		}
		if err := validateMZHeader(setupTmp); err != nil {
			return err
		}
		if err := atomicReplaceFile(setupTmp, m.SetupPath()); err != nil {
			return err
		}
	}

	// 2. Download signed PawnIO modules (.zip or individual .bin files)
	modAssets := m.resolveModuleAssets(ctx)
	var zipAsset string
	var binAssets []githubReleaseAsset
	for _, a := range modAssets {
		l := strings.ToLower(a.Name)
		switch {
		case strings.HasSuffix(l, ".zip"):
			if zipAsset == "" {
				zipAsset = a.BrowserDownloadURL
			}
		case strings.HasSuffix(l, ".bin") || strings.HasSuffix(l, ".amx"):
			if isDesiredModuleName(a.Name) {
				binAssets = append(binAssets, a)
			}
		}
	}

	if zipAsset != "" {
		zipTmp := filepath.Join(tmpDir, "modules.zip")
		if err := m.downloadWithMirrors(ctx, source, customURL, zipAsset, zipTmp, i18n.S("传感器模块包", "Sensor Modules")); err == nil {
			if exErr := extractSensorModulesFromZip(zipTmp, m.sensorDir); exErr == nil {
				return nil
			}
		}
	}
	if len(binAssets) == 0 {
		binAssets = []githubReleaseAsset{
			{Name: "IntelMSR.bin", BrowserDownloadURL: fallbackIntelMSRBinURL},
			{Name: "AMDFamily17.bin", BrowserDownloadURL: fallbackAMDFamily17BinURL},
		}
	}
	var gotAny bool
	var lastErr error
	for _, ba := range binAssets {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		tmpBin := filepath.Join(tmpDir, filepath.Base(ba.Name))
		if err := m.downloadWithMirrors(ctx, source, customURL, ba.BrowserDownloadURL, tmpBin, ba.Name); err != nil {
			lastErr = err
			continue
		}
		dst := filepath.Join(m.sensorDir, filepath.Base(ba.Name))
		if err := atomicReplaceFile(tmpBin, dst); err != nil {
			lastErr = err
			continue
		}
		gotAny = true
	}
	if !gotAny && lastErr != nil {
		return lastErr
	}
	return nil
}

// ImportLocal imports a user-supplied PawnIO_setup.exe, .bin/.amx module, or .zip archive.
func (m *SensorExtManager) ImportLocal(srcPath string) error {
	if strings.TrimSpace(m.sensorDir) == "" {
		return errors.New(i18n.S("未配置传感器扩展目录", "Sensor extension directory is not configured"))
	}
	if err := os.MkdirAll(m.sensorDir, 0o755); err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(srcPath))
	switch ext {
	case ".exe":
		if err := validateMZHeader(srcPath); err != nil {
			return err
		}
		return copyFileAtomic(srcPath, m.SetupPath())
	case ".bin", ".amx":
		info, err := os.Stat(srcPath)
		if err != nil || info.Size() < 16 {
			return errors.New(i18n.S("无效的 PawnIO 模块文件", "Invalid PawnIO module file"))
		}
		return copyFileAtomic(srcPath, filepath.Join(m.sensorDir, filepath.Base(srcPath)))
	case ".zip":
		return extractSensorModulesFromZip(srcPath, m.sensorDir)
	default:
		return errors.New(i18n.S("请选择 PawnIO_setup.exe、.zip 模块包或 .bin 模块文件", "Select PawnIO_setup.exe, a .zip bundle, or a .bin module file"))
	}
}

// RemoveLocalFiles deletes cached setup and module binaries in sensorDir.
func (m *SensorExtManager) RemoveLocalFiles() error {
	if m.sensorDir == "" {
		return nil
	}
	if err := os.RemoveAll(m.sensorDir); err != nil {
		return err
	}
	return nil
}

func isDesiredModuleName(name string) bool {
	l := strings.ToLower(filepath.Base(name))
	switch l {
	case "intelmsr.bin", "amdfamily17.bin", "ryzensmu.bin", "intelmsr.amx", "amdfamily17.amx":
		return true
	default:
		return strings.HasSuffix(l, ".bin") || strings.HasSuffix(l, ".amx")
	}
}

func (m *SensorExtManager) resolveSetupURL(ctx context.Context) string {
	assets := m.fetchReleaseAssets(ctx, m.setupAPIURL)
	for _, a := range assets {
		l := strings.ToLower(a.Name)
		if strings.HasSuffix(l, ".exe") && strings.Contains(l, "pawnio") {
			return strings.TrimSpace(a.BrowserDownloadURL)
		}
	}
	return fallbackPawnIOSetupURL
}

func (m *SensorExtManager) resolveModuleAssets(ctx context.Context) []githubReleaseAsset {
	assets := m.fetchReleaseAssets(ctx, m.modulesAPIURL)
	if len(assets) > 0 {
		return assets
	}
	return []githubReleaseAsset{
		{Name: "PawnIO.Modules.zip", BrowserDownloadURL: fallbackPawnIOModulesZIP},
	}
}

func (m *SensorExtManager) fetchReleaseAssets(ctx context.Context, apiURL string) []githubReleaseAsset {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "PureUtils-PureMonitor")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var payload struct {
		Assets []githubReleaseAsset `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil
	}
	return payload.Assets
}

func buildSensorMirrorCandidates(source, customURL, rawGH string, overrides ...map[string]string) []mirrorCandidate {
	ghProxyPrefix := "https://gh-proxy.org/"
	ghFastPrefix := "https://ghfast.top/"
	if len(overrides) > 0 && overrides[0] != nil {
		if v, ok := overrides[0][SensorSourceGHProxy]; ok {
			ghProxyPrefix = v
		}
		if v, ok := overrides[0][SensorSourceGHFast]; ok {
			ghFastPrefix = v
		}
	}
	makeCand := func(id string) mirrorCandidate {
		switch id {
		case SensorSourceGHProxy:
			if ghProxyPrefix == "" {
				return mirrorCandidate{}
			}
			return mirrorCandidate{SensorSourceGHProxy, SensorSourceLabel(SensorSourceGHProxy), ghProxyPrefix + rawGH}
		case SensorSourceGHFast:
			if ghFastPrefix == "" {
				return mirrorCandidate{}
			}
			return mirrorCandidate{SensorSourceGHFast, SensorSourceLabel(SensorSourceGHFast), ghFastPrefix + rawGH}
		case SensorSourceGitHub:
			return mirrorCandidate{SensorSourceGitHub, SensorSourceLabel(SensorSourceGitHub), rawGH}
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
			if !strings.HasSuffix(lc, ".exe") && !strings.HasSuffix(lc, ".zip") && !strings.HasSuffix(lc, ".bin") {
				resolved = strings.TrimRight(custom, "/") + "/" + rawGH
			}
		}
		out = append(out, mirrorCandidate{
			SourceID: "custom",
			Label:    i18n.S("自定义下载源", "Custom Download Source"),
			URL:      resolved,
		})
	}

	var order []string
	switch strings.ToLower(strings.TrimSpace(source)) {
	case SensorSourceGHProxy:
		order = []string{SensorSourceGHProxy, SensorSourceGHFast, SensorSourceGitHub}
	case SensorSourceGHFast:
		order = []string{SensorSourceGHFast, SensorSourceGHProxy, SensorSourceGitHub}
	case SensorSourceGitHub:
		order = []string{SensorSourceGitHub, SensorSourceGHProxy, SensorSourceGHFast}
	default:
		order = []string{SensorSourceGHProxy, SensorSourceGHFast, SensorSourceGitHub}
	}
	for _, id := range order {
		if c := makeCand(id); c.URL != "" {
			out = append(out, c)
		}
	}
	return out
}

func (m *SensorExtManager) downloadWithMirrors(ctx context.Context, source, customURL, rawURL, dst, itemLabel string) error {
	cands := buildSensorMirrorCandidates(source, customURL, rawURL, m.mirrorPrefixes)
	var lastErr error
	for _, c := range cands {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.updateProgress(func(p *SensorDownloadProgress) {
			p.SourceID = c.SourceID
			p.SourceLabel = c.Label
			p.Stage = i18n.F("正在下载 %s (%s)…", "Downloading %s (%s)…", itemLabel, c.Label)
			p.DoneBytes = 0
			p.TotalBytes = 0
		})
		if err := m.downloadFile(ctx, c.URL, dst); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			lastErr = fmt.Errorf("%s: %w", c.Label, err)
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New(i18n.S("所有下载源均失败", "All download sources failed"))
}

func (m *SensorExtManager) downloadFile(ctx context.Context, urlStr, dst string) error {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PureUtils-PureMonitor")
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
	m.updateProgress(func(p *SensorDownloadProgress) {
		p.TotalBytes = total
		p.DoneBytes = 0
	})
	buf := make([]byte, 32*1024)
	var done int64
	lastTick := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				_ = out.Close()
				return werr
			}
			done += int64(n)
			if time.Since(lastTick) >= 120*time.Millisecond {
				d := done
				m.updateProgress(func(p *SensorDownloadProgress) { p.DoneBytes = d })
				lastTick = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = out.Close()
			return rerr
		}
	}
	m.updateProgress(func(p *SensorDownloadProgress) {
		p.DoneBytes = done
		if p.TotalBytes <= 0 {
			p.TotalBytes = done
		}
	})
	return out.Close()
}

func validateMZHeader(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var hdr [64]byte
	n, err := io.ReadFull(f, hdr[:])
	if err != nil || n < 64 || hdr[0] != 'M' || hdr[1] != 'Z' {
		return errors.New(i18n.S("文件不是有效的 Windows 可执行文件", "File is not a valid Windows executable"))
	}
	return nil
}

func atomicReplaceFile(tmpSrc, dst string) error {
	_ = os.Remove(dst)
	return os.Rename(tmpSrc, dst)
}

func copyFileAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return atomicReplaceFile(tmp, dst)
}

func extractSensorModulesFromZip(srcZip, dstDir string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()
	extracted := 0
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".bin" && ext != ".amx" && !strings.EqualFold(base, "PawnIO_setup.exe") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		tmp := filepath.Join(dstDir, base+".tmp")
		out, err := os.Create(tmp)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, cpErr := io.Copy(out, rc)
		closeErr := out.Close()
		_ = rc.Close()
		if cpErr != nil {
			_ = os.Remove(tmp)
			return cpErr
		}
		if closeErr != nil {
			_ = os.Remove(tmp)
			return closeErr
		}
		if err := atomicReplaceFile(tmp, filepath.Join(dstDir, base)); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		extracted++
	}
	if extracted == 0 {
		return errors.New(i18n.S("ZIP 压缩包中未找到 PawnIO 模块或安装文件", "No PawnIO module or installer found in ZIP archive"))
	}
	return nil
}
