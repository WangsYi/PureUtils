package purezip

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/platform"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/purezip/archive"
)

// PluginID is the stable identifier of the PureZip plugin.
const PluginID = "purezip"

//go:embed zip.png
var zipIconPNG []byte

// zipExtractMode chooses where an extraction writes.
type zipExtractMode int

const (
	zipExtractSmart  zipExtractMode = iota // single root -> here, else <name>\
	zipExtractHere                         // archive directory
	zipExtractFolder                       // <archive dir>\<name>\
	zipExtractCustom                       // Dest as given
)

type zipExtractRequest struct {
	Archive    string
	Mode       zipExtractMode
	Dest       string
	Paths      []string
	BaseDir    string
	Password   string
	Overwrite  archive.OverwriteMode
	OpenFolder bool
	DeleteArc  bool
}

type zipCompressRequest struct {
	Dest          string
	Sources       []string
	Options       archive.CreateOptions
	DeleteSources bool
	OpenFolder    bool
}

// zipBatcher coalesces per-file Explorer invocations (one process per selected
// item) into a single compress / extract request.
type zipBatcher struct {
	mu    sync.Mutex
	cmd   core.Command
	paths []string
	timer *time.Timer
}

// Plugin adapts PureZip's archive manager, dialogs, jobs, and shell integration
// to purekit.Plugin.
type Plugin struct {
	mu   sync.RWMutex
	host purekit.Host
	cfg  config.PureZipConfig

	views   []*zipView
	dialogs []*mygo.Window
	jobsMu  sync.Mutex
	jobs    []*zipJob
	batch   zipBatcher
	icons   zipIconCache

	promptHook   func(*zipPrompt) zipPromptReply
	lastCompress *compressDialog
	lastExtract  *extractDialog
}

// NewPlugin constructs an uninitialized PureZip plugin instance.
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string { return PluginID }

func (p *Plugin) Info() purekit.PluginInfo {
	return purekit.PluginInfo{
		ID:          PluginID,
		Name:        "PureZip",
		Description: "压缩包浏览、智能解压、多线程压缩与资源管理器右键集成",
		Version:     core.AppVersion,
		ModuleID:    core.ModuleZip,
		IconPNG:     zipIconPNG,
	}
}

func (p *Plugin) Init(host purekit.Host) error {
	if host == nil {
		return fmt.Errorf("purezip: nil host")
	}
	cfg := config.LoadPureZip(host.Config(PluginID))
	p.mu.Lock()
	p.host = host
	p.cfg = cfg
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Start() error { return nil }

func (p *Plugin) Stop() error {
	p.batch.mu.Lock()
	if p.batch.timer != nil {
		p.batch.timer.Stop()
		p.batch.timer = nil
	}
	p.batch.paths = nil
	p.batch.mu.Unlock()

	p.jobsMu.Lock()
	jobs := append([]*zipJob(nil), p.jobs...)
	p.jobsMu.Unlock()
	for _, j := range jobs {
		j.Cancel()
	}

	if !p.headless() {
		p.mu.RLock()
		views := append([]*zipView(nil), p.views...)
		dialogs := append([]*mygo.Window(nil), p.dialogs...)
		p.mu.RUnlock()
		for _, d := range dialogs {
			if d != nil {
				d.Close()
			}
		}
		for _, v := range views {
			v.closed = true
			v.closeArchive()
			if v.win != nil {
				v.win.Close()
			}
		}
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

func (p *Plugin) copyText(text string) {
	if text == "" || p.headless() {
		return
	}
	p.mu.RLock()
	h := p.host
	p.mu.RUnlock()
	if h != nil && h.Clipboard() != nil {
		_ = h.Clipboard().WriteText(text, PluginID)
		return
	}
	mygo.Clipboard.WriteText(text)
}

// Config returns a copy of the active PureZip configuration.
func (p *Plugin) Config() config.PureZipConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := p.cfg
	out.RecentArchives = append([]string(nil), p.cfg.RecentArchives...)
	return out
}

// ApplyConfig normalizes and persists next as PureZip's configuration.
func (p *Plugin) ApplyConfig(next config.PureZipConfig) error {
	next.Normalize()
	p.mu.Lock()
	h := p.host
	p.cfg = next
	p.cfg.RecentArchives = append([]string(nil), next.RecentArchives...)
	p.mu.Unlock()
	if h != nil {
		return config.SavePureZip(h.Config(PluginID), next)
	}
	return nil
}

// HandleCommand routes Zip CLI and action verbs, coalescing rapid Explorer calls.
func (p *Plugin) HandleCommand(cmd core.Command, paths []string) {
	abs := make([]string, 0, len(paths))
	for _, raw := range paths {
		if s := strings.TrimSpace(strings.Trim(raw, `"`)); s != "" {
			if ap, err := filepath.Abs(s); err == nil {
				s = ap
			}
			abs = append(abs, s)
		}
	}
	if p.headless() || cmd == core.CmdOpenZip {
		p.zipRun(cmd, abs)
		return
	}
	b := &p.batch
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.timer != nil && b.cmd != cmd {
		pendCmd, pend := b.cmd, b.paths
		b.timer.Stop()
		b.timer, b.paths = nil, nil
		mygo.RunOnMain(func() { p.zipRun(pendCmd, pend) })
	}
	b.cmd = cmd
	b.paths = append(b.paths, abs...)
	if b.timer != nil {
		b.timer.Reset(350 * time.Millisecond)
		return
	}
	b.timer = time.AfterFunc(350*time.Millisecond, func() {
		b.mu.Lock()
		c, ps := b.cmd, dedupePaths(b.paths)
		b.paths, b.timer = nil, nil
		b.mu.Unlock()
		mygo.RunOnMain(func() { p.zipRun(c, ps) })
	})
}

func dedupePaths(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, p := range in {
		k := strings.ToLower(p)
		if !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}

// zipRun executes a coalesced Zip command on the UI thread.
func (p *Plugin) zipRun(cmd core.Command, paths []string) {
	s := p.Config()
	switch cmd {
	case core.CmdOpenZip:
		if len(paths) == 0 {
			p.OpenZipManager("")
		}
		for _, path := range paths {
			p.OpenZipManager(path)
		}
	case core.CmdExtractSmart, core.CmdExtractHere, core.CmdExtractTo:
		mode := map[core.Command]zipExtractMode{
			core.CmdExtractSmart: zipExtractSmart,
			core.CmdExtractHere:  zipExtractHere,
			core.CmdExtractTo:    zipExtractFolder,
		}[cmd]
		for _, path := range paths {
			p.startExtract(zipExtractRequest{Archive: path, Mode: mode, Overwrite: zipOverwriteMode(s.OverwriteMode), OpenFolder: s.OpenFolderAfterExtract})
		}
	case core.CmdExtractDialog:
		if len(paths) > 0 {
			p.ShowExtractDialog(paths, nil, "", "")
		}
	case core.CmdCompress:
		p.ShowCompressDialog(paths, "")
	case core.CmdCompressZip, core.CmdCompress7z:
		if len(paths) == 0 {
			return
		}
		f := archive.FormatZip
		if cmd == core.CmdCompress7z {
			f = archive.Format7z
		}
		p.startCompress(zipCompressRequest{
			Dest:    zipDefaultTarget(paths, f),
			Sources: paths,
			Options: archive.CreateOptions{Format: f, Level: s.Level},
		})
	case core.CmdTestArchive:
		for _, path := range paths {
			p.startTestArchive(path, "")
		}
	}
}

// openArchiveForJob opens path, asking for a password if the headers are encrypted.
func (p *Plugin) openArchiveForJob(j *zipJob, path, pw string) (*archive.Archive, string, error) {
	var arc *archive.Archive
	enc := p.Config().NameEncoding
	pw, err := j.withPassword(filepath.Base(path), pw, func(pw string) error {
		var err error
		arc, err = archive.Open(path, archive.OpenOptions{Password: pw, Encoding: enc})
		return err
	})
	return arc, pw, err
}

// startExtract extracts an archive in the background with a progress window.
func (p *Plugin) startExtract(req zipExtractRequest) {
	j := newZipJob(p, "解压", filepath.Base(req.Archive))
	p.startZipJob(j, func(j *zipJob) (string, error) {
		arc, pw, err := p.openArchiveForJob(j, req.Archive, req.Password)
		if err != nil {
			return "", err
		}
		defer arc.Close()
		if pw != "" {
			arc.SetPassword(pw)
		}
		dir := filepath.Dir(req.Archive)
		dest := req.Dest
		createsFolder := false
		switch req.Mode {
		case zipExtractSmart:
			dest, createsFolder = archive.SmartDestination(req.Archive, arc.Entries())
			if req.Dest != "" {
				rel, _ := filepath.Rel(dir, dest)
				dest = filepath.Join(req.Dest, rel)
			}
		case zipExtractHere:
			dest = dir
		case zipExtractFolder:
			base := dir
			if req.Dest != "" {
				base = req.Dest
			}
			dest = filepath.Join(base, archive.BaseName(req.Archive))
			createsFolder = true
		}
		if dest == "" {
			dest = dir
		}
		opts := archive.ExtractOptions{
			Dest:         dest,
			Paths:        req.Paths,
			BaseDir:      req.BaseDir,
			Overwrite:    req.Overwrite,
			OnConflict:   j.askConflict,
			Progress:     j.report,
			PreserveTime: true,
		}
		var res archive.ExtractResult
		_, err = j.withPassword(filepath.Base(req.Archive), arc.Password(), func(pw string) error {
			if pw != "" {
				arc.SetPassword(pw)
			}
			var err error
			res, err = arc.Extract(j.ctx, opts)
			return err
		})
		if err != nil {
			return "", err
		}
		j.target = dest
		if !createsFolder && len(res.Created) == 1 {
			j.target = res.Created[0]
		}
		if req.OpenFolder && !p.headless() {
			target := j.target
			reveal := !createsFolder && len(res.Created) == 1
			mygo.RunOnMain(func() {
				if reveal {
					mygo.Shell.ShowItemInFolder(target)
				} else {
					_ = mygo.Shell.OpenPath(target)
				}
			})
		}
		if req.DeleteArc {
			_ = mygo.Shell.TrashItem(req.Archive)
		}
		summary := fmt.Sprintf("已解压 %d 个文件（%s）到 %s", res.Files, zipSize(res.Bytes), dest)
		if res.Skipped > 0 {
			summary += fmt.Sprintf("，跳过 %d 个", res.Skipped)
		}
		return summary, nil
	})
}

// startCompress creates an archive in the background with a progress window.
func (p *Plugin) startCompress(req zipCompressRequest) *zipJob {
	j := newZipJob(p, "压缩", filepath.Base(req.Dest))
	j.target = req.Dest
	p.startZipJob(j, func(j *zipJob) (string, error) {
		opt := req.Options
		opt.Progress = j.report
		if err := archive.Create(j.ctx, req.Dest, req.Sources, opt); err != nil {
			return "", err
		}
		if req.DeleteSources {
			for _, s := range req.Sources {
				_ = mygo.Shell.TrashItem(s)
			}
		}
		if req.OpenFolder && !p.headless() {
			dest := req.Dest
			mygo.RunOnMain(func() { mygo.Shell.ShowItemInFolder(dest) })
		}
		size := int64(-1)
		if fi, err := os.Stat(req.Dest); err == nil {
			size = fi.Size()
		}
		return fmt.Sprintf("已创建 %s（%s）", filepath.Base(req.Dest), zipSize(size)), nil
	})
	return j
}

// startTestArchive verifies every entry of an archive.
func (p *Plugin) startTestArchive(path, pw string) *zipJob {
	j := newZipJob(p, "测试", filepath.Base(path))
	j.keepOpen = true
	p.startZipJob(j, func(j *zipJob) (string, error) {
		arc, pw, err := p.openArchiveForJob(j, path, pw)
		if err != nil {
			return "", err
		}
		defer arc.Close()
		_, err = j.withPassword(filepath.Base(path), pw, func(pw string) error {
			if pw != "" {
				arc.SetPassword(pw)
			}
			return arc.Test(j.ctx, j.report)
		})
		if err != nil {
			return "", err
		}
		info := arc.Info()
		return fmt.Sprintf("未发现错误：共 %d 个文件、%d 个文件夹，原始大小 %s。", info.Files, info.Dirs, zipSize(info.TotalSize)), nil
	})
	return j
}

// startArchiveEdit rewrites an archive (add / delete / rename) in the background.
func (p *Plugin) startArchiveEdit(verb, path string, work func(j *zipJob, opt archive.CreateOptions) error, pw string, onDone func(err error)) *zipJob {
	j := newZipJob(p, verb, filepath.Base(path))
	j.onDone = onDone
	p.startZipJob(j, func(j *zipJob) (string, error) {
		f, _ := archive.DetectFormat(path)
		cfg := p.Config()
		opt := archive.CreateOptions{Format: f, Level: cfg.Level, Password: pw, Progress: j.report}
		if pw != "" && f == archive.FormatZip {
			opt.Encryption = archive.ZipEncryption(cfg.Encryption)
		}
		if err := work(j, opt); err != nil {
			return "", err
		}
		return "已更新 " + filepath.Base(path), nil
	})
	return j
}

// rememberArchive records path in the recent archive list.
func (p *Plugin) rememberArchive(path string) {
	if p.headless() || path == "" {
		return
	}
	next := p.Config()
	next.AddRecentArchive(path)
	_ = p.ApplyConfig(next)
}

func (p *Plugin) applyZipWindowIdentity(win *mygo.Window) {
	if p == nil || win == nil || p.headless() || p.host == nil {
		return
	}
	ico := platform.EnsureICOFromPNG(zipIconPNG, "zip.ico", filepath.Join(p.host.Paths().Home, "icons", "zip.ico"))
	iconRes := ico
	if iconRes != "" {
		iconRes += ",0"
	}
	relaunch := ""
	if exe, err := os.Executable(); err == nil {
		if abs, err := filepath.Abs(exe); err == nil {
			exe = abs
		}
		relaunch = `"` + exe + `" --zip`
	}
	_ = platform.SetWindowTaskbarIdentity(platform.HWND(win.NativeHandle()), "PureUtils.PureZip", "PureZip", relaunch, iconRes)
	_ = win.SetIcon(zipIconPNG)
}

// OpenZipManager opens the archive manager window (empty start page for "").
func (p *Plugin) OpenZipManager(path string) {
	for _, v := range p.views {
		if v.win != nil && (strings.EqualFold(v.path, path) || (path != "" && v.path == "" && !v.loading)) {
			if path != "" && v.path == "" {
				v.load(path, "")
			}
			v.win.Show()
			v.win.Focus()
			return
		}
	}
	v := newZipView(p)
	if p.headless() {
		p.views = append(p.views, v)
		if path != "" {
			v.load(path, "")
		}
		return
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     "PureZip",
		Width:     1000,
		Height:    660,
		MinWidth:  720,
		MinHeight: 460,
		Hidden:    true,
		StateKey:  "zip",
		Content:   zipViewContent(v),
	})
	p.applyZipWindowIdentity(win)
	v.win = win
	win.OnClose(func(_ *mygo.CloseEvent) {
		v.closed = true
		v.closeArchive()
		p.removeZipView(v)
		p.notifyWindowClosed()
	})
	p.views = append(p.views, v)
	if path != "" {
		v.load(path, "")
	}
	win.Show()
	win.Focus()
}

func (p *Plugin) removeZipView(v *zipView) {
	for i, x := range p.views {
		if x == v {
			p.views = append(p.views[:i], p.views[i+1:]...)
			return
		}
	}
}

func (p *Plugin) HasOpenWindows() bool {
	p.jobsMu.Lock()
	jobs := len(p.jobs)
	p.jobsMu.Unlock()
	return len(p.views) > 0 || len(p.dialogs) > 0 || jobs > 0
}

func (p *Plugin) ShouldStayInTray() bool { return false }

func (p *Plugin) IsBusyForGlobalHooks() bool { return false }

func (p *Plugin) Features() []purekit.Feature {
	var openShortcut string
	if p.host != nil {
		openShortcut = p.host.HostConfig().Hotkeys.OpenZip
	}
	return []purekit.Feature{
		{
			ID:       "purezip.open",
			PluginID: PluginID,
			Title:    "打开 PureZip 压缩管理",
			Subtitle: "浏览、解压、加密或编辑压缩包",
			Keywords: []string{"zip", "7z", "rar", "tar", "archive", "压缩", "解压", "归档", "purezip"},
			Command:  core.CmdOpenZip,
			Shortcut: openShortcut,
			Badge:    "PureZip",
			Icon:     iconArchive,
			Run: func(_ context.Context, in purekit.ActionInput) error {
				p.HandleCommand(core.CmdOpenZip, in.Files)
				return nil
			},
		},
		{
			ID:       "purezip.compress",
			PluginID: PluginID,
			Title:    "PureZip 新建压缩文件",
			Subtitle: "将文件或文件夹打包为 ZIP / 7Z / TAR",
			Keywords: []string{"compress", "pack", "zip", "7z", "打包", "新建压缩"},
			Command:  core.CmdCompress,
			Badge:    "PureZip",
			Icon:     iconCompress,
			Run: func(_ context.Context, in purekit.ActionInput) error {
				p.ShowCompressDialog(in.Files, "")
				return nil
			},
		},
	}
}

func (p *Plugin) Matchers() []purekit.Matcher {
	return []purekit.Matcher{zipMatcher{plugin: p}}
}

type zipMatcher struct {
	plugin *Plugin
}

func (m zipMatcher) ID() string    { return "purezip.matcher" }
func (m zipMatcher) Title() string { return "PureZip 压缩匹配" }

func (m zipMatcher) Match(ctx purekit.MatchContext) []purekit.MatchResult {
	var archives []string
	seen := make(map[string]bool)
	addArchive := func(raw string) {
		raw = strings.TrimSpace(strings.Trim(raw, `"`))
		if raw == "" || !archive.IsArchivePath(raw) {
			return
		}
		k := strings.ToLower(raw)
		if seen[k] {
			return
		}
		seen[k] = true
		archives = append(archives, raw)
	}
	for _, f := range ctx.SelectedFiles {
		addArchive(f)
	}
	if q := strings.TrimSpace(strings.Trim(ctx.Query, `"`)); q != "" && archive.IsArchivePath(q) {
		if st, err := os.Stat(q); err == nil && !st.IsDir() {
			addArchive(q)
		}
	}

	var results []purekit.MatchResult
	if len(archives) > 0 {
		first := archives[0]
		allArcs := append([]string(nil), archives...)
		label := filepath.Base(first)
		if len(allArcs) > 1 {
			label = fmt.Sprintf("%s 等 %d 个压缩包", label, len(allArcs))
		}
		results = append(results,
			purekit.MatchResult{
				FeatureID: "purezip.open",
				PluginID:  PluginID,
				Title:     "用 PureZip 打开 " + filepath.Base(first),
				Subtitle:  first,
				Score:     95,
				Badge:     "PureZip",
				Icon:      iconArchive,
				Input:     purekit.ActionInput{Files: []string{first}, Source: "matcher"},
				Execute: func(_ context.Context) error {
					m.plugin.OpenZipManager(first)
					return nil
				},
			},
			purekit.MatchResult{
				FeatureID: "purezip.extract_smart",
				PluginID:  PluginID,
				Title:     "用 PureZip 智能解压 " + label,
				Subtitle:  filepath.Dir(first),
				Score:     90,
				Badge:     "PureZip",
				Icon:      iconExtract,
				Input:     purekit.ActionInput{Files: allArcs, Source: "matcher"},
				Execute: func(_ context.Context) error {
					m.plugin.HandleCommand(core.CmdExtractSmart, allArcs)
					return nil
				},
			},
		)
	}

	if len(ctx.SelectedFiles) > 0 {
		files := append([]string(nil), ctx.SelectedFiles...)
		score := 76
		if len(archives) == len(files) {
			score = 70
		}
		sub := files[0]
		if len(files) > 1 {
			sub = fmt.Sprintf("%s 等 %d 项", filepath.Base(files[0]), len(files))
		}
		results = append(results, purekit.MatchResult{
			FeatureID: "purezip.compress",
			PluginID:  PluginID,
			Title:     "用 PureZip 压缩选中项目",
			Subtitle:  sub,
			Score:     score,
			Badge:     "PureZip",
			Icon:      iconCompress,
			Input:     purekit.ActionInput{Files: files, Source: "matcher"},
			Execute: func(_ context.Context) error {
				m.plugin.ShowCompressDialog(files, "")
				return nil
			},
		})
	}
	return results
}

func (p *Plugin) Hotkeys() []purekit.HotkeySpec {
	return []purekit.HotkeySpec{
		{
			ID:             "purezip.open",
			Command:        core.CmdOpenZip,
			Label:          "打开 PureZip 压缩管理",
			DefaultGesture: "",
			CurrentGesture: func() string {
				if p.host == nil {
					return ""
				}
				return p.host.HostConfig().Hotkeys.OpenZip
			},
			SetGesture: func(g string) error {
				if p.host == nil {
					return nil
				}
				hc := p.host.HostConfig()
				hc.Hotkeys.OpenZip = g
				return p.host.SaveHostConfig(hc)
			},
			Callback: func() { p.OpenZipManager("") },
		},
	}
}

func (p *Plugin) TrayItems() []purekit.TrayItemSpec {
	var accel string
	if p.host != nil {
		accel = p.host.HostConfig().Hotkeys.OpenZip
	}
	return []purekit.TrayItemSpec{
		{
			ID:          "purezip.open",
			Section:     purekit.TraySectionPrimary,
			Kind:        purekit.TrayItemAction,
			Label:       "打开 PureZip 压缩管理",
			Accelerator: accel,
			Command:     core.CmdOpenZip,
			OnClick:     func() { p.OpenZipManager("") },
		},
	}
}

func (p *Plugin) CLICommands() []purekit.CLICommandSpec {
	cmds := []core.Command{
		core.CmdOpenZip,
		core.CmdExtractSmart,
		core.CmdExtractHere,
		core.CmdExtractTo,
		core.CmdExtractDialog,
		core.CmdCompress,
		core.CmdCompressZip,
		core.CmdCompress7z,
		core.CmdTestArchive,
	}
	out := make([]purekit.CLICommandSpec, 0, len(cmds))
	for _, cmd := range cmds {
		c := cmd
		out = append(out, purekit.CLICommandSpec{
			Command:    c,
			TakesPaths: true,
			Execute:    func(paths []string) { p.HandleCommand(c, paths) },
		})
	}
	return out
}

func (p *Plugin) SettingsTab() *purekit.SettingsTab {
	return &purekit.SettingsTab{
		ID:       PluginID,
		Title:    "PureZip · 压缩",
		Subtitle: "PureZip 压缩、解压与右键集成",
		Icon:     pureui.IconPackage,
		Order:    40,
		NewView: func(host purekit.Host) purekit.SettingsTabView {
			return newSettingsTabView(p, host)
		},
	}
}

// pickFiles shows the native open dialog off the UI thread.
func (p *Plugin) pickFiles(parent *mygo.Window, title string, dirs, multiple bool, filters []mygo.FileFilter, done func([]string)) {
	if p.headless() {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: title, Directory: dirs, Multiple: multiple, Filters: filters})
		if err != nil || len(paths) == 0 {
			return
		}
		mygo.RunOnMain(func() { done(paths) })
	}()
}

// archiveFilters lists readable archive extensions for the open dialog.
func archiveFilters() []mygo.FileFilter {
	exts := append([]string(nil), archive.ReadableExtensions...)
	sort.Strings(exts)
	return []mygo.FileFilter{{Name: "压缩文件", Extensions: exts}, {Name: "所有文件", Extensions: []string{"*"}}}
}

// zipErrText renders engine errors for the UI.
func zipErrText(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, archive.ErrPasswordRequired):
		return "需要密码"
	case errors.Is(err, archive.ErrWrongPassword):
		return "密码错误"
	case errors.Is(err, archive.ErrUnsupported):
		return "不支持的压缩格式：" + err.Error()
	default:
		return err.Error()
	}
}
