package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pureutils/pkg/purekit/core"
)

// PluginStorage exposes an isolated data directory, SQLite databases, content-
// addressed blobs, and read-only access to legacy data for one plugin.
type PluginStorage interface {
	PluginID() string
	DataDir() string
	EnsureDir() error
	Path(elem ...string) string
	HasDB(name string) bool
	OpenDB(name string, opts ...DBOption) (*DB, error)
	Blobs() (*BlobStore, error)
	BlobStore(subDir string) (*BlobStore, error)
	ProbeLegacyDB() (string, bool)
	ProbeLegacyDBs() []string
	OpenLegacyDB() (*sql.DB, string, error)
	OpenLegacyDBAt(path string) (*sql.DB, error)
	LegacyImagesDir() (string, bool)
	LegacyBlobs() (*BlobStore, bool, error)
}

// Manager isolates plugin storage under <Home>/plugins/<pluginID>.
type Manager struct {
	home string
}

// NewManager binds storage to home (defaulting to core.ResolvePaths().Home).
// It does not eagerly create legacy directories or database files.
func NewManager(home string) (*Manager, error) {
	if strings.TrimSpace(home) == "" {
		home = core.ResolvePaths().Home
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	return &Manager{home: abs}, nil
}

// Home returns the root PureUtils data directory.
func (m *Manager) Home() string {
	return m.home
}

// Plugin returns an isolated PluginStorage view for pluginID.
func (m *Manager) Plugin(pluginID string) PluginStorage {
	err := validatePluginID(pluginID)
	dataDir := ""
	if err == nil {
		dataDir = filepath.Join(m.home, "plugins", pluginID)
	}
	return &pluginStorage{
		mgr:      m,
		pluginID: pluginID,
		dataDir:  dataDir,
		initErr:  err,
	}
}

func validatePluginID(id string) error {
	if id == "" || strings.TrimSpace(id) != id {
		return fmt.Errorf("storage: invalid plugin id %q", id)
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\:`) {
		return fmt.Errorf("storage: invalid plugin id %q", id)
	}
	for _, r := range id {
		if r < 32 {
			return fmt.Errorf("storage: invalid plugin id %q", id)
		}
	}
	return nil
}

type pluginStorage struct {
	mgr      *Manager
	pluginID string
	dataDir  string
	initErr  error
}

func (p *pluginStorage) PluginID() string { return p.pluginID }
func (p *pluginStorage) DataDir() string  { return p.dataDir }

func (p *pluginStorage) EnsureDir() error {
	if p.initErr != nil {
		return p.initErr
	}
	return os.MkdirAll(p.dataDir, 0o755)
}

func (p *pluginStorage) Path(elem ...string) string {
	if p.initErr != nil {
		return ""
	}
	parts := append([]string{p.dataDir}, elem...)
	return filepath.Join(parts...)
}

func (p *pluginStorage) HasDB(name string) bool {
	if p.initErr != nil || name == "" {
		return false
	}
	info, err := os.Stat(p.Path(name))
	return err == nil && !info.IsDir()
}

func (p *pluginStorage) OpenDB(name string, opts ...DBOption) (*DB, error) {
	if p.initErr != nil {
		return nil, p.initErr
	}
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, `/\:`) || name == "." || name == ".." {
		return nil, fmt.Errorf("storage: invalid db name %q", name)
	}
	return OpenSQLite(p.Path(name), opts...)
}

func (p *pluginStorage) Blobs() (*BlobStore, error) {
	return p.BlobStore("blobs")
}

func (p *pluginStorage) BlobStore(subDir string) (*BlobStore, error) {
	if p.initErr != nil {
		return nil, p.initErr
	}
	if strings.TrimSpace(subDir) == "" || strings.ContainsAny(subDir, `/\:`) || subDir == "." || subDir == ".." {
		return nil, fmt.Errorf("storage: invalid blob subdir %q", subDir)
	}
	return NewBlobStore(p.Path(subDir))
}
