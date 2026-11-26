package pureclip

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	pureui "pureutils/pkg/purekit/ui"
)

// PluginID is the stable identifier of the PureClip plugin.
const PluginID = "pureclip"

// Plugin adapts PureClip's clipboard history service, popup panel, and
// CapClipboardHistory bus endpoint to purekit.Plugin.
type Plugin struct {
	mu      sync.RWMutex
	host    purekit.Host
	cfg     config.PureClipConfig
	repo    *Repo
	service *HistoryService

	unsubClip func()

	win          *mygo.Window
	view         *historyView
	previewWin   *mygo.Window
	previewState historyPreviewState

	changeMu   sync.Mutex
	changeNext atomic.Int64
	changeSubs []changeSubscriber
}

type changeSubscriber struct {
	id int64
	fn func()
}

// NewPlugin constructs an uninitialized PureClip plugin instance.
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string { return PluginID }

func (p *Plugin) Info() purekit.PluginInfo {
	return purekit.PluginInfo{
		ID:          PluginID,
		Name:        "PureClip",
		Description: i18n.S("剪贴板历史监控、多格式去重检索与快捷回放", "Clipboard history monitoring, deduplicated multi-format search, and quick paste"),
		Version:     core.AppVersion,
		ModuleID:    core.ModuleHistory,
		Capabilities: []purekit.CapabilityDescriptor{
			{
				Name:        purekit.CapClipboardHistory,
				Description: i18n.S("提供最近剪贴板历史查询、内容加载、防抖刷新与变更订阅", "Provides recent clipboard history query, content loading, debounced refresh, and change subscription"),
			},
		},
	}
}

func (p *Plugin) Init(host purekit.Host) error {
	if host == nil {
		return fmt.Errorf("pureclip: nil host")
	}
	cfg := config.LoadPureClip(host.Config(PluginID))
	ps := host.Storage(PluginID)
	repo, err := OpenRepo(ps, host.Log())
	if err != nil {
		return fmt.Errorf("pureclip: open repo: %w", err)
	}
	blobs, err := ps.Blobs()
	if err != nil {
		_ = repo.Close()
		return fmt.Errorf("pureclip: open blobs: %w", err)
	}
	legacyBlobs, _, _ := ps.LegacyBlobs()

	p.mu.Lock()
	p.host = host
	p.cfg = cfg
	p.repo = repo
	p.service = NewHistoryService(repo, blobs, legacyBlobs, host.Clipboard(), p.Config, host.Log(), p.notifyChanged)
	p.mu.Unlock()

	if clip := host.Clipboard(); clip != nil {
		p.unsubClip = clip.Subscribe(func(ev clipboard.Event) {
			if p.service != nil {
				p.service.HandleClipboardEvent(ev)
			}
		})
	}
	if bus := host.Bus(); bus != nil {
		if err := bus.Register(purekit.CapClipboardHistory, p.handleCapability); err != nil {
			_ = p.Stop()
			return fmt.Errorf("pureclip: register %s: %w", purekit.CapClipboardHistory, err)
		}
	}
	return nil
}

func (p *Plugin) Start() error {
	p.mu.RLock()
	host := p.host
	svc := p.service
	cfg := p.cfg
	p.mu.RUnlock()

	if host != nil && host.Clipboard() != nil {
		p.syncTimings(host.Clipboard(), cfg)
	}
	if svc != nil && cfg.History.Enabled {
		return svc.StartMonitoring()
	}
	return nil
}

func (p *Plugin) Stop() error {
	if p.unsubClip != nil {
		p.unsubClip()
		p.unsubClip = nil
	}

	p.mu.Lock()
	host := p.host
	svc := p.service
	repo := p.repo
	p.service = nil
	p.repo = nil
	p.mu.Unlock()

	if host != nil && host.Bus() != nil {
		host.Bus().Unregister(purekit.CapClipboardHistory)
	}
	if !p.headless() {
		mygo.RunOnMain(func() {
			p.closeHistoryPreview()
			if p.win != nil {
				w := p.win
				p.win = nil
				p.view = nil
				w.Close()
			}
		})
	}
	if svc != nil {
		svc.StopMonitoring()
	}
	if repo != nil {
		return repo.Close()
	}
	return nil
}

func (p *Plugin) headless() bool {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	return h == nil || h.Headless()
}

func (p *Plugin) syncTimings(clip clipboard.Service, cfg config.PureClipConfig) {
	if clip == nil {
		return
	}
	debounce := time.Duration(cfg.DebounceMs) * time.Millisecond
	suppress := time.Duration(cfg.SelfWriteSuppressMs) * time.Millisecond
	clip.SetTimings(debounce, suppress)
}

// Config returns a copy of PureClip's active configuration.
func (p *Plugin) Config() config.PureClipConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := p.cfg
	out.History.IgnoredProcesses = append([]string(nil), p.cfg.History.IgnoredProcesses...)
	return out
}

// ApplyConfig normalizes, persists, and applies next to the live service and UI.
func (p *Plugin) ApplyConfig(next config.PureClipConfig) error {
	next.Normalize()
	p.mu.Lock()
	host := p.host
	svc := p.service
	p.cfg = next
	p.cfg.History.IgnoredProcesses = append([]string(nil), next.History.IgnoredProcesses...)
	p.mu.Unlock()

	if host != nil {
		if err := config.SavePureClip(host.Config(PluginID), next); err != nil {
			return err
		}
		p.syncTimings(host.Clipboard(), next)
	}
	if svc != nil {
		if next.History.Enabled && !svc.Monitoring() {
			_ = svc.StartMonitoring()
		} else if !next.History.Enabled && svc.Monitoring() {
			svc.StopMonitoring()
		}
	}
	if !p.headless() {
		mygo.RunOnMain(func() {
			if p.win != nil && p.view != nil {
				p.view.Reload()
				p.win.Invalidate()
			}
		})
	}
	if host != nil {
		host.RebuildTray()
	}
	return nil
}

// Service returns the underlying HistoryService (used in tests and internal UI).
func (p *Plugin) Service() *HistoryService {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.service
}

// Monitoring reports whether background history ingestion is currently active.
func (p *Plugin) Monitoring() bool {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()
	return svc != nil && svc.Monitoring()
}

// ToggleMonitoring flips monitoring and notifies the host tray/subscribers.
func (p *Plugin) ToggleMonitoring() bool {
	p.mu.RLock()
	svc := p.service
	host := p.host
	p.mu.RUnlock()
	if svc == nil {
		return false
	}
	on := svc.ToggleMonitoring()
	if host != nil {
		host.RebuildTray()
		if host.Log() != nil {
			host.Log().Infof("剪贴板监控切换为 %v", on)
		}
	}
	p.notifyChanged()
	return on
}

func (p *Plugin) notifyChanged() {
	p.changeMu.Lock()
	subs := append([]changeSubscriber(nil), p.changeSubs...)
	p.changeMu.Unlock()
	for _, s := range subs {
		s.fn()
	}
	if p.headless() {
		return
	}
	mygo.RunOnMain(func() {
		if p.win != nil && p.view != nil {
			p.view.Reload()
			p.win.Invalidate()
		}
		if p.host != nil {
			p.host.RebuildTray()
		}
	})
}

func (p *Plugin) subscribeChanged(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	id := p.changeNext.Add(1)
	p.changeMu.Lock()
	p.changeSubs = append(p.changeSubs, changeSubscriber{id: id, fn: fn})
	p.changeMu.Unlock()
	return func() {
		p.changeMu.Lock()
		for i, s := range p.changeSubs {
			if s.id == id {
				p.changeSubs = append(p.changeSubs[:i], p.changeSubs[i+1:]...)
				break
			}
		}
		p.changeMu.Unlock()
	}
}

func (p *Plugin) handleCapability(_ context.Context, raw any) (any, error) {
	req, ok := raw.(purekit.ClipboardHistoryRequest)
	if !ok {
		return nil, fmt.Errorf("pureclip: unexpected %s request %T", purekit.CapClipboardHistory, raw)
	}
	p.mu.RLock()
	host := p.host
	svc := p.service
	cfg := p.cfg
	p.mu.RUnlock()

	active := svc != nil && cfg.History.Enabled
	mon := svc != nil && svc.Monitoring()

	switch req.Action {
	case "status":
		return purekit.ClipboardHistoryResponse{Active: active, Monitoring: mon}, nil
	case "flush":
		if host != nil && host.Clipboard() != nil {
			host.Clipboard().FlushPending()
		}
		return purekit.ClipboardHistoryResponse{Active: active, Monitoring: mon}, nil
	case "list_recent":
		limit := req.Limit
		if limit <= 0 {
			limit = cfg.History.ListLimit
		}
		if limit <= 0 {
			limit = 200
		}
		if svc == nil {
			return purekit.ClipboardHistoryResponse{Active: false, Monitoring: false}, nil
		}
		rows, err := svc.Search("", nil, nil, limit)
		if err != nil {
			return nil, err
		}
		entries := make([]purekit.HistoryEntry, len(rows))
		for i, r := range rows {
			entries[i] = purekit.HistoryEntry{
				ID:            r.ID,
				Kind:          r.Kind,
				Preview:       r.Preview,
				SourceProcess: r.SourceProcess,
			}
		}
		return purekit.ClipboardHistoryResponse{
			Active:     active,
			Monitoring: mon,
			Entries:    entries,
		}, nil
	case "load_content":
		if svc == nil {
			return purekit.ClipboardHistoryResponse{Active: false, Monitoring: false}, nil
		}
		c, err := svc.LoadContent(req.EntryID)
		if err != nil {
			return nil, err
		}
		return purekit.ClipboardHistoryResponse{
			Active:     active,
			Monitoring: mon,
			Content:    c,
		}, nil
	case "subscribe_changed":
		unsub := p.subscribeChanged(req.OnChanged)
		return purekit.ClipboardHistoryResponse{
			Active:      active,
			Monitoring:  mon,
			Unsubscribe: unsub,
		}, nil
	default:
		return nil, fmt.Errorf("pureclip: unknown %s action %q", purekit.CapClipboardHistory, req.Action)
	}
}

func (p *Plugin) canPin() bool {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	return h != nil && h.Bus() != nil && h.Bus().Has(purekit.CapImagePin)
}

func (p *Plugin) canPreviewImage() bool {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	return h != nil && h.Bus() != nil && h.Bus().Has(purekit.CapImagePreview)
}

func (p *Plugin) HasOpenWindows() bool {
	return p.win != nil
}

func (p *Plugin) ShouldStayInTray() bool { return true }

func (p *Plugin) IsBusyForGlobalHooks() bool { return false }

func (p *Plugin) Features() []purekit.Feature {
	var shortcut string
	if p.host != nil {
		shortcut = p.host.HostConfig().Hotkeys.ShowHistory
	}
	return []purekit.Feature{
		{
			ID:       "pureclip.show",
			PluginID: PluginID,
			Title:    i18n.S("剪贴板历史", "Clipboard History"),
			Subtitle: i18n.S("搜索并快速回放历史文本、图片与文件", "Search and quickly paste recent text, images, and files"),
			Keywords: []string{"clip", "clipboard", "history", "paste", "剪贴板", "历史", "粘贴", "pureclip"},
			Command:  core.CmdShowHistory,
			Shortcut: shortcut,
			Badge:    "PureClip",
			Icon:     pureui.IconClipboard,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.ShowHistory()
				return nil
			},
		},
		{
			ID:       "pureclip.toggle_monitoring",
			PluginID: PluginID,
			Title:    i18n.S("切换剪贴板监控", "Toggle Clipboard Monitoring"),
			Subtitle: i18n.S("暂停或恢复后台剪贴板历史收录", "Pause or resume background clipboard history capture"),
			Keywords: []string{"clip", "monitor", "pause", "监控", "暂停剪贴板"},
			Command:  core.CmdToggleMonitoring,
			Badge:    "PureClip",
			Icon:     pureui.IconPause,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.ToggleMonitoring()
				return nil
			},
		},
	}
}

func (p *Plugin) Matchers() []purekit.Matcher {
	return []purekit.Matcher{historyMatcher{plugin: p}}
}

type historyMatcher struct {
	plugin *Plugin
}

func (m historyMatcher) ID() string    { return "pureclip.matcher" }
func (m historyMatcher) Title() string { return i18n.S("PureClip 剪贴板搜索匹配", "PureClip Clipboard Search") }

func (m historyMatcher) Match(ctx purekit.MatchContext) []purekit.MatchResult {
	q := strings.TrimSpace(ctx.Query)
	svc := m.plugin.Service()
	if q == "" || svc == nil {
		return nil
	}
	rows, err := svc.Search(q, nil, nil, 5)
	if err != nil || len(rows) == 0 {
		return nil
	}
	results := make([]purekit.MatchResult, 0, len(rows))
	for i, r := range rows {
		id := r.ID
		results = append(results, purekit.MatchResult{
			FeatureID: "pureclip.restore",
			PluginID:  PluginID,
			Title:     r.Preview,
			Subtitle:  i18n.F("剪贴板历史 · %s · %s", "Clipboard · %s · %s", r.Kind.String(), timeAgo(r.LastUsedUTC)),
			Score:     74 - i,
			Badge:     "PureClip",
			Icon:      pureui.IconClipboard,
			Execute: func(_ context.Context) error {
				svc.Restore(id)
				return nil
			},
		})
	}
	return results
}

func (p *Plugin) Hotkeys() []purekit.HotkeySpec {
	return []purekit.HotkeySpec{
		{
			ID:             "pureclip.show",
			Command:        core.CmdShowHistory,
			Label:          i18n.S("剪贴板历史", "Clipboard History"),
			DefaultGesture: "Ctrl+Shift+V",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.ShowHistory
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.ShowHistory = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: p.ShowHistory,
		},
	}
}

func (p *Plugin) TrayItems() []purekit.TrayItemSpec {
	var accel string
	if p.host != nil {
		accel = p.host.HostConfig().Hotkeys.ShowHistory
	}
	return []purekit.TrayItemSpec{
		{
			ID:          "pureclip.show",
			Section:     purekit.TraySectionPrimary,
			Kind:        purekit.TrayItemAction,
			Label:       i18n.S("剪贴板历史", "Clipboard History"),
			Accelerator: accel,
			Command:     core.CmdShowHistory,
			OnClick:     p.ShowHistory,
		},
		{
			ID:      "pureclip.monitoring",
			Section: purekit.TraySectionToggle,
			Kind:    purekit.TrayItemCheckbox,
			Label:   i18n.S("剪贴板监控", "Clipboard Monitoring"),
			Checked: p.Monitoring(),
			Command: core.CmdToggleMonitoring,
			OnClick: func() { p.ToggleMonitoring() },
		},
	}
}

func (p *Plugin) CLICommands() []purekit.CLICommandSpec {
	return []purekit.CLICommandSpec{
		{
			Command: core.CmdShowHistory,
			Execute: func([]string) { p.ShowHistory() },
		},
		{
			Command: core.CmdToggleMonitoring,
			Execute: func([]string) { p.ToggleMonitoring() },
		},
	}
}

func (p *Plugin) SettingsTab() *purekit.SettingsTab {
	return &purekit.SettingsTab{
		ID:       PluginID,
		Title:    i18n.S("PureClip · 剪贴板", "PureClip · Clipboard"),
		Subtitle: i18n.S("监听类型、保留策略、隐私保护与历史维护", "Capture rules, retention, privacy exclusions, and maintenance"),
		Icon:     pureui.IconClipboard,
		Order:    20,
		NewView: func(host purekit.Host) purekit.SettingsTabView {
			return newSettingsTabView(p, host)
		},
	}
}
