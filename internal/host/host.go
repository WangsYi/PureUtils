package host

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	"pureutils/pkg/purekit/platform"
	"pureutils/pkg/purekit/storage"
)

//go:embed icon.png
var iconPNG []byte

// Options configures a PureUtils plugin host instance.
type Options struct {
	Paths     core.Paths
	Config    *config.Manager
	Modules   core.ModuleConfig
	Logger    *core.Logger
	Headless  bool
	Clipboard clipboard.Service
	Plugins   []purekit.Plugin
	QuitFunc  func()
}

// App is the thin PureUtils host kernel that assembles enabled plugins,
// shared services (Config/Storage/Clipboard/Bus), tray, hotkeys, and Settings.
type App struct {
	paths    core.Paths
	log      *core.Logger
	headless bool

	cfgMgr   *config.Manager
	storeMgr *storage.Manager
	clip     clipboard.Service
	bus      *purekit.Bus

	compiled  core.ModuleConfig
	installed core.ModuleConfig
	custom    bool

	pluginMu    sync.RWMutex
	modules     core.ModuleConfig
	plugins     []purekit.Plugin
	pluginByID  map[string]purekit.Plugin
	cliHandlers map[core.Command]purekit.CLICommandSpec

	tray               *mygo.Tray
	hotkeyMu           sync.RWMutex
	hotkeys            map[string]core.Gesture
	takeoverHotkeys    map[string]core.Gesture
	hotkeyErrs         map[core.Command]string
	hotkeyErrByID      map[string]string
	winTakeover        *platform.HotkeyTakeover
	registerShortcutFn func(acc string, fn func()) error

	hookMu       sync.Mutex
	hookSuppress map[string]bool

	settingsWin  *mygo.Window
	settingsView *settingsView

	launcherWin         *mygo.Window
	launcherView        *launcherView
	launcherCtxProvider func() purekit.MatchContext

	updater    *Updater
	updateMu   sync.Mutex
	updateStop chan struct{}

	trimMu    sync.Mutex
	trimTimer *time.Timer

	quitHook     func()
	started      atomic.Bool
	shuttingDown atomic.Bool
}

var _ purekit.Host = (*App)(nil)

// NewApp constructs the host shell and initializes enabled plugins.
// Construction is strictly read-only on legacy settings/databases.
func NewApp(opts Options) (*App, error) {
	cfgMgr := opts.Config
	if cfgMgr == nil {
		cfgMgr = config.Load(opts.Paths.Settings)
	}
	storeMgr, err := storage.NewManager(opts.Paths.Home)
	if err != nil {
		return nil, fmt.Errorf("host: init storage manager: %w", err)
	}

	hc := cfgMgr.Host()
	if !opts.Headless {
		lang := i18n.SetPreference(hc.Appearance.Language)
		mygo.App.SetLocale(i18n.MyGoLocale(lang))
	} else if normPref := i18n.NormalizePref(hc.Appearance.Language); normPref != i18n.PrefAuto {
		i18n.SetPreference(normPref)
	}
	clipSvc := opts.Clipboard
	if clipSvc == nil && !opts.Headless {
		mgr, err := clipboard.NewWin32Manager(clipboard.ManagerOptions{
			Debounce:          time.Duration(hc.Advanced.DebounceMs) * time.Millisecond,
			SelfWriteSuppress: time.Duration(hc.Advanced.SelfWriteSuppressMs) * time.Millisecond,
			Log:               opts.Logger,
		})
		if err != nil {
			return nil, fmt.Errorf("host: init clipboard manager: %w", err)
		}
		clipSvc = mgr
	}

	compiled := CompiledModules()
	installed := opts.Modules
	if !compiled.Capture {
		installed.Capture = false
	}
	if !compiled.History {
		installed.History = false
	}
	if !compiled.Viewer {
		installed.Viewer = false
	}
	if !compiled.Zip {
		installed.Zip = false
	}
	if !compiled.Video {
		installed.Video = false
	}
	if !compiled.Monitor {
		installed.Monitor = false
	}
	active := installed
	if opts.Plugins == nil {
		active = applyDisabledModules(installed, hc)
	}

	a := &App{
		paths:           opts.Paths,
		log:             opts.Logger,
		headless:        opts.Headless,
		cfgMgr:          cfgMgr,
		storeMgr:        storeMgr,
		clip:            clipSvc,
		bus:             purekit.NewBus(),
		compiled:        compiled,
		installed:       installed,
		modules:         active,
		custom:          opts.Plugins != nil,
		pluginByID:      map[string]purekit.Plugin{},
		cliHandlers:     map[core.Command]purekit.CLICommandSpec{},
		hotkeys:         map[string]core.Gesture{},
		takeoverHotkeys: map[string]core.Gesture{},
		hotkeyErrs:      map[core.Command]string{},
		hotkeyErrByID:   map[string]string{},
		hookSuppress:    map[string]bool{},
		quitHook:        opts.QuitFunc,
	}

	var trayStatusMu sync.Mutex
	var lastTrayStatus UpdateStatus
	a.updater = NewUpdater(UpdaterOptions{
		CurrentVersion: core.AppVersion,
		DataDir:        filepath.Join(opts.Paths.Home, "updates"),
		OnChange: func(snap UpdateSnapshot) {
			a.InvalidateSettings()
			trayStatusMu.Lock()
			changed := snap.Status != lastTrayStatus
			if changed {
				lastTrayStatus = snap.Status
			}
			trayStatusMu.Unlock()
			if changed && (snap.Status == UpdateAvailable || snap.Status == UpdateReady ||
				snap.Status == UpdateUpToDate || snap.Status == UpdateError || snap.Status == UpdateIdle) {
				a.RebuildTray()
			}
		},
		OnNotify: a.Notify,
		OnQuit:   a.Quit,
		Log:      opts.Logger,
	})

	var candidates []purekit.Plugin
	if opts.Plugins != nil {
		candidates = append(candidates, opts.Plugins...)
	} else {
		for _, f := range BuiltinPlugins() {
			if active.Enabled(f.ModuleID) && f.New != nil {
				candidates = append(candidates, f.New())
			}
		}
	}

	var initialized []purekit.Plugin
	for _, p := range candidates {
		if p == nil {
			continue
		}
		if err := p.Init(a); err != nil {
			for i := len(initialized) - 1; i >= 0; i-- {
				_ = initialized[i].Stop()
			}
			if opts.Clipboard == nil && a.clip != nil {
				a.clip.Close()
			}
			return nil, fmt.Errorf("host: init plugin %s: %w", p.ID(), err)
		}
		initialized = append(initialized, p)
		a.plugins = append(a.plugins, p)
		a.pluginByID[p.ID()] = p
		for _, spec := range p.CLICommands() {
			if spec.Command != "" {
				a.cliHandlers[spec.Command] = spec
			}
		}
	}
	return a, nil
}

// NewAppWithModules creates a host App for production startup from main.go.
func NewAppWithModules(paths core.Paths, cfgMgr *config.Manager, modules core.ModuleConfig, log *core.Logger) (*App, error) {
	return NewApp(Options{
		Paths:   paths,
		Config:  cfgMgr,
		Modules: modules,
		Logger:  log,
	})
}

func (a *App) Config(pluginID string) config.Section {
	return a.cfgMgr.Section(pluginID)
}

func (a *App) HostConfig() config.HostConfig {
	return a.cfgMgr.Host()
}

func (a *App) SaveHostConfig(cfg config.HostConfig) error {
	prevHC := a.cfgMgr.Host()
	if err := a.cfgMgr.SaveHost(cfg); err != nil {
		if a.log != nil {
			a.log.Errorf("保存设置失败: %v", err)
		}
		return err
	}
	hc := a.cfgMgr.Host()
	prevLang := i18n.Current()
	langChanged := false
	if !a.headless {
		nextLang := i18n.SetPreference(hc.Appearance.Language)
		langChanged = nextLang != prevLang
		mygo.App.SetLocale(i18n.MyGoLocale(nextLang))
	} else if normPref := i18n.NormalizePref(hc.Appearance.Language); normPref != i18n.PrefAuto {
		nextLang := i18n.SetPreference(normPref)
		langChanged = nextLang != prevLang
	}
	if a.clip != nil {
		a.clip.SetTimings(
			time.Duration(hc.Advanced.DebounceMs)*time.Millisecond,
			time.Duration(hc.Advanced.SelfWriteSuppressMs)*time.Millisecond,
		)
	}
	modulesChanged, modErr := a.applyPluginModules(hc)
	if !a.headless {
		mygo.Theme.SetSource(mygo.ThemeSource(hc.ThemeSource()))
		if !isIsolatedTestInstance() {
			a.ReloadHotkeys()
			startupChanged := hc.Advanced.RunOnStartup != prevHC.Advanced.RunOnStartup ||
				hc.Advanced.RunElevatedOnStartup != prevHC.Advanced.RunElevatedOnStartup
			if serr := a.applyStartupSetting(startupChanged); serr != nil {
				if hc.Advanced.RunElevatedOnStartup && !prevHC.Advanced.RunElevatedOnStartup {
					hc.Advanced.RunElevatedOnStartup = false
					_ = a.cfgMgr.SaveHost(hc)
				}
				if a.log != nil {
					a.log.Warnf("更新开机自启失败: %v", serr)
				}
				return serr
			}
		}
		if a.ShouldStayInTray() && a.tray == nil {
			a.createTray()
		} else if !a.ShouldStayInTray() && !a.HasOpenWindows() {
			a.Quit()
			return modErr
		}
	} else if modulesChanged {
		a.ReloadHotkeys()
	}
	if langChanged && a.settingsView != nil && !a.settingsView.closed && !a.settingsView.saving {
		if a.headless {
			a.settingsView.refreshForLanguageChange()
		} else {
			mygo.RunOnMain(func() {
				if a.settingsView != nil && !a.settingsView.closed {
					a.settingsView.refreshForLanguageChange()
				}
			})
		}
	}
	a.RebuildTray()
	if a.log != nil {
		a.log.Infof("设置已保存")
	}
	return modErr
}

func applyDisabledModules(installed core.ModuleConfig, hc config.HostConfig) core.ModuleConfig {
	out := installed
	if hc.IsModuleDisabled(core.ModuleCapture) {
		out.Capture = false
	}
	if hc.IsModuleDisabled(core.ModuleHistory) {
		out.History = false
	}
	if hc.IsModuleDisabled(core.ModuleViewer) {
		out.Viewer = false
	}
	if hc.IsModuleDisabled(core.ModuleZip) {
		out.Zip = false
	}
	if hc.IsModuleDisabled(core.ModuleVideo) {
		out.Video = false
	}
	if hc.IsModuleDisabled(core.ModuleMonitor) {
		out.Monitor = false
	}
	return out
}

func (a *App) snapshotPlugins() []purekit.Plugin {
	a.pluginMu.RLock()
	plugins := a.plugins
	a.pluginMu.RUnlock()
	return plugins
}

func (a *App) applyPluginModules(hc config.HostConfig) (bool, error) {
	if a.custom {
		return false, nil
	}
	desired := applyDisabledModules(a.installed, hc)

	a.pluginMu.RLock()
	curActive := a.modules
	curPlugins := a.plugins
	a.pluginMu.RUnlock()
	if desired == curActive {
		return false, nil
	}

	var toStop []purekit.Plugin
	var kept []purekit.Plugin
	keptByID := map[string]purekit.Plugin{}
	for _, p := range curPlugins {
		mod := p.Info().ModuleID
		if mod != "" && !desired.Enabled(mod) {
			toStop = append(toStop, p)
			continue
		}
		kept = append(kept, p)
		keptByID[p.ID()] = p
	}

	if len(toStop) > 0 {
		keptCLI := make(map[core.Command]purekit.CLICommandSpec)
		for _, p := range kept {
			for _, spec := range p.CLICommands() {
				if spec.Command != "" {
					keptCLI[spec.Command] = spec
				}
			}
		}
		stoppedMods := curActive
		if !desired.Capture {
			stoppedMods.Capture = false
		}
		if !desired.History {
			stoppedMods.History = false
		}
		if !desired.Viewer {
			stoppedMods.Viewer = false
		}
		if !desired.Zip {
			stoppedMods.Zip = false
		}
		if !desired.Video {
			stoppedMods.Video = false
		}
		if !desired.Monitor {
			stoppedMods.Monitor = false
		}
		a.pluginMu.Lock()
		a.plugins = kept
		a.pluginByID = keptByID
		a.cliHandlers = keptCLI
		a.modules = stoppedMods
		a.pluginMu.Unlock()

		for i := len(toStop) - 1; i >= 0; i-- {
			p := toStop[i]
			if err := p.Stop(); err != nil && a.log != nil {
				a.log.Warnf("停用组件 %s 时出错: %v", p.ID(), err)
			} else if a.log != nil {
				a.log.Infof("已停用组件 %s", p.ID())
			}
		}
	}

	actualMods := desired
	nextByID := make(map[string]purekit.Plugin, len(keptByID))
	mapsCopy := func() {
		a.pluginMu.RLock()
		for k, v := range a.pluginByID {
			nextByID[k] = v
		}
		a.pluginMu.RUnlock()
	}
	mapsCopy()

	var firstErr error
	factories := BuiltinPlugins()
	for _, f := range factories {
		if !desired.Enabled(f.ModuleID) || f.New == nil {
			continue
		}
		if _, exists := nextByID[f.ID]; exists {
			continue
		}
		p := f.New()
		if p == nil {
			continue
		}
		if err := p.Init(a); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s", i18n.F("启用组件 %s 失败: %w", "Failed to enable module %s: %w", f.ID, err))
			}
			if a.log != nil {
				a.log.Errorf("热启用组件 %s Init 失败: %v", f.ID, err)
			}
			switch f.ModuleID {
			case core.ModuleCapture:
				actualMods.Capture = false
			case core.ModuleHistory:
				actualMods.History = false
			case core.ModuleViewer:
				actualMods.Viewer = false
			case core.ModuleZip:
				actualMods.Zip = false
			case core.ModuleVideo:
				actualMods.Video = false
			case core.ModuleMonitor:
				actualMods.Monitor = false
			}
			continue
		}
		if a.started.Load() {
			if err := p.Start(); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s", i18n.F("启动组件 %s 失败: %w", "Failed to start module %s: %w", p.ID(), err))
				}
				if a.log != nil {
					a.log.Errorf("热启用组件 %s Start 失败: %v", p.ID(), err)
				}
			}
		}
		nextByID[p.ID()] = p
		if a.log != nil {
			a.log.Infof("已启用组件 %s", p.ID())
		}
	}

	nextPlugins := make([]purekit.Plugin, 0, len(nextByID))
	added := make(map[string]bool, len(nextByID))
	for _, f := range factories {
		if p, ok := nextByID[f.ID]; ok {
			nextPlugins = append(nextPlugins, p)
			added[f.ID] = true
		}
	}
	for _, p := range curPlugins {
		if _, ok := nextByID[p.ID()]; ok && !added[p.ID()] {
			nextPlugins = append(nextPlugins, p)
			added[p.ID()] = true
		}
	}
	nextCLI := make(map[core.Command]purekit.CLICommandSpec)
	for _, p := range nextPlugins {
		for _, spec := range p.CLICommands() {
			if spec.Command != "" {
				nextCLI[spec.Command] = spec
			}
		}
	}

	a.pluginMu.Lock()
	a.plugins = nextPlugins
	a.pluginByID = nextByID
	a.cliHandlers = nextCLI
	a.modules = actualMods
	a.pluginMu.Unlock()

	return true, firstErr
}

func (a *App) Storage(pluginID string) storage.PluginStorage {
	return a.storeMgr.Plugin(pluginID)
}

func (a *App) Clipboard() clipboard.Service { return a.clip }
func (a *App) Bus() *purekit.Bus            { return a.bus }
func (a *App) Log() *core.Logger            { return a.log }
func (a *App) Headless() bool               { return a.headless }
func (a *App) Paths() core.Paths            { return a.paths }

func (a *App) Notify(title, body string) {
	if a.headless {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body})
	if err := n.Show(); err != nil && a.log != nil {
		a.log.Warnf("通知失败 %s: %v", title, err)
	}
}

func (a *App) ModuleEnabled(id core.ModuleID) bool {
	if a.custom {
		for _, p := range a.snapshotPlugins() {
			if p.Info().ModuleID == id {
				return true
			}
		}
		return false
	}
	return a.compiled.Enabled(id) && a.ActiveModules().Enabled(id)
}

func (a *App) ReloadHotkeys() {
	a.registerHotkeys()
}

func (a *App) OpenSettings(tabID string) {
	if a.headless {
		a.ShowSettings(tabID)
		return
	}
	mygo.RunOnMain(func() { a.ShowSettings(tabID) })
}

// InvalidateSettings triggers a thread-safe repaint of the Settings window if open.
func (a *App) InvalidateSettings() {
	if a.headless || a.shuttingDown.Load() {
		return
	}
	mygo.RunOnMain(func() {
		if a.settingsView != nil && !a.settingsView.closed {
			a.settingsView.invalidate()
		}
	})
}

func (a *App) ExecuteCommand(cmd core.Command, paths ...string) {
	copied := append([]string(nil), paths...)
	if a.headless {
		a.dispatch(cmd, copied)
		return
	}
	mygo.RunOnMain(func() { a.dispatch(cmd, copied) })
}

func (a *App) HotkeyError(cmd core.Command) string {
	a.hotkeyMu.RLock()
	defer a.hotkeyMu.RUnlock()
	return a.hotkeyErrs[cmd]
}

// HotkeyErrorByID returns any registration error associated with a specific HotkeySpec ID.
func (a *App) HotkeyErrorByID(id string) string {
	a.hotkeyMu.RLock()
	defer a.hotkeyMu.RUnlock()
	return a.hotkeyErrByID[id]
}

func (a *App) SetGlobalHookSuppressed(source string, suppressed bool) {
	if source == "" {
		return
	}
	a.hookMu.Lock()
	if suppressed {
		a.hookSuppress[source] = true
	} else {
		delete(a.hookSuppress, source)
	}
	a.hookMu.Unlock()
}

func (a *App) IsGlobalHookSuppressed() bool {
	a.hookMu.Lock()
	busy := len(a.hookSuppress) > 0
	a.hookMu.Unlock()
	if busy {
		return true
	}
	for _, p := range a.snapshotPlugins() {
		if p.IsBusyForGlobalHooks() {
			return true
		}
	}
	return false
}

func (a *App) NotifyWindowClosed() {
	if !a.ShouldStayInTray() && !a.HasOpenWindows() {
		a.Quit()
		return
	}
	if !a.HasOpenWindows() {
		a.scheduleIdleMemoryTrim()
	}
}

func (a *App) scheduleIdleMemoryTrim() {
	if a.headless || a.shuttingDown.Load() {
		return
	}
	a.trimMu.Lock()
	if a.trimTimer != nil {
		a.trimTimer.Stop()
	}
	a.trimTimer = time.AfterFunc(2*time.Second, func() {
		if a.shuttingDown.Load() || a.HasOpenWindows() {
			return
		}
		platform.TrimWorkingSet()
	})
	a.trimMu.Unlock()
}

// Plugins returns a shallow copy of active plugins in registration order.
func (a *App) Plugins() []purekit.Plugin {
	return append([]purekit.Plugin(nil), a.snapshotPlugins()...)
}

// PluginByID returns the initialized plugin matching id, if active.
func (a *App) PluginByID(id string) purekit.Plugin {
	a.pluginMu.RLock()
	p := a.pluginByID[id]
	a.pluginMu.RUnlock()
	return p
}

// InstalledModules returns the install-time module flags clamped by compiled modules.
func (a *App) InstalledModules() core.ModuleConfig { return a.installed }

// ActiveModules returns the currently enabled module configuration.
func (a *App) ActiveModules() core.ModuleConfig {
	a.pluginMu.RLock()
	m := a.modules
	a.pluginMu.RUnlock()
	return m
}

// CompiledModules returns the compile-time compiled module flags.
func (a *App) CompiledModules() core.ModuleConfig { return a.compiled }

// ShouldStayInTray reports whether closing all windows keeps the host resident in tray.
func (a *App) ShouldStayInTray() bool {
	for _, p := range a.snapshotPlugins() {
		if p.ShouldStayInTray() {
			return true
		}
	}
	return false
}

// HasOpenWindows reports whether the host or any active plugin currently has an open window.
func (a *App) HasOpenWindows() bool {
	if a.settingsWin != nil || a.launcherWin != nil {
		return true
	}
	for _, p := range a.snapshotPlugins() {
		if p.HasOpenWindows() {
			return true
		}
	}
	return false
}

// Start boots active plugins, hotkeys, tray, and initial command-line launch action.
func (a *App) Start(args []string) {
	if a.log != nil {
		a.log.Infof("PureUtils 启动，数据目录 %s，模块 shot=%v clip=%v image=%v video=%v zip=%v monitor=%v",
			a.paths.Home,
			a.ModuleEnabled(core.ModuleCapture),
			a.ModuleEnabled(core.ModuleHistory),
			a.ModuleEnabled(core.ModuleViewer),
			a.ModuleEnabled(core.ModuleVideo),
			a.ModuleEnabled(core.ModuleZip),
			a.ModuleEnabled(core.ModuleMonitor),
		)
	}

	for _, p := range a.snapshotPlugins() {
		if err := p.Start(); err != nil && a.log != nil {
			a.log.Errorf("插件 %s 启动失败: %v", p.ID(), err)
		}
	}
	a.started.Store(true)

	req := core.ParseArgs(args)
	if !a.headless {
		if !isIsolatedTestInstance() {
			a.registerHotkeys()
		}
		if a.ShouldStayInTray() || req.Background {
			a.createTray()
		}
		if !isIsolatedTestInstance() {
			_ = a.applyStartupSetting(false)
			if a.updater != nil {
				go a.updater.CleanupOldDownloads()
				a.startUpdateScheduler()
			}
		}
	}

	switch {
	case a.routeLaunch(req):
	case req.HasCommand:
		a.ExecuteCommand(req.Command)
	case !req.Background && !a.ShouldStayInTray():
		switch {
		case a.ModuleEnabled(core.ModuleViewer):
			a.ExecuteCommand(core.CmdOpenViewer)
		case a.ModuleEnabled(core.ModuleVideo):
			a.ExecuteCommand(core.CmdOpenVideo)
		case a.ModuleEnabled(core.ModuleZip):
			a.ExecuteCommand(core.CmdOpenZip)
		case a.ModuleEnabled(core.ModuleMonitor):
			a.ExecuteCommand(core.CmdOpenMonitor)
		case len(a.snapshotPlugins()) == 0:
			a.ExecuteCommand(core.CmdShowSettings)
		}
	}
	if (req.Background || a.ShouldStayInTray()) && !a.HasOpenWindows() {
		a.scheduleIdleMemoryTrim()
	}
}

// HandleArgs dispatches a second-instance command line.
func (a *App) HandleArgs(args []string) {
	req := core.ParseArgs(args)
	if a.log != nil {
		a.log.Infof("收到外部启动命令: argc=%d cmd=%s", len(args), req.Command)
	}
	switch {
	case a.routeLaunch(req):
	case req.HasCommand:
		a.ExecuteCommand(req.Command)
	case a.ModuleEnabled(core.ModuleHistory):
		a.ExecuteCommand(core.CmdShowHistory)
	case a.ModuleEnabled(core.ModuleViewer):
		a.ExecuteCommand(core.CmdOpenViewer)
	case a.ModuleEnabled(core.ModuleVideo):
		a.ExecuteCommand(core.CmdOpenVideo)
	case a.ModuleEnabled(core.ModuleZip):
		a.ExecuteCommand(core.CmdOpenZip)
	case a.ModuleEnabled(core.ModuleMonitor):
		a.ExecuteCommand(core.CmdOpenMonitor)
	default:
		a.ExecuteCommand(core.CmdShowSettings)
	}
}

func (a *App) routeLaunch(req core.LaunchRequest) bool {
	if !req.HasCommand || !req.Command.TakesPaths() {
		return false
	}
	if req.Command == core.CmdOpenViewer && len(req.Paths) > 0 {
		if a.isArchiveByMatchers(req.Paths[0]) {
			a.pluginMu.RLock()
			_, hasZip := a.cliHandlers[core.CmdOpenZip]
			a.pluginMu.RUnlock()
			if hasZip {
				a.ExecuteCommand(core.CmdOpenZip, req.Paths...)
				return true
			}
		}
		if a.isVideoByMatchers(req.Paths[0]) {
			a.pluginMu.RLock()
			_, hasVideo := a.cliHandlers[core.CmdOpenVideo]
			a.pluginMu.RUnlock()
			if hasVideo {
				a.ExecuteCommand(core.CmdOpenVideo, req.Paths...)
				return true
			}
		}
	}
	a.pluginMu.RLock()
	spec, ok := a.cliHandlers[req.Command]
	a.pluginMu.RUnlock()
	if ok && spec.Execute != nil {
		a.ExecuteCommand(req.Command, req.Paths...)
		return true
	}
	if a.log != nil {
		a.log.Warnf("命令 %s 所属插件未启用或未编译", req.Command)
	}
	if !a.headless {
		a.Notify(
			i18n.S("功能组件未启用", "Module Disabled"),
			i18n.F("命令 %s 对应的组件当前未启用，可在「设置 → 高级」中开启。", "The module for command %s is currently disabled. Enable it in Settings → Advanced.", req.Command),
		)
	}
	return true
}

func (a *App) isVideoByMatchers(path string) bool {
	return a.matchFeatureByPath(path, "purevideo.open")
}

func (a *App) isArchiveByMatchers(path string) bool {
	return a.matchFeatureByPath(path, "purezip.open")
}

func (a *App) matchFeatureByPath(path, featureID string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	ctx := purekit.MatchContext{SelectedFiles: []string{path}}
	for _, p := range a.snapshotPlugins() {
		for _, m := range p.Matchers() {
			for _, res := range m.Match(ctx) {
				if res.FeatureID == featureID {
					return true
				}
			}
		}
	}
	return false
}

func (a *App) dispatch(cmd core.Command, paths []string) {
	if a.shuttingDown.Load() {
		return
	}
	if a.log != nil {
		a.log.Infof("执行命令: %s (paths=%d)", cmd, len(paths))
	}
	trace := core.BeginPerf()
	defer core.EndPerf("cmd."+string(cmd), trace)

	switch cmd {
	case core.CmdOpenLauncher:
		a.ToggleLauncher()
		return
	case core.CmdShowSettings:
		a.ShowSettings("")
		return
	case core.CmdCheckUpdate:
		a.ShowSettings("advanced")
		a.CheckForUpdates(true)
		return
	case core.CmdPerfReport:
		if report := core.PerfReport(); report != "" && a.log != nil {
			a.log.Infof("\n%s", report)
		}
		return
	case core.CmdExit:
		a.Quit()
		return
	}

	a.pluginMu.RLock()
	spec, ok := a.cliHandlers[cmd]
	a.pluginMu.RUnlock()
	if ok && spec.Execute != nil {
		spec.Execute(paths)
	}
}

func isIsolatedTestInstance() bool {
	return strings.TrimSpace(os.Getenv("PUREUTILS_INSTANCE_ID")) != ""
}

// IsElevated reports whether the host process currently runs with administrator privileges.
func (a *App) IsElevated() bool {
	if a.headless {
		return false
	}
	return platform.IsElevated()
}

// RestartAsAdmin prompts UAC elevation to relaunch PureUtils with administrator privileges.
func (a *App) RestartAsAdmin() error {
	if a.headless || platform.IsElevated() {
		return nil
	}
	args := fmt.Sprintf("--wait-pid %d --settings", os.Getpid())
	if err := platform.RelaunchElevated(args); err != nil {
		if a.log != nil {
			a.log.Warnf("以管理员身份重启失败: %v", err)
		}
		if errors.Is(err, platform.ErrElevationCanceled) {
			return errors.New(i18n.S("用户取消了管理员权限授权", "Administrator elevation was canceled"))
		}
		return err
	}
	if a.log != nil {
		a.log.Infof("正在以管理员身份重启 PureUtils")
	}
	a.Quit()
	return nil
}

func (a *App) applyStartupSetting(interactive bool) error {
	hc := a.HostConfig()
	args := ""
	if !a.ModuleEnabled(core.ModuleCapture) && !a.ModuleEnabled(core.ModuleHistory) &&
		(a.ModuleEnabled(core.ModuleViewer) || a.ModuleEnabled(core.ModuleVideo) || a.ModuleEnabled(core.ModuleZip) || a.ModuleEnabled(core.ModuleMonitor)) {
		args = "--background"
	}
	if hc.Advanced.RunOnStartup && hc.Advanced.RunElevatedOnStartup {
		if err := platform.SetElevatedRunOnStartupWithArgs(true, args, interactive); err != nil {
			if a.log != nil {
				a.log.Warnf("更新管理员开机自启失败: %v", err)
			}
			if errors.Is(err, platform.ErrElevationCanceled) {
				return errors.New(i18n.S("需要管理员权限配置计划任务（已取消）", "Administrator approval is required for elevated startup (canceled)"))
			}
			return err
		}
		return nil
	}
	var firstErr error
	if err := platform.SetElevatedRunOnStartupWithArgs(false, "", interactive); err != nil {
		if a.log != nil {
			a.log.Warnf("清理管理员开机自启计划任务失败: %v", err)
		}
		if errors.Is(err, platform.ErrElevationCanceled) {
			firstErr = errors.New(i18n.S("需要管理员权限移除计划任务（已取消）", "Administrator approval is required to remove the elevated task (canceled)"))
		} else {
			firstErr = err
		}
	}
	if err := platform.SetRunOnStartupWithArgs(hc.Advanced.RunOnStartup, args); err != nil {
		if a.log != nil {
			a.log.Warnf("更新开机自启失败: %v", err)
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Updater returns the host self-updater instance.
func (a *App) Updater() *Updater { return a.updater }

// CheckForUpdates starts an asynchronous release check.
func (a *App) CheckForUpdates(manual bool) {
	if a.updater == nil || a.shuttingDown.Load() {
		return
	}
	a.updater.StartCheck(a.HostConfig().Update, manual)
}

// DownloadUpdate starts downloading the discovered release using configured mirror settings.
func (a *App) DownloadUpdate() {
	if a.updater == nil || a.shuttingDown.Load() {
		return
	}
	a.updater.StartDownload(a.HostConfig().Update.DownloadSource)
}

// CancelUpdateDownload aborts an active update download.
func (a *App) CancelUpdateDownload() {
	if a.updater != nil {
		a.updater.CancelDownload()
	}
}

// InstallUpdate launches the downloaded installer and exits cleanly.
func (a *App) InstallUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("%s", i18n.S("更新器未初始化", "Updater is not initialized"))
	}
	return a.updater.InstallAndRestart()
}

// SkipUpdateVersion records ver so background notifications ignore it until a newer tag appears.
func (a *App) SkipUpdateVersion(ver string) error {
	hc := a.HostConfig()
	hc.Update.SkippedVersion = strings.TrimSpace(ver)
	return a.SaveHostConfig(hc)
}

func (a *App) startUpdateScheduler() {
	a.updateMu.Lock()
	if a.updateStop != nil {
		close(a.updateStop)
	}
	stop := make(chan struct{})
	a.updateStop = stop
	a.updateMu.Unlock()

	go func() {
		initTimer := time.NewTimer(10 * time.Second)
		defer initTimer.Stop()
		select {
		case <-stop:
			return
		case <-initTimer.C:
		}
		a.maybeRunAutoUpdateCheck()

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				a.maybeRunAutoUpdateCheck()
			}
		}
	}()
}

func (a *App) maybeRunAutoUpdateCheck() {
	if a.updater == nil || a.shuttingDown.Load() {
		return
	}
	uc := a.HostConfig().Update
	if !uc.AutoCheck {
		return
	}
	interval := time.Duration(uc.CheckIntervalHours) * time.Hour
	if a.updater.ShouldAutoCheck(interval, time.Now()) {
		a.updater.StartCheck(uc, false)
	}
}

func (a *App) stopUpdateScheduler() {
	a.updateMu.Lock()
	if a.updateStop != nil {
		close(a.updateStop)
		a.updateStop = nil
	}
	a.updateMu.Unlock()
	if a.updater != nil {
		a.updater.Stop()
	}
}

// Quit exits the application loop (or invokes test QuitFunc).
func (a *App) Quit() {
	if a.quitHook != nil {
		a.quitHook()
		return
	}
	if !a.headless {
		mygo.App.Quit()
	}
}

// Shutdown cleanly tears down shortcuts, plugins, clipboard, tray, and logger.
func (a *App) Shutdown() {
	if a.shuttingDown.Swap(true) {
		return
	}
	a.trimMu.Lock()
	if a.trimTimer != nil {
		a.trimTimer.Stop()
		a.trimTimer = nil
	}
	a.trimMu.Unlock()
	a.stopUpdateScheduler()
	if a.winTakeover != nil {
		a.winTakeover.Close()
		a.winTakeover = nil
	}
	if !a.headless {
		mygo.GlobalShortcut.UnregisterAll()
		if a.launcherWin != nil {
			a.launcherWin.Close()
		}
		if a.settingsWin != nil {
			a.settingsWin.Close()
		}
	}
	plugins := a.snapshotPlugins()
	for i := len(plugins) - 1; i >= 0; i-- {
		_ = plugins[i].Stop()
	}
	if a.clip != nil {
		a.clip.Close()
	}
	if a.tray != nil {
		a.tray.Destroy()
		a.tray = nil
	}
	if a.log != nil {
		if report := core.PerfReport(); report != "" {
			a.log.Infof("\n%s", report)
		}
		a.log.Infof("PureUtils 退出")
		a.log.Close()
	}
}
