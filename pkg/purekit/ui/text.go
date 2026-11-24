package ui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

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

var (
	fontsOnce sync.Once
	fonts     *fontSet
	fontsErr  error
)

func loadFonts() (*fontSet, error) {
	fontsOnce.Do(func() {
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		primary := loadFirstFont([]string{
			filepath.Join(dir, "segoeui.ttf"),
			filepath.Join(dir, "arial.ttf"),
		})
		fallback := loadFirstFont([]string{
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

func loadFirstFont(paths []string) *sfnt.Font {
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
	var buf sfnt.Buffer
	if fs.primary != nil {
		if idx, err := fs.primary.GlyphIndex(&buf, r); err == nil && idx != 0 {
			return fs.primary
		}
	}
	if fs.fallback != nil {
		if idx, err := fs.fallback.GlyphIndex(&buf, r); err == nil && idx != 0 {
			return fs.fallback
		}
	}
	return fs.primary
}

func (fs *fontSet) face(f *sfnt.Font, size float64) font.Face {
	if fs == nil || f == nil {
		return nil
	}
	name := "primary"
	if f == fs.fallback {
		name = "fallback"
	}
	key := fontKey{face: name, size: int(math.Round(size * 10))}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fc, ok := fs.faces[key]; ok {
		return fc
	}
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	fs.faces[key] = fc
	return fc
}

// WrapTextLines normalizes line endings/tabs and word-wraps text to fit within
// maxWidth pixels at fontSize, capping output at maxLines.
func WrapTextLines(text string, fontSize, maxWidth float64, maxLines int) (lines []string, maxW float64, truncated bool) {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	norm = strings.ReplaceAll(norm, "\r", "\n")
	norm = strings.ReplaceAll(norm, "\t", "    ")
	if fontSize <= 0 {
		fontSize = 13
	}
	if maxWidth <= 0 {
		maxWidth = 480
	}
	if maxLines <= 0 {
		maxLines = 40
	}
	fs, _ := loadFonts()
	measure := func(r rune) float64 {
		if fs == nil {
			if r < 128 {
				return fontSize * 0.56
			}
			return fontSize * 1.0
		}
		face := fs.face(fs.fontFor(r), fontSize)
		if face == nil {
			return fontSize * 0.6
		}
		adv, ok := face.GlyphAdvance(r)
		if !ok {
			return fontSize * 0.6
		}
		return float64(adv) / 64
	}

	rawLines := strings.Split(norm, "\n")
	for _, raw := range rawLines {
		if raw == "" {
			if len(lines) >= maxLines {
				truncated = true
				break
			}
			lines = append(lines, "")
			continue
		}
		var cur []rune
		curW := 0.0
		lastSpaceIdx := -1
		for _, r := range raw {
			rw := measure(r)
			if len(cur) > 0 && curW+rw > maxWidth {
				splitAt := len(cur)
				if lastSpaceIdx > 0 && lastSpaceIdx < len(cur) {
					splitAt = lastSpaceIdx + 1
				}
				lineStr := strings.TrimRight(string(cur[:splitAt]), " ")
				if w := measureLineWidth(lineStr, measure); w > maxW {
					maxW = w
				}
				if len(lines) >= maxLines {
					truncated = true
					return lines, maxW, true
				}
				lines = append(lines, lineStr)

				rem := cur[splitAt:]
				for len(rem) > 0 && rem[0] == ' ' {
					rem = rem[1:]
				}
				cur = append([]rune(nil), rem...)
				curW = measureRunesWidth(cur, measure)
				lastSpaceIdx = -1
				for i, rr := range cur {
					if rr == ' ' {
						lastSpaceIdx = i
					}
				}
			}
			cur = append(cur, r)
			curW += rw
			if r == ' ' {
				lastSpaceIdx = len(cur) - 1
			}
		}
		lineStr := string(cur)
		if curW > maxW {
			maxW = curW
		}
		if len(lines) >= maxLines {
			truncated = true
			return lines, maxW, true
		}
		lines = append(lines, lineStr)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines, maxW, truncated
}

func measureLineWidth(s string, m func(rune) float64) float64 {
	total := 0.0
	for _, r := range s {
		total += m(r)
	}
	return total
}

func measureRunesWidth(rs []rune, m func(rune) float64) float64 {
	total := 0.0
	for _, r := range rs {
		total += m(r)
	}
	return total
}
