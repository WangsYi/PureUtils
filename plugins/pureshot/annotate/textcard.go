package annotate

import (
	"image"
	"math"
	"strings"

	"github.com/fogleman/gg"

	"pureutils/pkg/purekit/core"
)

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

	rawLines := splitLines(norm)
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
				if w := measureLine(lineStr, measure); w > maxW {
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
				curW = measureRunes(cur, measure)
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

func measureLine(s string, m func(rune) float64) float64 {
	total := 0.0
	for _, r := range s {
		total += m(r)
	}
	return total
}

func measureRunes(rs []rune, m func(rune) float64) float64 {
	total := 0.0
	for _, r := range rs {
		total += m(r)
	}
	return total
}

// MeasureTextCard returns comfortable initial DIP dimensions for a text pin card.
func MeasureTextCard(text string, fontSize, maxContentW float64) (cardW, cardH float64) {
	if fontSize <= 0 {
		fontSize = 13
	}
	if maxContentW <= 0 {
		maxContentW = 500
	}
	lines, maxW, _ := WrapTextLines(text, fontSize, maxContentW, 40)
	lineH := fontSize * 1.50
	w := math.Ceil(maxW) + 34 // 16px horizontal padding each side + safety buffer
	h := math.Ceil(float64(len(lines))*lineH) + 32
	if w < 180 {
		w = 180
	}
	if w > 540 {
		w = 540
	}
	if h < 64 {
		h = 64
	}
	if h > 420 {
		h = 420
	}
	return w, h
}

// RenderTextCard rasterizes text into a crisp 2x HiDPI RGBA card image for
// PNG export and "copy as image" actions on floating text pins.
func RenderTextCard(text string, dark bool) *image.RGBA {
	const scale = 2.0
	const fontSize = 13.0
	cardW, cardH := MeasureTextCard(text, fontSize, 500)
	pw := int(math.Ceil(cardW * scale))
	ph := int(math.Ceil(cardH * scale))
	dst := image.NewRGBA(image.Rect(0, 0, pw, ph))

	dc := gg.NewContextForRGBA(dst)
	if dark {
		dc.SetRGBA255(26, 30, 40, 255)
	} else {
		dc.SetRGBA255(255, 255, 255, 255)
	}
	dc.Clear()

	// Subtle top accent stripe
	if dark {
		dc.SetRGBA255(59, 130, 246, 255)
	} else {
		dc.SetRGBA255(37, 99, 235, 255)
	}
	dc.DrawRectangle(0, 0, float64(pw), 3*scale)
	dc.Fill()

	// Subtle outer border
	if dark {
		dc.SetRGBA255(55, 64, 84, 255)
	} else {
		dc.SetRGBA255(197, 206, 222, 255)
	}
	dc.SetLineWidth(scale)
	dc.DrawRectangle(0.5, 0.5, float64(pw)-1, float64(ph)-1)
	dc.Stroke()

	lines, _, truncated := WrapTextLines(text, fontSize, cardW-34, 40)
	if truncated && len(lines) > 0 {
		lines[len(lines)-1] += " …"
	}

	fs, _ := loadFonts()
	textCol := core.ARGB(0xFFF3F4F6)
	if !dark {
		textCol = core.ARGB(0xFF111827)
	}
	padX := 16.0 * scale
	startY := 14.0*scale + 3.0*scale
	scaledFS := fontSize * scale
	lineH := scaledFS * 1.50
	for i, line := range lines {
		baseline := startY + scaledFS + float64(i)*lineH
		if baseline > float64(ph)-10*scale {
			break
		}
		drawText(fs, dst, padX, baseline, line, scaledFS, textCol)
	}
	return dst
}
