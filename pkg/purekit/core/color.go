package core

import "image/color"

// ARGB colors are packed as 0xAARRGGBB, the same order the settings and
// annotation JSON used.
type ARGB uint32

// A returns the alpha channel.
func (c ARGB) A() uint8 { return uint8(c >> 24) }

// R returns the red channel.
func (c ARGB) R() uint8 { return uint8(c >> 16) }

// G returns the green channel.
func (c ARGB) G() uint8 { return uint8(c >> 8) }

// B returns the blue channel.
func (c ARGB) B() uint8 { return uint8(c) }

// RGBA converts to an image color with straight alpha.
func (c ARGB) RGBA() color.RGBA { return color.RGBA{R: c.R(), G: c.G(), B: c.B(), A: c.A()} }

// WithAlpha replaces the alpha channel.
func (c ARGB) WithAlpha(a uint8) ARGB { return ARGB(uint32(a)<<24) | (c & 0x00ffffff) }

// Palette is the annotation palette shared by the preview and the baker.
var Palette = []ARGB{
	0xFFFF4D4D,
	0xFFFFA028,
	0xFFFFE01A,
	0xFF3DDC84,
	0xFF12C2E9,
	0xFF3B7BFF,
	0xFFB14BFF,
	0xFF111111,
	0xFFFFFFFF,
}

// Thicknesses are the line widths offered by the capture toolbar.
var Thicknesses = []float64{2, 4, 7, 12}

const (
	DefaultStrokeColor     = ARGB(0xFFFF4D4D)
	DefaultThickness       = 4.0
	DefaultHighlightColor  = ARGB(0x66FFE01A)
	DefaultHighlightWidth  = 20.0
	DefaultFontSize        = 20.0
	DefaultMosaicBlock     = 12.0
	DefaultBadgeRadius     = 14.0
	DefaultFillAlpha       = 0x50
	DefaultTextBackground  = ARGB(0xE6000000)
	DefaultBadgeTextColor  = ARGB(0xFFFFFFFF)
	DefaultPinBorderColor  = ARGB(0xFF3B7BFF)
	DefaultPinBorderWidth  = 1
	DefaultCaptureFileName = "shot_%yyyyMMdd_%HHmmss"
)
