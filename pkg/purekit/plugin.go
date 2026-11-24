package purekit

import (
	"context"

	mygoui "github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/ui"
)

// Plugin is the core contract implemented by every PureUtils built-in or future
// external plugin.
type Plugin interface {
	ID() string
	Info() PluginInfo
	Init(host Host) error
	Start() error
	Stop() error
	Features() []Feature
	Matchers() []Matcher
	SettingsTab() *SettingsTab
	Hotkeys() []HotkeySpec
	TrayItems() []TrayItemSpec
	CLICommands() []CLICommandSpec
	HasOpenWindows() bool
	ShouldStayInTray() bool
	IsBusyForGlobalHooks() bool
}

// CapabilityDescriptor advertises an optional Capability Bus endpoint provided
// by a plugin.
type CapabilityDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PluginInfo describes static metadata about a plugin.
type PluginInfo struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Version      string                 `json:"version"`
	ModuleID     core.ModuleID          `json:"moduleId"`
	Capabilities []CapabilityDescriptor `json:"capabilities,omitempty"`
	IconPNG      []byte                 `json:"-"`
}

// ActionInput carries user query or contextual payloads into a Feature run.
type ActionInput struct {
	Query     string                 `json:"query,omitempty"`
	Text      string                 `json:"text,omitempty"`
	Files     []string               `json:"files,omitempty"`
	Args      []string               `json:"args,omitempty"`
	Source    string                 `json:"source,omitempty"`
	Clipboard *core.ClipboardContent `json:"-"`
}

// Action represents an executable plugin command or feature entry point.
type Action struct {
	ID    string
	Label string
	Run   func(ctx context.Context, in ActionInput) error
}

// Feature declares one searchable or triggerable action exposed by a plugin.
type Feature struct {
	ID       string                                      `json:"id"`
	PluginID string                                      `json:"pluginId"`
	Title    string                                      `json:"title"`
	Subtitle string                                      `json:"subtitle"`
	Keywords []string                                    `json:"keywords,omitempty"`
	Command  core.Command                                `json:"command,omitempty"`
	Shortcut string                                      `json:"shortcut,omitempty"`
	Badge    string                                      `json:"badge,omitempty"`
	Icon     *mygoui.SVG                                 `json:"-"`
	Run      func(ctx context.Context, in ActionInput) error `json:"-"`
}

// MatchContext is passed to plugin Matchers when the global launcher opens or
// the user types a query.
type MatchContext struct {
	Query         string
	Clipboard     *core.ClipboardContent
	SelectedFiles []string
	ActiveApp     string
}

// MatchResult represents one scored item returned by a Matcher for the launcher.
type MatchResult struct {
	FeatureID string
	PluginID  string
	Title     string
	Subtitle  string
	Score     int
	Badge     string
	Shortcut  string
	Icon      *mygoui.SVG
	Input     ActionInput
	Execute   func(ctx context.Context) error
}

// Matcher inspects MatchContext (clipboard, Explorer selection, query) and
// produces contextual launcher suggestions.
type Matcher interface {
	ID() string
	Title() string
	Match(ctx MatchContext) []MatchResult
}

// SettingsTab defines a plugin-provided page mounted in the unified Settings window.
type SettingsTab struct {
	ID       string
	Title    string
	Subtitle string
	Icon     *mygoui.SVG
	Order    int
	NewView  func(host Host) SettingsTabView
}

// SettingsTabView is a stateful editor instance bound to a Settings window session.
type SettingsTabView interface {
	Render(c *mygoui.Context, pal ui.Palette)
	Dirty() bool
	Validate() error
	Save() error
	ResetDefaults()
}

// HotkeySpec binds a global keyboard shortcut managed by the host.
type HotkeySpec struct {
	ID             string
	Command        core.Command
	Label          string
	DefaultGesture string
	CurrentGesture func() string
	SetGesture     func(string) error
	Callback       func()
}

// TrayItemKind identifies the visual type of a tray menu entry.
type TrayItemKind string

const (
	TrayItemAction    TrayItemKind = "action"
	TrayItemCheckbox  TrayItemKind = "checkbox"
	TrayItemSeparator TrayItemKind = "separator"
)

// TraySection specifies where in the tray menu an item belongs.
type TraySection string

const (
	TraySectionPrimary TraySection = "primary"
	TraySectionToggle  TraySection = "toggle"
)

// TrayItemSpec declares a menu entry contributed by a plugin to the host tray.
type TrayItemSpec struct {
	ID          string
	Section     TraySection
	Kind        TrayItemKind
	Label       string
	Accelerator string
	Checked     bool
	Disabled    bool
	Command     core.Command
	OnClick     func()
}

// CLICommandSpec binds CLI commands handled by a plugin.
type CLICommandSpec struct {
	Command    core.Command
	Aliases    []string
	TakesPaths bool
	Execute    func(paths []string)
}
