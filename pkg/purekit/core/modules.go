package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ModuleID identifies one optional functional module of PureUtils.
type ModuleID string

const (
	ModuleCapture ModuleID = "capture"
	ModuleHistory ModuleID = "history"
	ModuleViewer  ModuleID = "viewer"
	ModuleZip     ModuleID = "zip"
	ModuleVideo   ModuleID = "video"
	ModuleMonitor ModuleID = "monitor"
)

// ModuleConfig describes which optional modules are enabled for the
// installation. It is persisted in modules.json beside the executable when
// installed with selective components.
type ModuleConfig struct {
	Capture bool `json:"capture"`
	History bool `json:"history"`
	Viewer  bool `json:"viewer"`
	Zip     bool `json:"zip"`
	Video   bool `json:"video"`
	Monitor bool `json:"monitor"`
}

// DefaultModules returns the default enabled modules for the current build.
func DefaultModules(viewerCompiled bool) ModuleConfig {
	return ModuleConfig{
		Capture: true,
		History: true,
		Viewer:  viewerCompiled,
		Zip:     true,
		Video:   true,
		Monitor: true,
	}
}

// Enabled reports whether id is enabled in the module configuration.
func (m ModuleConfig) Enabled(id ModuleID) bool {
	switch id {
	case ModuleCapture:
		return m.Capture
	case ModuleHistory:
		return m.History
	case ModuleViewer:
		return m.Viewer
	case ModuleZip:
		return m.Zip
	case ModuleVideo:
		return m.Video
	case ModuleMonitor:
		return m.Monitor
	default:
		return false
	}
}

// Any reports whether at least one module is enabled.
func (m ModuleConfig) Any() bool {
	return m.Capture || m.History || m.Viewer || m.Zip || m.Video || m.Monitor
}

// Normalize enforces compile-time constraints and falls back to defaults if a
// corrupt manifest disables every module.
func (m *ModuleConfig) Normalize(viewerCompiled bool) {
	if !viewerCompiled {
		m.Viewer = false
	}
	if !m.Any() {
		*m = DefaultModules(viewerCompiled)
	}
}

// ModulesManifestPath returns the expected modules.json path inside exeDir.
func ModulesManifestPath(exeDir string) string {
	return filepath.Join(exeDir, "modules.json")
}

// LoadModuleConfig reads modules.json from exeDir. Missing or malformed files
// fall back to DefaultModules(viewerCompiled) so portable and `go run` sessions
// enable all compiled modules by default.
func LoadModuleConfig(exeDir string, viewerCompiled bool) ModuleConfig {
	if exeDir == "" {
		return DefaultModules(viewerCompiled)
	}
	data, err := os.ReadFile(ModulesManifestPath(exeDir))
	if err != nil {
		return DefaultModules(viewerCompiled)
	}
	cfg := DefaultModules(viewerCompiled)
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultModules(viewerCompiled)
	}
	cfg.Normalize(viewerCompiled)
	return cfg
}

// SaveModuleConfig writes a module manifest atomically to path.
func SaveModuleConfig(path string, cfg ModuleConfig) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("modules: replace %s: %w", path, err)
	}
	return nil
}
