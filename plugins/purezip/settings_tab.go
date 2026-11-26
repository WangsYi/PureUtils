package purezip

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/purezip/archive"
	"pureutils/plugins/purezip/zipshell"
)

type settingsTabView struct {
	plugin *Plugin
	host   purekit.Host

	saved config.PureZipConfig
	draft config.PureZipConfig

	zipFormat          string
	zipLevelIdx        int
	zipEncIdx          int
	zipOverIdx         int
	zipEncLabel        string
	zipCtxRegistered   bool
	zipAssocRegistered bool
	status             string
}

func newSettingsTabView(p *Plugin, host purekit.Host) *settingsTabView {
	cur := p.Config()
	cur.Normalize()
	v := &settingsTabView{
		plugin: p,
		host:   host,
	}
	if host != nil && !host.Headless() {
		v.zipCtxRegistered = zipshell.ContextMenuRegistered()
		v.zipAssocRegistered = zipshell.FileAssocRegistered()
	}
	v.syncFrom(cur)
	return v
}

func (v *settingsTabView) headless() bool {
	return v.host == nil || v.host.Headless()
}

func (v *settingsTabView) syncFrom(cfg config.PureZipConfig) {
	cfg.Normalize()
	v.saved = cfg
	v.draft = cfg
	v.syncControls()
}

func (v *settingsTabView) syncControls() {
	v.zipFormat = archive.CreatableFormats[zipFormatIndex(v.draft.DefaultFormat)].Label()
	v.zipLevelIdx = zipLevelIndex(v.draft.Level)
	v.zipEncIdx = 0
	if v.draft.Encryption == "zipcrypto" {
		v.zipEncIdx = 1
	}
	v.zipOverIdx = 0
	for i, k := range zipOverwriteKeys {
		if k == v.draft.OverwriteMode {
			v.zipOverIdx = i
			break
		}
	}
	v.zipEncLabel = zipEncodingLabel(v.draft.NameEncoding)
}

func (v *settingsTabView) syncDraftFromControls() {
	v.draft.DefaultFormat = string(zipFormatFromLabel(v.zipFormat))
	v.draft.Level = zipLevelValues[clampInt(v.zipLevelIdx, 0, len(zipLevelValues)-1)]
	if v.zipEncIdx == 1 {
		v.draft.Encryption = "zipcrypto"
	} else {
		v.draft.Encryption = "aes256"
	}
	v.draft.OverwriteMode = zipOverwriteKeys[clampInt(v.zipOverIdx, 0, len(zipOverwriteKeys)-1)]
	v.draft.NameEncoding = zipEncodingCode(v.zipEncLabel)
}

func zipSettingsEqual(a, b config.PureZipConfig) bool {
	return a.DefaultFormat == b.DefaultFormat &&
		a.Level == b.Level &&
		a.Encryption == b.Encryption &&
		a.OverwriteMode == b.OverwriteMode &&
		a.NameEncoding == b.NameEncoding &&
		a.OpenFolderAfterExtract == b.OpenFolderAfterExtract &&
		a.CloseAfterExtract == b.CloseAfterExtract &&
		a.ImagePreview == b.ImagePreview
}

func (v *settingsTabView) Dirty() bool {
	v.syncDraftFromControls()
	cand := v.draft
	cand.Normalize()
	return !zipSettingsEqual(cand, v.saved)
}

func (v *settingsTabView) Validate() error { return nil }

func (v *settingsTabView) Save() error {
	v.syncDraftFromControls()
	cur := v.plugin.Config()
	cand := v.draft
	cand.RecentArchives = append([]string(nil), cur.RecentArchives...)
	cand.Normalize()
	if err := v.plugin.ApplyConfig(cand); err != nil {
		return err
	}
	v.syncFrom(v.plugin.Config())
	return nil
}

func (v *settingsTabView) ResetDefaults() {
	recent := append([]string(nil), v.draft.RecentArchives...)
	v.draft = config.DefaultPureZipConfig()
	v.draft.RecentArchives = recent
	v.syncControls()
}

func (v *settingsTabView) Render(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, i18n.S("默认压缩设置", "Default Compression"), i18n.S("新建压缩包与快速右键压缩的默认格式、级别及加密方式", "Default format, level, and encryption for new archives and context actions"), pal, func() {
		pureui.SettingRow(c, i18n.S("默认压缩格式", "Default Format"), i18n.S("ZIP 兼容性最好；7Z 压缩率更高；TAR 系列适合 Linux", "ZIP for best compatibility, 7Z for highest ratio, TAR for Linux"), pal, func() {
			ui.Select(c, &v.zipFormat, zipFormatLabels()).Width(130).Label(i18n.S("默认压缩格式", "Default Format")).Border(0, ui.Transparent)
		})
		ui.Divider(c)
		pureui.SettingBlock(c, i18n.S("默认压缩级别", "Default Compression Level"), i18n.S("级别越高体积越小但耗时更久", "Higher levels yield smaller files but take longer"), pal, func() {
			ui.Segmented(c, &v.zipLevelIdx, zipLevelLabels()...).Label(i18n.S("默认压缩级别", "Default Compression Level")).Border(0, ui.Transparent)
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("ZIP 默认加密方式", "Default ZIP Encryption"), i18n.S("AES-256 安全性更高；ZipCrypto 兼容 Windows 自带解压", "AES-256 is stronger; ZipCrypto is compatible with Windows built-in extractor"), pal, func() {
			ui.Segmented(c, &v.zipEncIdx, "AES-256", "ZipCrypto").Label(i18n.S("ZIP 默认加密", "Default ZIP Encryption")).Border(0, ui.Transparent)
		})
	})

	pureui.SettingCard(c, i18n.S("解压与浏览", "Extraction & Browsing"), i18n.S("配置冲突处理策略、非 UTF-8 压缩包文件名编码与浏览行为", "Configure overwrite policy, legacy filename encoding, and preview behavior"), pal, func() {
		pureui.SettingBlock(c, i18n.S("文件冲突处理", "File Conflict Policy"), i18n.S("解压目标已存在同名文件时的默认动作", "Default action when destination files already exist"), pal, func() {
			ui.Segmented(c, &v.zipOverIdx, zipOverwriteLabels()...).Label(i18n.S("默认覆盖方式", "Default Overwrite Mode")).Border(0, ui.Transparent)
		})
		ui.Divider(c)
		v.zipEncLabel = zipEncodingLabel(v.draft.NameEncoding)
		pureui.SettingRow(c, i18n.S("默认文件名编码", "Default Filename Encoding"), i18n.S("打开无 UTF-8 标记的旧版 ZIP 时使用的编码", "Encoding used for legacy ZIP archives without UTF-8 flags"), pal, func() {
			sel := ui.Select(c, &v.zipEncLabel, zipEncodingLabels()).Width(200).Label(i18n.S("默认文件名编码", "Default Filename Encoding")).Border(0, ui.Transparent)
			if sel.Changed() {
				v.draft.NameEncoding = zipEncodingCode(v.zipEncLabel)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("解压后打开文件夹", "Open Folder After Extract"), i18n.S("快速解压或对话框解压完成后在资源管理器中显示结果", "Reveal extracted items in Explorer after completion"), pal, func() {
			ui.Switch(c, &v.draft.OpenFolderAfterExtract).Label(i18n.S("开关：解压后打开文件夹", "Toggle: Open Folder After Extract"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("包内图片用 PureImage 查看", "Open Archive Images in PureImage"), i18n.S("在压缩管理器中双击图片时使用 PureImage 看图窗口打开", "Open images inside archives with PureImage when double-clicked"), pal, func() {
			ui.Switch(c, &v.draft.ImagePreview).Label(i18n.S("开关：包内图片用 PureImage 查看", "Toggle: Open Archive Images in PureImage"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("PureZip 压缩管理器", "PureZip Archive Manager"), i18n.S("管理压缩包内容、智能解压或批量创建压缩文件", "Browse archive contents, smart-extract, or create archives"), pal, func() {
			ui.Button(c, i18n.S("打开 PureZip 压缩管理器", "Open Archive Manager")).OnClick(func() {
				if !v.headless() {
					v.plugin.OpenZipManager("")
				}
			}).Border(0, ui.Transparent)
		})
	})

	pureui.SettingCard(c, i18n.S("资源管理器集成与文件关联", "Explorer Integration & Associations"), i18n.S("在资源管理器右键提供经典级联压缩/解压菜单，并将 13 种压缩格式注册到系统默认应用（HKCU 无需管理员权限）", "Adds PureZip submenu to Explorer context menu and registers 13 archive formats in HKCU (no admin required)"), pal, func() {
		pureui.SettingRow(c, i18n.S("右键菜单集成", "Explorer Context Menu"), i18n.S("在文件、文件夹及压缩包右键菜单中显示「PureZip」", "Show the PureZip submenu on files, folders, and archives"), pal, func() {
			if v.zipCtxRegistered {
				pureui.PillBadge(c, i18n.S("已注册", "Registered"), pal.Success, pal.SuccessSoft)
			} else {
				pureui.PillBadge(c, i18n.S("未注册", "Not Registered"), pal.TextMuted, pal.Bg)
			}
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c, i18n.S("注册右键菜单", "Register Context Menu")).OnClick(func() {
				if !v.headless() {
					if err := zipshell.RegisterContextMenu(); err != nil {
						v.status = i18n.F("注册右键菜单失败: %v", "Failed to register context menu: %v", err)
					} else {
						v.zipCtxRegistered = zipshell.ContextMenuRegistered()
						v.status = i18n.S("已注册资源管理器右键菜单", "Registered Explorer context menu")
						c.Toast(i18n.S("已注册右键菜单", "Context menu registered"))
					}
				}
			}).Border(0, ui.Transparent)
			ui.Button(c, i18n.S("移除右键菜单", "Remove Context Menu")).OnClick(func() {
				if !v.headless() {
					if err := zipshell.UnregisterContextMenu(); err != nil {
						v.status = i18n.F("移除右键菜单失败: %v", "Failed to remove context menu: %v", err)
					} else {
						v.zipCtxRegistered = zipshell.ContextMenuRegistered()
						v.status = i18n.S("已移除资源管理器右键菜单", "Removed Explorer context menu")
						c.Toast(i18n.S("已移除右键菜单", "Context menu removed"))
					}
				}
			}).Border(0, ui.Transparent)
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("压缩文件关联状态", "File Association Status"), i18n.S("将 zip、7z、rar、tar 等格式加入系统“打开方式”与默认应用", "Registers zip, 7z, rar, tar, and more in Windows Open With & Default Apps"), pal, func() {
			if v.zipAssocRegistered {
				pureui.PillBadge(c, i18n.S("已注册", "Registered"), pal.Success, pal.SuccessSoft)
			} else {
				pureui.PillBadge(c, i18n.S("未注册", "Not Registered"), pal.TextMuted, pal.Bg)
			}
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c, i18n.S("加入“打开方式”列表", "Add to Open With List")).OnClick(func() {
				if !v.headless() {
					if err := zipshell.RegisterFileAssoc(); err != nil {
						v.status = i18n.F("注册压缩关联失败: %v", "Failed to register file associations: %v", err)
					} else {
						v.zipAssocRegistered = zipshell.FileAssocRegistered()
						v.status = i18n.S("已将压缩格式加入“打开方式”列表", "Added archive formats to Open With list")
						c.Toast(i18n.S("已加入打开方式列表", "Added to Open With list"))
					}
				}
			}).Border(0, ui.Transparent)
			ui.Button(c, i18n.S("移除文件关联", "Remove Associations")).OnClick(func() {
				if !v.headless() {
					if err := zipshell.UnregisterFileAssoc(); err != nil {
						v.status = i18n.F("移除压缩关联失败: %v", "Failed to remove file associations: %v", err)
					} else {
						v.zipAssocRegistered = zipshell.FileAssocRegistered()
						v.status = i18n.S("已移除压缩文件关联", "Removed archive file associations")
						c.Toast(i18n.S("已移除压缩文件关联", "Removed file associations"))
					}
				}
			}).Border(0, ui.Transparent)
			ui.Button(c, i18n.S("打开系统默认应用设置", "Open Windows Default Apps")).OnClick(func() {
				if !v.headless() {
					if !zipshell.FileAssocRegistered() {
						if err := zipshell.RegisterFileAssoc(); err == nil {
							v.zipAssocRegistered = zipshell.FileAssocRegistered()
						}
					}
					_ = mygo.Shell.OpenExternal(zipshell.DefaultAppsURI)
				}
			}).Border(0, ui.Transparent)
		})
		if v.status != "" {
			ui.Text(c, v.status).FontSize(11.5).TextColor(pal.TextMuted)
		}
	})
	v.syncDraftFromControls()
}
