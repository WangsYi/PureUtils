package host

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/pkg/purekit/platform"
	pureui "pureutils/pkg/purekit/ui"
)

const (
	launcherWidth  = 640
	launcherHeight = 460
)

// LauncherItem represents one ranked actionable entry displayed in the global plugin launcher.
type LauncherItem struct {
	ID       string
	PluginID string
	Title    string
	Subtitle string
	Badge    string
	Shortcut string
	Icon     *ui.SVG
	Score    int
	IsMatch  bool
	Run      func(context.Context) error
}

type rankedLauncherItem struct {
	item  LauncherItem
	order int
}

// ToggleLauncher opens the global plugin launcher, or closes it if already open.
func (a *App) ToggleLauncher() {
	if a.launcherWin != nil || (a.launcherView != nil && !a.launcherView.closed) {
		a.closeLauncher()
		return
	}
	a.ShowLauncher()
}

func (a *App) closeLauncher() {
	if a.launcherView != nil {
		a.launcherView.closeWindow()
	} else if a.launcherWin != nil {
		win := a.launcherWin
		a.launcherWin = nil
		if !a.headless {
			win.Close()
		}
	}
}

// ShowLauncher captures current context (Explorer selection + clipboard + active app)
// and opens the frameless floating launcher panel.
func (a *App) ShowLauncher() {
	if !a.HostConfig().Launcher.Enabled {
		if a.log != nil {
			a.log.Infof("快捷启动器已在设置中禁用")
		}
		return
	}
	if a.launcherWin != nil {
		a.launcherWin.Show()
		a.launcherWin.Focus()
		a.launcherWin.Invalidate()
		return
	}

	ctx := a.captureLauncherContext()
	lv := newLauncherView(a, ctx)
	a.launcherView = lv
	if a.headless {
		return
	}

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:             i18n.S("PureUtils 快捷启动器", "PureUtils Launcher"),
		Width:             launcherWidth,
		Height:            launcherHeight,
		Hidden:            true,
		Frameless:         true,
		Panel:             true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		CornerRadius:      14,
		BackgroundColor:   "#00000000",
		Content:           ui.View(lv.view),
	})
	lv.win = win
	_ = win.SetIcon(iconPNG)
	placeLauncherWindow(win, launcherWidth, launcherHeight)

	win.OnBlur(func() {
		if lv.closed || lv.executing {
			return
		}
		win.Close()
	})
	win.OnClose(func(_ *mygo.CloseEvent) {
		lv.closed = true
		if a.launcherWin == win {
			a.launcherWin = nil
		}
		if a.launcherView == lv {
			a.launcherView = nil
		}
		a.NotifyWindowClosed()
	})

	a.launcherWin = win
	win.Show()
	win.Focus()
}

func placeLauncherWindow(win *mygo.Window, w, h int) {
	if win == nil {
		return
	}
	p := mygo.Screen.CursorScreenPoint()
	d := mygo.Screen.DisplayNearestPoint(p)
	wa := d.WorkArea
	x := wa.X + (wa.Width-w)/2
	y := wa.Y + wa.Height/5
	x = clampInt(x, wa.X+8, wa.X+wa.Width-w-8)
	y = clampInt(y, wa.Y+8, wa.Y+wa.Height-h-8)
	win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: w, Height: h})
}

func (a *App) captureLauncherContext() purekit.MatchContext {
	if a.launcherCtxProvider != nil {
		return a.launcherCtxProvider()
	}
	var ctx purekit.MatchContext
	if !a.headless {
		if files, err := platform.ExplorerSelectedFiles(); err == nil && len(files) > 0 {
			ctx.SelectedFiles = files
		}
		ctx.ActiveApp = platform.ForegroundProcessName()
	}
	if a.clip != nil {
		if c, err := a.clip.Read(); err == nil && c != nil && !c.IsEmpty() {
			ctx.Clipboard = c
		}
	}
	return ctx
}

// QueryLauncher queries all active plugins' Matchers and Features (plus host built-ins)
// and returns deduplicated, ranked results capped by HostConfig.Launcher.MaxResults.
func (a *App) QueryLauncher(ctx purekit.MatchContext) []LauncherItem {
	maxResults := a.HostConfig().Launcher.MaxResults
	if maxResults <= 0 {
		maxResults = 12
	}
	if maxResults > 50 {
		maxResults = 50
	}

	q := strings.TrimSpace(ctx.Query)
	pathQuery := looksLikePath(q)
	candidates := make([]rankedLauncherItem, 0, 24)
	seq := 0

	plugins := a.snapshotPlugins()

	// 1. Query plugin Matchers.
	for _, p := range plugins {
		for _, m := range p.Matchers() {
			for _, res := range m.Match(ctx) {
				if res.Execute == nil {
					continue
				}
				score := res.Score
				if q != "" {
					baseSub := ""
					if res.Subtitle != "" {
						baseSub = filepath.Base(res.Subtitle)
					}
					txtScore := scoreFeatureQuery(q, []string{res.Title}, baseSub, res.Badge, res.PluginID, res.FeatureID)
					if res.PluginID != "pureclip" && !pathQuery && txtScore == 0 {
						continue
					}
					if txtScore > 0 {
						score += txtScore / 5
					}
				}
				icon := res.Icon
				if icon == nil {
					icon = pureui.IconSearch
				}
				candidates = append(candidates, rankedLauncherItem{
					order: seq,
					item: LauncherItem{
						ID:       res.FeatureID,
						PluginID: res.PluginID,
						Title:    res.Title,
						Subtitle: res.Subtitle,
						Badge:    res.Badge,
						Shortcut: res.Shortcut,
						Icon:     icon,
						Score:    score,
						IsMatch:  true,
						Run:      res.Execute,
					},
				})
				seq++
			}
		}
	}

	// 2. Collect plugin Features.
	for _, p := range plugins {
		info := p.Info()
		for _, feat := range p.Features() {
			if feat.Run == nil {
				continue
			}
			f := feat
			badge := f.Badge
			if badge == "" {
				badge = info.Name
			}
			pluginID := f.PluginID
			if pluginID == "" {
				pluginID = info.ID
			}
			score := 50
			if q != "" {
				fields := append([]string{f.Title}, f.Keywords...)
				score = scoreFeatureQuery(q, fields, f.Subtitle, badge, pluginID)
				if score == 0 {
					continue
				}
			}
			icon := f.Icon
			if icon == nil {
				icon = pureui.IconSliders
			}
			in := purekit.ActionInput{
				Query:     q,
				Text:      q,
				Files:     ctx.SelectedFiles,
				Source:    "launcher",
				Clipboard: ctx.Clipboard,
			}
			candidates = append(candidates, rankedLauncherItem{
				order: seq,
				item: LauncherItem{
					ID:       f.ID,
					PluginID: pluginID,
					Title:    f.Title,
					Subtitle: f.Subtitle,
					Badge:    badge,
					Shortcut: f.Shortcut,
					Icon:     icon,
					Score:    score,
					Run: func(runCtx context.Context) error {
						return f.Run(runCtx, in)
					},
				},
			})
			seq++
		}
	}

	// 3. Collect host built-in features.
	for _, hf := range a.hostFeatures() {
		f := hf
		score := 30
		if q != "" {
			fields := append([]string{f.Title}, f.Keywords...)
			score = scoreFeatureQuery(q, fields, f.Subtitle, f.Badge, f.PluginID)
			if score == 0 {
				continue
			}
		}
		candidates = append(candidates, rankedLauncherItem{
			order: seq,
			item: LauncherItem{
				ID:       f.ID,
				PluginID: f.PluginID,
				Title:    f.Title,
				Subtitle: f.Subtitle,
				Badge:    f.Badge,
				Shortcut: f.Shortcut,
				Icon:     f.Icon,
				Score:    score,
				Run: func(runCtx context.Context) error {
					return f.Run(runCtx, purekit.ActionInput{Query: q, Source: "launcher"})
				},
			},
		})
		seq++
	}

	// 4. Deduplicate keeping highest-scoring entry.
	deduped := make([]rankedLauncherItem, 0, len(candidates))
	seen := make(map[string]int, len(candidates))
	for _, cand := range candidates {
		key := strings.ToLower(cand.item.PluginID + "|" + cand.item.ID + "|" + cand.item.Title)
		if prevIdx, exists := seen[key]; exists {
			if cand.item.Score > deduped[prevIdx].item.Score ||
				(cand.item.Score == deduped[prevIdx].item.Score && cand.item.IsMatch && !deduped[prevIdx].item.IsMatch) {
				deduped[prevIdx] = cand
			}
			continue
		}
		seen[key] = len(deduped)
		deduped = append(deduped, cand)
	}

	// 5. Sort descending by Score, IsMatch, then deterministic order.
	sort.Slice(deduped, func(i, j int) bool {
		if deduped[i].item.Score != deduped[j].item.Score {
			return deduped[i].item.Score > deduped[j].item.Score
		}
		if deduped[i].item.IsMatch != deduped[j].item.IsMatch {
			return deduped[i].item.IsMatch
		}
		return deduped[i].order < deduped[j].order
	})

	if len(deduped) > maxResults {
		deduped = deduped[:maxResults]
	}
	out := make([]LauncherItem, len(deduped))
	for i := range deduped {
		out[i] = deduped[i].item
	}
	return out
}

func (a *App) hostFeatures() []purekit.Feature {
	return []purekit.Feature{
		{
			ID:       "host.settings",
			PluginID: "host",
			Title:    i18n.S("打开 PureUtils 设置", "Open PureUtils Settings"),
			Subtitle: i18n.S("管理外观、快捷键与插件设置", "Manage appearance, hotkeys, and plugin settings"),
			Keywords: []string{"settings", "config", "preference", "设置", "配置", "快捷键"},
			Command:  core.CmdShowSettings,
			Badge:    "PureUtils",
			Icon:     pureui.IconSettings,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				a.OpenSettings("")
				return nil
			},
		},
		{
			ID:       "host.check_update",
			PluginID: "host",
			Title:    i18n.S("检查软件更新", "Check for Updates"),
			Subtitle: i18n.S("检查 GitHub 新版本并支持多镜像加速下载安装", "Check GitHub Releases with mirror acceleration"),
			Keywords: []string{"update", "upgrade", "version", "check", "更新", "升级", "版本", "检查更新"},
			Command:  core.CmdCheckUpdate,
			Badge:    "PureUtils",
			Icon:     pureui.IconDownload,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				a.ExecuteCommand(core.CmdCheckUpdate)
				return nil
			},
		},
		{
			ID:       "host.exit",
			PluginID: "host",
			Title:    i18n.S("退出 PureUtils", "Quit PureUtils"),
			Subtitle: i18n.S("关闭后台驻留与全部插件", "Quit background tray and all plugins"),
			Keywords: []string{"exit", "quit", "退出", "关闭"},
			Command:  core.CmdExit,
			Badge:    "PureUtils",
			Icon:     pureui.IconClose,
			Run: func(_ context.Context, _ purekit.ActionInput) error {
				a.Quit()
				return nil
			},
		},
	}
}

func looksLikePath(q string) bool {
	return strings.ContainsAny(q, `\/:`) || strings.HasPrefix(q, ".")
}

func scoreFeatureQuery(query string, primaryFields []string, secondaryFields ...string) int {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(tokens) == 0 {
		return 0
	}
	total := 0
	minTokenScore := 1000
	for _, tok := range tokens {
		best := 0
		for _, raw := range primaryFields {
			f := strings.ToLower(strings.TrimSpace(raw))
			if f == "" {
				continue
			}
			var s int
			switch {
			case f == tok:
				s = 120
			case strings.HasPrefix(f, tok):
				s = 100
			case strings.Contains(f, tok):
				s = 85
			case isRuneSubsequence(tok, f):
				s = 45
			}
			if s > best {
				best = s
			}
		}
		for _, raw := range secondaryFields {
			f := strings.ToLower(strings.TrimSpace(raw))
			if f != "" && strings.Contains(f, tok) && best < 65 {
				best = 65
			}
		}
		if best == 0 {
			return 0
		}
		total += best
		if best < minTokenScore {
			minTokenScore = best
		}
	}
	if len(tokens) == 1 {
		return total
	}
	return (minTokenScore + total/len(tokens)) / 2
}

func isRuneSubsequence(needle, haystack string) bool {
	if needle == "" {
		return false
	}
	nr := []rune(needle)
	ni := 0
	for _, hr := range haystack {
		if unicode.ToLower(hr) == unicode.ToLower(nr[ni]) {
			ni++
			if ni == len(nr) {
				return true
			}
		}
	}
	return false
}

type launcherView struct {
	app       *App
	win       *mygo.Window
	closed    bool
	executing bool

	ctx      purekit.MatchContext
	query    string
	items    []LauncherItem
	selected int

	search    ui.Handle
	list      ui.ListState
	needFocus bool
}

func newLauncherView(a *App, ctx purekit.MatchContext) *launcherView {
	l := &launcherView{
		app:       a,
		ctx:       ctx,
		needFocus: true,
	}
	l.refresh()
	return l
}

func (l *launcherView) refresh() {
	l.ctx.Query = l.query
	l.items = l.app.QueryLauncher(l.ctx)
	if len(l.items) == 0 {
		l.selected = -1
	} else {
		l.selected = 0
	}
}

func (l *launcherView) clearContext() {
	l.ctx.SelectedFiles = nil
	l.ctx.Clipboard = nil
	l.needFocus = true
	l.refresh()
	l.invalidate()
}

func (l *launcherView) moveSelection(delta int) {
	if len(l.items) == 0 {
		return
	}
	next := l.selected + delta
	if next < 0 {
		next = len(l.items) - 1
	} else if next >= len(l.items) {
		next = 0
	}
	l.selected = next
	l.list.ScrollTo(next, ui.Center)
	l.invalidate()
}

func (l *launcherView) execute(idx int) {
	if l.closed || idx < 0 || idx >= len(l.items) {
		return
	}
	run := l.items[idx].Run
	l.executing = true
	l.closeWindow()
	if run == nil {
		return
	}
	if l.app == nil || l.app.headless {
		_ = run(context.Background())
		return
	}
	mygo.RunOnMain(func() {
		_ = run(context.Background())
	})
}

func (l *launcherView) closeWindow() {
	if l.closed {
		return
	}
	l.closed = true
	if l.app != nil && l.app.launcherView == l {
		l.app.launcherView = nil
	}
	if l.win != nil {
		win := l.win
		l.win = nil
		if l.app == nil || !l.app.headless {
			win.Close()
		}
	}
}

func (l *launcherView) invalidate() {
	if l.win != nil {
		l.win.Invalidate()
	}
}

func (l *launcherView) hasContext() bool {
	return len(l.ctx.SelectedFiles) > 0 || (l.ctx.Clipboard != nil && !l.ctx.Clipboard.IsEmpty())
}

func (l *launcherView) view(c *ui.Context) {
	pal := pureui.ApplyTheme(c)

	if c.Shortcut(0, ui.KeyEscape) {
		l.closeWindow()
		return
	}
	if c.Shortcut(0, ui.KeyUp) {
		l.moveSelection(-1)
	}
	if c.Shortcut(0, ui.KeyDown) {
		l.moveSelection(1)
	}
	quickKeys := [...]ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}
	for i, key := range quickKeys {
		if i < len(l.items) && c.Shortcut(ui.Cmd, key) {
			l.execute(i)
			return
		}
	}

	ui.Column(c).Fill().Radius(14).Background(pal.BgElevated).
		Border(1, pal.BorderStrong).InsetShadow(0, 1, 0, 0, pal.Specular).Clip().Children(func() {
		// Seamless command search header
		ui.Row(c).Height(54).Padding(10, 14).Gap(10).AlignItems(ui.Center).
			Background(pal.BgElevated).DragWindow().Children(func() {
			field := ui.SearchField(c, &l.query).
				Placeholder(i18n.S("搜索插件功能、剪贴板历史，或直接回车执行…", "Search actions, clipboard history, or press Enter…")).
				Background(ui.Transparent).Border(0, ui.Transparent).FocusRing(false).
				Grow(1).MinWidth(120).Bind(&l.search)
			if l.needFocus {
				l.search.Focus()
				l.needFocus = false
			}
			if field.Changed() {
				l.refresh()
			}
			if field.Submitted() {
				l.execute(l.selected)
				return
			}
			if l.query != "" {
				if pureui.IconButton(c, pureui.IconXCircle, i18n.S("清空搜索", "Clear search"), i18n.S("清空搜索内容", "Clear search query"), false, false, pal).Size(26, 26).Radius(7).Clicked() {
					l.query = ""
					l.refresh()
					l.search.Focus()
					l.invalidate()
				}
			}
			pureui.Keycap(c, "Esc", pal)
		})

		ui.Box(c).Height(1).FillWidth().Background(pal.Border)

		// Contextual ribbon when Explorer files or clipboard items are attached
		if l.hasContext() {
			l.contextRibbon(c, pal)
			ui.Box(c).Height(1).FillWidth().Background(pal.Border)
		}

		// Results list
		ui.Column(c).Grow(1).Fill().Background(pal.Bg).Padding(8).Children(func() {
			if len(l.items) == 0 {
				pureui.EmptyState(c, pureui.IconSearch, i18n.S("未找到匹配功能", "No matching actions"), i18n.S("换个关键词试试，或按 Esc 关闭启动器", "Try a different keyword, or press Esc to close"), pal, nil)
				return
			}

			l.list.Label = func(i int) string { return l.items[i].Title }
			l.list.Selected = &l.selected

			selectedBg := pal.CardBg.Mix(pal.Accent, 0.08)
			if pal.Dark {
				selectedBg = pal.Bg.Mix(pal.Accent, 0.18)
			}
			listTheme := *c.Theme()
			listTheme.Accent = selectedBg
			listTheme.AccentText = pal.Text
			listTheme.SurfaceHover = pal.CardHover
			listTheme.Radius = 9

			ui.Themed(c, &listTheme).Fill().Background(ui.Transparent).Children(func() {
				list := ui.List(c, &l.list, len(l.items)).Gap(3).Rows(func(row ui.ListRow) {
					l.renderRow(row, pal, selectedBg)
				}).Grow(1)
				if len(l.items) > 0 && (l.selected < 0 || l.selected >= len(l.items)) {
					l.selected = 0
				}
				if list.Submitted() {
					l.execute(l.selected)
				}
			})
		})

		if !l.closed && !l.executing && c.Shortcut(0, ui.KeyEnter) {
			l.execute(l.selected)
			return
		}

		// Footer bar
		ui.Box(c).Height(1).FillWidth().Background(pal.Border)
		ui.Row(c).Height(32).Padding(0, 14).Gap(8).AlignItems(ui.Center).Background(pal.SidebarBg).Children(func() {
			ui.Text(c, i18n.S("↑↓ 选择 · Enter 执行 · Ctrl+1–9 快捷执行 · Esc 关闭", "↑↓ Select · Enter Run · Ctrl+1–9 Quick Run · Esc Close")).
				FontSize(10.5).SingleLine().TextColor(pal.TextMuted).Grow(1).MinWidth(0)
			pureui.PillBadge(c, i18n.F("%d 项", "%d items", len(l.items)), pal.TextMuted, pal.ControlBg)
		})
	})
}

func truncateLauncherMiddle(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes || maxRunes < 7 {
		return s
	}
	keep := (maxRunes - 1) / 2
	return string(r[:keep]) + "…" + string(r[len(r)-(maxRunes-1-keep):])
}

func (l *launcherView) contextRibbon(c *ui.Context, pal pureui.Palette) {
	ui.Row(c).Padding(6, 14).Gap(6).AlignItems(ui.Center).Background(pal.SidebarBg).Children(func() {
		ui.Text(c, i18n.S("上下文", "Context")).FontSize(10.5).FontWeight(600).TextColor(pal.TextMuted).Shrink(0)
		if len(l.ctx.SelectedFiles) > 0 {
			name := truncateLauncherMiddle(filepath.Base(l.ctx.SelectedFiles[0]), 26)
			label := i18n.S("已选中 · ", "Selected · ") + name
			if len(l.ctx.SelectedFiles) > 1 {
				label = i18n.F("已选中 %d 项 · %s", "%d selected · %s", len(l.ctx.SelectedFiles), name)
			}
			pureui.PillBadge(c, label, pal.KindFiles, pal.KindFilesSoft).MinWidth(0).Shrink(1)
		}
		if l.ctx.Clipboard != nil && !l.ctx.Clipboard.IsEmpty() {
			switch l.ctx.Clipboard.PrimaryKind() {
			case core.KindImage:
				pureui.PillBadge(c, i18n.S("剪贴板图片", "Clipboard Image"), pal.KindImage, pal.KindImageSoft)
			case core.KindFiles:
				pureui.PillBadge(c, i18n.S("剪贴板文件", "Clipboard Files"), pal.KindFiles, pal.KindFilesSoft)
			default:
				pureui.PillBadge(c, i18n.S("剪贴板文本", "Clipboard Text"), pal.KindText, pal.KindTextSoft)
			}
		}
		ui.Spacer(c)
		ui.Button(c, i18n.S("清除上下文", "Clear")).Shrink(0).Background(pal.ControlBg).Border(0, ui.Transparent).OnClick(func() { l.clearContext() })
	})
}

func (l *launcherView) renderRow(row ui.ListRow, pal pureui.Palette, selectedBg ui.Color) {
	idx := row.Index
	item := l.items[idx]
	c := row.Context
	selected := row.Selected()

	r := ui.Row(c).Height(48).Padding(6, 10).Gap(10).Radius(9).AlignItems(ui.Center)
	bg := ui.Transparent
	if selected {
		bg = selectedBg
	} else if r.Hovered() {
		bg = pal.CardHover
	}
	r.Background(bg)
	if r.Clicked() {
		l.selected = idx
		l.execute(idx)
		return
	}

	iconBg := pal.ControlBg
	iconFg := pal.TextSecondary
	if selected || item.IsMatch {
		iconBg = pal.AccentSoft
		iconFg = pal.Accent
	}
	icon := item.Icon
	if icon == nil {
		icon = pureui.IconSearch
	}

	r.Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).Background(iconBg).Center().Shrink(0).Children(func() {
			ui.Icon(c, icon).Size(15, 15).TextColor(iconFg)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			weight := 500
			if selected {
				weight = 600
			}
			ui.Text(c, item.Title).FontSize(13).FontWeight(weight).SingleLine().TextColor(pal.Text)
			if item.Subtitle != "" {
				ui.Text(c, item.Subtitle).FontSize(10.5).SingleLine().TextColor(pal.TextMuted)
			}
		})
		if item.Badge != "" {
			if item.IsMatch {
				pureui.PillBadge(c, item.Badge, pal.Accent, pal.AccentSoft)
			} else {
				pureui.PillBadge(c, item.Badge, pal.TextSecondary, pal.ControlBg)
			}
		}
		if item.Shortcut != "" {
			pureui.Keycap(c, item.Shortcut, pal)
		}
		if idx < 9 {
			pureui.Keycap(c, fmt.Sprintf("Ctrl+%d", idx+1), pal)
		}
	})
}
