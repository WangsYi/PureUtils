package pureimage

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	pureui "pureutils/pkg/purekit/ui"
)

type settingsTabView struct {
	plugin purekitSettingsTarget
	host   purekit.Host

	saved config.PureImageConfig
	draft config.PureImageConfig

	langIdx         int
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
	v.langIdx = viewerLangIndex(v.draft.Language)
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
	pureui.SettingCard(c, "浏览与渲染", "控制 PureImage 看图窗口的缩放、透明棋盘格、目录遍历与界面语言", pal, func() {
		pureui.SettingRow(c, "滚轮缩放", "开启时滚轮缩放图片，关闭时滚轮切换上/下一张（Ctrl+滚轮恒为缩放）", pal, func() {
			ui.Switch(c, &v.draft.WheelZoom).Label("开关：滚轮缩放")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "透明区域棋盘格", "为带有透明通道的 PNG/WebP 等图片显示深灰棋盘格背景", pal, func() {
			ui.Switch(c, &v.draft.Checkerboard).Label("开关：透明区域棋盘格")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "包含子目录递归", "浏览目录时递归扫描所有子文件夹中的图片", pal, func() {
			ui.Switch(c, &v.draft.Recursive).Label("开关：包含子目录递归")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "显示图片详细信息", "在看图窗口左上角常驻显示尺寸、拍摄参数与 EXIF 信息", pal, func() {
			ui.Switch(c, &v.draft.ShowDetails).Label("开关：显示图片详细信息")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "界面语言", "PureImage 看图窗口工具栏与菜单的显示语言", pal, func() {
			seg := ui.Segmented(c, &v.langIdx, "简体中文", "English", "跟随系统").Label("看图界面语言")
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.Language = viewerLangFromIndex(v.langIdx)
			}
		})
	})

	pureui.SettingCard(c, "性能与幻灯片", "配置幻灯片播放速度、相邻图片预读数量及原生 JPEG 解码状态", pal, func() {
		pureui.SettingRow(c, "幻灯片间隔", "按 S 开启幻灯片播放时每张图片的停留时间", pal, func() {
			seg := ui.Segmented(c, &v.slideIdx, "2 秒", "4 秒", "6 秒", "10 秒").Label("看图幻灯片间隔")
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.SlideshowSeconds = viewerSlideSecondsFromIndex(v.slideIdx)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, "预读数量", "后台提前解码后续图片，翻页更顺滑但会占用适量内存", pal, func() {
			seg := ui.Segmented(c, &v.preloadIdx, "关闭", "1 张", "2 张", "3 张").Label("看图预读数量")
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.PreloadCount = v.preloadIdx
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, "JPEG 原生加速 (libjpeg-turbo)", "利用同目录 turbojpeg.dll 实现 1/2·1/4·1/8 硬件级缩放解码", pal, func() {
			if v.turboAvailable {
				pureui.PillBadge(c, "已启用 turbojpeg.dll", pal.Success, pal.SuccessSoft)
			} else {
				pureui.PillBadge(c, "未安装，使用纯 Go 解码", pal.Warning, pal.WarningSoft)
			}
		})
	})

	pureui.SettingCard(c, "资源管理器空格预览", "在 Windows 资源管理器选中图片后按空格即可快速悬浮预览（保存后生效）", pal, func() {
		pureui.SettingRow(c, "启用空格预览", "拦截资源管理器选图时的空格键并弹出居中悬浮预览窗", pal, func() {
			ui.Switch(c, &v.draft.SpacePreview).Label("开关：资源管理器空格预览")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "看图窗口内快速预览快捷键", "在任意位置触发 PureImage 当前图片的快速预览浮窗", pal, func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				pureui.Keycap(c, "Ctrl+Shift+Space", pal)
				ui.Button(c, "打开看图窗口").OnClick(func() {
					if !v.headless() {
						v.plugin.Open("")
					}
				}).Border(0, ui.Transparent)
			})
		})
	})

	pureui.SettingCard(c, "系统文件关联与默认应用", "将 40 种图片与 RAW 格式注册到系统；受 Windows 10/11 保护机制限制，设为默认双击打开需在“系统默认应用”中一键确认", pal, func() {
		pureui.SettingRow(c, "打开方式注册状态", "在文件右键“打开方式”与 Windows 默认应用列表中可见", pal, func() {
			if v.assocRegistered {
				pureui.PillBadge(c, "已注册", pal.Success, pal.SuccessSoft)
			} else {
				pureui.PillBadge(c, "未注册", pal.TextMuted, pal.Bg)
			}
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c, "加入“打开方式”列表").OnClick(func() {
				if !v.headless() {
					if err := RegisterFileAssoc(); err != nil {
						v.status = "注册打开方式失败: " + err.Error()
					} else {
						v.assocRegistered = FileAssocRegistered()
						v.status = "已加入系统“打开方式”列表"
						c.Toast("已加入打开方式列表")
					}
				}
			}).Border(0, ui.Transparent)
			ui.Button(c, "移除文件关联").OnClick(func() {
				if !v.headless() {
					if err := UnregisterFileAssoc(); err != nil {
						v.status = "移除文件关联失败: " + err.Error()
					} else {
						v.assocRegistered = FileAssocRegistered()
						v.status = "已移除系统文件关联"
						c.Toast("已移除文件关联")
					}
				}
			}).Border(0, ui.Transparent)
			ui.Button(c, "打开系统默认应用设置").OnClick(func() {
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

func viewerLangIndex(lang string) int {
	switch lang {
	case "en-US":
		return 1
	case "system":
		return 2
	default:
		return 0
	}
}

func viewerLangFromIndex(idx int) string {
	switch idx {
	case 1:
		return "en-US"
	case 2:
		return "system"
	default:
		return "zh-CN"
	}
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
