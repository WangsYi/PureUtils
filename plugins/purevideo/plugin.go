//go:build windows

package purevideo

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	pureui "pureutils/pkg/purekit/ui"
)

//go:embed video.png
var videoIconPNG []byte

// PluginID is the stable identifier of the PureVideo plugin.
const PluginID = config.PluginPureVideo

// Plugin adapts PureVideo's built-in libmpv player to purekit.Plugin.
type Plugin struct {
	mu     sync.RWMutex
	host   purekit.Host
	cfg    config.PureVideoConfig
	mpvMgr *MPVManager
	player *Player
}

// NewPlugin constructs an uninitialized PureVideo plugin instance.
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string { return PluginID }

func (p *Plugin) Info() purekit.PluginInfo {
	return purekit.PluginInfo{
		ID:          PluginID,
		Name:        "PureVideo",
		Description: "内置 libmpv 全能视频浏览（支持硬解 HEVC / MKV / FLV / 外挂字幕）",
		Version:     core.AppVersion,
		ModuleID:    core.ModuleVideo,
		IconPNG:     videoIconPNG,
		Capabilities: []purekit.CapabilityDescriptor{
			{
				Name:        purekit.CapVideoPreview,
				Description: "打开本地视频并声明支持的视频格式",
			},
		},
	}
}

func (p *Plugin) Init(host purekit.Host) error {
	if host == nil {
		return fmt.Errorf("purevideo: nil host")
	}
	cfg := config.LoadPureVideo(host.Config(PluginID))
	pluginDir := host.Paths().PluginDir(PluginID)
	mgr := NewMPVManager(pluginDir)

	player := NewPlayer(cfg, PlayerCallbacks{
		PluginDir: pluginDir,
		OnPrefsChange: func(next config.PureVideoConfig) {
			p.savePlaybackPrefs(next)
		},
		OnOpenSettings: func() {
			host.OpenSettings(PluginID)
		},
		OnAllClosed: func() {
			host.NotifyWindowClosed()
		},
		Logf: func(format string, args ...any) {
			if l := host.Log(); l != nil {
				l.Infof("purevideo: "+format, args...)
			}
		},
		Headless: host.Headless(),
	})

	p.mu.Lock()
	p.host = host
	p.cfg = cfg
	p.mpvMgr = mgr
	p.player = player
	p.mu.Unlock()

	if bus := host.Bus(); bus != nil {
		if err := bus.Register(purekit.CapVideoPreview, p.handleCapability); err != nil {
			player.Shutdown()
			return err
		}
	}
	return nil
}

func (p *Plugin) Start() error { return nil }

func (p *Plugin) Stop() error {
	p.mu.RLock()
	host := p.host
	mgr := p.mpvMgr
	pl := p.player
	p.mu.RUnlock()

	if host != nil && host.Bus() != nil {
		host.Bus().Unregister(purekit.CapVideoPreview)
	}
	if mgr != nil {
		mgr.CancelDownload()
	}
	if pl != nil {
		pl.Shutdown()
	}
	return nil
}

func (p *Plugin) handleCapability(_ context.Context, raw any) (any, error) {
	req, ok := raw.(purekit.VideoPreviewRequest)
	if !ok {
		return nil, fmt.Errorf("purevideo: unexpected request type %T", raw)
	}
	switch req.Action {
	case "open_path":
		p.Open(req.Path)
		return purekit.VideoPreviewResponse{}, nil
	case "supported_exts":
		exts := make([]string, len(VideoExtList))
		for i, ext := range VideoExtList {
			exts[i] = "." + ext
		}
		return purekit.VideoPreviewResponse{Extensions: exts}, nil
	default:
		return nil, fmt.Errorf("purevideo: unknown capability action %q", req.Action)
	}
}

// Config returns a copy of the active PureVideo configuration.
func (p *Plugin) Config() config.PureVideoConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

func (p *Plugin) savePlaybackPrefs(next config.PureVideoConfig) {
	next.Normalize()
	p.mu.Lock()
	p.cfg = next
	host := p.host
	p.mu.Unlock()
	if host != nil {
		_ = config.SavePureVideo(host.Config(PluginID), next)
	}
}

// ApplyConfig validates, persists, and live-applies PureVideo configuration.
func (p *Plugin) ApplyConfig(cfg config.PureVideoConfig) error {
	cfg.Normalize()
	p.mu.Lock()
	host := p.host
	pl := p.player
	p.cfg = cfg
	p.mu.Unlock()

	if host != nil {
		if err := config.SavePureVideo(host.Config(PluginID), cfg); err != nil {
			return err
		}
	}
	if pl != nil {
		pl.ApplySettings(cfg)
	}
	return nil
}

// ReleaseBackend closes any active libmpv playback instance so libmpv-2.dll handles are released.
func (p *Plugin) ReleaseBackend() {
	p.mu.RLock()
	pl := p.player
	p.mu.RUnlock()
	if pl != nil {
		pl.ReleaseBackend()
	}
}

// Reload recreates the libmpv backend and resumes the currently opened video.
func (p *Plugin) Reload() {
	p.mu.RLock()
	pl := p.player
	p.mu.RUnlock()
	if pl != nil {
		pl.Reload()
	}
}

// Open opens path (or raises an empty player window if path is empty).
func (p *Plugin) Open(path string) {
	p.mu.RLock()
	pl := p.player
	p.mu.RUnlock()
	if pl != nil {
		pl.Open(path)
	}
}

// Player returns the underlying Player instance (primarily for tests).
func (p *Plugin) Player() *Player {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.player
}

// MPVManager returns the plugin's libmpv download/import manager.
func (p *Plugin) MPVManager() *MPVManager {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.mpvMgr
}

func (p *Plugin) HasOpenWindows() bool {
	p.mu.RLock()
	pl := p.player
	p.mu.RUnlock()
	return pl != nil && pl.IsOpen()
}

func (p *Plugin) ShouldStayInTray() bool       { return false }
func (p *Plugin) IsBusyForGlobalHooks() bool { return false }

func (p *Plugin) Features() []purekit.Feature {
	var openShortcut string
	if p.host != nil {
		openShortcut = p.host.HostConfig().Hotkeys.OpenVideo
	}
	return []purekit.Feature{
		{
			ID:       "purevideo.open",
			PluginID: PluginID,
			Title:    "打开 PureVideo 极速视频",
			Subtitle: "秒开本地视频（内置 libmpv 硬件解码，支持 21 种视频格式）",
			Keywords: []string{"video", "player", "mp4", "mkv", "mov", "webm", "avi", "视频", "播放", "purevideo"},
			Command:  core.CmdOpenVideo,
			Shortcut: openShortcut,
			Badge:    "PureVideo",
			Icon:     pureui.IconFilm,
			Run: func(_ context.Context, in purekit.ActionInput) error {
				if len(in.Files) > 0 {
					p.Open(in.Files[0])
				} else {
					p.Open("")
				}
				return nil
			},
		},
	}
}

func (p *Plugin) Matchers() []purekit.Matcher {
	return []purekit.Matcher{videoMatcher{plugin: p}}
}

type videoMatcher struct {
	plugin *Plugin
}

func (m videoMatcher) ID() string    { return "purevideo.matcher" }
func (m videoMatcher) Title() string { return "PureVideo 视频匹配" }

func (m videoMatcher) Match(ctx purekit.MatchContext) []purekit.MatchResult {
	var results []purekit.MatchResult
	seen := make(map[string]bool)
	addPath := func(raw string, score int) {
		raw = strings.TrimSpace(strings.Trim(raw, `"`))
		if raw == "" || !IsVideoPath(raw) {
			return
		}
		key := strings.ToLower(raw)
		if seen[key] {
			return
		}
		seen[key] = true
		path := raw
		results = append(results, purekit.MatchResult{
			FeatureID: "purevideo.open",
			PluginID:  PluginID,
			Title:     "用 PureVideo 播放 " + filepath.Base(path),
			Subtitle:  path,
			Score:     score,
			Badge:     "PureVideo",
			Icon:      pureui.IconFilm,
			Input:     purekit.ActionInput{Files: []string{path}, Source: "matcher"},
			Execute: func(_ context.Context) error {
				m.plugin.Open(path)
				return nil
			},
		})
	}

	for _, f := range ctx.SelectedFiles {
		addPath(f, 950)
		if len(results) >= 3 {
			break
		}
	}
	if q := strings.TrimSpace(strings.Trim(ctx.Query, `"`)); q != "" && IsVideoPath(q) {
		if st, err := os.Stat(q); err == nil && !st.IsDir() {
			addPath(q, 900)
		} else if len(results) == 0 && (strings.ContainsAny(q, `\/:`) || strings.HasPrefix(q, ".")) {
			addPath(q, 900)
		}
	}
	return results
}

func (p *Plugin) SettingsTab() *purekit.SettingsTab {
	return &purekit.SettingsTab{
		ID:       PluginID,
		Title:    "PureVideo · 视频",
		Subtitle: "libmpv 解码内核状态、播放体验与视频文件关联",
		Icon:     pureui.IconFilm,
		Order:    35,
		NewView: func(host purekit.Host) purekit.SettingsTabView {
			p.mu.RLock()
			mgr := p.mpvMgr
			p.mu.RUnlock()
			return newSettingsTabView(p, host, mgr)
		},
	}
}

func (p *Plugin) Hotkeys() []purekit.HotkeySpec {
	return []purekit.HotkeySpec{
		{
			ID:             "purevideo.open",
			Command:        core.CmdOpenVideo,
			Label:          "打开 PureVideo 视频",
			DefaultGesture: "",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.OpenVideo
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.OpenVideo = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: func() { p.Open("") },
		},
	}
}

func (p *Plugin) TrayItems() []purekit.TrayItemSpec {
	var accel string
	if p.host != nil {
		accel = p.host.HostConfig().Hotkeys.OpenVideo
	}
	return []purekit.TrayItemSpec{
		{
			ID:          "purevideo.open",
			Section:     purekit.TraySectionPrimary,
			Kind:        purekit.TrayItemAction,
			Label:       "打开 PureVideo 视频",
			Accelerator: accel,
			Command:     core.CmdOpenVideo,
			OnClick:     func() { p.Open("") },
		},
	}
}

func (p *Plugin) CLICommands() []purekit.CLICommandSpec {
	return []purekit.CLICommandSpec{
		{
			Command:    core.CmdOpenVideo,
			TakesPaths: true,
			Execute: func(paths []string) {
				if len(paths) > 0 {
					p.Open(paths[0])
				} else {
					p.Open("")
				}
			},
		},
	}
}
