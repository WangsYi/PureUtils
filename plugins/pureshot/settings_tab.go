package pureshot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	pureui "pureutils/pkg/purekit/ui"
)

type settingsTabView struct {
	plugin *Plugin
	host   purekit.Host

	saved config.PureShotConfig
	draft config.PureShotConfig

	captureActionIdx int
	opacity          float64
	borderThickness  float64
	borderColor      string
	borderUIColor    ui.Color
	status           string
}

func newSettingsTabView(p *Plugin, host purekit.Host) *settingsTabView {
	v := &settingsTabView{
		plugin: p,
		host:   host,
	}
	v.syncFrom(p.Config())
	return v
}

func (v *settingsTabView) headless() bool {
	return v.host == nil || v.host.Headless()
}

func (v *settingsTabView) syncFrom(cfg config.PureShotConfig) {
	cfg.Normalize()
	v.saved = cfg
	v.draft = cfg
	v.syncControls()
}

func (v *settingsTabView) syncControls() {
	v.captureActionIdx = captureActionIndex(v.draft.Capture.DefaultAction)
	v.opacity = v.draft.Pins.Opacity
	v.borderThickness = float64(v.draft.Pins.BorderThickness)
	v.borderUIColor = argbToUIColor(v.draft.Pins.BorderColor)
	v.borderColor = formatUIColorHex(v.borderUIColor)
}

func (v *settingsTabView) candidate() config.PureShotConfig {
	cand := v.draft
	cand.Capture.DefaultAction = captureActionFromIndex(v.captureActionIdx)
	cand.Pins.Opacity = v.opacity
	cand.Pins.BorderThickness = int(v.borderThickness)
	if col, err := parseHexColor(v.borderColor); err == nil {
		cand.Pins.BorderColor = col
	}
	cand.Normalize()
	return cand
}

func (v *settingsTabView) Dirty() bool {
	return v.candidate() != v.saved
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
	v.draft = config.DefaultPureShotConfig()
	v.syncControls()
}

func (v *settingsTabView) Render(c *ui.Context, pal pureui.Palette) {
	pureui.SettingCard(c, "默认操作与文件保存", "配置双击 / Enter 确认截图时的默认动作、默认保存目录与文件名模板", pal, func() {
		pureui.SettingRow(c, "双击 / Enter 默认操作", "在截图选区内双击、按 Enter 或点击“完成”时执行的默认动作", pal, func() {
			seg := ui.Segmented(c, &v.captureActionIdx, "复制到剪贴板", "保存到文件", "复制并保存", "贴到桌面").Label("截图默认完成操作")
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.Capture.DefaultAction = captureActionFromIndex(v.captureActionIdx)
			}
		})
		ui.Divider(c)
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, "保存目录").FontSize(13).FontWeight(500).TextColor(pal.Text)
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &v.draft.Capture.SaveFolder).Grow(1).Label("保存目录").Border(0, ui.Transparent).FocusRing(false)
				ui.Button(c, "浏览…").OnClick(func() {
					if v.headless() {
						return
					}
					go func() {
						paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
							Title:       "选择截图保存目录",
							Directory:   true,
							DefaultPath: v.draft.Capture.SaveFolder,
						})
						mygo.RunOnMain(func() {
							if err != nil {
								v.status = "选择目录失败: " + err.Error()
							} else if len(paths) > 0 {
								v.draft.Capture.SaveFolder = paths[0]
								v.status = "已选择目录"
							}
						})
					}()
				}).Border(0, ui.Transparent)
				if !v.headless() {
					ui.Button(c, "打开目录").OnClick(func() {
						folder := strings.TrimSpace(v.draft.Capture.SaveFolder)
						if folder == "" {
							folder = config.DefaultSaveFolder()
						}
						_ = mygo.Shell.OpenPath(folder)
					}).Border(0, ui.Transparent)
				}
			})
		})

		ui.Divider(c)

		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "文件名模板").FontSize(13).FontWeight(500).TextColor(pal.Text).Grow(1)
				pureui.PillBadge(c, "预览: "+previewFileName(v.draft.Capture.FileNamePattern), pal.Accent, pal.AccentSoft)
			})
			ui.TextInput(c, &v.draft.Capture.FileNamePattern).Label("文件名模板").Border(0, ui.Transparent).FocusRing(false)
			ui.Text(c, "支持占位符：%yyyyMMdd、%HHmmss、%yyyy、%MM、%dd、%HH、%mm、%ss、%counter").
				FontSize(11).TextColor(pal.TextMuted)
		})
	})

	pureui.SettingCard(c, "行为与框选辅助", "区域截图在光标所在显示器铺开冻结帧；整屏截图直接复制并记录", pal, func() {
		pureui.SettingRow(c, "截图后复制到剪贴板", "完成或复制选区时将结果写入系统剪贴板", pal, func() {
			ui.Switch(c, &v.draft.Capture.CopyToClipboard).Label("开关：截图后复制到剪贴板")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "截图后自动保存到目录", "每次完成截图自动落盘一份 PNG 到上方保存目录", pal, func() {
			ui.Switch(c, &v.draft.Capture.AutoSave).Label("开关：截图后自动保存到目录")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "整屏截图包含鼠标指针", "执行截取当前显示器时绘制鼠标光标", pal, func() {
			ui.Switch(c, &v.draft.Capture.IncludeCursor).Label("开关：整屏截图包含鼠标指针")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "框选时吸附窗口边缘", "拖拽框选自动对齐桌面可见窗口边界，单击可快速捕获窗口", pal, func() {
			ui.Switch(c, &v.draft.Capture.SnapToWindows).Label("开关：框选时吸附窗口边缘")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "显示放大镜与取色", "光标旁展示像素放大镜、RGB/HEX 色值，按 C 复制色值", pal, func() {
			ui.Switch(c, &v.draft.Capture.ShowMagnifier).Label("开关：显示放大镜与取色")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "显示选区尺寸", "在选区边角显示实时宽高像素尺寸标尺", pal, func() {
			ui.Switch(c, &v.draft.Capture.ShowRuler).Label("开关：显示选区尺寸")
		})
	})

	pureui.SettingCard(c, "悬浮贴图设置", "支持滚轮缩放及 Alt+滚轮微调单个贴图透明度", pal, func() {
		pureui.SettingRow(c, "新贴图默认置顶", "新建悬浮贴图时自动处于其他窗口上方", pal, func() {
			ui.Switch(c, &v.draft.Pins.AlwaysOnTop).Label("开关：新贴图默认置顶")
		})
		ui.Divider(c)
		pureui.SettingRow(c, "默认不透明度", "新建贴图窗口的初始不透明度（20%–100%）", pal, func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Slider(c, &v.opacity, 0.2, 1).Width(150).Label("滑块：默认不透明度").Changed() {
					v.draft.Pins.Opacity = v.opacity
				}
				pureui.PillBadge(c, fmt.Sprintf("%.0f%%", v.opacity*100), pal.Accent, pal.AccentSoft)
			})
		})
		ui.Divider(c)
		pureui.SettingRow(c, "边框宽度", "贴图窗口外框线像素宽度（0 表示无边框）", pal, func() {
			if ui.NumberInput(c, &v.borderThickness, 0, 10, 1).Label("数值：边框宽度").Changed() {
				v.draft.Pins.BorderThickness = int(v.borderThickness)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, "边框颜色", "点击色块呼出取色器，或直接输入十六进制色值", pal, func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				well := ui.ColorWell(c, &v.borderUIColor).Label("边框颜色选择器")
				if well.Changed() {
					v.borderColor = formatUIColorHex(v.borderUIColor)
					v.draft.Pins.BorderColor = uiColorToARGB(v.borderUIColor)
				}
				hexIn := ui.TextInput(c, &v.borderColor).Width(104).Placeholder("#3B7BFF").Label("边框颜色十六进制")
				hexIn.Border(0, ui.Transparent).FocusRing(false)
				if hexIn.Changed() {
					if col, err := parseHexColor(v.borderColor); err == nil {
						v.draft.Pins.BorderColor = col
						v.borderUIColor = argbToUIColor(col)
					}
				}
			})
		})
	})

	pureui.SettingCard(c, "实时效果预览", "按当前不透明度、边框宽度与颜色实时渲染悬浮贴图外观", pal, func() {
		ui.Box(c).Height(112).FillWidth().Radius(10).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
			tile := float32(14)
			c1 := ui.RGB(28, 32, 42)
			c2 := ui.RGB(38, 43, 56)
			if !pal.Dark {
				c1 = ui.RGB(236, 240, 247)
				c2 = ui.RGB(223, 229, 239)
			}
			p.Fill(r, c1, 0)
			for row := 0; float32(row)*tile < r.H; row++ {
				for col := 0; float32(col)*tile < r.W; col++ {
					if (row+col)%2 == 1 {
						x := r.X + float32(col)*tile
						y := r.Y + float32(row)*tile
						p.Fill(ui.Rect{X: x, Y: y, W: minf(tile, r.X+r.W-x), H: minf(tile, r.Y+r.H-y)}, c2, 0)
					}
				}
			}

			pw := minf(220, r.W-48)
			ph := float32(74)
			px := r.X + (r.W-pw)/2
			py := r.Y + (r.H-ph)/2
			card := ui.Rect{X: px, Y: py, W: pw, H: ph}
			alpha := float32(v.opacity)
			p.Shadow(card, 8, 0, 6, 18, 0, ui.RGBA(0, 0, 0, 0.28*alpha))
			p.FillGradient(card, ui.LinearGradient{
				From:  ui.RGBA(42, 65, 118, alpha),
				To:    ui.RGBA(24, 34, 62, alpha),
				Angle: 135,
			}, 8)
			p.Fill(ui.Rect{X: px + 14, Y: py + 16, W: 76, H: 8}, ui.RGBA(255, 255, 255, 0.85*alpha), 4)
			p.Fill(ui.Rect{X: px + 14, Y: py + 32, W: pw - 28, H: 6}, ui.RGBA(255, 255, 255, 0.35*alpha), 3)
			p.Fill(ui.Rect{X: px + 14, Y: py + 44, W: pw - 64, H: 6}, ui.RGBA(255, 255, 255, 0.25*alpha), 3)

			if v.borderThickness > 0 {
				bc := v.borderUIColor.Alpha(alpha)
				p.Stroke(card, bc, 8, float32(v.borderThickness))
			}
		})
		if v.status != "" {
			ui.Text(c, v.status).FontSize(11.5).TextColor(pal.TextMuted)
		}
	})
}

func captureActionIndex(action string) int {
	switch action {
	case config.CaptureActionSave:
		return 1
	case config.CaptureActionCopyAndSave:
		return 2
	case config.CaptureActionPin:
		return 3
	default:
		return 0
	}
}

func captureActionFromIndex(idx int) string {
	switch idx {
	case 1:
		return config.CaptureActionSave
	case 2:
		return config.CaptureActionCopyAndSave
	case 3:
		return config.CaptureActionPin
	default:
		return config.CaptureActionCopy
	}
}

func parseHexColor(s string) (core.ARGB, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) == 6 {
		v, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return 0, err
		}
		return core.ARGB(0xFF000000 | uint32(v)), nil
	}
	if len(s) == 8 {
		v, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return 0, err
		}
		return core.ARGB(uint32(v)), nil
	}
	return 0, fmt.Errorf("需要 #RRGGBB 或 #AARRGGBB")
}

func argbToUIColor(c core.ARGB) ui.Color {
	return ui.Color{R: c.R(), G: c.G(), B: c.B(), A: c.A()}
}

func uiColorToARGB(c ui.Color) core.ARGB {
	return core.ARGB(uint32(c.A)<<24 | uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B))
}

func formatUIColorHex(c ui.Color) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02X%02X%02X%02X", c.R, c.G, c.B, c.A)
}

// previewFileName expands pattern without mutating core's global file counter.
func previewFileName(pattern string) string {
	if strings.TrimSpace(pattern) == "" {
		pattern = core.DefaultCaptureFileName
	}
	now := time.Now()
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '%' || i+1 >= len(pattern) {
			b.WriteByte(pattern[i])
			continue
		}
		rest := pattern[i+1:]
		known := []string{"yyyyMMdd", "HHmmss", "counter", "yyyy", "MM", "dd", "HH", "mm", "ss"}
		matched := false
		for _, tok := range known {
			if strings.HasPrefix(rest, tok) {
				switch tok {
				case "yyyyMMdd":
					b.WriteString(now.Format("20060102"))
				case "HHmmss":
					b.WriteString(now.Format("150405"))
				case "counter":
					b.WriteByte('1')
				case "yyyy":
					fmt.Fprintf(&b, "%d", now.Year())
				case "MM":
					b.WriteString(now.Format("01"))
				case "dd":
					b.WriteString(now.Format("02"))
				case "HH":
					b.WriteString(now.Format("15"))
				case "mm":
					b.WriteString(now.Format("04"))
				case "ss":
					b.WriteString(now.Format("05"))
				}
				i += len(tok)
				matched = true
				break
			}
		}
		if !matched {
			b.WriteByte('%')
		}
	}
	out := b.String()
	for _, ch := range `<>:"/\|?*` {
		out = strings.ReplaceAll(out, string(ch), "_")
	}
	if !strings.HasSuffix(strings.ToLower(out), ".png") {
		out += ".png"
	}
	return out
}
