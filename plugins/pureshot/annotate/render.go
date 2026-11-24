package annotate

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"

	"github.com/fogleman/gg"

	"pureutils/pkg/purekit/core"
)

// Render draws the document's annotations over src, which must be the
// cropped source image. It returns a new image; src is not modified.
func Render(src *image.RGBA, doc *Document) (*image.RGBA, error) {
	if src == nil {
		return nil, fmt.Errorf("annotation render: nil source")
	}
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	if doc == nil || doc.IsEmpty() {
		return dst, nil
	}
	source := image.NewRGBA(src.Bounds())
	draw.Draw(source, source.Bounds(), src, src.Bounds().Min, draw.Src)

	fs, _ := loadFonts()
	dc := gg.NewContextForRGBA(dst)
	dc.SetLineCapRound()
	dc.SetLineJoinRound()

	for _, it := range doc.Snapshot() {
		if it == nil {
			continue
		}
		if it.Thickness <= 0 {
			it.Thickness = core.DefaultThickness
		}
		switch it.Tool {
		case ToolRect:
			drawBox(dc, it, false)
		case ToolEllipse:
			drawBox(dc, it, true)
		case ToolLine:
			setColor(dc, it.Color)
			dc.SetLineWidth(it.Thickness)
			dc.DrawLine(it.Start.X, it.Start.Y, it.End.X, it.End.Y)
			dc.Stroke()
		case ToolArrow:
			drawArrow(dc, it)
		case ToolPen:
			drawPen(dc, it)
		case ToolHighlight:
			setColor(dc, it.Color)
			dc.SetLineWidth(it.Thickness)
			dc.MoveTo(it.Start.X, it.Start.Y)
			dc.LineTo(it.End.X, it.End.Y)
			dc.Stroke()
		case ToolText:
			if it.Measured.W <= 0 || it.Measured.H <= 0 {
				it.Measured = MeasureText(it.Text, it.FontSize)
			}
			drawTextItem(dc, dst, fs, it)
		case ToolMosaic:
			drawMosaic(dc, source, it)
		case ToolBadge:
			drawBadge(dc, dst, fs, it)
		}
	}
	return dst, nil
}

// Bake crops frame and draws the document's annotations into the crop.
func Bake(frame *image.RGBA, crop image.Rectangle, doc *Document) (*image.RGBA, error) {
	if frame == nil {
		return nil, fmt.Errorf("annotation bake: nil frame")
	}
	crop = crop.Intersect(frame.Bounds())
	if crop.Empty() {
		return nil, fmt.Errorf("annotation bake: crop %v is empty or outside the frame", crop)
	}
	src := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	draw.Draw(src, src.Bounds(), frame, crop.Min, draw.Src)
	if doc != nil && (doc.Width != crop.Dx() || doc.Height != crop.Dy()) {
		return nil, fmt.Errorf("annotation bake: document is %dx%d but crop is %dx%d",
			doc.Width, doc.Height, crop.Dx(), crop.Dy())
	}
	return Render(src, doc)
}

// EncodePNG encodes an image as PNG.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func setColor(dc *gg.Context, c core.ARGB) {
	dc.SetRGBA255(int(c.R()), int(c.G()), int(c.B()), int(c.A()))
}

func drawBox(dc *gg.Context, it *Item, ellipse bool) {
	r := core.RectFromPoints(it.Start, it.End)
	if it.Filled {
		setColor(dc, it.FillColor)
		if ellipse {
			dc.DrawEllipse(r.X+r.W/2, r.Y+r.H/2, r.W/2, r.H/2)
			dc.Fill()
		} else {
			dc.DrawRectangle(r.X, r.Y, r.W, r.H)
			dc.Fill()
		}
	}
	setColor(dc, it.Color)
	dc.SetLineWidth(it.Thickness)
	if ellipse {
		dc.DrawEllipse(r.X+r.W/2, r.Y+r.H/2, r.W/2, r.H/2)
		dc.Stroke()
	} else {
		dc.DrawRectangle(r.X, r.Y, r.W, r.H)
		dc.Stroke()
	}
}

func drawArrow(dc *gg.Context, it *Item) {
	shape := ArrowGeometry(it.Start, it.End, it.ResolveHeadLength())
	setColor(dc, it.Color)
	dc.SetLineWidth(it.Thickness)
	dc.DrawLine(it.Start.X, it.Start.Y, shape.Base.X, shape.Base.Y)
	dc.Stroke()

	dc.MoveTo(shape.Tip.X, shape.Tip.Y)
	dc.LineTo(shape.Left.X, shape.Left.Y)
	dc.LineTo(shape.Right.X, shape.Right.Y)
	dc.ClosePath()
	dc.Fill()
}

func drawPen(dc *gg.Context, it *Item) {
	if len(it.Points) < 2 {
		return
	}
	setColor(dc, it.Color)
	dc.SetLineWidth(it.Thickness)
	dc.MoveTo(it.Points[0].X, it.Points[0].Y)
	for _, p := range it.Points[1:] {
		dc.LineTo(p.X, p.Y)
	}
	dc.Stroke()
}

func drawTextItem(dc *gg.Context, dst *image.RGBA, fs *fontSet, it *Item) {
	if it.Text == "" {
		return
	}
	if it.FontSize <= 0 {
		it.FontSize = core.DefaultFontSize
	}
	if it.HasBackground {
		setColor(dc, it.Background)
		dc.DrawRectangle(it.Origin.X, it.Origin.Y, it.Measured.W+4, it.Measured.H+2)
		dc.Fill()
	}
	setColor(dc, it.Color)
	for i, line := range splitLines(it.Text) {
		baseline := it.Origin.Y + it.FontSize + float64(i)*it.FontSize*1.35
		drawText(fs, dst, it.Origin.X+2, baseline, line, it.FontSize, it.Color)
	}
}

func drawMosaic(dc *gg.Context, source *image.RGBA, it *Item) {
	block := it.BlockSize
	if block <= 0 {
		block = core.DefaultMosaicBlock
	}
	r := it.Region.Intersect(core.RectOf(source.Bounds()))
	if r.Empty() {
		return
	}
	x0, y0 := int(math.Floor(r.X)), int(math.Floor(r.Y))
	x1, y1 := int(math.Ceil(r.X+r.W)), int(math.Ceil(r.Y+r.H))
	step := int(math.Max(1, math.Round(block)))
	for y := y0; y < y1; y += step {
		for x := x0; x < x1; x += step {
			rr := image.Rect(x, y, min(x+step, x1), min(y+step, y1))
			if rr.Empty() {
				continue
			}
			var sumR, sumG, sumB, n int
			for yy := rr.Min.Y; yy < rr.Max.Y; yy++ {
				for xx := rr.Min.X; xx < rr.Max.X; xx++ {
					c := source.RGBAAt(xx, yy)
					sumR += int(c.R)
					sumG += int(c.G)
					sumB += int(c.B)
					n++
				}
			}
			if n == 0 {
				continue
			}
			dc.SetRGBA255(sumR/n, sumG/n, sumB/n, 255)
			dc.DrawRectangle(float64(rr.Min.X), float64(rr.Min.Y), float64(rr.Dx()), float64(rr.Dy()))
			dc.Fill()
		}
	}
}

func drawBadge(dc *gg.Context, dst *image.RGBA, fs *fontSet, it *Item) {
	radius := it.Radius
	if radius <= 0 {
		radius = core.DefaultBadgeRadius
	}
	setColor(dc, it.Color)
	dc.DrawCircle(it.Center.X, it.Center.Y, radius)
	dc.Fill()
	if it.Number > 0 {
		text := fmt.Sprint(it.Number)
		size := radius * 1.15
		m := MeasureText(text, size)
		w := m.W
		baseline := it.Center.Y + size*0.36
		setColor(dc, it.TextColor)
		drawText(fs, dst, it.Center.X-w/2, baseline, text, size, it.TextColor)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
