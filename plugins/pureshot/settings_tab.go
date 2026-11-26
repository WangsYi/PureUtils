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
	"pureutils/pkg/purekit/i18n"
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
	pureui.SettingCard(c, i18n.S("默认操作与文件保存", "Default Action & File Saving"), i18n.S("配置双击 / Enter 确认截图时的默认动作、默认保存目录与文件名模板", "Configure the default action on double-click / Enter, save folder, and filename template"), pal, func() {
		pureui.SettingBlock(c, i18n.S("双击 / Enter 默认操作", "Default Finish Action (Double-Click / Enter)"), i18n.S("在截图选区内双击、按 Enter 或点击“完成”时执行的默认动作", "Action performed on double-click, Enter, or clicking Done"), pal, func() {
			seg := ui.Segmented(c, &v.captureActionIdx, i18n.S("复制到剪贴板", "Copy"), i18n.S("保存到文件", "Save"), i18n.S("复制并保存", "Copy & Save"), i18n.S("贴到桌面", "Pin")).Label(i18n.S("截图默认完成操作", "Default capture action"))
			seg.Border(0, ui.Transparent)
			if seg.Changed() {
				v.draft.Capture.DefaultAction = captureActionFromIndex(v.captureActionIdx)
			}
		})
		ui.Divider(c)
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, i18n.S("保存目录", "Save Folder")).FontSize(13).FontWeight(500).TextColor(pal.Text)
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &v.draft.Capture.SaveFolder).Grow(1).MinWidth(0).Label(i18n.S("保存目录", "Save Folder")).Border(0, ui.Transparent).FocusRing(false)
				ui.Button(c, i18n.S("浏览…", "Browse…")).Shrink(0).OnClick(func() {
					if v.headless() {
						return
					}
					go func() {
						paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
							Title:       i18n.S("选择截图保存目录", "Select Screenshot Save Folder"),
							Directory:   true,
							DefaultPath: v.draft.Capture.SaveFolder,
						})
						mygo.RunOnMain(func() {
							if err != nil {
								v.status = i18n.S("选择目录失败: ", "Failed to select folder: ") + err.Error()
							} else if len(paths) > 0 {
								v.draft.Capture.SaveFolder = paths[0]
								v.status = i18n.S("已选择目录", "Folder selected")
							}
						})
					}()
				}).Border(0, ui.Transparent)
				if !v.headless() {
					ui.Button(c, i18n.S("打开目录", "Open Folder")).Shrink(0).OnClick(func() {
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
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i18n.S("文件名模板", "Filename Template")).FontSize(13).FontWeight(500).TextColor(pal.Text).Grow(1).MinWidth(0)
				pureui.PillBadge(c, i18n.S("预览: ", "Preview: ")+previewFileName(v.draft.Capture.FileNamePattern), pal.Accent, pal.AccentSoft)
			})
			ui.TextInput(c, &v.draft.Capture.FileNamePattern).Label(i18n.S("文件名模板", "Filename Template")).Border(0, ui.Transparent).FocusRing(false)
			ui.Text(c, i18n.S("支持占位符：%yyyyMMdd、%HHmmss、%yyyy、%MM、%dd、%HH、%mm、%ss、%counter", "Tokens: %yyyyMMdd, %HHmmss, %yyyy, %MM, %dd, %HH, %mm, %ss, %counter")).
				FontSize(11).TextColor(pal.TextMuted)
		})
	})

	pureui.SettingCard(c, i18n.S("行为与框选辅助", "Behavior & Selection Helpers"), i18n.S("区域截图在光标所在显示器铺开冻结帧；整屏截图直接复制并记录", "Region capture freezes the monitor under cursor; full-screen capture copies immediately"), pal, func() {
		pureui.SettingRow(c, i18n.S("截图后复制到剪贴板", "Copy to Clipboard After Capture"), i18n.S("完成或复制选区时将结果写入系统剪贴板", "Write result to clipboard when finishing or copying"), pal, func() {
			ui.Switch(c, &v.draft.Capture.CopyToClipboard).Label(i18n.S("开关：截图后复制到剪贴板", "Toggle: Copy to clipboard"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("截图后自动保存到目录", "Auto-Save to Folder"), i18n.S("每次完成截图自动落盘一份 PNG 到上方保存目录", "Automatically save a PNG copy to the save folder"), pal, func() {
			ui.Switch(c, &v.draft.Capture.AutoSave).Label(i18n.S("开关：截图后自动保存到目录", "Toggle: Auto-save capture"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("整屏截图包含鼠标指针", "Include Cursor in Fullscreen Shot"), i18n.S("执行截取当前显示器时绘制鼠标光标", "Draw mouse pointer when capturing active display"), pal, func() {
			ui.Switch(c, &v.draft.Capture.IncludeCursor).Label(i18n.S("开关：整屏截图包含鼠标指针", "Toggle: Include cursor"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("框选时吸附窗口边缘", "Snap Selection to Window Edges"), i18n.S("拖拽框选自动对齐桌面可见窗口边界，单击可快速捕获窗口", "Align drag handles to visible window edges; click to select window"), pal, func() {
			ui.Switch(c, &v.draft.Capture.SnapToWindows).Label(i18n.S("开关：框选时吸附窗口边缘", "Toggle: Snap to windows"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("显示放大镜与取色", "Show Magnifier & Color Picker"), i18n.S("光标旁展示像素放大镜、RGB/HEX 色值，按 C 复制色值", "Show pixel loupe with RGB/HEX near cursor; press C to copy color"), pal, func() {
			ui.Switch(c, &v.draft.Capture.ShowMagnifier).Label(i18n.S("开关：显示放大镜与取色", "Toggle: Show magnifier"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("显示选区尺寸", "Show Selection Dimensions"), i18n.S("在选区边角显示实时宽高像素尺寸标尺", "Show live width × height badge next to the selection"), pal, func() {
			ui.Switch(c, &v.draft.Capture.ShowRuler).Label(i18n.S("开关：显示选区尺寸", "Toggle: Show dimensions"))
		})
	})

	pureui.SettingCard(c, i18n.S("悬浮贴图设置", "Floating Pin Settings"), i18n.S("支持滚轮缩放及 Alt+滚轮微调单个贴图透明度", "Scroll to zoom pins; Alt+Scroll to adjust opacity"), pal, func() {
		pureui.SettingRow(c, i18n.S("新贴图默认置顶", "Always on Top by Default"), i18n.S("新建悬浮贴图时自动处于其他窗口上方", "Keep new pin windows above other windows"), pal, func() {
			ui.Switch(c, &v.draft.Pins.AlwaysOnTop).Label(i18n.S("开关：新贴图默认置顶", "Toggle: Pins always on top"))
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("默认不透明度", "Default Opacity"), i18n.S("新建贴图窗口的初始不透明度（20%–100%）", "Initial opacity for newly created pins (20%–100%)"), pal, func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Slider(c, &v.opacity, 0.2, 1).Width(150).Label(i18n.S("滑块：默认不透明度", "Slider: Default opacity")).Changed() {
					v.draft.Pins.Opacity = v.opacity
				}
				pureui.PillBadge(c, fmt.Sprintf("%.0f%%", v.opacity*100), pal.Accent, pal.AccentSoft)
			})
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("边框宽度", "Border Width"), i18n.S("贴图窗口外框线像素宽度（0 表示无边框）", "Outer border thickness in pixels (0 for borderless)"), pal, func() {
			if ui.NumberInput(c, &v.borderThickness, 0, 10, 1).Label(i18n.S("数值：边框宽度", "Number: Border width")).Changed() {
				v.draft.Pins.BorderThickness = int(v.borderThickness)
			}
		})
		ui.Divider(c)
		pureui.SettingRow(c, i18n.S("边框颜色", "Border Color"), i18n.S("点击色块呼出取色器，或直接输入十六进制色值", "Click swatch to pick a color or enter a hex value"), pal, func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				well := ui.ColorWell(c, &v.borderUIColor).Label(i18n.S("边框颜色选择器", "Border color picker"))
				if well.Changed() {
					v.borderColor = formatUIColorHex(v.borderUIColor)
					v.draft.Pins.BorderColor = uiColorToARGB(v.borderUIColor)
				}
				hexIn := ui.TextInput(c, &v.borderColor).Width(104).Placeholder("#3B7BFF").Label(i18n.S("边框颜色十六进制", "Border color hex"))
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

	pureui.SettingCard(c, i18n.S("实时效果预览", "Live Pin Preview"), i18n.S("按当前不透明度、边框宽度与颜色实时渲染悬浮贴图外观", "Live preview of floating pin appearance with current opacity and border"), pal, func() {
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
	return 0, fmt.Errorf("%s", i18n.S("需要 #RRGGBB 或 #AARRGGBB", "Expected #RRGGBB or #AARRGGBB"))
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
