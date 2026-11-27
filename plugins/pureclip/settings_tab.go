package pureclip

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
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
	pureui.SettingCard(c, i18n.S("监听与回放", "Capture & Paste"), "", pal, func() {
		pureui.SettingRow(c, i18n.S("记录剪贴板历史", "Record Clipboard History"), "", pal, func() {
			ui.Switch(c, &v.draft.History.Enabled).Label(i18n.S("开关：记录剪贴板历史", "Toggle: Record Clipboard History"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("记录图片", "Capture Images"), "", pal, func() {
			ui.Switch(c, &v.draft.History.CaptureImages).Label(i18n.S("开关：记录图片", "Toggle: Capture Images"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("记录文件地址", "Capture File Paths"), "", pal, func() {
			ui.Switch(c, &v.draft.History.CaptureFiles).Label(i18n.S("开关：记录文件地址", "Toggle: Capture File Paths"))
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("回放后自动向原窗口发送粘贴", "Auto-Paste to Previous Window"), i18n.S("在历史面板选择粘贴后切回原前景窗口并模拟 Ctrl+V", "Focus the previous window and send Ctrl+V after selecting an item"), pal, func() {
			ui.Switch(c, &v.draft.History.PasteOnEnter).Label(i18n.S("开关：回放后自动向原窗口发送粘贴", "Toggle: Auto-Paste to Previous Window"))
		})
	})

	pureui.SettingCardTip(c, i18n.S("保留策略与上限", "Retention & Limits"), i18n.S("收藏项不会被自动清理；最大天数为 0 时仅按最大条数约束", "Starred items are kept; set Max Days to 0 to limit by count only"), pal, func() {
		pureui.SettingRow(c, i18n.S("最大条数", "Max Entries"), "", pal, func() {
			if ui.NumberInput(c, &v.maxEntries, 1, 100000, 10).Label(i18n.S("数值：最大条数", "Number: Max Entries")).Changed() {
				v.draft.History.MaxEntries = int(v.maxEntries)
			}
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("最大天数", "Max Age (Days)"), i18n.S("超过指定天数的非收藏记录将被自动清理（0 表示不限）", "Prune unstarred entries older than this many days (0 = unlimited)"), pal, func() {
			if ui.NumberInput(c, &v.maxAgeDays, 0, 3650, 1).Label(i18n.S("数值：最大天数", "Number: Max Age (Days)")).Changed() {
				v.draft.History.MaxAgeDays = int(v.maxAgeDays)
			}
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("正文内联上限", "Inline Text Limit"), i18n.S("单条文本存入 SQLite 的最大字符数，超出部分截断标记", "Maximum characters stored inline in SQLite per text entry"), pal, func() {
			if ui.NumberInput(c, &v.maxInlineChars, 1024, 1048576, 1024).Label(i18n.S("数值：正文内联上限", "Number: Inline Text Limit")).Changed() {
				v.draft.History.MaxInlineChars = int(v.maxInlineChars)
			}
		})
		ui.Divider(c)
		pureui.SettingRowTip(c, i18n.S("列表取回条数", "List Fetch Limit"), i18n.S("历史面板单次检索与展示的最大列表数量", "Maximum items queried and shown per search in history window"), pal, func() {
			if ui.NumberInput(c, &v.listLimit, 20, 2000, 20).Label(i18n.S("数值：列表取回条数", "Number: List Fetch Limit")).Changed() {
				v.draft.History.ListLimit = int(v.listLimit)
			}
		})
	})

	pureui.SettingCardTip(c, i18n.S("隐私保护", "Privacy Exclusions"), i18n.S("来自密码管理器等敏感前台进程的复制内容将被自动跳过", "Skip copies from sensitive foreground apps such as password managers"), pal, func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i18n.S("排除进程", "Excluded Processes")).FontSize(13).FontWeight(500).TextColor(pal.Text).Shrink(1).MinWidth(0)
				pureui.InfoTip(c, i18n.S("输入进程名（不含 .exe，忽略大小写）后按 Enter 或逗号添加；点击标签 × 移除。", "Type process names (without .exe, case-insensitive) and press Enter or comma; click × on a tag to remove."), pal)
			})
			tf := ui.TokenField(c, &v.ignoredTokens, []string{
				"keepass", "1password", "bitwarden", "keepassxc", "enpass", "lastpass",
			}).Border(0, ui.Transparent).Label(i18n.S("排除进程输入框", "Excluded processes input"))
			if tf.Changed() {
				v.ignoredTokens = normalizeTokens(v.ignoredTokens)
				v.draft.History.IgnoredProcesses = append([]string(nil), v.ignoredTokens...)
			}
		})
	})

	if v.plugin.service != nil {
		pureui.SettingCardTip(c, i18n.S("历史维护", "History Maintenance"), i18n.S("独立管理 plugins/pureclip 存储（只读兼容旧库不会被修改）", "Manage plugins/pureclip storage independently (legacy read-only DB is untouched)"), pal, func() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				ui.Button(c, i18n.S("清理过期历史", "Prune Expired")).Border(0, ui.Transparent).OnClick(func() {
					v.plugin.service.Prune()
					v.status = i18n.S("已按保留策略清理", "Pruned according to retention policy")
					c.Toast(v.status)
				})
				ui.Button(c, i18n.S("回收孤儿图片", "Clean Orphan Images")).Border(0, ui.Transparent).OnClick(func() {
					n, err := v.plugin.service.CollectOrphans()
					if err != nil {
						v.status = i18n.F("回收失败: %v", "Cleanup failed: %v", err)
					} else {
						v.status = i18n.F("已回收 %d 张孤儿图片", "Removed %d orphan images", n)
						c.Toast(v.status)
					}
				})
			})
			ui.Divider(c)
			pureui.SettingRowTip(c, i18n.S("清空非收藏历史", "Clear Unstarred History"), i18n.S("删除全部未收藏的剪贴板历史条目及孤儿图片，不可恢复", "Permanently delete all unstarred entries and orphan images"), pal, func() {
				clearBtn := ui.ButtonBase(c).Padding(6, 12).Radius(7).Label(i18n.S("立即清空", "Clear Now")).Tooltip(i18n.S("清空非收藏剪贴板历史", "Clear unstarred clipboard history"))
				bg := pal.DangerSoft
				fg := pal.Danger
				if clearBtn.Pressed() {
					bg = pal.Danger.Alpha(0.28)
				} else if clearBtn.Hovered() {
					bg = pal.Danger.Alpha(0.22)
				}
				clearBtn.Background(bg)
				clearBtn.Children(func() {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, pureui.IconTrash).Size(13, 13).TextColor(fg)
						ui.Text(c, i18n.S("立即清空", "Clear Now")).FontSize(12).FontWeight(500).SingleLine().TextColor(fg)
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
		i18n.S("清空非收藏的剪贴板历史？", "Clear unstarred clipboard history?"),
		i18n.S("收藏项会保留，其余历史与未被引用的内容删除后无法撤销。", "Starred items will remain. Deleted unstarred history and unreferenced files cannot be restored."),
		i18n.S("取消", "Cancel"), i18n.S("清空", "Clear"),
	); res == 1 {
		if v.plugin.service != nil {
			if err := v.plugin.service.Clear(); err != nil {
				v.status = i18n.F("清空失败: %v", "Clear failed: %v", err)
			} else {
				v.status = i18n.S("已清空非收藏历史", "Cleared unstarred history")
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
