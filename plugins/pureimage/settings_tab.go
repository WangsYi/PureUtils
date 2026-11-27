package pureimage

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
	pureui "pureutils/pkg/purekit/ui"
)

type settingsTabView struct {
	plugin purekitSettingsTarget
	host   purekit.Host

	saved config.PureImageConfig
	draft config.PureImageConfig

	slideIdx        int
	preloadIdx      int
	assocRegistered bool
	turboAvailable  bool
	status          string
}

type purekitSettingsTarget interface {
	Config() config.PureImageConfig
	ApplyConfig(config.PureImageConfig) error
	Open(string)
}

func newSettingsTabView(p purekitSettingsTarget, host purekit.Host) *settingsTabView {
	cur := p.Config()
	cur.Normalize()
	v := &settingsTabView{
		plugin:         p,
		host:           host,
		turboAvailable: TurboAvailable(),
	}
	if host != nil && !host.Headless() {
		v.assocRegistered = FileAssocRegistered()
	}
	v.syncFrom(cur)
	return v
}

func (v *settingsTabView) headless() bool {
	return v.host == nil || v.host.Headless()
}

func (v *settingsTabView) syncFrom(cfg config.PureImageConfig) {
	cfg.Normalize()
	v.saved = cfg
	v.draft = cfg
	v.syncIndices()
}

func (v *settingsTabView) syncIndices() {
	v.slideIdx = viewerSlideIndex(v.draft.SlideshowSeconds)
	v.preloadIdx = clampInt(v.draft.PreloadCount, 0, 3)
}

func (v *settingsTabView) Dirty() bool {
	cand := v.draft
	cand.Normalize()
	return cand != v.saved
}

func (v *settingsTabView) Validate() error { return nil }

func (v *settingsTabView) Save() error {
	cand := v.draft
	cand.Normalize()
	if err := v.plugin.ApplyConfig(cand); err != nil {
		return err
	}
	v.syncFrom(v.plugin.Config())
	return nil
}

func (v *settingsTabView) ResetDefaults() {
	v.draft = config.DefaultPureImageConfig()
	v.syncIndices()
}

func (v *settingsTabView) Render(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, i18n.S("浏览与渲染", "Browsing & Rendering"), "", pal, func() {
		pureui.SettingRowTip(c, i18n.S("滚轮缩放", "Mouse Wheel Zoom"), i18n.S("开启时滚轮缩放图片，关闭时滚轮切换上/下一张（Ctrl+滚轮恒为缩放）", "Scroll wheel zooms when on, or switches images when off (Ctrl+Wheel always zooms)"), pal, func() {
			ui.Switch(c, &v.draft.WheelZoom).Label(i18n.S("开关：滚轮缩放", "Toggle: Wheel zoom"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("透明区域棋盘格", "Transparency Checkerboard"), "", pal, func() {
			ui.Switch(c, &v.draft.Checkerboard).Label(i18n.S("开关：透明区域棋盘格", "Toggle: Checkerboard"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("包含子目录递归", "Include Subfolders"), "", pal, func() {
			ui.Switch(c, &v.draft.Recursive).Label(i18n.S("开关：包含子目录递归", "Toggle: Recursive"))
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("显示图片详细信息", "Show Image Details HUD"), i18n.S("在看图窗口左上角常驻显示尺寸、拍摄参数与 EXIF 信息", "Always display resolution, camera settings, and EXIF info in the top-left overlay"), pal, func() {
			ui.Switch(c, &v.draft.ShowDetails).Label(i18n.S("开关：显示图片详细信息", "Toggle: Show details"))
		})
	})

	pureui.SettingCard(c, i18n.S("性能与幻灯片", "Performance & Slideshow"), "", pal, func() {
		pureui.SettingRowTip(c, i18n.S("幻灯片间隔", "Slideshow Interval"), i18n.S("按 S 开启幻灯片播放时每张图片的停留时间", "Delay per image when slideshow mode (S) is active"), pal, func() {
			seg := ui.Segmented(c, &v.slideIdx, i18n.S("2 秒", "2s"), i18n.S("4 秒", "4s"), i18n.S("6 秒", "6s"), i18n.S("10 秒", "10s")).Label(i18n.S("看图幻灯片间隔", "Slideshow interval"))
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.SlideshowSeconds = viewerSlideSecondsFromIndex(v.slideIdx)
			}
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("预读数量", "Preload Count"), i18n.S("后台提前解码后续图片，翻页更顺滑但会占用适量内存", "Decode upcoming images in background for smoother paging"), pal, func() {
			seg := ui.Segmented(c, &v.preloadIdx, i18n.S("关闭", "Off"), i18n.S("1 张", "1"), i18n.S("2 张", "2"), i18n.S("3 张", "3")).Label(i18n.S("看图预读数量", "Preload count"))
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.PreloadCount = v.preloadIdx
			}
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("JPEG 原生加速 (libjpeg-turbo)", "Native JPEG Acceleration"), i18n.S("利用同目录 turbojpeg.dll 实现 1/2·1/4·1/8 硬件级缩放解码", "Use bundled turbojpeg.dll for fast 1/2·1/4·1/8 scaled decoding"), pal, func() {
			if v.turboAvailable {
				pureui.PillBadge(c, i18n.S("已启用 turbojpeg.dll", "turbojpeg.dll active"), pal.Success, pal.SuccessSoft)
			} else {
				pureui.PillBadge(c, i18n.S("未安装，使用纯 Go 解码", "Pure Go decoder"), pal.Warning, pal.WarningSoft)
			}
		})
	})

	pureui.SettingCardTip(c, i18n.S("资源管理器空格预览", "Explorer Spacebar Preview"), i18n.S("在 Windows 资源管理器选中图片后按空格即可快速悬浮预览（保存后生效）", "Press Spacebar on a selected image in Explorer for instant preview (saves on Save)"), pal, func() {
		pureui.SettingRow(c, i18n.S("启用空格预览", "Enable Space Preview"), "", pal, func() {
			ui.Switch(c, &v.draft.SpacePreview).Label(i18n.S("开关：资源管理器空格预览", "Toggle: Explorer Space Preview"))
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("快速预览快捷键", "Quick Preview Shortcut"), i18n.S("在看图窗口内触发 PureImage 当前图片的快速预览浮窗", "Trigger the floating quick preview for the current image"), pal, func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				pureui.Keycap(c, "Ctrl+Shift+Space", pal)
				ui.Button(c, i18n.S("打开看图窗口", "Open Viewer")).OnClick(func() {
					if !v.headless() {
						v.plugin.Open("")
					}
				}).Border(0, ui.Transparent)
			})
		})
	})

	pureui.SettingCardTip(c, i18n.S("系统文件关联与默认应用", "File Associations & Default Apps"), i18n.S("将 40 种图片与 RAW 格式注册到系统；受 Windows 10/11 保护机制限制，设为默认双击打开需在“系统默认应用”中一键确认", "Register 40 image & RAW formats to HKCU; set as default via Windows Default Apps"), pal, func() {
		pureui.SettingRow(c, i18n.S("打开方式注册状态", "Registration Status"), "", pal, func() {
			if v.assocRegistered {
				pureui.PillBadge(c, i18n.S("已注册", "Registered"), pal.Success, pal.SuccessSoft)
			} else {
				pureui.PillBadge(c, i18n.S("未注册", "Not Registered"), pal.TextMuted, pal.Bg)
			}
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c, i18n.S("加入“打开方式”列表", "Add to Open With")).OnClick(func() {
				if !v.headless() {
					if err := RegisterFileAssoc(); err != nil {
						v.status = i18n.S("注册打开方式失败: ", "Failed to register file associations: ") + err.Error()
					} else {
						v.assocRegistered = FileAssocRegistered()
						v.status = i18n.S("已加入系统“打开方式”列表", "Added to Windows Open With list")
						c.Toast(i18n.S("已加入打开方式列表", "Added to Open With"))
					}
				}
			}).Border(0, ui.Transparent)
			ui.Button(c, i18n.S("移除文件关联", "Remove Associations")).OnClick(func() {
				if !v.headless() {
					if err := UnregisterFileAssoc(); err != nil {
						v.status = i18n.S("移除文件关联失败: ", "Failed to remove associations: ") + err.Error()
					} else {
						v.assocRegistered = FileAssocRegistered()
						v.status = i18n.S("已移除系统文件关联", "Removed file associations")
						c.Toast(i18n.S("已移除文件关联", "Removed associations"))
					}
				}
			}).Border(0, ui.Transparent)
			ui.Button(c, i18n.S("打开系统默认应用设置", "Open Default Apps")).OnClick(func() {
				if !v.headless() {
					if !FileAssocRegistered() {
						if err := RegisterFileAssoc(); err == nil {
							v.assocRegistered = FileAssocRegistered()
						}
					}
					_ = mygo.Shell.OpenExternal(DefaultAppsURI)
				}
			}).Border(0, ui.Transparent)
		})
		if v.status != "" {
			ui.Text(c, v.status).FontSize(11.5).TextColor(pal.TextMuted)
		}
	})
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func viewerSlideIndex(sec int) int {
	switch sec {
	case 2:
		return 0
	case 6:
		return 2
	case 10:
		return 3
	default:
		return 1
	}
}

func viewerSlideSecondsFromIndex(idx int) int {
	switch idx {
	case 0:
		return 2
	case 2:
		return 6
	case 3:
		return 10
	default:
		return 4
	}
}
