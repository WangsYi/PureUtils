package purekit

import (
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/storage"
)

// Host defines the runtime API exposed by PureUtils to every plugin.
type Host interface {
	Config(pluginID string) config.Section
	HostConfig() config.HostConfig
	SaveHostConfig(cfg config.HostConfig) error
	Storage(pluginID string) storage.PluginStorage
	Clipboard() clipboard.Service
	Bus() *Bus
	Log() *core.Logger
	Notify(title, body string)
	Headless() bool
	Paths() core.Paths
	ModuleEnabled(id core.ModuleID) bool
	ReloadHotkeys()
	RebuildTray()
	OpenSettings(tabID string)
	ExecuteCommand(cmd core.Command, paths ...string)
	HotkeyError(cmd core.Command) string
	SetGlobalHookSuppressed(source string, suppressed bool)
	IsGlobalHookSuppressed() bool
	NotifyWindowClosed()
}
