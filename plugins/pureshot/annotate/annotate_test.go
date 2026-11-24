package annotate

import (
	"image"
	"image/color"
	"testing"

	"pureutils/pkg/purekit/core"
)

func testImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	return img
}

func TestDocumentUndoRedoJSON(t *testing.T) {
	d := NewDocument(100, 80)
	it := &Item{Tool: ToolRect, Color: core.DefaultStrokeColor, Thickness: 4,
		Start: core.Point{X: 10, Y: 10}, End: core.Point{X: 40, Y: 30}}
	d.Add(it)
	if it.ID != 1 || len(d.Items) != 1 {
		t.Fatalf("add: %+v", d.Items)
	}
	d.Add(&Item{Tool: ToolArrow, Start: core.Point{X: 1, Y: 2}, End: core.Point{X: 30, Y: 40}})
	if !d.CanUndo() {
		t.Fatal("no undo")
	}
	d.Undo()
	if len(d.Items) != 1 || d.Items[0].Tool != ToolRect {
		t.Fatalf("undo: %+v", d.Items)
	}
	d.Redo()
	if len(d.Items) != 2 || d.Items[1].Tool != ToolArrow {
		t.Fatalf("redo: %+v", d.Items)
	}
	json, err := d.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	got, err := FromJSON(json)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 100 || got.Height != 80 || len(got.Items) != 2 {
		t.Fatalf("json round trip: %+v", got)
	}
	if got.Items[1].Tool != ToolArrow {
		t.Errorf("item tool = %q", got.Items[1].Tool)
	}
}

func TestArrowGeometry(t *testing.T) {
	s := ArrowGeometry(core.Point{X: 0, Y: 0}, core.Point{X: 100, Y: 0}, 20)
	if s.Tip.X != 100 || s.Base.X != 80 {
		t.Errorf("shape = %+v", s)
	}
	if s.Left.Y == s.Right.Y {
		t.Errorf("head has no spread: %+v", s)
	}
}

func TestRenderRectAndArrow(t *testing.T) {
	src := testImage(100, 80)
	d := NewDocument(100, 80)
	d.Add(&Item{Tool: ToolRect, Color: 0xFFFF0000, Thickness: 4,
		Start: core.Point{X: 10, Y: 10}, End: core.Point{X: 60, Y: 50}})
	d.Add(&Item{Tool: ToolArrow, Color: 0xFF00FF00, Thickness: 4,
		Start: core.Point{X: 20, Y: 60}, End: core.Point{X: 80, Y: 70}})
	out, err := Render(src, d)
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds() != src.Bounds() {
		t.Fatalf("bounds = %v", out.Bounds())
	}
	// The rectangle's top edge should be red.
	r, g, b, a := out.At(30, 10).RGBA()
	if a == 0 || r < 0x8000 || g > 0x4000 || b > 0x4000 {
		t.Errorf("rect edge = %d,%d,%d,%d", r, g, b, a)
	}
	// The arrow's shaft should have green.
	r, g, b, a = out.At(50, 65).RGBA()
	if a == 0 || g < 0x4000 || r > 0x8000 || b > 0x8000 {
		t.Errorf("arrow = %d,%d,%d,%d", r, g, b, a)
	}
}

func TestRenderTextAndMosaic(t *testing.T) {
	src := testImage(160, 120)
	d := NewDocument(160, 120)
	d.Add(&Item{Tool: ToolText, Color: 0xFFFFFFFF, Thickness: 2,
		Origin: core.Point{X: 10, Y: 10}, Text: "Hi 你好", FontSize: 20,
		Measured: MeasureText("Hi 你好", 20)})
	d.Add(&Item{Tool: ToolMosaic, Color: core.DefaultStrokeColor, Thickness: 2,
		Region: core.Rect{X: 80, Y: 60, W: 48, H: 36}, BlockSize: 12})
	out, err := Render(src, d)
	if err != nil {
		t.Fatal(err)
	}
	// Text should change at least some pixels to white-ish.
	foundText := false
	for y := 0; y < 40 && !foundText; y++ {
		for x := 0; x < 100; x++ {
			r, g, b, _ := out.At(x, y).RGBA()
			if r > 0xC000 && g > 0xC000 && b > 0xC000 {
				foundText = true
				break
			}
		}
	}
	if !foundText {
		t.Error("text was not drawn")
	}
	// Mosaic blocks should be flat within one block.
	_, _, _, _ = out.At(82, 62).RGBA()
	r1, g1, b1, _ := out.At(82, 62).RGBA()
	r2, g2, b2, _ := out.At(90, 62).RGBA()
	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf("mosaic block is not flat: %d,%d,%d vs %d,%d,%d", r1, g1, b1, r2, g2, b2)
	}
}

func TestBakeCrop(t *testing.T) {
	frame := testImage(200, 150)
	d := NewDocument(80, 60)
	d.Add(&Item{Tool: ToolEllipse, Color: 0xFF3B7BFF, Thickness: 4,
		Start: core.Point{X: 10, Y: 10}, End: core.Point{X: 70, Y: 50}})
	out, err := Bake(frame, image.Rect(20, 30, 100, 90), d)
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds().Dx() != 80 || out.Bounds().Dy() != 60 {
		t.Errorf("baked size = %v", out.Bounds())
	}
	if _, err := Bake(frame, image.Rect(0, 0, 80, 60), NewDocument(10, 10)); err == nil {
		t.Error("mismatched document size was accepted")
	}
}

func TestTextCardWrapMeasureAndRender(t *testing.T) {
	sample := "ShotClip 文本贴图测试：supports mixed English words and 中文自动换行排版。\n第二行：func main() { return }"
	lines, maxW, truncated := WrapTextLines(sample, 13, 260, 40)
	if len(lines) < 2 || maxW <= 0 || truncated {
		t.Fatalf("WrapTextLines = len:%d maxW:%.1f trunc:%v", len(lines), maxW, truncated)
	}
	cw, ch := MeasureTextCard(sample, 13, 500)
	if cw < 180 || cw > 540 || ch < 64 || ch > 420 {
		t.Fatalf("MeasureTextCard out of bounds: %.1fx%.1f", cw, ch)
	}
	for _, dark := range []bool{true, false} {
		card := RenderTextCard(sample, dark)
		if card == nil || card.Bounds().Dx() < 360 || card.Bounds().Dy() < 128 {
			t.Fatalf("RenderTextCard(dark=%v) invalid bounds: %v", dark, card.Bounds())
		}
	}
}
