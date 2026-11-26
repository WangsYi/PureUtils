//go:build windows

// Package zipshell registers the Zip module with Windows Explorer: the classic
// cascading context menu (compress / extract verbs) and per-user archive file
// associations, all under HKCU so no elevation is needed.
package zipshell

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/purezip/archive"
)

const (
	// ProgID is the HKCU ProgID used for archive files.
	ProgID           = "PureUtils.Zip"
	AppDisplayName   = "PureZip"
	CapabilitiesPath = `Software\PureUtils\PureZip\Capabilities`
	DefaultAppsURI   = "ms-settings:defaultapps?registeredAppUser=PureZip"

	menuKey        = "PureUtils.PureZip"
	archiveMenuKey = "PureUtils.PureZipArchive"
	classes        = `Software\Classes\`

	shcneAssocChanged = 0x08000000
)

var procSHChangeNotify = windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify")

func notifyAssocChanged() { _, _, _ = procSHChangeNotify.Call(shcneAssocChanged, 0, 0, 0) }

// AssocExtensions are the archive extensions offered for association
// (package formats such as .jar/.apk are left to their own tools).
func AssocExtensions() []string {
	var out []string
	for _, e := range archive.ReadableExtensions {
		switch e {
		case "jar", "apk", "001":
			continue
		}
		out = append(out, e)
	}
	return out
}

func exePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(exe)
}

func iconValue(exe string) string {
	ico := filepath.Join(filepath.Dir(exe), "resources", "zip.ico")
	if _, err := os.Stat(ico); err == nil {
		return ico + ",0"
	}
	return exe + ",0"
}

type verb struct {
	id, label, arg string
}

func compressVerbs() []verb {
	return []verb{
		{"10compress", i18n.S("添加到压缩文件…", "Add to archive…"), "--compress"},
		{"11zip", i18n.S("压缩为同名 .zip", "Compress to .zip"), "--compress-zip"},
		{"12sevenzip", i18n.S("压缩为同名 .7z", "Compress to .7z"), "--compress-7z"},
	}
}

func extractVerbs() []verb {
	return []verb{
		{"01smart", i18n.S("智能解压（自动判断是否新建文件夹）", "Smart Extract"), "--extract-smart"},
		{"02here", i18n.S("解压到当前文件夹", "Extract Here"), "--extract-here"},
		{"03folder", i18n.S("解压到同名文件夹", "Extract to Subfolder"), "--extract-to"},
		{"04dialog", i18n.S("解压到…", "Extract To…"), "--extract-dialog"},
		{"05open", i18n.S("用 PureZip 打开", "Open with PureZip"), "--zip"},
		{"06test", i18n.S("测试压缩文件", "Test Archive"), "--test-archive"},
	}
}

func setStrings(path string, values map[string]string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer k.Close()
	for name, val := range values {
		if err := k.SetStringValue(name, val); err != nil {
			return err
		}
	}
	return nil
}

func writeCascade(base, title, icon, exe string, verbs []verb, appliesTo string) error {
	vals := map[string]string{"MUIVerb": title, "Icon": icon, "SubCommands": "", "MultiSelectModel": "Player"}
	if appliesTo != "" {
		vals["AppliesTo"] = appliesTo
	}
	if err := setStrings(base, vals); err != nil {
		return err
	}
	for _, v := range verbs {
		key := base + `\shell\` + v.id
		if err := setStrings(key, map[string]string{"MUIVerb": v.label, "MultiSelectModel": "Player"}); err != nil {
			return err
		}
		if err := setStrings(key+`\command`, map[string]string{"": `"` + exe + `" ` + v.arg + ` "%1"`}); err != nil {
			return err
		}
	}
	return nil
}

// RegisterContextMenu adds the "PureZip" cascade to files, folders and archives.
func RegisterContextMenu() error {
	cleanupLegacyZipContextMenu()
	exe, err := exePath()
	if err != nil {
		return err
	}
	icon := iconValue(exe)
	exts := AssocExtensions()
	var conds []string
	for _, e := range exts {
		conds = append(conds, "System.FileExtension:=."+e)
	}
	notArchive := "NOT (" + strings.Join(conds, " OR ") + ")"
	comp := compressVerbs()
	ext := extractVerbs()
	if err := writeCascade(classes+`*\shell\`+menuKey, AppDisplayName, icon, exe, comp, notArchive); err != nil {
		return err
	}
	if err := writeCascade(classes+`Directory\shell\`+menuKey, AppDisplayName, icon, exe, comp, ""); err != nil {
		return err
	}
	archiveVerbs := append(append([]verb(nil), ext...), comp[0])
	for _, e := range exts {
		if err := writeCascade(classes+`SystemFileAssociations\.`+e+`\shell\`+archiveMenuKey, AppDisplayName, icon, exe, archiveVerbs, ""); err != nil {
			return err
		}
	}
	notifyAssocChanged()
	return nil
}

// UnregisterContextMenu removes every context menu entry written by RegisterContextMenu.
func UnregisterContextMenu() error {
	root := registry.CURRENT_USER
	deleteTree(root, classes+`*\shell\`+menuKey)
	deleteTree(root, classes+`Directory\shell\`+menuKey)
	for _, e := range AssocExtensions() {
		deleteTree(root, classes+`SystemFileAssociations\.`+e+`\shell\`+archiveMenuKey)
	}
	cleanupLegacyZipContextMenu()
	notifyAssocChanged()
	return nil
}

func cleanupLegacyZipContextMenu() {
	root := registry.CURRENT_USER
	deleteTree(root, classes+`*\shell\ynm3000.Zip`)
	deleteTree(root, classes+`Directory\shell\ynm3000.Zip`)
	for _, e := range AssocExtensions() {
		deleteTree(root, classes+`SystemFileAssociations\.`+e+`\shell\ynm3000.ZipArchive`)
	}
}

// ContextMenuRegistered reports whether the cascade points at this executable.
func ContextMenuRegistered() bool {
	exe, err := exePath()
	if err != nil {
		return false
	}
	return commandContains(classes+`*\shell\`+menuKey+`\shell\`+compressVerbs()[1].id+`\command`, exe) &&
		commandContains(classes+`SystemFileAssociations\.zip\shell\`+archiveMenuKey+`\shell\`+extractVerbs()[0].id+`\command`, exe)
}

func commandContains(path, exe string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	return err == nil && strings.Contains(strings.ToLower(v), strings.ToLower(exe))
}

// RegisterFileAssoc registers PureUtils.Zip for the archive extensions (Open
// With list, Default Apps capabilities). Windows still asks the user to
// confirm the default app.
func RegisterFileAssoc() error {
	cleanupLegacyZipAssoc()
	exe, err := exePath()
	if err != nil {
		return err
	}
	icon := iconValue(exe)
	open := `"` + exe + `" --zip "%1"`
	prog := classes + ProgID
	if err := setStrings(prog, map[string]string{
		"":                 i18n.S("压缩文件", "Archive File"),
		"FriendlyTypeName": i18n.S("压缩文件 (PureZip)", "Archive File (PureZip)"),
	}); err != nil {
		return err
	}
	if err := setStrings(prog+`\DefaultIcon`, map[string]string{"": icon}); err != nil {
		return err
	}
	if err := setStrings(prog+`\shell\open\command`, map[string]string{"": open}); err != nil {
		return err
	}
	_ = setStrings(prog+`\Application`, map[string]string{"ApplicationName": AppDisplayName, "ApplicationIcon": icon})
	exts := AssocExtensions()
	for _, e := range exts {
		_ = setStrings(classes+"."+e+`\OpenWithProgids`, map[string]string{ProgID: ""})
	}

	// Shared Applications\<exe> entry (also used by the PureImage module).
	app := classes + `Applications\` + filepath.Base(exe)
	_ = setStrings(app, map[string]string{"FriendlyAppName": "PureUtils"})
	if !keyExists(app + `\shell\open\command`) {
		_ = setStrings(app+`\shell\open\command`, map[string]string{"": `"` + exe + `" "%1"`})
	}
	st := map[string]string{}
	for _, e := range exts {
		st["."+e] = ""
	}
	if err := setStrings(app+`\SupportedTypes`, st); err != nil {
		return err
	}

	if err := setStrings(CapabilitiesPath, map[string]string{
		"ApplicationName":        AppDisplayName,
		"ApplicationIcon":        icon,
		"ApplicationDescription": i18n.S("PureZip 压缩管理", "PureZip Archive Manager"),
	}); err != nil {
		return err
	}
	fa := map[string]string{}
	for _, e := range exts {
		fa["."+e] = ProgID
	}
	if err := setStrings(CapabilitiesPath+`\FileAssociations`, fa); err != nil {
		return err
	}
	if err := setStrings(`Software\RegisteredApplications`, map[string]string{AppDisplayName: CapabilitiesPath}); err != nil {
		return err
	}
	notifyAssocChanged()
	return nil
}

// UnregisterFileAssoc removes the archive associations written by RegisterFileAssoc.
func UnregisterFileAssoc() error {
	root := registry.CURRENT_USER
	exts := AssocExtensions()
	for _, e := range exts {
		if k, err := registry.OpenKey(root, classes+"."+e+`\OpenWithProgids`, registry.SET_VALUE); err == nil {
			_ = k.DeleteValue(ProgID)
			k.Close()
		}
	}
	if k, err := registry.OpenKey(root, `Software\RegisteredApplications`, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue(AppDisplayName)
		k.Close()
	}
	deleteTree(root, classes+ProgID)
	deleteTree(root, `Software\PureUtils\PureZip`)
	RemoveSupportedTypes(classes+`Applications\PureUtils.exe`, exts)
	if exe, err := exePath(); err == nil {
		app := classes + `Applications\` + filepath.Base(exe)
		RemoveSupportedTypes(app, exts)
	}
	cleanupLegacyZipAssoc()
	notifyAssocChanged()
	return nil
}

func cleanupLegacyZipAssoc() {
	root := registry.CURRENT_USER
	exts := AssocExtensions()
	for _, e := range exts {
		if k, err := registry.OpenKey(root, classes+"."+e+`\OpenWithProgids`, registry.SET_VALUE); err == nil {
			_ = k.DeleteValue("ynm3000.Zip")
			k.Close()
		}
	}
	if k, err := registry.OpenKey(root, `Software\RegisteredApplications`, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue("ynm3000 Zip")
		_ = k.DeleteValue("Zip")
		k.Close()
	}
	deleteTree(root, classes+`ynm3000.Zip`)
	deleteTree(root, `Software\ynm3000\Zip`)
	RemoveSupportedTypes(classes+`Applications\ynm3000.exe`, exts)
}

// RemoveSupportedTypes drops exts from an Applications\<exe>\SupportedTypes
// key and deletes the Applications entry once no module lists types there.
func RemoveSupportedTypes(appKey string, exts []string) {
	root := registry.CURRENT_USER
	k, err := registry.OpenKey(root, appKey+`\SupportedTypes`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return
	}
	for _, e := range exts {
		_ = k.DeleteValue("." + e)
	}
	left, _ := k.ReadValueNames(-1)
	k.Close()
	if len(left) == 0 {
		deleteTree(root, appKey)
	}
}

// FileAssocRegistered reports whether PureUtils.Zip opens archives with this executable.
func FileAssocRegistered() bool {
	exe, err := exePath()
	if err != nil {
		return false
	}
	if !commandContains(classes+ProgID+`\shell\open\command`, exe) {
		return false
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\RegisteredApplications`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(AppDisplayName)
	return err == nil && strings.EqualFold(v, CapabilitiesPath)
}

func keyExists(path string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

func deleteTree(root registry.Key, subkey string) {
	k, err := registry.OpenKey(root, subkey, registry.ENUMERATE_SUB_KEYS|registry.SET_VALUE)
	if err == nil {
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			deleteTree(root, subkey+`\`+n)
		}
	}
	_ = registry.DeleteKey(root, subkey)
}
