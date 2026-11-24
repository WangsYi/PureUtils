package core

import (
	"image"
	"math"
)

// Point is a point in image pixels or in DIPs, depending on the caller.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Size is a width and a height.
type Size struct {
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Rect is a rectangle with its top-left corner at X, Y.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// RectI is a rectangle in whole pixels.
type RectI struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// RectFromPoints returns the rectangle spanned by a and b.
func RectFromPoints(a, b Point) Rect {
	return Rect{
		X: math.Min(a.X, b.X),
		Y: math.Min(a.Y, b.Y),
		W: math.Abs(a.X - b.X),
		H: math.Abs(a.Y - b.Y),
	}
}

// Inflate grows the rectangle by d on every side.
func (r Rect) Inflate(d float64) Rect {
	return Rect{X: r.X - d, Y: r.Y - d, W: r.W + 2*d, H: r.H + 2*d}
}

// Point returns the rectangle's top-left corner.
func (r Rect) Point() Point { return Point{X: r.X, Y: r.Y} }

// Empty reports whether the rectangle has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Contains reports whether p is inside the rectangle, edges included.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.X <= r.X+r.W && p.Y >= r.Y && p.Y <= r.Y+r.H
}

// Intersect returns the overlap of two rectangles.
func (r Rect) Intersect(o Rect) Rect {
	x0, y0 := math.Max(r.X, o.X), math.Max(r.Y, o.Y)
	x1, y1 := math.Min(r.X+r.W, o.X+o.W), math.Min(r.Y+r.H, o.Y+o.H)
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Round returns the rectangle as image pixels.
func (r Rect) Round() image.Rectangle {
	return image.Rect(int(math.Round(r.X)), int(math.Round(r.Y)),
		int(math.Round(r.X+r.W)), int(math.Round(r.Y+r.H)))
}

// RectOf converts an image rectangle.
func RectOf(r image.Rectangle) Rect {
	return Rect{X: float64(r.Min.X), Y: float64(r.Min.Y),
		W: float64(r.Dx()), H: float64(r.Dy())}
}

// ImageRect converts the rectangle to image pixels.
func (r RectI) ImageRect() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)
}

// Empty reports whether the rectangle has no area.
func (r RectI) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Intersect returns the overlap of two pixel rectangles.
func (r RectI) Intersect(o RectI) RectI {
	x0, y0 := max(r.X, o.X), max(r.Y, o.Y)
	x1, y1 := min(r.X+r.W, o.X+o.W), min(r.Y+r.H, o.Y+o.H)
	if x1 <= x0 || y1 <= y0 {
		return RectI{}
	}
	return RectI{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Contains reports whether p is inside the pixel rectangle.
func (r RectI) Contains(p image.Point) bool {
	return p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
}

// RectOfI converts a pixel rectangle.
func RectOfI(r image.Rectangle) RectI {
	return RectI{X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy()}
}

// Clamp returns v bounded by lo and hi.
func Clamp[T ~int | ~int64 | ~float64](v, lo, hi T) T {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
