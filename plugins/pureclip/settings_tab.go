package pureclip

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	pureui "pureutils/pkg/purekit/ui"
)

type settingsTabView struct {
	plugin *Plugin
	host   purekit.Host

	saved config.PureClipConfig
	draft config.PureClipConfig

	ignoredTokens  []string
	maxEntries     float64
	maxAgeDays     float64
	maxInlineChars float64
	listLimit      float64

	confirmClear bool
	status       string
}

func newSettingsTabView(p *Plugin, host purekit.Host) *settingsTabView {
	v := &settingsTabView{
		plugin: p,
		host:   host,
	}
	v.syncFrom(p.Config())
	return v
}

func (v *settingsTabView) syncFrom(cfg config.PureClipConfig) {
	cfg.Normalize()
	v.saved = clonePureClipConfig(cfg)
	v.draft = clonePureClipConfig(cfg)
	v.syncFields()
}

func (v *settingsTabView) syncFields() {
	v.ignoredTokens = normalizeTokens(v.draft.History.IgnoredProcesses)
	v.draft.History.IgnoredProcesses = append([]string(nil), v.ignoredTokens...)
	v.maxEntries = float64(v.draft.History.MaxEntries)
	v.maxAgeDays = float64(v.draft.History.MaxAgeDays)
	v.maxInlineChars = float64(v.draft.History.MaxInlineChars)
	v.listLimit = float64(v.draft.History.ListLimit)
}

func clonePureClipConfig(cfg config.PureClipConfig) config.PureClipConfig {
	out := cfg
	out.History.IgnoredProcesses = append([]string(nil), cfg.History.IgnoredProcesses...)
	return out
}

func pureClipConfigEqual(a, b config.PureClipConfig) bool {
	if a.History.Enabled != b.History.Enabled ||
		a.History.MaxEntries != b.History.MaxEntries ||
		a.History.MaxAgeDays != b.History.MaxAgeDays ||
		a.History.MaxInlineChars != b.History.MaxInlineChars ||
		a.History.CaptureImages != b.History.CaptureImages ||
		a.History.CaptureFiles != b.History.CaptureFiles ||
		a.History.ListLimit != b.History.ListLimit ||
		a.History.PasteOnEnter != b.History.PasteOnEnter ||
		a.PreviewHeight != b.PreviewHeight ||
		a.SelfWriteSuppressMs != b.SelfWriteSuppressMs ||
		a.DebounceMs != b.DebounceMs {
		return false
	}
	if len(a.History.IgnoredProcesses) != len(b.History.IgnoredProcesses) {
		return false
	}
	for i := range a.History.IgnoredProcesses {
		if a.History.IgnoredProcesses[i] != b.History.IgnoredProcesses[i] {
			return false
		}
	}
	return true
}

func (v *settingsTabView) candidate() config.PureClipConfig {
	cand := clonePureClipConfig(v.draft)
	cand.History.MaxEntries = int(v.maxEntries)
	cand.History.MaxAgeDays = int(v.maxAgeDays)
	cand.History.MaxInlineChars = int(v.maxInlineChars)
	cand.History.ListLimit = int(v.listLimit)
	cand.History.IgnoredProcesses = normalizeTokens(v.ignoredTokens)
	cand.Normalize()
	return cand
}

func (v *settingsTabView) Dirty() bool {
	return !pureClipConfigEqual(v.candidate(), v.saved)
}

func (v *settingsTabView) Validate() error { return nil }

func (v *settingsTabView) Save() error {
	cand := v.candidate()
	if err := v.plugin.ApplyConfig(cand); err != nil {
		return err
	}
	v.syncFrom(v.plugin.Config())
	return nil
}

func (v *settingsTabView) ResetDefaults() {
	v.draft = config.DefaultPureClipConfig()
	v.syncFields()
}

func (v *settingsTabView) Render(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, "监听与回放", "控制后台剪贴板收集类型以及回放粘贴行为", pal, func() {
		pureui.SettingRow(c, "记录剪贴板历史", "启用后台剪贴板变更监听与去重入库", pal, func() {
			ui.Switch(c, &v.draft.History.Enabled).Label("开关：记录剪贴板历史")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "记录图片", "将复制的位图与截图保存为内容寻址 PNG 图片", pal, func() {
			ui.Switch(c, &v.draft.History.CaptureImages).Label("开关：记录图片")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "记录文件地址", "记录资源管理器复制的文件与文件夹路径引用", pal, func() {
			ui.Switch(c, &v.draft.History.CaptureFiles).Label("开关：记录文件地址")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "回放后自动向原窗口发送粘贴", "在历史面板选择粘贴后切回原前景窗口并模拟 Ctrl+V", pal, func() {
			ui.Switch(c, &v.draft.History.PasteOnEnter).Label("开关：回放后自动向原窗口发送粘贴")
		})
	})

	pureui.SettingCard(c, "保留策略与上限", "收藏项不会被自动清理；最大天数为 0 时仅按最大条数约束", pal, func() {
		pureui.SettingRow(c, "最大条数", "非收藏记录的保留条数上限", pal, func() {
			if ui.NumberInput(c, &v.maxEntries, 1, 100000, 10).Label("数值：最大条数").Changed() {
				v.draft.History.MaxEntries = int(v.maxEntries)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, "最大天数", "超过指定天数的非收藏记录将被自动清理（0 表示不限）", pal, func() {
			if ui.NumberInput(c, &v.maxAgeDays, 0, 3650, 1).Label("数值：最大天数").Changed() {
				v.draft.History.MaxAgeDays = int(v.maxAgeDays)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, "正文内联上限", "单条文本存入 SQLite 的最大字符数，超出部分截断标记", pal, func() {
			if ui.NumberInput(c, &v.maxInlineChars, 1024, 1048576, 1024).Label("数值：正文内联上限").Changed() {
				v.draft.History.MaxInlineChars = int(v.maxInlineChars)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, "列表取回条数", "历史面板单次检索与展示的最大列表数量", pal, func() {
			if ui.NumberInput(c, &v.listLimit, 20, 2000, 20).Label("数值：列表取回条数").Changed() {
				v.draft.History.ListLimit = int(v.listLimit)
			}
		})
	})

	pureui.SettingCard(c, "隐私保护", "来自密码管理器等敏感前台进程的复制内容将被自动跳过", pal, func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, "排除进程").FontSize(13).FontWeight(500).TextColor(pal.Text)
			tf := ui.TokenField(c, &v.ignoredTokens, []string{
				"keepass", "1password", "bitwarden", "keepassxc", "enpass", "lastpass",
			}).Label("排除进程输入框")
			if tf.Changed() {
				v.ignoredTokens = normalizeTokens(v.ignoredTokens)
				v.draft.History.IgnoredProcesses = append([]string(nil), v.ignoredTokens...)
			}
			ui.Text(c, "输入进程名（不含 .exe，忽略大小写）后按 Enter 或逗号添加；点击标签 × 移除。").
				FontSize(11).TextColor(pal.TextMuted)
		})
	})

	if v.plugin.service != nil {
		pureui.SettingCard(c, "历史维护", "独立管理 plugins/pureclip 存储（只读兼容旧库不会被修改）", pal, func() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				ui.Button(c, "清理过期历史").OnClick(func() {
					v.plugin.service.Prune()
					v.status = "已按保留策略清理"
					c.Toast(v.status)
				})
				ui.Button(c, "回收孤儿图片").OnClick(func() {
					n, err := v.plugin.service.CollectOrphans()
					if err != nil {
						v.status = "回收失败: " + err.Error()
					} else {
						v.status = fmt.Sprintf("已回收 %d 张孤儿图片", n)
						c.Toast(v.status)
					}
				})
			})
			ui.Divider(c)
			pureui.SettingRow(c, "清空非收藏历史", "删除全部未收藏的剪贴板历史条目及孤儿图片，不可恢复", pal, func() {
				clearBtn := ui.ButtonBase(c).Padding(6, 12).Radius(7).Label("立即清空").Tooltip("清空非收藏剪贴板历史")
				bg := pal.DangerSoft
				fg := pal.Danger
				if clearBtn.Pressed() {
					bg = pal.Danger.Alpha(0.28)
				} else if clearBtn.Hovered() {
					bg = pal.Danger.Alpha(0.22)
				}
				clearBtn.Background(bg).Border(1, pal.Danger.Alpha(0.38))
				clearBtn.Children(func() {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, pureui.IconTrash).Size(13, 13).TextColor(fg)
						ui.Text(c, "立即清空").FontSize(12).FontWeight(500).SingleLine().TextColor(fg)
					})
				})
				clearBtn.OnClick(func() {
					v.confirmClear = true
				})
			})
			if v.status != "" {
				ui.Text(c, v.status).FontSize(11.5).TextColor(pal.TextMuted)
			}
		})
	}

	if res := ui.AlertDialog(c, &v.confirmClear,
		"清空非收藏的剪贴板历史？",
		"收藏项会保留，其余历史与未被引用的内容删除后无法撤销。",
		"取消", "清空",
	); res == 1 {
		if v.plugin.service != nil {
			if err := v.plugin.service.Clear(); err != nil {
				v.status = "清空失败: " + err.Error()
			} else {
				v.status = "已清空非收藏历史"
				c.Toast(v.status)
			}
		}
	}
}

func normalizeTokens(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		t := strings.ToLower(strings.TrimSpace(raw))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}
