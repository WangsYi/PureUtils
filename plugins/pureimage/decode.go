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
	return decodePreviewEx(path, maxDim, rotate, flip, false)
}

func decodePreviewEx(path string, maxDim, rotate int, flipH, flipV bool) (*ui.Bitmap, image.Image, int, int, ExifData, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, 0, 0, ExifData{}, false, err
	}
	img, ex, srcW, srcH, err := DecodeSource(data, path, maxDim)
	if err != nil {
		return nil, nil, 0, 0, ex, false, err
	}
	img = TransformImageEx(img, ex.Orientation, rotate, flipH, flipV)
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
	return decodeFullEx(path, rotate, flip, false)
}

func decodeFullEx(path string, rotate int, flipH, flipV bool) (*ui.Bitmap, image.Image, int, int, ExifData, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, 0, 0, ExifData{}, false, err
	}
	img, ex, _, _, err := DecodeSource(data, path, 0)
	if err != nil {
		return nil, nil, 0, 0, ex, false, err
	}
	img = TransformImageEx(img, ex.Orientation, rotate, flipH, flipV)
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
	if img == nil && ext == ".webp" && CodecsAvailable() {
		if cimg, cex, _, _, err := DecodeCodecImage(PicFmtWebP, data, 0); err == nil {
			img = cimg
			if ex.Orientation == 1 && cex.Orientation != 1 || ex.Make == "" && cex.Make != "" {
				ex = cex
			}
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
	} else {
		b := img.Bounds()
		if maxDim > 0 && ext == ".webp" && max(b.Dx(), b.Dy()) > maxDim {
			srcW, srcH = b.Dx(), b.Dy()
			nw, nh := FitWithin(srcW, srcH, maxDim)
			dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
			xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, stddraw.Src, nil)
			img = dst
		}
		if srcW <= 0 || srcH <= 0 {
			srcW, srcH = b.Dx(), b.Dy()
		}
	}
	return img, ex, srcW, srcH, nil
}

func codecFormatForExt(ext string) (uint32, bool) {
	switch strings.ToLower(ext) {
	case ".heic", ".heif", ".hif", ".cr3":
		return PicFmtHEIC, true
	case ".avif", ".avifs":
		return PicFmtAVIF, true
	case ".jxl":
		return PicFmtJXL, true
	case ".webp":
		return PicFmtWebP, true
	case ".jpg", ".jpeg", ".jpe", ".jfif":
		return PicFmtJPEG, true
	default:
		return 0, false
	}
}

// DecodeSpecial handles HEIC, AVIF, JXL, and camera RAW files.
func DecodeSpecial(data []byte, path string) (image.Image, ExifData, error) {
	ext := strings.ToLower(filepath.Ext(path))
	ex := ExifData{Orientation: 1}
	switch ext {
	case ".heic", ".heif", ".hif":
		img, hex, _, _, err := DecodeCodecImage(PicFmtHEIC, data, 0)
		if err != nil {
			if jpg := LargestEmbeddedJPEG(data); len(jpg) > 0 {
				if jimg, _, jerr := image.Decode(bytes.NewReader(jpg)); jerr == nil {
					return jimg, ParseExif(jpg), nil
				}
			}
			return nil, ex, err
		}
		return img, hex, nil
	case ".avif", ".avifs":
		img, aex, _, _, err := DecodeCodecImage(PicFmtAVIF, data, 0)
		if err != nil {
			return nil, ex, err
		}
		return img, aex, nil
	case ".jxl":
		img, jex, _, _, err := DecodeCodecImage(PicFmtJXL, data, 0)
		if err != nil {
			return nil, ex, err
		}
		return img, jex, nil
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
		if img, _, _, _, err := DecodeCodecImage(PicFmtHEIC, data, 0); err == nil {
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
	return decodeAnimationEx(path, maxDim, rotate, flip, false)
}

func decodeAnimationEx(path string, maxDim, rotate int, flipH, flipV bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, 0, 0, ExifData{}, false, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".gif":
		return decodeGIFEx(data, maxDim, rotate, flipH, flipV)
	case ".png":
		return decodeAPNGEx(data, maxDim, rotate, flipH, flipV)
	case ".heic", ".heif", ".hif", ".avif", ".avifs", ".jxl", ".webp":
		if fmtID, ok := codecFormatForExt(ext); ok {
			imgs, delays, ex, err := DecodeCodecAnimation(fmtID, data)
			if err != nil {
				return nil, nil, nil, 0, 0, ExifData{Orientation: 1}, false, err
			}
			return imagesToAnimationEx(imgs, delays, maxDim, ex, rotate, flipH, flipV)
		}
	}
	return nil, nil, nil, 0, 0, ExifData{}, false, errUnsupportedAnimation
}

// ImagesToAnimation transforms and uploads animation frames as MyGo bitmaps.
func ImagesToAnimation(images []image.Image, delays []time.Duration, maxDim int, ex ExifData, rotate int, flip bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
	return imagesToAnimationEx(images, delays, maxDim, ex, rotate, flip, false)
}

func imagesToAnimationEx(images []image.Image, delays []time.Duration, maxDim int, ex ExifData, rotate int, flipH, flipV bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
	if len(images) == 0 {
		return nil, nil, nil, 0, 0, ex, false, errUnsupportedAnimation
	}
	frames := make([]*ui.Bitmap, 0, len(images))
	var firstImg image.Image
	for i, src := range images {
		img := TransformImageEx(src, ex.Orientation, rotate, flipH, flipV)
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
	return decodeGIFEx(data, maxDim, rotate, flip, false)
}

func decodeGIFEx(data []byte, maxDim, rotate int, flipH, flipV bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
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
	return imagesToAnimationEx(images, delays, maxDim, ExifData{Orientation: 1}, rotate, flipH, flipV)
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

// DecodeAPNG decodes an animated PNG with disposal and blending composition.
func DecodeAPNG(data []byte, maxDim, rotate int, flip bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
	return decodeAPNGEx(data, maxDim, rotate, flip, false)
}

func decodeAPNGEx(data []byte, maxDim, rotate int, flipH, flipV bool) ([]*ui.Bitmap, image.Image, []time.Duration, int, int, ExifData, bool, error) {
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
	return imagesToAnimationEx(images, delays, maxDim, ExifData{Orientation: 1}, rotate, flipH, flipV)
}
