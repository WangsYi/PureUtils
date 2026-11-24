package pureimage

import (
	"bytes"
	"fmt"
	"image"
	stddraw "image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/gen2brain/avif"
	"github.com/gen2brain/heic"
	"github.com/gen2brain/jpegxl"
	"github.com/gen2brain/webp"
	"github.com/kettek/apng"
	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	previewDim   = 1600
	animationDim = 1000
)

type animationError struct{ s string }

func (e *animationError) Error() string { return e.s }

var errUnsupportedAnimation = &animationError{"unsupported animation"}

// DecodePreview decodes path scaled to fit maxDim (or full size if maxDim <= 0)
// and returns both the MyGo bitmap and raw transformed RGBA/image alongside metadata.
func DecodePreview(path string, maxDim, rotate int, flip bool) (*ui.Bitmap, image.Image, int, int, ExifData, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, 0, 0, ExifData{}, false, err
	}
	img, ex, srcW, srcH, err := DecodeSource(data, path, maxDim)
	if err != nil {
		return nil, nil, 0, 0, ex, false, err
	}
	img = TransformImage(img, ex.Orientation, rotate, flip)
	sw, sh := TransformedDims(srcW, srcH, ex.Orientation, rotate)
	if sw <= 0 || sh <= 0 {
		return nil, nil, 0, 0, ex, false, fmt.Errorf("empty image")
	}
	alpha := false
	if !IsJPEGExt(filepath.Ext(path)) {
		alpha = ImageHasAlpha(img)
	}
	return ui.NewBitmap(img), img, sw, sh, ex, alpha, nil
}

// DecodeFull decodes path at full native resolution and transforms it.
func DecodeFull(path string, rotate int, flip bool) (*ui.Bitmap, image.Image, int, int, ExifData, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, 0, 0, ExifData{}, false, err
	}
	img, ex, _, _, err := DecodeSource(data, path, 0)
	if err != nil {
		return nil, nil, 0, 0, ex, false, err
	}
	img = TransformImage(img, ex.Orientation, rotate, flip)
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, nil, 0, 0, ex, false, fmt.Errorf("empty image")
	}
	alpha := false
	if !IsJPEGExt(filepath.Ext(path)) {
		alpha = ImageHasAlpha(img)
	}
	return ui.NewBitmap(img), img, sw, sh, ex, alpha, nil
}

// DecodeSource decodes raw bytes from path (applying TurboJPEG native scaling when available).
func DecodeSource(data []byte, path string, maxDim int) (image.Image, ExifData, int, int, error) {
	ext := strings.ToLower(filepath.Ext(path))
	ex := ParseExif(data)
	var img image.Image
	var srcW, srcH int

	if IsJPEGExt(ext) && maxDim > 0 && LoadTurboJPEG() == nil {
		if timg, w, h, err := DecodeTurboJPEG(data, maxDim); err == nil {
			img, srcW, srcH = timg, w, h
		}
	}
	if img == nil {
		if IsSpecialExt(ext) {
			simg, sex, err := DecodeSpecial(data, path)
			if err != nil {
				return nil, sex, 0, 0, err
			}
			img, ex = simg, sex
		} else {
			gimg, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return nil, ex, 0, 0, err
			}
			img = gimg
		}
		b := img.Bounds()
		srcW, srcH = b.Dx(), b.Dy()
		if srcW <= 0 || srcH <= 0 {
			return nil, ex, 0, 0, fmt.Errorf("empty image")
		}
		if maxDim > 0 && max(srcW, srcH) > maxDim {
			nw, nh := FitWithin(srcW, srcH, maxDim)
			dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
			xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, stddraw.Src, nil)
			img = dst
		}
	}
	return img, ex, srcW, srcH, nil
}

// DecodeSpecial handles HEIC, AVIF, JXL, and camera RAW files.
func DecodeSpecial(data []byte, path string) (image.Image, ExifData, error) {
	ext := strings.ToLower(filepath.Ext(path))
	ex := ExifData{Orientation: 1}
	switch ext {
	case ".heic", ".heif", ".hif":
		img, err := heic.Decode(bytes.NewReader(data))
		if err != nil {
			if jpg := LargestEmbeddedJPEG(data); len(jpg) > 0 {
				if jimg, _, jerr := image.Decode(bytes.NewReader(jpg)); jerr == nil {
					return jimg, ParseExif(jpg), nil
				}
			}
			return nil, ex, err
		}
		if hx, err := heic.DecodeExif(bytes.NewReader(data)); err == nil {
			ex = ExifFromFields(hx.Make, hx.Model, hx.DateTime, hx.DateTimeOriginal, hx.ExposureTime, hx.FNumber, hx.ISOSpeed, hx.FocalLength)
		}
		return img, ex, nil
	case ".avif", ".avifs":
		img, err := avif.Decode(bytes.NewReader(data), avif.Options{AutoRotate: true})
		if err != nil {
			return nil, ex, err
		}
		if ax, err := avif.DecodeExif(bytes.NewReader(data)); err == nil {
			ex = ExifFromFields(ax.Make, ax.Model, ax.DateTime, ax.DateTimeOriginal, ax.ExposureTime, ax.FNumber, ax.ISOSpeed, ax.FocalLength)
		}
		return img, ex, nil
	case ".jxl":
		img, err := jpegxl.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, ex, err
		}
		if jx, err := jpegxl.DecodeExif(bytes.NewReader(data)); err == nil {
			ex = ExifFromFields(jx.Make, jx.Model, jx.DateTime, jx.DateTimeOriginal, jx.ExposureTime, jx.FNumber, jx.ISOSpeed, jx.FocalLength)
		}
		return img, ex, nil
	}

	if img, err := decodeWICRaw(path); err == nil {
		jpg := LargestEmbeddedJPEG(data)
		rex := ExifData{Orientation: 1}
		if len(jpg) > 0 {
			rex = ParseExif(jpg)
		}
		if rex.Orientation >= 2 && rex.Orientation <= 8 {
			img = OrientImage(img, rex.Orientation)
		}
		rex.Orientation = 1
		return img, rex, nil
	}
	if ext == ".cr3" {
		if img, err := heic.Decode(bytes.NewReader(data)); err == nil {
			return img, ex, nil
		}
	}
	jpg := LargestEmbeddedJPEG(data)
	if len(jpg) == 0 {
		return nil, ex, fmt.Errorf("raw: no embedded JPEG preview")
	}
	img, _, err := image.Decode(bytes.NewReader(jpg))
	if err != nil {
		return nil, ex, fmt.Errorf("raw: embedded preview: %w", err)
	}
	return img, ParseExif(jpg), nil
}

// DecodeAnimation decodes multi-frame GIF, APNG, WebP, HEIC, AVIF, or JXL files.
func DecodeAnimation(path string, maxDim, rotate int, flip bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, 0, 0, ExifData{}, false, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".gif":
		return DecodeGIF(data, maxDim, rotate, flip)
	case ".heic", ".heif", ".hif":
		h, err := heic.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, err
		}
		ex := ExifData{Orientation: 1}
		if hx, e := heic.DecodeExif(bytes.NewReader(data)); e == nil {
			ex = ExifFromFields(hx.Make, hx.Model, hx.DateTime, hx.DateTimeOriginal, hx.ExposureTime, hx.FNumber, hx.ISOSpeed, hx.FocalLength)
		}
		return ImagesToAnimation(h.Image, secondsDelays(h.Delay), maxDim, ex, rotate, flip)
	case ".avif", ".avifs":
		a, err := avif.DecodeAll(bytes.NewReader(data), avif.Options{AutoRotate: true})
		if err != nil {
			return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, err
		}
		ex := ExifData{Orientation: 1}
		if ax, e := avif.DecodeExif(bytes.NewReader(data)); e == nil {
			ex = ExifFromFields(ax.Make, ax.Model, ax.DateTime, ax.DateTimeOriginal, ax.ExposureTime, ax.FNumber, ax.ISOSpeed, ax.FocalLength)
		}
		return ImagesToAnimation(a.Image, secondsDelays(a.Delay), maxDim, ex, rotate, flip)
	case ".jxl":
		j, err := jpegxl.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, err
		}
		ex := ExifData{Orientation: 1}
		if jx, e := jpegxl.DecodeExif(bytes.NewReader(data)); e == nil {
			ex = ExifFromFields(jx.Make, jx.Model, jx.DateTime, jx.DateTimeOriginal, jx.ExposureTime, jx.FNumber, jx.ISOSpeed, jx.FocalLength)
		}
		delays := make([]time.Duration, len(j.Delay))
		for i, d := range j.Delay {
			delays[i] = time.Duration(d) * time.Millisecond
		}
		return ImagesToAnimation(j.Image, delays, maxDim, ex, rotate, flip)
	case ".webp":
		w, err := webp.DecodeAll(bytes.NewReader(data), webp.Options{AutoRotate: true})
		if err != nil {
			return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, err
		}
		ex := ExifData{Orientation: 1}
		if wx, e := webp.DecodeExif(bytes.NewReader(data)); e == nil {
			ex = ExifFromFields(wx.Make, wx.Model, wx.DateTime, wx.DateTimeOriginal, wx.ExposureTime, wx.FNumber, wx.ISOSpeed, wx.FocalLength)
		}
		delays := make([]time.Duration, len(w.Delay))
		for i, d := range w.Delay {
			delays[i] = time.Duration(d) * time.Millisecond
		}
		return ImagesToAnimation(w.Image, delays, maxDim, ex, rotate, flip)
	case ".png":
		return DecodeAPNG(data, maxDim, rotate, flip)
	}
	return nil, nil, nil, 0, 0, ExifData{}, false, errUnsupportedAnimation
}

func secondsDelays(ds []float64) []time.Duration {
	out := make([]time.Duration, len(ds))
	for i, d := range ds {
		out[i] = time.Duration(d * float64(time.Second))
	}
	return out
}

// ImagesToAnimation transforms and uploads animation frames as MyGo bitmaps.
func ImagesToAnimation(images []image.Image, delays []time.Duration, maxDim int, ex ExifData, rotate int, flip bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
	if len(images) == 0 {
		return nil, nil, nil, 0, 0, ex, false, errUnsupportedAnimation
	}
	frames := make([]*ui.Bitmap, 0, len(images))
	var firstImg image.Image
	for i, src := range images {
		img := TransformImage(src, ex.Orientation, rotate, flip)
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		if maxDim > 0 && max(w, h) > maxDim {
			nw, nh := FitWithin(w, h, maxDim)
			dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
			xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, stddraw.Src, nil)
			img = dst
		}
		if i == 0 {
			firstImg = img
		}
		frames = append(frames, ui.NewBitmap(img))
	}
	b := images[0].Bounds()
	sw, sh := TransformedDims(b.Dx(), b.Dy(), ex.Orientation, rotate)
	if len(delays) != len(frames) {
		delays = make([]time.Duration, len(frames))
	}
	alpha := ImageHasAlpha(images[0])
	return frames, firstImg, delays, sw, sh, ex, alpha, nil
}

// DecodeGIF decodes an animated or static GIF with frame disposal composition.
func DecodeGIF(data []byte, maxDim, rotate int, flip bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, err
	}
	w, h := g.Config.Width, g.Config.Height
	if w <= 0 || h <= 0 {
		return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, errUnsupportedAnimation
	}
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	var previous *image.RGBA
	var prevBounds image.Rectangle
	images := make([]image.Image, 0, len(g.Image))
	for i, src := range g.Image {
		if i > 0 {
			switch g.Disposal[i-1] {
			case gif.DisposalBackground:
				stddraw.Draw(canvas, prevBounds, image.Transparent, image.Point{}, stddraw.Src)
			case gif.DisposalPrevious:
				if previous != nil {
					copy(canvas.Pix, previous.Pix)
				}
			}
		}
		previous = cloneRGBA(canvas)
		stddraw.Draw(canvas, src.Bounds(), src, src.Bounds().Min, stddraw.Over)
		prevBounds = src.Bounds()
		images = append(images, cloneRGBA(canvas))
	}
	delays := make([]time.Duration, len(g.Delay))
	for i, d := range g.Delay {
		delays[i] = time.Duration(d) * 10 * time.Millisecond
	}
	return ImagesToAnimation(images, delays, maxDim, ExifData{Orientation: 1}, rotate, flip)
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

// DecodeAPNG decodes an animated PNG with disposal and blending composition.
func DecodeAPNG(data []byte, maxDim, rotate int, flip bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
	a, err := apng.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, err
	}
	if len(a.Frames) == 0 {
		return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, errUnsupportedAnimation
	}
	w, h := 0, 0
	for _, f := range a.Frames {
		b := f.Image.Bounds()
		if x := f.XOffset + b.Dx(); x > w {
			w = x
		}
		if y := f.YOffset + b.Dy(); y > h {
			h = y
		}
	}
	if w <= 0 || h <= 0 {
		return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, errUnsupportedAnimation
	}
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	var previous *image.RGBA
	var prevRect image.Rectangle
	var images []image.Image
	var delays []time.Duration
	for i, f := range a.Frames {
		if f.IsDefault {
			continue
		}
		if len(images) > 0 {
			switch a.Frames[i-1].DisposeOp {
			case apng.DISPOSE_OP_BACKGROUND:
				stddraw.Draw(canvas, prevRect, image.Transparent, image.Point{}, stddraw.Src)
			case apng.DISPOSE_OP_PREVIOUS:
				if previous != nil {
					copy(canvas.Pix, previous.Pix)
				}
			}
		}
		previous = cloneRGBA(canvas)
		b := f.Image.Bounds()
		dst := image.Rect(f.XOffset, f.YOffset, f.XOffset+b.Dx(), f.YOffset+b.Dy())
		op := stddraw.Over
		if f.BlendOp == apng.BLEND_OP_SOURCE {
			op = stddraw.Src
		}
		stddraw.Draw(canvas, dst, f.Image, b.Min, op)
		prevRect = dst
		images = append(images, cloneRGBA(canvas))
		delays = append(delays, time.Duration(f.GetDelay()*float64(time.Second)))
	}
	if len(images) == 0 {
		return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, errUnsupportedAnimation
	}
	return ImagesToAnimation(images, delays, maxDim, ExifData{Orientation: 1}, rotate, flip)
}
