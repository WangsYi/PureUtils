//go:build windows

package overlay

import (
	"os"
	"strings"
	"sync"

	"pureutils/pkg/purekit/config"
	"pureutils/plugins/puremonitor/collector"
)

// Callbacks connects native overlay actions back to the PureMonitor plugin.
type Callbacks struct {
	OnOpenDashboard func()
	OnOpenSettings  func()
	OnToggleTaskbar func()
	OnToggleFloat   func()
	OnFloatMoved    func(x, y int, collapsed, locked, through bool)
}

// Manager owns the taskbar, floating, and game HUD Win32 layered overlays.
type Manager struct {
	mu       sync.Mutex
	headless bool
	cfg      config.PureMonitorConfig
	cb       Callbacks

	taskbar  *taskbarOverlay
	floating *floatingOverlay
	gameHUD  *gameHUDOverlay
	stopped  bool
}

// NewManager creates an overlay manager. When headless is true or PUREUTILS_INSTANCE_ID
// is set (E2E isolation), real desktop overlays are disabled so the host taskbar stays untouched.
func NewManager(headless bool, cfg config.PureMonitorConfig, cb Callbacks) *Manager {
	isolated := strings.TrimSpace(os.Getenv("PUREUTILS_INSTANCE_ID")) != ""
	return &Manager{
		headless: headless || isolated,
		cfg:      cfg.Clone(),
		cb:       cb,
	}
}

// ApplyConfig reconciles active overlay windows with cfg.
func (m *Manager) ApplyConfig(cfg config.PureMonitorConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg.Clone()
	if m.headless || m.stopped {
		return
	}

	if cfg.Enabled && cfg.ShowTaskbar {
		if m.taskbar == nil {
			m.taskbar = newTaskbarOverlay(cfg, m.cb.OnOpenDashboard, m.cb.OnOpenSettings, m.cb.OnToggleFloat, m.cb.OnToggleTaskbar)
		} else {
			m.taskbar.UpdateConfig(cfg)
		}
	} else if m.taskbar != nil {
		m.taskbar.Stop()
		m.taskbar = nil
	}

	if cfg.Enabled && cfg.ShowFloating {
		if m.floating == nil {
			m.floating = newFloatingOverlay(cfg, m.cb.OnOpenDashboard, m.cb.OnOpenSettings, m.cb.OnToggleFloat, m.cb.OnFloatMoved)
		} else {
			m.floating.UpdateConfig(cfg)
		}
	} else if m.floating != nil {
		m.floating.Stop()
		m.floating = nil
	}

	if cfg.Enabled && cfg.GameFPSEnabled {
		if m.gameHUD == nil {
			m.gameHUD = newGameHUDOverlay(cfg)
		}
	} else if m.gameHUD != nil {
		m.gameHUD.Stop()
		m.gameHUD = nil
	}
}

// Update pushes a fresh telemetry snapshot and game HUD bounds to all active overlays.
func (m *Manager) Update(snap collector.SystemSnapshot, hudActive bool, monX, monY, monW, monH int) {
	m.mu.Lock()
	tb := m.taskbar
	flt := m.floating
	hud := m.gameHUD
	cfg := m.cfg
	headless := m.headless
	stopped := m.stopped
	m.mu.Unlock()

	if headless || stopped {
		return
	}
	if tb != nil {
		tb.Update(snap, cfg)
	}
	if flt != nil {
		flt.Update(snap, cfg)
	}
	if hud != nil {
		hud.Update(snap, cfg, hudActive, monX, monY, monW, monH)
	}
}

// Stop destroys all native overlay windows and exits their threads.
func (m *Manager) Stop() {
	m.mu.Lock()
	m.stopped = true
	tb := m.taskbar
	flt := m.floating
	hud := m.gameHUD
	m.taskbar = nil
	m.floating = nil
	m.gameHUD = nil
	m.mu.Unlock()

	if tb != nil {
		tb.Stop()
	}
	if flt != nil {
		flt.Stop()
	}
	if hud != nil {
		hud.Stop()
	}
}
