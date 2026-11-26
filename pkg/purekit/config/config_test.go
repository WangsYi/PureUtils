package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
)

func TestRoundTripPluginAndHostSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	mgr := Load(path)

	host := mgr.Host()
	host.Appearance.Theme = "dark"
	host.Appearance.Language = "en-US"
	host.Launcher.Shortcut = "Ctrl+Space"
	host.Hotkeys.Launcher = "Ctrl+Space"
	host.Hotkeys.Plugins = map[string]string{"demo": "Ctrl+Alt+D"}
	host.SetModuleDisabled(core.ModuleZip, true)
	host.SetModuleDisabled(core.ModuleHistory, true)
	if err := mgr.SaveHost(host); err != nil {
		t.Fatalf("SaveHost: %v", err)
	}

	shot := DefaultPureShotConfig()
	shot.Capture.DefaultAction = CaptureActionSave
	shot.Pins.Opacity = 0.75
	if err := SavePureShot(mgr.Section(PluginPureShot), shot); err != nil {
		t.Fatalf("SavePureShot: %v", err)
	}

	clip := DefaultPureClipConfig()
	clip.History.MaxEntries = 88
	clip.PreviewHeight = 140
	if err := SavePureClip(mgr.Section(PluginPureClip), clip); err != nil {
		t.Fatalf("SavePureClip: %v", err)
	}

	img := DefaultPureImageConfig()
	img.SpacePreview = true
	img.Sort = "size"
	if err := SavePureImage(mgr.Section(PluginPureImage), img); err != nil {
		t.Fatalf("SavePureImage: %v", err)
	}

	zip := DefaultPureZipConfig()
	zip.DefaultFormat = "7z"
	zip.AddRecentArchive(`D:\a.7z`)
	if err := SavePureZip(mgr.Section(PluginPureZip), zip); err != nil {
		t.Fatalf("SavePureZip: %v", err)
	}

	reloaded, err := Open(path)
	if err != nil {
		t.Fatalf("Open reload: %v", err)
	}
	if reloaded.Host().SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", reloaded.Host().SchemaVersion, CurrentSchemaVersion)
	}
	if reloaded.Host().Launcher.Shortcut != "Ctrl+Space" || reloaded.Host().Appearance.Theme != "dark" || reloaded.Host().Appearance.Language != i18n.PrefEN {
		t.Fatalf("Host mismatch: %+v", reloaded.Host())
	}
	if reloaded.Host().Hotkeys.Plugins["demo"] != "Ctrl+Alt+D" {
		t.Fatalf("Hotkeys.Plugins = %+v", reloaded.Host().Hotkeys.Plugins)
	}
	if !reloaded.Host().IsModuleDisabled(core.ModuleZip) || !reloaded.Host().IsModuleDisabled(core.ModuleHistory) || reloaded.Host().IsModuleDisabled(core.ModuleCapture) {
		t.Fatalf("DisabledModules round-trip = %+v", reloaded.Host().DisabledModules)
	}
	reEnabled := reloaded.Host()
	reEnabled.SetModuleDisabled(core.ModuleZip, false)
	reEnabled.SetModuleDisabled(core.ModuleHistory, false)
	if err := reloaded.SaveHost(reEnabled); err != nil {
		t.Fatalf("SaveHost reEnabled: %v", err)
	}
	if afterEnable, err := Open(path); err != nil {
		t.Fatalf("Open after re-enable: %v", err)
	} else if len(afterEnable.Host().DisabledModules) != 0 {
		t.Fatalf("expected empty DisabledModules after re-enable, got %+v", afterEnable.Host().DisabledModules)
	}
	if got := LoadPureShot(reloaded.Section(PluginPureShot)); got.Capture.DefaultAction != CaptureActionSave || got.Pins.Opacity != 0.75 {
		t.Fatalf("PureShot reload = %+v", got)
	}
	if got := LoadPureClip(reloaded.Section(PluginPureClip)); got.History.MaxEntries != 88 || got.PreviewHeight != 140 {
		t.Fatalf("PureClip reload = %+v", got)
	}
	if got := LoadPureImage(reloaded.Section(PluginPureImage)); !got.SpacePreview || got.Sort != "size" {
		t.Fatalf("PureImage reload = %+v", got)
	}
	if got := LoadPureZip(reloaded.Section(PluginPureZip)); got.DefaultFormat != "7z" || len(got.RecentArchives) != 1 || got.RecentArchives[0] != `D:\a.7z` {
		t.Fatalf("PureZip reload = %+v", got)
	}
}

func TestMissingFileDoesNotCreateDiskFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	mgr, err := Open(path)
	if err != nil {
		t.Fatalf("Open missing: %v", err)
	}
	if mgr.Host().Launcher.Shortcut != "Alt+Space" {
		t.Fatalf("default shortcut = %q", mgr.Host().Launcher.Shortcut)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("Open(missing) created file on disk: %v", statErr)
	}
}

func TestCorruptFileBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	badRaw := []byte("{broken json")
	if err := os.WriteFile(path, badRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := Open(path)
	if err == nil {
		t.Fatal("expected error on corrupt JSON")
	}
	if mgr.Host().Launcher.Shortcut != "Alt+Space" {
		t.Fatalf("corrupt fallback host = %+v", mgr.Host())
	}
	bak, err := os.ReadFile(path + ".bad")
	if err != nil || !bytes.Equal(bak, badRaw) {
		t.Fatalf("backup mismatch: bytes=%q err=%v", bak, err)
	}
	orig, _ := os.ReadFile(path)
	if !bytes.Equal(orig, badRaw) {
		t.Fatal("corrupt original file was overwritten")
	}
}

func TestLegacyFallbackAllSectionsReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	legacyJSON := `{
  "schemaVersion": 1,
  "hotkeys": {
    "captureRegion": "Ctrl+Shift+A",
    "showHistory": "Alt+V",
    "openViewer": "Ctrl+Alt+V"
  },
  "capture": {
    "defaultAction": "pin",
    "saveFolder": "D:\\Shots",
    "includeCursor": true
  },
  "pins": {
    "borderColorArgb": 4294921215,
    "opacity": 0.8
  },
  "history": {
    "maxEntries": 123,
    "pasteOnEnter": false
  },
  "appearance": {
    "theme": "dark",
    "previewHeight": 160
  },
  "advanced": {
    "selfWriteSuppressMs": 250,
    "debounceMs": 90,
    "verboseLogging": true
  },
  "viewer": {
    "spacePreview": true,
    "sort": "modified"
  },
  "zip": {
    "defaultFormat": "7z",
    "level": 9
  }
}`
	if err := os.WriteFile(path, []byte(legacyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := Open(path)
	if err != nil {
		t.Fatalf("Open legacy: %v", err)
	}
	h := mgr.Host()
	if h.Hotkeys.CaptureRegion != "Ctrl+Shift+A" || h.Hotkeys.OpenViewer != "Ctrl+Alt+V" || h.Appearance.Theme != "dark" || !h.Advanced.VerboseLogging {
		t.Fatalf("Host legacy fallback mismatch: %+v", h)
	}
	shot := LoadPureShot(mgr.Section(PluginPureShot))
	if shot.Capture.DefaultAction != CaptureActionPin || shot.Capture.SaveFolder != `D:\Shots` || !shot.Capture.IncludeCursor ||
		uint32(shot.Pins.BorderColor) != 4294921215 || shot.Pins.Opacity != 0.8 || !shot.Capture.CopyToClipboard || !shot.Pins.AlwaysOnTop {
		t.Fatalf("PureShot legacy fallback = %+v", shot)
	}
	clip := LoadPureClip(mgr.Section(PluginPureClip))
	if clip.History.MaxEntries != 123 || clip.History.PasteOnEnter || !clip.History.Enabled ||
		clip.PreviewHeight != 160 || clip.SelfWriteSuppressMs != 250 || clip.DebounceMs != 90 {
		t.Fatalf("PureClip legacy fallback = %+v", clip)
	}
	img := LoadPureImage(mgr.Section(PluginPureImage))
	if !img.SpacePreview || img.Sort != "modified" || !img.WheelZoom {
		t.Fatalf("PureImage legacy fallback = %+v", img)
	}
	zip := LoadPureZip(mgr.Section(PluginPureZip))
	if zip.DefaultFormat != "7z" || zip.Level != 9 || !zip.ImagePreview {
		t.Fatalf("PureZip legacy fallback = %+v", zip)
	}

	afterBytes, _ := os.ReadFile(path)
	afterInfo, _ := os.Stat(path)
	afterEntries, _ := os.ReadDir(dir)
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatal("legacy settings.json bytes changed on read-only open/load")
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatal("legacy settings.json ModTime changed on read-only open/load")
	}
	if len(beforeEntries) != len(afterEntries) {
		t.Fatalf("unexpected files created in dir: %d -> %d", len(beforeEntries), len(afterEntries))
	}
}

func TestPartialLegacyFallbackRespectsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := `{"appearance":{"theme":"light"},"pins":{"borderColorArgb":4294921215}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	clip := LoadPureClip(mgr.Section(PluginPureClip))
	if clip.PreviewHeight != 96 || clip.SelfWriteSuppressMs != 400 || clip.DebounceMs != 160 {
		t.Fatalf("partial legacy clobbered PureClip defaults: %+v", clip)
	}
	shot := LoadPureShot(mgr.Section(PluginPureShot))
	if shot.Pins.BorderColor != core.ARGB(4294921215) || !shot.Capture.CopyToClipboard {
		t.Fatalf("partial legacy PureShot = %+v", shot)
	}
}

func TestPluginSectionShadowsLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := `{
  "capture": {"defaultAction": "pin"},
  "plugins": {
    "pureshot": {"capture": {"defaultAction": "copy_and_save"}}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	shot := LoadPureShot(mgr.Section(PluginPureShot))
	if shot.Capture.DefaultAction != CaptureActionCopyAndSave {
		t.Fatalf("plugins.pureshot should shadow legacy capture, got %q", shot.Capture.DefaultAction)
	}
	// Preserve extra top-level keys across unrelated plugin save.
	if err := SavePureImage(mgr.Section(PluginPureImage), DefaultPureImageConfig()); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(path)
	var check map[string]json.RawMessage
	if err := json.Unmarshal(saved, &check); err != nil {
		t.Fatal(err)
	}
	if _, ok := check["capture"]; !ok {
		t.Fatal("expected untouched legacy key capture to be preserved when saving another plugin")
	}
}
