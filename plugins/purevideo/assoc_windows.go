//go:build windows

package purevideo

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"

	"pureutils/pkg/purekit/i18n"
)

const (
	// ProgID is the HKCU ProgID registered for PureUtils's PureVideo module.
	ProgID           = "PureUtils.Video"
	AppDisplayName   = "PureVideo"
	CapabilitiesPath = `Software\PureUtils\PureVideo\Capabilities`
	DefaultAppsURI   = "ms-settings:defaultapps?registeredAppUser=PureVideo"

	shcneAssocChanged = 0x08000000
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procSHChangeNotify = shell32.NewProc("SHChangeNotify")
)

func notifyAssocChanged() {
	_, _, _ = procSHChangeNotify.Call(shcneAssocChanged, 0, 0, 0)
}

func assocIconValue(exe string) string {
	dir := filepath.Dir(exe)
	ico := filepath.Join(dir, "resources", "video.ico")
	if _, err := os.Stat(ico); err == nil {
		return ico + ",0"
	}
	return exe + ",0"
}

// RegisterFileAssoc registers PureUtils.Video for all supported video extensions in HKCU.
func RegisterFileAssoc() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.Abs(exe)
	iconVal := assocIconValue(exe)

	root := registry.CURRENT_USER
	progPath := `Software\Classes\` + ProgID
	key, _, err := registry.CreateKey(root, progPath, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue("", AppDisplayName); err != nil {
		return err
	}
	if err := key.SetStringValue("FriendlyTypeName", AppDisplayName); err != nil {
		return err
	}

	if iconKey, _, err := registry.CreateKey(root, progPath+`\DefaultIcon`, registry.ALL_ACCESS); err == nil {
		_ = iconKey.SetStringValue("", iconVal)
		iconKey.Close()
	}

	cmdKey, _, err := registry.CreateKey(root, progPath+`\shell\open\command`, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer cmdKey.Close()
	if err := cmdKey.SetStringValue("", `"`+exe+`" --video "%1"`); err != nil {
		return err
	}

	if appKey, _, err := registry.CreateKey(root, progPath+`\Application`, registry.ALL_ACCESS); err == nil {
		_ = appKey.SetStringValue("ApplicationName", AppDisplayName)
		_ = appKey.SetStringValue("ApplicationIcon", iconVal)
		appKey.Close()
	}

	for _, ext := range VideoExtList {
		keyPath := `Software\Classes\.` + ext + `\OpenWithProgids`
		k, _, err := registry.CreateKey(root, keyPath, registry.ALL_ACCESS)
		if err != nil {
			continue
		}
		_ = k.SetStringValue(ProgID, "")
		k.Close()
	}
	if err := registerApplicationEntry(exe, iconVal); err != nil {
		return err
	}
	if err := registerCapabilities(iconVal); err != nil {
		return err
	}
	notifyAssocChanged()
	return nil
}

func registerApplicationEntry(exe, iconVal string) error {
	root := registry.CURRENT_USER
	appSubkey := `Software\Classes\Applications\` + filepath.Base(exe)
	appKey, _, err := registry.CreateKey(root, appSubkey, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	_ = appKey.SetStringValue("FriendlyAppName", "PureUtils")
	appKey.Close()

	if diKey, _, err := registry.CreateKey(root, appSubkey+`\DefaultIcon`, registry.ALL_ACCESS); err == nil {
		// Preserve existing DefaultIcon if PureImage already wrote it.
		if _, _, getErr := diKey.GetStringValue(""); getErr != nil {
			_ = diKey.SetStringValue("", iconVal)
		}
		diKey.Close()
	}
	// Only write open command when missing so PureImage (--view) doesn't break;
	// bare-path routing in host dispatches video files to CmdOpenVideo either way.
	if existing, err := registry.OpenKey(root, appSubkey+`\shell\open\command`, registry.QUERY_VALUE); err == nil {
		existing.Close()
	} else {
		cmdKey, _, err := registry.CreateKey(root, appSubkey+`\shell\open\command`, registry.ALL_ACCESS)
		if err != nil {
			return err
		}
		if err := cmdKey.SetStringValue("", `"`+exe+`" "%1"`); err != nil {
			cmdKey.Close()
			return err
		}
		cmdKey.Close()
	}

	stKey, _, err := registry.CreateKey(root, appSubkey+`\SupportedTypes`, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer stKey.Close()
	for _, ext := range VideoExtList {
		if err := stKey.SetStringValue("."+ext, ""); err != nil {
			return err
		}
	}
	return nil
}

func registerCapabilities(iconVal string) error {
	root := registry.CURRENT_USER
	key, _, err := registry.CreateKey(root, CapabilitiesPath, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue("ApplicationName", AppDisplayName); err != nil {
		return err
	}
	if err := key.SetStringValue("ApplicationIcon", iconVal); err != nil {
		return err
	}
	if err := key.SetStringValue("ApplicationDescription", i18n.S("PureVideo 极速视频浏览", "PureVideo Fast Video Player")); err != nil {
		return err
	}

	faKey, _, err := registry.CreateKey(root, CapabilitiesPath+`\FileAssociations`, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer faKey.Close()
	for _, ext := range VideoExtList {
		if err := faKey.SetStringValue("."+ext, ProgID); err != nil {
			return err
		}
	}

	regKey, _, err := registry.CreateKey(root, `Software\RegisteredApplications`, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer regKey.Close()
	return regKey.SetStringValue(AppDisplayName, CapabilitiesPath)
}

// UnregisterFileAssoc removes the HKCU file associations and Capabilities entries.
func UnregisterFileAssoc() error {
	root := registry.CURRENT_USER
	for _, ext := range VideoExtList {
		keyPath := `Software\Classes\.` + ext + `\OpenWithProgids`
		if k, err := registry.OpenKey(root, keyPath, registry.SET_VALUE); err == nil {
			_ = k.DeleteValue(ProgID)
			k.Close()
		}
	}
	if regKey, err := registry.OpenKey(root, `Software\RegisteredApplications`, registry.SET_VALUE); err == nil {
		_ = regKey.DeleteValue(AppDisplayName)
		regKey.Close()
	}
	deleteRegistryTree(root, `Software\Classes\`+ProgID)
	deleteRegistryTree(root, `Software\PureUtils\PureVideo`)
	removeVideoSupportedTypes(root, `Software\Classes\Applications\PureUtils.exe`)
	if exe, err := os.Executable(); err == nil {
		if base := filepath.Base(exe); base != "" && !strings.EqualFold(base, "PureUtils.exe") {
			removeVideoSupportedTypes(root, `Software\Classes\Applications\`+base)
		}
	}
	notifyAssocChanged()
	return nil
}

func removeVideoSupportedTypes(root registry.Key, appKey string) {
	k, err := registry.OpenKey(root, appKey+`\SupportedTypes`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return
	}
	for _, ext := range VideoExtList {
		_ = k.DeleteValue("." + ext)
	}
	left, _ := k.ReadValueNames(-1)
	k.Close()
	if len(left) == 0 {
		deleteRegistryTree(root, appKey)
	}
}

func deleteRegistryTree(root registry.Key, subkey string) {
	k, err := registry.OpenKey(root, subkey, registry.ENUMERATE_SUB_KEYS|registry.SET_VALUE)
	if err == nil {
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			deleteRegistryTree(root, subkey+`\`+n)
		}
	}
	_ = registry.DeleteKey(root, subkey)
}

// FileAssocRegistered reports whether HKCU video associations point to this executable.
func FileAssocRegistered() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exe, _ = filepath.Abs(exe)

	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\`+ProgID+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	lv := strings.ToLower(v)
	if err != nil || !strings.Contains(lv, strings.ToLower(exe)) || !strings.Contains(lv, "--video") {
		return false
	}

	regKey, err := registry.OpenKey(registry.CURRENT_USER, `Software\RegisteredApplications`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer regKey.Close()
	capPath, _, err := regKey.GetStringValue(AppDisplayName)
	if err != nil || !strings.EqualFold(capPath, CapabilitiesPath) {
		return false
	}

	faKey, err := registry.OpenKey(registry.CURRENT_USER, CapabilitiesPath+`\FileAssociations`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer faKey.Close()
	_, _, err = faKey.GetStringValue(".mp4")
	return err == nil
}
