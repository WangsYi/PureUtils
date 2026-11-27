package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	PluginPureShot    = "pureshot"
	PluginPureClip    = "pureclip"
	PluginPureImage   = "pureimage"
	PluginPureZip     = "purezip"
	PluginPureVideo   = "purevideo"
	PluginPureMonitor = "puremonitor"

	CurrentSchemaVersion = 2
)

// Section provides scoped read/write access to one plugin's settings block
// inside settings.json, with automatic read-only fallback to legacy v1 keys.
type Section interface {
	ID() string
	Exists() bool
	Load(dst any) error
	Save(src any) error
	Raw() (json.RawMessage, bool)
	SaveRaw(raw json.RawMessage) error
	Delete() error
}

// Manager owns settings.json persistence, host configuration, and plugin-scoped
// sections. Read operations never create or modify files on disk.
type Manager struct {
	mu      sync.RWMutex
	path    string
	host    HostConfig
	plugins map[string]json.RawMessage
	rawTop  map[string]json.RawMessage
}

var reservedTopKeys = map[string]bool{
	"schemaVersion":   true,
	"hotkeys":         true,
	"appearance":      true,
	"advanced":        true,
	"launcher":        true,
	"disabledModules": true,
	"plugins":         true,
}

// Open reads settings.json from path. A missing file initializes in-memory
// defaults without creating anything on disk; corrupt JSON is backed up to
// path+".bad" without overwriting path.
func Open(path string) (*Manager, error) {
	m := newDefaultManager(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return m, err
	}
	if err := m.applyJSON(data); err != nil {
		_ = os.WriteFile(path+".bad", data, 0o600)
		return newDefaultManager(path), err
	}
	return m, nil
}

// Load opens path and ignores any load error, returning defaults on failure.
func Load(path string) *Manager {
	m, _ := Open(path)
	return m
}

func newDefaultManager(path string) *Manager {
	return &Manager{
		path:    path,
		host:    DefaultHostConfig(),
		plugins: map[string]json.RawMessage{},
		rawTop:  map[string]json.RawMessage{},
	}
}

func (m *Manager) applyJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("config: root must be a JSON object")
	}
	var rawTop map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &rawTop); err != nil {
		return err
	}
	if rawTop == nil {
		rawTop = map[string]json.RawMessage{}
	}

	host := DefaultHostConfig()
	if err := json.Unmarshal(trimmed, &host); err != nil {
		return err
	}
	host.Normalize()

	plugins := map[string]json.RawMessage{}
	if pRaw, ok := rawTop["plugins"]; ok && !isJSONNull(pRaw) {
		var pMap map[string]json.RawMessage
		if err := json.Unmarshal(pRaw, &pMap); err != nil {
			return err
		}
		for k, v := range pMap {
			if !isJSONNull(v) {
				plugins[k] = append(json.RawMessage(nil), v...)
			}
		}
	}

	m.host = host
	m.plugins = plugins
	m.rawTop = rawTop
	return nil
}

// Reload re-reads settings.json from disk without modifying disk files.
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			m.host = DefaultHostConfig()
			m.plugins = map[string]json.RawMessage{}
			m.rawTop = map[string]json.RawMessage{}
			return nil
		}
		return err
	}
	next := newDefaultManager(m.path)
	if err := next.applyJSON(data); err != nil {
		return err
	}
	m.host = next.host
	m.plugins = next.plugins
	m.rawTop = next.rawTop
	return nil
}

// Path returns the configured settings.json path.
func (m *Manager) Path() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.path
}

// Host returns a deep copy of the normalized host configuration.
func (m *Manager) Host() HostConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.host.Clone()
}

// SaveHost normalizes cfg, sets SchemaVersion to CurrentSchemaVersion, and
// atomically persists settings.json.
func (m *Manager) SaveHost(cfg HostConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := cfg.Clone()
	next.Normalize()
	next.SchemaVersion = CurrentSchemaVersion
	prev := m.host
	m.host = next
	if err := m.atomicSaveLocked(); err != nil {
		m.host = prev
		return err
	}
	return nil
}

// Section returns a scoped Section handle for pluginID.
func (m *Manager) Section(pluginID string) Section {
	return &sectionHandle{mgr: m, id: pluginID}
}

func (m *Manager) rawSection(pluginID string) (json.RawMessage, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if raw, ok := m.plugins[pluginID]; ok && !isJSONNull(raw) {
		return append(json.RawMessage(nil), raw...), true
	}
	return synthesizeLegacySection(pluginID, m.rawTop)
}

func (m *Manager) saveSectionRaw(pluginID string, raw json.RawMessage) error {
	if pluginID == "" {
		return fmt.Errorf("config: empty plugin id")
	}
	if !json.Valid(raw) {
		return fmt.Errorf("config: invalid JSON for plugin %q", pluginID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.plugins == nil {
		m.plugins = map[string]json.RawMessage{}
	}
	prev, hadPrev := m.plugins[pluginID]
	m.plugins[pluginID] = append(json.RawMessage(nil), raw...)
	if m.host.SchemaVersion < CurrentSchemaVersion {
		m.host.SchemaVersion = CurrentSchemaVersion
	}
	if err := m.atomicSaveLocked(); err != nil {
		if hadPrev {
			m.plugins[pluginID] = prev
		} else {
			delete(m.plugins, pluginID)
		}
		return err
	}
	return nil
}

func (m *Manager) deleteSection(pluginID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev, hadPrev := m.plugins[pluginID]
	if !hadPrev {
		return nil
	}
	delete(m.plugins, pluginID)
	if err := m.atomicSaveLocked(); err != nil {
		m.plugins[pluginID] = prev
		return err
	}
	return nil
}

func (m *Manager) atomicSaveLocked() error {
	if m.path == "" {
		return fmt.Errorf("config: empty settings path")
	}
	out := map[string]json.RawMessage{}
	for k, v := range m.rawTop {
		if !reservedTopKeys[k] {
			out[k] = append(json.RawMessage(nil), v...)
		}
	}

	mustMarshal := func(v any) (json.RawMessage, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(b), nil
	}

	sv, err := mustMarshal(CurrentSchemaVersion)
	if err != nil {
		return err
	}
	out["schemaVersion"] = sv

	hk, err := mustMarshal(m.host.Hotkeys)
	if err != nil {
		return err
	}
	out["hotkeys"] = hk

	ap, err := mustMarshal(m.host.Appearance)
	if err != nil {
		return err
	}
	out["appearance"] = ap

	ad, err := mustMarshal(m.host.Advanced)
	if err != nil {
		return err
	}
	out["advanced"] = ad

	ln, err := mustMarshal(m.host.Launcher)
	if err != nil {
		return err
	}
	out["launcher"] = ln

	if len(m.host.DisabledModules) > 0 {
		dm, err := mustMarshal(m.host.DisabledModules)
		if err != nil {
			return err
		}
		out["disabledModules"] = dm
	}

	if len(m.plugins) > 0 {
		pluginsCopy := make(map[string]json.RawMessage, len(m.plugins))
		maps.Copy(pluginsCopy, m.plugins)
		pl, err := mustMarshal(pluginsCopy)
		if err != nil {
			return err
		}
		out["plugins"] = pl
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	var renameErr error
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 10 * time.Millisecond)
		}
		renameErr = os.Rename(tmp, m.path)
		if renameErr == nil {
			break
		}
	}
	if renameErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("config: replace %s: %w", m.path, renameErr)
	}
	m.rawTop = out
	return nil
}

type sectionHandle struct {
	mgr *Manager
	id  string
}

func (s *sectionHandle) ID() string { return s.id }

func (s *sectionHandle) Exists() bool {
	_, ok := s.mgr.rawSection(s.id)
	return ok
}

func (s *sectionHandle) Raw() (json.RawMessage, bool) {
	return s.mgr.rawSection(s.id)
}

func (s *sectionHandle) Load(dst any) error {
	if dst == nil {
		return fmt.Errorf("config: nil destination")
	}
	raw, ok := s.mgr.rawSection(s.id)
	if !ok {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func (s *sectionHandle) Save(src any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return s.mgr.saveSectionRaw(s.id, json.RawMessage(b))
}

func (s *sectionHandle) SaveRaw(raw json.RawMessage) error {
	return s.mgr.saveSectionRaw(s.id, raw)
}

func (s *sectionHandle) Delete() error {
	return s.mgr.deleteSection(s.id)
}

func isJSONNull(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) == 0 || bytes.Equal(t, []byte("null"))
}
