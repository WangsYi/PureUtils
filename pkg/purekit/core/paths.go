package core

import (
	"os"
	"path/filepath"
)

// Paths are the application's writable directories.
type Paths struct {
	Home     string
	DB       string
	Settings string
	Images   string
	Logs     string
}

// ResolvePaths finds the data directory. PUREUTILS_HOME (or legacy YNM3000_HOME / SHOTCLIP_HOME)
// overrides it for tests and portable installs; otherwise it is %LOCALAPPDATA%\PureUtils.
func ResolvePaths() Paths {
	home := os.Getenv("PUREUTILS_HOME")
	if home == "" {
		home = os.Getenv("YNM3000_HOME")
	}
	if home == "" {
		home = os.Getenv("SHOTCLIP_HOME")
	}
	if home == "" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base, _ = os.UserConfigDir()
		}
		home = filepath.Join(base, "PureUtils")
	}
	home, _ = filepath.Abs(home)
	return Paths{
		Home:     home,
		DB:       filepath.Join(home, "pureutils.db"),
		Settings: filepath.Join(home, "settings.json"),
		Images:   filepath.Join(home, "images"),
		Logs:     filepath.Join(home, "logs"),
	}
}

// Ensure creates the base writable directories.
func (p Paths) Ensure() error {
	for _, dir := range []string{p.Home, p.Images, p.Logs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// PluginDir returns the isolated directory for plugin id under Home/plugins/<id>.
func (p Paths) PluginDir(id string) string {
	return filepath.Join(p.Home, "plugins", id)
}
