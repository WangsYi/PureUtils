package pureshot

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"

	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/pkg/purekit/platform"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/pureshot/annotate"
)

type overlayMode int

const (
	modeNone overlayMode = iota
	modeSelect
	modeResize
	modeMove
	modeAnnotate
)

type resizeHandle int

const (
	handleNone resizeHandle = iota
	handleNW
	handleN
	handleNE
	handleE
	handleSE
	handleS
	handleSW
	handleW
	handleMove
)

type captureSession struct {
	plugin  *Plugin
	win     *mygo.Window
	frame   *image.RGBA
	monitor platform.Monitor
	display mygo.Display

	shotBitmap     *ui.Bitmap
	magnifier      *ui.Bitmap
	magnifierColor color.RGBA

	contentW, contentH float64
	cursor             core.Point

	selection     core.RectI
	origSelection core.RectI
	doc           *annotate.Document

	tool        annotate.Tool
	color       core.ARGB
	thickness   float64
	filled      bool
	textHasBg   bool
	fontSize    float64
	mosaicBlock float64

	mode              overlayMode
	activeHandle      resizeHandle
	hoverHandle       resizeHandle
	pressRaw          core.Point
	dragStart         core.Point
	dragCurrent       core.Point
	moveOrigin        core.Point
	draft             *annotate.Item
	badgeAddedOnClick bool

	textEditing bool
	textOrigin  core.Point
	textValue   string

	windows       []core.RectI
	hoveredWindow core.RectI

	dockW, dockH float32
	status       string
	finishing    bool
	closed       bool
}

// StartRegionCapture freezes the display under the cursor and opens the
// full-screen selection/annotation overlay.
func (a *Plugin) StartRegionCapture() {
	if a.overlay != nil {
		if a.overlay.win != nil {
			a.overlay.win.Focus()
		}
		return
	}
	_ = platform.EnsurePerMonitorV2()
	mon, err := platform.MonitorAtCursor()
	if err != nil {
		if a.log() != nil { a.log().Errorf("区域截图：找不到显示器: %v", err) }
		return
	}
	display := a.displayForMonitor(mon)
	frame, err := a.capture.CaptureMonitor(mon, false)
	if err != nil {
		if a.log() != nil { a.log().Errorf("区域截图：冻结屏幕失败: %v", err) }
		a.notify(i18n.S("截图失败", "Capture Failed"), err.Error())
		return
	}
	curPos, curErr := platform.CursorPosition()
	if curErr != nil || !mon.Bounds.Contains(image.Pt(int(curPos.X), int(curPos.Y))) {
		curPos = core.Point{
			X: float64(mon.Bounds.X) + float64(mon.Bounds.W)/2,
			Y: float64(mon.Bounds.Y) + float64(mon.Bounds.H)/2,
		}
	}
	windows := platform.VisibleWindowRects(mon.Bounds)
	s := &captureSession{
		plugin: a, frame: frame, monitor: mon, display: display,
		tool: annotate.ToolRect, color: core.DefaultStrokeColor,
		thickness:   core.DefaultThickness,
		fontSize:    core.DefaultFontSize,
		mosaicBlock: core.DefaultMosaicBlock,
		cursor:      curPos,
		contentW:    float64(display.Bounds.Width),
		contentH:    float64(display.Bounds.Height),
		windows:     windows,
	}
	s.hoveredWindow = s.hitWindow(curPos)
	s.shotBitmap = ui.NewBitmap(frame)
	mag := image.NewRGBA(image.Rect(0, 0, 120, 120))
	s.magnifier = ui.NewBitmap(mag)
	s.magnifier.Pixelated()
	s.updateMagnifier(curPos)

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:             i18n.S("截图", "Capture"),
		Width:             display.Bounds.Width,
		Height:            display.Bounds.Height,
		X:                 display.Bounds.X,
		Y:                 display.Bounds.Y,
		Hidden:            true,
		Frameless:         true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		DisableResize:     true,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		DisableShadow:     true,
		BackgroundColor:   "#000000",
		Content:           ui.View(s.view),
	})
	s.win = win
	win.OnClose(func(_ *mygo.CloseEvent) {
		s.closed = true
		if a.overlay == s {
			a.overlay = nil
			a.overlayActive.Store(false)
			if a.host != nil {
				a.host.SetGlobalHookSuppressed("pureshot:overlay", false)
			}
		}
		a.notifyWindowClosed()
	})
	a.overlay = s
	a.overlayActive.Store(true)
	if a.host != nil {
		a.host.SetGlobalHookSuppressed("pureshot:overlay", true)
	}
	win.SetBounds(mygo.Rectangle{
		X: display.Bounds.X, Y: display.Bounds.Y,
		Width: display.Bounds.Width, Height: display.Bounds.Height,
	})
	platform.SetWindowBoundsPhysical(platform.HWND(win.NativeHandle()), mon.Bounds, true)
	win.Show()
	win.Focus()
	win.SetAlwaysOnTop(true)
	win.SetIgnoreMouseEvents(false)
}

func (a *Plugin) displayForMonitor(m platform.Monitor) mygo.Display {
	for _, d := range mygo.Screen.Displays() {
		if d.Label == m.Device {
			return d
		}
	}
	return mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
}

func applyDarkOverlayTheme(c *ui.Context) pureui.Palette {
	pal := pureui.PaletteFor(true)
	base := ui.DarkTheme()
	t := *base
	t.Dark = true
	t.Background = pal.Bg
	t.Surface = pal.CardBg
	t.SurfaceHover = pal.CardHover
	t.SurfacePressed = pal.CardActive
	t.Border = pal.Border
	t.Text = pal.Text
	t.TextMuted = pal.TextMuted
	t.Accent = pal.Accent
	t.AccentHover = pal.AccentHover
	t.AccentPressed = pal.AccentPressed
	t.AccentText = pal.AccentText
	t.Radius = 8
	t.Spacing = 4
	c.SetTheme(&t)
	return pal
}

func (s *captureSession) view(c *ui.Context) {
	if s.closed {
		return
	}
	pal := applyDarkOverlayTheme(c)
	w, h := c.Size()
	s.contentW, s.contentH = float64(w), float64(h)

	if s.textEditing {
		if c.Shortcut(0, ui.KeyEscape) {
			s.cancelText()
			return
		}
	} else {
		if c.Shortcut(0, ui.KeyEscape) {
			s.cancel()
			return
		}
		if c.Shortcut(ui.Cmd, ui.KeyZ) {
			s.undo()
		}
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyZ) || c.Shortcut(ui.Cmd, ui.KeyY) {
			s.redo()
		}
		if c.Shortcut(ui.Cmd, ui.KeyC) {
			s.finish("copy")
		}
		if c.Shortcut(ui.Cmd, ui.KeyS) {
			s.finish("save")
		}
		if c.Shortcut(0, ui.KeyEnter) {
			s.finish("done")
		}
		if c.Shortcut(0, ui.KeyC) {
			s.copyMagnifierColor()
		}
		if s.hasSelection() {
			numKeys := [...]ui.Key{
				ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5,
				ui.Key6, ui.Key7, ui.Key8, ui.Key9,
			}
			for i, k := range numKeys {
				if i < len(annotate.Tools) && c.Shortcut(0, k) {
					s.tool = annotate.Tools[i]
					s.invalidate()
					break
				}
			}
		}
	}

	ui.Box(c).Fill().Draw(s.draw).Children(func() {
		ui.Box(c).Fill().Cursor(s.currentCursor()).HandleInput(s.handleInput)
		if !s.hasSelection() && s.mode == modeNone && s.contentW >= 260 {
			s.guidePill(c, pal)
		}
		if s.hasSelection() {
			s.toolbar(c, pal)
		}
		if s.textEditing {
			s.textEditor(c, pal)
		}
	})
}

func (s *captureSession) guidePill(c *ui.Context, pal pureui.Palette) {
	ui.Row(c).Absolute().Top(16).Left(0).Right(0).Justify(ui.Center).PassThrough().Children(func() {
		ui.Row(c).Padding(7, 14).Gap(8).Radius(999).AlignItems(ui.Center).
			Material(glass.Glass{Tint: pal.HudTint}).
			Border(1, pal.HudBorder).
			InsetShadow(0, 1, 0, 0, pal.HudHighlight).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.42)).
			Children(func() {
				ui.Box(c).Size(20, 20).Radius(10).Gradient(pal.Accent, pal.AccentEnd, 135).Center().Children(func() {
					ui.Icon(c, pureui.IconCrop).Size(11, 11).TextColor(pal.AccentText)
				})
				ui.Text(c, i18n.S("拖拽框选区域 · 单击选中窗口 · 双击完成", "Drag to select · Click window · Double-click to finish")).
					FontSize(11).FontWeight(500).SingleLine().TextColor(pal.HudText)
				ui.Box(c).Width(1).Height(12).Background(pal.HudDivider)
				pureui.HudKeycap(c, "C", pal)
				ui.Text(c, i18n.S("复制色值", "Copy Color")).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.HudMuted)
				ui.Box(c).Width(1).Height(12).Background(pal.HudDivider)
				pureui.HudKeycap(c, "Esc", pal)
				ui.Text(c, i18n.S("退出", "Exit")).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.HudMuted)
			})
	})
}

func (s *captureSession) hasSelection() bool {
	return s.selection.W >= 4 && s.selection.H >= 4
}

func (s *captureSession) isClickMove(p core.Point) bool {
	return math.Hypot((p.X-s.pressRaw.X)*s.scaleX(), (p.Y-s.pressRaw.Y)*s.scaleY()) < 5
}

func (s *captureSession) displaySelection() core.RectI {
	if s.mode == modeSelect {
		if s.isClickMove(s.cursor) {
			return core.RectI{}
		}
		r := core.RectFromPoints(s.dragStart, s.dragCurrent)
		sel := core.RectI{
			X: int(math.Round(r.X)),
			Y: int(math.Round(r.Y)),
			W: int(math.Round(r.W)),
			H: int(math.Round(r.H)),
		}
		return sel.Intersect(s.monitor.Bounds)
	}
	return s.selection
}

func (s *captureSession) resolvedFontSize() float64 {
	if s.fontSize <= 0 {
		return core.DefaultFontSize
	}
	return s.fontSize
}

func (s *captureSession) resolvedMosaicBlock() float64 {
	if s.mosaicBlock <= 0 {
		return core.DefaultMosaicBlock
	}
	return s.mosaicBlock
}

func (s *captureSession) currentCursor() ui.Cursor {
	h := s.hoverHandle
	if s.mode == modeResize || s.mode == modeMove {
		h = s.activeHandle
	}
	switch h {
	case handleNW, handleSE:
		return ui.CursorResizeNWSE
	case handleNE, handleSW:
		return ui.CursorResizeNESW
	case handleN, handleS:
		return ui.CursorResizeNS
	case handleE, handleW:
		return ui.CursorResizeEW
	case handleMove:
		return ui.CursorMove
	default:
		return ui.CursorCrosshair
	}
}

func (s *captureSession) scaleX() float64 {
	if s.monitor.Bounds.W <= 0 || s.contentW <= 0 {
		return 1
	}
	return s.contentW / float64(s.monitor.Bounds.W)
}

func (s *captureSession) scaleY() float64 {
	if s.monitor.Bounds.H <= 0 || s.contentH <= 0 {
		return 1
	}
	return s.contentH / float64(s.monitor.Bounds.H)
}

func (s *captureSession) toDIP(px, py float64) (float32, float32) {
	return float32((px - float64(s.monitor.Bounds.X)) * s.scaleX()),
		float32((py - float64(s.monitor.Bounds.Y)) * s.scaleY())
}

func (s *captureSession) toPhysical(x, y float32) core.Point {
	return core.Point{
		X: float64(s.monitor.Bounds.X) + float64(x)/s.scaleX(),
		Y: float64(s.monitor.Bounds.Y) + float64(y)/s.scaleY(),
	}
}

func (s *captureSession) rectToDIP(rect core.RectI) ui.Rect {
	x0, y0 := s.toDIP(float64(rect.X), float64(rect.Y))
	x1, y1 := s.toDIP(float64(rect.X+rect.W), float64(rect.Y+rect.H))
	return ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

func (s *captureSession) selectionDIP() ui.Rect {
	return s.rectToDIP(s.selection)
}

func (s *captureSession) rel(p core.Point) core.Point {
	return core.Point{X: p.X - float64(s.selection.X), Y: p.Y - float64(s.selection.Y)}
}

func drawCornerBrackets(p *ui.Painter, sel ui.Rect, arm, width float32, col ui.Color) {
	p.Line(sel.X, sel.Y, sel.X+arm, sel.Y, width, col)
	p.Line(sel.X, sel.Y, sel.X, sel.Y+arm, width, col)
	p.Line(sel.X+sel.W-arm, sel.Y, sel.X+sel.W, sel.Y, width, col)
	p.Line(sel.X+sel.W, sel.Y, sel.X+sel.W, sel.Y+arm, width, col)
	p.Line(sel.X, sel.Y+sel.H-arm, sel.X, sel.Y+sel.H, width, col)
	p.Line(sel.X, sel.Y+sel.H, sel.X+arm, sel.Y+sel.H, width, col)
	p.Line(sel.X+sel.W-arm, sel.Y+sel.H, sel.X+sel.W, sel.Y+sel.H, width, col)
	p.Line(sel.X+sel.W, sel.Y+sel.H-arm, sel.X+sel.W, sel.Y+sel.H, width, col)
}

func (s *captureSession) draw(p *ui.Painter, r ui.Rect) {
	p.Image(s.shotBitmap, r, ui.FillBox)
	dim := ui.RGBA(7, 9, 14, 0.56)

	live := s.displaySelection()
	if live.W >= 2 && live.H >= 2 {
		sel := s.rectToDIP(live)
		if sel.Y > r.Y {
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: sel.Y - r.Y}, dim, 0)
		}
		if sel.Y+sel.H < r.Y+r.H {
			p.Fill(ui.Rect{X: r.X, Y: sel.Y + sel.H, W: r.W, H: r.Y + r.H - (sel.Y + sel.H)}, dim, 0)
		}
		if sel.X > r.X {
			p.Fill(ui.Rect{X: r.X, Y: sel.Y, W: sel.X - r.X, H: sel.H}, dim, 0)
		}
		if sel.X+sel.W < r.X+r.W {
			p.Fill(ui.Rect{X: sel.X + sel.W, Y: sel.Y, W: r.X + r.W - (sel.X + sel.W), H: sel.H}, dim, 0)
		}

		if s.hasSelection() {
			if s.doc != nil {
				for _, item := range s.doc.Snapshot() {
					s.drawItem(p, item)
				}
			}
			if s.draft != nil {
				s.drawItem(p, s.draft)
			}
		}

		if (s.mode == modeSelect || s.mode == modeResize || s.mode == modeMove) && sel.W >= 60 && sel.H >= 60 {
			gridCol := ui.RGBA(255, 255, 255, 0.20)
			p.Line(sel.X+sel.W/3, sel.Y, sel.X+sel.W/3, sel.Y+sel.H, 1, gridCol)
			p.Line(sel.X+2*sel.W/3, sel.Y, sel.X+2*sel.W/3, sel.Y+sel.H, 1, gridCol)
			p.Line(sel.X, sel.Y+sel.H/3, sel.X+sel.W, sel.Y+sel.H/3, 1, gridCol)
			p.Line(sel.X, sel.Y+2*sel.H/3, sel.X+sel.W, sel.Y+2*sel.H/3, 1, gridCol)
		}

		accent := ui.Hex("#3B82F6")
		p.Stroke(ui.Rect{X: sel.X - 0.75, Y: sel.Y - 0.75, W: sel.W + 1.5, H: sel.H + 1.5}, ui.RGBA(0, 0, 0, 0.45), 0, 1.5)
		p.Stroke(sel, accent, 0, 1.5)
		if sel.W >= 24 && sel.H >= 24 {
			arm := minf(12, minf(sel.W, sel.H)/4)
			drawCornerBrackets(p, sel, arm, 2, ui.RGBA(255, 255, 255, 0.90))
		}
		if s.hasSelection() {
			handles := []struct {
				h    resizeHandle
				x, y float32
			}{
				{handleNW, sel.X, sel.Y}, {handleN, sel.X + sel.W/2, sel.Y}, {handleNE, sel.X + sel.W, sel.Y},
				{handleW, sel.X, sel.Y + sel.H/2}, {handleE, sel.X + sel.W, sel.Y + sel.H/2},
				{handleSW, sel.X, sel.Y + sel.H}, {handleS, sel.X + sel.W/2, sel.Y + sel.H}, {handleSE, sel.X + sel.W, sel.Y + sel.H},
			}
			for _, item := range handles {
				active := s.hoverHandle == item.h || s.activeHandle == item.h
				hd := float32(8.5)
				fillCol := ui.RGB(255, 255, 255)
				strokeCol := accent
				strokeW := float32(1.5)
				if active {
					hd = 11
					fillCol = ui.Hex("#60A5FA")
					strokeCol = ui.RGB(255, 255, 255)
					strokeW = 2
				}
				dot := ui.Rect{X: item.x - hd/2, Y: item.y - hd/2, W: hd, H: hd}
				p.Shadow(dot, hd/2, 0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.45))
				p.Fill(dot, fillCol, hd/2)
				p.Stroke(dot, strokeCol, hd/2, strokeW)
			}
		}
		if s.plugin.Config().Capture.ShowRuler {
			s.drawRuler(p, sel, live)
		}
	} else {
		if s.hoveredWindow.W >= 8 && s.hoveredWindow.H >= 8 {
			hw := s.rectToDIP(s.hoveredWindow)
			if hw.Y > r.Y {
				p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: hw.Y - r.Y}, dim, 0)
			}
			if hw.Y+hw.H < r.Y+r.H {
				p.Fill(ui.Rect{X: r.X, Y: hw.Y + hw.H, W: r.W, H: r.Y + r.H - (hw.Y + hw.H)}, dim, 0)
			}
			if hw.X > r.X {
				p.Fill(ui.Rect{X: r.X, Y: hw.Y, W: hw.X - r.X, H: hw.H}, dim, 0)
			}
			if hw.X+hw.W < r.X+r.W {
				p.Fill(ui.Rect{X: hw.X + hw.W, Y: hw.Y, W: r.X + r.W - (hw.X + hw.W), H: hw.H}, dim, 0)
			}
			p.Stroke(hw, ui.RGBA(59, 130, 246, 0.35), 0, 4)
			p.Stroke(hw, ui.Hex("#60A5FA"), 0, 2)
			if hw.W >= 24 && hw.H >= 24 {
				drawCornerBrackets(p, hw, minf(12, minf(hw.W, hw.H)/4), 2, ui.RGBA(255, 255, 255, 0.90))
			}
			s.drawRuler(p, hw, s.hoveredWindow)
		} else {
			p.Fill(r, dim, 0)
		}
		cx, cy := s.toDIP(s.cursor.X, s.cursor.Y)
		p.Fill(ui.Rect{X: cx + 1, Y: r.Y, W: 1, H: r.H}, ui.RGBA(0, 0, 0, 0.35), 0)
		p.Fill(ui.Rect{X: r.X, Y: cy + 1, W: r.W, H: 1}, ui.RGBA(0, 0, 0, 0.35), 0)
		p.Fill(ui.Rect{X: cx, Y: r.Y, W: 1, H: r.H}, ui.RGBA(255, 255, 255, 0.38), 0)
		p.Fill(ui.Rect{X: r.X, Y: cy, W: r.W, H: 1}, ui.RGBA(255, 255, 255, 0.38), 0)
	}

	if s.plugin.Config().Capture.ShowMagnifier && !s.textEditing &&
		(s.mode == modeNone || s.mode == modeSelect || s.mode == modeResize) {
		s.drawMagnifier(p)
	}
	if s.status != "" {
		w, h := p.MeasureText(0, ui.Span{Text: s.status, Size: 12})
		pill := ui.Rect{X: r.X + 16, Y: r.Y + 16, W: w + 34, H: h + 14}
		p.Shadow(pill, 9, 0, 6, 18, 0, ui.RGBA(0, 0, 0, 0.42))
		p.Fill(pill, ui.RGBA(14, 17, 24, 0.92), 9)
		p.Stroke(pill, ui.RGBA(255, 255, 255, 0.16), 9, 1)
		p.Line(pill.X+9, pill.Y+1, pill.X+pill.W-9, pill.Y+1, 1, ui.RGBA(255, 255, 255, 0.14))
		p.Fill(ui.Rect{X: pill.X + 11, Y: pill.Y + (pill.H-6)/2, W: 6, H: 6}, ui.Hex("#22C55E"), 3)
		p.Text(pill.X+23, pill.Y+7, s.status, 12, ui.RGB(245, 247, 250))
	}
}

func (s *captureSession) drawRuler(p *ui.Painter, sel ui.Rect, phys core.RectI) {
	text := fmt.Sprintf("%d × %d px", phys.W, phys.H)
	w, h := p.MeasureText(0, ui.Span{Text: text, Size: 11})
	pw, ph := w+26, h+10
	x := sel.X
	y := sel.Y - ph - 8
	if y < 6 {
		y = sel.Y + 8
	}
	if float64(x+pw+6) > s.contentW {
		x = float32(s.contentW) - pw - 6
	}
	if x < 6 {
		x = 6
	}
	pill := ui.Rect{X: x, Y: y, W: pw, H: ph}
	p.Shadow(pill, 8, 0, 4, 14, 0, ui.RGBA(0, 0, 0, 0.40))
	p.Fill(pill, ui.RGBA(14, 17, 24, 0.90), 8)
	p.Stroke(pill, ui.RGBA(255, 255, 255, 0.16), 8, 1)
	p.Line(pill.X+8, pill.Y+1, pill.X+pill.W-8, pill.Y+1, 1, ui.RGBA(255, 255, 255, 0.14))
	p.Fill(ui.Rect{X: x + 9, Y: y + (ph-6)/2, W: 6, H: 6}, ui.Hex("#60A5FA"), 3)
	p.Text(x+19, y+5, text, 11, ui.RGB(245, 247, 250))
}

func (s *captureSession) drawMagnifier(p *ui.Painter) {
	if s.magnifier == nil {
		return
	}
	cx, cy := s.toDIP(s.cursor.X, s.cursor.Y)
	const panelW = float32(168)
	const panelH = float32(214)
	const imgSize = float32(152)

	x, y := cx+18, cy+18
	if float64(x+panelW+10) > s.contentW {
		x = cx - panelW - 18
	}
	if float64(y+panelH+10) > s.contentH {
		y = cy - panelH - 18
	}
	if x < 8 {
		x = 8
	}
	if y < 8 {
		y = 8
	}

	card := ui.Rect{X: x, Y: y, W: panelW, H: panelH}
	p.Shadow(card, 12, 0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.50))
	p.Fill(card, ui.RGBA(14, 17, 24, 0.92), 12)
	p.Stroke(card, ui.RGBA(255, 255, 255, 0.16), 12, 1)
	p.Line(card.X+12, card.Y+1, card.X+card.W-12, card.Y+1, 1, ui.RGBA(255, 255, 255, 0.14))

	imgRect := ui.Rect{X: x + 8, Y: y + 8, W: imgSize, H: imgSize}
	cell := imgSize / 31
	midX := imgRect.X + imgSize/2 - cell/2
	midY := imgRect.Y + imgSize/2 - cell/2
	p.Clip(imgRect, 10, func() {
		p.Image(s.magnifier, imgRect, ui.FillBox)
		gridCol := ui.RGBA(255, 255, 255, 0.08)
		for i := 1; i < 31; i++ {
			off := float32(i) * cell
			p.Line(imgRect.X+off, imgRect.Y, imgRect.X+off, imgRect.Y+imgSize, 0.75, gridCol)
			p.Line(imgRect.X, imgRect.Y+off, imgRect.X+imgSize, imgRect.Y+off, 0.75, gridCol)
		}
		axisCol := ui.RGBA(96, 165, 250, 0.52)
		cyLine := midY + cell/2
		cxLine := midX + cell/2
		p.Line(imgRect.X, cyLine, midX-1, cyLine, 1, axisCol)
		p.Line(midX+cell+1, cyLine, imgRect.X+imgSize, cyLine, 1, axisCol)
		p.Line(cxLine, imgRect.Y, cxLine, midY-1, 1, axisCol)
		p.Line(cxLine, midY+cell+1, cxLine, imgRect.Y+imgSize, 1, axisCol)

		p.Stroke(ui.Rect{X: midX - 1, Y: midY - 1, W: cell + 2, H: cell + 2}, ui.RGBA(0, 0, 0, 0.90), 1, 1.2)
		p.Stroke(ui.Rect{X: midX, Y: midY, W: cell, H: cell}, ui.RGB(255, 255, 255), 1, 1.4)
	})
	p.Stroke(imgRect, ui.RGBA(255, 255, 255, 0.16), 10, 1)

	c := s.magnifierColor
	hex := fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
	row1Y := y + imgSize + 15
	swatch := ui.Rect{X: x + 10, Y: row1Y, W: 18, H: 18}
	p.Fill(swatch, ui.RGB(c.R, c.G, c.B), 6)
	p.Stroke(swatch, ui.RGBA(0, 0, 0, 0.45), 6, 1)
	p.Stroke(ui.Rect{X: swatch.X + 1, Y: swatch.Y + 1, W: swatch.W - 2, H: swatch.H - 2}, ui.RGBA(255, 255, 255, 0.55), 5, 1)
	_, hexH := p.MeasureText(0, ui.Span{Text: hex, Size: 12})
	p.Text(x+34, row1Y+(18-hexH)/2, hex, 12, ui.RGB(245, 247, 250))

	copyTag := i18n.S("C 复制", "C Copy")
	tw, th := p.MeasureText(0, ui.Span{Text: copyTag, Size: 10})
	tagW, tagH := tw+12, float32(18)
	tagRect := ui.Rect{X: x + panelW - 10 - tagW, Y: row1Y, W: tagW, H: tagH}
	p.Fill(tagRect, ui.RGBA(255, 255, 255, 0.10), 5)
	p.Stroke(tagRect, ui.RGBA(255, 255, 255, 0.18), 5, 1)
	p.Text(tagRect.X+6, tagRect.Y+(tagH-th)/2, copyTag, 10, ui.RGB(209, 213, 219))

	p.Line(x+10, y+imgSize+38, x+panelW-10, y+imgSize+38, 1, ui.RGBA(255, 255, 255, 0.06))
	rgbText := fmt.Sprintf("RGB(%d,%d,%d)", c.R, c.G, c.B)
	posText := fmt.Sprintf("(%d,%d)", int(s.cursor.X), int(s.cursor.Y))
	posW, _ := p.MeasureText(0, ui.Span{Text: posText, Size: 10})
	p.Text(x+10, y+imgSize+42, rgbText, 10, ui.RGB(156, 163, 175))
	p.Text(x+panelW-10-posW, y+imgSize+42, posText, 10, ui.RGB(156, 163, 175))
}

func (s *captureSession) drawItem(p *ui.Painter, it *annotate.Item) {
	if it == nil {
		return
	}
	col := ui.RGBA(it.Color.R(), it.Color.G(), it.Color.B(), float32(it.Color.A())/255)
	width := float32(it.Thickness * s.scaleX())
	if width < 1 {
		width = 1
	}
	pt := func(cp core.Point) (float32, float32) {
		return s.toDIP(float64(s.selection.X)+cp.X, float64(s.selection.Y)+cp.Y)
	}
	switch it.Tool {
	case annotate.ToolRect:
		x0, y0 := pt(it.Start)
		x1, y1 := pt(it.End)
		rect := ui.Rect{X: minf(x0, x1), Y: minf(y0, y1), W: absf(x1 - x0), H: absf(y1 - y0)}
		if it.Filled {
			p.Fill(rect, ui.RGBA(it.FillColor.R(), it.FillColor.G(), it.FillColor.B(), float32(it.FillColor.A())/255), 0)
		}
		p.Stroke(rect, col, 0, width)
	case annotate.ToolEllipse:
		x0, y0 := pt(it.Start)
		x1, y1 := pt(it.End)
		cx, cy := (x0+x1)/2, (y0+y1)/2
		rx, ry := absf(x1-x0)/2, absf(y1-y0)/2
		path := ellipsePath(cx, cy, rx, ry)
		if it.Filled {
			p.FillPath(path, ui.RGBA(it.FillColor.R(), it.FillColor.G(), it.FillColor.B(), float32(it.FillColor.A())/255))
		}
		p.StrokePath(path, width, col)
	case annotate.ToolLine:
		x0, y0 := pt(it.Start)
		x1, y1 := pt(it.End)
		p.StrokePolyline([][2]float32{{x0, y0}, {x1, y1}}, width, col)
	case annotate.ToolArrow:
		x0, y0 := pt(it.Start)
		x1, y1 := pt(it.End)
		shape := annotate.ArrowGeometry(
			core.Point{X: float64(x0), Y: float64(y0)},
			core.Point{X: float64(x1), Y: float64(y1)},
			it.ResolveHeadLength()*s.scaleX(),
		)
		p.StrokePolyline([][2]float32{{x0, y0}, {float32(shape.Base.X), float32(shape.Base.Y)}}, width, col)
		tri := (&ui.Path{}).MoveTo(float32(shape.Tip.X), float32(shape.Tip.Y)).
			LineTo(float32(shape.Left.X), float32(shape.Left.Y)).
			LineTo(float32(shape.Right.X), float32(shape.Right.Y)).Close()
		p.FillPath(tri, col)
	case annotate.ToolPen:
		if len(it.Points) < 2 {
			return
		}
		pts := make([][2]float32, len(it.Points))
		for i, cp := range it.Points {
			x, y := pt(cp)
			pts[i] = [2]float32{x, y}
		}
		p.StrokePolyline(pts, width, col)
	case annotate.ToolHighlight:
		x0, y0 := pt(it.Start)
		x1, y1 := pt(it.End)
		p.StrokePolyline([][2]float32{{x0, y0}, {x1, y1}}, width, col)
	case annotate.ToolText:
		x, y := pt(it.Origin)
		size := float32(it.FontSize * s.scaleX())
		m := it.Measured
		if m.W <= 0 || m.H <= 0 {
			m = annotate.MeasureText(it.Text, it.FontSize)
		}
		if it.HasBackground {
			p.Fill(ui.Rect{X: x, Y: y, W: float32(m.W*s.scaleX()) + 4, H: float32(m.H*s.scaleY()) + 2},
				ui.RGBA(it.Background.R(), it.Background.G(), it.Background.B(), float32(it.Background.A())/255), 3)
		}
		for i, line := range strings.Split(it.Text, "\n") {
			p.Text(x+2, y+float32(i)*size*1.35, line, size, col)
		}
	case annotate.ToolMosaic:
		block := it.BlockSize
		if block <= 0 {
			block = core.DefaultMosaicBlock
		}
		r := it.Region
		x0, y0 := int(math.Floor(float64(s.selection.X)+r.X)), int(math.Floor(float64(s.selection.Y)+r.Y))
		x1, y1 := int(math.Ceil(float64(s.selection.X)+r.X+r.W)), int(math.Ceil(float64(s.selection.Y)+r.Y+r.H))
		step := int(math.Max(1, math.Round(block)))
		for yy := y0; yy < y1; yy += step {
			for xx := x0; xx < x1; xx += step {
				bx1, by1 := min(xx+step, x1), min(yy+step, y1)
				if bx1 <= xx || by1 <= yy {
					continue
				}
				var sr, sg, sb, n int
				for py := yy; py < by1; py++ {
					for px := xx; px < bx1; px++ {
						if px < s.frame.Bounds().Min.X || px >= s.frame.Bounds().Max.X ||
							py < s.frame.Bounds().Min.Y || py >= s.frame.Bounds().Max.Y {
							continue
						}
						c := s.frame.RGBAAt(px, py)
						sr, sg, sb, n = sr+int(c.R), sg+int(c.G), sb+int(c.B), n+1
					}
				}
				if n == 0 {
					continue
				}
				dx0, dy0 := s.toDIP(float64(xx), float64(yy))
				dx1, dy1 := s.toDIP(float64(bx1), float64(by1))
				p.Fill(ui.Rect{X: dx0, Y: dy0, W: dx1 - dx0, H: dy1 - dy0},
					ui.RGB(uint8(sr/n), uint8(sg/n), uint8(sb/n)), 0)
			}
		}
	case annotate.ToolBadge:
		x, y := pt(it.Center)
		radius := float32(it.Radius * s.scaleX())
		if radius <= 0 {
			radius = 14
		}
		p.Fill(ui.Rect{X: x - radius, Y: y - radius, W: radius * 2, H: radius * 2}, col, radius)
		text := fmt.Sprint(it.Number)
		tw, th := p.MeasureText(0, ui.Span{Text: text, Size: radius * 1.15})
		p.Text(x-tw/2, y-th/2, text, radius*1.15,
			ui.RGBA(it.TextColor.R(), it.TextColor.G(), it.TextColor.B(), float32(it.TextColor.A())/255))
	}
}

func ellipsePath(cx, cy, rx, ry float32) *ui.Path {
	k := float32(0.5522847498)
	kx, ky := rx*k, ry*k
	return (&ui.Path{}).MoveTo(cx+rx, cy).
		CubeTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry).
		CubeTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy).
		CubeTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry).
		CubeTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy).Close()
}

func (s *captureSession) hitWindow(p core.Point) core.RectI {
	if s.hasSelection() || !s.plugin.Config().Capture.SnapToWindows || len(s.windows) == 0 {
		return core.RectI{}
	}
	pt := image.Pt(int(p.X), int(p.Y))
	for _, w := range s.windows {
		clipped := w.Intersect(s.monitor.Bounds)
		if clipped.W < 8 || clipped.H < 8 {
			continue
		}
		if clipped.Contains(pt) {
			return clipped
		}
	}
	return core.RectI{}
}

func (s *captureSession) hitHandle(dipX, dipY float32) resizeHandle {
	if !s.hasSelection() {
		return handleNone
	}
	sel := s.selectionDIP()
	const r = float32(8)
	handles := []struct {
		h    resizeHandle
		x, y float32
	}{
		{handleNW, sel.X, sel.Y},
		{handleNE, sel.X + sel.W, sel.Y},
		{handleSE, sel.X + sel.W, sel.Y + sel.H},
		{handleSW, sel.X, sel.Y + sel.H},
		{handleN, sel.X + sel.W/2, sel.Y},
		{handleE, sel.X + sel.W, sel.Y + sel.H/2},
		{handleS, sel.X + sel.W/2, sel.Y + sel.H},
		{handleW, sel.X, sel.Y + sel.H/2},
	}
	for _, item := range handles {
		if absf(dipX-item.x) <= r && absf(dipY-item.y) <= r {
			return item.h
		}
	}
	const band = float32(5)
	inX := dipX >= sel.X-band && dipX <= sel.X+sel.W+band
	inY := dipY >= sel.Y-band && dipY <= sel.Y+sel.H+band
	nearTop := absf(dipY-sel.Y) <= band && inX
	nearBottom := absf(dipY-(sel.Y+sel.H)) <= band && inX
	nearLeft := absf(dipX-sel.X) <= band && inY
	nearRight := absf(dipX-(sel.X+sel.W)) <= band && inY
	if nearTop || nearBottom || nearLeft || nearRight {
		return handleMove
	}
	return handleNone
}

func (s *captureSession) applyResize(p core.Point) {
	orig := s.origSelection
	left, top := orig.X, orig.Y
	right, bottom := orig.X+orig.W, orig.Y+orig.H
	mon := s.monitor.Bounds
	minSize := 16
	px := int(math.Round(s.snapX(p.X)))
	py := int(math.Round(s.snapY(p.Y)))

	switch s.activeHandle {
	case handleNW:
		left = clampInt(px, mon.X, right-minSize)
		top = clampInt(py, mon.Y, bottom-minSize)
	case handleN:
		top = clampInt(py, mon.Y, bottom-minSize)
	case handleNE:
		right = clampInt(px, left+minSize, mon.X+mon.W)
		top = clampInt(py, mon.Y, bottom-minSize)
	case handleE:
		right = clampInt(px, left+minSize, mon.X+mon.W)
	case handleSE:
		right = clampInt(px, left+minSize, mon.X+mon.W)
		bottom = clampInt(py, top+minSize, mon.Y+mon.H)
	case handleS:
		bottom = clampInt(py, top+minSize, mon.Y+mon.H)
	case handleSW:
		left = clampInt(px, mon.X, right-minSize)
		bottom = clampInt(py, top+minSize, mon.Y+mon.H)
	case handleW:
		left = clampInt(px, mon.X, right-minSize)
	}
	s.selection = core.RectI{X: left, Y: top, W: right - left, H: bottom - top}
}

func (s *captureSession) applyMove(p core.Point) {
	dx := int(math.Round(p.X - s.moveOrigin.X))
	dy := int(math.Round(p.Y - s.moveOrigin.Y))
	mon := s.monitor.Bounds
	nx := clampInt(s.origSelection.X+dx, mon.X, mon.X+mon.W-s.origSelection.W)
	ny := clampInt(s.origSelection.Y+dy, mon.Y, mon.Y+mon.H-s.origSelection.H)
	s.selection = core.RectI{X: nx, Y: ny, W: s.origSelection.W, H: s.origSelection.H}
}

func (s *captureSession) handleInput(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputPointerMove:
		p := s.toPhysical(ev.X, ev.Y)
		s.cursor = p
		s.updateMagnifier(p)
		switch s.mode {
		case modeSelect:
			if !s.isClickMove(p) {
				s.badgeAddedOnClick = false
			}
			s.dragCurrent = s.snapPoint(p)
		case modeResize:
			s.badgeAddedOnClick = false
			s.applyResize(p)
		case modeMove:
			s.badgeAddedOnClick = false
			s.applyMove(p)
		case modeAnnotate:
			s.badgeAddedOnClick = false
			if s.draft != nil {
				s.updateDraft(p)
			}
		default:
			if s.hasSelection() {
				s.hoverHandle = s.hitHandle(ev.X, ev.Y)
				s.hoveredWindow = core.RectI{}
			} else {
				s.hoverHandle = handleNone
				s.hoveredWindow = s.hitWindow(p)
			}
		}
		return true

	case ui.InputPointerDown:
		if s.finishing {
			return false
		}
		if ev.Button == 1 {
			return true
		}
		if ev.Button != 0 {
			return false
		}
		p := s.toPhysical(ev.X, ev.Y)
		s.cursor = p
		s.updateMagnifier(p)
		if s.textEditing {
			s.commitText()
		}
		if h := s.hitHandle(ev.X, ev.Y); h != handleNone {
			s.badgeAddedOnClick = false
			s.activeHandle = h
			s.origSelection = s.selection
			if h == handleMove {
				s.mode = modeMove
				s.moveOrigin = p
			} else {
				s.mode = modeResize
			}
			return true
		}
		if s.hasSelection() && s.selection.Contains(image.Pt(int(p.X), int(p.Y))) {
			if ev.Clicks >= 2 && s.mode == modeNone {
				if s.badgeAddedOnClick && s.doc != nil {
					s.doc.Undo()
					s.badgeAddedOnClick = false
				}
				s.finish("done")
				return true
			}
			s.badgeAddedOnClick = false
			switch s.tool {
			case annotate.ToolText:
				s.textOrigin = p
				s.textValue = ""
				s.textEditing = true
				s.invalidate()
				return true
			case annotate.ToolBadge:
				if s.doc != nil {
					num := s.doc.NextBadge()
					s.doc.Add(&annotate.Item{
						Tool: annotate.ToolBadge, Color: s.color, Thickness: s.thickness,
						Center: s.rel(p), Number: num, Radius: core.DefaultBadgeRadius,
						TextColor: core.DefaultBadgeTextColor,
					})
					s.badgeAddedOnClick = true
				}
				s.invalidate()
				return true
			}
			s.mode = modeAnnotate
			s.dragStart = s.rel(p)
			s.dragCurrent = s.dragStart
			s.draft = s.newDraft(s.dragStart)
			return true
		}
		s.selection = core.RectI{}
		s.doc = nil
		s.draft = nil
		s.textEditing = false
		s.badgeAddedOnClick = false
		s.hoverHandle = handleNone
		s.hoveredWindow = s.hitWindow(p)
		s.mode = modeSelect
		s.pressRaw = p
		s.dragStart = s.snapPoint(p)
		s.dragCurrent = s.dragStart
		return true

	case ui.InputPointerUp:
		if ev.Button == 1 {
			switch {
			case s.textEditing:
				s.cancelText()
			case s.hasSelection():
				s.resetSelection()
			default:
				s.cancel()
			}
			return true
		}
		if ev.Button != 0 {
			return false
		}
		p := s.toPhysical(ev.X, ev.Y)
		s.cursor = p
		s.updateMagnifier(p)
		switch s.mode {
		case modeSelect:
			s.finalizeSelection(p)
		case modeResize:
			s.applyResize(p)
			if s.doc != nil {
				s.doc.Width, s.doc.Height = s.selection.W, s.selection.H
			}
			s.activeHandle = handleNone
		case modeMove:
			s.applyMove(p)
			s.activeHandle = handleNone
		case modeAnnotate:
			s.finalizeDraft(p)
		}
		s.mode = modeNone
		s.invalidate()
		return true
	}
	return false
}

func (s *captureSession) snapAxis(v, thresh float64, candidates func(func(float64))) float64 {
	if !s.plugin.Config().Capture.SnapToWindows || len(s.windows) == 0 {
		return v
	}
	if thresh < 8.0 {
		thresh = 8.0
	}
	out := v
	dist := thresh + 1
	check := func(c float64) {
		if d := math.Abs(v - c); d < dist {
			dist, out = d, c
		}
	}
	candidates(check)
	if dist <= thresh {
		return out
	}
	return v
}

func (s *captureSession) snapX(x float64) float64 {
	return s.snapAxis(x, 8.0/s.scaleX(), func(add func(float64)) {
		add(float64(s.monitor.Bounds.X))
		add(float64(s.monitor.Bounds.X + s.monitor.Bounds.W))
		for _, w := range s.windows {
			add(float64(w.X))
			add(float64(w.X + w.W))
		}
	})
}

func (s *captureSession) snapY(y float64) float64 {
	return s.snapAxis(y, 8.0/s.scaleY(), func(add func(float64)) {
		add(float64(s.monitor.Bounds.Y))
		add(float64(s.monitor.Bounds.Y + s.monitor.Bounds.H))
		for _, w := range s.windows {
			add(float64(w.Y))
			add(float64(w.Y + w.H))
		}
	})
}

func (s *captureSession) snapPoint(p core.Point) core.Point {
	return core.Point{X: s.snapX(p.X), Y: s.snapY(p.Y)}
}

func (s *captureSession) newDraft(start core.Point) *annotate.Item {
	it := &annotate.Item{Color: s.color, Thickness: s.thickness}
	switch s.tool {
	case annotate.ToolRect, annotate.ToolEllipse:
		it.Tool = s.tool
		it.Start, it.End = start, start
		it.Filled = s.filled
		it.FillColor = s.color.WithAlpha(core.DefaultFillAlpha)
	case annotate.ToolLine:
		it.Tool = annotate.ToolLine
		it.Start, it.End = start, start
	case annotate.ToolArrow:
		it.Tool = annotate.ToolArrow
		it.Start, it.End = start, start
	case annotate.ToolPen:
		it.Tool = annotate.ToolPen
		it.Points = []core.Point{start}
	case annotate.ToolHighlight:
		it.Tool = annotate.ToolHighlight
		it.Color = core.DefaultHighlightColor
		it.Thickness = core.DefaultHighlightWidth
		it.Start, it.End = start, start
	case annotate.ToolMosaic:
		it.Tool = annotate.ToolMosaic
		it.Region = core.Rect{X: start.X, Y: start.Y}
		it.BlockSize = s.resolvedMosaicBlock()
	default:
		it.Tool = annotate.ToolRect
		it.Start, it.End = start, start
	}
	return it
}

func (s *captureSession) updateDraft(p core.Point) {
	if s.draft == nil {
		return
	}
	rel := s.rel(p)
	switch s.draft.Tool {
	case annotate.ToolPen:
		s.draft.Points = append(s.draft.Points, rel)
	case annotate.ToolMosaic:
		s.draft.Region = core.RectFromPoints(s.draft.Region.Point(), rel)
	default:
		s.draft.End = rel
	}
}

func (s *captureSession) finalizeSelection(p core.Point) {
	if s.isClickMove(p) && s.hoveredWindow.W >= 4 && s.hoveredWindow.H >= 4 {
		sel := s.hoveredWindow.Intersect(s.monitor.Bounds)
		if sel.W >= 4 && sel.H >= 4 {
			s.selection = sel
			s.doc = annotate.NewDocument(sel.W, sel.H)
			s.hoveredWindow = core.RectI{}
			s.status = ""
			s.invalidate()
			return
		}
	}
	end := s.snapPoint(p)
	r := core.RectFromPoints(s.dragStart, end)
	sel := core.RectI{
		X: int(math.Round(r.X)),
		Y: int(math.Round(r.Y)),
		W: int(math.Round(r.W)),
		H: int(math.Round(r.H)),
	}
	sel = sel.Intersect(s.monitor.Bounds)
	if sel.W < 4 || sel.H < 4 {
		s.selection = core.RectI{}
		s.doc = nil
		s.status = i18n.S("选区太小，请重新拖拽", "Selection too small, drag again")
		return
	}
	s.selection = sel
	s.doc = annotate.NewDocument(sel.W, sel.H)
	s.hoveredWindow = core.RectI{}
	s.status = ""
	s.invalidate()
}

func (s *captureSession) finalizeDraft(p core.Point) {
	if s.draft == nil {
		return
	}
	if s.draft.Tool == annotate.ToolPen {
		s.draft.Points = append(s.draft.Points, s.rel(p))
	} else if s.draft.Tool == annotate.ToolMosaic {
		s.draft.Region = core.RectFromPoints(s.draft.Region.Point(), s.rel(p))
	} else {
		s.draft.End = s.rel(p)
	}
	valid := true
	switch s.draft.Tool {
	case annotate.ToolRect, annotate.ToolEllipse, annotate.ToolMosaic:
		valid = s.draft.Bounds().W >= 2 && s.draft.Bounds().H >= 2
	case annotate.ToolLine, annotate.ToolArrow, annotate.ToolHighlight:
		valid = math.Hypot(s.draft.End.X-s.draft.Start.X, s.draft.End.Y-s.draft.Start.Y) >= 2
	case annotate.ToolPen:
		valid = len(s.draft.Points) >= 2
	}
	if valid && s.doc != nil {
		s.doc.Add(s.draft)
	}
	s.draft = nil
	s.invalidate()
}

func (s *captureSession) updateMagnifier(p core.Point) {
	if s.magnifier == nil || s.frame == nil {
		return
	}
	cx, cy := int(p.X), int(p.Y)
	s.magnifierColor = color.RGBA{}
	if cx >= s.frame.Bounds().Min.X && cx < s.frame.Bounds().Max.X &&
		cy >= s.frame.Bounds().Min.Y && cy < s.frame.Bounds().Max.Y {
		s.magnifierColor = s.frame.RGBAAt(cx, cy)
	}
	src := image.Rect(cx-15, cy-15, cx+16, cy+16).Intersect(s.frame.Bounds())
	if src.Empty() {
		return
	}
	dst := image.NewRGBA(image.Rect(0, 0, 120, 120))
	xdraw.NearestNeighbor.Scale(dst, dst.Bounds(), s.frame, src, xdraw.Src, nil)
	s.magnifier.Update(dst)
}

func (s *captureSession) copyMagnifierColor() {
	c := s.magnifierColor
	hex := fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
	if s.plugin != nil {
		if err := s.plugin.copyEphemeralText(hex); err != nil {
			s.status = i18n.S("复制色值失败: ", "Failed to copy color: ") + err.Error()
			s.invalidate()
			return
		}
	}
	s.status = i18n.S("已复制色值 ", "Copied color ") + hex
	s.invalidate()
}

func (s *captureSession) toolbarPosition(dockW, dockH float32) (left, top float32) {
	cw := maxf(float32(s.contentW), 260)
	ch := maxf(float32(s.contentH), 200)
	const margin = float32(12)
	sel := s.selectionDIP()

	left = sel.X + (sel.W-dockW)/2
	left = clampf(left, margin, maxf(margin, cw-dockW-margin))

	below := sel.Y + sel.H + margin
	above := sel.Y - dockH - margin
	switch {
	case below+dockH <= ch-margin:
		top = below
	case above >= margin:
		top = above
	default:
		top = maxf(margin, ch-dockH-16)
	}
	top = clampf(top, margin, maxf(margin, ch-dockH-margin))
	return left, top
}

func (s *captureSession) toolbar(c *ui.Context, pal pureui.Palette) {
	maxDockW := minf(860, maxf(240, float32(s.contentW)-24))
	estW := s.dockW
	if estW <= 0 || estW > maxDockW {
		estW = minf(800, maxDockW)
	}
	estH := s.dockH
	if estH <= 0 {
		estH = 92
	}
	left, top := s.toolbarPosition(estW, estH)

	dock := ui.Column(c).Absolute().Left(left).Top(top).MaxWidth(maxDockW).
		Padding(8, 10).Gap(7).Radius(16).
		Material(glass.Glass{Tint: pal.HudTint}).
		Border(1, pal.HudBorder).
		InsetShadow(0, 1, 0, 0, pal.HudHighlight).
		Shadow(0, 14, 36, 0, ui.RGBA(0, 0, 0, 0.55))

	dock.Children(func() {
		// Primary dock row: 9 tools rail | Undo/Redo/Reset rail | Actions
		ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Row(c).Padding(3).Gap(2).Radius(10).Background(pal.HudWell).Children(func() {
				for i, tool := range annotate.Tools {
					tip := fmt.Sprintf("%s (%d)", annotate.ToolLabel(tool), i+1)
					btn := pureui.HudIconButton(c, toolIcon(tool), annotate.ToolLabel(tool), tip, s.tool == tool, false, false, pal)
					btn.OnClick(func() {
						s.tool = tool
						s.badgeAddedOnClick = false
						s.invalidate()
					})
				}
			})

			ui.Box(c).Width(1).Height(20).Background(pal.HudDivider)

			ui.Row(c).Padding(3).Gap(2).Radius(10).Background(pal.HudWell).Children(func() {
				canUndo, canRedo := false, false
				if s.doc != nil {
					canUndo, canRedo = s.doc.CanUndo(), s.doc.CanRedo()
				}
				pureui.HudIconButton(c, pureui.IconUndo, i18n.S("撤销", "Undo"), i18n.S("撤销 (Ctrl+Z)", "Undo (Ctrl+Z)"), false, !canUndo, false, pal).OnClick(s.undo)
				pureui.HudIconButton(c, pureui.IconRedo, i18n.S("重做", "Redo"), i18n.S("重做 (Ctrl+Y)", "Redo (Ctrl+Y)"), false, !canRedo, false, pal).OnClick(s.redo)
				pureui.HudIconButton(c, pureui.IconRotateCcw, i18n.S("重选", "Reselect"), i18n.S("重新框选区域", "Reselect region"), false, false, false, pal).OnClick(s.resetSelection)
			})

			ui.Box(c).Width(1).Height(20).Background(pal.HudDivider)

			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				s.actionPill(c, pureui.IconCopy, i18n.S("复制", "Copy"), i18n.S("复制到剪贴板 (Ctrl+C)", "Copy to clipboard (Ctrl+C)"), false, pal, func() { s.finish("copy") })
				s.actionPill(c, pureui.IconSave, i18n.S("保存", "Save"), i18n.S("保存到文件 (Ctrl+S)", "Save to file (Ctrl+S)"), false, pal, func() { s.finish("save") })
				s.actionPill(c, pureui.IconPin, i18n.S("贴图", "Pin"), i18n.S("贴到桌面", "Pin to screen"), false, pal, func() { s.finish("pin") })
				defLabel := captureActionLabel(s.plugin.Config().Capture.DefaultAction)
				doneBtn := ui.ButtonBase(c).Height(28).Padding(0, 11).Radius(8).Gap(4).AlignItems(ui.Center).
					Label(i18n.S("完成", "Done")).Tooltip(i18n.F("完成截图 · %s (双击 / Enter)", "Finish capture · %s (Double-click / Enter)", defLabel)).
					InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.25)).
					Shadow(0, 3, 10, 0, ui.RGBA(59, 130, 246, 0.42))
				if doneBtn.Pressed() {
					doneBtn.Background(pal.AccentPressed)
				} else if doneBtn.Hovered() {
					doneBtn.Gradient(pal.AccentHover, pal.Accent, 135)
				} else {
					doneBtn.Gradient(pal.Accent, pal.AccentEnd, 135)
				}
				doneBtn.Children(func() {
					ui.Icon(c, pureui.IconCheck).Size(12, 12).TextColor(pal.AccentText)
					ui.Text(c, i18n.S("完成", "Done")).FontSize(11).FontWeight(600).SingleLine().TextColor(pal.AccentText)
				})
				doneBtn.OnClick(func() { s.finish("done") })
				s.actionPill(c, pureui.IconClose, i18n.S("取消", "Cancel"), i18n.S("退出截图 (Esc)", "Cancel capture (Esc)"), true, pal, s.cancel)
			})
		})

		ui.Box(c).Height(1).FillWidth().Background(ui.RGBA(255, 255, 255, 0.08))

		// Contextual properties sub-row
		ui.Row(c).FillWidth().Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Row(c).Padding(3, 6).Gap(5).Radius(999).Background(pal.HudWell).AlignItems(ui.Center).Children(func() {
				for i, col := range core.Palette {
					colUI := ui.RGBA(col.R(), col.G(), col.B(), float32(col.A())/255)
					swatch := ui.ButtonBase(c).Size(18, 18).Radius(9).Label(i18n.F("颜色 %d", "Color %d", i+1)).
						Background(colUI)
					if s.color == col {
						swatch.Border(2, ui.RGB(255, 255, 255)).Shadow(0, 0, 8, 0, colUI.Alpha(0.75))
					} else {
						swatch.Border(1, ui.RGBA(255, 255, 255, 0.34))
					}
					swatch.OnClick(func() {
						s.color = col
						s.invalidate()
					})
				}
			})

			ui.Row(c).Padding(2, 4).Gap(2).Radius(8).Background(pal.HudWell).AlignItems(ui.Center).Children(func() {
				for _, th := range core.Thicknesses {
					active := s.thickness == th
					btn := ui.ButtonBase(c).Size(24, 24).Radius(6).Center().Label(i18n.F("粗细 %.0f", "Width %.0f", th)).
						Tooltip(i18n.F("线宽 %.0f px", "Stroke %.0f px", th))
					if active {
						btn.Background(pal.AccentSoft)
					} else if btn.Hovered() {
						btn.Background(pal.HudHover)
					}
					dotSize := clampf(float32(th)+3, 4, 14)
					btn.Children(func() {
						dotCol := pal.HudText
						if active {
							lum := 0.299*float32(s.color.R()) + 0.587*float32(s.color.G()) + 0.114*float32(s.color.B())
							if lum > 70 {
								dotCol = ui.RGBA(s.color.R(), s.color.G(), s.color.B(), 1)
							} else {
								dotCol = pal.AccentHover
							}
						}
						ui.Box(c).Size(dotSize, dotSize).Radius(dotSize / 2).Background(dotCol)
					})
					btn.OnClick(func() {
						s.thickness = th
						s.invalidate()
					})
				}
			})

			switch s.tool {
			case annotate.ToolRect, annotate.ToolEllipse:
				s.toggleChip(c, pureui.IconFill, i18n.S("填充", "Fill"), s.filled, pal, func() {
					s.filled = !s.filled
					s.invalidate()
				})
			case annotate.ToolText:
				ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
					for _, fs := range []float64{16, 20, 28, 36} {
						s.optionChip(c, fmt.Sprintf("%.0f", fs), s.resolvedFontSize() == fs, pal, func() {
							s.fontSize = fs
							s.invalidate()
						})
					}
				})
				s.toggleChip(c, nil, i18n.S("文字背景", "Text BG"), s.textHasBg, pal, func() {
					s.textHasBg = !s.textHasBg
					s.invalidate()
				})
			case annotate.ToolMosaic:
				ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
					for _, preset := range []struct {
						label string
						size  float64
					}{
						{i18n.S("细", "Fine"), 8},
						{i18n.S("中", "Med"), 12},
						{i18n.S("粗", "Coarse"), 20},
					} {
						s.optionChip(c, preset.label, s.resolvedMosaicBlock() == preset.size, pal, func() {
							s.mosaicBlock = preset.size
							s.invalidate()
						})
					}
				})
			}

			ui.Spacer(c)

			ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
				defLabel := captureActionLabel(s.plugin.Config().Capture.DefaultAction)
				telemetry := fmt.Sprintf("#%02X%02X%02X · %.0fpx", s.color.R(), s.color.G(), s.color.B(), s.thickness)
				ui.Text(c, telemetry).FontSize(10).FontWeight(500).SingleLine().TextColor(pal.HudMuted)
				pureui.HudKeycap(c, "Enter", pal)
				ui.Text(c, i18n.S("双击 · ", "Dbl-click · ")+defLabel).FontSize(10).FontWeight(500).SingleLine().TextColor(pal.HudMuted)
			})
		})
	})

	if b := dock.Bounds(); b.W > 0 && b.H > 0 {
		s.dockW, s.dockH = b.W, b.H
	}
}

func (s *captureSession) actionPill(c *ui.Context, icon *ui.SVG, label, tooltip string, danger bool, pal pureui.Palette, fn func()) {
	b := ui.ButtonBase(c).Height(28).Padding(0, 8).Radius(8).Gap(4).AlignItems(ui.Center).Label(label).Tooltip(tooltip)
	bg := pal.HudWell
	fg := pal.HudText
	if danger {
		if b.Pressed() {
			bg = pal.Danger.Alpha(0.30)
			fg = ui.RGB(254, 180, 180)
		} else if b.Hovered() {
			bg = pal.Danger.Alpha(0.22)
			fg = ui.RGB(254, 165, 165)
		}
	} else if b.Pressed() {
		bg = ui.RGBA(255, 255, 255, 0.18)
	} else if b.Hovered() {
		bg = ui.RGBA(255, 255, 255, 0.14)
	}
	b.Background(bg)
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(12, 12).TextColor(fg)
		}
		ui.Text(c, label).FontSize(11).FontWeight(500).SingleLine().TextColor(fg)
	})
	b.OnClick(fn)
}

func (s *captureSession) toggleChip(c *ui.Context, icon *ui.SVG, label string, on bool, pal pureui.Palette, fn func()) {
	b := ui.ButtonBase(c).Padding(4, 8).Radius(7).Gap(4).AlignItems(ui.Center).Label(label)
	bg := pal.HudWell
	fg := pal.HudText
	if on {
		bg = pal.AccentSoft
		fg = pal.AccentHover
	} else if b.Hovered() {
		bg = pal.HudHover
	}
	b.Background(bg)
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(12, 12).TextColor(fg)
		}
		ui.Text(c, label).FontSize(11).FontWeight(500).SingleLine().TextColor(fg)
	})
	b.OnClick(fn)
}

func (s *captureSession) optionChip(c *ui.Context, label string, on bool, pal pureui.Palette, fn func()) {
	b := ui.ButtonBase(c).Padding(3, 7).Radius(7).Center().Label(label)
	bg := pal.HudWell
	fg := pal.HudText
	if on {
		bg = pal.Accent
		fg = pal.AccentText
	} else if b.Hovered() {
		bg = pal.HudHover
	}
	b.Background(bg)
	b.Children(func() {
		ui.Text(c, label).FontSize(10).FontWeight(600).SingleLine().TextColor(fg)
	})
	b.OnClick(fn)
}

func (s *captureSession) textEditor(c *ui.Context, pal pureui.Palette) {
	x, y := s.toDIP(s.textOrigin.X, s.textOrigin.Y)
	const w = float32(308)
	const h = float32(128)
	x = clampf(x, 8, maxf(8, float32(s.contentW)-w-8))
	y = clampf(y+14, 8, maxf(8, float32(s.contentH)-h-8))
	colUI := ui.RGBA(s.color.R(), s.color.G(), s.color.B(), 1)
	borderCol := ui.RGBA(s.color.R(), s.color.G(), s.color.B(), 0.85)
	ui.Column(c).Absolute().Left(x).Top(y).Width(w).Padding(11).Gap(9).Radius(12).
		Material(glass.Glass{Tint: ui.RGBA(16, 19, 27, 0.94)}).
		Border(1.5, borderCol).
		InsetShadow(0, 1, 0, 0, pal.HudHighlight).
		Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.52)).Children(func() {
		input := ui.TextInput(c, &s.textValue).AutoFocus().Placeholder(i18n.S("输入文字，Enter 确认", "Enter text, press Enter"))
		if input.Submitted() {
			s.commitText()
			return
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(10, 10).Radius(5).Background(colUI).Border(1, ui.RGBA(255, 255, 255, 0.45))
			ui.Text(c, fmt.Sprintf("%.0f px", s.resolvedFontSize())).FontSize(10).TextColor(pal.HudMuted)
			ui.Spacer(c)
			ui.Button(c, i18n.S("取消", "Cancel")).FontSize(11).OnClick(s.cancelText)
			ui.PrimaryButton(c, i18n.S("确定", "OK")).FontSize(11).OnClick(s.commitText)
		})
	})
}

func (s *captureSession) commitText() {
	if strings.TrimSpace(s.textValue) != "" && s.doc != nil {
		text := s.textValue
		fs := s.resolvedFontSize()
		item := &annotate.Item{
			Tool: annotate.ToolText, Color: s.color, Thickness: s.thickness,
			Origin: s.rel(s.textOrigin), Text: text, FontSize: fs,
			HasBackground: s.textHasBg, Background: core.DefaultTextBackground,
			Measured: annotate.MeasureText(text, fs),
		}
		s.doc.Add(item)
	}
	s.textValue = ""
	s.textEditing = false
	s.invalidate()
}

func (s *captureSession) cancelText() {
	s.textValue = ""
	s.textEditing = false
	s.invalidate()
}

func (s *captureSession) undo() {
	if s.doc != nil {
		s.doc.Undo()
		s.invalidate()
	}
}

func (s *captureSession) redo() {
	if s.doc != nil {
		s.doc.Redo()
		s.invalidate()
	}
}

func (s *captureSession) resetSelection() {
	s.selection = core.RectI{}
	s.doc = nil
	s.draft = nil
	s.textEditing = false
	s.textValue = ""
	s.hoverHandle = handleNone
	s.activeHandle = handleNone
	s.status = ""
	s.invalidate()
}

func (s *captureSession) cancel() {
	s.closed = true
	if s.win != nil {
		s.win.Close()
	}
}

func (s *captureSession) finish(target string) {
	if s.finishing || !s.hasSelection() {
		return
	}
	s.finishing = true
	crop := image.Rect(
		s.selection.X-s.monitor.Bounds.X, s.selection.Y-s.monitor.Bounds.Y,
		s.selection.X-s.monitor.Bounds.X+s.selection.W, s.selection.Y-s.monitor.Bounds.Y+s.selection.H,
	)
	complete := func(res *ShotResult, err error) {
		if s.closed {
			return
		}
		if err != nil {
			s.finishing = false
			s.status = i18n.S("处理失败: ", "Processing failed: ") + err.Error()
			s.invalidate()
			return
		}
		defAction := s.plugin.Config().Capture.DefaultAction
		needSave := target == "save" ||
			(target == "done" && (defAction == config.CaptureActionSave || defAction == config.CaptureActionCopyAndSave))
		if needSave && res.SavedPath == "" {
			s.finishing = false
			s.status = i18n.S("保存失败，请检查保存目录", "Save failed, please check save folder")
			s.invalidate()
			return
		}
		if res.ShouldPin && res.PinImage != nil {
			if !s.plugin.headless() {
				s.plugin.ShowPinImage(res.PinImage)
			}
		} else if !s.plugin.headless() && res.SavedPath != "" && (target == "save" || target == "done") {
			s.plugin.notify(i18n.S("截图已保存", "Screenshot Saved"), res.SavedPath)
		}
		s.closeWindow()
	}
	if s.plugin.headless() {
		res, err := s.plugin.capture.Complete(s.frame, crop, s.doc, target)
		complete(res, err)
		return
	}
	go func() {
		res, err := s.plugin.capture.Complete(s.frame, crop, s.doc, target)
		mygo.RunOnMain(func() { complete(res, err) })
	}()
}

func captureActionLabel(action string) string {
	switch action {
	case config.CaptureActionSave:
		return i18n.S("保存到文件", "Save")
	case config.CaptureActionCopyAndSave:
		return i18n.S("复制并保存", "Copy & Save")
	case config.CaptureActionPin:
		return i18n.S("贴到桌面", "Pin")
	default:
		return i18n.S("复制到剪贴板", "Copy")
	}
}

func (s *captureSession) closeWindow() {
	s.closed = true
	if s.win != nil {
		s.win.Close()
	}
}

// Close shuts down the capture overlay.
func (s *captureSession) Close() {
	s.closeWindow()
}

// CaptureWholeScreen captures the display under the cursor, copies it and
// records it without opening the overlay.
func (a *Plugin) CaptureWholeScreen() {
	mon, err := platform.MonitorAtCursor()
	if err != nil {
		if a.log() != nil { a.log().Errorf("整屏截图：找不到显示器: %v", err) }
		return
	}
	settings := a.Config()
	go func() {
		frame, err := platform.CaptureMonitor(mon, settings.Capture.IncludeCursor)
		if err != nil {
			if a.log() != nil { a.log().Errorf("整屏截图：冻结屏幕失败: %v", err) }
			mygo.RunOnMain(func() { a.notify(i18n.S("截图失败", "Capture Failed"), err.Error()) })
			return
		}
		doc := annotate.NewDocument(frame.Bounds().Dx(), frame.Bounds().Dy())
		res, err := a.capture.Complete(frame, frame.Bounds(), doc, "copy")
		if err != nil && a.log() != nil {
			a.log().Errorf("整屏截图：保存失败: %v", err)
		}
		mygo.RunOnMain(func() {
			if err != nil {
				a.notify(i18n.S("截图失败", "Capture Failed"), err.Error())
				return
			}
			body := fmt.Sprintf("%d × %d", res.Width, res.Height)
			if res.SavedPath != "" {
				body += i18n.S("，已保存到 ", "\nSaved to ") + res.SavedPath
			}
			a.notify(i18n.S("截图已复制", "Screenshot Copied"), body)
		})
	}()
}

func (s *captureSession) invalidate() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

func clampf(v, lo, hi float32) float32 {
	if lo > hi {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func absf(a float32) float32 {
	if a < 0 {
		return -a
	}
	return a
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if lo > hi {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func toolIcon(t annotate.Tool) *ui.SVG {
	switch t {
	case annotate.ToolRect:
		return pureui.IconToolRect
	case annotate.ToolEllipse:
		return pureui.IconToolEllipse
	case annotate.ToolLine:
		return pureui.IconToolLine
	case annotate.ToolArrow:
		return pureui.IconToolArrow
	case annotate.ToolPen:
		return pureui.IconToolPen
	case annotate.ToolHighlight:
		return pureui.IconToolHighlight
	case annotate.ToolText:
		return pureui.IconToolText
	case annotate.ToolMosaic:
		return pureui.IconToolMosaic
	case annotate.ToolBadge:
		return pureui.IconToolBadge
	default:
		return pureui.IconToolRect
	}
}