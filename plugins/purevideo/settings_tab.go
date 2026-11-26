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
				v.status = "应用 libmpv 配置失败: " + err.Error()
			} else {
				v.syncFrom(v.plugin.Config())
				v.plugin.Reload()
				v.status = "libmpv 已下载就绪并已重新加载"
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

	pureui.SettingCard(c, "libmpv 解码内核", "PureVideo 默认内置裁剪版 libmpv-2.dll 硬件解码（支持 MP4 / MKV / MOV / HEVC / FLV / 字幕）；也可按需更新或导入自定义内核", pal, func() {
		pureui.SettingRow(c, "libmpv 状态", dllPath, pal, func() {
			if installed {
				label := "已就绪"
				if dllSize > 0 {
					label = fmt.Sprintf("已就绪 · %.1f MB", float64(dllSize)/(1024*1024))
				}
				pureui.PillBadge(c, label, pal.Success, pal.SuccessSoft)
			} else {
				pureui.PillBadge(c, "未检测到", pal.TextMuted, pal.Bg)
			}
		})

		ui.Divider(c)

		sourceOpts := make([]string, len(DownloadSources))
		for i, s := range DownloadSources {
			sourceOpts[i] = s.Label
		}
		pureui.SettingRow(c, "下载源选择", "国内网络推荐保持「自动优选」，会按顺序探测加速镜像、SourceForge 与 GitHub 官方源", pal, func() {
			sel := ui.Select(c, &v.sourceLabel, sourceOpts).Width(236).Label("libmpv 下载源")
			sel.Border(0, ui.Transparent)
			if sel.Changed() {
				v.draft.DownloadSource = DownloadSourceIDFromLabel(v.sourceLabel)
			}
		})

		ui.Divider(c)

		pureui.SettingRow(c, "自定义镜像地址（可选）", "支持直链 .7z / .zip / .dll 或加速前缀，填写后将优先于预设源尝试", pal, func() {
			ti := ui.TextInput(c, &v.draft.CustomMirrorURL).
				Placeholder("https://...（留空使用预设源）").
				Width(260).
				Label("自定义镜像地址")
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
				ui.Button(c, "取消下载").OnClick(func() {
					if v.mpvMgr != nil {
						v.mpvMgr.CancelDownload()
					}
				}).Border(0, ui.Transparent)
			} else {
				btnLabel := "下载 libmpv"
				if installed {
					btnLabel = "更新 / 重新下载 libmpv"
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
				ui.Button(c, "从本地导入…").OnClick(func() {
					if v.headless() || v.mpvMgr == nil {
						return
					}
					v.importLocalPackage(c)
				}).Border(0, ui.Transparent)
				if hasOverride {
					ui.Button(c, "恢复内置内核").OnClick(func() {
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
			Title: "选择 libmpv-2.dll 或压缩包 (.7z / .zip)",
			Filters: []mygo.FileFilter{
				{Name: "libmpv 内核或压缩包", Extensions: []string{"dll", "7z", "zip"}},
				{Name: "所有文件", Extensions: []string{"*"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		v.plugin.ReleaseBackend()
		err = v.mpvMgr.ImportLocal(paths[0])
		mygo.RunOnMain(func() {
			if err != nil {
				v.status = "导入失败: " + err.Error()
			} else {
				v.draft.Engine = EngineMPV
				v.syncIndices()
				_ = v.plugin.ApplyConfig(v.draft)
				v.syncFrom(v.plugin.Config())
				v.plugin.Reload()
				v.status = "已导入并启用 libmpv"
				if c != nil {
					c.Toast("已导入并启用 libmpv")
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
		v.status = "移除失败（请先关闭正在播放的视频窗口）: " + err.Error()
		v.invalidate()
		return
	}
	v.draft.Engine = EngineMPV
	v.syncIndices()
	_ = v.plugin.ApplyConfig(v.draft)
	v.syncFrom(v.plugin.Config())
	v.plugin.Reload()
	v.status = "已移除自定义覆盖，恢复使用内置 libmpv"
	if c != nil {
		c.Toast("已恢复内置 libmpv")
	}
	v.invalidate()
}

func (v *settingsTabView) renderPlaybackCard(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, "播放体验", "控制打开视频后的默认播放行为、倍速与目录浏览顺序", pal, func() {
		pureui.SettingRow(c, "打开后自动播放", "打开本地视频时立即开始播放", pal, func() {
			ui.Switch(c, &v.draft.AutoPlay).Label("开关：打开后自动播放")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "默认循环播放", "视频播放完毕后自动重头循环，而非自动切换下一条", pal, func() {
			ui.Switch(c, &v.draft.Loop).Label("开关：默认循环播放")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "记住音量与静音状态", "在窗口底部调节音量或静音时自动保存供下次使用", pal, func() {
			ui.Switch(c, &v.draft.RememberVolume).Label("开关：记住音量与静音状态")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "默认播放倍速", "支持 0.75x 到 2x 预设或播放界面 C 键循环切换", pal, func() {
			seg := ui.Segmented(c, &v.speedIdx, "0.75x", "1x", "1.25x", "1.5x", "2x").Label("默认播放倍速")
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.DefaultSpeed = videoSpeedFromIndex(v.speedIdx)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, "目录排序方式", "控制左右翻片时同目录视频的排列顺序", pal, func() {
			seg := ui.Segmented(c, &v.sortIdx, "文件名", "修改时间", "文件大小").Label("目录排序方式")
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.Sort = videoSortFromIndex(v.sortIdx)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, "降序排列", "按名称 Z→A、最新时间或最大体积优先排序", pal, func() {
			ui.Switch(c, &v.draft.SortDesc).Label("开关：降序排列")
		})
	})
}

func (v *settingsTabView) renderAssocCard(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, "系统文件关联（21 种视频格式）", "写入当前用户的“打开方式”列表（HKCU），随后在 Windows“默认应用”设为默认后即可双击秒开视频", pal, func() {
		pureui.SettingRow(c, "关联注册状态", "涵盖 MP4 / MKV / MOV / WebM / AVI / WMV / FLV / TS 等常见格式", pal, func() {
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
