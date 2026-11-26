package config

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
)

const (
	CaptureActionCopy        = "copy"
	CaptureActionSave        = "save"
	CaptureActionCopyAndSave = "copy_and_save"
	CaptureActionPin         = "pin"
	MaxRecentArchives        = 12
)

// HotkeyConfig holds global shortcuts configured by the user.
type HotkeyConfig struct {
	CaptureRegion   string            `json:"captureRegion"`
	CaptureMonitor  string            `json:"captureMonitor"`
	PinClipboard    string            `json:"pinClipboard"`
	ShowHistory     string            `json:"showHistory"`
	TogglePins      string            `json:"togglePins"`
	OpenViewer      string            `json:"openViewer,omitempty"`
	OpenVideo       string            `json:"openVideo,omitempty"`
	OpenZip         string            `json:"openZip,omitempty"`
	Launcher        string            `json:"launcher,omitempty"`
	TakeoverWinKeys bool              `json:"takeoverWinKeys"`
	Plugins         map[string]string `json:"plugins,omitempty"`
}

// AppearanceConfig holds host-wide theme, language, and preview layout preferences.
type AppearanceConfig struct {
	Theme         string  `json:"theme"`
	Language      string  `json:"language,omitempty"` // "auto" | "zh-CN" | "en-US"
	PreviewHeight float64 `json:"previewHeight"`
}

// AdvancedConfig holds global timing, startup, and logging controls.
type AdvancedConfig struct {
	SelfWriteSuppressMs int  `json:"selfWriteSuppressMs"`
	DebounceMs          int  `json:"debounceMs"`
	RunOnStartup        bool `json:"runOnStartup"`
	VerboseLogging      bool `json:"verboseLogging"`
}

// LauncherConfig controls the global Alt+Space plugin launcher.
type LauncherConfig struct {
	Enabled    bool   `json:"enabled"`
	Shortcut   string `json:"shortcut"`
	MaxResults int    `json:"maxResults"`
}

// UpdateConfig controls automatic update checking and download mirrors.
type UpdateConfig struct {
	AutoCheck          bool   `json:"autoCheck"`
	AutoDownload       bool   `json:"autoDownload"`
	CheckIntervalHours int    `json:"checkIntervalHours"`
	DownloadSource     string `json:"downloadSource"` // "auto" | "ghproxy" | "ghfast" | "github"
	SkippedVersion     string `json:"skippedVersion,omitempty"`
}

// HostConfig holds host-level configuration persisted outside plugins.<id>.
type HostConfig struct {
	SchemaVersion   int              `json:"schemaVersion"`
	Hotkeys         HotkeyConfig     `json:"hotkeys"`
	Appearance      AppearanceConfig `json:"appearance"`
	Advanced        AdvancedConfig   `json:"advanced"`
	Launcher        LauncherConfig   `json:"launcher"`
	Update          UpdateConfig     `json:"update"`
	DisabledModules []core.ModuleID  `json:"disabledModules,omitempty"`
}

// DefaultHostConfig returns default host configuration.
func DefaultHostConfig() HostConfig {
	return HostConfig{
		SchemaVersion: CurrentSchemaVersion,
		Hotkeys: HotkeyConfig{
			CaptureRegion:   "F1",
			CaptureMonitor:  "Ctrl+F1",
			PinClipboard:    "F3",
			ShowHistory:     "Ctrl+Shift+V",
			TogglePins:      "Ctrl+Alt+X",
			Launcher:        "Alt+Space",
			TakeoverWinKeys: true,
		},
		Appearance: AppearanceConfig{
			Theme:         "system",
			Language:      i18n.PrefAuto,
			PreviewHeight: 96,
		},
		Advanced: AdvancedConfig{
			SelfWriteSuppressMs: 400,
			DebounceMs:          160,
		},
		Launcher: LauncherConfig{
			Enabled:    true,
			Shortcut:   "Alt+Space",
			MaxResults: 12,
		},
		Update: UpdateConfig{
			AutoCheck:          true,
			AutoDownload:       false,
			CheckIntervalHours: 24,
			DownloadSource:     "auto",
		},
	}
}

// Normalize clamps host fields and fills missing defaults.
func (h *HostConfig) Normalize() {
	d := DefaultHostConfig()
	if h.SchemaVersion <= 0 {
		h.SchemaVersion = CurrentSchemaVersion
	}
	if h.Hotkeys.CaptureRegion == "" {
		h.Hotkeys.CaptureRegion = d.Hotkeys.CaptureRegion
	}
	if h.Hotkeys.CaptureMonitor == "" {
		h.Hotkeys.CaptureMonitor = d.Hotkeys.CaptureMonitor
	}
	if h.Hotkeys.PinClipboard == "" {
		h.Hotkeys.PinClipboard = d.Hotkeys.PinClipboard
	}
	if h.Hotkeys.ShowHistory == "" {
		h.Hotkeys.ShowHistory = d.Hotkeys.ShowHistory
	}
	if h.Hotkeys.TogglePins == "" {
		h.Hotkeys.TogglePins = d.Hotkeys.TogglePins
	}
	if h.Hotkeys.Launcher == "" && h.Launcher.Shortcut != "" {
		h.Hotkeys.Launcher = h.Launcher.Shortcut
	}
	if h.Launcher.Shortcut == "" && h.Hotkeys.Launcher != "" {
		h.Launcher.Shortcut = h.Hotkeys.Launcher
	}
	if h.Hotkeys.Launcher == "" {
		h.Hotkeys.Launcher = d.Hotkeys.Launcher
	}
	if h.Launcher.Shortcut == "" {
		h.Launcher.Shortcut = d.Launcher.Shortcut
	}
	if h.Launcher.MaxResults <= 0 {
		h.Launcher.MaxResults = d.Launcher.MaxResults
	}
	if h.Launcher.MaxResults > 50 {
		h.Launcher.MaxResults = 50
	}
	if h.Appearance.Theme == "" {
		h.Appearance.Theme = d.Appearance.Theme
	}
	h.Appearance.Language = i18n.NormalizePref(h.Appearance.Language)
	if h.Appearance.PreviewHeight < 32 {
		h.Appearance.PreviewHeight = 32
	}
	if h.Appearance.PreviewHeight > 400 {
		h.Appearance.PreviewHeight = 400
	}
	if h.Advanced.SelfWriteSuppressMs < 0 {
		h.Advanced.SelfWriteSuppressMs = 0
	}
	if h.Advanced.SelfWriteSuppressMs > 5000 {
		h.Advanced.SelfWriteSuppressMs = 5000
	}
	if h.Advanced.DebounceMs < 0 {
		h.Advanced.DebounceMs = 0
	}
	if h.Advanced.DebounceMs > 2000 {
		h.Advanced.DebounceMs = 2000
	}
	if h.Update.CheckIntervalHours <= 0 {
		h.Update.CheckIntervalHours = d.Update.CheckIntervalHours
	}
	if h.Update.CheckIntervalHours < 1 {
		h.Update.CheckIntervalHours = 1
	}
	if h.Update.CheckIntervalHours > 168 {
		h.Update.CheckIntervalHours = 168
	}
	switch strings.ToLower(strings.TrimSpace(h.Update.DownloadSource)) {
	case "ghproxy", "ghfast", "github", "auto":
		h.Update.DownloadSource = strings.ToLower(strings.TrimSpace(h.Update.DownloadSource))
	default:
		h.Update.DownloadSource = d.Update.DownloadSource
	}
	h.Update.SkippedVersion = strings.TrimSpace(h.Update.SkippedVersion)
	h.DisabledModules = normalizeDisabledModules(h.DisabledModules)
}

// IsModuleDisabled reports whether id has been disabled in settings.json.
func (h HostConfig) IsModuleDisabled(id core.ModuleID) bool {
	target := core.ModuleID(strings.ToLower(strings.TrimSpace(string(id))))
	if target == "" {
		return false
	}
	for _, item := range h.DisabledModules {
		if core.ModuleID(strings.ToLower(strings.TrimSpace(string(item)))) == target {
			return true
		}
	}
	return false
}

// SetModuleDisabled adds or removes id from DisabledModules in canonical order.
func (h *HostConfig) SetModuleDisabled(id core.ModuleID, disabled bool) {
	target := core.ModuleID(strings.ToLower(strings.TrimSpace(string(id))))
	if target == "" {
		return
	}
	filtered := make([]core.ModuleID, 0, len(h.DisabledModules)+1)
	for _, item := range h.DisabledModules {
		norm := core.ModuleID(strings.ToLower(strings.TrimSpace(string(item))))
		if norm != "" && norm != target {
			filtered = append(filtered, norm)
		}
	}
	if disabled {
		filtered = append(filtered, target)
	}
	h.DisabledModules = normalizeDisabledModules(filtered)
}

func normalizeDisabledModules(list []core.ModuleID) []core.ModuleID {
	if len(list) == 0 {
		return nil
	}
	seen := make(map[core.ModuleID]bool, len(list))
	out := make([]core.ModuleID, 0, len(list))
	for _, raw := range list {
		id := core.ModuleID(strings.ToLower(strings.TrimSpace(string(raw))))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}

// Clone returns a deep copy of HostConfig.
func (h HostConfig) Clone() HostConfig {
	out := h
	if h.Hotkeys.Plugins != nil {
		out.Hotkeys.Plugins = make(map[string]string, len(h.Hotkeys.Plugins))
		maps.Copy(out.Hotkeys.Plugins, h.Hotkeys.Plugins)
	}
	if h.DisabledModules != nil {
		out.DisabledModules = append([]core.ModuleID(nil), h.DisabledModules...)
	}
	return out
}

// ThemeSource returns "light", "dark", or "system".
func (h HostConfig) ThemeSource() string {
	switch strings.ToLower(h.Appearance.Theme) {
	case "light":
		return "light"
	case "dark":
		return "dark"
	default:
		return "system"
	}
}

// CaptureConfig controls PureShot screen capture.
type CaptureConfig struct {
	DefaultAction   string `json:"defaultAction,omitempty"`
	CopyToClipboard bool   `json:"copyToClipboard"`
	AutoSave        bool   `json:"autoSave"`
	SaveFolder      string `json:"saveFolder"`
	FileNamePattern string `json:"fileNamePattern"`
	IncludeCursor   bool   `json:"includeCursor"`
	SnapToWindows   bool   `json:"snapToWindows"`
	ShowMagnifier   bool   `json:"showMagnifier"`
	ShowRuler       bool   `json:"showRuler"`
}

// PinConfig controls PureShot floating pin windows.
type PinConfig struct {
	AlwaysOnTop     bool      `json:"alwaysOnTop"`
	Opacity         float64   `json:"opacity"`
	BorderThickness int       `json:"borderThickness"`
	BorderColor     core.ARGB `json:"borderColorArgb"`
}

// PureShotConfig is the plugin-scoped configuration for PureShot (`plugins.pureshot`).
type PureShotConfig struct {
	Capture CaptureConfig `json:"capture"`
	Pins    PinConfig     `json:"pins"`
}

// DefaultSaveFolder returns Pictures\PureUtils or "shots".
func DefaultSaveFolder() string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, "Pictures", "PureUtils")
	}
	return "shots"
}

// DefaultPureShotConfig returns default settings for PureShot.
func DefaultPureShotConfig() PureShotConfig {
	return PureShotConfig{
		Capture: CaptureConfig{
			DefaultAction:   CaptureActionCopy,
			CopyToClipboard: true,
			SaveFolder:      DefaultSaveFolder(),
			FileNamePattern: core.DefaultCaptureFileName,
			SnapToWindows:   true,
			ShowMagnifier:   true,
			ShowRuler:       true,
		},
		Pins: PinConfig{
			AlwaysOnTop:     true,
			Opacity:         1,
			BorderThickness: core.DefaultPinBorderWidth,
			BorderColor:     core.DefaultPinBorderColor,
		},
	}
}

// Normalize enforces valid ranges and fills missing defaults.
func (c *PureShotConfig) Normalize() {
	d := DefaultPureShotConfig()
	switch c.Capture.DefaultAction {
	case CaptureActionCopy, CaptureActionSave, CaptureActionCopyAndSave, CaptureActionPin:
	default:
		switch {
		case c.Capture.AutoSave && c.Capture.CopyToClipboard:
			c.Capture.DefaultAction = CaptureActionCopyAndSave
		case c.Capture.AutoSave:
			c.Capture.DefaultAction = CaptureActionSave
		default:
			c.Capture.DefaultAction = CaptureActionCopy
		}
	}
	if c.Capture.SaveFolder == "" {
		c.Capture.SaveFolder = d.Capture.SaveFolder
	}
	if c.Capture.FileNamePattern == "" {
		c.Capture.FileNamePattern = d.Capture.FileNamePattern
	}
	if c.Pins.Opacity <= 0 {
		c.Pins.Opacity = d.Pins.Opacity
	}
	if c.Pins.Opacity > 1 {
		c.Pins.Opacity = 1
	}
	if c.Pins.BorderThickness < 0 {
		c.Pins.BorderThickness = 0
	}
	if c.Pins.BorderColor == 0 {
		c.Pins.BorderColor = d.Pins.BorderColor
	}
}

// HistoryConfig controls PureClip clipboard history capture and retention.
type HistoryConfig struct {
	Enabled          bool     `json:"enabled"`
	MaxEntries       int      `json:"maxEntries"`
	MaxAgeDays       int      `json:"maxAgeDays"`
	MaxInlineChars   int      `json:"maxInlineChars"`
	CaptureImages    bool     `json:"captureImages"`
	CaptureFiles     bool     `json:"captureFiles"`
	IgnoredProcesses []string `json:"ignoredProcesses"`
	ListLimit        int      `json:"listLimit"`
	PasteOnEnter     bool     `json:"pasteOnEnter"`
}

// PureClipConfig is the plugin-scoped configuration for PureClip (`plugins.pureclip`).
type PureClipConfig struct {
	History             HistoryConfig `json:"history"`
	PreviewHeight       float64       `json:"previewHeight"`
	SelfWriteSuppressMs int           `json:"selfWriteSuppressMs"`
	DebounceMs          int           `json:"debounceMs"`
}

// DefaultPureClipConfig returns default settings for PureClip.
func DefaultPureClipConfig() PureClipConfig {
	return PureClipConfig{
		History: HistoryConfig{
			Enabled:          true,
			MaxEntries:       500,
			MaxAgeDays:       30,
			MaxInlineChars:   65536,
			CaptureImages:    true,
			CaptureFiles:     true,
			IgnoredProcesses: []string{"keepass", "1password", "bitwarden"},
			ListLimit:        200,
			PasteOnEnter:     true,
		},
		PreviewHeight:       96,
		SelfWriteSuppressMs: 400,
		DebounceMs:          160,
	}
}

// Normalize enforces valid bounds on PureClipConfig.
func (c *PureClipConfig) Normalize() {
	d := DefaultPureClipConfig()
	if c.History.MaxEntries <= 0 {
		c.History.MaxEntries = d.History.MaxEntries
	}
	if c.History.MaxEntries > 100000 {
		c.History.MaxEntries = 100000
	}
	if c.History.MaxAgeDays < 0 {
		c.History.MaxAgeDays = 0
	}
	if c.History.MaxAgeDays > 3650 {
		c.History.MaxAgeDays = 3650
	}
	if c.History.MaxInlineChars <= 0 {
		c.History.MaxInlineChars = d.History.MaxInlineChars
	}
	if c.History.ListLimit <= 0 {
		c.History.ListLimit = d.History.ListLimit
	}
	if c.History.ListLimit > 2000 {
		c.History.ListLimit = 2000
	}
	if c.History.IgnoredProcesses == nil {
		c.History.IgnoredProcesses = append([]string(nil), d.History.IgnoredProcesses...)
	}
	if c.PreviewHeight < 32 {
		c.PreviewHeight = 32
	}
	if c.PreviewHeight > 400 {
		c.PreviewHeight = 400
	}
	if c.SelfWriteSuppressMs < 0 {
		c.SelfWriteSuppressMs = 0
	}
	if c.SelfWriteSuppressMs > 5000 {
		c.SelfWriteSuppressMs = 5000
	}
	if c.DebounceMs < 0 {
		c.DebounceMs = 0
	}
	if c.DebounceMs > 2000 {
		c.DebounceMs = 2000
	}
}

// PureImageConfig is the plugin-scoped configuration for PureImage (`plugins.pureimage`).
type PureImageConfig struct {
	Recursive        bool   `json:"recursive"`
	WheelZoom        bool   `json:"wheelZoom"`
	Checkerboard     bool   `json:"checkerboard"`
	ShowDetails      bool   `json:"showDetails"`
	SpacePreview     bool   `json:"spacePreview"`
	SlideshowSeconds int    `json:"slideshowSeconds"`
	PreloadCount     int    `json:"preloadCount"`
	Sort             string `json:"sort"`
	SortDesc         bool   `json:"sortDesc"`
	Language         string `json:"language"`
}

// DefaultPureImageConfig returns default settings for PureImage.
func DefaultPureImageConfig() PureImageConfig {
	return PureImageConfig{
		WheelZoom:        true,
		Checkerboard:     true,
		SlideshowSeconds: 4,
		Sort:             "name",
		Language:         "zh-CN",
	}
}

// Normalize enforces valid bounds on PureImageConfig.
func (c *PureImageConfig) Normalize() {
	d := DefaultPureImageConfig()
	if c.SlideshowSeconds < 1 || c.SlideshowSeconds > 60 {
		c.SlideshowSeconds = d.SlideshowSeconds
	}
	if c.PreloadCount < 0 || c.PreloadCount > 3 {
		c.PreloadCount = d.PreloadCount
	}
	switch c.Sort {
	case "name", "modified", "size", "random":
	default:
		c.Sort = d.Sort
	}
	switch c.Language {
	case "zh-CN", "en-US", "system":
	default:
		c.Language = d.Language
	}
}

// PureZipConfig is the plugin-scoped configuration for PureZip (`plugins.purezip`).
type PureZipConfig struct {
	DefaultFormat          string   `json:"defaultFormat"`
	Level                  int      `json:"level"`
	Encryption             string   `json:"encryption"`
	OverwriteMode          string   `json:"overwriteMode"`
	NameEncoding           string   `json:"nameEncoding"`
	OpenFolderAfterExtract bool     `json:"openFolderAfterExtract"`
	CloseAfterExtract      bool     `json:"closeAfterExtract"`
	ImagePreview           bool     `json:"imagePreview"`
	RecentArchives         []string `json:"recentArchives"`
}

// DefaultPureZipConfig returns default settings for PureZip.
func DefaultPureZipConfig() PureZipConfig {
	return PureZipConfig{
		DefaultFormat:          "zip",
		Level:                  6,
		Encryption:             "aes256",
		OverwriteMode:          "ask",
		NameEncoding:           "auto",
		OpenFolderAfterExtract: true,
		ImagePreview:           true,
		RecentArchives:         []string{},
	}
}

// AddRecentArchive moves path to the front of RecentArchives, bounded by MaxRecentArchives.
func (c *PureZipConfig) AddRecentArchive(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	out := []string{path}
	for _, p := range c.RecentArchives {
		if !strings.EqualFold(p, path) && len(out) < MaxRecentArchives {
			out = append(out, p)
		}
	}
	c.RecentArchives = out
}

// Normalize enforces valid bounds on PureZipConfig.
func (c *PureZipConfig) Normalize() {
	d := DefaultPureZipConfig()
	switch c.DefaultFormat {
	case "zip", "7z", "tar", "tar.gz", "tar.xz", "tar.zst", "tar.bz2":
	default:
		c.DefaultFormat = d.DefaultFormat
	}
	if c.Level < 0 || c.Level > 9 {
		c.Level = d.Level
	}
	switch c.Encryption {
	case "aes256", "zipcrypto":
	default:
		c.Encryption = d.Encryption
	}
	switch c.OverwriteMode {
	case "ask", "overwrite", "skip", "rename", "newer":
	default:
		c.OverwriteMode = d.OverwriteMode
	}
	switch c.NameEncoding {
	case "auto", "UTF-8", "GBK", "BIG5", "SHIFT_JIS", "CP437":
	default:
		c.NameEncoding = d.NameEncoding
	}
	if c.RecentArchives == nil {
		c.RecentArchives = []string{}
	}
	if len(c.RecentArchives) > MaxRecentArchives {
		c.RecentArchives = c.RecentArchives[:MaxRecentArchives]
	}
}

func LoadPureShot(sec Section) PureShotConfig {
	cfg := DefaultPureShotConfig()
	if sec != nil {
		_ = sec.Load(&cfg)
	}
	cfg.Normalize()
	return cfg
}

func SavePureShot(sec Section, cfg PureShotConfig) error {
	cfg.Normalize()
	return sec.Save(cfg)
}

func LoadPureClip(sec Section) PureClipConfig {
	cfg := DefaultPureClipConfig()
	if sec != nil {
		_ = sec.Load(&cfg)
	}
	cfg.Normalize()
	return cfg
}

func SavePureClip(sec Section, cfg PureClipConfig) error {
	cfg.Normalize()
	return sec.Save(cfg)
}

func LoadPureImage(sec Section) PureImageConfig {
	cfg := DefaultPureImageConfig()
	if sec != nil {
		_ = sec.Load(&cfg)
	}
	cfg.Normalize()
	return cfg
}

func SavePureImage(sec Section, cfg PureImageConfig) error {
	cfg.Normalize()
	return sec.Save(cfg)
}

func LoadPureZip(sec Section) PureZipConfig {
	cfg := DefaultPureZipConfig()
	if sec != nil {
		_ = sec.Load(&cfg)
	}
	cfg.Normalize()
	return cfg
}

func SavePureZip(sec Section, cfg PureZipConfig) error {
	cfg.Normalize()
	return sec.Save(cfg)
}

// PureVideoConfig is the plugin-scoped configuration for PureVideo (`plugins.purevideo`).
type PureVideoConfig struct {
	Engine          string  `json:"engine"`          // "mpv" (default and sole backend)
	DownloadSource  string  `json:"downloadSource"`  // "auto" | "ghproxy" | "ghfast" | "sourceforge" | "github"
	CustomMirrorURL string  `json:"customMirrorUrl"`
	Volume          int     `json:"volume"`          // 0..100
	Muted           bool    `json:"muted"`
	Loop            bool    `json:"loop"`
	AutoPlay        bool    `json:"autoPlay"`
	RememberVolume  bool    `json:"rememberVolume"`
	DefaultSpeed    float64 `json:"defaultSpeed"`    // 0.25..4.0
	Sort            string  `json:"sort"`            // "name" | "modified" | "size"
	SortDesc        bool    `json:"sortDesc"`
}

// DefaultPureVideoConfig returns default settings for PureVideo.
func DefaultPureVideoConfig() PureVideoConfig {
	return PureVideoConfig{
		Engine:         "mpv",
		DownloadSource: "auto",
		Volume:         80,
		AutoPlay:       true,
		RememberVolume: true,
		DefaultSpeed:   1.0,
		Sort:           "name",
	}
}

// Normalize enforces valid bounds and choices on PureVideoConfig.
func (c *PureVideoConfig) Normalize() {
	d := DefaultPureVideoConfig()
	c.Engine = "mpv"
	switch strings.ToLower(strings.TrimSpace(c.DownloadSource)) {
	case "ghproxy", "ghfast", "sourceforge", "github", "auto":
		c.DownloadSource = strings.ToLower(strings.TrimSpace(c.DownloadSource))
	default:
		c.DownloadSource = d.DownloadSource
	}
	c.CustomMirrorURL = strings.TrimSpace(c.CustomMirrorURL)
	if c.Volume < 0 {
		c.Volume = 0
	}
	if c.Volume > 100 {
		c.Volume = 100
	}
	if c.DefaultSpeed <= 0 {
		c.DefaultSpeed = d.DefaultSpeed
	}
	if c.DefaultSpeed < 0.25 {
		c.DefaultSpeed = 0.25
	}
	if c.DefaultSpeed > 4.0 {
		c.DefaultSpeed = 4.0
	}
	switch strings.ToLower(strings.TrimSpace(c.Sort)) {
	case "modified", "size", "name":
		c.Sort = strings.ToLower(strings.TrimSpace(c.Sort))
	default:
		c.Sort = d.Sort
	}
}

func LoadPureVideo(sec Section) PureVideoConfig {
	cfg := DefaultPureVideoConfig()
	if sec != nil {
		_ = sec.Load(&cfg)
	}
	cfg.Normalize()
	return cfg
}

func SavePureVideo(sec Section, cfg PureVideoConfig) error {
	cfg.Normalize()
	return sec.Save(cfg)
}
