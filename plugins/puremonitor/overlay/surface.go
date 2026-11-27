package overlay

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"

	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/puremonitor/collector"
)

type fontRole int

const (
	fontUI fontRole = iota
	fontUIBold
	fontMono
)

type fontKey struct {
	role fontRole
	size int // integer half-points (e.g. px*2)
}

var (
	renderMu     sync.Mutex
	fontInitOnce sync.Once
	uiFont       *opentype.Font
	uiBoldFont   *opentype.Font
	monoFont     *opentype.Font
	fontMu       sync.Mutex
	fontFaces    = map[fontKey]font.Face{}
)

func initSystemFonts() {
	fontInitOnce.Do(func() {
		winDir := os.Getenv("WINDIR")
		if winDir == "" {
			winDir = `C:\Windows`
		}
		fontsDir := filepath.Join(winDir, "Fonts")

		loadSingle := func(names ...string) *opentype.Font {
			for _, name := range names {
				f, err := os.Open(filepath.Join(fontsDir, name))
				if err != nil {
					continue
				}
				if col, err := opentype.ParseCollectionReaderAt(f); err == nil && col.NumFonts() > 0 {
					if fontObj, err := col.Font(0); err == nil {
						return fontObj
					}
				}
				if fontObj, err := opentype.ParseReaderAt(f); err == nil {
					return fontObj
				}
				_ = f.Close()
			}
			return nil
		}

		uiFont = loadSingle("msyh.ttc", "segoeui.ttf", "simsun.ttc")
		uiBoldFont = loadSingle("msyhbd.ttc", "segoeuib.ttf", "msyh.ttc", "segoeui.ttf")
		monoFont = loadSingle("consolab.ttf", "consola.ttf", "segoeuib.ttf", "msyh.ttc")
	})
}

func getFace(role fontRole, px float64) font.Face {
	initSystemFonts()
	if px < 8 {
		px = 8
	}
	key := fontKey{role: role, size: int(math.Round(px * 2))}
	fontMu.Lock()
	defer fontMu.Unlock()
	if f, ok := fontFaces[key]; ok {
		return f
	}
	var src *opentype.Font
	switch role {
	case fontUIBold:
		src = uiBoldFont
	case fontMono:
		src = monoFont
	default:
		src = uiFont
	}
	if src == nil {
		return basicfont.Face7x13
	}
	face, err := opentype.NewFace(src, &opentype.FaceOptions{
		Size:    px,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return basicfont.Face7x13
	}
	fontFaces[key] = face
	return face
}

// OverlayTheme holds color tokens for layered taskbar, floating, and HUD windows.
type OverlayTheme struct {
	Dark         bool
	Bg           color.NRGBA
	BgAlt        color.NRGBA
	Border       color.NRGBA
	Highlight    color.NRGBA
	Text         color.NRGBA
	Muted        color.NRGBA
	Accent       color.NRGBA
	RamAccent    color.NRGBA
	TempAccent   color.NRGBA
	Success      color.NRGBA
	Warn         color.NRGBA
	Crit         color.NRGBA
	NetDown      color.NRGBA
	NetUp        color.NRGBA
	BarTrack     color.NRGBA
}

// ThemePalette builds an OverlayTheme for dark or light mode and opacity [0..1].
func ThemePalette(dark bool, opacity float64, taskbarEmbedded bool) OverlayTheme {
	if opacity <= 0 {
		opacity = 0.90
	}
	if opacity > 1 {
		opacity = 1
	}
	alpha := uint8(math.Round(opacity * 242))
	if taskbarEmbedded {
		alpha = 205
	}
	if dark {
		return OverlayTheme{
			Dark:       true,
			Bg:         color.NRGBA{R: 18, G: 22, B: 31, A: alpha},
			BgAlt:      color.NRGBA{R: 28, G: 34, B: 48, A: alpha},
			Border:     color.NRGBA{R: 255, G: 255, B: 255, A: 28},
			Highlight:  color.NRGBA{R: 255, G: 255, B: 255, A: 14},
			Text:       color.NRGBA{R: 241, G: 245, B: 249, A: 255},
			Muted:      color.NRGBA{R: 148, G: 163, B: 184, A: 240},
			Accent:     color.NRGBA{R: 56, G: 189, B: 248, A: 255},
			RamAccent:  color.NRGBA{R: 167, G: 139, B: 250, A: 255},
			TempAccent: color.NRGBA{R: 251, G: 146, B: 60, A: 255},
			Success:    color.NRGBA{R: 52, G: 211, B: 153, A: 255},
			Warn:       color.NRGBA{R: 251, G: 191, B: 36, A: 255},
			Crit:       color.NRGBA{R: 248, G: 113, B: 113, A: 255},
			NetDown:    color.NRGBA{R: 52, G: 211, B: 153, A: 255},
			NetUp:      color.NRGBA{R: 96, G: 165, B: 250, A: 255},
			BarTrack:   color.NRGBA{R: 255, G: 255, B: 255, A: 26},
		}
	}
	return OverlayTheme{
		Dark:       false,
		Bg:         color.NRGBA{R: 248, G: 250, B: 252, A: alpha},
		BgAlt:      color.NRGBA{R: 241, G: 245, B: 249, A: alpha},
		Border:     color.NRGBA{R: 15, G: 23, B: 42, A: 26},
		Highlight:  color.NRGBA{R: 255, G: 255, B: 255, A: 180},
		Text:       color.NRGBA{R: 15, G: 23, B: 42, A: 255},
		Muted:      color.NRGBA{R: 100, G: 116, B: 139, A: 245},
		Accent:     color.NRGBA{R: 14, G: 165, B: 233, A: 255},
		RamAccent:  color.NRGBA{R: 124, G: 58, B: 237, A: 255},
		TempAccent: color.NRGBA{R: 234, G: 88, B: 12, A: 255},
		Success:    color.NRGBA{R: 16, G: 185, B: 129, A: 255},
		Warn:       color.NRGBA{R: 217, G: 119, B: 6, A: 255},
		Crit:       color.NRGBA{R: 220, G: 38, B: 38, A: 255},
		NetDown:    color.NRGBA{R: 5, G: 150, B: 105, A: 255},
		NetUp:      color.NRGBA{R: 37, G: 99, B: 235, A: 255},
		BarTrack:   color.NRGBA{R: 15, G: 23, B: 42, A: 24},
	}
}

func levelColor(lvl collector.MetricStatusLevel, id collector.MetricID, th OverlayTheme) color.NRGBA {
	switch lvl {
	case collector.StatusCrit:
		return th.Crit
	case collector.StatusWarn:
		return th.Warn
	default:
		switch id {
		case collector.MetricNetDown:
			return th.NetDown
		case collector.MetricNetUp:
			return th.NetUp
		case collector.MetricFPS, collector.MetricFPSAvg, collector.MetricFPS1Low:
			return th.Success
		default:
			return th.Text
		}
	}
}

func formatOpts(cfg config.PureMonitorConfig) (collector.FormatOptions, collector.MetricAlertThresholds) {
	return collector.FormatOptions{
			UnitSpeed: cfg.UnitSpeed,
			TempUnit:  cfg.TempUnit,
		}, collector.MetricAlertThresholds{
			CPUPct: cfg.WarnCPUPct,
			RAMPct: cfg.WarnRAMPct,
			TempC:  cfg.WarnTempC,
		}
}

func netMetricInfo(id collector.MetricID, compact bool, th OverlayTheme) (ok bool, down bool, tag string, col color.NRGBA) {
	switch id {
	case collector.MetricNetDown:
		if compact {
			return true, true, i18n.S("下", "DN"), th.NetDown
		}
		return true, true, i18n.S("下载", "Down"), th.NetDown
	case collector.MetricNetUp:
		if compact {
			return true, false, i18n.S("上", "UP"), th.NetUp
		}
		return true, false, i18n.S("上传", "Up"), th.NetUp
	default:
		return false, false, "", color.NRGBA{}
	}
}

func drawNetArrowIcon(dc *gg.Context, cx, cy, size float64, down bool, col color.NRGBA) {
	half := size * 0.48
	stemW := math.Max(1.6, size*0.26)
	headW := size * 0.44
	dc.SetColor(col)
	if down {
		dc.DrawRectangle(cx-stemW/2, cy-half, stemW, half*1.08)
		dc.Fill()
		dc.MoveTo(cx-headW, cy-half*0.02)
		dc.LineTo(cx+headW, cy-half*0.02)
		dc.LineTo(cx, cy+half)
		dc.ClosePath()
		dc.Fill()
	} else {
		dc.MoveTo(cx, cy-half)
		dc.LineTo(cx+headW, cy+half*0.02)
		dc.LineTo(cx-headW, cy+half*0.02)
		dc.ClosePath()
		dc.Fill()
		dc.DrawRectangle(cx-stemW/2, cy-half*0.08, stemW, half*1.08)
		dc.Fill()
	}
}

func measureTextWidth(face font.Face, s string) float64 {
	if face == nil || s == "" {
		return 0
	}
	return float64(font.MeasureString(face, s)) / 64.0
}

func sampleWorstMetricVal(id collector.MetricID, fOpts collector.FormatOptions) string {
	switch id {
	case collector.MetricNetDown, collector.MetricNetUp, collector.MetricDiskRead, collector.MetricDiskWrite:
		if id != collector.MetricDiskRead && id != collector.MetricDiskWrite && strings.EqualFold(fOpts.UnitSpeed, "mbps") {
			return "999Mb"
		}
		return "99.9M/s"
	case collector.MetricCPUTemp, collector.MetricGPUTemp, collector.MetricDiskTemp:
		if strings.EqualFold(fOpts.TempUnit, "F") {
			return "199°F"
		}
		return "99°C"
	case collector.MetricCPUFreq:
		return "5.99G"
	case collector.MetricRAMGB, collector.MetricVRAM:
		return "99.9G"
	case collector.MetricCPUPower, collector.MetricGPUPower:
		return "999W"
	case collector.MetricFrametime:
		return "99.9ms"
	case collector.MetricFPS, collector.MetricFPSAvg, collector.MetricFPS1Low, collector.MetricFPS01Low:
		return "999"
	default:
		return "100%"
	}
}

func categoryAccentColor(id collector.MetricID, lvl collector.MetricStatusLevel, th OverlayTheme) color.NRGBA {
	if lvl == collector.StatusCrit {
		return th.Crit
	}
	if lvl == collector.StatusWarn {
		return th.Warn
	}
	switch id {
	case collector.MetricRAM, collector.MetricRAMGB:
		return th.RamAccent
	case collector.MetricGPU:
		return th.Success
	case collector.MetricVRAM:
		return th.Accent
	case collector.MetricCPUTemp, collector.MetricGPUTemp, collector.MetricDiskTemp:
		return th.TempAccent
	default:
		return th.Accent
	}
}

// RenderTaskbarImage renders the taskbar monitor surface into an RGBA image at physical scale.
func RenderTaskbarImage(snap collector.SystemSnapshot, cfg config.PureMonitorConfig, dark bool, scale float64) *image.RGBA {
	renderMu.Lock()
	defer renderMu.Unlock()
	if scale <= 0 {
		scale = 1
	}
	metrics := cfg.TaskbarMetrics
	if len(metrics) == 0 {
		metrics = []string{"cpu", "ram", "net_down", "net_up"}
	}
	fOpts, thres := formatOpts(cfg)
	th := ThemePalette(dark, 0.92, true)

	fontPx := float64(cfg.FontSize) * scale
	if fontPx < 10 {
		fontPx = 10
	}
	singleRow := strings.EqualFold(cfg.TaskbarLayout, "single_row")

	type cell struct {
		id    collector.MetricID
		label string
		val   string
		lvl   collector.MetricStatusLevel
	}
	cells := make([]cell, 0, len(metrics))
	for _, raw := range metrics {
		id := collector.MetricID(raw)
		lbl, val, lvl := collector.FormatMetricValue(id, snap, fOpts, thres)
		cells = append(cells, cell{id: id, label: lbl, val: val, lvl: lvl})
	}

	lblFace := getFace(fontUIBold, fontPx*0.80)
	valFace := getFace(fontMono, fontPx*0.88)

	var cols int
	if singleRow {
		cols = len(cells)
	} else {
		cols = (len(cells) + 1) / 2
	}
	if cols < 1 {
		cols = 1
	}

	cellPadX := 5.5 * scale
	colGap := 5.0 * scale
	arrowW := 7.5 * scale
	arrowGap := 3.0 * scale
	colWidths := make([]float64, cols)
	for i, c := range cells {
		col := i
		if !singleRow {
			col = i / 2
		}
		var lblW float64
		if ok, _, tag, _ := netMetricInfo(c.id, true, th); ok {
			lblW = arrowW + arrowGap + measureTextWidth(lblFace, tag)
		} else {
			lblW = measureTextWidth(lblFace, c.label)
		}
		valW := math.Max(
			measureTextWidth(valFace, sampleWorstMetricVal(c.id, fOpts)),
			measureTextWidth(valFace, c.val),
		)
		cw := math.Ceil(cellPadX*2 + lblW + colGap + valW)
		if cw > colWidths[col] {
			colWidths[col] = cw
		}
	}

	padX := 5.0 * scale
	totalW := padX * 2
	for _, cw := range colWidths {
		totalW += cw
	}
	w := int(math.Round(totalW))
	var h int
	if singleRow {
		h = int(math.Round(26 * scale))
	} else {
		h = int(math.Round(34 * scale))
	}
	if w < 64 {
		w = 64
	}
	if h < 22 {
		h = 22
	}

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	dc := gg.NewContextForRGBA(rgba)

	rad := 6 * scale
	dc.SetColor(th.Bg)
	dc.DrawRoundedRectangle(0.5, 0.5, float64(w)-1, float64(h)-1, rad)
	dc.Fill()
	dc.SetColor(th.Border)
	dc.SetLineWidth(math.Max(1, scale*0.85))
	dc.DrawRoundedRectangle(0.5, 0.5, float64(w)-1, float64(h)-1, rad)
	dc.Stroke()
	if th.Dark {
		dc.SetColor(th.Highlight)
		dc.SetLineWidth(math.Max(1, scale*0.75))
		dc.DrawLine(rad, 1, float64(w)-rad, 1)
		dc.Stroke()
	}

	drawSep := func(x float64) {
		dc.SetColor(th.Border)
		dc.SetLineWidth(math.Max(1, scale*0.8))
		dc.DrawLine(x, 6*scale, x, float64(h)-6*scale)
		dc.Stroke()
	}

	drawCell := func(x, cw, cy float64, c cell) {
		if ok, down, tag, nCol := netMetricInfo(c.id, true, th); ok {
			drawNetArrowIcon(dc, x+cellPadX+arrowW*0.45, cy+0.5*scale, arrowW, down, nCol)
			dc.SetFontFace(lblFace)
			dc.SetColor(nCol)
			dc.DrawStringAnchored(tag, x+cellPadX+arrowW+arrowGap, cy, 0, 0.36)
		} else {
			dc.SetFontFace(lblFace)
			dc.SetColor(th.Muted)
			dc.DrawStringAnchored(c.label, x+cellPadX, cy, 0, 0.36)
		}
		dc.SetFontFace(valFace)
		dc.SetColor(levelColor(c.lvl, c.id, th))
		dc.DrawStringAnchored(c.val, x+cw-cellPadX, cy, 1, 0.36)
	}

	colX := make([]float64, cols)
	curX := padX
	for i, cw := range colWidths {
		colX[i] = curX
		if i > 0 {
			drawSep(curX)
		}
		curX += cw
	}

	if singleRow {
		cy := float64(h) * 0.52
		for i, c := range cells {
			drawCell(colX[i], colWidths[i], cy, c)
		}
		return rgba
	}

	row1Y := float64(h) * 0.30
	row2Y := float64(h) * 0.72
	for i, c := range cells {
		col := i / 2
		row := i % 2
		y := row1Y
		if len(cells)%2 == 1 && i == len(cells)-1 {
			// Vertically center lone cell in the final column rather than leaving half a blank column.
			y = float64(h) * 0.52
		} else if row == 1 {
			y = row2Y
		}
		drawCell(colX[col], colWidths[col], y, c)
	}
	return rgba
}

// RenderFloatingImage renders either the collapsed edge pill or expanded floating monitor card.
func RenderFloatingImage(snap collector.SystemSnapshot, cfg config.PureMonitorConfig, dark bool, scale float64, collapsed bool) *image.RGBA {
	return RenderFloatingImageAt(snap, cfg, dark, scale, collapsed, "right")
}

// RenderFloatingImageAt renders the floating monitor card or an edge-aware collapsed pill ("left", "right", "top", or "").
func RenderFloatingImageAt(snap collector.SystemSnapshot, cfg config.PureMonitorConfig, dark bool, scale float64, collapsed bool, dockEdge string) *image.RGBA {
	renderMu.Lock()
	defer renderMu.Unlock()
	if scale <= 0 {
		scale = 1
	}
	fOpts, thres := formatOpts(cfg)
	th := ThemePalette(dark, cfg.FloatOpacity, false)
	fontPx := float64(cfg.FontSize) * scale
	if fontPx < 10 {
		fontPx = 10
	}

	if collapsed {
		return renderCollapsedFloatingPill(snap, cfg, fOpts, thres, th, fontPx, scale, dockEdge)
	}

	metrics := cfg.FloatMetrics
	if len(metrics) == 0 {
		metrics = []string{"cpu", "cpu_temp", "ram", "gpu", "gpu_temp", "net_down", "net_up"}
	}
	headerH := 20 * scale
	topGap := 3 * scale
	rowH := 17.5 * scale
	padX := 9 * scale
	padBottom := 6 * scale
	w := int(math.Round(160 * scale))
	h := int(math.Round(headerH + topGap + float64(len(metrics))*rowH + padBottom))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	dc := gg.NewContextForRGBA(rgba)
	rad := 9 * scale
	dc.SetColor(th.Bg)
	dc.DrawRoundedRectangle(0.5, 0.5, float64(w)-1, float64(h)-1, rad)
	dc.Fill()
	dc.SetColor(th.Border)
	dc.SetLineWidth(math.Max(1, scale))
	dc.DrawRoundedRectangle(0.5, 0.5, float64(w)-1, float64(h)-1, rad)
	dc.Stroke()
	if th.Dark {
		dc.SetColor(th.Highlight)
		dc.SetLineWidth(math.Max(1, scale*0.75))
		dc.DrawLine(rad, 1, float64(w)-rad, 1)
		dc.Stroke()
	}

	// Sleek header strip + hairline divider
	dc.SetColor(th.Accent)
	dc.DrawCircle(padX+2.5*scale, headerH*0.52, 2.4*scale)
	dc.Fill()
	dc.SetFontFace(getFace(fontUIBold, fontPx*0.80))
	dc.SetColor(th.Text)
	dc.DrawStringAnchored("PureMonitor", padX+8.5*scale, headerH*0.52, 0, 0.36)
	if cfg.FloatLocked {
		dc.SetFontFace(getFace(fontUI, fontPx*0.72))
		dc.SetColor(th.Muted)
		dc.DrawStringAnchored("LOCK", float64(w)-padX, headerH*0.52, 1, 0.36)
	}
	dc.SetColor(th.Border)
	dc.SetLineWidth(math.Max(1, scale*0.75))
	dc.DrawLine(padX, headerH, float64(w)-padX, headerH)
	dc.Stroke()

	lblFace := getFace(fontUIBold, fontPx*0.82)
	netLblFace := getFace(fontUIBold, fontPx*0.82)
	valFace := getFace(fontMono, fontPx*0.88)

	barX := padX + 35*scale
	valReserve := 35 * scale
	barW := float64(w) - padX - valReserve - barX

	for i, raw := range metrics {
		id := collector.MetricID(raw)
		lbl, val, lvl := collector.FormatMetricValue(id, snap, fOpts, thres)
		cy := headerH + topGap + float64(i)*rowH + rowH*0.50

		if ok, down, tag, nCol := netMetricInfo(id, false, th); ok {
			drawNetArrowIcon(dc, padX+4.0*scale, cy+0.5*scale, 8.0*scale, down, nCol)
			dc.SetFontFace(netLblFace)
			dc.SetColor(nCol)
			dc.DrawStringAnchored(tag, padX+11.5*scale, cy, 0, 0.36)
		} else {
			dc.SetFontFace(lblFace)
			dc.SetColor(th.Muted)
			dc.DrawStringAnchored(lbl, padX, cy, 0, 0.36)
		}

		// Span mini utilization bar neatly across available middle column for percentage metrics.
		if pct, hasBar := metricPercent(id, snap); hasBar && barW > 12*scale {
			barH := 3.5 * scale
			by := cy - barH/2 + 0.5*scale
			dc.SetColor(th.BarTrack)
			dc.DrawRoundedRectangle(barX, by, barW, barH, barH/2)
			dc.Fill()
			fillW := barW * math.Min(1, math.Max(0, pct/100.0))
			if fillW > 0 {
				dc.SetColor(categoryAccentColor(id, lvl, th))
				dc.DrawRoundedRectangle(barX, by, math.Max(barH, fillW), barH, barH/2)
				dc.Fill()
			}
		}

		dc.SetFontFace(valFace)
		dc.SetColor(levelColor(lvl, id, th))
		dc.DrawStringAnchored(val, float64(w)-padX, cy, 1, 0.36)
	}
	return rgba
}

func collapsedPillMetrics(cfg config.PureMonitorConfig) []string {
	src := cfg.FloatCollapsedMetrics
	if len(src) == 0 {
		src = []string{"cpu", "ram"}
	}
	out := make([]string, 0, 2)
	for _, m := range src {
		if len(out) >= 2 {
			break
		}
		out = append(out, m)
	}
	return out
}

func drawAsymmetricRoundedRect(dc *gg.Context, x, y, w, h, rTL, rTR, rBR, rBL float64) {
	const kappa = 0.55228475
	dc.MoveTo(x+rTL, y)
	dc.LineTo(x+w-rTR, y)
	if rTR > 0 {
		k := rTR * kappa
		dc.CubicTo(x+w-rTR+k, y, x+w, y+rTR-k, x+w, y+rTR)
	}
	dc.LineTo(x+w, y+h-rBR)
	if rBR > 0 {
		k := rBR * kappa
		dc.CubicTo(x+w, y+h-rBR+k, x+w-rBR+k, y+h, x+w-rBR, y+h)
	}
	dc.LineTo(x+rBL, y+h)
	if rBL > 0 {
		k := rBL * kappa
		dc.CubicTo(x+rBL-k, y+h, x, y+h-rBL+k, x, y+h-rBL)
	}
	dc.LineTo(x, y+rTL)
	if rTL > 0 {
		k := rTL * kappa
		dc.CubicTo(x, y+rTL-k, x+rTL-k, y, x+rTL, y)
	}
	dc.ClosePath()
}

func renderCollapsedFloatingPill(
	snap collector.SystemSnapshot,
	cfg config.PureMonitorConfig,
	fOpts collector.FormatOptions,
	thres collector.MetricAlertThresholds,
	th OverlayTheme,
	fontPx, scale float64,
	dockEdge string,
) *image.RGBA {
	edge := strings.ToLower(strings.TrimSpace(dockEdge))
	metrics := collapsedPillMetrics(cfg)

	type pillItem struct {
		id  collector.MetricID
		lbl string
		val string
		lvl collector.MetricStatusLevel
	}
	items := make([]pillItem, 0, len(metrics))
	for _, raw := range metrics {
		id := collector.MetricID(raw)
		lbl, val, lvl := collector.FormatMetricValue(id, snap, fOpts, thres)
		items = append(items, pillItem{id: id, lbl: lbl, val: val, lvl: lvl})
	}

	lblFace := getFace(fontUIBold, fontPx*0.74)
	valFace := getFace(fontMono, fontPx*0.88)
	rad := 10 * scale
	gripCol := th.Muted
	gripCol.A = 95

	// Top-docked edge renders as a sleek Dynamic-Island horizontal mini capsule.
	if edge == "top" {
		padX := 10 * scale
		itemGap := 12 * scale
		gripH := 5 * scale
		h := int(math.Round(27 * scale))

		itemWidths := make([]float64, len(items))
		totalW := padX * 2
		for i, it := range items {
			var lw float64
			if ok, _, tag, _ := netMetricInfo(it.id, true, th); ok {
				lw = 7.0*scale + 2.5*scale + measureTextWidth(lblFace, tag)
			} else {
				lw = 3.5*scale + 3.0*scale + measureTextWidth(lblFace, it.lbl)
			}
			vw := math.Max(measureTextWidth(valFace, it.val), 24*scale)
			iw := lw + 4.5*scale + vw
			itemWidths[i] = iw
			totalW += iw
			if i > 0 {
				totalW += itemGap
			}
		}
		w := int(math.Round(math.Max(68*scale, totalW)))
		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		dc := gg.NewContextForRGBA(rgba)

		dc.SetColor(th.Bg)
		drawAsymmetricRoundedRect(dc, 0.5, -1, float64(w)-1, float64(h)+0.5, 0, 0, rad, rad)
		dc.Fill()
		dc.SetColor(th.Border)
		dc.SetLineWidth(math.Max(1, scale))
		drawAsymmetricRoundedRect(dc, 0.5, -1, float64(w)-1, float64(h)+0.5, 0, 0, rad, rad)
		dc.Stroke()

		gripW := 16 * scale
		dc.SetColor(gripCol)
		dc.DrawRoundedRectangle((float64(w)-gripW)/2, 2.2*scale, gripW, 2.0*scale, 1.0*scale)
		dc.Fill()

		cy := gripH + (float64(h)-gripH)*0.48
		curX := padX
		for i, it := range items {
			if i > 0 {
				sx := curX - itemGap*0.5
				dc.SetColor(th.Border)
				dc.SetLineWidth(math.Max(1, scale*0.75))
				dc.DrawLine(sx, cy-5.5*scale, sx, cy+5.5*scale)
				dc.Stroke()
			}
			if ok, down, tag, nCol := netMetricInfo(it.id, true, th); ok {
				drawNetArrowIcon(dc, curX+3.2*scale, cy+0.4*scale, 6.8*scale, down, nCol)
				dc.SetFontFace(lblFace)
				dc.SetColor(nCol)
				dc.DrawStringAnchored(tag, curX+9.0*scale, cy, 0, 0.36)
			} else {
				dc.SetColor(categoryAccentColor(it.id, it.lvl, th))
				dc.DrawRoundedRectangle(curX, cy-3.2*scale, 2.4*scale, 6.4*scale, 1.2*scale)
				dc.Fill()
				dc.SetFontFace(lblFace)
				dc.SetColor(th.Muted)
				dc.DrawStringAnchored(it.lbl, curX+5.2*scale, cy, 0, 0.36)
			}
			dc.SetFontFace(valFace)
			dc.SetColor(levelColor(it.lvl, it.id, th))
			dc.DrawStringAnchored(it.val, curX+itemWidths[i], cy, 1, 0.36)
			curX += itemWidths[i] + itemGap
		}
		return rgba
	}

	// Side-docked ("left" / "right") or undocked ("") vertical edge tab.
	maxContentW := 36 * scale
	for _, it := range items {
		var lw float64
		if ok, _, tag, _ := netMetricInfo(it.id, true, th); ok {
			lw = 7.0*scale + 3.0*scale + measureTextWidth(lblFace, tag)
		} else {
			lw = 4.8*scale + measureTextWidth(lblFace, it.lbl)
		}
		vw := measureTextWidth(valFace, it.val)
		if lw > maxContentW {
			maxContentW = lw
		}
		if vw > maxContentW {
			maxContentW = vw
		}
	}
	padLeft := 8.0 * scale
	padRight := 8.0 * scale
	gripZone := 4.0 * scale
	if edge == "left" {
		padLeft += gripZone
	} else if edge == "right" {
		padRight += gripZone
	}
	w := int(math.Round(math.Max(54*scale, math.Min(78*scale, padLeft+maxContentW+padRight))))

	padY := 5.5 * scale
	slotH := 26.5 * scale
	slotGap := 5.5 * scale
	gaps := 0
	if len(items) > 1 {
		gaps = len(items) - 1
	}
	h := int(math.Round(padY*2 + float64(len(items))*slotH + float64(gaps)*slotGap))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	dc := gg.NewContextForRGBA(rgba)

	var rTL, rTR, rBR, rBL float64
	rx, ry, rw, rh := 0.5, 0.5, float64(w)-1, float64(h)-1
	switch edge {
	case "right":
		rTL, rBL = rad, rad
		rw = float64(w) + 1
	case "left":
		rTR, rBR = rad, rad
		rx = -1.5
		rw = float64(w) + 1
	default:
		rTL, rTR, rBR, rBL = rad, rad, rad, rad
	}

	dc.SetColor(th.Bg)
	drawAsymmetricRoundedRect(dc, rx, ry, rw, rh, rTL, rTR, rBR, rBL)
	dc.Fill()
	dc.SetColor(th.Border)
	dc.SetLineWidth(math.Max(1, scale))
	drawAsymmetricRoundedRect(dc, rx, ry, rw, rh, rTL, rTR, rBR, rBL)
	dc.Stroke()
	if th.Dark {
		dc.SetColor(th.Highlight)
		dc.SetLineWidth(math.Max(1, scale*0.75))
		dc.DrawLine(math.Max(3*scale, rTL), 1.0, float64(w)-math.Max(3*scale, rTR), 1.0)
		dc.Stroke()
	}

	if edge == "right" || edge == "left" {
		gripH := math.Min(float64(h)*0.36, 16*scale)
		gripX := float64(w) - 4.5*scale
		if edge == "left" {
			gripX = 2.5 * scale
		}
		dc.SetColor(gripCol)
		dc.DrawRoundedRectangle(gripX, (float64(h)-gripH)/2, 2.0*scale, gripH, 1.0*scale)
		dc.Fill()
	}

	contentL := padLeft
	contentR := float64(w) - padRight
	contentW := math.Max(12*scale, contentR-contentL)
	if edge == "right" {
		contentL -= 0.5 * scale
	}

	for i, it := range items {
		sy := padY + float64(i)*(slotH+slotGap)
		if i > 0 {
			divY := sy - slotGap*0.5
			dc.SetColor(th.Border)
			dc.SetLineWidth(math.Max(1, scale*0.75))
			dc.DrawLine(contentL, divY, contentR, divY)
			dc.Stroke()
		}

		pct, hasBar := metricPercent(it.id, snap)
		lblY := sy + 6.5*scale
		valY := sy + 18.0*scale
		if !hasBar {
			lblY = sy + 7.5*scale
			valY = sy + 19.5*scale
		}

		if ok, down, tag, nCol := netMetricInfo(it.id, true, th); ok {
			drawNetArrowIcon(dc, contentL+3.2*scale, lblY+0.3*scale, 6.6*scale, down, nCol)
			dc.SetFontFace(lblFace)
			dc.SetColor(nCol)
			dc.DrawStringAnchored(tag, contentL+8.8*scale, lblY, 0, 0.36)
		} else {
			dc.SetColor(categoryAccentColor(it.id, it.lvl, th))
			dc.DrawRoundedRectangle(contentL, lblY-3.2*scale, 2.2*scale, 6.4*scale, 1.1*scale)
			dc.Fill()
			dc.SetFontFace(lblFace)
			dc.SetColor(th.Muted)
			dc.DrawStringAnchored(it.lbl, contentL+4.8*scale, lblY, 0, 0.36)
		}

		dc.SetFontFace(valFace)
		dc.SetColor(levelColor(it.lvl, it.id, th))
		dc.DrawStringAnchored(it.val, contentL, valY, 0, 0.36)

		if hasBar {
			barH := 2.2 * scale
			by := sy + slotH - barH
			dc.SetColor(th.BarTrack)
			dc.DrawRoundedRectangle(contentL, by, contentW, barH, barH/2)
			dc.Fill()
			fillW := contentW * math.Min(1, math.Max(0, pct/100.0))
			if fillW > 0 {
				dc.SetColor(categoryAccentColor(it.id, it.lvl, th))
				dc.DrawRoundedRectangle(contentL, by, math.Max(barH, fillW), barH, barH/2)
				dc.Fill()
			}
		}
	}
	return rgba
}

func metricPercent(id collector.MetricID, snap collector.SystemSnapshot) (float64, bool) {
	switch id {
	case collector.MetricCPU:
		return snap.CPUPercent, true
	case collector.MetricRAM:
		return snap.RAMPct, true
	case collector.MetricGPU:
		if g, ok := snap.PrimaryGPU(); ok {
			return g.Load3D, true
		}
	case collector.MetricVRAM:
		if g, ok := snap.PrimaryGPU(); ok && g.VRAMTotalMB > 0 {
			return g.VRAMUsedMB / g.VRAMTotalMB * 100.0, true
		}
	}
	return 0, false
}

// RenderGameHUDImage renders the click-through corner Game FPS HUD in compact, detailed, or graph mode.
func RenderGameHUDImage(snap collector.SystemSnapshot, cfg config.PureMonitorConfig, scale float64) *image.RGBA {
	renderMu.Lock()
	defer renderMu.Unlock()
	if scale <= 0 {
		scale = 1
	}
	fOpts, thres := formatOpts(cfg)
	th := ThemePalette(true, cfg.GameHUDOpacity, false)
	fontPx := float64(cfg.FontSize) * scale
	if fontPx < 10 {
		fontPx = 10
	}

	style := strings.ToLower(strings.TrimSpace(cfg.GameHUDStyle))
	pad := 10 * scale

	if style == "compact" {
		w := int(math.Round(132 * scale))
		h := int(math.Round(34 * scale))
		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		dc := gg.NewContextForRGBA(rgba)
		dc.SetColor(th.Bg)
		dc.DrawRoundedRectangle(0.5, 0.5, float64(w)-1, float64(h)-1, 8*scale)
		dc.Fill()
		dc.SetColor(th.Border)
		dc.SetLineWidth(math.Max(1, scale))
		dc.DrawRoundedRectangle(0.5, 0.5, float64(w)-1, float64(h)-1, 8*scale)
		dc.Stroke()

		fpsStr := "--"
		if snap.FPS > 0 {
			fpsStr = fmt.Sprintf("%.0f FPS", snap.FPS)
		}
		_, _, lvl := collector.FormatMetricValue(collector.MetricFPS, snap, fOpts, thres)
		dc.SetFontFace(getFace(fontMono, fontPx*1.12))
		dc.SetColor(levelColor(lvl, collector.MetricFPS, th))
		dc.DrawStringAnchored(fpsStr, pad, float64(h)*0.52, 0, 0.36)

		lowStr := ""
		if snap.FPS1Low > 0 {
			lowStr = fmt.Sprintf("1%% %.0f", snap.FPS1Low)
		}
		dc.SetFontFace(getFace(fontMono, fontPx*0.86))
		dc.SetColor(th.Muted)
		dc.DrawStringAnchored(lowStr, float64(w)-pad, float64(h)*0.52, 1, 0.36)
		return rgba
	}

	metrics := cfg.GameHUDMetrics
	if len(metrics) == 0 {
		metrics = []string{"fps", "fps_1low", "frametime", "gpu", "gpu_temp", "cpu", "cpu_temp", "ram"}
	}
	var hwMetrics []string
	for _, m := range metrics {
		switch collector.MetricID(m) {
		case collector.MetricFPS, collector.MetricFPSAvg, collector.MetricFPS1Low, collector.MetricFPS01Low, collector.MetricFrametime:
			// FPS hero band handles these at top of HUD.
		default:
			hwMetrics = append(hwMetrics, m)
		}
	}

	heroH := 40 * scale
	rowH := 18 * scale
	graphH := 0.0
	if style == "graph" {
		graphH = 38 * scale
	}
	w := int(math.Round(212 * scale))
	h := int(math.Round(heroH + float64(len(hwMetrics))*rowH + graphH + pad))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	dc := gg.NewContextForRGBA(rgba)
	dc.SetColor(th.Bg)
	dc.DrawRoundedRectangle(0.5, 0.5, float64(w)-1, float64(h)-1, 10*scale)
	dc.Fill()
	dc.SetColor(th.Border)
	dc.SetLineWidth(math.Max(1, scale))
	dc.DrawRoundedRectangle(0.5, 0.5, float64(w)-1, float64(h)-1, 10*scale)
	dc.Stroke()

	// Hero FPS row
	_, fpsVal, fpsLvl := collector.FormatMetricValue(collector.MetricFPS, snap, fOpts, thres)
	dc.SetFontFace(getFace(fontMono, fontPx*1.45))
	dc.SetColor(levelColor(fpsLvl, collector.MetricFPS, th))
	dc.DrawStringAnchored(fpsVal, pad, heroH*0.50, 0, 0.38)
	dc.SetFontFace(getFace(fontUIBold, fontPx*0.82))
	dc.SetColor(th.Muted)
	dc.DrawStringAnchored("FPS", pad+46*scale, heroH*0.56, 0, 0.38)

	subRight := fmt.Sprintf("1%% %.0f  %.1fms", snap.FPS1Low, snap.FrameTimeMs)
	if snap.FPS <= 0 {
		subRight = "ETW READY"
	}
	dc.SetFontFace(getFace(fontMono, fontPx*0.86))
	dc.SetColor(th.Text)
	dc.DrawStringAnchored(subRight, float64(w)-pad, heroH*0.54, 1, 0.38)

	// Divider
	dc.SetColor(th.Border)
	dc.SetLineWidth(1)
	dc.DrawLine(pad, heroH, float64(w)-pad, heroH)
	dc.Stroke()

	lblFace := getFace(fontUIBold, fontPx*0.84)
	valFace := getFace(fontMono, fontPx*0.90)
	yCur := heroH + 4*scale
	for _, raw := range hwMetrics {
		id := collector.MetricID(raw)
		lbl, val, lvl := collector.FormatMetricValue(id, snap, fOpts, thres)
		cy := yCur + rowH*0.5
		if ok, down, tag, nCol := netMetricInfo(id, false, th); ok {
			drawNetArrowIcon(dc, pad+4.5*scale, cy+0.5*scale, 8.5*scale, down, nCol)
			dc.SetFontFace(lblFace)
			dc.SetColor(nCol)
			dc.DrawStringAnchored(tag, pad+12*scale, cy, 0, 0.36)
		} else {
			dc.SetFontFace(lblFace)
			dc.SetColor(th.Muted)
			dc.DrawStringAnchored(lbl, pad, cy, 0, 0.36)
		}
		dc.SetFontFace(valFace)
		dc.SetColor(levelColor(lvl, id, th))
		dc.DrawStringAnchored(val, float64(w)-pad, cy, 1, 0.36)
		yCur += rowH
	}

	if style == "graph" {
		gx := pad
		gy := yCur + 4*scale
		gw := float64(w) - 2*pad
		gh := graphH - 8*scale
		dc.SetColor(th.BgAlt)
		dc.DrawRoundedRectangle(gx, gy, gw, gh, 4*scale)
		dc.Fill()

		pts := snap.RecentFrameTimes
		if len(pts) >= 2 {
			maxFT := float32(33.3)
			for _, v := range pts {
				if v > maxFT {
					maxFT = v
				}
			}
			if maxFT > 100 {
				maxFT = 100
			}
			dc.SetLineWidth(math.Max(1.3, 1.5*scale))
			dc.SetColor(th.Success)
			if snap.FrameTimeMs > 25 {
				dc.SetColor(th.Warn)
			}
			for i, ft := range pts {
				px := gx + float64(i)/float64(len(pts)-1)*gw
				norm := math.Min(1, math.Max(0, float64(ft)/float64(maxFT)))
				py := gy + gh - norm*(gh-4*scale) - 2*scale
				if i == 0 {
					dc.MoveTo(px, py)
				} else {
					dc.LineTo(px, py)
				}
			}
			dc.Stroke()
		}
	}

	return rgba
}
