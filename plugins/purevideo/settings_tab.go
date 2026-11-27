//go:build windows

package purevideo

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
	pureui "pureutils/pkg/purekit/ui"
)

type settingsTabTarget interface {
	Config() config.PureVideoConfig
	ApplyConfig(config.PureVideoConfig) error
	ReleaseBackend()
	Reload()
	Open(string)
}

type settingsTabView struct {
	plugin settingsTabTarget
	host   purekit.Host
	mpvMgr *MPVManager

	mu    sync.Mutex
	saved config.PureVideoConfig
	draft config.PureVideoConfig

	sourceLabel     string
	speedIdx        int
	sortIdx         int
	assocRegistered bool
	autoSwitchedOn  bool
	status          string
}

func newSettingsTabView(p settingsTabTarget, host purekit.Host, mpvMgr *MPVManager) *settingsTabView {
	cur := p.Config()
	cur.Normalize()
	v := &settingsTabView{
		plugin: p,
		host:   host,
		mpvMgr: mpvMgr,
	}
	if host != nil && !host.Headless() {
		v.assocRegistered = FileAssocRegistered()
	}
	v.syncFrom(cur)
	if mpvMgr != nil {
		mpvMgr.SetOnChange(v.onDownloadProgressChanged)
	}
	return v
}

func (v *settingsTabView) headless() bool {
	return v.host == nil || v.host.Headless()
}

func (v *settingsTabView) invalidate() {
	if inv, ok := v.host.(purekit.SettingsInvalidator); ok {
		inv.InvalidateSettings()
	}
}

func (v *settingsTabView) onDownloadProgressChanged() {
	if v.mpvMgr == nil {
		return
	}
	prog := v.mpvMgr.Progress()
	apply := func() {
		if prog.Completed && !v.autoSwitchedOn {
			v.autoSwitchedOn = true
			v.draft.Engine = EngineMPV
			v.draft.DownloadSource = DownloadSourceIDFromLabel(v.sourceLabel)
			v.syncIndices()
			if err := v.plugin.ApplyConfig(v.draft); err != nil {
				v.status = i18n.F("应用 libmpv 配置失败: %v", "Failed to apply libmpv config: %v", err)
			} else {
				v.syncFrom(v.plugin.Config())
				v.plugin.Reload()
				v.status = i18n.S("libmpv 已下载就绪并已重新加载", "libmpv downloaded and reloaded")
			}
		} else if !prog.Active && prog.Err != "" {
			v.status = prog.Err
		}
		v.invalidate()
	}
	if v.headless() {
		apply()
	} else {
		mygo.RunOnMain(apply)
	}
}

func (v *settingsTabView) syncFrom(cfg config.PureVideoConfig) {
	cfg.Normalize()
	v.saved = cfg
	v.draft = cfg
	v.syncIndices()
}

func (v *settingsTabView) syncIndices() {
	v.sourceLabel = DownloadSourceLabel(v.draft.DownloadSource)
	v.speedIdx = videoSpeedIndex(v.draft.DefaultSpeed)
	v.sortIdx = videoSortIndex(v.draft.Sort)
}

func (v *settingsTabView) Dirty() bool {
	cand := v.draft
	cand.DownloadSource = DownloadSourceIDFromLabel(v.sourceLabel)
	cand.Normalize()
	return cand != v.saved
}

func (v *settingsTabView) Validate() error { return nil }

func (v *settingsTabView) Save() error {
	cand := v.draft
	cand.DownloadSource = DownloadSourceIDFromLabel(v.sourceLabel)
	cand.Normalize()
	if err := v.plugin.ApplyConfig(cand); err != nil {
		return err
	}
	v.syncFrom(v.plugin.Config())
	return nil
}

func (v *settingsTabView) ResetDefaults() {
	v.draft = config.DefaultPureVideoConfig()
	v.syncIndices()
}

func (v *settingsTabView) Render(c *ui.Context, pal pureui.Palette) {
	v.renderEngineCard(c, pal)
	v.renderPlaybackCard(c, pal)
	v.renderAssocCard(c, pal)
}

func (v *settingsTabView) renderEngineCard(c *ui.Context, pal pureui.Palette) {
	installed, dllPath, dllSize := false, "", int64(0)
	hasOverride := false
	var prog DownloadProgress
	if v.mpvMgr != nil {
		installed, dllPath, dllSize = v.mpvMgr.Status()
		hasOverride = v.mpvMgr.HasManagedOverride()
		prog = v.mpvMgr.Progress()
	}

	pureui.SettingCardTip(c, i18n.S("libmpv 解码内核", "libmpv Decoder Backend"), i18n.S("PureVideo 默认内置裁剪版 libmpv-2.dll 硬件解码（支持 MP4 / MKV / MOV / HEVC / FLV / 字幕）；也可按需更新或导入自定义内核", "PureVideo bundles a trimmed libmpv-2.dll hardware decoder (MP4 / MKV / MOV / HEVC / FLV / subtitles). You can update or import a custom build below."), pal, func() {
		pureui.SettingRow(c, i18n.S("libmpv 状态", "libmpv Status"), dllPath, pal, func() {
			if installed {
				label := i18n.S("已就绪", "Ready")
				if dllSize > 0 {
					label = i18n.F("已就绪 · %.1f MB", "Ready · %.1f MB", float64(dllSize)/(1024*1024))
				}
				pureui.PillBadge(c, label, pal.Success, pal.SuccessSoft)
			} else {
				pureui.PillBadge(c, i18n.S("未检测到", "Not Found"), pal.TextMuted, pal.Bg)
			}
		})

		ui.Divider(c)

		v.sourceLabel = DownloadSourceLabel(v.draft.DownloadSource)
		sourceOpts := DownloadSourceLabels()
		pureui.SettingRowTip(c, i18n.S("下载源选择", "Download Source"), i18n.S("国内网络推荐保持「自动优选」，会按顺序探测加速镜像、SourceForge 与 GitHub 官方源", "Auto tries mirror proxies, SourceForge, and GitHub in order"), pal, func() {
			sel := ui.Select(c, &v.sourceLabel, sourceOpts).Width(224).Label(i18n.S("libmpv 下载源", "libmpv Download Source"))
			sel.Border(0, ui.Transparent)
			if sel.Changed() {
				v.draft.DownloadSource = DownloadSourceIDFromLabel(v.sourceLabel)
			}
		})

		ui.Divider(c)

		pureui.SettingRowTip(c, i18n.S("自定义镜像地址（可选）", "Custom Mirror URL (Optional)"), i18n.S("支持直链 .7z / .zip / .dll 或加速前缀，填写后将优先于预设源尝试", "Direct .7z / .zip / .dll link or proxy prefix; tried before preset sources when set"), pal, func() {
			ti := ui.TextInput(c, &v.draft.CustomMirrorURL).
				Placeholder(i18n.S("https://...（留空使用预设源）", "https://... (leave empty for preset)")).
				Width(220).
				Label(i18n.S("自定义镜像地址", "Custom Mirror URL"))
			ti.Border(0, ui.Transparent).FocusRing(false)
		})

		if prog.Active {
			ui.Divider(c)
			pct := 0.0
			if prog.TotalBytes > 0 {
				pct = float64(prog.DoneBytes) / float64(prog.TotalBytes)
				if pct > 1 {
					pct = 1
				}
			}
			ui.Column(c).Gap(6).FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, prog.Stage).FontSize(12).FontWeight(500).TextColor(pal.Text).Grow(1)
					if prog.TotalBytes > 0 {
						ui.Text(c, fmt.Sprintf("%.1f MB / %.1f MB",
							float64(prog.DoneBytes)/(1024*1024),
							float64(prog.TotalBytes)/(1024*1024),
						)).FontSize(11).TextColor(pal.TextSecondary)
					} else if prog.DoneBytes > 0 {
						ui.Text(c, fmt.Sprintf("%.1f MB", float64(prog.DoneBytes)/(1024*1024))).
							FontSize(11).TextColor(pal.TextSecondary)
					}
				})
				ui.Progress(c, pct).FillWidth().Height(6).Radius(3)
			})
		}

		ui.Row(c).Gap(8).Wrap().Children(func() {
			if prog.Active {
				ui.Button(c, i18n.S("取消下载", "Cancel Download")).OnClick(func() {
					if v.mpvMgr != nil {
						v.mpvMgr.CancelDownload()
					}
				}).Border(0, ui.Transparent)
			} else {
				btnLabel := i18n.S("下载 libmpv", "Download libmpv")
				if installed {
					btnLabel = i18n.S("更新 / 重新下载 libmpv", "Update / Re-download libmpv")
				}
				ui.PrimaryButton(c, btnLabel).OnClick(func() {
					if v.mpvMgr == nil {
						return
					}
					v.plugin.ReleaseBackend()
					v.autoSwitchedOn = false
					v.status = ""
					src := DownloadSourceIDFromLabel(v.sourceLabel)
					v.mpvMgr.StartDownload(src, v.draft.CustomMirrorURL)
				})
				ui.Button(c, i18n.S("从本地导入…", "Import Local File…")).OnClick(func() {
					if v.headless() || v.mpvMgr == nil {
						return
					}
					v.importLocalPackage(c)
				}).Border(0, ui.Transparent)
				if hasOverride {
					ui.Button(c, i18n.S("恢复内置内核", "Restore Bundled Backend")).OnClick(func() {
						v.removeLibMPV(c)
					}).Border(0, ui.Transparent)
				}
			}
		})

		msg := strings.TrimSpace(v.status)
		msgColor := pal.TextMuted
		if !prog.Active && strings.TrimSpace(prog.Err) != "" {
			msg = prog.Err
			msgColor = pal.Danger
		}
		if msg != "" {
			ui.Text(c, msg).FontSize(11.5).TextColor(msgColor)
		}
	})
}

func (v *settingsTabView) importLocalPackage(c *ui.Context) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: i18n.S("选择 libmpv-2.dll 或压缩包 (.7z / .zip)", "Select libmpv-2.dll or Archive (.7z / .zip)"),
			Filters: []mygo.FileFilter{
				{Name: i18n.S("libmpv 内核或压缩包", "libmpv Backend or Archive"), Extensions: []string{"dll", "7z", "zip"}},
				{Name: i18n.S("所有文件", "All Files"), Extensions: []string{"*"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		v.plugin.ReleaseBackend()
		err = v.mpvMgr.ImportLocal(paths[0])
		mygo.RunOnMain(func() {
			if err != nil {
				v.status = i18n.F("导入失败: %v", "Import failed: %v", err)
			} else {
				v.draft.Engine = EngineMPV
				v.syncIndices()
				_ = v.plugin.ApplyConfig(v.draft)
				v.syncFrom(v.plugin.Config())
				v.plugin.Reload()
				v.status = i18n.S("已导入并启用 libmpv", "Imported and enabled libmpv")
				if c != nil {
					c.Toast(i18n.S("已导入并启用 libmpv", "Imported and enabled libmpv"))
				}
			}
			v.invalidate()
		})
	}()
}

func (v *settingsTabView) removeLibMPV(c *ui.Context) {
	if v.mpvMgr == nil {
		return
	}
	v.plugin.ReleaseBackend()
	if err := v.mpvMgr.Remove(); err != nil {
		v.status = i18n.F("移除失败（请先关闭正在播放的视频窗口）: %v", "Removal failed (close active player windows first): %v", err)
		v.invalidate()
		return
	}
	v.draft.Engine = EngineMPV
	v.syncIndices()
	_ = v.plugin.ApplyConfig(v.draft)
	v.syncFrom(v.plugin.Config())
	v.plugin.Reload()
	v.status = i18n.S("已移除自定义覆盖，恢复使用内置 libmpv", "Custom override removed; restored bundled libmpv")
	if c != nil {
		c.Toast(i18n.S("已恢复内置 libmpv", "Restored bundled libmpv"))
	}
	v.invalidate()
}

func (v *settingsTabView) renderPlaybackCard(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, i18n.S("播放体验", "Playback"), "", pal, func() {
		pureui.SettingRow(c, i18n.S("打开后自动播放", "Auto-Play on Open"), "", pal, func() {
			ui.Switch(c, &v.draft.AutoPlay).Label(i18n.S("开关：打开后自动播放", "Toggle: Auto-Play on Open"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("默认循环播放", "Loop by Default"), "", pal, func() {
			ui.Switch(c, &v.draft.Loop).Label(i18n.S("开关：默认循环播放", "Toggle: Loop by Default"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("记住音量与静音状态", "Remember Volume & Mute"), "", pal, func() {
			ui.Switch(c, &v.draft.RememberVolume).Label(i18n.S("开关：记住音量与静音状态", "Toggle: Remember Volume & Mute"))
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("默认播放倍速", "Default Speed"), i18n.S("支持 0.75x 到 2x 预设或播放界面 C 键循环切换", "Presets from 0.75x to 2x, or cycle in player with C"), pal, func() {
			seg := ui.Segmented(c, &v.speedIdx, "0.75x", "1x", "1.25x", "1.5x", "2x").Label(i18n.S("默认播放倍速", "Default Speed"))
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.DefaultSpeed = videoSpeedFromIndex(v.speedIdx)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("目录排序方式", "Folder Sort Order"), "", pal, func() {
			seg := ui.Segmented(c, &v.sortIdx,
				i18n.S("文件名", "Name"),
				i18n.S("修改时间", "Modified"),
				i18n.S("文件大小", "Size"),
			).Label(i18n.S("目录排序方式", "Folder Sort Order"))
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.Sort = videoSortFromIndex(v.sortIdx)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("降序排列", "Descending Order"), "", pal, func() {
			ui.Switch(c, &v.draft.SortDesc).Label(i18n.S("开关：降序排列", "Toggle: Descending Order"))
		})
	})
}

func (v *settingsTabView) renderAssocCard(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCardTip(c, i18n.S("系统文件关联（21 种视频格式）", "System File Associations (21 Formats)"), i18n.S("写入当前用户的“打开方式”列表（HKCU），随后在 Windows“默认应用”设为默认后即可双击秒开视频", "Registers PureVideo in your Windows Open With list (HKCU); set as default in Windows Settings for double-click launch"), pal, func() {
		pureui.SettingRowTip(c, i18n.S("关联注册状态", "Registration Status"), i18n.S("涵盖 MP4 / MKV / MOV / WebM / AVI / WMV / FLV / TS 等常见格式", "Covers MP4, MKV, MOV, WebM, AVI, WMV, FLV, TS, and more"), pal, func() {
			if v.assocRegistered {
				pureui.PillBadge(c, i18n.S("已注册", "Registered"), pal.Success, pal.SuccessSoft)
			} else {
				pureui.PillBadge(c, i18n.S("未注册", "Not Registered"), pal.TextMuted, pal.Bg)
			}
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c, i18n.S("加入“打开方式”列表", "Add to Open With List")).OnClick(func() {
				if !v.headless() {
					if err := RegisterFileAssoc(); err != nil {
						v.status = i18n.F("注册打开方式失败: %v", "Failed to register file associations: %v", err)
					} else {
						v.assocRegistered = FileAssocRegistered()
						v.status = i18n.S("已加入系统“打开方式”列表", "Added to Windows Open With list")
						c.Toast(i18n.S("已加入打开方式列表", "Added to Open With list"))
					}
				}
			}).Border(0, ui.Transparent)
			ui.Button(c, i18n.S("移除文件关联", "Remove Associations")).OnClick(func() {
				if !v.headless() {
					if err := UnregisterFileAssoc(); err != nil {
						v.status = i18n.F("移除文件关联失败: %v", "Failed to remove file associations: %v", err)
					} else {
						v.assocRegistered = FileAssocRegistered()
						v.status = i18n.S("已移除系统文件关联", "Removed file associations")
						c.Toast(i18n.S("已移除文件关联", "Removed file associations"))
					}
				}
			}).Border(0, ui.Transparent)
			ui.Button(c, i18n.S("打开系统默认应用设置", "Open Windows Default Apps")).OnClick(func() {
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
	})
}

func videoSpeedIndex(s float64) int {
	presets := []float64{0.75, 1.0, 1.25, 1.5, 2.0}
	best := 1
	bestDiff := 999.0
	for i, p := range presets {
		d := math.Abs(s - p)
		if d < bestDiff {
			bestDiff = d
			best = i
		}
	}
	return best
}

func videoSpeedFromIndex(idx int) float64 {
	presets := []float64{0.75, 1.0, 1.25, 1.5, 2.0}
	if idx < 0 || idx >= len(presets) {
		return 1.0
	}
	return presets[idx]
}

func videoSortIndex(sort string) int {
	switch sort {
	case "modified":
		return 1
	case "size":
		return 2
	default:
		return 0
	}
}

func videoSortFromIndex(idx int) string {
	switch idx {
	case 1:
		return "modified"
	case 2:
		return "size"
	default:
		return "name"
	}
}
