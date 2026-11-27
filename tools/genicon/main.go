package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/fogleman/gg"
	xdraw "golang.org/x/image/draw"
)

var iconSizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	videoOnly := flag.Bool("video-only", false, "generate only PureVideo icon assets without touching .syso")
	monitorOnly := flag.Bool("monitor-only", false, "generate only PureMonitor icon assets without touching .syso")
	flag.Parse()
	if err := run(*videoOnly, *monitorOnly); err != nil {
		fmt.Fprintf(os.Stderr, "genicon: %v\n", err)
		os.Exit(1)
	}
}

func run(videoOnly, monitorOnly bool) error {
	if videoOnly {
		return generateVideoIcons()
	}
	if monitorOnly {
		return generateMonitorIcons()
	}

	appMaster := renderAppIcon(1024)
	appPNGs, err := buildScaledPNGs(appMaster, iconSizes)
	if err != nil {
		return fmt.Errorf("scale app icon: %w", err)
	}
	if err := os.WriteFile(filepath.Join("resources", "icon.png"), appPNGs[256], 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("internal", "host", "icon.png"), appPNGs[256], 0o644); err != nil {
		return err
	}
	if err := writeICO(filepath.Join("resources", "icon.ico"), iconSizes, appPNGs); err != nil {
		return err
	}

	setupMaster := renderSetupIcon(1024)
	setupPNGs, err := buildScaledPNGs(setupMaster, iconSizes)
	if err != nil {
		return fmt.Errorf("scale setup icon: %w", err)
	}
	if err := os.WriteFile(filepath.Join("resources", "setup.png"), setupPNGs[256], 0o644); err != nil {
		return err
	}
	if err := writeICO(filepath.Join("resources", "setup.ico"), iconSizes, setupPNGs); err != nil {
		return err
	}

	if err := generateVideoIcons(); err != nil {
		return err
	}

	if err := generateMonitorIcons(); err != nil {
		return err
	}

	if err := generateButtonArt(); err != nil {
		return err
	}

	syso, err := buildSysoAMD64(iconSizes, appPNGs)
	if err != nil {
		return fmt.Errorf("build syso: %w", err)
	}
	if err := os.WriteFile("rsrc_windows_amd64.syso", syso, 0o644); err != nil {
		return err
	}
	fmt.Println("Generated resources/icon.{png,ico}, internal/host/icon.png, resources/setup.{png,ico}, resources/video.{png,ico}, resources/monitor.{png,ico}, plugins/purevideo/video.png, plugins/puremonitor/monitor.png, resources/btn-primary.png, rsrc_windows_amd64.syso")
	return nil
}

func generateButtonArt() error {
	const targetW, targetH = 392, 80
	sw, sh := targetW*2, targetH*2
	dc := gg.NewContext(sw, sh)
	w, h := float64(sw), float64(sh)
	r := 32.0 // 16px at 2x, 8px at 100% DPI

	dc.DrawRoundedRectangle(0, 0, w, h, r)
	dc.Clip()

	bg := gg.NewLinearGradient(0, 0, 0, h)
	bg.AddColorStop(0.0, color.NRGBA{R: 99, G: 102, B: 241, A: 255}) // #6366F1
	bg.AddColorStop(1.0, color.NRGBA{R: 79, G: 70, B: 229, A: 255})  // #4F46E5
	dc.SetFillStyle(bg)
	dc.DrawRectangle(0, 0, w, h)
	dc.Fill()

	// Delicate top highlight rim so button separates cleanly on both light and dark surfaces
	dc.SetRGBA255(255, 255, 255, 48)
	dc.SetLineWidth(3.0)
	dc.DrawRoundedRectangle(1.5, 1.5, w-3.0, h-3.0, r-1.5)
	dc.Stroke()

	master := imageToRGBA(dc.Image())
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), master, master.Bounds(), xdraw.Over, nil)

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("resources", "btn-primary.png"), buf.Bytes(), 0o644)
}

func generateVideoIcons() error {
	videoMaster := renderVideoIcon(1024)
	videoPNGs, err := buildScaledPNGs(videoMaster, iconSizes)
	if err != nil {
		return fmt.Errorf("scale video icon: %w", err)
	}
	if err := os.MkdirAll("resources", 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join("plugins", "purevideo"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("resources", "video.png"), videoPNGs[256], 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("plugins", "purevideo", "video.png"), videoPNGs[256], 0o644); err != nil {
		return err
	}
	if err := writeICO(filepath.Join("resources", "video.ico"), iconSizes, videoPNGs); err != nil {
		return err
	}
	fmt.Println("Generated resources/video.{png,ico} and plugins/purevideo/video.png")
	return nil
}

func generateMonitorIcons() error {
	monMaster := renderMonitorIcon(1024)
	monPNGs, err := buildScaledPNGs(monMaster, iconSizes)
	if err != nil {
		return fmt.Errorf("scale monitor icon: %w", err)
	}
	if err := os.MkdirAll("resources", 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join("plugins", "puremonitor"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("resources", "monitor.png"), monPNGs[256], 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("plugins", "puremonitor", "monitor.png"), monPNGs[256], 0o644); err != nil {
		return err
	}
	if err := writeICO(filepath.Join("resources", "monitor.ico"), iconSizes, monPNGs); err != nil {
		return err
	}
	fmt.Println("Generated resources/monitor.{png,ico} and plugins/puremonitor/monitor.png")
	return nil
}

func renderMonitorIcon(size int) *image.RGBA {
	dc := gg.NewContext(size, size)
	s := float64(size)
	dc.Push()

	radius := s * 0.22
	dc.DrawRoundedRectangle(0, 0, s, s, radius)
	dc.Clip()

	// Emerald-Teal to Slate-Navy telemetry gradient using NRGBA stops.
	bg := gg.NewLinearGradient(0, 0, s, s)
	bg.AddColorStop(0.0, color.NRGBA{R: 15, G: 23, B: 42, A: 255})   // #0F172A
	bg.AddColorStop(0.5, color.NRGBA{R: 13, G: 148, B: 136, A: 255}) // #0D9488
	bg.AddColorStop(1.0, color.NRGBA{R: 16, G: 185, B: 129, A: 255}) // #10B981
	dc.SetFillStyle(bg)
	dc.DrawRectangle(0, 0, s, s)
	dc.Fill()

	glow := gg.NewRadialGradient(s*0.24, s*0.22, 0, s*0.24, s*0.22, s*0.66)
	glow.AddColorStop(0.0, color.NRGBA{R: 255, G: 255, B: 255, A: 58})
	glow.AddColorStop(1.0, color.NRGBA{R: 255, G: 255, B: 255, A: 0})
	dc.SetFillStyle(glow)
	dc.DrawRectangle(0, 0, s, s)
	dc.Fill()

	dc.SetRGBA255(255, 255, 255, 42)
	dc.SetLineWidth(s * 0.014)
	dc.DrawRoundedRectangle(s*0.007, s*0.007, s-s*0.014, s-s*0.014, radius-s*0.007)
	dc.Stroke()

	// Outer telemetry frame
	fx, fy, fw, fh := s*0.17, s*0.22, s*0.66, s*0.56
	dc.SetRGBA255(255, 255, 255, 46)
	dc.SetLineWidth(s * 0.032)
	dc.DrawRoundedRectangle(fx, fy, fw, fh, s*0.095)
	dc.Stroke()

	// Pulse waveform shadow + luminous stroke
	pts := [][2]float64{
		{s * 0.24, s * 0.54},
		{s * 0.36, s * 0.54},
		{s * 0.43, s * 0.34},
		{s * 0.52, s * 0.68},
		{s * 0.61, s * 0.40},
		{s * 0.67, s * 0.54},
		{s * 0.76, s * 0.54},
	}
	dc.SetLineCap(gg.LineCapRound)
	dc.SetLineJoin(gg.LineJoinRound)
	dc.SetRGBA255(15, 23, 42, 72)
	dc.SetLineWidth(s * 0.056)
	for i, pt := range pts {
		if i == 0 {
			dc.MoveTo(pt[0], pt[1]+s*0.022)
		} else {
			dc.LineTo(pt[0], pt[1]+s*0.022)
		}
	}
	dc.Stroke()

	waveGrad := gg.NewLinearGradient(s*0.24, s*0.34, s*0.76, s*0.68)
	waveGrad.AddColorStop(0.0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	waveGrad.AddColorStop(1.0, color.NRGBA{R: 209, G: 250, B: 229, A: 255})
	dc.SetStrokeStyle(waveGrad)
	dc.SetLineWidth(s * 0.050)
	for i, pt := range pts {
		if i == 0 {
			dc.MoveTo(pt[0], pt[1])
		} else {
			dc.LineTo(pt[0], pt[1])
		}
	}
	dc.Stroke()

	dc.Pop()
	return imageToRGBA(dc.Image())
}

func renderVideoIcon(size int) *image.RGBA {
	dc := gg.NewContext(size, size)
	s := float64(size)
	dc.Push()

	radius := s * 0.22
	dc.DrawRoundedRectangle(0, 0, s, s, radius)
	dc.Clip()

	// Cool obsidian-indigo to sapphire gradient tailored for PureVideo.
	bg := gg.NewLinearGradient(0, 0, s, s)
	bg.AddColorStop(0.0, color.RGBA{R: 30, G: 41, B: 59, A: 255})   // #1E293B
	bg.AddColorStop(0.45, color.RGBA{R: 67, G: 56, B: 202, A: 255}) // #4338CA
	bg.AddColorStop(1.0, color.RGBA{R: 14, G: 165, B: 233, A: 255}) // #0EA5E9
	dc.SetFillStyle(bg)
	dc.DrawRectangle(0, 0, s, s)
	dc.Fill()

	// Specular glow at top-left
	glow := gg.NewRadialGradient(s*0.26, s*0.22, 0, s*0.26, s*0.22, s*0.66)
	glow.AddColorStop(0.0, color.RGBA{R: 255, G: 255, B: 255, A: 62})
	glow.AddColorStop(1.0, color.RGBA{R: 255, G: 255, B: 255, A: 0})
	dc.SetFillStyle(glow)
	dc.DrawRectangle(0, 0, s, s)
	dc.Fill()

	// Subtle inner rim highlight
	dc.SetRGBA255(255, 255, 255, 44)
	dc.SetLineWidth(s * 0.014)
	dc.DrawRoundedRectangle(s*0.007, s*0.007, s-s*0.014, s-s*0.014, radius-s*0.007)
	dc.Stroke()

	// Translucent outer cinema frame
	fx, fy, fw, fh := s*0.18, s*0.23, s*0.64, s*0.54
	dc.SetRGBA255(255, 255, 255, 48)
	dc.SetLineWidth(s * 0.032)
	dc.DrawRoundedRectangle(fx, fy, fw, fh, s*0.095)
	dc.Stroke()

	// Subtle film sprocket accents on left and right edges inside frame
	dc.SetRGBA255(224, 231, 255, 110)
	dotW, dotH := s*0.042, s*0.056
	for _, dy := range []float64{s * 0.31, s * 0.472, s * 0.634} {
		dc.DrawRoundedRectangle(fx+s*0.045, dy, dotW, dotH, s*0.014)
		dc.Fill()
		dc.DrawRoundedRectangle(fx+fw-s*0.045-dotW, dy, dotW, dotH, s*0.014)
		dc.Fill()
	}

	// Play triangle soft drop shadow
	cx, cy := s*0.515, s*0.50
	triR := s * 0.165
	x0, y0 := cx-triR*0.62, cy-triR*0.88
	x1, y1 := cx-triR*0.62, cy+triR*0.88
	x2, y2 := cx+triR*0.95, cy
	_ = math.Pi

	dc.SetRGBA255(15, 23, 42, 72)
	dc.MoveTo(x0, y0+s*0.025)
	dc.LineTo(x1, y1+s*0.025)
	dc.LineTo(x2, y2+s*0.025)
	dc.ClosePath()
	dc.SetLineWidth(s * 0.048)
	dc.SetLineJoin(gg.LineJoinRound)
	dc.FillPreserve()
	dc.Stroke()

	// Crisp luminous white play triangle
	triGrad := gg.NewLinearGradient(x0, y0, x2, y1)
	triGrad.AddColorStop(0.0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	triGrad.AddColorStop(1.0, color.RGBA{R: 224, G: 242, B: 254, A: 255}) // #E0F2FE
	dc.SetFillStyle(triGrad)
	dc.SetStrokeStyle(triGrad)
	dc.MoveTo(x0, y0)
	dc.LineTo(x1, y1)
	dc.LineTo(x2, y2)
	dc.ClosePath()
	dc.SetLineWidth(s * 0.044)
	dc.SetLineJoin(gg.LineJoinRound)
	dc.FillPreserve()
	dc.Stroke()

	dc.Pop()
	return imageToRGBA(dc.Image())
}

func drawRoundedTile(dc *gg.Context, x, y, w, h, r float64) {
	dc.DrawRoundedRectangle(x, y, w, h, r)
}

func drawAppBase(dc *gg.Context, s float64) {
	radius := s * 0.22
	dc.DrawRoundedRectangle(0, 0, s, s, radius)
	dc.Clip()

	// Deep Royal Indigo -> Violet diagonal gradient
	bg := gg.NewLinearGradient(0, 0, s, s)
	bg.AddColorStop(0.0, color.RGBA{R: 99, G: 102, B: 241, A: 255})  // #6366F1
	bg.AddColorStop(0.52, color.RGBA{R: 79, G: 70, B: 229, A: 255})  // #4F46E5
	bg.AddColorStop(1.0, color.RGBA{R: 124, G: 58, B: 237, A: 255})  // #7C3AED
	dc.SetFillStyle(bg)
	dc.DrawRectangle(0, 0, s, s)
	dc.Fill()

	// Subtle top-left specular glow
	glow := gg.NewRadialGradient(s*0.24, s*0.20, 0, s*0.24, s*0.20, s*0.68)
	glow.AddColorStop(0.0, color.RGBA{R: 255, G: 255, B: 255, A: 58})
	glow.AddColorStop(1.0, color.RGBA{R: 255, G: 255, B: 255, A: 0})
	dc.SetFillStyle(glow)
	dc.DrawRectangle(0, 0, s, s)
	dc.Fill()

	// Subtle inner rim highlight
	dc.SetRGBA255(255, 255, 255, 42)
	dc.SetLineWidth(s * 0.014)
	dc.DrawRoundedRectangle(s*0.007, s*0.007, s-s*0.014, s-s*0.014, radius-s*0.007)
	dc.Stroke()
}

func drawFourModuleEmblem(dc *gg.Context, s, scale, offsetX, offsetY float64) {
	dc.Push()
	dc.Translate(offsetX, offsetY)
	dc.Scale(scale, scale)

	m := s * 0.20
	t := s * 0.255
	g := s * 0.090
	tr := s * 0.068

	// Soft layered drop shadow for the 4 modular tiles
	for _, sh := range []struct {
		dy    float64
		alpha uint8
	}{
		{s * 0.022, 46},
		{s * 0.040, 26},
	} {
		dc.SetRGBA255(20, 12, 64, int(sh.alpha))
		drawRoundedTile(dc, m, m+sh.dy, t, t, tr)
		dc.Fill()
		drawRoundedTile(dc, m, m+t+g+sh.dy, t, t, tr)
		dc.Fill()
		drawRoundedTile(dc, m+t+g, m+t+g+sh.dy, t, t, tr)
		dc.Fill()

		dc.Push()
		cx, cy := s*0.686, s*0.314+sh.dy
		dt := s * 0.235
		dc.RotateAbout(gg.Radians(45), cx, cy)
		drawRoundedTile(dc, cx-dt/2, cy-dt/2, dt, dt, s*0.056)
		dc.Fill()
		dc.Pop()
	}

	// Standard 3 rounded white tiles (top-left, bottom-left, bottom-right)
	tilePositions := [][2]float64{
		{m, m},
		{m, m + t + g},
		{m + t + g, m + t + g},
	}
	for _, pos := range tilePositions {
		grad := gg.NewLinearGradient(pos[0], pos[1], pos[0], pos[1]+t)
		grad.AddColorStop(0.0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		grad.AddColorStop(1.0, color.RGBA{R: 224, G: 231, B: 255, A: 255}) // #E0E7FF
		dc.SetFillStyle(grad)
		drawRoundedTile(dc, pos[0], pos[1], t, t, tr)
		dc.Fill()
	}

	// Elevated 4th rotated diamond tile ("Pure Spark")
	dc.Push()
	cx, cy := s*0.686, s*0.314
	dt := s * 0.235
	dc.RotateAbout(gg.Radians(45), cx, cy)
	sparkGrad := gg.NewLinearGradient(cx-dt/2, cy-dt/2, cx+dt/2, cy+dt/2)
	sparkGrad.AddColorStop(0.0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	sparkGrad.AddColorStop(1.0, color.RGBA{R: 186, G: 230, B: 253, A: 255}) // #BAE6FD
	dc.SetFillStyle(sparkGrad)
	drawRoundedTile(dc, cx-dt/2, cy-dt/2, dt, dt, s*0.056)
	dc.Fill()
	dc.Pop()

	dc.Pop()
}

func renderAppIcon(size int) *image.RGBA {
	dc := gg.NewContext(size, size)
	s := float64(size)
	dc.Push()
	drawAppBase(dc, s)
	drawFourModuleEmblem(dc, s, 1.0, 0, 0)
	dc.Pop()
	return imageToRGBA(dc.Image())
}

func renderSetupIcon(size int) *image.RGBA {
	dc := gg.NewContext(size, size)
	s := float64(size)

	// Draw main PureUtils app squircle slightly smaller at top-left so the
	// emerald install badge has confident prominence without looking cramped.
	appScale := 0.90
	appS := s * appScale
	dc.Push()
	dc.Push()
	dc.DrawRoundedRectangle(0, 0, appS, appS, appS*0.22)
	dc.Clip()

	bg := gg.NewLinearGradient(0, 0, appS, appS)
	bg.AddColorStop(0.0, color.RGBA{R: 99, G: 102, B: 241, A: 255})
	bg.AddColorStop(0.52, color.RGBA{R: 79, G: 70, B: 229, A: 255})
	bg.AddColorStop(1.0, color.RGBA{R: 124, G: 58, B: 237, A: 255})
	dc.SetFillStyle(bg)
	dc.DrawRectangle(0, 0, appS, appS)
	dc.Fill()

	glow := gg.NewRadialGradient(appS*0.24, appS*0.20, 0, appS*0.24, appS*0.20, appS*0.68)
	glow.AddColorStop(0.0, color.RGBA{R: 255, G: 255, B: 255, A: 58})
	glow.AddColorStop(1.0, color.RGBA{R: 255, G: 255, B: 255, A: 0})
	dc.SetFillStyle(glow)
	dc.DrawRectangle(0, 0, appS, appS)
	dc.Fill()

	dc.SetRGBA255(255, 255, 255, 42)
	dc.SetLineWidth(appS * 0.015)
	dc.DrawRoundedRectangle(appS*0.0075, appS*0.0075, appS-appS*0.015, appS-appS*0.015, appS*0.22-appS*0.0075)
	dc.Stroke()

	drawFourModuleEmblem(dc, s, appScale, 0, 0)
	dc.Pop()
	dc.Pop()

	// Bottom-right emerald installer badge
	bx, by := s*0.73, s*0.73
	outerR := s * 0.255
	innerR := s * 0.212

	// Subtle badge drop shadow
	dc.SetRGBA255(15, 23, 42, 65)
	dc.DrawCircle(bx, by+s*0.018, outerR)
	dc.Fill()

	// Pure white rim separating badge from squircle
	dc.SetRGBA255(255, 255, 255, 255)
	dc.DrawCircle(bx, by, outerR)
	dc.Fill()

	// Emerald circle fill
	badgeGrad := gg.NewLinearGradient(bx, by-innerR, bx, by+innerR)
	badgeGrad.AddColorStop(0.0, color.RGBA{R: 52, G: 211, B: 153, A: 255}) // #34D399
	badgeGrad.AddColorStop(1.0, color.RGBA{R: 5, G: 150, B: 105, A: 255})   // #059669
	dc.SetFillStyle(badgeGrad)
	dc.DrawCircle(bx, by, innerR)
	dc.Fill()

	// Crisp white download arrow inside badge
	dc.SetRGBA255(255, 255, 255, 255)
	dc.SetLineCap(gg.LineCapRound)
	dc.SetLineJoin(gg.LineJoinRound)
	dc.SetLineWidth(s * 0.044)

	// Vertical shaft
	dc.MoveTo(bx, by-s*0.095)
	dc.LineTo(bx, by+s*0.040)
	dc.Stroke()

	// Chevron head
	dc.MoveTo(bx-s*0.072, by-s*0.015)
	dc.LineTo(bx, by+s*0.056)
	dc.LineTo(bx+s*0.072, by-s*0.015)
	dc.Stroke()

	// Bottom tray bar
	dc.MoveTo(bx-s*0.082, by+s*0.102)
	dc.LineTo(bx+s*0.082, by+s*0.102)
	dc.Stroke()

	return imageToRGBA(dc.Image())
}

func imageToRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	xdraw.Copy(dst, image.Point{}, src, b, xdraw.Over, nil)
	return dst
}

func buildScaledPNGs(master *image.RGBA, sizes []int) (map[int][]byte, error) {
	out := make(map[int][]byte, len(sizes))
	for _, sz := range sizes {
		dst := image.NewRGBA(image.Rect(0, 0, sz, sz))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), master, master.Bounds(), xdraw.Over, nil)
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, dst); err != nil {
			return nil, err
		}
		out[sz] = buf.Bytes()
	}
	return out, nil
}

func writeICO(path string, sizes []int, pngs map[int][]byte) error {
	var buf bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&buf, le, uint16(0))
	_ = binary.Write(&buf, le, uint16(1))
	_ = binary.Write(&buf, le, uint16(len(sizes)))

	offset := uint32(6 + 16*len(sizes))
	for _, sz := range sizes {
		dim := byte(sz)
		if sz >= 256 {
			dim = 0
		}
		data := pngs[sz]
		buf.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&buf, le, uint16(1))
		_ = binary.Write(&buf, le, uint16(32))
		_ = binary.Write(&buf, le, uint32(len(data)))
		_ = binary.Write(&buf, le, offset)
		offset += uint32(len(data))
	}
	for _, sz := range sizes {
		buf.Write(pngs[sz])
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func buildSysoAMD64(sizes []int, pngs map[int][]byte) ([]byte, error) {
	const (
		machineAMD64     = uint16(0x8664)
		relocAMD64Addr32 = uint16(0x0003) // IMAGE_REL_AMD64_ADDR32NB
		rtIcon           = uint16(3)
		rtGroupIcon      = uint16(14)
		rtVersion        = uint16(16)
		rtManifest       = uint16(24)
		langID           = uint16(0x0409)
	)

	res := map[uint16]map[uint16][]byte{
		rtIcon:      {},
		rtVersion:   {1: buildVersionInfo()},
		rtManifest:  {1: []byte(buildManifest())},
		rtGroupIcon: {},
	}
	var group bytes.Buffer
	_ = binary.Write(&group, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	for i, sz := range sizes {
		id := uint16(i + 1)
		img := pngs[sz]
		res[rtIcon][id] = img
		dim := byte(sz)
		if sz >= 256 {
			dim = 0
		}
		group.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&group, binary.LittleEndian, [2]uint16{1, 32})
		_ = binary.Write(&group, binary.LittleEndian, uint32(len(img)))
		_ = binary.Write(&group, binary.LittleEndian, id)
	}
	res[rtGroupIcon][1] = group.Bytes()

	types := sortedKeys(res)
	dirSize := func(n int) int { return 16 + 8*n }
	size := dirSize(len(types))
	for _, t := range types {
		size += dirSize(len(res[t])) + len(res[t])*dirSize(1)
	}
	entriesAt := size
	count := 0
	for _, t := range types {
		count += len(res[t])
	}
	dataAt := entriesAt + 16*count

	raw := make([]byte, dataAt)
	le := binary.LittleEndian
	dir := func(at, n int) {
		le.PutUint16(raw[at+14:], uint16(n))
	}
	var relocs []uint32
	var data bytes.Buffer
	dir(0, len(types))
	next := dirSize(len(types))
	entry := entriesAt
	for i, t := range types {
		le.PutUint32(raw[16+8*i:], uint32(t))
		le.PutUint32(raw[16+8*i+4:], uint32(next)|0x80000000)
		ids := sortedKeys(res[t])
		idDir := next
		dir(idDir, len(ids))
		next += dirSize(len(ids))
		for j, id := range ids {
			le.PutUint32(raw[idDir+16+8*j:], uint32(id))
			le.PutUint32(raw[idDir+16+8*j+4:], uint32(next)|0x80000000)
			dir(next, 1)
			le.PutUint32(raw[next+16:], uint32(langID))
			le.PutUint32(raw[next+20:], uint32(entry))
			next += dirSize(1)

			for data.Len()%8 != 0 {
				data.WriteByte(0)
			}
			le.PutUint32(raw[entry:], uint32(dataAt+data.Len()))
			le.PutUint32(raw[entry+4:], uint32(len(res[t][id])))
			relocs = append(relocs, uint32(entry))
			entry += 16
			data.Write(res[t][id])
		}
	}
	raw = append(raw, data.Bytes()...)
	for len(raw)%4 != 0 {
		raw = append(raw, 0)
	}

	var out bytes.Buffer
	const headerSize, sectionHeaderSize = 20, 40
	rawAt := headerSize + sectionHeaderSize
	relocAt := rawAt + len(raw)
	symbolsAt := relocAt + 10*len(relocs)
	_ = binary.Write(&out, le, struct {
		Machine, Sections                   uint16
		Time, Symbols, SymbolCount          uint32
		OptionalHeaderSize, Characteristics uint16
	}{machineAMD64, 1, 0, uint32(symbolsAt), 1, 0, 0})
	_ = binary.Write(&out, le, struct {
		Name                                        [8]byte
		VirtualSize, VirtualAddress, RawSize, RawAt uint32
		RelocAt, LineNumbersAt                      uint32
		Relocs, LineNumbers                         uint16
		Characteristics                             uint32
	}{[8]byte{'.', 'r', 's', 'r', 'c'}, 0, 0, uint32(len(raw)), uint32(rawAt), uint32(relocAt), 0, uint16(len(relocs)), 0, 0x40000040})
	out.Write(raw)
	for _, r := range relocs {
		_ = binary.Write(&out, le, r)
		_ = binary.Write(&out, le, uint32(0))
		_ = binary.Write(&out, le, relocAMD64Addr32)
	}
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	_ = binary.Write(&out, le, uint32(0))
	_ = binary.Write(&out, le, uint16(1))
	_ = binary.Write(&out, le, uint16(0))
	out.Write([]byte{3, 0})
	_ = binary.Write(&out, le, uint32(4))
	return out.Bytes(), nil
}

func sortedKeys[V any](m map[uint16]V) []uint16 {
	keys := make([]uint16, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func buildManifest() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="com.pureutils.app" version="1.3.0.0" processorArchitecture="*"/>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness>
      <activeCodePage xmlns="http://schemas.microsoft.com/SMI/2019/WindowsSettings">UTF-8</activeCodePage>
    </windowsSettings>
  </application>
</assembly>
`
}

func buildVersionInfo() []byte {
	fixed := make([]byte, 52)
	le := binary.LittleEndian
	le.PutUint32(fixed[0:], 0xFEEF04BD)
	le.PutUint32(fixed[4:], 0x00010000)
	le.PutUint32(fixed[8:], uint32(1)<<16|uint32(3))
	le.PutUint32(fixed[12:], uint32(0)<<16|uint32(0))
	copy(fixed[16:24], fixed[8:16])
	le.PutUint32(fixed[24:], 0x3F)
	le.PutUint32(fixed[32:], 0x00040004)
	le.PutUint32(fixed[36:], 1)

	var table []verNode
	for _, s := range [][2]string{
		{"CompanyName", "PureUtils"},
		{"FileDescription", "PureUtils"},
		{"FileVersion", "1.3.0"},
		{"InternalName", "PureUtils.exe"},
		{"LegalCopyright", "© 2026 PureUtils"},
		{"OriginalFilename", "PureUtils.exe"},
		{"ProductName", "PureUtils"},
		{"ProductVersion", "1.3.0"},
		{"MyGoIdentifier", "com.pureutils.app"},
	} {
		text := utf16.Encode([]rune(s[1] + "\x00"))
		value := make([]byte, 2*len(text))
		for i, u := range text {
			le.PutUint16(value[2*i:], u)
		}
		table = append(table, verNode{key: s[0], text: true, value: value, valueLen: len(text)})
	}
	root := verNode{
		key: "VS_VERSION_INFO", value: fixed, valueLen: len(fixed),
		children: []verNode{
			{key: "StringFileInfo", text: true, children: []verNode{{key: strings.ToLower("040904b0"), text: true, children: table}}},
			{key: "VarFileInfo", text: true, children: []verNode{{key: "Translation", value: []byte{0x09, 0x04, 0xB0, 0x04}, valueLen: 4}}},
		},
	}
	return root.encode()
}

type verNode struct {
	key      string
	text     bool
	value    []byte
	valueLen int
	children []verNode
}

func (n verNode) encode() []byte {
	b := make([]byte, 6)
	for _, u := range utf16.Encode([]rune(n.key + "\x00")) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	pad := func() {
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
	}
	pad()
	b = append(b, n.value...)
	for _, child := range n.children {
		pad()
		b = append(b, child.encode()...)
	}
	binary.LittleEndian.PutUint16(b[0:], uint16(len(b)))
	binary.LittleEndian.PutUint16(b[2:], uint16(n.valueLen))
	if n.text {
		binary.LittleEndian.PutUint16(b[4:], 1)
	}
	return b
}
