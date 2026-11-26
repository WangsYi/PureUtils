package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
)

// UpdateStatus describes the lifecycle state of PureUtils self-update.
type UpdateStatus string

const (
	UpdateIdle        UpdateStatus = "idle"
	UpdateChecking    UpdateStatus = "checking"
	UpdateUpToDate    UpdateStatus = "up_to_date"
	UpdateAvailable   UpdateStatus = "available"
	UpdateDownloading UpdateStatus = "downloading"
	UpdateReady       UpdateStatus = "ready"
	UpdateInstalling  UpdateStatus = "installing"
	UpdateError       UpdateStatus = "error"
)

const (
	UpdateSourceAuto    = "auto"
	UpdateSourceGHProxy = "ghproxy"
	UpdateSourceGHFast  = "ghfast"
	UpdateSourceGitHub  = "github"

	defaultUpdateMinBytes int64 = 1 << 20 // 1 MiB minimum valid setup/binary payload
)

// UpdateSourceInfo describes one selectable mirror source for GitHub releases.
type UpdateSourceInfo struct {
	ID    string
	Label string
	Hint  string
}

// UpdateDownloadSources lists selectable update download mirrors in UI display order.
var UpdateDownloadSources = []UpdateSourceInfo{
	{UpdateSourceAuto, "自动优选（推荐）", "按顺序尝试国内 GitHub 镜像与官方源"},
	{UpdateSourceGHProxy, "GitHub 加速 (gh-proxy.org)", "国内代理加速 GitHub Release"},
	{UpdateSourceGHFast, "GitHub 加速 (ghfast.top)", "备用国内 GitHub Release 镜像"},
	{UpdateSourceGitHub, "GitHub 官方直连", "直连 github.com/WangsYi/PureUtils"},
}

// UpdateSourceLabel returns the Chinese UI label for an update source ID.
func UpdateSourceLabel(id string) string {
	for _, s := range UpdateDownloadSources {
		if s.ID == id {
			return s.Label
		}
	}
	return UpdateDownloadSources[0].Label
}

// UpdateSourceIDFromLabel resolves an update source ID from its UI label.
func UpdateSourceIDFromLabel(label string) string {
	for _, s := range UpdateDownloadSources {
		if s.Label == label {
			return s.ID
		}
	}
	return UpdateSourceAuto
}

// ReleaseInfo captures the resolved target GitHub release and its selected asset.
type ReleaseInfo struct {
	TagName     string `json:"tagName"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"htmlUrl"`
	PublishedAt string `json:"publishedAt"`
	AssetName   string `json:"assetName"`
	AssetURL    string `json:"assetUrl"`
	AssetSize   int64  `json:"assetSize"`
	AssetSHA256 string `json:"assetSha256,omitempty"`
	IsInstaller bool   `json:"isInstaller"`
}

// UpdateSnapshot is an immutable snapshot of updater progress and state for UI/tray rendering.
type UpdateSnapshot struct {
	Status         UpdateStatus
	CurrentVersion string
	Release        *ReleaseInfo
	DownloadedPath string
	SourceID       string
	SourceLabel    string
	Stage          string
	DoneBytes      int64
	TotalBytes     int64
	LastCheck      time.Time
	Err            string
	Background     bool
}

// UpdaterOptions configures a PureUtils Updater instance.
type UpdaterOptions struct {
	CurrentVersion    string
	RepoOwner         string
	RepoName          string
	DataDir           string
	ExePath           string
	HTTPClient        *http.Client
	APIURL            string
	ReleasePageURL    string
	MirrorPrefixes    map[string]string
	MinAssetBytes     int64
	LaunchInstallerFn func(installerPath, currentExe string) error
	OnChange          func(UpdateSnapshot)
	OnNotify          func(title, body string)
	OnQuit            func()
	Log               *core.Logger
}

type updaterDiskState struct {
	LastCheck       time.Time `json:"lastCheck,omitempty"`
	NotifiedVersion string    `json:"notifiedVersion,omitempty"`
	ReadyVersion    string    `json:"readyVersion,omitempty"`
}

type updateMirrorCandidate struct {
	SourceID string
	Label    string
	URL      string
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubReleasePayload struct {
	TagName     string               `json:"tag_name"`
	Name        string               `json:"name"`
	Body        string               `json:"body"`
	HTMLURL     string               `json:"html_url"`
	PublishedAt string               `json:"published_at"`
	Assets      []githubReleaseAsset `json:"assets"`
}

var defaultUpdateMirrorPrefixes = map[string]string{
	UpdateSourceGHProxy: "https://gh-proxy.org/",
	UpdateSourceGHFast:  "https://ghfast.top/",
}

// Updater handles GitHub Release checking, multi-mirror downloading, SHA-256 verification,
// and launching in-place upgrades.
type Updater struct {
	currentVersion    string
	repoOwner         string
	repoName          string
	dataDir           string
	exePath           string
	httpClient        *http.Client
	apiURL            string
	releasePageURL    string
	mirrorPrefixes    map[string]string
	minAssetBytes     int64
	launchInstallerFn func(installerPath, currentExe string) error
	onChange          func(UpdateSnapshot)
	onNotify          func(title, body string)
	onQuit            func()
	log               *core.Logger

	mu          sync.RWMutex
	snap        UpdateSnapshot
	state       updaterDiskState
	checkCancel context.CancelFunc
	dlCancel    context.CancelFunc
	lastCfg     config.UpdateConfig
}

// NewUpdater creates an Updater with optional test overrides.
func NewUpdater(opts UpdaterOptions) *Updater {
	ver := strings.TrimSpace(opts.CurrentVersion)
	if ver == "" {
		ver = core.AppVersion
	}
	owner := strings.TrimSpace(opts.RepoOwner)
	if owner == "" {
		owner = core.DefaultRepoOwner
	}
	repo := strings.TrimSpace(opts.RepoName)
	if repo == "" {
		repo = core.DefaultRepoName
	}
	apiURL := strings.TrimSpace(opts.APIURL)
	if apiURL == "" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	}
	pageURL := strings.TrimSpace(opts.ReleasePageURL)
	if pageURL == "" {
		pageURL = fmt.Sprintf("https://github.com/%s/%s/releases/latest", owner, repo)
	}
	prefixes := make(map[string]string, len(defaultUpdateMirrorPrefixes))
	for k, v := range defaultUpdateMirrorPrefixes {
		prefixes[k] = v
	}
	for k, v := range opts.MirrorPrefixes {
		if v == "" {
			delete(prefixes, k)
		} else {
			prefixes[k] = v
		}
	}
	minBytes := opts.MinAssetBytes
	if minBytes <= 0 {
		minBytes = defaultUpdateMinBytes
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	launchFn := opts.LaunchInstallerFn
	if launchFn == nil {
		launchFn = launchUpdateInstaller
	}

	u := &Updater{
		currentVersion:    ver,
		repoOwner:         owner,
		repoName:          repo,
		dataDir:           opts.DataDir,
		exePath:           opts.ExePath,
		httpClient:        client,
		apiURL:            apiURL,
		releasePageURL:    pageURL,
		mirrorPrefixes:    prefixes,
		minAssetBytes:     minBytes,
		launchInstallerFn: launchFn,
		onChange:          opts.OnChange,
		onNotify:          opts.OnNotify,
		onQuit:            opts.OnQuit,
		log:               opts.Log,
		lastCfg:           config.DefaultHostConfig().Update,
		snap: UpdateSnapshot{
			Status:         UpdateIdle,
			CurrentVersion: ver,
		},
	}
	u.loadState()
	u.snap.LastCheck = u.state.LastCheck
	return u
}

func (u *Updater) stateFilePath() string {
	if strings.TrimSpace(u.dataDir) == "" {
		return ""
	}
	return filepath.Join(u.dataDir, "state.json")
}

func (u *Updater) loadState() {
	p := u.stateFilePath()
	if p == "" {
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var st updaterDiskState
	if json.Unmarshal(data, &st) == nil {
		u.state = st
	}
}

func (u *Updater) saveStateLocked() {
	p := u.stateFilePath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(u.dataDir, 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(u.state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o600)
}

// Snapshot returns a copy of the current updater state.
func (u *Updater) Snapshot() UpdateSnapshot {
	u.mu.RLock()
	defer u.mu.RUnlock()
	cp := u.snap
	if u.snap.Release != nil {
		relCopy := *u.snap.Release
		cp.Release = &relCopy
	}
	return cp
}

// ShouldAutoCheck reports whether interval has elapsed since LastCheck.
func (u *Updater) ShouldAutoCheck(interval time.Duration, now time.Time) bool {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	u.mu.RLock()
	defer u.mu.RUnlock()
	if u.snap.Status == UpdateChecking || u.snap.Status == UpdateDownloading || u.snap.Status == UpdateInstalling {
		return false
	}
	if u.state.LastCheck.IsZero() {
		return true
	}
	return now.Sub(u.state.LastCheck) >= interval
}

func (u *Updater) emitChange() {
	if u.onChange != nil {
		u.onChange(u.Snapshot())
	}
}

func (u *Updater) resolvedExePath() string {
	if strings.TrimSpace(u.exePath) != "" {
		return u.exePath
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return ""
}

// StartCheck starts an asynchronous release check.
func (u *Updater) StartCheck(cfg config.UpdateConfig, manual bool) {
	u.mu.Lock()
	if u.snap.Status == UpdateChecking || u.snap.Status == UpdateDownloading || u.snap.Status == UpdateInstalling {
		u.mu.Unlock()
		return
	}
	if u.checkCancel != nil {
		u.checkCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.checkCancel = cancel
	u.lastCfg = cfg
	u.mu.Unlock()

	go func() {
		_, _ = u.Check(ctx, cfg, manual)
	}()
}

// Check synchronously queries GitHub Releases (with HTML redirect fallback) and updates state.
func (u *Updater) Check(ctx context.Context, cfg config.UpdateConfig, manual bool) (UpdateSnapshot, error) {
	u.mu.Lock()
	if u.snap.Status == UpdateDownloading || u.snap.Status == UpdateInstalling {
		snap := u.Snapshot()
		u.mu.Unlock()
		return snap, fmt.Errorf("当前正在下载或安装更新")
	}
	u.lastCfg = cfg
	u.snap.Status = UpdateChecking
	u.snap.Err = ""
	u.snap.Background = !manual
	u.snap.Stage = "正在检查新版本…"
	u.mu.Unlock()
	u.emitChange()

	checkCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	rel, err := u.fetchLatestRelease(checkCtx, cfg.DownloadSource)
	if err != nil {
		u.mu.Lock()
		u.checkCancel = nil
		if errors.Is(err, context.Canceled) {
			u.snap.Status = UpdateIdle
			u.snap.Stage = ""
			u.snap.Err = ""
		} else {
			u.snap.Status = UpdateError
			u.snap.Stage = "检查更新失败"
			u.snap.Err = err.Error()
		}
		u.mu.Unlock()
		u.emitChange()
		return u.Snapshot(), err
	}

	now := time.Now()
	u.mu.Lock()
	u.checkCancel = nil
	u.state.LastCheck = now
	u.snap.LastCheck = now
	u.snap.Err = ""
	u.snap.Release = &rel
	u.saveStateLocked()

	if !core.IsNewerVersion(u.currentVersion, rel.TagName) {
		u.snap.Status = UpdateUpToDate
		u.snap.Stage = "当前已是最新版本 " + u.currentVersion
		u.snap.DownloadedPath = ""
		u.mu.Unlock()
		u.emitChange()
		return u.Snapshot(), nil
	}

	// Check if already downloaded & verified on disk.
	if cached, ok := u.existingValidDownloadLocked(rel); ok {
		u.snap.Status = UpdateReady
		u.snap.DownloadedPath = cached
		u.snap.Stage = fmt.Sprintf("新版本 %s 已下载就绪", rel.TagName)
		shouldNotify := !manual && rel.TagName != cfg.SkippedVersion && u.state.ReadyVersion != rel.TagName
		if shouldNotify {
			u.state.ReadyVersion = rel.TagName
			u.saveStateLocked()
		}
		u.mu.Unlock()
		u.emitChange()
		if shouldNotify && u.onNotify != nil {
			u.onNotify("PureUtils "+rel.TagName+" 已下载就绪", "可在「设置 → 高级」或托盘菜单中立即安装并重启。")
		}
		return u.Snapshot(), nil
	}

	u.snap.Status = UpdateAvailable
	u.snap.DownloadedPath = ""
	u.snap.Stage = fmt.Sprintf("发现新版本 %s", rel.TagName)
	shouldAutoDL := !manual && cfg.AutoDownload && rel.TagName != cfg.SkippedVersion && rel.AssetURL != ""
	shouldNotifyAvail := !manual && !shouldAutoDL && rel.TagName != cfg.SkippedVersion && u.state.NotifiedVersion != rel.TagName
	if shouldNotifyAvail {
		u.state.NotifiedVersion = rel.TagName
		u.saveStateLocked()
	}
	u.mu.Unlock()
	u.emitChange()

	if shouldAutoDL {
		u.StartDownload(cfg.DownloadSource)
	} else if shouldNotifyAvail && u.onNotify != nil {
		u.onNotify("PureUtils 发现新版本 "+rel.TagName, "可在「设置 → 高级」中查看更新说明并一键下载安装。")
	}
	return u.Snapshot(), nil
}

func (u *Updater) existingValidDownloadLocked(rel ReleaseInfo) (string, bool) {
	if strings.TrimSpace(u.dataDir) == "" || strings.TrimSpace(rel.AssetName) == "" {
		return "", false
	}
	p := filepath.Join(u.dataDir, rel.AssetName)
	if err := u.verifyDownloadedFile(p, rel.AssetSHA256); err != nil {
		return "", false
	}
	return p, true
}

func (u *Updater) fetchLatestRelease(ctx context.Context, source string) (ReleaseInfo, error) {
	if rel, err := u.fetchLatestViaAPI(ctx); err == nil {
		return rel, nil
	} else {
		fallback, fbErr := u.fetchLatestViaRedirect(ctx, source)
		if fbErr == nil {
			return fallback, nil
		}
		return ReleaseInfo{}, err
	}
}

func (u *Updater) fetchLatestViaAPI(ctx context.Context) (ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.apiURL, nil)
	if err != nil {
		return ReleaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "PureUtils/"+core.ShortVersion(u.currentVersion))

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("连接 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ReleaseInfo{}, fmt.Errorf("GitHub API 返回状态 %d", resp.StatusCode)
	}
	var payload githubReleasePayload
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return ReleaseInfo{}, fmt.Errorf("解析版本信息失败: %w", err)
	}
	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		return ReleaseInfo{}, fmt.Errorf("版本响应缺少 tag_name")
	}

	asset, ok := selectReleaseAsset(payload.Assets, tag, u.resolvedExePath())
	if !ok {
		return ReleaseInfo{}, fmt.Errorf("新版本 %s 未提供可用的 Windows 安装包或可执行文件", tag)
	}
	sha := parseSHA256Digest(asset.Digest)
	if sha == "" {
		if sumURL := findChecksumsAssetURL(payload.Assets); sumURL != "" {
			if fetched, err := u.fetchChecksumForAsset(ctx, sumURL, asset.Name); err == nil {
				sha = fetched
			}
		}
	}
	htmlURL := strings.TrimSpace(payload.HTMLURL)
	if htmlURL == "" {
		htmlURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", u.repoOwner, u.repoName, tag)
	}
	return ReleaseInfo{
		TagName:     tag,
		Version:     core.ShortVersion(tag),
		Name:        strings.TrimSpace(payload.Name),
		Body:        strings.TrimSpace(payload.Body),
		HTMLURL:     htmlURL,
		PublishedAt: payload.PublishedAt,
		AssetName:   asset.Name,
		AssetURL:    asset.BrowserDownloadURL,
		AssetSize:   asset.Size,
		AssetSHA256: sha,
		IsInstaller: isInstallerAssetName(asset.Name),
	}, nil
}

var releaseTagRegex = regexp.MustCompile(`/releases/tag/([^/?#"'\s<>]+)`)

func (u *Updater) fetchLatestViaRedirect(ctx context.Context, source string) (ReleaseInfo, error) {
	candidates := buildUpdateMirrorCandidates(source, u.releasePageURL, u.mirrorPrefixes)
	noRedirectClient := *u.httpClient
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	var lastErr error
	for _, cand := range candidates {
		if ctx.Err() != nil {
			return ReleaseInfo{}, ctx.Err()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cand.URL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "PureUtils/"+core.ShortVersion(u.currentVersion))
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		loc := resp.Header.Get("Location")
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		resp.Body.Close()

		var tag string
		if m := releaseTagRegex.FindStringSubmatch(loc); len(m) == 2 {
			tag = strings.TrimSpace(m[1])
		} else if resp.Request != nil && resp.Request.URL != nil {
			if m := releaseTagRegex.FindStringSubmatch(resp.Request.URL.Path); len(m) == 2 {
				tag = strings.TrimSpace(m[1])
			}
		}
		if tag == "" && resp.StatusCode == http.StatusOK {
			if m := releaseTagRegex.FindStringSubmatch(string(bodyBytes)); len(m) == 2 {
				tag = strings.TrimSpace(m[1])
			}
		}
		if tag == "" {
			lastErr = fmt.Errorf("未从 %s 解析出最新版本号 (HTTP %d)", cand.Label, resp.StatusCode)
			continue
		}
		short := core.ShortVersion(tag)
		assetName := fmt.Sprintf("PureUtils-%s-setup.exe", short)
		assetURL := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", u.repoOwner, u.repoName, tag, assetName)
		htmlURL := fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", u.repoOwner, u.repoName, tag)
		return ReleaseInfo{
			TagName:     tag,
			Version:     short,
			Name:        "PureUtils " + tag,
			HTMLURL:     htmlURL,
			AssetName:   assetName,
			AssetURL:    assetURL,
			IsInstaller: true,
		}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("检查更新失败")
	}
	return ReleaseInfo{}, lastErr
}

func isInstallerAssetName(name string) bool {
	ln := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(ln, "setup.exe")
}

func selectReleaseAsset(assets []githubReleaseAsset, tag, exePath string) (githubReleaseAsset, bool) {
	short := strings.ToLower(core.ShortVersion(tag))
	// Prefer Inno Setup installer first (PureUtils-<ver>-setup.exe, then *-setup.exe).
	for _, a := range assets {
		ln := strings.ToLower(strings.TrimSpace(a.Name))
		if strings.HasPrefix(ln, "pureutils-") && strings.HasSuffix(ln, "-setup.exe") {
			if short == "" || strings.Contains(ln, short) {
				return a, true
			}
		}
	}
	for _, a := range assets {
		if isInstallerAssetName(a.Name) {
			return a, true
		}
	}
	// Fallback to standalone executable if no installer asset is attached.
	base := strings.ToLower(filepath.Base(exePath))
	for _, a := range assets {
		ln := strings.ToLower(strings.TrimSpace(a.Name))
		if (base != "" && ln == base) || ln == "pureutils.exe" {
			return a, true
		}
	}
	return githubReleaseAsset{}, false
}

func parseSHA256Digest(digest string) string {
	digest = strings.TrimSpace(digest)
	if rest, ok := strings.CutPrefix(strings.ToLower(digest), "sha256:"); ok {
		rest = strings.TrimSpace(rest)
		if len(rest) == 64 {
			if _, err := hex.DecodeString(rest); err == nil {
				return rest
			}
		}
	}
	return ""
}

func findChecksumsAssetURL(assets []githubReleaseAsset) string {
	for _, a := range assets {
		ln := strings.ToLower(strings.TrimSpace(a.Name))
		if ln == "sha256sums" || ln == "sha256sums.txt" || strings.HasSuffix(ln, ".sha256") {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

func (u *Updater) fetchChecksumForAsset(ctx context.Context, urlStr, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "PureUtils/"+core.ShortVersion(u.currentVersion))
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", err
	}
	target := strings.ToLower(strings.TrimSpace(assetName))
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		hash := strings.ToLower(strings.TrimSpace(fields[0]))
		if len(hash) != 64 {
			continue
		}
		if _, err := hex.DecodeString(hash); err != nil {
			continue
		}
		if len(fields) == 1 || strings.EqualFold(strings.TrimPrefix(fields[1], "*"), target) {
			return hash, nil
		}
	}
	return "", fmt.Errorf("未找到 %s 校验值", assetName)
}

func buildUpdateMirrorCandidates(source, targetURL string, prefixes map[string]string) []updateMirrorCandidate {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return nil
	}
	wrap := func(id string) (updateMirrorCandidate, bool) {
		switch id {
		case UpdateSourceGHProxy:
			p := strings.TrimSpace(prefixes[UpdateSourceGHProxy])
			if p == "" {
				return updateMirrorCandidate{}, false
			}
			if !strings.HasSuffix(p, "/") {
				p += "/"
			}
			return updateMirrorCandidate{id, UpdateSourceLabel(id), p + targetURL}, true
		case UpdateSourceGHFast:
			p := strings.TrimSpace(prefixes[UpdateSourceGHFast])
			if p == "" {
				return updateMirrorCandidate{}, false
			}
			if !strings.HasSuffix(p, "/") {
				p += "/"
			}
			return updateMirrorCandidate{id, UpdateSourceLabel(id), p + targetURL}, true
		default:
			return updateMirrorCandidate{UpdateSourceGitHub, UpdateSourceLabel(UpdateSourceGitHub), targetURL}, true
		}
	}

	var order []string
	switch source {
	case UpdateSourceGHProxy:
		order = []string{UpdateSourceGHProxy, UpdateSourceGitHub}
	case UpdateSourceGHFast:
		order = []string{UpdateSourceGHFast, UpdateSourceGitHub}
	case UpdateSourceGitHub:
		order = []string{UpdateSourceGitHub}
	default:
		order = []string{UpdateSourceGHProxy, UpdateSourceGHFast, UpdateSourceGitHub}
	}

	out := make([]updateMirrorCandidate, 0, len(order))
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		cand, ok := wrap(id)
		if !ok || seen[cand.URL] {
			continue
		}
		seen[cand.URL] = true
		out = append(out, cand)
	}
	return out
}

// StartDownload begins asynchronous download of the currently discovered release.
func (u *Updater) StartDownload(source string) {
	u.mu.Lock()
	if u.snap.Status == UpdateDownloading || u.snap.Status == UpdateInstalling || u.snap.Release == nil {
		u.mu.Unlock()
		return
	}
	if strings.TrimSpace(source) == "" {
		source = u.lastCfg.DownloadSource
	}
	if strings.TrimSpace(source) == "" {
		source = UpdateSourceAuto
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.dlCancel = cancel
	u.mu.Unlock()

	go func() {
		_ = u.Download(ctx, source)
	}()
}

// CancelDownload aborts an active update download.
func (u *Updater) CancelDownload() {
	u.mu.Lock()
	cancel := u.dlCancel
	u.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Download synchronously downloads and verifies the release asset into DataDir.
func (u *Updater) Download(ctx context.Context, source string) error {
	u.mu.Lock()
	if u.snap.Release == nil || strings.TrimSpace(u.snap.Release.AssetURL) == "" {
		u.mu.Unlock()
		return fmt.Errorf("未发现可下载的新版本")
	}
	rel := *u.snap.Release
	if strings.TrimSpace(u.dataDir) == "" {
		u.mu.Unlock()
		return fmt.Errorf("未配置更新缓存目录")
	}
	if err := os.MkdirAll(u.dataDir, 0o755); err != nil {
		u.mu.Unlock()
		return err
	}
	bg := u.snap.Background
	u.snap.Status = UpdateDownloading
	u.snap.Err = ""
	u.snap.DoneBytes = 0
	u.snap.TotalBytes = rel.AssetSize
	u.snap.SourceID = source
	u.snap.SourceLabel = UpdateSourceLabel(source)
	u.snap.Stage = "准备下载 " + rel.AssetName + "…"
	u.mu.Unlock()
	u.emitChange()

	candidates := buildUpdateMirrorCandidates(source, rel.AssetURL, u.mirrorPrefixes)
	if len(candidates) == 0 {
		return fmt.Errorf("未找到可用的更新下载源")
	}

	targetPath := filepath.Join(u.dataDir, rel.AssetName)
	partPath := targetPath + ".part"
	defer func() { _ = os.Remove(partPath) }()

	var lastErr error
	for _, cand := range candidates {
		if ctx.Err() != nil {
			lastErr = ctx.Err()
			break
		}
		u.mu.Lock()
		u.snap.SourceID = cand.SourceID
		u.snap.SourceLabel = cand.Label
		u.snap.Stage = "正在连接 " + cand.Label + "…"
		u.snap.DoneBytes = 0
		u.snap.TotalBytes = rel.AssetSize
		u.mu.Unlock()
		u.emitChange()

		if err := u.downloadOneCandidate(ctx, cand, rel, partPath); err != nil {
			lastErr = err
			_ = os.Remove(partPath)
			if errors.Is(err, context.Canceled) {
				break
			}
			continue
		}
		if err := u.verifyDownloadedFile(partPath, rel.AssetSHA256); err != nil {
			lastErr = err
			_ = os.Remove(partPath)
			continue
		}
		_ = os.Remove(targetPath)
		if err := os.Rename(partPath, targetPath); err != nil {
			lastErr = fmt.Errorf("写入安装包失败: %w", err)
			continue
		}

		u.mu.Lock()
		u.dlCancel = nil
		u.snap.Status = UpdateReady
		u.snap.DownloadedPath = targetPath
		u.snap.Err = ""
		u.snap.Stage = fmt.Sprintf("新版本 %s 已下载就绪", rel.TagName)
		shouldNotify := bg && u.state.ReadyVersion != rel.TagName
		if shouldNotify {
			u.state.ReadyVersion = rel.TagName
			u.saveStateLocked()
		}
		u.mu.Unlock()
		u.emitChange()
		if shouldNotify && u.onNotify != nil {
			u.onNotify("PureUtils "+rel.TagName+" 已下载就绪", "可在「设置 → 高级」或托盘菜单中立即安装并重启。")
		}
		return nil
	}

	u.mu.Lock()
	u.dlCancel = nil
	if errors.Is(lastErr, context.Canceled) || ctx.Err() != nil {
		if u.snap.Release != nil {
			u.snap.Status = UpdateAvailable
		} else {
			u.snap.Status = UpdateIdle
		}
		u.snap.Stage = "已取消下载"
		u.snap.Err = ""
		lastErr = context.Canceled
	} else {
		u.snap.Status = UpdateError
		u.snap.Stage = "下载更新失败"
		if lastErr != nil {
			u.snap.Err = lastErr.Error()
		} else {
			u.snap.Err = "全部下载源均不可用"
			lastErr = errors.New(u.snap.Err)
		}
	}
	u.mu.Unlock()
	u.emitChange()
	return lastErr
}

func (u *Updater) downloadOneCandidate(ctx context.Context, cand updateMirrorCandidate, rel ReleaseInfo, partPath string) error {
	dlCtx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, cand.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PureUtils/"+core.ShortVersion(u.currentVersion))

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s 请求失败: %w", cand.Label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s 返回 HTTP %d", cand.Label, resp.StatusCode)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = rel.AssetSize
	}

	f, err := os.Create(partPath)
	if err != nil {
		return err
	}
	defer f.Close()

	u.mu.Lock()
	u.snap.TotalBytes = total
	u.snap.Stage = "正在从 " + cand.Label + " 下载…"
	u.mu.Unlock()
	u.emitChange()

	buf := make([]byte, 64*1024)
	var done int64
	lastEmit := time.Now()
	for {
		if dlCtx.Err() != nil {
			return dlCtx.Err()
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if time.Since(lastEmit) >= 80*time.Millisecond || rerr == io.EOF {
				u.mu.Lock()
				u.snap.DoneBytes = done
				u.snap.TotalBytes = total
				u.mu.Unlock()
				u.emitChange()
				lastEmit = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	return f.Close()
}

func (u *Updater) verifyDownloadedFile(path, expectedSHA string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() < u.minAssetBytes {
		return fmt.Errorf("安装包体积异常（%d 字节）", info.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var head [2]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return fmt.Errorf("读取安装包头部失败: %w", err)
	}
	if head[0] != 'M' || head[1] != 'Z' {
		return fmt.Errorf("下载内容不是有效的 Windows 可执行安装包")
	}

	if expectedSHA == "" {
		return nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expectedSHA) {
		return fmt.Errorf("SHA-256 校验不匹配（期望 %s，实际 %s）", expectedSHA[:12], got[:12])
	}
	return nil
}

// InstallAndRestart launches the downloaded installer and triggers OnQuit so the running
// instance exits cleanly while the upgrade completes.
func (u *Updater) InstallAndRestart() error {
	u.mu.Lock()
	if u.snap.Status != UpdateReady || strings.TrimSpace(u.snap.DownloadedPath) == "" {
		u.mu.Unlock()
		return fmt.Errorf("当前没有已就绪的更新安装包")
	}
	pkgPath := u.snap.DownloadedPath
	var expectedSHA string
	if u.snap.Release != nil {
		expectedSHA = u.snap.Release.AssetSHA256
	}
	u.mu.Unlock()

	if err := u.verifyDownloadedFile(pkgPath, expectedSHA); err != nil {
		u.mu.Lock()
		u.snap.Status = UpdateError
		u.snap.Err = err.Error()
		u.snap.Stage = "安装包校验失败"
		u.mu.Unlock()
		u.emitChange()
		return err
	}

	u.mu.Lock()
	u.snap.Status = UpdateInstalling
	u.snap.Stage = "正在启动安装程序并退出…"
	u.mu.Unlock()
	u.emitChange()

	if err := u.launchInstallerFn(pkgPath, u.resolvedExePath()); err != nil {
		u.mu.Lock()
		u.snap.Status = UpdateError
		u.snap.Err = "启动安装程序失败: " + err.Error()
		u.snap.Stage = "安装启动失败"
		u.mu.Unlock()
		u.emitChange()
		return err
	}
	if u.onQuit != nil {
		u.onQuit()
	}
	return nil
}

// CleanupOldDownloads removes partial files, stale .old backups, and obsolete cached installers <= currentVersion.
func (u *Updater) CleanupOldDownloads() {
	if exe := u.resolvedExePath(); exe != "" {
		_ = os.Remove(exe + ".old")
	}
	if strings.TrimSpace(u.dataDir) == "" {
		return
	}
	entries, err := os.ReadDir(u.dataDir)
	if err != nil {
		return
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		ln := strings.ToLower(name)
		full := filepath.Join(u.dataDir, name)
		if strings.HasSuffix(ln, ".part") {
			_ = os.Remove(full)
			continue
		}
		if strings.HasPrefix(ln, "pureutils-") && strings.HasSuffix(ln, "-setup.exe") {
			ver := strings.TrimSuffix(strings.TrimPrefix(ln, "pureutils-"), "-setup.exe")
			if !core.IsNewerVersion(u.currentVersion, ver) {
				_ = os.Remove(full)
			}
		}
	}
}

// Stop cancels any active check or download goroutine during application shutdown.
func (u *Updater) Stop() {
	u.mu.Lock()
	chk := u.checkCancel
	dl := u.dlCancel
	u.checkCancel = nil
	u.dlCancel = nil
	u.mu.Unlock()
	if chk != nil {
		chk()
	}
	if dl != nil {
		dl()
	}
}
