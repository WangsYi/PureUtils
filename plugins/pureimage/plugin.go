package pureimage

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	pureui "pureutils/pkg/purekit/ui"
)

// PluginID is the stable identifier of the PureImage plugin.
const PluginID = "pureimage"

// Plugin adapts PureImage's viewer engine and Explorer Space preview to purekit.Plugin.
type Plugin struct {
	mu     sync.RWMutex
	host   purekit.Host
	cfg    config.PureImageConfig
	viewer *Viewer
}

// NewPlugin constructs an uninitialized PureImage plugin instance.
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string { return PluginID }

func (p *Plugin) Info() purekit.PluginInfo {
	return purekit.PluginInfo{
		ID:          PluginID,
		Name:        "PureImage",
		Description: "高性能看图、RAW/动图解码与资源管理器空格预览",
		Version:     "1.0.0",
		ModuleID:    core.ModuleViewer,
		IconPNG:     viewerIconPNG,
		Capabilities: []purekit.CapabilityDescriptor{
			{
				Name:        purekit.CapImagePreview,
				Description: "打开图片路径或内存图像、查询支持格式、切换快速预览",
			},
		},
	}
}

func (p *Plugin) Init(host purekit.Host) error {
	if host == nil {
		return fmt.Errorf("pureimage: nil host")
	}
	cfg := config.LoadPureImage(host.Config(PluginID))

	p.mu.Lock()
	p.host = host
	p.cfg = cfg
	v := New(cfg, Callbacks{
		OnPrefsChange: p.savePrefsFromViewer,
		CanPin: func() bool {
			return host.Bus() != nil && host.Bus().Has(purekit.CapImagePin)
		},
		OnPinImage: func(img image.Image) {
			if host.Bus() != nil && img != nil {
				_ = purekit.CallImagePinImage(context.Background(), host.Bus(), img, 0)
			}
		},
		OnCopyText: func(text string) {
			if cb := host.Clipboard(); cb != nil {
				_ = cb.WriteText(text, PluginID)
			} else {
				mygo.Clipboard.WriteText(text)
			}
		},
		OnAllClosed: func() {
			host.NotifyWindowClosed()
		},
		ShouldIgnoreSpace: func() bool {
			return host.IsGlobalHookSuppressed()
		},
		Logf: func(format string, args ...any) {
			if l := host.Log(); l != nil {
				l.Debugf(format, args...)
			}
		},
		Headless: host.Headless(),
	})
	p.viewer = v
	p.mu.Unlock()

	if host.Bus() != nil {
		if err := host.Bus().Register(purekit.CapImagePreview, p.handleImagePreview); err != nil {
			return fmt.Errorf("pureimage: register %s: %w", purekit.CapImagePreview, err)
		}
	}
	return nil
}

func (p *Plugin) Start() error {
	p.mu.RLock()
	host := p.host
	v := p.viewer
	space := p.cfg.SpacePreview
	p.mu.RUnlock()
	if v != nil && space && host != nil && !host.Headless() {
		v.SetSpacePreview(true)
	}
	return nil
}

func (p *Plugin) Stop() error {
	p.mu.Lock()
	host := p.host
	v := p.viewer
	p.viewer = nil
	p.mu.Unlock()

	if host != nil && host.Bus() != nil {
		host.Bus().Unregister(purekit.CapImagePreview)
	}
	if v != nil {
		v.Shutdown()
	}
	return nil
}

func (p *Plugin) handleImagePreview(_ context.Context, raw any) (any, error) {
	req, ok := raw.(purekit.ImagePreviewRequest)
	if !ok {
		return nil, fmt.Errorf("pureimage: unexpected request type %T", raw)
	}
	switch req.Action {
	case "open_path":
		p.Open(req.Path)
		return purekit.ImagePreviewResponse{}, nil
	case "open_image":
		p.OpenImage(req.Title, req.Image)
		return purekit.ImagePreviewResponse{}, nil
	case "supported_exts":
		return purekit.ImagePreviewResponse{Extensions: supportedDotExts()}, nil
	case "toggle_quick_preview":
		p.ToggleQuickPreview()
		return purekit.ImagePreviewResponse{}, nil
	default:
		return nil, fmt.Errorf("pureimage: unknown %s action %q", purekit.CapImagePreview, req.Action)
	}
}

func supportedDotExts() []string {
	out := make([]string, len(ImageExtList))
	for i, ext := range ImageExtList {
		out[i] = "." + strings.TrimPrefix(strings.ToLower(ext), ".")
	}
	return out
}

// Config returns a copy of the active PureImage configuration.
func (p *Plugin) Config() config.PureImageConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

// ApplyConfig normalizes, persists, and applies next to the live viewer.
func (p *Plugin) ApplyConfig(next config.PureImageConfig) error {
	next.Normalize()
	p.mu.Lock()
	host := p.host
	v := p.viewer
	prevSpace := p.cfg.SpacePreview
	p.cfg = next
	p.mu.Unlock()

	if host != nil {
		if err := config.SavePureImage(host.Config(PluginID), next); err != nil {
			return err
		}
	}
	if v != nil {
		v.ApplySettings(next)
	}
	if prevSpace != next.SpacePreview && host != nil {
		host.ReloadHotkeys()
		host.RebuildTray()
	}
	return nil
}

func (p *Plugin) savePrefsFromViewer(vs config.PureImageConfig) {
	vs.Normalize()
	p.mu.Lock()
	host := p.host
	prevSpace := p.cfg.SpacePreview
	p.cfg = vs
	p.mu.Unlock()

	if host != nil {
		_ = config.SavePureImage(host.Config(PluginID), vs)
		if prevSpace != vs.SpacePreview {
			host.ReloadHotkeys()
			host.RebuildTray()
		}
	}
}

func (p *Plugin) toggleSpacePreview() {
	cfg := p.Config()
	cfg.SpacePreview = !cfg.SpacePreview
	_ = p.ApplyConfig(cfg)
}

// Open opens path (or focuses an empty window when path is "") in PureImage.
func (p *Plugin) Open(path string) {
	p.mu.RLock()
	v := p.viewer
	p.mu.RUnlock()
	if v != nil {
		v.Open(path)
	}
}

// OpenImage opens an in-memory image in PureImage.
func (p *Plugin) OpenImage(title string, img image.Image) {
	p.mu.RLock()
	v := p.viewer
	p.mu.RUnlock()
	if v != nil {
		v.OpenImage(title, img)
	}
}

// ShowWindow raises the viewer window or opens an empty one.
func (p *Plugin) ShowWindow() {
	p.mu.RLock()
	v := p.viewer
	p.mu.RUnlock()
	if v != nil {
		v.ShowWindow()
	}
}

// ToggleQuickPreview toggles the floating preview window for the active image.
func (p *Plugin) ToggleQuickPreview() {
	p.mu.RLock()
	v := p.viewer
	p.mu.RUnlock()
	if v != nil {
		v.ToggleQuickPreview()
	}
}

// SetSpacePreview enables or disables Explorer Space preview.
func (p *Plugin) SetSpacePreview(on bool) {
	cfg := p.Config()
	cfg.SpacePreview = on
	_ = p.ApplyConfig(cfg)
}

// SpacePreviewActive reports whether the Space preview hook is active.
func (p *Plugin) SpacePreviewActive() bool {
	p.mu.RLock()
	v := p.viewer
	p.mu.RUnlock()
	return v != nil && v.SpacePreviewActive()
}

// Viewer returns the underlying Viewer instance (for direct window inspection or tests).
func (p *Plugin) Viewer() *Viewer {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.viewer
}

func (p *Plugin) HasOpenWindows() bool {
	p.mu.RLock()
	v := p.viewer
	p.mu.RUnlock()
	return v != nil && v.IsOpen()
}

func (p *Plugin) ShouldStayInTray() bool {
	return p.Config().SpacePreview
}

func (p *Plugin) IsBusyForGlobalHooks() bool { return false }

func (p *Plugin) Features() []purekit.Feature {
	var openShortcut string
	if p.host != nil {
		openShortcut = p.host.HostConfig().Hotkeys.OpenViewer
	}
	return []purekit.Feature{
		{
			ID:       "pureimage.open",
			PluginID: PluginID,
			Title:    "打开 PureImage 看图",
			Subtitle: "浏览本地图片、RAW 与动图",
			Keywords: []string{"image", "viewer", "photo", "pic", "看图", "图片", "预览", "pureimage"},
			Command:  core.CmdOpenViewer,
			Shortcut: openShortcut,
			Badge:    "PureImage",
			Icon:     pureui.IconImage,
			Run: func(_ context.Context, in purekit.ActionInput) error {
				if len(in.Files) > 0 {
					p.Open(in.Files[0])
				} else {
					p.Open("")
				}
				return nil
			},
		},
		{
			ID:       "pureimage.quick_preview",
			PluginID: PluginID,
			Title:    "PureImage 快速预览",
			Subtitle: "显示或隐藏当前图片的悬浮预览窗",
			Keywords: []string{"preview", "quick", "快速预览", "悬浮预览"},
			Shortcut: "Ctrl+Shift+Space",
			Badge:    "PureImage",
			Icon:     pureui.IconEye,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.ToggleQuickPreview()
				return nil
			},
		},
		{
			ID:       "pureimage.toggle_space",
			PluginID: PluginID,
			Title:    "切换资源管理器空格预览",
			Subtitle: "在资源管理器选中图片后按空格快速预览",
			Keywords: []string{"space", "explorer", "空格预览", "资源管理器"},
			Badge:    "PureImage",
			Icon:     pureui.IconEye,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.toggleSpacePreview()
				return nil
			},
		},
	}
}

func (p *Plugin) Matchers() []purekit.Matcher {
	return []purekit.Matcher{imageMatcher{plugin: p}}
}

type imageMatcher struct {
	plugin *Plugin
}

func (m imageMatcher) ID() string    { return "pureimage.matcher" }
func (m imageMatcher) Title() string { return "PureImage 看图匹配" }

func (m imageMatcher) Match(ctx purekit.MatchContext) []purekit.MatchResult {
	var results []purekit.MatchResult
	seen := make(map[string]bool)
	addPath := func(raw string, score int) {
		raw = strings.TrimSpace(strings.Trim(raw, `"`))
		if raw == "" || !IsImagePath(raw) {
			return
		}
		key := strings.ToLower(raw)
		if seen[key] {
			return
		}
		seen[key] = true
		path := raw
		results = append(results, purekit.MatchResult{
			FeatureID: "pureimage.open",
			PluginID:  PluginID,
			Title:     "用 PureImage 查看 " + filepath.Base(path),
			Subtitle:  path,
			Score:     score,
			Badge:     "PureImage",
			Icon:      pureui.IconImage,
			Input:     purekit.ActionInput{Files: []string{path}, Source: "matcher"},
			Execute: func(_ context.Context) error {
				m.plugin.Open(path)
				return nil
			},
		})
	}

	for _, f := range ctx.SelectedFiles {
		addPath(f, 95)
		if len(results) >= 3 {
			break
		}
	}
	if q := strings.TrimSpace(strings.Trim(ctx.Query, `"`)); q != "" && IsImagePath(q) {
		if st, err := os.Stat(q); err == nil && !st.IsDir() {
			addPath(q, 98)
		}
	}

	if ctx.Clipboard != nil && len(ctx.Clipboard.ImagePNG) > 0 && matchesImageKeywords(ctx.Query) {
		pngBytes := ctx.Clipboard.ImagePNG
		results = append(results, purekit.MatchResult{
			FeatureID: "pureimage.open_clipboard",
			PluginID:  PluginID,
			Title:     "用 PureImage 查看剪贴板图片",
			Subtitle:  "在独立看图窗口打开当前剪贴板中的图片",
			Score:     82,
			Badge:     "PureImage",
			Icon:      pureui.IconImage,
			Input:     purekit.ActionInput{Clipboard: ctx.Clipboard, Source: "matcher"},
			Execute: func(_ context.Context) error {
				img, err := png.Decode(bytes.NewReader(pngBytes))
				if err != nil {
					return err
				}
				m.plugin.OpenImage("剪贴板图片", img)
				return nil
			},
		})
	}
	return results
}

func matchesImageKeywords(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	for _, kw := range []string{"img", "image", "pic", "photo", "看图", "图片", "预览", "pureimage"} {
		if strings.Contains(kw, q) || strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func (p *Plugin) Hotkeys() []purekit.HotkeySpec {
	return []purekit.HotkeySpec{
		{
			ID:             "pureimage.open",
			Command:        core.CmdOpenViewer,
			Label:          "打开 PureImage 看图",
			DefaultGesture: "",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.OpenViewer
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.OpenViewer = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: func() { p.Open("") },
		},
		{
			ID:             "pureimage.quick_preview",
			Label:          "PureImage 快速预览",
			DefaultGesture: "Ctrl+Shift+Space",
			CurrentGesture: func() string {
				if p.Config().SpacePreview {
					return "Ctrl+Shift+Space"
				}
				return ""
			},
			Callback: p.ToggleQuickPreview,
		},
	}
}

func (p *Plugin) TrayItems() []purekit.TrayItemSpec {
	var accel string
	if p.host != nil {
		accel = p.host.HostConfig().Hotkeys.OpenViewer
	}
	return []purekit.TrayItemSpec{
		{
			ID:          "pureimage.open",
			Section:     purekit.TraySectionPrimary,
			Kind:        purekit.TrayItemAction,
			Label:       "打开 PureImage 看图",
			Accelerator: accel,
			Command:     core.CmdOpenViewer,
			OnClick:     func() { p.Open("") },
		},
		{
			ID:      "pureimage.space_preview",
			Section: purekit.TraySectionToggle,
			Kind:    purekit.TrayItemCheckbox,
			Label:   "资源管理器空格预览",
			Checked: p.Config().SpacePreview,
			OnClick: p.toggleSpacePreview,
		},
	}
}

func (p *Plugin) CLICommands() []purekit.CLICommandSpec {
	return []purekit.CLICommandSpec{
		{
			Command:    core.CmdOpenViewer,
			TakesPaths: true,
			Execute: func(paths []string) {
				if len(paths) > 0 {
					p.Open(paths[0])
				} else {
					p.Open("")
				}
			},
		},
		{
			Command: core.CmdToggleSpacePreview,
			Execute: func([]string) { p.toggleSpacePreview() },
		},
	}
}

func (p *Plugin) SettingsTab() *purekit.SettingsTab {
	return &purekit.SettingsTab{
		ID:       PluginID,
		Title:    "PureImage · 看图",
		Subtitle: "PureImage 浏览、渲染与关联",
		Icon:     pureui.IconImage,
		Order:    30,
		NewView: func(host purekit.Host) purekit.SettingsTabView {
			return newSettingsTabView(p, host)
		},
	}
}
