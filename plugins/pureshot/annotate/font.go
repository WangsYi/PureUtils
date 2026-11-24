package annotate

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"pureutils/pkg/purekit/core"
)

// fontSet draws text with Segoe UI and falls back to Microsoft YaHei for
// scripts Segoe UI lacks, which is what Windows itself does for CJK text.
type fontSet struct {
	primary  *sfnt.Font
	fallback *sfnt.Font
	faces    map[fontKey]font.Face
	mu       sync.Mutex
}

type fontKey struct {
	face string
	size int
}

var fontsOnce sync.Once
var fonts *fontSet
var fontsErr error

func loadFonts() (*fontSet, error) {
	fontsOnce.Do(func() {
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		primary := loadFirst([]string{
			filepath.Join(dir, "segoeui.ttf"),
			filepath.Join(dir, "arial.ttf"),
		})
		fallback := loadFirst([]string{
			filepath.Join(dir, "msyh.ttc"),
			filepath.Join(dir, "simhei.ttf"),
			filepath.Join(dir, "simsun.ttc"),
		})
		if primary == nil {
			fontsErr = fmt.Errorf("no usable system font found in %s", dir)
			return
		}
		fonts = &fontSet{primary: primary, fallback: fallback, faces: map[fontKey]font.Face{}}
	})
	return fonts, fontsErr
}

func loadFirst(paths []string) *sfnt.Font {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if coll, err := sfnt.ParseCollection(data); err == nil && coll.NumFonts() > 0 {
			if f, err := coll.Font(0); err == nil {
				return f
			}
			continue
		}
		if f, err := sfnt.Parse(data); err == nil {
			return f
		}
	}
	return nil
}

func (fs *fontSet) fontFor(r rune) *sfnt.Font {
	if fs == nil {
		return nil
	}
	if fs.primary != nil {
		var buf sfnt.Buffer
		if idx, err := fs.primary.GlyphIndex(&buf, r); err == nil && idx != 0 {
			return fs.primary
		}
	}
	if fs.fallback != nil {
		var buf sfnt.Buffer
		if idx, err := fs.fallback.GlyphIndex(&buf, r); err == nil && idx != 0 {
			return fs.fallback
		}
	}
	return fs.primary
}

func (fs *fontSet) face(f *sfnt.Font, size float64) font.Face {
	if f == nil {
		return nil
	}
	px := int(size + 0.5)
	if px < 1 {
		px = 1
	}
	key := fontKey{face: fontID(f), size: px}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if face, ok := fs.faces[key]; ok {
		return face
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(px),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	fs.faces[key] = face
	return face
}

var fontIDs sync.Map

func fontID(f *sfnt.Font) string {
	if v, ok := fontIDs.Load(f); ok {
		return v.(string)
	}
	id := fmt.Sprintf("%p", f)
	fontIDs.Store(f, id)
	return id
}

// MeasureText returns the width and height of annotation text at a size in
// image pixels. It wraps at newlines only, as the text tool does.
func MeasureText(text string, size float64) core.Size {
	fs, err := loadFonts()
	if err != nil || fs == nil || size <= 0 {
		return core.Size{W: float64(len([]rune(text))) * size * 0.6, H: size * 1.4}
	}
	lines := splitLines(text)
	maxW := 0.0
	for _, line := range lines {
		w := 0.0
		for _, r := range line {
			face := fs.face(fs.fontFor(r), size)
			if face == nil {
				continue
			}
			adv, ok := face.GlyphAdvance(r)
			if !ok {
				continue
			}
			w += float64(adv) / 64
		}
		if w > maxW {
			maxW = w
		}
	}
	h := float64(len(lines)) * size * 1.35
	if len(lines) == 0 {
		h = size * 1.35
	}
	return core.Size{W: maxW, H: h}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// drawText draws a string with per-rune fallback and returns the width.
func drawText(fs *fontSet, dst *image.RGBA, x, baseline float64, text string, size float64, c core.ARGB) float64 {
	if fs == nil {
		return 0
	}
	pen := x
	for _, r := range text {
		face := fs.face(fs.fontFor(r), size)
		if face == nil {
			continue
		}
		adv, ok := face.GlyphAdvance(r)
		if !ok {
			continue
		}
		d := font.Drawer{
			Dst:  dst,
			Src:  image.NewUniform(color.RGBA{R: c.R(), G: c.G(), B: c.B(), A: c.A()}),
			Face: face,
			Dot:  fixed.Point26_6{X: fixed.Int26_6(pen * 64), Y: fixed.Int26_6(baseline * 64)},
		}
		d.DrawString(string(r))
		pen += float64(adv) / 64
	}
	return pen - x
}
