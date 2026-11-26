package pureshot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/storage"
	pureui "pureutils/pkg/purekit/ui"
)

// PluginID is the stable identifier of the PureShot plugin.
const PluginID = "pureshot"

type pinnedRecord struct {
	IsImage bool
	Text    string
	Width   int
	Height  int
	Cascade int
}

// Plugin adapts PureShot's region/screen capture, annotation, and floating pin
// windows to purekit.Plugin.
type Plugin struct {
	mu      sync.RWMutex
	host    purekit.Host
	cfg     config.PureShotConfig
	repo    *Repo
	blobs   *storage.BlobStore
	capture *CaptureService

	overlay       *captureSession
	overlayActive atomic.Bool

	pins          map[*pinWindow]bool
	pinsHidden    bool
	pinExecMu     sync.Mutex
	pinSeqMu      sync.Mutex
	pinQueue      []int64
	pinQueuePos   int
	pinCascade    int
	pinnedHistory []pinnedRecord

	histUnsubMu sync.Mutex
	histUnsub   func()
	histSubVer  uint64
}

// NewPlugin constructs an uninitialized PureShot plugin instance.
func NewPlugin() *Plugin {
	return &Plugin{
		pins: map[*pinWindow]bool{},
	}
}

func (p *Plugin) ID() string { return PluginID }

func (p *Plugin) Info() purekit.PluginInfo {
	return purekit.PluginInfo{
		ID:          PluginID,
		Name:        "PureShot",
		Description: "区域与整屏截图、9 种矢量标注与悬浮置顶贴图",
		Version:     core.AppVersion,
		ModuleID:    core.ModuleCapture,
		Capabilities: []purekit.CapabilityDescriptor{
			{
				Name:        purekit.CapImagePin,
				Description: "将图片、文本或剪贴板内容置顶贴图到桌面",
			},
		},
	}
}

func (p *Plugin) Init(host purekit.Host) error {
	if host == nil {
		return fmt.Errorf("pureshot: nil host")
	}
	cfg := config.LoadPureShot(host.Config(PluginID))
	ps := host.Storage(PluginID)
	repo, err := OpenRepo(ps)
	if err != nil {
		return fmt.Errorf("pureshot: open repo: %w", err)
	}
	blobs, err := ps.Blobs()
	if err != nil {
		_ = repo.Close()
		return fmt.Errorf("pureshot: open blobs: %w", err)
	}

	p.mu.Lock()
	p.host = host
	p.cfg = cfg
	p.repo = repo
	p.blobs = blobs
	p.capture = NewCaptureService(p.Config, host.Clipboard(), repo, blobs, host.Log())
	p.mu.Unlock()

	if bus := host.Bus(); bus != nil {
		if err := bus.Register(purekit.CapImagePin, p.handleImagePin); err != nil {
			_ = p.Stop()
			return fmt.Errorf("pureshot: register %s: %w", purekit.CapImagePin, err)
		}
	}
	return nil
}

func (p *Plugin) Start() error {
	p.ensureHistorySubscription()
	return nil
}

func (p *Plugin) Stop() error {
	p.clearHistorySubscription()

	p.mu.Lock()
	host := p.host
	repo := p.repo
	p.repo = nil
	p.capture = nil
	p.mu.Unlock()

	if host != nil {
		host.SetGlobalHookSuppressed("pureshot:overlay", false)
		if bus := host.Bus(); bus != nil {
			bus.Unregister(purekit.CapImagePin)
		}
	}

	if !p.headless() {
		mygo.RunOnMain(func() {
			if p.overlay != nil {
				p.overlay.Close()
			}
			for pw := range p.pins {
				pw.closeWindow()
			}
		})
	} else {
		for pw := range p.pins {
			pw.closed = true
			delete(p.pins, pw)
		}
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

func (p *Plugin) log() *core.Logger {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	if h == nil {
		return nil
	}
	return h.Log()
}

func (p *Plugin) notify(title, body string) {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	if h != nil {
		h.Notify(title, body)
	}
}

func (p *Plugin) notifyWindowClosed() {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	if h != nil {
		h.NotifyWindowClosed()
	}
}

func (p *Plugin) rebuildTray() {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	if h != nil {
		h.RebuildTray()
	}
}

func (p *Plugin) canPreviewImage() bool {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	return h != nil && h.Bus() != nil && h.Bus().Has(purekit.CapImagePreview)
}

// Config returns a copy of PureShot's active configuration.
func (p *Plugin) Config() config.PureShotConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

// ApplyConfig normalizes, persists, and applies next.
func (p *Plugin) ApplyConfig(next config.PureShotConfig) error {
	next.Normalize()
	p.mu.Lock()
	host := p.host
	p.cfg = next
	p.mu.Unlock()

	if host != nil {
		if err := config.SavePureShot(host.Config(PluginID), next); err != nil {
			return err
		}
	}
	if !p.headless() {
		mygo.RunOnMain(func() {
			for pw := range p.pins {
				pw.invalidate()
			}
		})
	}
	return nil
}

// Capture returns the CaptureService instance (used in tests and overlay).
func (p *Plugin) Capture() *CaptureService {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.capture
}

// Repo returns PureShot's isolated shot repository.
func (p *Plugin) Repo() *Repo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.repo
}

// PinnedCount returns the number of active pins.
func (p *Plugin) PinnedCount() int {
	return len(p.pins)
}

// PinnedRecords returns a snapshot of pins created in headless or live mode.
func (p *Plugin) PinnedRecords() []pinnedRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]pinnedRecord(nil), p.pinnedHistory...)
}

func (p *Plugin) recordPin(rec pinnedRecord) {
	p.mu.Lock()
	p.pinnedHistory = append(p.pinnedHistory, rec)
	p.mu.Unlock()
}

func (p *Plugin) ensureHistorySubscription() {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()

	var curVer uint64
	if h != nil && h.Bus() != nil {
		curVer = h.Bus().Version(purekit.CapClipboardHistory)
	}

	p.histUnsubMu.Lock()
	if curVer == 0 {
		old := p.histUnsub
		p.histUnsub = nil
		p.histSubVer = 0
		p.histUnsubMu.Unlock()
		if old != nil {
			old()
		}
		return
	}
	if p.histUnsub != nil && p.histSubVer == curVer {
		p.histUnsubMu.Unlock()
		return
	}
	old := p.histUnsub
	p.histUnsub = nil
	p.histSubVer = 0
	p.histUnsubMu.Unlock()
	if old != nil {
		old()
	}

	unsub, err := purekit.CallClipboardHistorySubscribeChanged(context.Background(), h.Bus(), p.resetPinSequence)
	if err == nil && unsub != nil {
		p.histUnsubMu.Lock()
		p.histUnsub = unsub
		p.histSubVer = curVer
		p.histUnsubMu.Unlock()
	}
}

func (p *Plugin) clearHistorySubscription() {
	p.histUnsubMu.Lock()
	unsub := p.histUnsub
	p.histUnsub = nil
	p.histSubVer = 0
	p.histUnsubMu.Unlock()
	if unsub != nil {
		unsub()
	}
}

func (p *Plugin) copyEphemeralText(text string) error {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	if h == nil || h.Clipboard() == nil {
		return fmt.Errorf("clipboard service unavailable")
	}
	return h.Clipboard().WriteEphemeralText(text, PluginID)
}

func (p *Plugin) copyPinText(text string) error {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	if h == nil || h.Clipboard() == nil {
		return fmt.Errorf("clipboard service unavailable")
	}
	return h.Clipboard().WriteText(text, PluginID)
}

func (p *Plugin) copyPinImage(pngData []byte) error {
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	if h == nil || h.Clipboard() == nil {
		return fmt.Errorf("clipboard service unavailable")
	}
	return h.Clipboard().WriteImage(pngData, PluginID)
}

func (p *Plugin) handleImagePin(_ context.Context, raw any) (any, error) {
	req, ok := raw.(purekit.ImagePinRequest)
	if !ok {
		return nil, fmt.Errorf("pureshot: unexpected %s request %T", purekit.CapImagePin, raw)
	}
	run := func(fn func()) {
		if p.headless() {
			fn()
			return
		}
		mygo.RunOnMain(fn)
	}
	switch req.Action {
	case "pin_image":
		if req.Image == nil {
			return nil, fmt.Errorf("pureshot: empty image")
		}
		img := req.Image
		cascade := req.Cascade
		run(func() { p.ShowPinImageAt(img, cascade) })
		return nil, nil
	case "pin_text":
		text := req.Text
		cascade := req.Cascade
		run(func() { p.ShowPinTextAt(text, cascade) })
		return nil, nil
	case "pin_content":
		if req.Content == nil || req.Content.IsEmpty() {
			return nil, fmt.Errorf("pureshot: empty clipboard content")
		}
		res, ok := resolvePinFromContent(req.Content)
		if !ok {
			return nil, fmt.Errorf("pureshot: content cannot be pinned")
		}
		cascade := req.Cascade
		run(func() {
			if res.img != nil {
				p.ShowPinImageAt(res.img, cascade)
			} else {
				p.ShowPinTextAt(res.text, cascade)
			}
		})
		return nil, nil
	default:
		return nil, fmt.Errorf("pureshot: unknown %s action %q", purekit.CapImagePin, req.Action)
	}
}

func (p *Plugin) HasOpenWindows() bool {
	return p.overlay != nil || len(p.pins) > 0
}

func (p *Plugin) ShouldStayInTray() bool { return true }

func (p *Plugin) IsBusyForGlobalHooks() bool {
	return p.overlayActive.Load()
}

func (p *Plugin) Features() []purekit.Feature {
	var rKey, mKey, pKey, tKey string
	if p.host != nil {
		hk := p.host.HostConfig().Hotkeys
		rKey = hk.CaptureRegion
		mKey = hk.CaptureMonitor
		pKey = hk.PinClipboard
		tKey = hk.TogglePins
	}
	return []purekit.Feature{
		{
			ID:       "pureshot.region",
			PluginID: PluginID,
			Title:    "区域截图",
			Subtitle: "框选屏幕区域并添加标注、复制或贴图",
			Keywords: []string{"shot", "capture", "screenshot", "snip", "截图", "区域截图", "pureshot"},
			Command:  core.CmdCaptureRegion,
			Shortcut: rKey,
			Badge:    "PureShot",
			Icon:     pureui.IconCrop,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.StartRegionCapture()
				return nil
			},
		},
		{
			ID:       "pureshot.monitor",
			PluginID: PluginID,
			Title:    "截取当前显示器",
			Subtitle: "立即截取光标所在的整个屏幕并复制",
			Keywords: []string{"screen", "monitor", "fullscreen", "整屏截图", "全屏截图"},
			Command:  core.CmdCaptureMonitor,
			Shortcut: mKey,
			Badge:    "PureShot",
			Icon:     pureui.IconMaximize,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.CaptureWholeScreen()
				return nil
			},
		},
		{
			ID:       "pureshot.pin",
			PluginID: PluginID,
			Title:    "贴图剪贴板",
			Subtitle: "将最近剪贴板中的图片或文本悬浮置顶在桌面上",
			Keywords: []string{"pin", "stick", "clipboard", "贴图", "置顶", "贴剪贴板"},
			Command:  core.CmdPinClipboard,
			Shortcut: pKey,
			Badge:    "PureShot",
			Icon:     pureui.IconLayers,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.PinClipboard()
				return nil
			},
		},
		{
			ID:       "pureshot.toggle_pins",
			PluginID: PluginID,
			Title:    "显示/隐藏贴图",
			Subtitle: "切换当前所有桌面悬浮贴图的可见状态",
			Keywords: []string{"hide", "show", "pins", "隐藏贴图", "显示贴图"},
			Command:  core.CmdTogglePins,
			Shortcut: tKey,
			Badge:    "PureShot",
			Icon:     pureui.IconEye,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				p.TogglePins()
				return nil
			},
		},
	}
}

func (p *Plugin) Matchers() []purekit.Matcher {
	return []purekit.Matcher{shotMatcher{plugin: p}}
}

type shotMatcher struct {
	plugin *Plugin
}

func (m shotMatcher) ID() string    { return "pureshot.matcher" }
func (m shotMatcher) Title() string { return "PureShot 贴图匹配" }

func (m shotMatcher) Match(ctx purekit.MatchContext) []purekit.MatchResult {
	var results []purekit.MatchResult

	for _, f := range ctx.SelectedFiles {
		raw := strings.TrimSpace(strings.Trim(f, `"`))
		if raw == "" {
			continue
		}
		if st, err := os.Stat(raw); err == nil && !st.IsDir() {
			if res, ok := resolvePinFromContent(&core.ClipboardContent{Files: []string{raw}}); ok && res.img != nil {
				img := res.img
				results = append(results, purekit.MatchResult{
					FeatureID: "pureshot.pin_file",
					PluginID:  PluginID,
					Title:     "用 PureShot 贴图显示",
					Subtitle:  raw,
					Score:     74,
					Badge:     "PureShot",
					Icon:      pureui.IconPin,
					Input:     purekit.ActionInput{Files: []string{raw}, Source: "matcher"},
					Execute: func(_ context.Context) error {
						m.plugin.ShowPinImage(img)
						return nil
					},
				})
				break
			}
		}
	}

	if ctx.Clipboard != nil && !ctx.Clipboard.IsEmpty() && matchesPinKeywords(ctx.Query) {
		if res, ok := resolvePinFromContent(ctx.Clipboard); ok {
			sub := core.PreviewOf(ctx.Clipboard)
			results = append(results, purekit.MatchResult{
				FeatureID: "pureshot.pin",
				PluginID:  PluginID,
				Title:     "将剪贴板内容贴到桌面",
				Subtitle:  sub,
				Score:     78,
				Badge:     "PureShot",
				Icon:      pureui.IconPin,
				Input:     purekit.ActionInput{Clipboard: ctx.Clipboard, Source: "matcher"},
				Execute: func(_ context.Context) error {
					if res.img != nil {
						m.plugin.ShowPinImage(res.img)
					} else {
						m.plugin.ShowPinText(res.text)
					}
					return nil
				},
			})
		}
	}
	return results
}

func matchesPinKeywords(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	for _, kw := range []string{"pin", "stick", "shot", "贴图", "置顶", "剪贴板", "pureshot"} {
		if strings.Contains(kw, q) || strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func (p *Plugin) Hotkeys() []purekit.HotkeySpec {
	return []purekit.HotkeySpec{
		{
			ID:             "pureshot.region",
			Command:        core.CmdCaptureRegion,
			Label:          "区域截图",
			DefaultGesture: "F1",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.CaptureRegion
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.CaptureRegion = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: p.StartRegionCapture,
		},
		{
			ID:             "pureshot.monitor",
			Command:        core.CmdCaptureMonitor,
			Label:          "截取当前显示器",
			DefaultGesture: "Ctrl+F1",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.CaptureMonitor
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.CaptureMonitor = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: p.CaptureWholeScreen,
		},
		{
			ID:             "pureshot.pin",
			Command:        core.CmdPinClipboard,
			Label:          "贴图剪贴板",
			DefaultGesture: "F3",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.PinClipboard
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.PinClipboard = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: p.PinClipboard,
		},
		{
			ID:             "pureshot.toggle_pins",
			Command:        core.CmdTogglePins,
			Label:          "显示/隐藏贴图",
			DefaultGesture: "Ctrl+Alt+X",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.TogglePins
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.TogglePins = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: p.TogglePins,
		},
	}
}

func (p *Plugin) TrayItems() []purekit.TrayItemSpec {
	var rKey, mKey, pKey, tKey string
	if p.host != nil {
		hk := p.host.HostConfig().Hotkeys
		rKey = hk.CaptureRegion
		mKey = hk.CaptureMonitor
		pKey = hk.PinClipboard
		tKey = hk.TogglePins
	}
	pinCount := len(p.pins)
	passthroughCount := p.passthroughPinCount()
	pinLabel := "显示贴图"
	if pinCount > 0 {
		pinLabel = fmt.Sprintf("显示贴图 (%d)", pinCount)
	}
	ptLabel := "贴图鼠标穿透"
	if passthroughCount > 0 && passthroughCount < pinCount {
		ptLabel = fmt.Sprintf("贴图鼠标穿透 (%d/%d)", passthroughCount, pinCount)
	}
	return []purekit.TrayItemSpec{
		{
			ID:          "pureshot.region",
			Section:     purekit.TraySectionPrimary,
			Kind:        purekit.TrayItemAction,
			Label:       "区域截图",
			Accelerator: rKey,
			Command:     core.CmdCaptureRegion,
			OnClick:     p.StartRegionCapture,
		},
		{
			ID:          "pureshot.monitor",
			Section:     purekit.TraySectionPrimary,
			Kind:        purekit.TrayItemAction,
			Label:       "截取当前显示器",
			Accelerator: mKey,
			Command:     core.CmdCaptureMonitor,
			OnClick:     p.CaptureWholeScreen,
		},
		{
			ID:          "pureshot.pin",
			Section:     purekit.TraySectionPrimary,
			Kind:        purekit.TrayItemAction,
			Label:       "贴图剪贴板",
			Accelerator: pKey,
			Command:     core.CmdPinClipboard,
			OnClick:     p.PinClipboard,
		},
		{
			ID:          "pureshot.show_pins",
			Section:     purekit.TraySectionToggle,
			Kind:        purekit.TrayItemCheckbox,
			Label:       pinLabel,
			Accelerator: tKey,
			Checked:     pinCount > 0 && !p.pinsHidden,
			Disabled:    pinCount == 0,
			Command:     core.CmdTogglePins,
			OnClick:     p.TogglePins,
		},
		{
			ID:       "pureshot.passthrough_pins",
			Section:  purekit.TraySectionToggle,
			Kind:     purekit.TrayItemCheckbox,
			Label:    ptLabel,
			Checked:  passthroughCount > 0,
			Disabled: pinCount == 0,
			Command:  core.CmdTogglePinsPassthrough,
			OnClick:  p.togglePinsPassthrough,
		},
	}
}

func (p *Plugin) togglePinsPassthrough() {
	if len(p.pins) == 0 {
		return
	}
	next := p.passthroughPinCount() == 0
	p.SetPinsPassthrough(next)
	if !p.headless() {
		if next {
			p.notify("贴图鼠标穿透已开启", "贴图将忽略鼠标点击，可在托盘菜单取消勾选恢复。")
		} else {
			p.notify("贴图鼠标穿透已关闭", "现在可以拖动、缩放或关闭贴图了。")
		}
	}
}

func (p *Plugin) CLICommands() []purekit.CLICommandSpec {
	return []purekit.CLICommandSpec{
		{Command: core.CmdCaptureRegion, Execute: func([]string) { p.StartRegionCapture() }},
		{Command: core.CmdCaptureMonitor, Execute: func([]string) { p.CaptureWholeScreen() }},
		{Command: core.CmdPinClipboard, Execute: func([]string) { p.PinClipboard() }},
		{Command: core.CmdTogglePins, Execute: func([]string) { p.TogglePins() }},
		{
			Command: core.CmdPinsClickable,
			Execute: func([]string) {
				p.SetPinsPassthrough(false)
				if !p.headless() {
					p.notify("贴图鼠标穿透已关闭", "现在可以拖动、缩放或关闭贴图了。")
				}
			},
		},
		{
			Command: core.CmdTogglePinsPassthrough,
			Execute: func([]string) { p.togglePinsPassthrough() },
		},
	}
}

func (p *Plugin) SettingsTab() *purekit.SettingsTab {
	return &purekit.SettingsTab{
		ID:       PluginID,
		Title:    "PureShot · 截图贴图",
		Subtitle: "截图保存、框选辅助与悬浮贴图",
		Icon:     pureui.IconCrop,
		Order:    10,
		NewView: func(host purekit.Host) purekit.SettingsTabView {
			return newSettingsTabView(p, host)
		},
	}
}
