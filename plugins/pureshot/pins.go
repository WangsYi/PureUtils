package pureshot

import (
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/platform"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/pureshot/annotate"
)

const (
	pinHUDHeight        = 36
	pinHUDStandardWidth = 208
	pinHUDOpacityWidth  = 246
	pinHUDGap           = 6
	pinHUDHideGrace     = 260 * time.Millisecond
)

type pinWindow struct {
	plugin *Plugin
	win *mygo.Window

	img    image.Image
	bitmap *ui.Bitmap
	text   string
	baseW  int
	baseH  int
	zoom   float64
	dark   bool

	natW   int
	natH   int
	width  int
	height int
	scale  float64

	opacity     float64
	alwaysOnTop bool
	passthrough bool
	closed      bool

	dragging        bool
	dragStartCursor mygo.Point
	dragStartBounds mygo.Rectangle

	hudWin        *mygo.Window
	hudVisible    bool
	hudW          int
	hudH          int
	hudAnchorW    int
	hudAnchorH    int
	pinHovered    bool
	hudHovered    bool
	hudSyncQueued atomic.Bool
	hudHideTimer  *time.Timer
}

// proportionalSize computes proportional dimensions from natW×natH at display
// scale and zoom factor, keeping the shorter side at least minSide when non-zero.
func proportionalSize(natW, natH int, scale, factor float64, minSide int) (int, int) {
	if natW <= 0 || natH <= 0 {
		return minSide, minSide
	}
	if scale <= 0 {
		scale = 1
	}
	w := float64(natW) / scale * factor
	h := float64(natH) / scale * factor
	if w < float64(minSide) || h < float64(minSide) {
		up := math.Max(float64(minSide)/math.Max(w, 1), float64(minSide)/math.Max(h, 1))
		w *= up
		h *= up
	}
	return int(math.Round(w)), int(math.Round(h))
}

// ShowPinImage creates a floating, always-on-top image window.
func (a *Plugin) ShowPinImage(img image.Image) {
	a.ShowPinImageAt(img, 0)
}

// ShowPinImageAt creates a floating, always-on-top image window staggered by cascade.
func (a *Plugin) ShowPinImageAt(img image.Image, cascade int) {
	if img == nil {
		return
	}
	if a.pinsHidden {
		a.TogglePins()
	}
	s := a.Config()
	b := img.Bounds()
	natW, natH := b.Dx(), b.Dy()
	scale := 1.0
	w, h := proportionalSize(natW, natH, scale, 1, 40)
	if a.headless() {
		p := &pinWindow{
			plugin: a, img: img,
			natW: natW, natH: natH, width: w, height: h, scale: scale,
			opacity: s.Pins.Opacity, alwaysOnTop: s.Pins.AlwaysOnTop,
		}
		a.pins[p] = true
		a.recordPin(pinnedRecord{IsImage: true, Width: natW, Height: natH, Cascade: cascade})
		a.rebuildTray()
		return
	}
	display := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	wa := display.WorkArea
	if display.ScaleFactor > 0 {
		scale = display.ScaleFactor
		w, h = proportionalSize(natW, natH, scale, 1, 40)
	}
	maxW, maxH := wa.Width*3/4, wa.Height*3/4
	if maxW > 0 && maxH > 0 && (w > maxW || h > maxH) {
		f := math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h))
		w, h = int(math.Round(float64(w)*f)), int(math.Round(float64(h)*f))
	}
	p := &pinWindow{
		plugin: a, img: img, bitmap: ui.NewBitmap(img),
		natW: natW, natH: natH, width: w, height: h, scale: scale,
		opacity: s.Pins.Opacity, alwaysOnTop: s.Pins.AlwaysOnTop,
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "贴图",
		Width:           w,
		Height:          h,
		Frameless:       true,
		DisableResize:   true,
		AlwaysOnTop:     p.alwaysOnTop,
		SkipTaskbar:     true,
		CornerRadius:    10,
		BackgroundColor: "#000000",
		Content:         ui.View(p.view),
	})
	p.win = win
	win.OnClosed(func() {
		p.closed = true
		p.closeHUD()
		delete(a.pins, p)
		if len(a.pins) == 0 {
			a.pinsHidden = false
			a.resetPinSequence()
		}
		a.rebuildTray()
		a.notifyWindowClosed()
	})
	a.pins[p] = true
	a.recordPin(pinnedRecord{IsImage: true, Width: natW, Height: natH, Cascade: cascade})
	p.placeNearCursor(cascade)
	win.Show()
	win.SetOpacity(p.opacity)
	win.SetAlwaysOnTop(p.alwaysOnTop)
	a.rebuildTray()
	if a.log() != nil {
		a.log().Infof("贴图 %dx%d", natW, natH)
	}
}

// ShowPinText creates a floating, always-on-top text card window.
func (a *Plugin) ShowPinText(text string) {
	a.ShowPinTextAt(text, 0)
}

// ShowPinTextAt creates a floating text card window staggered by cascade.
func (a *Plugin) ShowPinTextAt(text string, cascade int) {
	norm := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if strings.TrimSpace(norm) == "" {
		return
	}
	if a.pinsHidden {
		a.TogglePins()
	}
	s := a.Config()
	cw, ch := annotate.MeasureTextCard(norm, 13, 500)
	w, h := int(math.Round(cw)), int(math.Round(ch))
	if a.headless() {
		p := &pinWindow{
			plugin: a, text: norm, baseW: w, baseH: h, zoom: 1.0,
			width: w, height: h, scale: 1.0,
			opacity: s.Pins.Opacity, alwaysOnTop: s.Pins.AlwaysOnTop,
		}
		a.pins[p] = true
		a.recordPin(pinnedRecord{IsImage: false, Text: norm, Width: w, Height: h, Cascade: cascade})
		a.rebuildTray()
		return
	}
	display := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	wa := display.WorkArea
	scale := display.ScaleFactor
	if scale <= 0 {
		scale = 1
	}
	maxW, maxH := wa.Width*3/4, wa.Height*3/4
	if maxW > 180 && w > maxW {
		w = maxW
	}
	if maxH > 64 && h > maxH {
		h = maxH
	}
	bgHex := "#1A1E28"
	if a.host != nil && strings.EqualFold(a.host.HostConfig().Appearance.Theme, "light") {
		bgHex = "#FFFFFF"
	}
	p := &pinWindow{
		plugin: a, text: norm, baseW: w, baseH: h, zoom: 1.0,
		width: w, height: h, scale: scale,
		opacity: s.Pins.Opacity, alwaysOnTop: s.Pins.AlwaysOnTop,
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "贴图",
		Width:           w,
		Height:          h,
		Frameless:       true,
		DisableResize:   true,
		AlwaysOnTop:     p.alwaysOnTop,
		SkipTaskbar:     true,
		CornerRadius:    10,
		BackgroundColor: bgHex,
		Content:         ui.View(p.view),
	})
	p.win = win
	win.OnClosed(func() {
		p.closed = true
		p.closeHUD()
		delete(a.pins, p)
		if len(a.pins) == 0 {
			a.pinsHidden = false
			a.resetPinSequence()
		}
		a.rebuildTray()
		a.notifyWindowClosed()
	})
	a.pins[p] = true
	a.recordPin(pinnedRecord{IsImage: false, Text: norm, Width: w, Height: h, Cascade: cascade})
	p.placeNearCursor(cascade)
	win.Show()
	win.SetOpacity(p.opacity)
	win.SetAlwaysOnTop(p.alwaysOnTop)
	a.rebuildTray()
	if a.log() != nil {
		a.log().Infof("文字贴图 %dx%d (%d 字符)", w, h, len([]rune(norm)))
	}
}

func (p *pinWindow) placeNearCursor(cascade int) {
	if p.win == nil {
		return
	}
	c := mygo.Screen.CursorScreenPoint()
	d := mygo.Screen.DisplayNearestPoint(c)
	wa := d.WorkArea
	offset := (cascade % 8) * 28
	x := int(c.X) + 24 + offset
	y := int(c.Y) + 24 + offset
	if x+p.width > wa.X+wa.Width {
		x = int(c.X) - p.width - 24 + (cascade%4)*20
	}
	if y+p.height > wa.Y+wa.Height {
		y = int(c.Y) - p.height - 24 + (cascade%4)*20
	}
	x = clampInt(x, wa.X+4, wa.X+wa.Width-p.width-4)
	y = clampInt(y, wa.Y+4, wa.Y+wa.Height-p.height-4)
	p.win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: p.width, Height: p.height})
}

func (p *pinWindow) view(c *ui.Context) {
	if p.closed {
		return
	}
	isText := p.text != ""
	var pal pureui.Palette
	if isText {
		pal = pureui.ApplyTheme(c)
	} else {
		pal = pureui.PaletteFor(true)
	}
	p.dark = pal.Dark

	if c.Shortcut(0, ui.KeyEscape) {
		p.closeWindow()
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) {
		p.copy()
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) {
		p.saveAs()
	}
	if c.Shortcut(0, ui.Key0) {
		p.actualSize()
	}
	if c.Shortcut(0, ui.KeyEqual) {
		p.zoomBy(1.12)
	}
	if c.Shortcut(0, ui.KeyMinus) {
		p.zoomBy(1 / 1.12)
	}

	root := ui.Box(c).Fill().Cursor(ui.CursorMove).HandleInput(p.handleInput)
	root.ContextMenu(func(m *ui.Menu) {
		if isText {
			if m.Item("复制文本").Chosen() {
				p.copy()
			}
			if m.Item("复制为图片").Chosen() {
				p.copyAsImage()
			}
		} else {
			if m.Item("复制图片").Chosen() {
				p.copy()
			}
		}
		if m.Item("另存为…").Chosen() {
			p.saveAs()
		}
		if m.Item("实际大小").Chosen() {
			p.actualSize()
		}
		if !isText && p.plugin != nil && p.plugin.canPreviewImage() && p.img != nil {
			if m.Item("在 PureImage 中查看").Chosen() {
				_ = purekit.CallImagePreviewImage(context.Background(), p.plugin.host.Bus(), "贴图预览", p.img)
			}
		}
		m.Separator()
		if m.Item("鼠标穿透").Checked(p.passthrough).Chosen() {
			p.setPassthrough(!p.passthrough)
		}
		if m.Item("置顶").Checked(p.alwaysOnTop).Chosen() {
			p.setAlwaysOnTop(!p.alwaysOnTop)
		}
		m.Separator()
		if m.Item("关闭").Shortcut(0, ui.KeyEscape).Chosen() {
			p.closeWindow()
		}
	})

	showHUD := root.Hovered()
	if showHUD != p.pinHovered {
		p.pinHovered = showHUD
		p.scheduleHUDSync()
	}
	hudAlpha := root.Animate("hud", boolFloat(p.pinHovered || p.hudHovered), 150*time.Millisecond)

	root.Draw(func(painter *ui.Painter, r ui.Rect) {
		if isText {
			painter.Fill(r, pal.CardBg, 0)
			painter.FillGradient(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 3}, ui.LinearGradient{
				From: pal.Accent, To: pal.AccentEnd, Angle: 90,
			}, 0)
			border := p.plugin.Config().Pins.BorderThickness
			if border > 0 {
				col := p.plugin.Config().Pins.BorderColor
				painter.Stroke(r, ui.RGBA(col.R(), col.G(), col.B(), float32(col.A())/255), 0, float32(border))
			} else {
				painter.Stroke(r, pal.BorderStrong, 0, 1)
			}
		} else {
			painter.Fill(r, ui.RGB(0, 0, 0), 0)
			if p.bitmap != nil {
				painter.Image(p.bitmap, r, ui.Contain)
			}
			border := p.plugin.Config().Pins.BorderThickness
			if border > 0 {
				col := p.plugin.Config().Pins.BorderColor
				painter.Stroke(r, ui.RGBA(col.R(), col.G(), col.B(), float32(col.A())/255), 0, float32(border))
			}
		}
		if hudAlpha > 0.02 {
			painter.Stroke(ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}, ui.RGBA(255, 255, 255, 0.22*hudAlpha), 0, 1)
		}
	})

	root.Children(func() {
		if isText {
			z := p.zoom
			if z <= 0 {
				z = 1
			}
			fs := clampf(float32(13*z), 9, 42)
			ui.Column(c).Fill().Padding(14, 16, 12, 16).PassThrough().Clip().Children(func() {
				ui.Text(c, p.text).FontSize(fs).LineHeight(1.50).TextColor(pal.Text).PassThrough()
			})
		}
	})
}

func (p *pinWindow) hudBadgeText() string {
	zoomPct := p.zoomPercent()
	if p.opacity < 0.995 {
		return fmt.Sprintf("%d%% · %d%%", zoomPct, int(math.Round(p.opacity*100)))
	}
	return fmt.Sprintf("%d%%", zoomPct)
}

func (p *pinWindow) hudSize() (int, int) {
	if p.opacity < 0.995 {
		return pinHUDOpacityWidth, pinHUDHeight
	}
	return pinHUDStandardWidth, pinHUDHeight
}

func (p *pinWindow) hudView(c *ui.Context) {
	if p.closed {
		return
	}
	pal := pureui.PaletteFor(true)
	isText := p.text != ""
	copyLabel := "复制图片"
	copyTip := "复制图片 (Ctrl+C)"
	if isText {
		copyLabel = "复制文本"
		copyTip = "复制文本 (Ctrl+C)"
	}

	zoomPct := p.zoomPercent()
	badge := p.hudBadgeText()
	dotCol := pal.HudMuted
	if zoomPct != 100 || p.opacity < 0.995 {
		dotCol = pal.AccentHover
	}

	row := ui.Row(c).Fill().Padding(4, 6).Gap(4).Radius(999).AlignItems(ui.Center).
		Background(ui.RGBA(16, 19, 27, 0.94)).
		Border(1, pal.HudBorder).
		InsetShadow(0, 1, 0, 0, pal.HudHighlight)
	hovered := row.Hovered()
	if hovered != p.hudHovered {
		p.hudHovered = hovered
		p.scheduleHUDSync()
	}

	row.Children(func() {
		ui.Row(c).Padding(0, 4, 0, 5).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
			ui.Box(c).Size(5, 5).Radius(2.5).Background(dotCol)
			ui.Text(c, badge).FontSize(10.5).FontWeight(600).SingleLine().TextColor(pal.HudText)
		})
		ui.Box(c).Width(1).Height(14).Background(pal.HudDivider).Shrink(0)
		ui.Spacer(c)
		ui.Row(c).Gap(3).AlignItems(ui.Center).Shrink(0).Children(func() {
			pureui.HudIconButton(c, pureui.IconPin, "置顶", "置顶 / 取消置顶", p.alwaysOnTop, false, false, pal).
				Size(26, 26).Radius(999).OnClick(func() {
				p.setAlwaysOnTop(!p.alwaysOnTop)
			})
			pureui.HudIconButton(c, pureui.IconCopy, copyLabel, copyTip, false, false, false, pal).
				Size(26, 26).Radius(999).OnClick(p.copy)
			pureui.HudIconButton(c, pureui.IconSave, "另存为", "另存为… (Ctrl+S)", false, false, false, pal).
				Size(26, 26).Radius(999).OnClick(p.saveAs)
			ui.Box(c).Width(1).Height(14).Background(pal.HudDivider)
			pureui.HudIconButton(c, pureui.IconClose, "关闭", "关闭贴图 (双击 / Esc)", false, false, true, pal).
				Size(26, 26).Radius(999).OnClick(p.closeWindow)
		})
	})
}

func (p *pinWindow) cancelHUDHideTimer() {
	if p.hudHideTimer != nil {
		p.hudHideTimer.Stop()
		p.hudHideTimer = nil
	}
}

func (p *pinWindow) hideHUDImmediate() {
	p.cancelHUDHideTimer()
	p.hudHovered = false
	p.hudVisible = false
	if p.hudWin != nil && (p.plugin == nil || !p.plugin.headless()) && p.hudWin.IsVisible() {
		p.hudWin.Hide()
	}
}

func (p *pinWindow) closeHUD() {
	p.cancelHUDHideTimer()
	p.hudHovered = false
	p.hudVisible = false
	p.hudW, p.hudH, p.hudAnchorW, p.hudAnchorH = 0, 0, 0, 0
	if w := p.hudWin; w != nil {
		p.hudWin = nil
		if p.plugin == nil || !p.plugin.headless() {
			w.Close()
		}
	}
}

func (p *pinWindow) scheduleHUDSync() {
	if p.closed || p.win == nil || (p.plugin != nil && p.plugin.headless()) {
		return
	}
	if p.hudSyncQueued.Swap(true) {
		return
	}
	go func() {
		mygo.RunOnMain(func() {
			p.hudSyncQueued.Store(false)
			p.syncHUD()
		})
	}()
}

func (p *pinWindow) syncHUD() {
	if p.closed || p.win == nil || p.passthrough || p.dragging || (p.plugin != nil && (p.plugin.headless() || p.plugin.pinsHidden)) {
		p.hideHUDImmediate()
		return
	}
	if p.pinHovered || p.hudHovered {
		p.cancelHUDHideTimer()
		hw, hh := p.hudSize()
		anchor := mygo.Rectangle{X: 0, Y: 0, Width: p.width, Height: p.height}
		if p.hudWin == nil {
			fw := mygo.NewFlyout(mygo.FlyoutOptions{
				Parent:       p.win,
				Anchor:       anchor,
				Placement:    mygo.PlacementTop,
				Gap:          pinHUDGap,
				Width:        hw,
				Height:       hh,
				Focusable:    false,
				Hidden:       true,
				Shadow:       false,
				CornerRadius: hh / 2,
				Content:      ui.View(p.hudView),
			})
			fw.OnClose(func(_ *mygo.CloseEvent) {
				if p.hudWin == fw {
					p.hudWin = nil
					p.hudVisible = false
					p.hudHovered = false
					p.hudW, p.hudH, p.hudAnchorW, p.hudAnchorH = 0, 0, 0, 0
				}
			})
			p.hudWin = fw
			p.hudW, p.hudH = hw, hh
			p.hudAnchorW, p.hudAnchorH = p.width, p.height
		} else {
			if p.hudW != hw || p.hudH != hh {
				p.hudWin.SetSize(hw, hh)
				p.hudW, p.hudH = hw, hh
			}
			if p.hudAnchorW != p.width || p.hudAnchorH != p.height {
				p.hudWin.SetAnchor(anchor)
				p.hudAnchorW, p.hudAnchorH = p.width, p.height
			}
		}
		if !p.hudVisible || !p.hudWin.IsVisible() {
			p.hudWin.SetAnchor(anchor)
			p.hudAnchorW, p.hudAnchorH = p.width, p.height
			p.hudWin.ShowInactive()
			p.hudWin.SetAlwaysOnTop(p.alwaysOnTop)
			p.hudVisible = true
		}
		p.hudWin.Invalidate()
		return
	}

	if !p.hudVisible || p.hudWin == nil {
		return
	}
	if p.hudHideTimer == nil {
		p.hudHideTimer = time.AfterFunc(pinHUDHideGrace, func() {
			mygo.RunOnMain(func() {
				p.hudHideTimer = nil
				if p.closed || p.pinHovered || p.hudHovered {
					return
				}
				p.hideHUDImmediate()
			})
		})
	}
}

func boolFloat(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func (p *pinWindow) zoomPercent() int {
	if p.text != "" {
		z := p.zoom
		if z <= 0 {
			z = 1
		}
		return int(math.Round(z * 100))
	}
	if p.natW <= 0 {
		return 100
	}
	scale := p.scale
	if scale <= 0 {
		scale = 1
	}
	baseW := float64(p.natW) / scale
	if baseW <= 0 {
		return 100
	}
	return int(math.Round(float64(p.width) / baseW * 100))
}

func (p *pinWindow) handleInput(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		if ev.Clicks >= 2 {
			p.dragging = false
			p.closeWindow()
			return true
		}
		if p.win != nil {
			p.dragging = true
			p.hideHUDImmediate()
			p.dragStartCursor = mygo.Screen.CursorScreenPoint()
			p.dragStartBounds = p.win.Bounds()
		}
		return true
	case ui.InputPointerMove:
		if p.dragging && p.win != nil {
			c := mygo.Screen.CursorScreenPoint()
			dx, dy := int(c.X)-int(p.dragStartCursor.X), int(c.Y)-int(p.dragStartCursor.Y)
			p.win.SetBounds(mygo.Rectangle{
				X: p.dragStartBounds.X + dx, Y: p.dragStartBounds.Y + dy,
				Width: p.width, Height: p.height,
			})
			return true
		}
		return false
	case ui.InputPointerUp:
		if p.dragging {
			p.dragging = false
			p.scheduleHUDSync()
			return true
		}
		return false
	case ui.InputScroll:
		if ev.Mods&ui.Alt != 0 {
			p.adjustOpacity(-float64(ev.DY) * 0.001)
			return true
		}
		factor := 1 - float64(ev.DY)*0.005
		if factor < 0.5 {
			factor = 0.5
		}
		if factor > 2 {
			factor = 2
		}
		p.zoomBy(factor)
		return true
	}
	return false
}

func (p *pinWindow) adjustOpacity(delta float64) {
	p.opacity = math.Max(0.2, math.Min(1, p.opacity+delta))
	if p.win != nil {
		p.win.SetOpacity(p.opacity)
	}
	p.invalidate()
	if p.hudVisible {
		p.scheduleHUDSync()
	}
	if p.hudWin != nil && (p.plugin == nil || !p.plugin.headless()) {
		p.hudWin.Invalidate()
	}
}

func (p *pinWindow) zoomBy(factor float64) {
	if factor <= 0 {
		return
	}
	if p.text != "" {
		z := p.zoom
		if z <= 0 {
			z = 1
		}
		nextZ := math.Max(0.5, math.Min(3.0, z*factor))
		p.zoom = nextZ
		bw, bh := max(p.baseW, 180), max(p.baseH, 64)
		w := max(120, int(math.Round(float64(bw)*nextZ)))
		h := max(48, int(math.Round(float64(bh)*nextZ)))
		p.setBoundsSize(w, h)
		return
	}
	var w, h int
	if p.natW > 0 && p.natH > 0 {
		newW := float64(p.width) * factor
		newH := newW * float64(p.natH) / float64(p.natW)
		if newW < 48 || newH < 48 {
			up := math.Max(48/math.Max(newW, 1), 48/math.Max(newH, 1))
			newW *= up
			newH *= up
		}
		maxW, maxH := 3840.0, 2160.0
		if p.win != nil {
			d := mygo.Screen.DisplayNearestPoint(mygo.Point{X: p.win.Bounds().X, Y: p.win.Bounds().Y})
			if d.WorkArea.Width > 0 && d.WorkArea.Height > 0 {
				maxW = float64(d.WorkArea.Width) * 1.5
				maxH = float64(d.WorkArea.Height) * 1.5
			}
		}
		if newW > maxW || newH > maxH {
			down := math.Min(maxW/newW, maxH/newH)
			newW *= down
			newH *= down
		}
		w, h = int(math.Round(newW)), int(math.Round(newH))
	} else {
		w = max(48, int(math.Round(float64(p.width)*factor)))
		h = max(48, int(math.Round(float64(p.height)*factor)))
	}
	p.setBoundsSize(w, h)
}

func (p *pinWindow) actualSize() {
	if p.text != "" {
		p.zoom = 1.0
		bw, bh := max(p.baseW, 180), max(p.baseH, 64)
		p.setBoundsSize(bw, bh)
		return
	}
	w, h := proportionalSize(p.natW, p.natH, p.scale, 1, 40)
	p.setBoundsSize(w, h)
}

func (p *pinWindow) setBoundsSize(w, h int) {
	p.width, p.height = w, h
	if p.win != nil {
		b := p.win.Bounds()
		p.win.SetBounds(mygo.Rectangle{X: b.X, Y: b.Y, Width: w, Height: h})
	}
	p.invalidate()
	if p.hudVisible {
		p.scheduleHUDSync()
	}
}

func (p *pinWindow) setPassthrough(v bool) {
	p.applyPassthrough(v)
	if p.plugin != nil {
		p.plugin.rebuildTray()
	}
}

func (p *pinWindow) applyPassthrough(v bool) {
	p.passthrough = v
	if v {
		p.hideHUDImmediate()
	}
	if p.win != nil {
		p.win.SetIgnoreMouseEvents(v)
	}
	p.invalidate()
}

func (p *pinWindow) setAlwaysOnTop(v bool) {
	p.alwaysOnTop = v
	if p.win != nil {
		p.win.SetAlwaysOnTop(v)
	}
	if p.hudWin != nil && (p.plugin == nil || !p.plugin.headless()) {
		p.hudWin.SetAlwaysOnTop(v)
		p.hudWin.Invalidate()
	}
	p.invalidate()
}

func (p *pinWindow) closeWindow() {
	p.closed = true
	p.closeHUD()
	if p.win != nil {
		p.win.Close()
	}
}

func (p *pinWindow) invalidate() {
	if p.win != nil {
		p.win.Invalidate()
	}
}

func (p *pinWindow) copy() {
	if p.plugin == nil {
		return
	}
	if p.text != "" {
		if err := p.plugin.copyPinText(p.text); err != nil {
			p.plugin.notify("复制文本失败", err.Error())
			return
		}
		p.plugin.notify("文本已复制", fmt.Sprintf("%d 字符", len([]rune(p.text))))
		return
	}
	if p.img == nil {
		return
	}
	data, err := annotate.EncodePNG(p.img)
	if err != nil {
		p.plugin.notify("复制贴图失败", err.Error())
		return
	}
	if err := p.plugin.copyPinImage(data); err != nil {
		p.plugin.notify("复制贴图失败", err.Error())
		return
	}
	p.plugin.notify("贴图已复制", fmt.Sprintf("%d × %d", p.natW, p.natH))
}

func (p *pinWindow) copyAsImage() {
	if p.text == "" || p.plugin == nil {
		return
	}
	card := annotate.RenderTextCard(p.text, p.dark)
	data, err := annotate.EncodePNG(card)
	if err != nil {
		p.plugin.notify("生成图片失败", err.Error())
		return
	}
	if err := p.plugin.copyPinImage(data); err != nil {
		p.plugin.notify("复制图片失败", err.Error())
		return
	}
	b := card.Bounds()
	p.plugin.notify("已复制为图片", fmt.Sprintf("%d × %d", b.Dx(), b.Dy()))
}

func (p *pinWindow) saveAs() {
	if p.win == nil || (p.img == nil && p.text == "") {
		return
	}
	isText := p.text != ""
	dark := p.dark
	go func() {
		opts := mygo.SaveDialogOptions{
			Parent: p.win, Title: "保存贴图", DefaultPath: "pin.png",
			Filters: []mygo.FileFilter{{Name: "PNG 图片", Extensions: []string{"png"}}},
		}
		if isText {
			opts.Filters = []mygo.FileFilter{
				{Name: "PNG 图片", Extensions: []string{"png"}},
				{Name: "文本文件", Extensions: []string{"txt"}},
			}
		}
		path, err := mygo.Dialog.Save(opts)
		if err != nil {
			mygo.RunOnMain(func() { p.plugin.notify("保存失败", err.Error()) })
			return
		}
		if path == "" {
			return
		}
		var data []byte
		if isText && strings.EqualFold(filepath.Ext(path), ".txt") {
			data = []byte(p.text)
		} else if isText {
			data, err = annotate.EncodePNG(annotate.RenderTextCard(p.text, dark))
		} else {
			data, err = annotate.EncodePNG(p.img)
		}
		if err == nil {
			if dir := filepath.Dir(path); dir != "" {
				err = os.MkdirAll(dir, 0o755)
			}
			if err == nil {
				err = os.WriteFile(path, data, 0o600)
			}
		}
		mygo.RunOnMain(func() {
			if err != nil {
				p.plugin.notify("保存失败", err.Error())
			} else {
				p.plugin.notify("贴图已保存", path)
			}
		})
	}()
}

// TogglePins hides or shows every pin.
func (a *Plugin) TogglePins() {
	if len(a.pins) == 0 {
		return
	}
	a.pinsHidden = !a.pinsHidden
	for p := range a.pins {
		if p.win == nil {
			continue
		}
		if a.pinsHidden {
			p.hideHUDImmediate()
			p.win.Hide()
		} else {
			p.win.Show()
			p.win.SetAlwaysOnTop(p.alwaysOnTop)
		}
	}
	a.rebuildTray()
}

func (a *Plugin) passthroughPinCount() int {
	n := 0
	for p := range a.pins {
		if p.passthrough {
			n++
		}
	}
	return n
}

// SetPinsPassthrough sets mouse passthrough on every pin.
func (a *Plugin) SetPinsPassthrough(v bool) {
	for p := range a.pins {
		p.applyPassthrough(v)
	}
	a.rebuildTray()
}

type resolvedPin struct {
	img  image.Image
	text string
}

func decodeImageFile(path string) (image.Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return platform.DecodeImageBytes(b)
}

func resolvePinFromContent(content *core.ClipboardContent) (resolvedPin, bool) {
	if content == nil || content.IsEmpty() {
		return resolvedPin{}, false
	}
	if len(content.ImagePNG) > 0 {
		if img, err := platform.DecodeImageBytes(content.ImagePNG); err == nil {
			return resolvedPin{img: img}, true
		}
	}
	if len(content.Files) > 0 {
		for _, p := range content.Files {
			if !fileExists(p) {
				continue
			}
			if img, err := decodeImageFile(p); err == nil {
				return resolvedPin{img: img}, true
			}
		}
		joined := strings.TrimSpace(strings.Join(content.Files, "\n"))
		if joined != "" {
			return resolvedPin{text: joined}, true
		}
	}
	text := content.Text
	if text == "" && content.HTML != "" {
		text = core.StripHTML(content.HTML)
	}
	if text == "" && content.RTF != "" {
		text = core.StripRTF(content.RTF)
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return resolvedPin{}, false
	}
	if !strings.ContainsAny(trimmed, "\r\n") && fileExists(trimmed) {
		if img, err := decodeImageFile(trimmed); err == nil {
			return resolvedPin{img: img}, true
		}
	}
	return resolvedPin{text: text}, true
}

func (a *Plugin) resetPinSequence() {
	a.pinSeqMu.Lock()
	a.pinQueue = nil
	a.pinQueuePos = 0
	a.pinCascade = 0
	a.pinSeqMu.Unlock()
}

// nextPinFromHistory queries CapClipboardHistory via the Capability Bus without
// importing PureClip. Returns (content, cascade, exhausted, usedHistory, ok).
func (a *Plugin) nextPinFromHistory() (content *core.ClipboardContent, cascade int, exhausted bool, usedHistory bool, ok bool) {
	if a.host == nil || a.host.Bus() == nil || !a.host.Bus().Has(purekit.CapClipboardHistory) {
		return nil, 0, false, false, false
	}
	a.ensureHistorySubscription()

	ctx := context.Background()
	bus := a.host.Bus()
	_ = purekit.CallClipboardHistoryFlush(ctx, bus)

	a.pinSeqMu.Lock()
	if a.pinQueue == nil {
		res, err := purekit.CallClipboardHistoryListRecent(ctx, bus, 200)
		if err != nil || !res.Active || !res.Monitoring || len(res.Entries) == 0 {
			a.pinSeqMu.Unlock()
			return nil, 0, false, res.Active && res.Monitoring, false
		}
		q := make([]int64, len(res.Entries))
		for i, r := range res.Entries {
			q[i] = r.ID
		}
		a.pinQueue = q
		a.pinQueuePos = 0
	}
	for a.pinQueuePos < len(a.pinQueue) {
		id := a.pinQueue[a.pinQueuePos]
		cas := a.pinCascade
		a.pinQueuePos++
		a.pinCascade++
		a.pinSeqMu.Unlock()

		loaded, err := purekit.CallClipboardHistoryLoadContent(ctx, bus, id)
		if err == nil && loaded != nil && !loaded.IsEmpty() {
			return loaded, cas, false, true, true
		}

		a.pinSeqMu.Lock()
	}
	hasQueue := len(a.pinQueue) > 0
	a.pinSeqMu.Unlock()
	if hasQueue {
		return nil, 0, true, true, false
	}
	return nil, 0, false, true, false
}

// PinClipboard sequentially pins clipboard history entries from newest to oldest
// when CapClipboardHistory is active; falls back cleanly to host.Clipboard().Read().
func (a *Plugin) PinClipboard() {
	if a.headless() {
		a.PinClipboardSync()
		return
	}
	go a.PinClipboardSync()
}

// PinClipboardSync performs one sequential pin or live-clipboard fallback pin.
func (a *Plugin) PinClipboardSync() {
	a.pinExecMu.Lock()
	defer a.pinExecMu.Unlock()

	runPin := func(res resolvedPin, cascade int) {
		if a.headless() {
			if res.img != nil {
				a.ShowPinImageAt(res.img, cascade)
			} else {
				a.ShowPinTextAt(res.text, cascade)
			}
			return
		}
		mygo.RunOnMain(func() {
			if res.img != nil {
				a.ShowPinImageAt(res.img, cascade)
			} else {
				a.ShowPinTextAt(res.text, cascade)
			}
		})
	}
	notifyUI := func(title, body string) {
		if a.headless() {
			a.notify(title, body)
			return
		}
		mygo.RunOnMain(func() { a.notify(title, body) })
	}

	if content, cascade, exhausted, _, ok := a.nextPinFromHistory(); ok {
		if res, rok := resolvePinFromContent(content); rok {
			runPin(res, cascade)
			return
		}
	} else if exhausted {
		notifyUI("已贴出全部历史记录", "继续复制新内容，或关闭贴图即可重新从最新一条开始。")
		return
	}

	if a.host != nil && a.host.Clipboard() != nil {
		if content, err := a.host.Clipboard().Read(); err == nil && content != nil {
			if res, ok := resolvePinFromContent(content); ok {
				runPin(res, 0)
				return
			}
		}
	}
	notifyUI("剪贴板里没有可贴的内容", "先复制图片、文本或文件路径。")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}